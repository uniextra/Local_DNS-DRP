# AdGuardHome-Restore

Herramienta ligera escrita en Go para monitorizar instancias de AdGuard Home y recuperar automáticamente el DNS mediante un router Askey RTF8316VW cuando todos los DNS monitorizados dejan de responder.

## Funcionalidades

- Monitorización de varias instancias AdGuard Home mediante consultas DNS UDP directas.
- Notificaciones Telegram al detectar fallos y recuperaciones.
- Recuperación opcional del router Askey cuando todos los destinos fallan durante tres comprobaciones consecutivas.
- Actualización de los DNS DHCP y reinicio simultáneo de las radios Wi-Fi de 2,4 y 5 GHz.
- Cooldown para evitar ejecuciones repetidas durante una caída prolongada.
- Contenedor endurecido, sin puertos publicados, sin capabilities y con filesystem raíz de solo lectura.

## Despliegue

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

Toda la configuración se realiza directamente en la sección `environment` de `docker-compose.yml`. **No se utiliza fichero `.env` ni interpolación de variables**. Edita los valores de `TARGETS`, `TELEGRAM_BOT_TOKEN` y `TELEGRAM_CHAT_ID`. Para activar la recuperación Askey, introduce la contraseña web en `ROUTER_PASSWORD`; si queda vacía, la recuperación queda desactivada.

```bash
docker compose up -d --build
docker compose logs -f adguard-dns-monitor
```

## Configuración del Docker Compose

| Variable | Ejemplo / valor inicial | Descripción |
| --- | --- | --- |
| `TARGETS` | `adguard-home=192.168.1.2,adguard-backup=192.168.1.3` | Destinos DNS separados por comas, formato `nombre=IP[:puerto]`. |
| `TELEGRAM_BOT_TOKEN` | `YOUR_BOT_TOKEN` | Token del bot de Telegram. |
| `TELEGRAM_CHAT_ID` | `YOUR_CHAT_ID` | ID del usuario, grupo o canal destinatario. |
| `CHECK_INTERVAL` | `60` | Intervalo entre comprobaciones, en segundos. |
| `DNS_TIMEOUT` | `3` | Timeout de cada consulta DNS, en segundos. |
| `DNS_TEST_DOMAIN` | `google.com` | Dominio consultado mediante registro A. |
| `TELEGRAM_TIMEOUT` | `10` | Timeout de las peticiones a Telegram, en segundos. |
| `ROUTER_URL` | `http://192.168.1.1` | Dirección del router Askey. |
| `ROUTER_USERNAME` | `1234` | Usuario web del router. |
| `ROUTER_PASSWORD` | Vacío | Contraseña web. Su presencia activa la recuperación automática. |
| `DNS_PRIMARY` | `1.1.1.1` | DNS primario configurado en DHCP. |
| `DNS_SECONDARY` | `8.8.8.8` | DNS secundario configurado en DHCP. |
| `WIFI_OFF_SECONDS` | `10` | Tiempo que ambas radios permanecen apagadas. |
| `RECOVERY_COOLDOWN` | `900` | Tiempo mínimo entre intentos de recuperación, en segundos. |

### Otras propiedades YAML

- `build`: construye la imagen con el Dockerfile del proyecto.
- `container_name`: nombre fijo del contenedor.
- `restart: unless-stopped`: reinicia el contenedor salvo parada manual.
- `dns`: resolvers usados por el contenedor para sus propias conexiones salientes, por ejemplo Telegram. No cambia los DNS DHCP ni los destinos monitorizados.
- `security_opt: no-new-privileges:true`: impide obtener privilegios adicionales.
- `cap_drop: ALL`: elimina las Linux capabilities.
- `read_only: true`: filesystem raíz de solo lectura.
- `tmpfs`: directorio temporal de 16 MB en memoria.
- `logging`: logs JSON limitados a 10 MB por archivo y tres archivos conservados.

## Recuperación automática Askey

La recuperación se activa cuando `ROUTER_PASSWORD` contiene la contraseña del Askey RTF8316VW. Si todos los destinos de `TARGETS` fallan durante tres ciclos consecutivos, el servicio inicia sesión en el router, actualiza y verifica los DNS DHCP, apaga simultáneamente las radios Wi-Fi de 2,4 y 5 GHz durante `WIFI_OFF_SECONDS`, las restaura y comprueba su estado operativo. El resultado se notifica usando el cliente Telegram existente.

Se aplica el cooldown configurado en `RECOVERY_COOLDOWN` (900 segundos por defecto). Si la contraseña está vacía, el monitor DNS sigue funcionando sin ejecutar cambios en el router.

Consulta [AdguardHome-Restore/README.md](AdguardHome-Restore/README.md) para el detalle del funcionamiento DNS, sintaxis de destinos, Telegram, YAML y seguridad.

**Seguridad:** los tokens y la contraseña del router se escriben directamente en `docker-compose.yml`. Mantén el archivo privado y no publiques credenciales reales.
