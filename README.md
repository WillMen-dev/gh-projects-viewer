# Consulta de Proyectos de GitHub Projects

Aplicación web interna que consulta los proyectos y tareas de **GitHub Projects (v2)** y los muestra en una interfaz sencilla para personas sin acceso a GitHub (RRHH, administración, responsables de proyecto). GitHub sigue siendo la fuente de verdad: la aplicación **solo lee**, nunca crea ni modifica tareas.

| Capa | Tecnología |
|---|---|
| Backend | Go 1.24 (solo biblioteca estándar + driver oficial de MongoDB) |
| Frontend | HTML5, CSS3 y JavaScript sin frameworks (embebido en el binario) |
| Base de datos | MongoDB 7 (usuarios, sesiones y auditoría; **no** replica datos de GitHub) |
| Integración | API GraphQL de GitHub (ProjectsV2), solo consultas |

> **¿Usas Mac?** Sigue [GUIA_MAC.md](GUIA_MAC.md): instalación, conexión con tu cuenta de GitHub, MongoDB real y pruebas, paso a paso.
>
> **¿Vas a instalarla en el servidor de la empresa?** Sigue [INSTALACION_SERVIDOR.md](INSTALACION_SERVIDOR.md).

## Funcionalidades

- Inicio de sesión con usuarios propios de la aplicación (roles **Administrador** y **Consulta**).
- Lista de proyectos de la organización con búsqueda y opción de ver cerrados.
- Vista de proyecto con **resumen**: total, completadas, abiertas, vencidas, por vencer (7 días), sin responsable, % de avance y estimación completada; desglose por estado, responsable y prioridad.
- **Tablero** (columnas por estado) y **Tabla** ordenable.
- **Filtros**: texto libre, estado, prioridad, responsable y "solo vencidas".
- Detalle de cada tarea (responsables, estado, prioridad, estimación, fechas, iteración, etiquetas y campos personalizados).
- **Exportar CSV** de las tareas filtradas (reemplaza el copiado manual a Excel/Sheets).
- Botón "Actualizar desde GitHub" (la información se guarda en caché 2 minutos por defecto).
- Administración: crear usuarios, cambiar rol, activar/desactivar, restablecer contraseña y **bitácora de auditoría**.
- **Modo demo** con datos ficticios para probar sin token de GitHub.

## Estructura

```
cmd/server/            Punto de entrada (main.go) y comandos check / create-admin
internal/config/       Configuración por variables de entorno
internal/github/       Cliente GraphQL, mapeo de campos, caché y datos demo
internal/auth/         Hash de contraseñas, tokens, limitador de intentos
internal/store/        Persistencia: MongoDB y memoria (pruebas)
internal/httpapi/      API REST, middlewares de seguridad y handlers
web/public/            Frontend (login.html, index.html, static/css, static/js)
deploy/mongo-init.js   Crea el usuario de MongoDB con mínimo privilegio
Dockerfile, docker-compose.yml, .env.example, Makefile
SEGURIDAD.md           Detalle de las medidas de seguridad
GUIA_MAC.md            Paso a paso en Mac con GitHub y MongoDB reales
```

---

## Paso a paso para ejecutarlo

Hay tres formas, de la más sencilla a la más completa. Recomendación: haga primero la **Opción A** para ver la aplicación funcionando en 2 minutos.

### Requisitos

- **Go 1.24 o superior**: https://go.dev/dl/ (verifique con `go version`).
- Para la opción B: **MongoDB 7** local (https://www.mongodb.com/try/download/community).
- Para la opción C: **Docker Desktop** (incluye Docker Compose).

Descomprima el zip y abra una terminal en la carpeta `gh-projects-viewer`.

### Opción A — Modo demo (sin GitHub y sin MongoDB)

1. Descargue las dependencias (genera `go.sum`, solo la primera vez):
   ```bash
   go mod tidy
   ```
2. Inicie el servidor.

   **Linux / macOS / Git Bash:**
   ```bash
   GITHUB_MODE=demo STORE=memory COOKIE_SECURE=false \
   BOOTSTRAP_ADMIN_EMAIL=admin@empresa.com BOOTSTRAP_ADMIN_PASSWORD=CambiarEsta2026 \
   go run ./cmd/server
   ```
   **Windows PowerShell:**
   ```powershell
   $env:GITHUB_MODE="demo"; $env:STORE="memory"; $env:COOKIE_SECURE="false"
   $env:BOOTSTRAP_ADMIN_EMAIL="admin@empresa.com"; $env:BOOTSTRAP_ADMIN_PASSWORD="CambiarEsta2026"
   go run ./cmd/server
   ```
   (o simplemente `make demo` si tiene `make`).
3. Abra **http://localhost:8080** e ingrese con `admin@empresa.com` / `CambiarEsta2026`.
4. La aplicación le pedirá **cambiar la contraseña** (mínimo 12 caracteres con letras y números). Después verá los 3 proyectos de demostración.

> En modo `STORE=memory` los usuarios se borran al detener el servidor.

### Opción B — Local con GitHub real y MongoDB

1. Instale e inicie MongoDB (por defecto en `mongodb://localhost:27017`).
2. Cree el token de GitHub (ver sección siguiente).
3. Copie la configuración y edítela (la app **lee `.env` automáticamente**):
   ```bash
   cp .env.example .env        # Windows: copy .env.example .env
   ```
   Complete en `.env` al menos: `GITHUB_TOKEN`, `GITHUB_OWNER_TYPE` (`auto`, `org` o `user`; ver [Organizaciones](#organizaciones)), `GITHUB_OWNER` si aplica y `BOOTSTRAP_ADMIN_EMAIL`/`BOOTSTRAP_ADMIN_PASSWORD`.
4. Verifique todo antes de arrancar:
   ```bash
   go run ./cmd/server check
   ```
   Muestra si el token es válido, cuántos proyectos ve, los nombres reales de los campos y si MongoDB responde.
5. Inicie: `go run ./cmd/server` y abra http://localhost:8080.

### Comandos disponibles

| Comando | Para qué |
|---|---|
| `go run ./cmd/server` | Inicia la aplicación |
| `go run ./cmd/server check` | Diagnóstico de configuración, GitHub y MongoDB (no modifica nada) |
| `ADMIN_EMAIL=... ADMIN_PASSWORD=... go run ./cmd/server create-admin` | Crea un administrador o restablece su contraseña (si se olvidó) |
| `MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/store -run Mongo -v` | Prueba la capa de datos contra un MongoDB real |

### Opción C — Docker Compose (aplicación + MongoDB)

1. `cp .env.example .env` y edítelo: **cambie todas las contraseñas** (`MONGO_ROOT_PASSWORD`, `MONGO_APP_PASSWORD`, `BOOTSTRAP_ADMIN_PASSWORD`) y configure GitHub (o deje `GITHUB_MODE=demo`).
   - Use contraseñas de Mongo sin caracteres `@ : / ? #` (van dentro de la URI de conexión).
2. Levante todo:
   ```bash
   docker compose up -d --build
   ```
   El build ejecuta también las pruebas automatizadas; si alguna falla, la imagen no se construye.
3. Revise el estado y los logs:
   ```bash
   docker compose ps
   docker compose logs -f app
   ```
4. Abra http://localhost:8080.
5. Para detener: `docker compose down` (agregue `-v` solo si quiere **borrar** la base de datos).

---

## Crear el token de GitHub (solo lectura)

**Opción recomendada — Fine-grained token** (GitHub → Settings → Developer settings → Personal access tokens → *Fine-grained tokens* → *Generate new token*):

- **Resource owner**: la organización (puede requerir aprobación de un administrador de la organización).
- **Expiration**: 90 días o menos (y renovarlo).
- **Repository access**: solo los repositorios cuyos issues/PR aparecen en los proyectos.
- **Repository permissions**: `Issues: Read-only`, `Pull requests: Read-only`, `Metadata: Read-only`.
- **Organization permissions**: `Projects: Read-only`.
- **Ningún** permiso de escritura.

**Proyectos de cuenta personal — Token clásico**: scopes `read:project` y `repo` (este último solo si los repositorios son privados). Para proyectos de usuario se usa el token clásico; el fine-grained se recomienda para organizaciones.

Ideal en la empresa: crear el token con una **cuenta de servicio** dedicada, no con la cuenta personal de un desarrollador.

### Organizaciones

La aplicación muestra los Projects de **una o varias organizaciones**, agrupados por organización. En la pantalla de proyectos hay un filtro por organización, el detalle de cada proyecto indica a qué organización pertenece y el CSV incluye las columnas *Organización* y *Proyecto*.

| `GITHUB_OWNER_TYPE` | Qué proyectos se muestran |
|---|---|
| `auto` (recomendado) | Los de **todas las organizaciones a las que pertenece la cuenta del token**, detectadas automáticamente (se revisan cada 5 minutos). Requiere token clásico con `read:org`. `GITHUB_OWNER` es opcional y agrega dueños extra (p. ej. un usuario personal). |
| `org` / `user` | Solo los de los dueños listados en `GITHUB_OWNER`, separados por coma: `GITHUB_OWNER=empresa-a,empresa-b`. |

- Se ven los Projects que **pertenecen a la organización**, sin importar qué miembro los creó. Los Projects privados solo se ven si la cuenta del token tiene acceso; por eso conviene una cuenta de servicio con lectura sobre toda la organización.
- Los Projects personales de los miembros (`github.com/users/...`) no pertenecen a la organización y no aparecen.
- Si una organización falla (por ejemplo exige SSO y el token no está autorizado), se omite y se muestran las demás; `check` indica cuál falló.
- Todos los usuarios de la aplicación ven todas las organizaciones configuradas.

### Nombres de campos

GitHub Projects permite nombrar los campos libremente. La app reconoce por defecto `Status/Estado`, `Priority/Prioridad`, `Estimate/Estimación/Size`, `Start date/Fecha de inicio` y `Target date/Due date/Fecha de entrega`. Si el proyecto usa otros nombres, ajústelos en `.env` (`FIELD_STATUS`, `FIELD_PRIORITY`, etc., separados por coma). Los demás campos personalizados aparecen igual en el detalle de cada tarea.

---

## Cómo probarlo

### 1. Pruebas automatizadas

```bash
go mod tidy
go vet ./...
go test ./...
```

Resultado esperado: `ok` en `internal/auth`, `internal/github` e `internal/httpapi`. Las pruebas cubren:

| Paquete | Qué verifica |
|---|---|
| `config` | Lectura del archivo `.env` y validación de la configuración |
| `store` | Contrato de persistencia (usuarios, sesiones, auditoría) en memoria y, con `MONGO_TEST_URI`, en MongoDB real |
| `auth` | Hash PBKDF2 (sal única, comparación), política de contraseñas, tokens, bloqueo por intentos |
| `github` | Errores parciales (tareas de repos sin acceso no tumban el proyecto), token inválido (401), permisos/SSO (403), consulta y paginación con un servidor GitHub simulado, mapeo de campos (estado, prioridad, estimación, fechas, iteración), borradores y elementos restringidos, rechazo de proyectos de otro dueño, manejo de errores (403, NOT_FOUND, 500), caché e invalidación, que nunca se envíen mutaciones |
| `httpapi` | Acceso sin sesión → 401, login correcto/incorrecto con mensaje genérico, cookie HttpOnly + SameSite, cambio obligatorio de contraseña, CSRF, logout que invalida la sesión, detalle y resumen del proyecto, IDs inválidos, permisos por rol, bloqueo por fuerza bruta, cabeceras de seguridad, rechazo de peticiones de otro origen, path traversal, campos desconocidos, expiración por inactividad, gestión de usuarios y auditoría |

Para ver el detalle: `go test -v ./...` · cobertura: `go test -cover ./...`

### 2. Pruebas manuales (lista de verificación)

Con la aplicación en modo demo (Opción A):

1. Entrar a http://localhost:8080 sin sesión → redirige a `/login`.
2. Contraseña incorrecta → "Correo o contraseña incorrectos".
3. 5 intentos fallidos seguidos con el mismo correo → bloqueo de 15 minutos.
4. Ingreso correcto → obliga a cambiar la contraseña.
5. Lista de proyectos → 3 proyectos; probar el buscador.
6. Abrir "Portal de clientes" → revisar tarjetas de resumen y barras.
7. Tablero: 4 columnas; la tarea vencida aparece con borde rojo.
8. Marcar "Solo vencidas", filtrar por responsable y por prioridad, buscar "pagos".
9. Pestaña Tabla → ordenar por columnas.
10. Clic en una tarea → ventana de detalle → `Esc` para cerrar.
11. "Exportar CSV" → abrir el archivo en Excel.
12. Administración → crear usuario con rol Consulta (botón "Generar" para la contraseña temporal).
13. En otra ventana privada, ingresar con ese usuario → no ve el menú Administración; `http://localhost:8080/api/admin/users` devuelve 403.
14. Desactivar ese usuario desde el admin → su sesión se cierra inmediatamente.
15. Revisar la bitácora de auditoría.

### 3. Validación contra GitHub (objetivo específico 8)

Con `GITHUB_MODE=api`, elija un proyecto real y compare 5–10 tareas entre la app y GitHub: título, estado, responsable, prioridad y fechas. Cambie una tarea en GitHub, pulse "Actualizar desde GitHub" y verifique que se refleja. Documente los resultados como evidencia.

### Probar la API directamente (opcional)

```bash
# login (guarda la cookie)
curl -c cookies.txt -H "Content-Type: application/json" \
  -d '{"email":"admin@empresa.com","password":"SuNuevaClave2026"}' http://localhost:8080/api/auth/login
# listar proyectos
curl -b cookies.txt http://localhost:8080/api/projects
```

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/healthz` | Estado del servicio |
| POST | `/api/auth/login` | Iniciar sesión |
| POST | `/api/auth/logout` | Cerrar sesión |
| GET | `/api/auth/me` | Usuario actual y token CSRF |
| POST | `/api/auth/password` | Cambiar contraseña |
| GET | `/api/projects` | Lista de proyectos |
| GET | `/api/projects/{id}` | Proyecto, tareas y resumen |
| POST | `/api/projects/{id}/refresh` | Vaciar caché del proyecto |
| GET/POST | `/api/admin/users` | Listar / crear usuarios (admin) |
| PATCH | `/api/admin/users/{id}` | Cambiar rol, estado o contraseña (admin) |
| GET | `/api/admin/audit` | Bitácora (admin) |

Las peticiones POST/PATCH requieren la cabecera `X-CSRF-Token` (la entrega `/api/auth/me` o el login).

---

## Qué tiene y qué le falta para producción

**Ya funciona en entorno real** (no requiere cambios de código): conexión a la API real de GitHub, MongoDB real con índices y TTL, usuarios y roles, auditoría, Docker, y el diagnóstico `check`.

**Lo que falta es principalmente infraestructura y decisiones de la empresa:**

| Pendiente | Por qué | Cómo |
|---|---|---|
| Servidor y dominio | Hoy corre en un computador local | VM interna o nube (presupuesto "Por definir") con Docker |
| **HTTPS** | Sin él, contraseñas y cookies viajan en claro | Proxy Caddy/Nginx con certificado; `COOKIE_SECURE=true`, `TRUST_PROXY_HEADERS=true` |
| MongoDB con autenticación y respaldos | El Mongo local no tiene usuario ni backups | El `docker-compose.yml` ya crea usuario de mínimo privilegio; programar `mongodump` diario o usar MongoDB Atlas |
| Token de la empresa | Un token personal depende de una persona | Cuenta de servicio o **GitHub App** de solo lectura (recomendado a mediano plazo) |
| Aprobación de la organización | Las orgs suelen exigir aprobar tokens o SSO | Solicitar al administrador de GitHub |
| Acceso restringido por red | Es una herramienta interna | Publicarla solo en la red interna / VPN |
| Monitoreo | Saber si se cae | `/healthz` con un monitor (Uptime Kuma, etc.); logs JSON centralizados |
| Recuperación de contraseña por correo | Hoy el admin la restablece | Opcional: integrar SMTP, o inicio de sesión con Google/Microsoft de la empresa (SSO) |
| CI/CD | Calidad en cada cambio | GitHub Actions ejecutando `go vet`, `go test` y `govulncheck` |

Ejemplo de Caddy (certificado HTTPS automático):
```
proyectos.empresa.com {
    reverse_proxy 127.0.0.1:8080
}
```

## Solución de problemas

| Síntoma | Causa probable |
|---|---|
| `GITHUB_TOKEN es obligatorio` | Está en `GITHUB_MODE=api` sin token. Use `demo` o configure el token. |
| "GitHub limitó temporalmente las consultas" | Token sin permisos (403) o límite de API. Revise los permisos del token. |
| No aparecen proyectos | `GITHUB_OWNER`/`GITHUB_OWNER_TYPE` incorrectos, o el token no tiene acceso a los proyectos de la organización. |
| Tareas sin estado/prioridad | Los campos tienen otro nombre: ajuste `FIELD_STATUS`, `FIELD_PRIORITY`, etc. |
| El login no mantiene la sesión en http:// | `COOKIE_SECURE=true` sin HTTPS. En local use `false`. |
| `no fue posible conectar con MongoDB` | Mongo apagado o `MONGO_URI` incorrecta. |
| Nadie puede ingresar | No se definió `BOOTSTRAP_ADMIN_EMAIL` cuando la BD estaba vacía. Use `create-admin`. |
| Tareas "(Elemento sin acceso)" | El token no tiene acceso a ese repositorio privado. |

Ejecute siempre `go run ./cmd/server check` primero: indica la causa de la mayoría de problemas.
