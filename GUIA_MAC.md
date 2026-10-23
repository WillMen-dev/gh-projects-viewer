# Guía paso a paso en Mac: GitHub real + MongoDB real

Esta guía lleva la aplicación desde el zip hasta funcionar con **tus proyectos reales de GitHub** y **MongoDB real** en tu Mac (Apple Silicon o Intel). Tiempo estimado: 30–45 minutos la primera vez.

Todos los comandos se escriben en la app **Terminal** (Cmd + Espacio → "Terminal").

---

## Paso 1. Instalar Homebrew (gestor de paquetes)

Si ya lo tienes (`brew --version` responde), salta al paso 2.

```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

Al terminar, el instalador muestra "Next steps" con dos comandos para agregar `brew` al PATH (en Mac con chip Apple suelen ser):

```bash
echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile
eval "$(/opt/homebrew/bin/brew shellenv)"
```

Comprueba: `brew --version`

## Paso 2. Instalar Go y MongoDB

```bash
brew install go
brew tap mongodb/brew
brew install mongodb-community
brew services start mongodb-community
```

Verifica:

```bash
go version                                   # debe ser 1.24 o superior
mongosh --eval "db.runCommand({ping:1})"     # debe responder { ok: 1 }
```

> MongoDB queda corriendo en segundo plano, solo accesible desde tu Mac (`localhost:27017`) y arranca solo al encender el equipo. Para detenerlo: `brew services stop mongodb-community`.
>
> Opcional: una interfaz gráfica para ver la base de datos: `brew install --cask mongodb-compass`.

## Paso 3. Preparar el proyecto y correr las pruebas

```bash
cd ~/Downloads                      # o donde descargaste el zip
unzip gh-projects-viewer.zip
cd gh-projects-viewer

go mod tidy                         # descarga dependencias (solo la primera vez)
go test ./...                       # pruebas unitarias: todo debe decir "ok"
make test-mongo                     # prueba contra tu MongoDB real: debe decir "PASS"
```

`make test-mongo` crea una base temporal, prueba usuarios, sesiones y auditoría, y la borra. Si pasa, la capa de base de datos funciona con Mongo real.

## Paso 4. Preparar GitHub

### 4.1 Crear un proyecto de prueba (recomendado antes de usar los de la empresa)

1. Entra a https://github.com → tu foto (arriba a la derecha) → **Your projects** → **New project**.
2. Elige la plantilla **Board** (tablero) y ponle un nombre, por ejemplo "Prueba práctica".
3. El campo **Status** ya existe (Todo / In Progress / Done).
4. Agrega campos con el botón **+** en la vista de tabla (o en *Settings* del proyecto → *Custom fields*):
   - **Priority** → tipo *Single select* con opciones `P0`, `P1`, `P2`
   - **Estimate** → tipo *Number*
   - **Start date** → tipo *Date*
   - **Target date** → tipo *Date*
5. Crea un repositorio de prueba (puede ser privado) con 5–8 issues. Agrégalos al proyecto (en el proyecto: **+ Add item** → `#` → elige el repositorio y el issue). También puedes crear ítems "Draft" directamente.
6. Asígnate algunas tareas, ponles estado, prioridad, estimación y fechas. Deja **una con fecha de entrega pasada** y que no esté en Done, para probar "vencidas".

### 4.2 Crear el token de acceso (solo lectura)

**Si los proyectos son de tu cuenta personal → token clásico:**

1. GitHub → foto → **Settings** → (abajo a la izquierda) **Developer settings** → **Personal access tokens** → **Tokens (classic)** → **Generate new token (classic)**.
2. *Note*: `consulta-proyectos`. *Expiration*: 30 o 90 días.
3. Marca **solo**:
   - `read:project`
   - `repo` **solo si** los issues del proyecto están en repositorios privados.
4. **Generate token** y copia el valor (`ghp_...`). GitHub no lo vuelve a mostrar.

**Si los proyectos son de una organización (la empresa) → token fine-grained:**

1. **Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token**.
2. *Resource owner*: la organización. *Expiration*: 90 días o menos.
3. *Repository access*: solo los repositorios de los proyectos.
4. *Repository permissions*: `Issues: Read-only`, `Pull requests: Read-only` (Metadata se agrega solo).
5. *Organization permissions*: `Projects: Read-only`.
6. Según la configuración de la organización, un administrador debe **aprobar** el token. Si la organización usa SSO, en la lista de tokens pulsa **Configure SSO → Authorize**.

> El token es una contraseña: no lo pegues en chats, correos ni lo subas a Git.

## Paso 5. Configurar la aplicación

```bash
cp .env.example .env
chmod 600 .env          # solo tu usuario puede leerlo
open -e .env            # lo abre en TextEdit
```

Edita estas líneas (el resto puede quedar igual):

```
GITHUB_MODE=api
GITHUB_TOKEN=ghp_tu_token_aqui
GITHUB_OWNER=tu-usuario-de-github        # o el nombre de la organización
GITHUB_OWNER_TYPE=user                   # org si es de una organización
STORE=mongo
MONGO_URI=mongodb://localhost:27017
BOOTSTRAP_ADMIN_EMAIL=tu-correo@empresa.com
BOOTSTRAP_ADMIN_PASSWORD=UnaClaveTemporal2026
```

¿Cuál es el `GITHUB_OWNER`? Mira la URL del proyecto: `github.com/users/**wmendoza**/projects/1` → `wmendoza` y tipo `user`; `github.com/orgs/**mi-empresa**/projects/3` → `mi-empresa` y tipo `org`.

Guarda el archivo (Cmd + S).

## Paso 6. Verificar la conexión

```bash
go run ./cmd/server check
```

Resultado esperado (ejemplo):

```
1) Configuración
  ✅ Variables de entorno válidas (GITHUB_MODE=api, STORE=mongo)
2) MongoDB
  ✅ Conectado a la base "gh_projects_viewer" (0 usuario(s) registrados)
     Al iniciar se creará el administrador tu-correo@empresa.com
3) GitHub
  ✅ Token válido. Pertenece al usuario de GitHub: wmendoza
     Token clásico con permisos: read:project
  ✅ 1 proyecto(s) encontrados en wmendoza
  ✅ Proyecto #1 "Prueba práctica": 7 tarea(s)
     Campos del proyecto: Title, Assignees, Status, Labels, ..., Priority, Estimate, Start date, Target date
       Estado          reconocido en 7 tarea(s)
       Prioridad       reconocido en 6 tarea(s)
       ...
Todo listo. Inicie la aplicación con: go run ./cmd/server
```

Si aparece ❌ o ⚠️, el mismo mensaje indica qué corregir. El caso más común es que un campo se llame distinto (por ejemplo "Fecha límite"). Agrégalo a la variable correspondiente en `.env`, por ejemplo `FIELD_DUE_DATE=Target date,Fecha límite`, y vuelve a ejecutar `check`.

## Paso 7. Iniciar la aplicación

```bash
go run ./cmd/server
```

Verás `servidor escuchando ... abrir=http://localhost:8080`. Abre **http://localhost:8080** en Safari o Chrome.

1. Ingresa con `BOOTSTRAP_ADMIN_EMAIL` y `BOOTSTRAP_ADMIN_PASSWORD`.
2. Cambia la contraseña (mínimo 12 caracteres, letras y números).
3. Verás tus proyectos reales de GitHub.

Para detener el servidor: **Ctrl + C** en la Terminal.

> Después del primer inicio, `BOOTSTRAP_ADMIN_PASSWORD` ya no se usa (el admin existe en Mongo). Puedes borrarla del `.env`.

## Paso 8. Pruebas en entorno real (evidencia para el informe)

| # | Prueba | Resultado esperado |
|---|---|---|
| 1 | Abrir un proyecto y comparar 5–10 tareas con GitHub (título, estado, responsable, prioridad, fechas) | Coinciden |
| 2 | En GitHub, cambiar el estado de una tarea; en la app pulsar **Actualizar desde GitHub** | Se refleja el cambio |
| 3 | Sin pulsar actualizar, esperar 2 minutos y recargar | Se refleja (caché expiró) |
| 4 | Filtro "Solo vencidas" | Solo la tarea con fecha pasada |
| 5 | Exportar CSV y abrirlo en Excel / Numbers | Mismas tareas que en pantalla |
| 6 | Administración → crear usuario "RRHH" con rol Consulta | Aparece en la lista |
| 7 | Ventana privada (Cmd+Shift+N) → ingresar como RRHH | Pide cambio de clave; no ve Administración |
| 8 | Detener (Ctrl+C) y volver a iniciar el servidor | Los usuarios siguen existiendo (persisten en Mongo) |
| 9 | 5 contraseñas incorrectas seguidas | Bloqueo de 15 minutos |
| 10 | Desactivar al usuario RRHH desde Administración | Su sesión se cierra al instante |
| 11 | Revisar la bitácora de auditoría | Registra ingresos, fallos y cambios |

Ver los datos guardados en MongoDB:

```bash
mongosh gh_projects_viewer
db.users.find({}, {password_hash: 0})            // usuarios (sin el hash)
db.sessions.countDocuments()                     // sesiones activas
db.audit_log.find().sort({at: -1}).limit(5)      // últimos eventos
exit
```

Fíjate que en MongoDB **no** hay datos de GitHub (solo usuarios, sesiones y auditoría): GitHub sigue siendo la fuente de verdad.

## Paso 9 (opcional). Probar con Docker Desktop

Útil para simular el despliegue en el servidor de la empresa.

1. Instala Docker Desktop: `brew install --cask docker` y ábrelo desde Aplicaciones.
2. Detén el Mongo de Homebrew para evitar confusiones: `brew services stop mongodb-community`.
3. En `.env` cambia las contraseñas `MONGO_ROOT_PASSWORD` y `MONGO_APP_PASSWORD` (sin `@ : / ? #`).
4. `docker compose up -d --build` → abre http://localhost:8080.
5. Logs: `docker compose logs -f app` · Detener: `docker compose down`.

---

## Problemas comunes

| Mensaje / síntoma | Solución |
|---|---|
| `command not found: brew` | Ejecuta los dos comandos de "Next steps" del paso 1 y abre una Terminal nueva. |
| `command not found: go` tras instalar | Abre una Terminal nueva. |
| `no fue posible conectar con MongoDB` | `brew services start mongodb-community` y revisa con `brew services list`. |
| `El token de GitHub no es válido o expiró` | Token mal copiado, expirado o revocado. Genera uno nuevo. |
| `no tiene permisos suficientes` | Falta `read:project`, o la organización exige aprobar el token / autorizar SSO. |
| `organization "x" no existe o el token no tiene acceso` | Revisa `GITHUB_OWNER` y `GITHUB_OWNER_TYPE` (user vs org). |
| Proyecto aparece pero sin tareas | Las tareas deben estar agregadas al *proyecto*, no solo al repositorio. |
| Tareas "(Elemento sin acceso)" | Son de un repositorio privado sin permiso: agrega `repo` (clásico) o ese repositorio (fine-grained). |
| Estado/prioridad/fechas vacíos | Nombres de campo distintos: ajusta `FIELD_*` en `.env` (el `check` muestra los nombres reales). |
| Login correcto pero vuelve al login | `COOKIE_SECURE=true` en http. Para localhost usa `false`. |
| Olvidé la clave del administrador | `ADMIN_EMAIL=tu@correo.com ADMIN_PASSWORD='NuevaTemporal2026' go run ./cmd/server create-admin` |
| `address already in use` | Ya hay otro servidor en el 8080: ciérralo, o cambia `APP_ADDR=127.0.0.1:8081`. |
