# AdGuardHome-Restore

Utilities for AdGuard Home backup, restore, and DNS monitoring.

## DNS Monitor (Docker)

A headless Docker service that monitors multiple AdGuard Home DNS instances by IP address and sends Telegram notifications when they fail or recover.

- No web interface and no exposed ports.
- DNS checks go directly to each configured AdGuard Home IP.
- The container uses `1.1.1.1` and `8.8.8.8` as its own DNS resolvers, independently of the Docker host DNS.
- One alert is sent on failure and another on recovery.

## Deployment

### 1. Clone the repository

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

### 2. Configure `docker-compose.yml`

Edit the existing Compose file. All application variables are configured directly under `environment:`; **you do not need to create a `.env` file**.

```yaml
services:
  adguard-dns-monitor:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: adguard-dns-monitor
    restart: unless-stopped
    environment:
      # DNS targets: name=IP[:port], comma-separated
      TARGETS: "home=192.168.1.2,backup=192.168.1.3"
      TELEGRAM_BOT_TOKEN: "YOUR_BOT_TOKEN"
      TELEGRAM_CHAT_ID: "YOUR_CHAT_ID"
      CHECK_INTERVAL: "60"
      DNS_TIMEOUT: "3"
      DNS_TEST_DOMAIN: "google.com"
      TELEGRAM_TIMEOUT: "10"
      LOG_LEVEL: "INFO"
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

Replace the sample DNS IPs, Telegram bot token and chat ID with your own values. The Compose file already exists in the repository; you only need to edit it.

### 3. Build and start

Run from the `AdguardHome-Restore` directory:

```bash
docker compose up -d --build
```

View logs:

```bash
docker compose logs -f
```

Stop the service:

```bash
docker compose down
```

After changing environment values in the YAML, apply them by recreating the container:

```bash
docker compose up -d
```

## Environment variables

Set these values in the Compose `environment:` section. Only `TARGETS`, `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are required.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `TARGETS` | Yes | — | Comma-separated list of DNS targets in `name=IP[:port]` format. Port defaults to `53`; use IP addresses, not hostnames. |
| `TELEGRAM_BOT_TOKEN` | Yes | — | Telegram bot token obtained from `@BotFather`. |
| `TELEGRAM_CHAT_ID` | Yes | — | User, group or channel ID that receives alerts. Group IDs are often negative. |
| `CHECK_INTERVAL` | No | `60` | Seconds between monitoring cycles; integer greater than zero. |
| `DNS_TIMEOUT` | No | `3` | DNS query timeout in seconds; decimals are supported. |
| `DNS_TEST_DOMAIN` | No | `google.com` | Domain queried using an A (IPv4) record request. |
| `TELEGRAM_TIMEOUT` | No | `10` | Telegram HTTP request timeout in seconds; integer greater than zero. |
| `LOG_LEVEL` | No | `INFO` | Log level: `DEBUG`, `INFO`, `WARNING`, `ERROR` or `CRITICAL`. |

### TARGETS format

```yaml
environment:
  TARGETS: "home=192.168.1.2,backup=192.168.1.3"
```

Custom ports and IPv6 are supported. IPv6 addresses must be bracketed when a port is specified, for example `office=[2001:db8::53]:53`. Target IP/port combinations must be unique.

### Telegram configuration

1. Create a bot with `@BotFather` using `/newbot`.
2. Put its token in `TELEGRAM_BOT_TOKEN`.
3. Start a conversation with the bot, or add it to the destination group and allow it to send messages.
4. Put the destination chat/group ID in `TELEGRAM_CHAT_ID`.

**Security note:** credentials are stored in plain text in `docker-compose.yml`. Keep this file private and do not commit real tokens to Git. Anyone with access to the repository or server file can read them.

## DNS behavior

The monitor sends a recursive DNS UDP query for an A record directly to each configured target. A target is healthy only if the response has the matching transaction ID, is not truncated, returns `NOERROR`, and contains at least one answer. It sends an alert on the first failure and a recovery notification when the next successful check occurs.

Docker's `dns:` setting configures the container's own resolver to use Cloudflare (`1.1.1.1`) and Google (`8.8.8.8`), regardless of host DNS. It does not change the host DNS. The health checks themselves go directly to the target IPs in `TARGETS`.

The Docker host must be able to reach target DNS IPs over UDP. The container needs outbound HTTPS (TCP 443) to Telegram.
