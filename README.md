# Local_DNS-DRP

Disaster recovery monitor for local DNS servers (such as **AdGuard Home** or **Pi-hole**).

When all monitored local DNS instances go down while public resolvers are still reachable, **Local_DNS-DRP** automatically updates your router's DHCP DNS settings to known working resolvers (`1.1.1.1` and `8.8.8.8` by default) so the rest of your network stays online, and notifies you via Telegram.

The service exposes no ports or web UI and runs as a hardened multi-architecture Docker container (`linux/amd64` and `linux/arm64`).

## Supported Routers

This project is designed to grow with community-contributed router adapters behind a common `RouterRecovery` interface:

| `ROUTER_MODEL` | Router / Firmware | Status | Recovery Actions |
| --- | --- | --- | --- |
| `askey-rtf8316vw` | Movistar / Askey RTF8316VW | **Supported** | Updates DHCP DNS + restarts 2.4/5 GHz Wi-Fi radios |
| `asus-generic` | AsusWrt / AsusWrt-Merlin | **Experimental** | Updates DHCP DNS + restarts `dnsmasq` (and Wi-Fi if enabled) |

Want to add your router? See [CONTRIBUTING.md](CONTRIBUTING.md).

## How It Works

1. Sends direct UDP `A`-record queries to every IP in `TARGETS` (no hostname resolution dependency).
2. Sends Telegram alerts on state transitions (down / recovered), startup, and shutdown.
3. If **all** targets fail for **3 consecutive checks** and `ROUTER_PASSWORD` is set:
   - Probes `DNS_PRIMARY` / `DNS_SECONDARY` first; if they also fail, assumes an ISP/WAN outage and skips router changes.
   - Otherwise, logs into the configured `ROUTER_MODEL`, sets DHCP DNS to `DNS_PRIMARY` and `DNS_SECONDARY`, verifies the change, and enforces `RECOVERY_COOLDOWN` (15 min default).
   - After a successful recovery, it does not repeat router changes until at least one target recovers, avoiding repeated Wi-Fi restarts during a prolonged outage.

For the full flow diagram and DNS validation details, see [docs/architecture.md](docs/architecture.md).

## Quick Start

### 1. Prerequisites

- Docker Engine & Docker Compose
- UDP reachability from the container to `TARGETS`
- Outbound HTTPS (TCP 443) to `api.telegram.org`
- *(Optional)* Local HTTP/HTTPS access and web credentials for a supported router

### 2. Configure `docker-compose.yml`

```bash
git clone https://github.com/uniextra/Local_DNS-DRP.git
cd Local_DNS-DRP
```

Or copy this ready-to-use `docker-compose.yml` (no clone required, pulls the multi-arch image directly from Docker Hub):

```yaml
services:
  adguard-dns-monitor:
    image: uniextra/local-dns-drp:latest
    pull_policy: always
    container_name: adguard-dns-monitor
    restart: unless-stopped
    environment:
      # DNS targets: comma-separated name=IP[:port] entries
      TARGETS: "adguard-home=192.168.1.2,adguard-backup=192.168.1.3"

      # Telegram credentials
      TELEGRAM_BOT_TOKEN: "YOUR_TELEGRAM_BOT_TOKEN"
      TELEGRAM_CHAT_ID: "YOUR_TELEGRAM_CHAT_ID"

      # DNS monitor
      CHECK_INTERVAL: "60"
      DNS_TIMEOUT: "3"
      DNS_TEST_DOMAIN: "google.com"
      TELEGRAM_TIMEOUT: "10"

      # Optional automatic router recovery (askey-rtf8316vw or asus-generic)
      ROUTER_MODEL: "askey-rtf8316vw"
      ROUTER_URL: "http://192.168.1.1"
      ROUTER_USERNAME: "1234"
      ROUTER_PASSWORD: "" # Set the router web password to enable recovery
      DNS_PRIMARY: "1.1.1.1"
      DNS_SECONDARY: "8.8.8.8"
      WIFI_OFF_SECONDS: "10"
      RECOVERY_COOLDOWN: "900"
    dns:
      - 1.1.1.1
      - 8.8.8.8
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    read_only: true
    tmpfs:
      - /tmp:size=16m,noexec,nosuid,nodev
    logging:
      driver: json-file
      options:
        max-size: 10m
        max-file: "3"
```

Edit `environment` in [`docker-compose.yml`](docker-compose.yml):

- `TARGETS`: comma-separated `name=IP[:port]` entries (e.g. `adguard-home=192.168.1.2,adguard-backup=192.168.1.3`).
- `TELEGRAM_BOT_TOKEN` & `TELEGRAM_CHAT_ID`: your `@BotFather` token and target chat ID.
- `ROUTER_MODEL`: `askey-rtf8316vw` (default) or `asus-generic`.
- `ROUTER_PASSWORD`: router web password. Leave empty to run as a DNS monitor with Telegram alerts only.

> [!CAUTION]
> Keep your `docker-compose.yml` private and never commit real tokens or router passwords to a public repository.

### 3. Deploy

```bash
docker compose config
docker compose pull
docker compose up -d
docker compose logs -f adguard-dns-monitor
```

Stop with `docker compose down`.

## Configuration Reference

### Monitor & Telegram

| Variable | Default / Example | Description |
| --- | --- | --- |
| `TARGETS` | *(required)* | Comma-separated `name=IP[:port]` list (default port `53`, IPv4/IPv6 IPs only). |
| `TELEGRAM_BOT_TOKEN` | *(required)* | Bot token from `@BotFather`. |
| `TELEGRAM_CHAT_ID` | *(required)* | Destination user, group, or channel ID. |
| `CHECK_INTERVAL` | `60` | Seconds between check cycles (supports decimals). |
| `DNS_TIMEOUT` | `3` | Per-query UDP timeout in seconds. |
| `DNS_TEST_DOMAIN` | `google.com` | Domain queried via `A` record. |
| `TELEGRAM_TIMEOUT` | `10` | HTTP timeout for Telegram API calls in seconds. |
| `TARGET_FAILURE_THRESHOLD` | `2` | Consecutive failed checks before a single target is reported as down (1–10). |

### Router Recovery (Optional)

| Variable | Default | Description |
| --- | --- | --- |
| `ROUTER_PASSWORD` | *(empty)* | Enables automatic router recovery when non-empty. |
| `ROUTER_MODEL` | `askey-rtf8316vw` | Adapter selector (`askey-rtf8316vw`, `asus-generic`). |
| `ROUTER_URL` | `http://192.168.1.1` | Router management URL (`http://` or `https://`). |
| `ROUTER_USERNAME` | `1234` | Router web username. |
| `DNS_PRIMARY` | `1.1.1.1` | Fallback primary DNS written to router DHCP. |
| `DNS_SECONDARY` | `8.8.8.8` | Fallback secondary DNS written to router DHCP. |
| `WIFI_OFF_SECONDS` | `10` | Seconds Wi-Fi radios stay off during Askey recovery. |
| `RECOVERY_COOLDOWN` | `900` | Minimum seconds between recovery runs (15 minutes). |

## Project Structure

```text
.
├── cmd/dns-monitor/        # Binary entrypoint and signal handling
├── internal/
│   ├── config/             # Environment loading and validation
│   ├── dns/                # UDP A-record checks and CNAME chain validation
│   ├── monitor/            # Polling loop, fallback DNS probe, and cooldown
│   ├── notify/             # Telegram client (message truncation and token redaction)
│   └── recovery/           # RouterRecovery interface, Askey & AsusWrt adapters
├── docs/                   # Architecture and AI-assisted router discovery guides
├── Dockerfile              # Static multi-arch build (scratch, linux/amd64 & arm64)
├── docker-compose.yml      # Hardened Compose deployment
└── CONTRIBUTING.md         # Guide for adding new router adapters
```

## Development & Contributing

- **Architecture & DNS validation:** [docs/architecture.md](docs/architecture.md)
- **Add a new router adapter:** [CONTRIBUTING.md](CONTRIBUTING.md)
- **License:** [MIT](LICENSE)
