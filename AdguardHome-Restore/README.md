# AdGuard Home DNS Monitor

A lightweight, headless Docker service that checks multiple AdGuard Home DNS
instances and sends Telegram notifications when an instance fails or recovers.

## Features

- Monitors multiple DNS servers by IP address and UDP port.
- Sends an alert on the first failed check and another when service recovers.
- Sends a startup and shutdown notification.
- No web interface and no published ports.
- Uses container DNS resolvers `1.1.1.1` and `8.8.8.8` for the monitor's
  own hostname resolution (for example, Telegram API requests), independently
  of the host's configured DNS.
- DNS health checks are sent directly to each configured target IP, not through
  the container's resolver.

## Quick start

1. Copy `.env.example` to `.env`.
2. Set the target AdGuard Home IPs and Telegram credentials.
3. Start the service:

   ```bash
   docker compose up -d --build
   ```

4. Follow logs:

   ```bash
   docker compose logs -f
   ```

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `TARGETS` | Yes | — | Comma-separated `name=IP[:port]` entries. Port defaults to 53. |
| `TELEGRAM_BOT_TOKEN` | Yes | — | Telegram bot token from BotFather. |
| `TELEGRAM_CHAT_ID` | Yes | — | Chat or group ID receiving alerts. |
| `CHECK_INTERVAL` | No | `60` | Seconds between monitoring cycles. |
| `DNS_TIMEOUT` | No | `3` | Timeout per DNS query, in seconds. |
| `DNS_TEST_DOMAIN` | No | `example.com` | Name queried with an A record request. |
| `TELEGRAM_TIMEOUT` | No | `10` | Telegram HTTP request timeout, in seconds. |
| `LOG_LEVEL` | No | `INFO` | Python log level. |

### Target examples

```dotenv
TARGETS=home=192.168.1.2,remote=10.0.0.53
```

A custom DNS port can be specified:

```dotenv
TARGETS=home=192.168.1.2:5353
```

IPv6 addresses are supported:

```dotenv
TARGETS=office=[2001:db8::53]:53
```

## DNS check behavior

For each target, the monitor sends a DNS A query directly to the configured IP
and UDP port. The target is considered healthy only when it returns a matching
DNS transaction ID, a non-truncated response, the `NOERROR` response code,
and at least one answer record.

The monitor reports state changes rather than sending a Telegram message on
every failed polling cycle. A failed target sends one alert; a recovery sends
one recovery notification.

## Docker DNS configuration

Compose explicitly configures the container resolvers to Cloudflare
(`1.1.1.1`) and Google (`8.8.8.8`). This configures DNS resolution performed
by the container itself, and does not change the DNS configuration of the host.

The health check for an AdGuard Home target is independent of those resolvers:
it sends the query directly to the target IP.

## Security notes

- Keep `.env` private; it contains the Telegram bot token.
- The container runs as an unprivileged user, drops Linux capabilities, and
  uses a read-only root filesystem.
- No host ports or Docker socket are required.

## Development

Install dependencies and run:

```bash
python -m pip install -r requirements.txt
TARGETS=local=127.0.0.1 TELEGRAM_BOT_TOKEN=test TELEGRAM_CHAT_ID=test python monitor.py
```
