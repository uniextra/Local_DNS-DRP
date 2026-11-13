# AdGuardHome-Restore — Monitor DNS

Servicio ligero, sin interfaz web, que comprueba periódicamente varias instancias de AdGuard Home mediante consultas DNS reales y envía avisos por Telegram cuando una instancia deja de resolver o se recupera.

## Funcionalidades

- Monitoriza varias instancias AdGuard Home por IP y puerto DNS.
- Envía una alerta cuando detecta un fallo y otra cuando la instancia se recupera.
- Envía al arrancar un aviso de Telegram indicando cuántos servidores DNS está monitorizando y sus direcciones; también avisa al detenerse.
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

### 2. Configurar `docker-compose.yml`

Todas las variables se definen directamente en la sección `environment:` de Compose. No hace falta crear ningún fichero `.env`. Edita los valores de ejemplo en `docker-compose.yml` antes de arrancar el servicio.

```yaml
services:
  adguard-dns-monitor:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: adguard-dns-monitor
    restart: unless-stopped
    environment:
      TARGETS: "adguard-home=192.168.1.2,adguard-backup=192.168.1.3"
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

Sustituye las IP, el token y el Chat ID por tus valores reales. No añadas `env_file:`; la configuración se inyecta directamente desde este YAML.

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

Cuando cambies el código o el Dockerfile, vuelve a construir con `docker compose up -d --build`. Si solo cambias valores de `environment:` en el YAML, recrea el contenedor con `docker compose up -d`.

## Variables de entorno

Todas las variables se configuran directamente en `environment:` dentro de `docker-compose.yml`. No es necesario crear un fichero `.env`.

| Variable | Obligatoria | Valor por defecto | Descripción |
| --- | --- | --- | --- |
| `TARGETS` | Sí | — | Servidores DNS a monitorizar. Lista separada por comas con formato `nombre=IP[:puerto]`. Puerto por defecto: `53`. Utiliza IP, no nombres DNS. |
| `TELEGRAM_BOT_TOKEN` | Sí | — | Token del bot obtenido de `@BotFather`. |
| `TELEGRAM_CHAT_ID` | Sí | — | ID del usuario, grupo o canal que recibirá los avisos. Los grupos suelen tener un ID negativo. |
| `CHECK_INTERVAL` | No | `60` | Segundos entre ciclos de monitorización. Entero mayor que cero. |
| `DNS_TIMEOUT` | No | `3` | Tiempo máximo de espera de cada consulta DNS, en segundos. Admite decimales. |
| `DNS_TEST_DOMAIN` | No | `google.com` | Dominio consultado mediante un registro A (IPv4). |
| `TELEGRAM_TIMEOUT` | No | `10` | Timeout HTTP para Telegram, en segundos. Entero mayor que cero. |
| `LOG_LEVEL` | No | `INFO` | Nivel de log: `DEBUG`, `INFO`, `WARNING`, `ERROR` o `CRITICAL`. |

### Ejemplos de TARGETS

```yaml
environment:
  TARGETS: "home=192.168.1.2,backup=192.168.1.3"
```

También puedes indicar puertos personalizados o IPv6:

```yaml
environment:
  TARGETS: "home=192.168.1.2:53,lab=192.168.1.10:5353"
  # Para IPv6 con puerto, utiliza corchetes:
  # TARGETS: "office=[2001:db8::53]:53"
```

Cada combinación IP/puerto debe ser única. El nombre es una etiqueta que aparecerá en los logs y avisos de Telegram.

### Configurar Telegram

1. Abre una conversación con `@BotFather`, envía `/newbot` y sigue sus instrucciones.
2. Copia el token generado en `TELEGRAM_BOT_TOKEN`.
3. Inicia una conversación con tu bot (pulsa **Iniciar**). Para un grupo, añade el bot y permite que envíe mensajes.
4. Obtén el ID del chat o grupo y configúralo en `TELEGRAM_CHAT_ID`.

**Seguridad:** el token queda guardado en texto plano dentro de `docker-compose.yml`. Mantén el fichero privado, limita sus permisos y no publiques credenciales reales en Git. Aunque el repositorio sea privado, cualquier persona con acceso al repositorio podrá leerlas.

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

- El token de Telegram está escrito directamente en `docker-compose.yml`; protege ese fichero y evita publicar credenciales reales.
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
