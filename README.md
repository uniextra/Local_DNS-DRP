# AdGuardHome-Restore

Utilities for AdGuard Home backup, restore, and DNS monitoring.

## DNS Monitor (Go / Docker)

A lightweight, headless Go service that monitors multiple AdGuard Home DNS instances using direct UDP DNS queries and sends Telegram notifications on failure and recovery.

- No web interface and no exposed ports.
- Checks each configured DNS IP directly.
- Sends one alert when a target fails and one when it recovers; no repeated alerts while its state remains unchanged.
- Sends startup and shutdown notifications.
- Container DNS resolvers (`1.1.1.1` and `8.8.8.8`) are used only for the container's own outbound name resolution, such as Telegram.

## Deployment

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

Edit `docker-compose.yml` and configure `TARGETS`, `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID`. All settings are configured directly in Compose; no `.env` file is required.

```bash
docker compose up -d --build
docker compose logs -f
```

See [AdguardHome-Restore/README.md](AdguardHome-Restore/README.md) for configuration, target syntax, Telegram setup, DNS behavior and security details.
