# AdGuardHome-Restore — Monitor DNS

Monitor DNS ligero escrito en Go, sin interfaz web. Comprueba periódicamente varias instancias de AdGuard Home mediante consultas DNS UDP directas y envía notificaciones por Telegram cuando una instancia falla o se recupera.

## Funcionalidades

- Monitoriza varias instancias por IP y puerto DNS.
- Envía un aviso en el primer fallo y otro cuando la instancia vuelve a responder; no repite avisos mientras el estado no cambie.
- Envía un mensaje de inicio con los destinos monitorizados y otro al detenerse.
- No publica puertos ni necesita interfaz web.
- Las comprobaciones se envían directamente a cada destino de `TARGETS`.
- El contenedor usa `1.1.1.1` y `8.8.8.8` como resolvers propios para sus conexiones salientes, por ejemplo Telegram.

## Despliegue con Docker Compose

Clona el repositorio y entra en el directorio:

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

Edita `docker-compose.yml` y sustituye las IP, el token del bot y el Chat ID. No hace falta crear un fichero `.env`.

```yaml
environment:
  TARGETS: "adguard-home=192.168.1.2,adguard-backup=192.168.1.3"
  TELEGRAM_BOT_TOKEN: "YOUR_BOT_TOKEN"
  TELEGRAM_CHAT_ID: "YOUR_CHAT_ID"
  CHECK_INTERVAL: "60"
  DNS_TIMEOUT: "3"
  DNS_TEST_DOMAIN: "google.com"
  TELEGRAM_TIMEOUT: "10"
```

Construye y arranca:

```bash
docker compose up -d --build
docker compose logs -f
```

Para detenerlo:

```bash
docker compose down
```

## Variables de entorno

| Variable | Obligatoria | Por defecto | Descripción |
| --- | --- | --- | --- |
| `TARGETS` | Sí | — | Lista separada por comas con formato `nombre=IP[:puerto]`. Puerto por defecto: `53`. Deben ser IPs, no hostnames. |
| `TELEGRAM_BOT_TOKEN` | Sí | — | Token del bot creado con `@BotFather`. |
| `TELEGRAM_CHAT_ID` | Sí | — | ID del usuario, grupo o canal que recibe los avisos. |
| `CHECK_INTERVAL` | No | `60` | Segundos entre comprobaciones. Admite decimales. |
| `DNS_TIMEOUT` | No | `3` | Timeout de cada consulta DNS, en segundos. Admite decimales. |
| `DNS_TEST_DOMAIN` | No | `google.com` | Dominio consultado mediante un registro A (IPv4). |
| `TELEGRAM_TIMEOUT` | No | `10` | Timeout de las peticiones a Telegram, en segundos. Admite decimales. |

### Ejemplos de TARGETS

```yaml
TARGETS: "home=192.168.1.2,backup=192.168.1.3"
```

Se admiten puertos personalizados y direcciones IPv6. Para IPv6 con puerto, usa corchetes:

```yaml
TARGETS: "home=192.168.1.2:53,lab=192.168.1.10:5353,office=[2001:db8::53]:53"
```

Cada combinación IP/puerto debe ser única.

## Telegram

1. Crea un bot con `@BotFather` mediante `/newbot`.
2. Configura el token en `TELEGRAM_BOT_TOKEN`.
3. Inicia una conversación con el bot o añádelo al grupo destinatario.
4. Configura el ID correspondiente en `TELEGRAM_CHAT_ID`.

**Seguridad:** las credenciales se guardan en texto plano en `docker-compose.yml`. Mantén el fichero privado y no subas tokens reales al repositorio.

## Cómo funciona

El monitor envía a cada destino una consulta DNS UDP recursiva de tipo A. Considera operativo el servidor si recibe una respuesta DNS con el mismo identificador, no truncada, con código `NOERROR` y al menos una respuesta.

- **Fallo:** notifica en la primera comprobación fallida.
- **Fallo persistente:** no repite el aviso en cada ciclo.
- **Recuperación:** notifica cuando una comprobación vuelve a ser correcta.

El estado se mantiene en memoria; al reiniciar el contenedor, los destinos se evalúan de nuevo desde cero.

## Red y seguridad del contenedor

- El host Docker debe poder alcanzar las IP de `TARGETS` por UDP en el puerto configurado.
- El contenedor necesita salida HTTPS (TCP 443) hacia Telegram.
- El contenedor usa un usuario sin privilegios, elimina capabilities, tiene filesystem raíz de solo lectura y no expone puertos.
- Los DNS definidos en la propiedad Compose `dns:` solo son los resolvers propios del contenedor; no sustituyen ni modifican los destinos comprobados por el monitor.


## Recuperación automática del router Askey

La recuperación es opcional y se activa únicamente cuando se define `ROUTER_PASSWORD`. El monitor conserva su funcionamiento DNS y Telegram existente. Si **todos** los destinos de `TARGETS` fallan durante tres ciclos consecutivos, ejecuta la recuperación del router Askey RTF8316VW:

1. Inicia sesión en la interfaz HTTP del router.
2. Actualiza los DNS del servidor DHCP preservando la IP LAN, máscara, estado DHCP y rango DHCP actuales.
3. Vuelve a leer la configuración para verificar los DNS.
4. Lee la configuración actual de las radios 2,4 GHz y 5 GHz.
5. Desactiva ambas radios, espera 10 segundos y vuelve a activar las que estaban activas.
6. Verifica estado administrativo y operativo de ambas radios.
7. Notifica por el mismo cliente Telegram el inicio y resultado de la recuperación.

Se aplica un cooldown de 15 minutos entre intentos para evitar reinicios repetidos durante una caída prolongada. Los fallos de Telegram no bloquean la recuperación.

### Variables de recuperación

| Variable | Obligatoria | Por defecto | Descripción |
| --- | --- | --- | --- |
| `ROUTER_PASSWORD` | Sí, para activar | — | Contraseña web del router. Si está vacía, la recuperación queda desactivada. |
| `ROUTER_URL` | No | `http://192.168.1.1` | URL del Askey. |
| `ROUTER_USERNAME` | No | `1234` | Usuario de la interfaz web. |
| `DNS_PRIMARY` | No | `1.1.1.1` | DNS primario que se configura en DHCP. |
| `DNS_SECONDARY` | No | `8.8.8.8` | DNS secundario que se configura en DHCP. |
| `WIFI_OFF_SECONDS` | No | `10` | Tiempo con ambas radios apagadas. |
| `RECOVERY_COOLDOWN` | No | `900` | Segundos entre intentos de recuperación. |

Configura el secreto en el entorno del contenedor o en un fichero `.env` local no versionado. No escribas contraseñas reales en el Compose ni las subas a Git.

Ejemplo de `.env` local:

```dotenv
ROUTER_PASSWORD=your-router-password
ROUTER_URL=http://192.168.1.1
ROUTER_USERNAME=1234
DNS_PRIMARY=1.1.1.1
DNS_SECONDARY=8.8.8.8
WIFI_OFF_SECONDS=10
RECOVERY_COOLDOWN=900
```

> La integración está implementada para el Askey RTF8316VW y sus endpoints observados. Debe probarse en el router real antes de considerarla una recuperación de producción; no se simula ni se afirma una validación contra hardware desde CI.
