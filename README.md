# AdGuardHome-Restore

Monitor DNS ligero escrito en Go para comprobar varias instancias de AdGuard Home y ejecutar una recuperación automática del router Askey RTF8316VW cuando todos los servidores DNS monitorizados dejan de responder.

El servicio no tiene interfaz web ni publica puertos. Se ejecuta como contenedor Docker y utiliza Telegram para notificar los cambios de estado y los resultados de recuperación.

## Funcionalidades

- Monitoriza varias instancias DNS mediante consultas UDP directas a las IP configuradas.
- Envía una notificación cuando un destino falla y otra cuando se recupera, sin repetir avisos mientras el estado no cambia.
- Envía notificaciones de inicio y parada.
- Activa opcionalmente la recuperación del Askey RTF8316VW cuando **todos** los destinos configurados están caídos durante tres comprobaciones consecutivas.
- Actualiza los DNS del servidor DHCP del router preservando la configuración LAN y el rango DHCP existente.
- Reinicia simultáneamente las radios Wi-Fi de 2,4 GHz y 5 GHz durante 10 segundos (configurable).
- Verifica la configuración DNS y el estado de las radios después del cambio.
- Aplica un cooldown para evitar recuperaciones repetidas durante una caída prolongada.
- Reutiliza el cliente Telegram existente para informar del inicio y resultado de la recuperación.

## Requisitos

- Docker Engine y Docker Compose.
- Conectividad desde el contenedor hacia los destinos de `TARGETS` mediante UDP en el puerto DNS configurado.
- Conectividad HTTPS saliente (TCP 443) hacia Telegram.
- Para utilizar la recuperación automática: un router Askey RTF8316VW accesible desde el contenedor y sus credenciales web.

## Despliegue

Clona el repositorio y entra en el directorio del servicio:

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

Toda la configuración se realiza **directamente en la sección `environment` de `docker-compose.yml`**. No se utiliza fichero `.env`, interpolación `${...}` ni variables externas del host.

Edita los valores de ejemplo en el Compose:

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

Sustituye:

- `TARGETS`: las IP y puertos de tus instancias AdGuard Home.
- `TELEGRAM_BOT_TOKEN` y `TELEGRAM_CHAT_ID`: las credenciales del bot y el destinatario.
- `ROUTER_PASSWORD`: la contraseña web del router para habilitar la recuperación automática.

Si `ROUTER_PASSWORD` está vacía, la recuperación del Askey queda desactivada. El monitor DNS y las notificaciones habituales continúan funcionando.

Valida la configuración y arranca el contenedor:

```bash
docker compose config
docker compose up -d --build
docker compose logs -f adguard-dns-monitor
```

Para detenerlo:

```bash
docker compose down
```

## Variables de entorno

Todas estas variables se definen directamente dentro de `environment` en `docker-compose.yml`.

### Monitor DNS y Telegram

| Variable | Valor de ejemplo | Descripción |
| --- | --- | --- |
| `TARGETS` | `adguard-home=192.168.1.2,adguard-backup=192.168.1.3` | Lista de destinos separados por comas con formato `nombre=IP[:puerto]`. El puerto por defecto es 53. Deben utilizarse direcciones IP, no hostnames. |
| `TELEGRAM_BOT_TOKEN` | `YOUR_BOT_TOKEN` | Token del bot creado con `@BotFather`. |
| `TELEGRAM_CHAT_ID` | `YOUR_CHAT_ID` | ID del usuario, grupo o canal que recibe las notificaciones. |
| `CHECK_INTERVAL` | `60` | Intervalo entre comprobaciones, en segundos. Admite decimales. |
| `DNS_TIMEOUT` | `3` | Timeout de cada consulta DNS, en segundos. Admite decimales. |
| `DNS_TEST_DOMAIN` | `google.com` | Dominio consultado mediante un registro A (IPv4). |
| `TELEGRAM_TIMEOUT` | `10` | Timeout de las peticiones a Telegram, en segundos. Admite decimales. |

Ejemplo de destinos con puertos personalizados e IPv6:

```yaml
TARGETS: "home=192.168.1.2:53,lab=192.168.1.10:5353,office=[2001:db8::53]:53"
```

Cada combinación IP/puerto debe ser única.

### Recuperación del router Askey

| Variable | Valor de ejemplo | Descripción |
| --- | --- | --- |
| `ROUTER_URL` | `http://192.168.1.1` | URL de administración del router Askey. |
| `ROUTER_USERNAME` | `1234` | Usuario web del router. |
| `ROUTER_PASSWORD` | Vacío | Contraseña web. La recuperación se activa cuando esta variable tiene valor. |
| `DNS_PRIMARY` | `1.1.1.1` | DNS primario que se configurará en DHCP. |
| `DNS_SECONDARY` | `8.8.8.8` | DNS secundario que se configurará en DHCP. |
| `WIFI_OFF_SECONDS` | `10` | Tiempo durante el que ambas radios Wi-Fi permanecen apagadas. |
| `RECOVERY_COOLDOWN` | `900` | Tiempo mínimo entre intentos de recuperación, en segundos (15 minutos). |

## Cómo funciona el monitor DNS

El monitor envía a cada destino una consulta DNS UDP recursiva de tipo A. Considera operativo un servidor si recibe una respuesta DNS con el mismo identificador, no truncada, con código `NOERROR` y al menos una respuesta.

- **Fallo:** se notifica en la primera comprobación fallida.
- **Fallo persistente:** no se repite la notificación en cada ciclo mientras el estado no cambie.
- **Recuperación:** se notifica cuando una comprobación vuelve a ser correcta.

El estado de los destinos se mantiene en memoria. Al reiniciar el contenedor, los destinos se evalúan de nuevo desde cero.

## Recuperación automática del Askey RTF8316VW

La recuperación es opcional y se activa únicamente cuando `ROUTER_PASSWORD` tiene valor.

### Condición de activación

La recuperación solo se inicia cuando **todos los destinos de `TARGETS` fallan durante tres comprobaciones consecutivas**. La caída de un único DNS no debe provocar el reinicio del router si otro destino sigue respondiendo.

### Secuencia de recuperación

1. El monitor detecta la caída total y consecutiva de los DNS configurados.
2. Envía una notificación de inicio mediante el cliente Telegram existente.
3. Inicia sesión en la interfaz HTTP del Askey.
4. Lee la configuración LAN y DHCP actual.
5. Actualiza los DNS DHCP con `DNS_PRIMARY` y `DNS_SECONDARY`, preservando la IP LAN, máscara, estado DHCP y rango DHCP existentes.
6. Vuelve a leer la configuración del router y verifica los DNS guardados.
7. Lee el estado y la configuración actual de las radios Wi-Fi de 2,4 GHz y 5 GHz.
8. Desactiva ambas radios simultáneamente.
9. Espera el tiempo configurado en `WIFI_OFF_SECONDS` (10 segundos por defecto).
10. Restaura las radios que estaban habilitadas antes del reinicio.
11. Comprueba el estado administrativo y operativo de ambas radios.
12. Notifica el resultado de la recuperación por Telegram.

Se aplica un cooldown de 15 minutos por defecto para evitar repetir el procedimiento durante una caída prolongada. El valor puede cambiarse con `RECOVERY_COOLDOWN`.

Los errores o timeouts de una petición de escritura no se interpretan automáticamente como prueba de que el cambio no se aplicó: el servicio consulta el estado real del router antes de decidir cómo continuar. Los fallos de Telegram no deben bloquear la recuperación.

## Telegram

1. Crea un bot con `@BotFather` mediante `/newbot`.
2. Copia el token en `TELEGRAM_BOT_TOKEN` dentro de `docker-compose.yml`.
3. Inicia una conversación con el bot o añádelo al grupo destinatario.
4. Introduce el ID correspondiente en `TELEGRAM_CHAT_ID`.

El monitor reutiliza el mismo cliente Telegram para las notificaciones normales y los avisos de recuperación.

## Detalle de las propiedades del Docker Compose

| Propiedad | Función |
| --- | --- |
| `services.adguard-dns-monitor` | Define el servicio del monitor. |
| `build.context: .` | Usa el directorio actual como contexto de construcción. |
| `build.dockerfile` | Indica el Dockerfile que construye la imagen. |
| `container_name` | Fija el nombre del contenedor como `adguard-dns-monitor`. |
| `restart: unless-stopped` | Reinicia el servicio ante fallos, salvo que se haya detenido manualmente. |
| `environment` | Define directamente las variables de entorno que consume el proceso Go. |
| `dns` | Configura los resolvers propios del contenedor para sus conexiones salientes, por ejemplo Telegram. No modifica los DNS DHCP del router ni los destinos monitorizados. |
| `security_opt: no-new-privileges:true` | Impide que procesos del contenedor obtengan privilegios adicionales. |
| `cap_drop: ALL` | Elimina todas las Linux capabilities del contenedor. |
| `read_only: true` | Monta el filesystem raíz del contenedor en solo lectura. |
| `tmpfs` | Crea un directorio `/tmp` temporal de 16 MB en memoria, sin ejecución, setuid ni dispositivos. |
| `logging.driver: json-file` | Utiliza el driver JSON de Docker. |
| `logging.options.max-size: 10m` | Rota cada archivo de log al alcanzar 10 MB. |
| `logging.options.max-file: "3"` | Conserva hasta tres archivos de log. |

### Diferencia entre los DNS del contenedor y los DNS DHCP

La propiedad Compose `dns:` establece los servidores DNS que utiliza **el propio contenedor** para resolver nombres de sus conexiones salientes, como Telegram.

Las variables `DNS_PRIMARY` y `DNS_SECONDARY` son diferentes: son los servidores que el módulo de recuperación configura en el **servidor DHCP del router** para los dispositivos de la red.

Los destinos de `TARGETS` también son independientes: son las IP de AdGuard Home que el monitor comprueba directamente.

## Red y seguridad

- El host Docker debe poder alcanzar las IP de `TARGETS` por UDP en los puertos configurados.
- El contenedor necesita salida HTTPS (TCP 443) hacia Telegram.
- No se publican puertos.
- `cap_drop: ALL` elimina las capabilities de Linux.
- `no-new-privileges:true` impide elevar privilegios.
- `read_only: true` mantiene el filesystem raíz en solo lectura.
- `tmpfs` proporciona el espacio temporal necesario sin habilitar escritura general en el contenedor.

**Credenciales:** el token de Telegram y la contraseña del router se escriben directamente en `docker-compose.yml`, tal como requiere esta configuración. Mantén el repositorio privado y no publiques credenciales reales en un repositorio público. Ten en cuenta que las variables de entorno del contenedor pueden ser visibles a usuarios con permisos suficientes sobre el host Docker.

## Desarrollo y validación

La integración incluye pruebas automatizadas y un workflow de GitHub Actions para compilar y comprobar el código Go. Las pruebas de CI no sustituyen una prueba contra el router físico: los endpoints y formularios utilizados corresponden al firmware observado del Askey RTF8316VW y pueden variar con futuras versiones.

