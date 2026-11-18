# Prompt: investigar un router para Local_DNS-DRP

Actúa como un ingeniero de protocolos HTTP, automatización de navegadores y desarrollo Go. Tu tarea es investigar, documentar y preparar la implementación futura de un adaptador de recuperación para **Local_DNS-DRP**.

Este prompt es agnóstico al proveedor: puedes ejecutarlo con Claude, Codex, Antigravity u otro agente. Utiliza las herramientas que realmente tengas disponibles. Si dispones de automatización de navegador, puedes usar Chrome DevTools MCP u otra integración compatible; no asumas que una herramienta concreta está instalada.

## Objetivo

Investiga el router indicado por el usuario para averiguar, con evidencias verificables:

1. Cómo se autentica la interfaz y cómo gestiona sesiones/CSRF.
2. Cómo se lee la configuración LAN/DHCP, especialmente DNS.
3. Cómo se actualizan los DNS que distribuye DHCP.
4. Cómo se lee y, si se autoriza expresamente, controla el estado de las radios Wi-Fi.
5. Cómo confirmar cada cambio mediante lectura posterior.
6. Cómo responde ante credenciales incorrectas, sesión caducada, errores y timeouts.

El resultado principal debe ser un **Router Discovery Report** siguiendo la plantilla `docs/router-discovery-report-template.md` del repositorio. No escribas endpoints, parámetros ni comportamientos que no hayas observado o respaldado por una fuente identificable.

## Datos del router

Utiliza los datos que proporcione el usuario. No inventes los que falten.

- Fabricante:
- Modelo exacto:
- Revisión de hardware:
- Firmware/build:
- Operador/región:
- Dirección local de la interfaz:
- Objetivo específico o limitaciones:

Si un dato es desconocido, márcalo como pendiente.

## Restricciones de seguridad: obligatorias

**Empieza en modo solo lectura.** No ejecutes ninguna acción de escritura hasta que el usuario haya revisado un plan concreto y te haya autorizado expresamente esa operación.

Sin autorización expresa, NO debes:

- Cambiar DNS, DHCP, LAN, Wi-Fi ni ninguna otra configuración.
- Desactivar o reiniciar radios.
- Reiniciar el router, restaurar valores de fábrica, actualizar firmware o cambiar contraseñas.
- Desactivar firewall, controles de acceso o medidas de seguridad.
- Enviar formularios o peticiones POST/PUT/PATCH/DELETE que puedan modificar estado.
- Repetir una petición de escritura cuyo resultado sea ambiguo.

Antes de cualquier escritura autorizada, explica: endpoint, método, campos que se enviarán, efecto esperado, posible pérdida de conectividad y cómo se revierte. Cambia una sola cosa cada vez. Lee y registra el estado previo. Tras la operación, vuelve a leer el estado y comprueba el resultado. Si el resultado es ambiguo, no reintentes a ciegas: consulta primero el estado real.

No extraigas, reproduzcas, guardes ni incluyas en el informe credenciales, cookies, valores de `Set-Cookie`, cabeceras `Authorization`, tokens CSRF, IDs de sesión, API keys ni códigos de un solo uso. Puedes identificar el nombre de un campo o mecanismo, pero redacta su valor como `<REDACTED>`.

También sanea IP públicas, MAC, números de serie, SSID privados, identificadores de abonado, emails y datos personales. Revisa manualmente las capturas, HAR, logs y ficheros antes de guardarlos o compartirlos. Nunca guardes un HAR completo sin sanear.

Utiliza un perfil de navegador dedicado si automatizas Chrome. No inspecciones otras pestañas, cuentas o aplicaciones personales. No expongas el puerto de depuración remota a la red.

## Método de trabajo

### Fase 1 — Identificar y preparar

1. Confirma modelo exacto, hardware revision y firmware. Prioriza etiqueta del equipo e información visible en la interfaz.
2. Registra URL/base path de administración y si usa HTTP o HTTPS.
3. Comprueba el acceso a la interfaz, sin enviar cambios.
4. Enumera las herramientas disponibles. Si no tienes acceso al navegador, pide al usuario capturas o ejemplos saneados; no finjas haber inspeccionado tráfico.
5. Anota las limitaciones del entorno y qué pruebas no se podrán realizar.

### Fase 2 — Observar la autenticación

Con las herramientas disponibles, observa la carga inicial y el login sin revelar valores sensibles.

Documenta:
- Método y ruta.
- Tipo de contenido.
- Nombres de campos del formulario/payload, no sus valores secretos.
- Cabeceras funcionales relevantes, eliminando cookies, tokens y autorización.
- Respuesta HTTP y estructura del cuerpo saneado.
- Mecanismo de sesión, CSRF/nonce y cómo detecta la sesión caducada.
- Secuencia de llamadas necesaria para acceder a páginas protegidas.

No copies una cookie de la sesión de navegador para usarla en el código Go. El adaptador debe obtener su propia sesión autenticándose de forma normal.

### Fase 3 — Leer configuración (solo lectura)

Localiza y observa las páginas/peticiones utilizadas para leer:
- IP LAN y máscara.
- DHCP habilitado y rango de direcciones.
- DNS primario y secundario que anuncia DHCP.
- Estado de Wi-Fi por banda/radio y parámetros necesarios para conservar la configuración.

Registra método, ruta, parámetros, Content-Type, forma del payload y respuesta saneada. Identifica qué campos son necesarios y cuáles deben preservarse. No asumas que la misma API aplica a todas las revisiones/firmwares.

### Fase 4 — Analizar escrituras sin ejecutarlas

A partir del código HTML/JavaScript, documentación o peticiones observadas, identifica cómo podrían modificarse:
- DNS DHCP.
- Estado de cada radio Wi-Fi.

En esta fase solo describe la operación propuesta; no la envíes. Para cada endpoint indica qué evidencia respalda el análisis y qué parte es inferida.

Si el usuario autoriza una prueba controlada:
1. Presenta antes el plan exacto y espera autorización para esa operación.
2. Guarda el estado previo saneado.
3. Ejecuta una única modificación autorizada.
4. Lee nuevamente el estado desde el router.
5. Confirma qué cambió realmente y qué se mantuvo intacto.
6. Documenta cómo restaurar el estado original, si corresponde.
7. No continúes con otras operaciones sin autorización independiente cuando sean disruptivas.

### Fase 5 — Errores, recuperación y consistencia

Investiga mediante observación o fuentes disponibles, sin provocar cambios peligrosos:
- Credenciales incorrectas.
- Sesión expirada.
- CSRF/nonce inválido.
- Respuestas malformadas.
- Escritura rechazada.
- HTTP 200 sin cambio efectivo.
- Timeout durante escritura.
- Pérdida de conexión.

No marques como probado ningún error que no hayas observado. Recomienda qué debe hacer el adaptador cuando el resultado no sea concluyente. En particular, un timeout durante una escritura debe tratarse como resultado ambiguo: leer el estado antes de reintentar.

### Fase 6 — Generar el informe y evidencias

Rellena `docs/router-discovery-report-template.md` o produce el mismo formato si no tienes acceso al repositorio.

Clasifica cada hallazgo explícitamente como:
- **Observado:** existe evidencia directa.
- **Inferido:** explicación plausible no comprobada.
- **Pendiente de verificar:** falta evidencia.

Incluye ejemplos HTTP mínimos y saneados si aportan valor. No incluyas secretos ni volcados completos. Identifica las fuentes/evidencias con IDs (E-01, E-02...). Indica firmware exacto y limitaciones.

## Fase 7 — Preparar la implementación Go (solo después del informe)

No empieces a implementar el adaptador hasta que el usuario haya revisado el informe o lo haya solicitado explícitamente.

El diseño final debe:
- Integrarse mediante la interfaz `RouterRecovery` existente.
- Registrarse mediante `ROUTER_MODEL`, con identificador estable kebab-case.
- Utilizar HTTP desde Go, sin navegador, Chrome ni MCP en producción.
- Usar timeouts y respetar `context.Context`.
- Mantener la autenticación y parseo aislados en el adaptador.
- Validar respuestas y verificar cambios mediante lecturas posteriores.
- Preservar configuración ajena a la recuperación.
- Restaurar radios/servicios desactivados temporalmente ante errores/cancelación, con timeout de limpieza independiente cuando sea necesario.
- No registrar secretos.
- Fallar de forma segura si el firmware/API no coincide con lo probado.

Crea tests deterministas con `net/http/httptest` y fixtures saneados. Los tests no deben conectarse al router real ni a Internet. Ejecuta `gofmt`, `go test -race -count=1 ./...`, `go vet ./...` y `go build ./...`.

## Formato de salida requerido

Al finalizar la investigación, entrega:

1. Identificación exacta del equipo y firmware.
2. Resumen del flujo de autenticación.
3. Tabla de lecturas DNS/DHCP y Wi-Fi.
4. Tabla de escrituras potenciales, indicando cuáles se probaron con autorización.
5. Matriz de errores y verificaciones.
6. Evidencias saneadas y su referencia.
7. Riesgos, limitaciones y compatibilidad conocida.
8. Recomendación: listo para implementar / faltan evidencias / no viable con los datos actuales.
9. Lista de preguntas concretas para el usuario, solo cuando bloqueen una conclusión.

Sé preciso y conservador. La ausencia de evidencia no significa que un endpoint no exista. No inventes detalles ni presentes inferencias como hechos.
