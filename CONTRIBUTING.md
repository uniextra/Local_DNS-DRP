# Contribuir con adaptadores de recuperación para routers

Este proyecto acepta contribuciones para añadir recuperación automática en otros fabricantes, modelos y versiones de firmware. Cada router debe implementarse mediante un adaptador independiente, de forma que sus endpoints HTTP, autenticación, payloads y particularidades no se mezclen con el monitor DNS.

Antes de empezar, consulta la [lista de routers soportados](README.md#añadir-soporte-para-otro-router). Si no tienes claro si tu revisión de hardware o firmware ya está cubierta, abre primero una incidencia o discusión. Cuando tengas información suficiente para implementar y probar el adaptador, puedes abrir directamente un pull request.

## 1. Identificar el router exacto

Incluye en la descripción del PR:

- Fabricante y modelo comercial exacto (por ejemplo, `Askey RTF8316VW`).
- Revisión/versión de hardware, indicada en la etiqueta o en la interfaz de administración.
- Versión y número de build del firmware.
- País/región y operador, cuando el firmware esté personalizado por el ISP.
- Acciones de recuperación que soporta el router y cuáles implementa el adaptador:
  - Actualizar los DNS que distribuye el servidor DHCP.
  - Reiniciar o reactivar las radios Wi-Fi, si aplica.
  - Leer el estado posterior y verificar cada cambio.

No des por hecho que dos unidades con el mismo nombre comercial utilizan la misma API. Las revisiones de hardware, los firmwares de operador y las versiones de firmware pueden cambiar endpoints y campos. Si el soporte depende de una versión concreta, indícalo expresamente y no lo presentes como compatible con todas las variantes.

## 2. Recopilar evidencias sin exponer datos sensibles

Son útiles los ejemplos HTTP saneados de petición/respuesta, rutas de endpoints, campos relevantes de HTML/JavaScript, payloads XML/JSON y el estado del router antes y después del cambio.

Antes de adjuntar evidencias, elimina o sustituye:

- Contraseñas, nombres de usuario que identifiquen a una persona, claves API y tokens.
- Cookies, identificadores de sesión, tokens CSRF y cabeceras de autorización.
- IP públicas, números de serie, direcciones MAC y datos personales o de cuenta.
- SSID y nombres privados que no quieras publicar.

Utiliza marcadores como `<REDACTED>`, `192.0.2.10` y `example.invalid`. Nunca subas un HAR, captura de navegador, captura de paquetes, backup o exportación de configuración sin sanear. No incluyas credenciales del router en el PR, incidencias, fixtures de tests, capturas ni logs.

Documenta si la interfaz de administración usa HTTP o HTTPS, cómo funciona la autenticación, cómo caducan las sesiones y cómo informa el firmware de las escrituras rechazadas o aplicadas parcialmente. Una respuesta HTTP 200 no demuestra por sí sola que el cambio se haya aplicado.

## 3. Elegir el identificador ROUTER_MODEL

La variable de entorno `ROUTER_MODEL` selecciona el adaptador. Utiliza un identificador estable en minúsculas y formato kebab-case, basado en fabricante y modelo. Ejemplos:

- `askey-rtf8316vw`
- `tplink-archer-ax55`
- `fritzbox-7530-ax`

Usa exactamente el mismo identificador en el selector de adaptadores, los tests, la tabla de routers soportados del README y los ejemplos de Compose. No utilices como identificador un hostname, una IP, una versión de firmware ni un alias definido por cada usuario.

Por compatibilidad, si se omite `ROUTER_MODEL` se selecciona `askey-rtf8316vw`. Cuando `ROUTER_PASSWORD` habilita la recuperación, un identificador desconocido debe producir un error de configuración. Nunca debe caer por defecto en el adaptador Askey.

## 4. Implementar un adaptador independiente

El contrato común es:

```go
type RouterRecovery interface {
    Recover(context.Context) error
}
```

Añade la implementación del fabricante/modelo en un fichero propio, preferiblemente con el nombre `router_<fabricante>_<modelo>.go`. Mantén dentro de ese adaptador el login, las sesiones, las peticiones HTTP, el parseo de respuestas y la lógica de lectura/verificación de escrituras.

Esquema simplificado:

```go
type TPLinkArcherAX55Router struct {
    cfg    RouterConfig
    client *http.Client
}

func newTPLinkArcherAX55Router(cfg RouterConfig) RouterRecovery {
    return &TPLinkArcherAX55Router{
        cfg:    cfg,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (r *TPLinkArcherAX55Router) Recover(ctx context.Context) error {
    // 1. Autenticarse y crear una sesión nueva.
    // 2. Leer la configuración actual de DHCP/Wi-Fi.
    // 3. Aplicar únicamente los cambios de recuperación necesarios.
    // 4. Volver a leer la configuración y verificar los cambios.
    // 5. Restaurar las radios/servicios desactivados temporalmente,
    //    incluso cuando se cancele el contexto.
    return nil
}
```

Es solo un ejemplo de estructura, no una implementación funcional. Utiliza la API documentada u observada del router; no copies los endpoints ni payloads del Askey salvo que hayas comprobado que realmente son el mismo protocolo.

Registra el adaptador en `newRouterRecovery`, dentro de `router_recovery.go`:

```go
switch model {
case RouterModelAskeyRTF8316VW:
    return newAskeyRouter(cfg), nil
case "tplink-archer-ax55":
    return newTPLinkArcherAX55Router(cfg), nil
default:
    return nil, fmt.Errorf("unsupported ROUTER_MODEL %q", model)
}
```

Añade también una constante con nombre para el nuevo identificador. No cambies el modelo por defecto salvo que exista una decisión explícita del proyecto para hacerlo.

### Requisitos del adaptador

Un adaptador listo para producción debe:

- Respetar el contexto recibido y utilizar timeouts HTTP limitados.
- No registrar contraseñas, cookies, IDs de sesión ni datos de autorización.
- Validar las respuestas del router y rechazar estados inesperados o malformados.
- Leer la configuración existente antes de cambiarla y preservar los parámetros LAN, DHCP y Wi-Fi que no formen parte de la recuperación.
- Verificar cada cambio mediante una lectura posterior. No confiar solo en HTTP 200 o en una respuesta optimista de la API.
- Tratar los timeouts de escritura como resultados ambiguos y comprobar el estado antes de reintentar.
- Gestionar de forma explícita las sesiones caducadas, sin bucles infinitos de autenticación.
- Restaurar las radios o servicios desactivados temporalmente, también si hay errores o se cancela el contexto. Cuando sea necesario, utilizar un contexto de limpieza independiente y con timeout.
- Evitar reiniciar el router o cambiar parámetros ajenos al objetivo de recuperación.
- Devolver errores útiles sin revelar secretos.
- Fallar de forma segura cuando el firmware/API no corresponda con el que se ha probado.

Si el modelo no tiene radios Wi-Fi o alguna operación no aplica, documenta esa limitación. No informes de una operación como completada si el hardware no la soporta.

## 5. Añadir tests automatizados

Los tests deben ejecutarse sin router físico y sin Internet. Utiliza `net/http/httptest` u otro servidor simulado determinista y, cuando corresponda, fixtures saneados bajo `testdata/routers/<router-model>/`.

Como mínimo, cubre:

- Autenticación y recuperación correctas.
- Credenciales rechazadas y sesiones caducadas o inválidas.
- Campos ausentes, respuestas malformadas o inesperadas.
- Escrituras correctas seguidas de lectura y verificación.
- Respuestas HTTP 200 en las que el estado solicitado no se ha aplicado.
- Errores/timeouts ambiguos de escritura, verificando el estado antes de reintentar.
- Conservación de parámetros del router que no deban cambiar.
- Cancelaciones/errores y restauración de radios o servicios desactivados temporalmente.
- Rechazo de identificadores no soportados por el selector de adaptadores.

No hagas que los tests contacten con un router real, utilicen credenciales reales o dependan de la red doméstica de un usuario. Una prueba adicional contra el router físico es útil, pero no sustituye los tests deterministas de CI.

Ejecuta antes de enviar el PR:

```bash
gofmt -w *.go
go test -race -count=1 ./...
go vet ./...
go build ./...
```

La CI del repositorio también construye la imagen Docker para `linux/amd64` y `linux/arm64`.

## 6. Actualizar la documentación y Compose

Un PR que añada un router debe actualizar:

1. La tabla de routers soportados del `README.md`, incluyendo las revisiones de hardware/firmware realmente probadas.
2. La descripción de `ROUTER_MODEL` si cambia la lista o el comportamiento de selección.
3. La secuencia de recuperación y las limitaciones específicas del router.
4. El ejemplo de Compose solo cuando sea necesaria una nueva variable de configuración común.

El Compose de ejemplo define las credenciales mediante `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` y `ROUTER_PASSWORD` en `environment`. No incluyas valores reales de credenciales en los PR, issues, fixtures ni capturas; utiliza siempre placeholders. Los usuarios que desplieguen el proyecto deben mantener su `docker-compose.yml` privado y restringir el acceso al host Docker.

## 7. Abrir el pull request

Crea una rama basada en el `main` más reciente, añade el adaptador y sus tests, y abre un PR usando la plantilla del repositorio. Incluye:

- Modelo exacto, revisión de hardware y firmware/build.
- Operador/región si el firmware está personalizado.
- Evidencias saneadas y una explicación breve del flujo de autenticación/API.
- Operaciones de recuperación implementadas y limitaciones conocidas.
- Cobertura de tests y resultados de los comandos de validación.
- Si se ha probado también contra el router físico.

Un router no se considera soportado solo porque aparezca en el título del PR. Debe tener un adaptador registrado, tests deterministas, documentación actualizada y CI satisfactoria. Los mantenedores pueden solicitar evidencias adicionales del firmware antes de incluirlo en la lista de modelos soportados.

Gracias por ayudar a ampliar la recuperación automática más allá del Askey original.
