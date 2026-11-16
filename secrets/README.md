# Local Docker Compose secrets

Create the following files locally before running Docker Compose:

- `telegram_bot_token.txt`: Telegram bot token.
- `telegram_chat_id.txt`: Telegram chat, group, or channel ID.
- `router_password.txt`: Askey web password. Leave this file empty to keep router recovery disabled.

Copy the matching `.example` files, replace the placeholders, and keep the resulting `.txt` files out of Git. The `.txt` files are ignored by the repository.

Linux example:

```sh
cp secrets/telegram_bot_token.example secrets/telegram_bot_token.txt
cp secrets/telegram_chat_id.example secrets/telegram_chat_id.txt
cp secrets/router_password.example secrets/router_password.txt
chmod 0444 secrets/*.txt
```

The container runs as UID/GID `10001:10001`, so the mounted files must be readable by that unprivileged identity. Keep the host-side `secrets` directory restricted to administrators and do not publish or commit its contents.

Compose file-backed secrets are mounted read-only inside the container under `/run/secrets/`. They are not encrypted at rest by Docker Compose; host filesystem permissions remain important. For Docker Swarm, use Swarm-managed secrets instead of local file-backed secrets.
