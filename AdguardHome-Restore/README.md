# AdGuardHome-Restore — Monitor DNS

Servicio ligero, sin interfaz web, que comprueba periódicamente varias instancias de AdGuard Home mediante consultas DNS reales y envía avisos por Telegram cuando una instancia deja de resolver o se recupera.

## Funcionalidades

- Monitoriza varias instancias AdGuard Home por IP y puerto DNS.
- Envía una alerta cuando detecta un fallo y otra cuando la instancia se recupera.
- Envía un aviso al arrancar y al detener el contenedor.
- No publica puertos ni necesita interfaz web.
- Las consultas de monitorización se envían directamente a cada IP configurada.
- El contenedor utiliza `1.1.1.1` (Cloudflare) y `8.8.8.8` (Google) como DNS propios, independientemente de los DNS configurados en el servidor Docker.

## Despliegue con Docker Compose

### 1. Descargar el proyecto

Clona el repositorio y entra en el directorio de la aplicación:

```bash
git clone https://github.com/uniextra/Adguardhome-restore.git
cd Adguardhome-restore/AdguardHome-Restore
```

Si el repositorio ya está clonado en tu servidor, simplemente entra en `AdguardHome-Restore`.

### 2. Crear `.env`

Copia el fichero de ejemplo:

```bash
cp .env.example .env
```

Edita `.env` y configura como mínimo:
- Las IP de las instancias DNS en `TARGETS`.
- El token del bot en `TELEGRAM_BOT_TOKEN`.
- El chat o grupo receptor en `TELEGRAM_CHAT_ID`.

Consulta la sección [Variables de entorno](#variables-de-entorno) para conocer todos los parámetros.

**Importante:** no subas ni compartas el fichero `.env`; contiene las credenciales del bot de Telegram.

### 3. Revisar o crear `docker-compose.yml`

El repositorio ya incluye `docker-compose.yml`. Si necesitas crearlo manualmente, utiliza este contenido:

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
    # No se publican puertos: el servicio no tiene interfaz web.
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

Mantén `docker-compose.yml`, `Dockerfile`, `monitor.py`, `requirements.txt` y `.env` en el mismo directorio. La configuración `dns:` fija los resolvers del contenedor; no modifica la configuración DNS del host.

### 4. Construir y arrancar

Desde el directorio `AdguardHome-Restore`:

```bash
docker compose up -d --build
```

Para revisar el arranque y las comprobaciones:

```bash
docker compose logs -f
```

Para detener el servicio:

```bash
docker compose down
```

Cuando cambies el código o el Dockerfile, vuelve a construir con `docker compose up -d --build`. Si solo cambias valores de `.env`, recrea el contenedor con `docker compose up -d`.

## Variables de entorno

Todas las opciones se configuran en el fichero `.env`. No es necesario modificar el código.

| Variable | Obligatoria | Valor por defecto | Descripción |
| --- | --- | --- | --- |
| `TARGETS` | Sí | — | Lista de servidores DNS a monitorizar. Formato: `nombre=IP[:puerto]`, separados por comas. El puerto por defecto es `53`. |
| `TELEGRAM_BOT_TOKEN` | Sí | — | Token del bot de Telegram. Se obtiene hablando con `@BotFather`. |
| `TELEGRAM_CHAT_ID` | Sí | — | ID del chat, grupo o canal al que se enviarán las notificaciones. |
| `CHECK_INTERVAL` | No | `60` | Intervalo entre ciclos de comprobación, en segundos. Debe ser un entero mayor que cero. |
| `DNS_TIMEOUT` | No | `3` | Tiempo máximo de espera de cada consulta DNS, en segundos. Admite decimales. |
| `DNS_TEST_DOMAIN` | No | `example.com` | Dominio consultado en cada prueba. Se solicita un registro DNS de tipo A (IPv4). |
| `TELEGRAM_TIMEOUT` | No | `10` | Tiempo máximo de espera de la petición HTTP a Telegram, en segundos. Debe ser un entero mayor que cero. |
| `LOG_LEVEL` | No | `INFO` | Nivel de detalle del log de Python: `DEBUG`, `INFO`, `WARNING`, `ERROR` o `CRITICAL`. |

### TARGETS: configurar varias instancias

Cada destino se define como `nombre=IP` o `nombre=IP:puerto`. Separa los destinos con comas. El nombre es una etiqueta descriptiva que aparecerá en los logs y en los avisos de Telegram.

Ejemplo con dos instancias usando el puerto DNS estándar:

```dotenv
TARGETS=adguard-home=192.168.1.2,adguard-backup=192.168.1.3
```

Ejemplo con un puerto DNS personalizado:

```dotenv
TARGETS=adguard-home=192.168.1.2:53,adguard-test=192.168.1.10:5353
```

También se admiten direcciones IPv6. Si se indica un puerto, encierra la dirección entre corchetes:

```dotenv
TARGETS=adguard-v6=[2001:db8::53]:53
```

Se deben indicar direcciones IP, no nombres DNS. El monitor contacta directamente con cada IP configurada. Cada combinación IP/puerto debe ser única.

### Telegram: crear el bot y obtener el chat ID

1. Abre Telegram y busca `@BotFather`.
2. Ejecuta `/newbot` y sigue los pasos para crear el bot.
3. Copia el token que te proporciona BotFather y ponlo en `TELEGRAM_BOT_TOKEN`.
4. Abre el chat con tu nuevo bot y pulsa **Iniciar** (o envíale un mensaje). Si vas a recibir avisos en un grupo, añade el bot al grupo y asegúrate de que puede escribir.
5. Obtén el ID del chat o grupo con un método fiable de consulta de actualizaciones de Telegram y configúralo en `TELEGRAM_CHAT_ID`. Los IDs de grupos suelen ser negativos.

Ejemplo:

```dotenv
TELEGRAM_BOT_TOKEN=123456789:AA_example_token
TELEGRAM_CHAT_ID=123456789
```

No utilices esos valores de ejemplo: sustitúyelos por las credenciales reales.

### Ejemplo completo de `.env`

```dotenv
TARGETS=adguard-home=192.168.1.2,adguard-backup=192.168.1.3:53

TELEGRAM_BOT_TOKEN=123456789:replace_with_real_token
TELEGRAM_CHAT_ID=123456789

CHECK_INTERVAL=60
DNS_TIMEOUT=3
DNS_TEST_DOMAIN=example.com
TELEGRAM_TIMEOUT=10
LOG_LEVEL=INFO
```

## Cómo funciona la comprobación DNS

Para cada destino, el monitor envía directamente una consulta DNS UDP de tipo A al IP y puerto configurados. La consulta solicita recursión, como lo haría un cliente DNS normal.

Una instancia se considera operativa si responde con el mismo identificador de transacción, la respuesta no está truncada, el código DNS es `NOERROR` y contiene al menos una respuesta.

- **Fallo:** se envía una alerta en la primera comprobación fallida.
- **Continúa el fallo:** no se repite el aviso en cada ciclo; se mantiene el estado en los logs.
- **Recuperación:** se envía una notificación cuando una comprobación vuelve a ser correcta.

El estado se mantiene en memoria. Si el contenedor se reinicia, las instancias vuelven a evaluarse desde el inicio y un nuevo fallo puede generar otra alerta.

## DNS del contenedor y DNS monitorizado

Son dos funciones distintas:

- **DNS del contenedor:** Docker configura `1.1.1.1` y `8.8.8.8` para resolver nombres que necesita la propia aplicación, por ejemplo `api.telegram.org`. Esto es independiente de los DNS del servidor host.
- **Prueba de AdGuard Home:** el monitor envía la consulta directamente a la IP indicada en `TARGETS`. No utiliza los resolvers públicos del contenedor para comprobar esa instancia.

Por tanto, aunque el servidor Docker tenga configurado AdGuard Home como DNS, el contenedor seguirá usando los resolvers públicos indicados en Compose para sus propias resoluciones.

## Seguridad y requisitos de red

- Mantén `.env` fuera del control de versiones; contiene el token de Telegram.
- El contenedor se ejecuta como usuario sin privilegios, elimina capabilities, utiliza un sistema de archivos raíz de solo lectura y no publica puertos.
- No necesita acceso al socket de Docker.
- El host Docker debe poder alcanzar por UDP las IP y puertos DNS definidos en `TARGETS`.
- El contenedor necesita salida a Internet para enviar mensajes a la API de Telegram por HTTPS (TCP 443) y para sus propias resoluciones DNS mediante los resolvers configurados.

## Desarrollo y tests

Instala las dependencias:

```bash
python -m pip install -r requirements.txt
```

Ejecuta los tests unitarios:

```bash
python -m unittest discover -s tests -v
```

Para ejecutar el monitor directamente fuera de Docker, define las variables obligatorias:

```bash
TARGETS=local=127.0.0.1 TELEGRAM_BOT_TOKEN=test TELEGRAM_CHAT_ID=test python monitor.py
```

La ejecución anterior es solo un ejemplo de desarrollo: el envío de notificaciones requiere credenciales reales de Telegram.
