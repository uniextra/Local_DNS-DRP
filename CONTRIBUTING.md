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

## 3. Descubrir la API del router con un agente de IA

No es necesario que todos los colaboradores sepan hacer ingeniería inversa del tráfico HTTP ni que utilicen una herramienta concreta. Puedes apoyarte en un agente de IA, por ejemplo **Claude, Codex, Antigravity u otro agente compatible con automatización del navegador**.

La herramienta de navegador es opcional. Cuando esté disponible, puedes conectar el agente a Chrome mediante **Chrome DevTools MCP** u otra integración de automatización/inspección del navegador. El agente también puede trabajar con documentación del fabricante, capturas y ejemplos HTTP saneados que le proporciones manualmente.

El objetivo de esta fase es obtener un **Router Discovery Report** reproducible. El agente ayuda a observar y documentar el protocolo; no sustituye las pruebas ni debe inventar endpoints.

### Preparar el entorno

1. Conecta el ordenador a la misma red que el router y comprueba que puedes abrir su interfaz de administración.
2. Utiliza, preferiblemente, un perfil de navegador dedicado a investigar routers, sin sesiones personales ni otras pestañas privadas.
3. Configura en tu agente la integración de navegador que tengas disponible. Si utilizas Chrome DevTools MCP, verifica que el agente puede inspeccionar la pestaña correcta y sus peticiones de red.
4. Ten a mano el modelo exacto, revisión de hardware, versión de firmware y operador/país si el firmware está personalizado.
5. No compartas con el agente ni con el repositorio contraseñas, tokens, cookies o identificadores de sesión como parte del informe.

La conexión MCP no es un requisito de ejecución de Local_DNS-DRP: se utiliza únicamente durante la fase de investigación y desarrollo. El adaptador final debe funcionar directamente desde Go, sin Chrome, DevTools ni un navegador en producción.

### Secuencia de descubrimiento

Pide al agente que trabaje por fases y que distinga siempre entre **observado**, **inferido** y **pendiente de verificar**:

1. **Inventario y autenticación (solo lectura):** identifica las páginas, peticiones de login, mecanismo de sesión, cookies, tokens CSRF y caducidad. Documenta el flujo sin guardar valores secretos.
2. **Lectura de configuración:** navega por las páginas LAN/DHCP y Wi-Fi y registra las peticiones que permiten consultar los DNS actuales, el rango DHCP y el estado de cada radio.
3. **Análisis de las operaciones:** describe qué endpoint, método, cabeceras y payload parecen modificar cada parámetro. Antes de realizar escrituras, el agente debe explicar qué cambiará y solicitar tu autorización.
4. **Prueba controlada:** solo con tu autorización, cambia una opción cada vez. Guarda el estado previo, ejecuta la operación, consulta de nuevo el estado y comprueba el resultado. Cambiar el Wi-Fi puede interrumpir la sesión del navegador; tenlo en cuenta antes de probarlo.
5. **Errores y sesiones:** documenta respuestas de credenciales incorrectas, sesión caducada, escritura rechazada, timeout y respuesta HTTP 200 sin cambio efectivo.
6. **Informe:** completa la plantilla [Router Discovery Report](docs/router-discovery-report-template.md). Incluye ejemplos saneados y las limitaciones de la revisión de hardware/firmware probada.

No permitas que el agente reinicie el router, restaure valores de fábrica, cambie la contraseña, desactive el firewall ni modifique otros parámetros ajenos a la recuperación. Cualquier operación que pueda interrumpir conectividad o cambiar configuración requiere tu autorización expresa.

### Prompt genérico para el agente

Puedes copiar el prompt de [Router Discovery Agent](docs/router-discovery-agent-prompt.md) en Claude, Codex, Antigravity u otro agente. Adapta el nombre del router y la información disponible. No depende de una marca, modelo de agente ni proveedor de MCP concretos.

### Sanitización obligatoria

Las herramientas de inspección del navegador pueden mostrar información sensible. Antes de guardar, compartir o adjuntar cualquier resultado, elimina:

- `Cookie`, `Set-Cookie`, `Authorization`, contraseñas y tokens CSRF.
- IDs de sesión, claves temporales, tokens de acceso y parámetros de login.
- IP públicas, números de serie, direcciones MAC, SSID y datos de cuenta.
- Cuerpos de petición/respuesta que incluyan información personal o secretos.

No adjuntes HAR, capturas de red o volcados sin revisar. Si necesitas conservar una petición, sustituye los valores sensibles por `<REDACTED>` y usa direcciones de documentación como `192.0.2.10`. No confíes únicamente en que el agente haya ocultado automáticamente los secretos: revisa tú mismo los ficheros generados.

### Del informe al adaptador

Cuando el informe esté completo:

- Implementa un adaptador Go independiente que reproduzca solo las peticiones necesarias.
- No copies cookies o tokens observados durante la sesión de descubrimiento; implementa el proceso de autenticación para obtener una sesión nueva en cada ejecución.
- Usa lectura posterior para comprobar cada escritura; HTTP 200 no es suficiente.
- Crea tests deterministas con `httptest` y fixtures saneados. Los tests no deben conectarse al router físico ni a Internet.
- Registra el identificador `ROUTER_MODEL`, actualiza la documentación y adjunta el informe saneado al PR.
- Indica el firmware exacto probado. No declares compatibles otras revisiones o versiones que no hayas verificado.

El agente puede ayudar a generar el adaptador y los tests, pero el colaborador sigue siendo responsable de revisar el código, los cambios que realiza sobre el router y los resultados de las pruebas.

## 4. Elegir el identificador ROUTER_MODEL

La variable de entorno `ROUTER_MODEL` selecciona el adaptador. Utiliza un identificador estable en minúsculas y formato kebab-case, basado en fabricante y modelo. Ejemplos:

- `askey-rtf8316vw`
- `tplink-archer-ax55`
- `fritzbox-7530-ax`
- `asus-generic` (adaptador experimental para ASUSWRT/Asuswrt-Merlin)

Usa exactamente el mismo identificador en el selector de adaptadores, los tests, la tabla de routers soportados del README y los ejemplos de Compose. No utilices como identificador un hostname, una IP, una versión de firmware ni un alias definido por cada usuario. `asus-generic` es la excepción intencionada al esquema por modelo: identifica una familia de firmware con compatibilidad experimental.

Por compatibilidad, si se omite `ROUTER_MODEL` se selecciona `askey-rtf8316vw`. Cuando `ROUTER_PASSWORD` habilita la recuperación, un identificador desconocido debe producir un error de configuración. Nunca debe caer por defecto en el adaptador Askey.

### Notas para el adaptador ASUS genérico

El identificador `asus-generic` es un adaptador de familia de firmware, no de un único modelo. Su ámbito previsto es ASUSWRT y Asuswrt-Merlin que expongan la API web local común.

La implementación de referencia en `router_asus_generic.go` utiliza:

- `/login.cgi` para autenticación y obtención de `asus_token`.
- `/appGet.cgi` con `nvram_get(...)` para lecturas.
- `/applyapp.cgi` con `action_mode=apply` para cambios y ejecución de servicios.
- `dhcp_dns1_x` y `dhcp_dns2_x` para los DNS anunciados por DHCP. No los confundas con `wan*_dns*`, que son DNS de WAN.
- `restart_dnsmasq` después de modificar DNS DHCP.
- `wlN_radio` y `restart_wireless` para controlar las radios disponibles.

Estas convenciones aparecen en implementaciones públicas de ASUSWRT, pero no constituyen un contrato universal de todos los firmwares ASUS. Antes de ampliar la compatibilidad, comprueba el modelo, la revisión de hardware y el build exactos. Las variantes con firmware de operador, versiones antiguas, SDN/Guest Network, AiMesh, modos AP/repetidor y modelos Wi-Fi 7 pueden requerir tratamiento específico.

El adaptador debe seguir fallando de forma segura si faltan variables, el estado no es `0/1`, la API no devuelve la estructura esperada o una operación no se puede verificar. No añadas un fallback silencioso al adaptador Askey ni supongas que existen exactamente dos radios. Mantén la detección dinámica y documenta cualquier límite de unidades.

No implementes la recuperación mediante SSH, ejecución de shell, `nvram` CLI ni scripts instalados en el router: el servicio se comunica con la API web desde Go. Tampoco conviertas la lectura de una variable en prueba suficiente de que los clientes DHCP reciben ese DNS; documenta las lecturas posteriores y la recarga de `dnsmasq`.

El estado de soporte de `asus-generic` es **experimental** hasta que se disponga de pruebas físicas reproducibles en modelos y builds concretos. Añade al README una matriz de compatibilidad con la evidencia real; no marques como probado todo el catálogo ASUS solo porque comparta ASUSWRT.

## 5. Implementar un adaptador independiente

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

## 6. Añadir tests automatizados

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

## 7. Actualizar la documentación y Compose

Un PR que añada un router debe actualizar:

1. La tabla de routers soportados del `README.md`, incluyendo las revisiones de hardware/firmware realmente probadas.
2. La descripción de `ROUTER_MODEL` si cambia la lista o el comportamiento de selección.
3. La secuencia de recuperación y las limitaciones específicas del router.
4. El ejemplo de Compose solo cuando sea necesaria una nueva variable de configuración común.

El Compose de ejemplo define las credenciales mediante `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` y `ROUTER_PASSWORD` en `environment`. No incluyas valores reales de credenciales en los PR, issues, fixtures ni capturas; utiliza siempre placeholders. Los usuarios que desplieguen el proyecto deben mantener su `docker-compose.yml` privado y restringir el acceso al host Docker.

## 8. Abrir el pull request

Crea una rama basada en el `main` más reciente, añade el adaptador y sus tests, y abre un PR usando la plantilla del repositorio. Incluye:

- Modelo exacto, revisión de hardware y firmware/build.
- Operador/región si el firmware está personalizado.
- Evidencias saneadas y una explicación breve del flujo de autenticación/API.
- Operaciones de recuperación implementadas y limitaciones conocidas.
- Cobertura de tests y resultados de los comandos de validación.
- Si se ha probado también contra el router físico.

Un router no se considera soportado solo porque aparezca en el título del PR. Debe tener un adaptador registrado, tests deterministas, documentación actualizada y CI satisfactoria. Los mantenedores pueden solicitar evidencias adicionales del firmware antes de incluirlo en la lista de modelos soportados.

Gracias por ayudar a ampliar la recuperación automática más allá del Askey original.
