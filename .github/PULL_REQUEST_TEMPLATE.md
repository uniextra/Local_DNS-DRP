## Soporte de router

- [ ] Este PR añade o modifica un adaptador de recuperación.
- [ ] El adaptador está registrado en `newRouterRecovery` y tiene un identificador `ROUTER_MODEL` estable.
- [ ] Se ha actualizado la tabla de routers soportados y la documentación del README.
- [ ] Los tests utilizan un router/servidor simulado y fixtures saneados; no contactan con un router físico.
- [ ] Se adjunta el [Router Discovery Report](../docs/router-discovery-report-template.md) cumplimentado o se explica por qué no aplica.

## Dispositivo y firmware

- Fabricante y modelo exacto:
- Revisión de hardware:
- Firmware/build:
- Operador y región (si el firmware está personalizado):
- Identificador `ROUTER_MODEL`:
- Agente de IA utilizado (opcional: Claude, Codex, Antigravity u otro):
- Herramienta de navegador/MCP utilizada (opcional):

## Comportamiento de recuperación

- [ ] Actualización de DNS DHCP
- [ ] Reinicio/reactivación de Wi-Fi (si aplica)
- [ ] Verificación mediante lectura posterior
- [ ] Restauración segura ante errores/cancelación

Describe las operaciones implementadas y las limitaciones conocidas:

## Evidencias

Describe los endpoints y ejemplos de petición/respuesta saneados o las evidencias del firmware incluidas. Adjunta el Router Discovery Report cuando se haya realizado esa fase. Distingue los hallazgos observados, inferidos y pendientes de verificar. Confirma que has eliminado contraseñas, cookies, IDs de sesión, tokens, números de serie, direcciones MAC y datos personales.

## Validación

- [ ] `gofmt -w *.go`
- [ ] `go test -race -count=1 ./...`
- [ ] `go vet ./...`
- [ ] `go build ./...`
- [ ] Prueba contra router físico realizada (opcional; indica firmware y resultado):

## Seguridad

- [ ] No se han incluido credenciales, tokens, cookies, IDs de sesión ni capturas sin sanear.
- [ ] El adaptador no registra secretos ni cambia parámetros ajenos a la recuperación.
- [ ] Las escrituras se verifican leyendo el estado resultante del router.
