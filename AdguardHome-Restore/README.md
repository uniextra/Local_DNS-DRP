# AdGuardHome-Restore — Monitor DNS

Monitor DNS ligero escrito en Go, sin interfaz web. Comprueba periódicamente varias instancias de AdGuard Home mediante consultas DNS UDP directas y envía notificaciones por Telegram cuando una instancia falla o se recupera.

## Funcionalidades

- Monitoriza varias instancias por IP y puerto DNS.
- Envía un aviso en el primer fallo y otro cuando la instancia vuelve a responder; no repite avisos mientras el estado no cambie.
- Envía mensajes de inicio y parada.
- No publica puertos ni necesita interfaz web.
- Las comprobaciones se envían directamente a cada destino de `TARGETS`.
- El contenedor usa `1.1.1.1` y `8.8.8.8` como resolvers propios para sus conexiones salientes, por ejemplo Telegram.

## Despliegue con Docker Compose

Clona el repositorio y entra en el directorio:

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

**Toda la configuración se introduce directamente como variables de entorno en `docker-compose.yml`. No se utiliza fichero `.env`, variables externas del host ni interpolación `${...}`.**

Edita el bloque `environment` del Compose y sustituye los valores de ejemplo por los tuyos:

```yaml
services:
  adguard-dns-monitor:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: adguard-dns-monitor
    restart: unless-stopped
    environment:
      # DNS targets: comma-separated name=IP[:port] entries
      TARGETS: "adguard-home=192.168.1.2,adguard-backup=192.168.1.3"
      # Telegram notifications
      TELEGRAM_BOT_TOKEN: "YOUR_BOT_TOKEN"
      TELEGRAM_CHAT_ID: "YOUR_CHAT_ID"
      # DNS monitor
      CHECK_INTERVAL: "60"
      DNS_TIMEOUT: "3"
      DNS_TEST_DOMAIN: "google.com"
      TELEGRAM_TIMEOUT: "10"
      # Optional automatic recovery for Askey RTF8316VW
      ROUTER_URL: "http://192.168.1.1"
      ROUTER_USERNAME: "1234"
      ROUTER_PASSWORD: ""
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

Cambia `TARGETS`, `TELEGRAM_BOT_TOKEN` y `TELEGRAM_CHAT_ID`. Para habilitar la recuperación del router, introduce la contraseña web en `ROUTER_PASSWORD`. Si la dejas vacía, la recuperación queda desactivada y el monitor DNS funciona normalmente.

Construye y arranca:

```bash
docker compose up -d --build
docker compose logs -f adguard-dns-monitor
```

Para detenerlo:

```bash
docker compose down
```

## Detalle de las propiedades YAML

| Propiedad | Función |
| --- | --- |
| `services.adguard-dns-monitor` | Define el servicio del monitor. |
| `build.context: .` | Usa el directorio actual como contexto de construcción. |
| `build.dockerfile` | Dockerfile utilizado para construir la imagen. |
| `container_name` | Fija el nombre del contenedor. |
| `restart: unless-stopped` | Reinicia el servicio salvo que se haya detenido manualmente. |
| `environment` | Define directamente las variables de entorno que consume el proceso Go. |
| `dns` | Resolvers del propio contenedor para conexiones salientes; no cambia los DNS de DHCP ni los destinos monitorizados. |
| `security_opt` | Activa `no-new-privileges`. |
| `cap_drop: ALL` | Elimina todas las Linux capabilities. |
| `read_only: true` | Monta el filesystem raíz en solo lectura. |
| `tmpfs` | Crea un `/tmp` temporal de 16 MB en memoria, sin ejecución, setuid ni dispositivos. |
| `logging` | Usa logs JSON, con máximo de 10 MB por archivo y tres archivos conservados. |

## Variables de entorno

| Variable | Valor inicial | Descripción |
| --- | --- | --- |
| `TARGETS` | — | Destinos separados por comas con formato `nombre=IP[:puerto]`. Puerto por defecto: 53. Se admiten IPv6 entre corchetes cuando se especifica puerto. |
| `TELEGRAM_BOT_TOKEN` | `YOUR_BOT_TOKEN` | Token del bot creado con `@BotFather`. |
| `TELEGRAM_CHAT_ID` | `YOUR_CHAT_ID` | ID del usuario, grupo o canal destinatario. |
| `CHECK_INTERVAL` | `60` | Segundos entre comprobaciones. Admite decimales. |
| `DNS_TIMEOUT` | `3` | Timeout de cada consulta DNS, en segundos. Admite decimales. |
| `DNS_TEST_DOMAIN` | `google.com` | Dominio consultado mediante un registro A (IPv4). |
| `TELEGRAM_TIMEOUT` | `10` | Timeout de las peticiones a Telegram, en segundos. |
| `ROUTER_URL` | `http://192.168.1.1` | URL del router Askey. |
| `ROUTER_USERNAME` | `1234` | Usuario web del router. |
| `ROUTER_PASSWORD` | Vacío | Contraseña web. Debe tener valor para activar la recuperación. |
| `DNS_PRIMARY` | `1.1.1.1` | DNS primario que se configura en DHCP. |
| `DNS_SECONDARY` | `8.8.8.8` | DNS secundario que se configura en DHCP. |
| `WIFI_OFF_SECONDS` | `10` | Duración del apagado simultáneo de las radios Wi-Fi. |
| `RECOVERY_COOLDOWN` | `900` | Segundos entre intentos de recuperación. |

## Telegram

1. Crea un bot con `@BotFather` mediante `/newbot`.
2. Copia el token en `TELEGRAM_BOT_TOKEN` dentro del Compose.
3. Inicia una conversación con el bot o añádelo al grupo destinatario.
4. Introduce el ID correspondiente en `TELEGRAM_CHAT_ID`.

## Cómo funciona el monitor DNS

El monitor envía a cada destino una consulta DNS UDP recursiva de tipo A. Considera operativo el servidor si recibe una respuesta DNS con el mismo identificador, no truncada, con código `NOERROR` y al menos una respuesta.

- **Fallo:** notifica en la primera comprobación fallida.
- **Fallo persistente:** no repite el aviso en cada ciclo.
- **Recuperación:** notifica cuando una comprobación vuelve a ser correcta.

El estado se mantiene en memoria; al reiniciar el contenedor, los destinos se evalúan de nuevo desde cero.

## Recuperación automática del router Askey

La recuperación es opcional y se activa únicamente cuando `ROUTER_PASSWORD` tiene valor. Si todos los destinos de `TARGETS` fallan durante tres ciclos consecutivos:

1. Inicia sesión en la interfaz HTTP del Askey RTF8316VW.
2. Actualiza los DNS DHCP preservando la configuración LAN, máscara, estado DHCP y rango DHCP actuales.
3. Vuelve a leer la configuración para verificar los DNS.
4. Lee el estado actual de las radios Wi-Fi de 2,4 GHz y 5 GHz.
5. Desactiva ambas radios simultáneamente, espera `WIFI_OFF_SECONDS` y restaura las que estaban habilitadas.
6. Verifica el estado administrativo y operativo de las radios.
7. Envía el resultado mediante el cliente Telegram existente.

El cooldown predeterminado es de 15 minutos. Los fallos de Telegram no deben impedir la ejecución de la recuperación.

## Red y seguridad

- El host Docker debe poder alcanzar las IP de `TARGETS` por UDP en el puerto configurado.
- El contenedor necesita salida HTTPS (TCP 443) hacia Telegram.
- No se publican puertos.
- El contenedor elimina capabilities, usa filesystem raíz de solo lectura y ejecuta con usuario sin privilegios.
- La propiedad Compose `dns:` configura únicamente los resolvers propios del contenedor.

**Seguridad:** el token de Telegram y la contraseña del router se escriben directamente en `docker-compose.yml`. Mantén el archivo privado y no subas credenciales reales a un repositorio público.
