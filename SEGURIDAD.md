# Medidas de seguridad implementadas

Resumen de los controles de seguridad de la aplicación, agrupados por riesgo. Sirve como insumo para las secciones *Restricciones de seguridad e integridad* y *Aspectos éticos* del informe. Entre corchetes se indica el riesgo del análisis de riesgos que mitiga (R6, R10).

## 1. Credenciales de GitHub [R6]

| Control | Dónde |
|---|---|
| El token se lee de variables de entorno; nunca está en el código ni en Git (`.env` está en `.gitignore` y `.dockerignore`). | `internal/config` |
| El token **nunca se envía al navegador**: el frontend solo habla con el backend propio. | Arquitectura |
| Integración **solo lectura**: el cliente solo contiene consultas GraphQL (no hay mutaciones; una prueba lo verifica). Token clásico con permisos de lectura (`read:project`, `read:org`, `repo`), idealmente de una cuenta de servicio. | `internal/github/client.go`, README |
| Solo se muestran proyectos de los dueños permitidos (las organizaciones de la cuenta del token en modo `auto`, o la lista de `GITHUB_OWNER`): se valida el `owner` de cada proyecto y que el ID esté en la lista permitida. Si una organización falla, se omite sin exponer las demás. | `client.go`, `handlers_projects.go` |
| Los errores de GitHub se registran sin cuerpo de respuesta ni cabeceras (no se filtra el token a los logs). | `client.go` |
| Límite de tamaño de respuesta (20 MB) y de elementos (2000) para evitar abuso de memoria. | `client.go` |

## 2. Autenticación y sesiones [R10]

| Control | Detalle |
|---|---|
| Contraseñas con **PBKDF2-HMAC-SHA256, 600.000 iteraciones** y sal aleatoria de 16 bytes (recomendación OWASP). Biblioteca estándar de Go. | `internal/auth/password.go` |
| Política: 12–128 caracteres, letras y números. | `ValidatePassword` |
| Contraseña temporal obligatoria de cambiar en el primer ingreso (bootstrap y usuarios creados por admin). Sin cambiarla no se accede a datos. | `pwChanged` |
| **Protección contra fuerza bruta**: 5 fallos por cuenta o 10 por IP en 15 min → bloqueo temporal. | `ratelimit.go`, `handleLogin` |
| Mensaje de error genérico y tiempo de respuesta igual exista o no el usuario (evita enumerar cuentas). | `DummyCheck` |
| Token de sesión aleatorio de 256 bits; en la base de datos solo se guarda su **SHA-256** (un volcado de la BD no permite secuestrar sesiones). | `HashToken` |
| Cookie `HttpOnly` (inaccesible a JS), `SameSite=Strict`, `Secure` y prefijo `__Host-` en producción. | `setSessionCookie` |
| Expiración por **inactividad (30 min)** y **absoluta (8 h)**; índice TTL en Mongo elimina sesiones vencidas. | `authed`, `mongo.go` |
| Logout invalida la sesión en el servidor (no solo borra la cookie). Cambiar la contraseña cierra las demás sesiones. Desactivar un usuario o cambiar su rol revoca sus sesiones al instante. | handlers |

## 3. Autorización

- Roles **admin** y **viewer**; las rutas de administración verifican el rol en el servidor (ocultar el menú no es el control).
- Un administrador no puede quitarse su propio rol ni desactivarse (evita quedar sin administradores).
- Usuarios inactivos no pueden ingresar.

## 4. Ataques web

| Ataque | Mitigación |
|---|---|
| **XSS** | El frontend inserta todo dato con `textContent` (nunca `innerHTML`). Enlaces filtrados para permitir solo `http(s)` (bloquea `javascript:`). **Content-Security-Policy** estricta: sin scripts ni estilos en línea, sin recursos externos salvo avatares de GitHub. |
| **CSRF** | Cookie `SameSite=Strict` + token CSRF por sesión en cabecera `X-CSRF-Token` (comparación en tiempo constante) + verificación de `Origin` y `Sec-Fetch-Site`. |
| **Clickjacking** | `X-Frame-Options: DENY` y `frame-ancestors 'none'`. |
| **Inyección NoSQL** | Las consultas a Mongo usan filtros tipados construidos en el servidor; el JSON del cliente se decodifica en estructuras fijas que **rechazan campos desconocidos**. |
| **Inyección en GraphQL** | Las consultas son constantes; los datos viajan como variables. El ID de proyecto se valida con expresión regular. |
| **Inyección de fórmulas en CSV** | Al exportar, celdas que empiezan con `= + - @` se prefijan con `'` para que Excel no las ejecute. |
| **Path traversal** | Solo se sirven archivos embebidos en el binario bajo `/static/`. |
| **DoS básico** | Timeouts de lectura/escritura/encabezados, límite de 64 KB en cuerpos JSON y en cabeceras, límite de refrescos a GitHub (6/min por usuario), caché. |
| Fuga de información | Errores genéricos al cliente (sin trazas); `Referrer-Policy: no-referrer`; enlaces externos con `rel="noopener noreferrer"`; `Cache-Control: no-store` en la API. |

Otras cabeceras: `X-Content-Type-Options: nosniff`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` y `Strict-Transport-Security` con HTTPS.

## 5. Infraestructura (Docker)

- Imagen final **distroless** sin shell, usuario **no root**, binario estático.
- Contenedor `read_only`, `no-new-privileges`, todas las *capabilities* eliminadas.
- MongoDB **no se expone** fuera de la red interna de Docker; la app se publica solo en `127.0.0.1`.
- Usuario de MongoDB de la aplicación con rol `readWrite` **solo** sobre su base (mínimo privilegio); la app no recibe la contraseña root.
- El build ejecuta las pruebas automatizadas antes de generar la imagen.

## 6. Trazabilidad y privacidad

- **Bitácora de auditoría**: ingresos, fallos, bloqueos, salidas, cambios de contraseña y gestión de usuarios (con IP y resultado). Se conserva 180 días (TTL).
- Logs de acceso sin cuerpos, contraseñas ni query strings.
- **Minimización de datos**: no se copia la información de GitHub a la base de datos (solo caché en memoria por minutos). Se muestran la descripción y los campos de las tareas de los Projects, que son la información de seguimiento; no se leen código, comentarios ni otros contenidos de los repositorios.

## 7. Responsabilidades operativas (fuera del código)

1. Usar **HTTPS** en producción y `COOKIE_SECURE=true`.
2. Guardar `.env` con permisos restringidos; cambiar todas las contraseñas de ejemplo.
3. Token de GitHub de solo lectura, con expiración, idealmente de una cuenta de servicio; rotarlo periódicamente.
4. Respaldos de MongoDB y revisión periódica de la bitácora y de usuarios inactivos.
5. Mantener Go y las dependencias actualizadas (`go list -m -u all`, `govulncheck ./...`).
6. Informar a los usuarios qué datos se muestran y para qué fin (seguimiento de proyectos, no evaluación individual automatizada).
