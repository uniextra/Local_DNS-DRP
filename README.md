# AdGuardHome-Restore

Utilities for AdGuard Home backup, restore, and DNS monitoring.

## DNS Monitor (Docker)

A headless Docker service that monitors multiple AdGuard Home DNS instances by IP address. It sends Telegram notifications when a DNS server stops responding and when it recovers.

- No web interface and no exposed ports.
- DNS checks go directly to each configured AdGuard Home IP.
- The container uses `1.1.1.1` and `8.8.8.8` as its own DNS resolvers, regardless of the DNS configuration on the Docker host.
- Sends one alert on failure and one notification on recovery (not one alert per failed polling cycle).

## Deployment

### 1. Clone the repository

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

If the repository is already cloned, just change to the `AdguardHome-Restore` directory.

### 2. Create the environment file

Copy the example file:

```bash
cp .env.example .env
```

Edit `.env` and configure the target DNS servers and Telegram credentials:

```dotenv
# AdGuard Home DNS servers: name=IP[:port], separated by commas
TARGETS=adguard-home=192.168.1.2,adguard-backup=192.168.1.3

# Telegram bot credentials
TELEGRAM_BOT_TOKEN=123456789:replace_with_real_token
TELEGRAM_CHAT_ID=123456789

# Monitoring settings
CHECK_INTERVAL=60
DNS_TIMEOUT=3
DNS_TEST_DOMAIN=example.com
TELEGRAM_TIMEOUT=10
LOG_LEVEL=INFO
```

### 3. Create or review `docker-compose.yml`

The repository already includes this file. If you need to create it manually, use:

```yaml
services:
  adguard-dns-monitor:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: adguard-dns-monitor
    restart: unless-stopped
    env_file:
      - .env
    dns:
      - 1.1.1.1
      - 8.8.8.8
    # No ports are exposed: this service has no web interface.
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

Keep `docker-compose.yml`, `Dockerfile`, `monitor.py`, `requirements.txt` and `.env` in the same `AdguardHome-Restore` directory.

### 4. Build and start the container

Run from inside `AdguardHome-Restore`:

```bash
docker compose up -d --build
```

Check the logs:

```bash
docker compose logs -f
```

Stop the service:

```bash
docker compose down
```

After changing only `.env`, recreate the container with `docker compose up -d`. After changing application code or the Dockerfile, rebuild with `docker compose up -d --build`.

## Environment variables

All application settings are supplied through the `.env` file. Only the first three variables are mandatory.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `TARGETS` | Yes | — | DNS servers to monitor. Comma-separated list of `name=IP[:port]`. The port defaults to `53`. Use IP addresses, not hostnames. |
| `TELEGRAM_BOT_TOKEN` | Yes | — | Telegram bot token obtained from `@BotFather`. |
| `TELEGRAM_CHAT_ID` | Yes | — | ID of the Telegram user, group or channel that receives alerts. |
| `CHECK_INTERVAL` | No | `60` | Time between monitoring cycles, in seconds. Integer greater than zero. |
| `DNS_TIMEOUT` | No | `3` | Maximum wait for each DNS query, in seconds. Decimals are supported. |
| `DNS_TEST_DOMAIN` | No | `example.com` | Domain queried during each check. The monitor requests an IPv4 A record. |
| `TELEGRAM_TIMEOUT` | No | `10` | HTTP timeout for Telegram API requests, in seconds. Integer greater than zero. |
| `LOG_LEVEL` | No | `INFO` | Log verbosity: `DEBUG`, `INFO`, `WARNING`, `ERROR` or `CRITICAL`. |

### TARGETS examples

Two AdGuard Home instances using the standard DNS port:

```dotenv
TARGETS=home=192.168.1.2,backup=192.168.1.3
```

Custom DNS port:

```dotenv
TARGETS=home=192.168.1.2:53,testing=192.168.1.10:5353
```

IPv6 is supported. Put the address in square brackets when specifying a port:

```dotenv
TARGETS=office=[2001:db8::53]:53
```

Each target must have a unique IP/port combination. The name before `=` is a label used in logs and Telegram messages.

### Telegram setup

1. Open Telegram and start a conversation with `@BotFather`.
2. Send `/newbot` and follow the instructions.
3. Copy the generated token into `TELEGRAM_BOT_TOKEN`.
4. Open your new bot and press **Start** (or send it a message). For a group, add the bot and allow it to send messages.
5. Set `TELEGRAM_CHAT_ID` to the destination chat or group ID. Group IDs are commonly negative. Obtain the ID from Telegram's bot updates or another trusted Telegram chat-ID lookup method.

Keep the real token private. Never commit the `.env` file to Git.

## How DNS monitoring works

For every target, the monitor sends a DNS UDP query for an A record directly to the configured IP and port. It requests recursion, as a normal DNS client would.

A target is considered healthy only if the response has the matching transaction ID, is not truncated, returns `NOERROR`, and contains at least one answer record.

- **Failure:** one Telegram alert is sent when a target first fails.
- **Continued failure:** no repeated Telegram alert is sent on every polling cycle; the failure remains visible in the logs.
- **Recovery:** a Telegram notification is sent when the target responds successfully again.

The current state is kept in memory. After a container restart, targets are checked again from an initial unknown state.

## Container DNS vs monitored DNS

These are separate:

- **Container DNS:** Compose sets `1.1.1.1` (Cloudflare) and `8.8.8.8` (Google) for the container's own name resolution, such as resolving `api.telegram.org`. This does not change the Docker host's DNS settings.
- **Monitored DNS:** checks are sent directly to the IP addresses in `TARGETS`; they do not use the container's public resolvers.

The Docker host must be able to reach each target IP and DNS port over UDP. The container also needs outbound HTTPS access (TCP 443) to Telegram.

## Security

- Keep `.env` private; it contains the Telegram bot token.
- The container runs as an unprivileged user, drops Linux capabilities and uses a read-only root filesystem.
- No host ports or Docker socket are required.

## Development and tests

From the `AdguardHome-Restore` directory:

```bash
python -m pip install -r requirements.txt
python -m unittest discover -s tests -v
```

For more implementation details, see the [AdguardHome-Restore component README](AdguardHome-Restore/README.md).
