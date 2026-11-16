# Local_DNS-DRP

Monitor DNS ligero escrito en Go para comprobar varias instancias de AdGuard Home y ejecutar una recuperación automática del router Askey RTF8316VW cuando todos los servidores DNS monitorizados dejan de responder.

El servicio no tiene interfaz web ni publica puertos. Se distribuye como imagen multi-arquitectura para Linux `amd64` y `arm64` (incluidas Raspberry Pi compatibles) y utiliza Telegram para notificar los cambios de estado y los resultados de recuperación.

## Funcionalidades

- Monitoriza varias instancias DNS mediante consultas UDP directas a las IP configuradas.
- Envía una notificación cuando un destino falla y otra cuando se recupera, sin repetir avisos mientras el estado no cambia.
- Envía notificaciones de inicio y parada.
- Activa opcionalmente la recuperación del Askey RTF8316VW cuando **todos** los destinos configurados están caídos durante tres comprobaciones consecutivas.
- Actualiza los DNS del servidor DHCP del router preservando la configuración LAN y el rango DHCP existente.
- Desactiva y verifica las radios Wi-Fi de 2,4 GHz y 5 GHz; tras el intervalo configurado restaura las que estaban habilitadas. Las órdenes se envían y verifican secuencialmente porque no se ha validado una operación conjunta o concurrente del firmware.
- Verifica la configuración DNS y el estado de las radios después del cambio.
- Aplica un cooldown para evitar recuperaciones repetidas durante una caída prolongada.
- Reutiliza el cliente Telegram existente para informar del inicio y resultado de la recuperación.
- Lee el token de Telegram, el chat ID y la contraseña del router desde Docker Compose secrets montados como archivos.
- Publica imágenes Docker multi-arquitectura `linux/amd64` y `linux/arm64`.

## Requisitos

- Docker Engine y Docker Compose.
- Conectividad desde el contenedor hacia los destinos de `TARGETS` mediante UDP en el puerto DNS configurado.
- Conectividad HTTPS saliente (TCP 443) hacia Telegram.
- Para utilizar la recuperación automática: un router Askey RTF8316VW accesible desde el contenedor y sus credenciales web.

## Despliegue

Clona el repositorio y entra en el directorio del servicio:

```bash
git clone https://github.com/uniextra/Local_DNS-DRP.git
cd Local_DNS-DRP
```

El Compose descarga la imagen multi-arquitectura de Docker Hub; Docker selecciona automáticamente `amd64` o `arm64` según el host. Antes del primer arranque, crea los archivos locales de secretos (no se versionan):

```yaml
services:
  adguard-dns-monitor:
    image: uniextra/local-dns-drp:latest
    container_name: adguard-dns-monitor
    restart: unless-stopped
    environment:
      TARGETS: "adguard-home=192.168.1.2,adguard-backup=192.168.1.3"

      # Read credentials from read-only files mounted under /run/secrets.
      TELEGRAM_BOT_TOKEN_FILE: /run/secrets/telegram_bot_token
      TELEGRAM_CHAT_ID_FILE: /run/secrets/telegram_chat_id

      CHECK_INTERVAL: "60"
      DNS_TIMEOUT: "3"
      DNS_TEST_DOMAIN: "google.com"
      TELEGRAM_TIMEOUT: "10"

      ROUTER_URL: "http://192.168.1.1"
      ROUTER_USERNAME: "1234"
      ROUTER_PASSWORD_FILE: /run/secrets/router_password
      DNS_PRIMARY: "1.1.1.1"
      DNS_SECONDARY: "8.8.8.8"
      WIFI_OFF_SECONDS: "10"
      RECOVERY_COOLDOWN: "900"
    secrets:
      - telegram_bot_token
      - telegram_chat_id
      - router_password
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

secrets:
  telegram_bot_token:
    file: ./secrets/telegram_bot_token.txt
  telegram_chat_id:
    file: ./secrets/telegram_chat_id.txt
  router_password:
    file: ./secrets/router_password.txt
```

Crea los archivos locales a partir de las plantillas incluidas:

```bash
cp secrets/telegram_bot_token.example secrets/telegram_bot_token.txt
cp secrets/telegram_chat_id.example secrets/telegram_chat_id.txt
cp secrets/router_password.example secrets/router_password.txt
```

Edita los dos primeros archivos con tus valores. Deja `router_password.txt` completamente vacío para desactivar la recuperación del Askey. En Linux, el contenedor se ejecuta como UID/GID `10001:10001`; asigna los archivos a ese usuario y limita su lectura:

```bash
sudo chown 10001:10001 secrets/*.txt
sudo chmod 0400 secrets/*.txt
```

Sustituye también `TARGETS` por las IP y puertos de tus instancias AdGuard Home y ajusta los valores no sensibles del router.

Valida la configuración y arranca el contenedor:

```bash
docker compose config
docker compose pull
docker compose up -d
docker compose logs -f adguard-dns-monitor
```

La imagen publicada contiene manifiestos para `linux/amd64` y `linux/arm64`. En Raspberry Pi con sistema operativo de 64 bits se utilizará la variante ARM64 automáticamente.

Para detenerlo:

```bash
docker compose down
```

## Variables de entorno

Las variables no sensibles se definen en `environment` en `docker-compose.yml`. Para credenciales, Compose monta los archivos declarados en `secrets:` bajo `/run/secrets/`; el programa los lee mediante variables `*_FILE`. Por compatibilidad también admite las variables antiguas (`TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` y `ROUTER_PASSWORD`), pero no se recomienda volver a introducir secretos directamente en Compose.

### Monitor DNS y Telegram

| Variable | Valor de ejemplo | Descripción |
| --- | --- | --- |
| `TARGETS` | `adguard-home=192.168.1.2,adguard-backup=192.168.1.3` | Lista de destinos separados por comas con formato `nombre=IP[:puerto]`. El puerto por defecto es 53. Deben utilizarse direcciones IP, no hostnames. |
| `TELEGRAM_BOT_TOKEN_FILE` | `/run/secrets/telegram_bot_token` | Ruta del archivo con el token del bot creado con `@BotFather`. |
| `TELEGRAM_CHAT_ID_FILE` | `/run/secrets/telegram_chat_id` | Ruta del archivo con el ID del usuario, grupo o canal que recibe las notificaciones. |
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
| `ROUTER_URL` | `http://192.168.1.1` | URL HTTP(S) de administración del router Askey. Se recomienda HTTPS con un certificado confiable cuando el firmware lo permita. |
| `ROUTER_USERNAME` | `1234` | Usuario web del router. |
| `ROUTER_PASSWORD_FILE` | `/run/secrets/router_password` | Ruta del archivo con la contraseña web. Un archivo vacío desactiva la recuperación. |
| `DNS_PRIMARY` | `1.1.1.1` | DNS primario que se configurará en DHCP. Debe ser una dirección IP. |
| `DNS_SECONDARY` | `8.8.8.8` | DNS secundario que se configurará en DHCP. Debe ser una dirección IP. |
| `WIFI_OFF_SECONDS` | `10` | Tiempo durante el que ambas radios Wi-Fi permanecen apagadas. |
| `RECOVERY_COOLDOWN` | `900` | Tiempo mínimo entre intentos de recuperación, en segundos (15 minutos). |

## Cómo funciona el monitor DNS

El monitor envía a cada destino configurado una consulta DNS UDP recursiva de tipo A. Considera operativo un servidor únicamente si la respuesta tiene el mismo identificador, corresponde a la pregunta enviada, no está truncada, devuelve `NOERROR` y contiene un registro A válido para el nombre consultado o su cadena CNAME. Un registro A ajeno a la consulta no basta para marcar el servidor como saludable.

- **Fallo:** se notifica en la primera comprobación fallida.
- **Fallo persistente:** no se repite la notificación en cada ciclo mientras el estado no cambie.
- **Recuperación:** se notifica cuando una comprobación vuelve a ser correcta.

El estado de los destinos se mantiene en memoria. Al reiniciar el contenedor, los destinos se evalúan de nuevo desde cero.

## Recuperación automática del Askey RTF8316VW

La recuperación es opcional y se activa únicamente cuando el archivo indicado por `ROUTER_PASSWORD_FILE` contiene una contraseña. Un archivo vacío mantiene la recuperación desactivada.

### Condición de activación

La recuperación solo se inicia cuando **todos los destinos de `TARGETS` fallan durante tres comprobaciones consecutivas** y, además, **al menos uno de `DNS_PRIMARY` / `DNS_SECONDARY` responde** a la misma consulta de prueba (UDP, puerto 53). La caída de un único DNS no debe provocar el reinicio del router si otro destino sigue respondiendo.

Si ninguno de los DNS de respaldo responde, la causa está fuera de la red local (por ejemplo, una caída del proveedor de internet): cambiar el DHCP y reiniciar el Wi-Fi no la arreglaría y solo desconectaría a los clientes. En ese caso el monitor no toca el router, avisa una sola vez por Telegram y vuelve a comprobarlo en cada ciclo. El cooldown solo se consume cuando una recuperación llega a iniciarse.

### Secuencia de recuperación

1. El monitor detecta la caída total y consecutiva de los DNS configurados.
2. Comprueba que al menos uno de `DNS_PRIMARY` / `DNS_SECONDARY` responde a la consulta de prueba. Si ninguno responde, omite la recuperación (ver arriba).
3. Envía una notificación de inicio mediante el cliente Telegram existente.
4. Inicia sesión en la interfaz HTTP del Askey.
5. Lee la configuración LAN y DHCP actual.
6. Actualiza los DNS DHCP con `DNS_PRIMARY` y `DNS_SECONDARY`, preservando la IP LAN, máscara, estado DHCP y rango DHCP existentes.
7. Vuelve a leer la configuración del router y verifica los DNS guardados.
8. Lee el estado y la configuración actual de las radios Wi-Fi de 2,4 GHz y 5 GHz.
9. Desactiva ambas radios simultáneamente.
10. Espera el tiempo configurado en `WIFI_OFF_SECONDS` (10 segundos por defecto).
11. Restaura las radios que estaban habilitadas antes del reinicio.
12. Comprueba el estado administrativo y operativo de ambas radios.
13. Notifica el resultado de la recuperación por Telegram.

Se aplica un cooldown de 15 minutos por defecto para evitar repetir el procedimiento durante una caída prolongada. El valor puede cambiarse con `RECOVERY_COOLDOWN`.

**Importante:** la recuperación no revierte los DNS del DHCP. Cuando AdGuard vuelve a responder, los dispositivos siguen recibiendo `DNS_PRIMARY` / `DNS_SECONDARY` (y, por tanto, sin el filtrado de AdGuard) hasta que restaures a mano los DNS originales en el router.

Los errores o timeouts de una petición de escritura DNS no se interpretan automáticamente como prueba de que el cambio no se aplicó: el servicio vuelve a leer la configuración DHCP y comprueba el valor real. Las órdenes Wi-Fi también se verifican mediante una lectura posterior, incluso si el POST devuelve HTTP 200. Si el firmware rechaza una sesión, el servicio intenta renovarla una sola vez. Los fallos de Telegram no deben bloquear la recuperación.

## Telegram

1. Crea un bot con `@BotFather` mediante `/newbot`.
2. Copia el token en `secrets/telegram_bot_token.txt`.
3. Inicia una conversación con el bot o añádelo al grupo destinatario.
4. Introduce el ID correspondiente en `secrets/telegram_chat_id.txt`.

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

**Credenciales:** el Compose no contiene valores de secretos. El token de Telegram, el chat ID y la contraseña del router se almacenan en archivos locales ignorados por Git y se montan en modo solo lectura bajo `/run/secrets/`. Docker Compose file-backed secrets no cifra esos archivos en reposo: protege los permisos del directorio `secrets/` en el host y no compartas ni publiques los archivos `.txt`. En Docker Swarm, utiliza secretos gestionados por Swarm.

**Transporte hacia el router:** el Askey puede exponer su interfaz de administración únicamente por HTTP. En ese caso, la contraseña y los datos de sesión viajan sin cifrar y el programa registra un aviso al iniciar la recuperación. Utiliza `ROUTER_URL: https://...` solo si el router ofrece HTTPS con un certificado válido; no desactives la validación TLS para ocultar errores de certificado. Si el firmware solo soporta HTTP, limita el acceso de administración a una LAN de confianza y nunca expongas esa interfaz a Internet.

## Codex co-author attribution

This repository includes a Git `prepare-commit-msg` hook based on
[ sample-codex-coauthor ](https://github.com/realsigridjin/sample-codex-coauthor).
The hook appends the official Codex attribution trailer to future local commits:

```text
Co-authored-by: Codex <noreply@openai.com>
```

The address above follows the current attribution string used by the
[official OpenAI Codex repository](https://github.com/openai/codex). It is
different from older community examples using `codex@openai.com`.

### Enable it in a local clone

Run this once from the repository root:

```bash
bash scripts/setup-codex-attribution.sh
```

This configures the clone's Git hook path and co-author entry. After setup,
normal local commits automatically receive the trailer. The setup is local
to each clone; cloning the repository does not activate Git hooks by itself.

The hook skips merge commits, avoids duplicate trailers, and supports a
one-commit opt-out with `SKIP_AI_COAUTHORS=1`.

Commits created through GitHub's web interface or an API must include the
same trailer in the commit message; local Git hooks do not run for those
operations.

## Imagen Docker y arquitecturas

El workflow de publicación construye y publica un manifiesto multi-arquitectura para `linux/amd64` y `linux/arm64`. En Raspberry Pi, utiliza un sistema operativo de 64 bits para ejecutar la variante ARM64. Docker selecciona la arquitectura correcta al hacer `docker pull uniextra/local-dns-drp:latest`.

Todas las GitHub Actions están fijadas a un SHA completo de commit, con el tag de versión conservado como comentario para facilitar su auditoría.

## Licencia

Este proyecto se distribuye bajo la licencia [MIT](LICENSE).

## Desarrollo y validación

La integración incluye un único workflow de GitHub Actions para comprobar `gofmt`, ejecutar `go test -race`, `go vet` y compilar el binario. Las pruebas simuladas no sustituyen una prueba contra el router físico: los endpoints y formularios utilizados corresponden al firmware observado del Askey RTF8316VW y pueden variar con futuras versiones. La activación simultánea exacta de ambas radios no se da por garantizada; las órdenes se envían secuencialmente y el intervalo de espera empieza cuando las dos están confirmadas como apagadas.

