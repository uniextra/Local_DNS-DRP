#!/usr/bin/env python3
"""Monitor AdGuard Home DNS endpoints and report state changes to Telegram."""

from __future__ import annotations

import ipaddress
import logging
import os
import signal
import sys
import time
from dataclasses import dataclass
from typing import Optional

import dns.exception
import dns.message
import dns.query
import dns.rcode
import dns.rdatatype
import requests


LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()
logging.basicConfig(
    level=getattr(logging, LOG_LEVEL, logging.INFO),
    format="%(asctime)s %(levelname)s %(message)s",
)
logger = logging.getLogger("adguard-monitor")


def env_int(name: str, default: int, minimum: int = 1) -> int:
    raw = os.getenv(name, str(default))
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value < minimum:
        raise ValueError(f"{name} must be >= {minimum}")
    return value


@dataclass(frozen=True)
class Target:
    name: str
    address: str
    port: int


def parse_targets() -> list[Target]:
    """Parse TARGETS as comma-separated name=IP[:port] entries."""
    raw = os.getenv("TARGETS", "").strip()
    if not raw:
        raise ValueError("TARGETS is required (example: router=192.168.1.2)")

    targets: list[Target] = []
    seen: set[tuple[str, int]] = set()
    for item in raw.split(","):
        item = item.strip()
        if not item:
            continue
        if "=" in item:
            name, endpoint = (part.strip() for part in item.split("=", 1))
        else:
            endpoint = item
            name = item
        if not name or not endpoint:
            raise ValueError(f"Invalid TARGETS entry: {item!r}")

        # IPv4 and IPv6 are supported. Bracket IPv6 when specifying a port.
        if endpoint.startswith("["):
            closing = endpoint.find("]")
            if closing == -1:
                raise ValueError(f"Invalid bracketed IPv6 endpoint: {endpoint}")
            address = endpoint[1:closing]
            suffix = endpoint[closing + 1 :]
            if suffix and not suffix.startswith(":"):
                raise ValueError(f"Invalid endpoint: {endpoint}")
            port = int(suffix[1:]) if suffix else 53
        else:
            # An unbracketed IPv6 literal is treated as an address on port 53.
            if endpoint.count(":") > 1:
                address, port = endpoint, 53
            elif ":" in endpoint:
                address, port_text = endpoint.rsplit(":", 1)
                port = int(port_text)
            else:
                address, port = endpoint, 53

        if not address or not (1 <= port <= 65535):
            raise ValueError(f"Invalid endpoint: {endpoint}")
        try:
            address = str(ipaddress.ip_address(address))
        except ValueError as exc:
            raise ValueError(f"TARGETS must contain IP addresses, not hostnames: {address}") from exc
        key = (address, port)
        if key in seen:
            raise ValueError(f"Duplicate DNS endpoint: {endpoint}")
        seen.add(key)
        targets.append(Target(name=name, address=address, port=port))

    if not targets:
        raise ValueError("TARGETS contains no valid endpoints")
    return targets


class Telegram:
    def __init__(self, token: str, chat_id: str, timeout: int) -> None:
        self.token = token
        self.chat_id = chat_id
        self.timeout = timeout
        self.session = requests.Session()
        self.url = f"https://api.telegram.org/bot{token}/sendMessage"

    def send(self, text: str) -> bool:
        try:
            response = self.session.post(
                self.url,
                json={
                    "chat_id": self.chat_id,
                    "text": text,
                    "disable_web_page_preview": True,
                },
                timeout=self.timeout,
            )
            response.raise_for_status()
            payload = response.json()
            if not payload.get("ok"):
                logger.error("Telegram API returned an unsuccessful response")
                return False
            return True
        except (requests.RequestException, ValueError) as exc:
            logger.error("Unable to send Telegram notification: %s", exc)
            return False


def check_dns(target: Target, query_name: str, timeout: float) -> tuple[bool, str]:
    """Send a DNS A query directly to the target and require a valid answer."""
    try:
        # make_query() sets RD (recursion desired), like a normal DNS client.
        request = dns.message.make_query(query_name, dns.rdatatype.A)
        response = dns.query.udp(
            request,
            target.address,
            port=target.port,
            timeout=timeout,
        )
        if response.id != request.id:
            return False, "DNS response ID mismatch"
        if response.flags & dns.flags.TC:
            return False, "truncated DNS response"
        rcode = response.rcode()
        if rcode != dns.rcode.NOERROR:
            return False, f"DNS response code: {dns.rcode.to_text(rcode)}"
        if not response.answer:
            return False, "DNS response contains no answer"
        return True, "DNS query successful"
    except (dns.exception.DNSException, OSError, TimeoutError) as exc:
        return False, f"{type(exc).__name__}: {exc}"


def main() -> int:
    targets = parse_targets()
    token = os.getenv("TELEGRAM_BOT_TOKEN", "").strip()
    chat_id = os.getenv("TELEGRAM_CHAT_ID", "").strip()
    if not token or not chat_id:
        raise ValueError("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required")

    interval = env_int("CHECK_INTERVAL", 60)
    timeout = float(os.getenv("DNS_TIMEOUT", "3"))
    if timeout <= 0:
        raise ValueError("DNS_TIMEOUT must be > 0")
    telegram_timeout = env_int("TELEGRAM_TIMEOUT", 10)
    query_name = os.getenv("DNS_TEST_DOMAIN", "example.com").strip().rstrip(".")
    if not query_name:
        raise ValueError("DNS_TEST_DOMAIN cannot be empty")

    notifier = Telegram(token, chat_id, telegram_timeout)
    states: dict[tuple[str, int], Optional[bool]] = {
        (target.address, target.port): None for target in targets
    }
    running = True

    def stop_handler(_signum: int, _frame: object) -> None:
        nonlocal running
        running = False

    signal.signal(signal.SIGTERM, stop_handler)
    signal.signal(signal.SIGINT, stop_handler)

    logger.info(
        "Starting AdGuard Home monitor: %d target(s), interval=%ss, query=%s",
        len(targets),
        interval,
        query_name,
    )
    for target in targets:
        logger.info("Configured target: %s (%s:%s)", target.name, target.address, target.port)

    # Initial notification gives immediate confirmation that configuration works.
    notifier.send(
        "🟢 AdGuard DNS Monitor iniciado\n"
        + "\n".join(f"• {t.name}: {t.address}:{t.port}" for t in targets)
    )

    while running:
        cycle_started = time.monotonic()
        for target in targets:
            if not running:
                break
            key = (target.address, target.port)
            healthy, detail = check_dns(target, query_name, timeout)
            previous = states[key]
            states[key] = healthy

            if healthy:
                logger.info("%s (%s:%s): OK", target.name, target.address, target.port)
                if previous is False:
                    notifier.send(
                        f"🟢 AdGuard DNS RECUPERADO\n"
                        f"Instancia: {target.name}\n"
                        f"IP: {target.address}:{target.port}\n"
                        f"Prueba: {query_name}\n"
                        f"Estado: DNS operativo"
                    )
            else:
                logger.warning(
                    "%s (%s:%s): FAILED - %s",
                    target.name,
                    target.address,
                    target.port,
                    detail,
                )
                # Alert on first failed check and subsequent recovery transitions only.
                if previous is not False:
                    notifier.send(
                        f"🔴 FALLO AdGuard DNS\n"
                        f"Instancia: {target.name}\n"
                        f"IP: {target.address}:{target.port}\n"
                        f"Prueba: {query_name}\n"
                        f"Error: {detail}"
                    )

        elapsed = time.monotonic() - cycle_started
        sleep_for = max(0.0, interval - elapsed)
        end = time.monotonic() + sleep_for
        while running and time.monotonic() < end:
            time.sleep(min(0.5, end - time.monotonic()))

    logger.info("Stopping AdGuard Home monitor")
    notifier.send("⚪ AdGuard DNS Monitor detenido")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        logger.exception("Fatal configuration/runtime error: %s", exc)
        sys.exit(1)
