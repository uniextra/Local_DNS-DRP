# Router Discovery Report

> Informe de investigación del protocolo de administración de un router para preparar un adaptador de recuperación de Local_DNS-DRP.
>
> Completa lo que conozcas. Usa `No observado` o `Pendiente de verificar` en lugar de adivinar. Adjunta únicamente evidencias saneadas.

## 1. Identificación del equipo

| Campo | Valor |
|---|---|
| Fabricante | |
| Modelo comercial exacto | |
| Revisión de hardware | |
| Firmware / build | |
| Operador / región | |
| URL o tipo de interfaz (HTTP/HTTPS) | |
| Identificador propuesto `ROUTER_MODEL` | |
| Fecha de investigación | |
| Agente de IA utilizado (opcional) | |
| Herramienta de navegador/MCP utilizada (opcional) | |

## 2. Alcance y entorno

- Acceso utilizado (LAN, Wi-Fi, equipo conectado por cable):
- Navegador y perfil dedicado (si aplica):
- Versión de firmware y condiciones relevantes:
- Operaciones investigadas:
  - [ ] Lectura de configuración LAN/DHCP
  - [ ] Actualización de DNS DHCP
  - [ ] Lectura de estado Wi-Fi
  - [ ] Desactivación/reactivación de radios Wi-Fi
  - [ ] Autenticación y gestión de sesión
  - [ ] Tratamiento de errores y sesiones caducadas
- ¿Se realizaron escrituras en el router? Indica cuáles y confirma que fueron autorizadas:
- ¿Se mantuvo el acceso al router durante las pruebas?:

## 3. Flujo de autenticación

Describe el flujo sin incluir valores secretos.

| Paso | Método | Ruta (sin parámetros sensibles) | Estado/respuesta esperada | Observaciones |
|---|---|---|---|---|
| Carga/login inicial | | | | |
| Envío de credenciales | | | | |
| Obtención/renovación de sesión | | | | |
| Comprobación de sesión | | | | |
| Logout (si aplica) | | | | |

- Tipo de autenticación: formulario, Basic, Digest, token, otro.
- Mecanismo de sesión: cookie, bearer, token CSRF, otro.
- ¿Cómo se detecta una sesión caducada?:
- ¿Existe renovación automática?:
- ¿Hay dependencias de orden, nonce, challenge o campos dinámicos?:

**No incluyas** cookies, tokens CSRF, contraseñas, cabeceras Authorization, identificadores de sesión ni valores de login.

## 4. Lectura de configuración

### LAN / DHCP

| Dato | Método y ruta | Campo de respuesta | Ejemplo saneado | Clasificación |
|---|---|---|---|---|
| IP LAN / máscara | | | | |
| Rango DHCP | | | | |
| DNS primario | | | | |
| DNS secundario | | | | |
| DHCP habilitado | | | | |

### Wi-Fi

| Radio/banda | Método y ruta | Campo de respuesta | Estado/campo | Clasificación |
|---|---|---|---|---|
| 2,4 GHz | | | | |
| 5 GHz | | | | |
| Otras bandas (si aplica) | | | | |

## 5. Operaciones de escritura observadas

> Completa esta sección solo si las escrituras fueron autorizadas. No ejecutes cambios, reinicios ni acciones disruptivas para completar el informe.

| Operación | Método y ruta | Content-Type | Campos del payload (sin secretos) | Respuesta | Verificación posterior |
|---|---|---|---|---|---|
| Actualizar DNS DHCP | | | | | |
| Desactivar radio | | | | | |
| Reactivar radio | | | | | |

Para cada operación, documenta:

- Estado previo y parámetros que deben preservarse.
- Si el endpoint exige una secuencia concreta de peticiones.
- Si la respuesta confirma la aceptación o solo la recepción.
- Qué lectura posterior demuestra que el cambio se aplicó.
- Efectos secundarios o pérdida temporal de conectividad.
- Cómo se recupera el estado original si falla un paso.

## 6. Errores y comportamiento límite

| Caso | Código/respuesta observada | ¿Cómo detectarlo? | ¿Qué debería hacer el adaptador? |
|---|---|---|---|
| Credenciales incorrectas | | | |
| Sesión caducada | | | |
| CSRF/nonce inválido | | | |
| Petición malformada | | | |
| Escritura rechazada | | | |
| HTTP 200 sin aplicar el cambio | | | |
| Timeout durante una escritura | | | |
| Router inaccesible | | | |

Distingue las respuestas observadas de los comportamientos que todavía no se han probado.

## 7. Evidencias adjuntas

| ID | Archivo o referencia | Qué demuestra | Cómo se saneó |
|---|---|---|---|
| E-01 | | | |
| E-02 | | | |
| E-03 | | | |

Adjunta fixtures mínimos de request/response en `testdata/routers/<router-model>/` cuando sean útiles. No adjuntes HAR completos, backups ni exportaciones de configuración sin una revisión exhaustiva.

## 8. Clasificación de hallazgos

Marca cada conclusión:

- **Observado:** comprobado directamente mediante una petición/respuesta o cambio de estado.
- **Inferido:** explicación plausible, todavía no comprobada.
- **Pendiente de verificar:** no hay evidencia suficiente.

| Hallazgo | Estado | Evidencia | Riesgo o limitación |
|---|---|---|---|
| Login/sesión | | | |
| Lectura DNS DHCP | | | |
| Escritura DNS DHCP | | | |
| Lectura estado Wi-Fi | | | |
| Control Wi-Fi | | | |
| Compatibilidad entre firmware | | | |

## 9. Recomendación de implementación

- ¿Es viable implementar un adaptador Go sin navegador?:
- Endpoints mínimos necesarios:
- Datos que deben consultarse antes de escribir:
- Lecturas necesarias para verificar las escrituras:
- Restauración/limpieza necesaria ante errores:
- Riesgos conocidos:
- Versiones exactas que se consideran cubiertas:
- Casos que deberían cubrir los tests con `httptest`:

## 10. Revisión de seguridad

- [ ] No hay contraseñas ni nombres de usuario reales.
- [ ] No hay cookies, tokens CSRF, IDs de sesión ni cabeceras Authorization.
- [ ] No hay IP públicas, números de serie, direcciones MAC ni datos personales.
- [ ] No hay SSID privados ni datos de cuenta.
- [ ] Se han revisado manualmente las capturas, HAR, logs y fixtures adjuntos.
- [ ] No se han realizado cambios de configuración sin autorización explícita.
- [ ] Se ha verificado que las evidencias adjuntas no permiten reutilizar una sesión.

## 11. Resumen

Explica en unas líneas qué se ha descubierto, qué está probado, qué queda pendiente y si recomiendas iniciar el adaptador.
