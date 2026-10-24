# Consulta de Proyectos de GitHub Projects

Aplicación web interna que muestra los **proyectos y tareas de GitHub Projects** a personas que no usan GitHub (RRHH, gerencia, administración, responsables de área), en una interfaz sencilla, con resumen de avance, tablero, tabla, filtros y exportación a CSV.

**GitHub es la fuente de la verdad.** La aplicación **solo lee**: nunca crea, modifica ni borra tareas, proyectos ni repositorios. Lo que cambie en GitHub se ve en la aplicación en máximo 2 minutos (o al instante con el botón *Actualizar desde GitHub*).

---

## Contenido

1. [Qué es y qué alcance tiene](#1-qué-es-y-qué-alcance-tiene)
2. [Cómo funciona](#2-cómo-funciona)
3. [Conceptos de GitHub que hay que conocer](#3-conceptos-de-github-que-hay-que-conocer)
4. [Requisitos](#4-requisitos)
5. [Roles y responsabilidades](#5-roles-y-responsabilidades)
6. [Instalación en un servidor (producción)](#6-instalación-en-un-servidor-producción)
7. [Instalación local para probar (Mac, Linux o Windows)](#7-instalación-local-para-probar-mac-linux-o-windows)
8. [Configuración: referencia completa del archivo `.env`](#8-configuración-referencia-completa-del-archivo-env)
9. [Preparar los Projects en GitHub](#9-preparar-los-projects-en-github)
10. [Guía del usuario (rol Consulta)](#10-guía-del-usuario-rol-consulta)
11. [Guía del administrador de la aplicación](#11-guía-del-administrador-de-la-aplicación)
12. [Guía de TI: operación y mantenimiento](#12-guía-de-ti-operación-y-mantenimiento)
13. [Guía técnica para desarrolladores](#13-guía-técnica-para-desarrolladores)
14. [Base de datos](#14-base-de-datos)
15. [Seguridad](#15-seguridad)
16. [Límites conocidos y posibles mejoras](#16-límites-conocidos-y-posibles-mejoras)
17. [Solución de problemas](#17-solución-de-problemas)
18. [Glosario](#18-glosario)
19. [Referencia rápida de comandos](#19-referencia-rápida-de-comandos)

---

## 1. Qué es y qué alcance tiene

### Problema que resuelve

Los equipos de desarrollo organizan su trabajo en GitHub Projects, pero las áreas no técnicas no tienen cuenta de GitHub, no saben usarlo o no deberían tener acceso a los repositorios. Hasta ahora el seguimiento se hacía copiando datos a mano a Excel o Google Sheets. Esta aplicación les da acceso de **solo lectura** a esa información, siempre actualizada, sin copiar nada a mano.

### Qué hace

| Función | Detalle |
|---|---|
| **Inicio de sesión propio** | Usuarios y contraseñas de la aplicación (no se necesita cuenta de GitHub). Dos roles: **Administrador** y **Consulta**. |
| **Varias organizaciones** | Muestra los Projects de todas las organizaciones de GitHub a las que pertenece la cuenta del token, agrupados por organización. |
| **Lista de proyectos** | Búsqueda por nombre, filtro por organización, opción de ver proyectos cerrados. |
| **Resumen del proyecto** | Total de tareas, completadas, abiertas, vencidas, que vencen en 7 días, sin responsable, % de avance, estimación completada y desglose por estado, responsable y prioridad. |
| **Tablero** | Columnas por estado, en el mismo orden que en GitHub. Las tareas vencidas se resaltan en rojo. |
| **Tabla** | Lista ordenable por tarea, estado, prioridad, responsables, estimación, inicio y entrega. |
| **Filtros** | Texto libre (título, descripción, número, repositorio, etiquetas, responsables), estado, prioridad, responsable y "solo vencidas". |
| **Detalle de tarea** | Título, descripción completa, tipo, repositorio, estados, prioridad, estimación, fechas, iteración, responsables, etiquetas y **todos los campos del proyecto** (incluidos Milestone, issue padre, progreso de sub-issues y campos personalizados), tengan valor o no. |
| **Exportar CSV** | Exporta las tareas que se ven con los filtros aplicados, con todas sus columnas. Se abre en Excel, Numbers o Google Sheets. |
| **Administración** | Crear usuarios, cambiar rol, activar/desactivar, restablecer contraseña y bitácora de auditoría. |
| **Diagnóstico** | Comando `check` que verifica token, organizaciones, proyectos, campos y base de datos, e indica qué corregir. |
| **Modo demo** | Datos ficticios para probar sin GitHub ni base de datos. |

### Qué NO hace (alcance)

- **No escribe en GitHub.** No se pueden crear, mover ni editar tareas desde la aplicación; eso se sigue haciendo en GitHub.
- **No copia los datos de GitHub** a su base de datos. Solo los guarda en memoria unos minutos (caché).
- **No muestra repositorios sueltos**, solo Projects. Un repositorio aparece en la aplicación cuando sus issues o pull requests están agregados a un Project.
- **No muestra los Projects personales** de los miembros de una organización (los que están en `github.com/users/...`), salvo que se agregue ese usuario explícitamente en la configuración.
- **No separa permisos por organización**: todos los usuarios de la aplicación ven todas las organizaciones configuradas.
- **No envía correos** (ni para recuperar contraseñas ni notificaciones). Las contraseñas las restablece un administrador.
- **No tiene inicio de sesión con Google/Microsoft (SSO)** para los usuarios de la aplicación.
- **No evalúa personas**: muestra el estado del trabajo, no calcula indicadores de desempeño individual.

---

## 2. Cómo funciona

```
 Usuarios (navegador)                Servidor de la empresa                       GitHub
┌───────────────────┐   HTTPS   ┌──────────────────────────────┐   API GraphQL  ┌────────────────────┐
│ RRHH, gerencia,   │ ────────► │  Aplicación (Go)             │ ─────────────► │ Organizaciones     │
│ administradores   │ ◄──────── │   · interfaz web             │ ◄───────────── │  └ Projects        │
└───────────────────┘           │   · caché en memoria (2 min) │  solo lectura  │     └ Tareas       │
                                │                              │  (token)       │        (issues/PR) │
                                │  MongoDB                     │                └────────────────────┘
                                │   · usuarios                 │
                                │   · sesiones                 │
                                │   · bitácora de auditoría    │
                                └──────────────────────────────┘
```

1. Un usuario entra a la aplicación con su correo y contraseña (cuentas propias de la aplicación, guardadas en MongoDB).
2. Al abrir la lista de proyectos, la aplicación consulta a GitHub con **un único token** guardado en el servidor (archivo `.env`). El token nunca llega al navegador.
3. La aplicación pregunta a GitHub a qué organizaciones pertenece la cuenta del token y obtiene sus Projects.
4. Al abrir un proyecto, trae todas sus tareas y campos, calcula el resumen y lo muestra.
5. Las respuestas se guardan en memoria durante 2 minutos para no consultar GitHub en cada clic.

**Consecuencias importantes:**

- Todos los usuarios ven **lo mismo**: lo que puede ver la cuenta de GitHub dueña del token.
- Los usuarios de la aplicación **no necesitan cuenta de GitHub**.
- Si se pierde la base de datos, se pierden los usuarios y la bitácora, **pero no los proyectos** (siguen en GitHub).

---

## 3. Conceptos de GitHub que hay que conocer

| Concepto | Qué es |
|---|---|
| **Cuenta de usuario** | Cuenta personal de una persona en GitHub (p. ej. `WillMen-dev`). |
| **Organización** | Espacio compartido de una empresa o equipo (p. ej. `github.com/mi-empresa`). La crea cualquier cuenta (gratis) y queda como **dueña** (*owner*); invita a otras cuentas como **miembros**. Una cuenta puede pertenecer a varias organizaciones. |
| **Repositorio** | Donde vive el código y los **issues** (tareas) y **pull requests** (cambios de código). |
| **Project (GitHub Projects)** | Tablero/tabla de seguimiento que agrupa issues, pull requests y borradores de **uno o varios repositorios**, con campos como Status, Priority o fechas. Puede pertenecer a una organización (`github.com/orgs/ORG/projects/N`) o a un usuario (`github.com/users/USUARIO/projects/N`). |
| **Item / tarea** | Cada elemento de un Project: un issue, un pull request o un **borrador** (*draft*, tarea que existe solo en el Project). |
| **Campos del Project** | Columnas de información de cada tarea. Hay campos **propios de GitHub** (Title, Assignees, Status, Labels, Milestone, Repository, Reviewers, Linked pull requests, Parent issue, Sub-issues progress) y campos **creados por el equipo** (Priority, Estimate, Start date, Target date o cualquier otro). |
| **Token de acceso personal** | "Llave" que permite a un programa consultar GitHub en nombre de una cuenta, con permisos limitados. La aplicación usa uno de **solo lectura**. |
| **SSO de la organización** | Algunas organizaciones exigen inicio de sesión corporativo; en ese caso el token debe "autorizarse" para esa organización. |

**¿De quién son los Projects de una organización?** De la organización, sin importar qué miembro los creó. La aplicación los muestra todos, siempre que la cuenta del token tenga acceso (los Projects privados requieren permiso de lectura).

---

## 4. Requisitos

### Para el servidor (producción)

| Requisito | Detalle |
|---|---|
| Sistema operativo | Linux (recomendado: Ubuntu 22.04+ o Debian 12+), Windows Server o macOS. |
| Hardware mínimo | 1 vCPU, 1 GB de RAM, 5 GB de disco. Para decenas de usuarios es suficiente. |
| Software | **Docker** con **Docker Compose** y **Git**. Nada más: la aplicación y MongoDB corren dentro de Docker. |
| Red | Salida a internet hacia `api.github.com` (puerto 443). Acceso desde la red de los usuarios al puerto de la aplicación. |
| Opcional | Un dominio (p. ej. `proyectos.empresa.com`) y un proxy HTTPS (Caddy o Nginx). |

### Para GitHub

| Requisito | Detalle |
|---|---|
| Cuenta para el token | Una cuenta de GitHub que sea **miembro** de las organizaciones a mostrar. Recomendado: una **cuenta de servicio** (p. ej. `empresa-reportes`) en vez de la cuenta personal de alguien. |
| Token clásico | Con permisos `read:project`, `read:org` y `repo` (este último si hay repositorios privados). |
| Projects | Al menos un Project en la organización, con las tareas agregadas. |

### Para los usuarios

- Un navegador actualizado (Chrome, Edge, Firefox o Safari). Funciona también en el celular.
- Acceso de red al servidor.
- Un usuario de la aplicación creado por un administrador. **No necesitan cuenta de GitHub.**

### Para desarrollo local (opcional)

- **Go 1.24 o superior** (https://go.dev/dl/).
- **MongoDB 7** o superior, o usar el modo memoria (sin base de datos).

---

## 5. Roles y responsabilidades

| Rol | Quién suele ser | Qué hace |
|---|---|---|
| **Dueño de GitHub** | Quien administra la organización en GitHub | Genera el token, invita a la cuenta del token a las organizaciones, mantiene los Projects ordenados. |
| **TI** | Área de sistemas o quien tenga el servidor | Instala, configura `.env`, pone HTTPS, hace respaldos, renueva el token, actualiza la aplicación. |
| **Administrador de la aplicación** | Gerencia, RRHH, líder de proyectos | Crea y gestiona usuarios, revisa la bitácora. También consulta proyectos. |
| **Usuario de consulta** | RRHH, administración, responsables de área | Consulta proyectos, filtra y exporta reportes. |

Una misma persona puede tener varios roles (por ejemplo, TI y administrador a la vez).

---

## 6. Instalación en un servidor (producción)

Tiempo estimado: 20–30 minutos.

### Paso 1. Generar el token de GitHub (dueño de GitHub)

El token es una llave de **solo lectura**. La aplicación nunca modifica nada en GitHub.

1. Entrar a https://github.com con la cuenta que tendrá el token (idealmente la cuenta de servicio, miembro de las organizaciones).
2. Foto (arriba a la derecha) → **Settings** → abajo a la izquierda **Developer settings** → **Personal access tokens** → **Tokens (classic)** → **Generate new token (classic)**.
3. *Note*: `consulta-proyectos`. *Expiration*: 90 días (anotar la fecha para renovarlo).
4. Marcar **solo**:
   - `read:project`: leer los Projects.
   - `read:org`: saber a qué organizaciones pertenece la cuenta.
   - `repo`: leer las tareas de repositorios privados (si todos son públicos, no hace falta).
5. **Generate token** y copiar el valor (`ghp_...`). GitHub no lo vuelve a mostrar.
6. Si alguna organización usa SSO: en la lista de tokens, junto al token, **Configure SSO** → **Authorize** en cada organización.
7. Entregar el token a TI por un medio seguro (no por correo ni chat abierto).

> **Organizaciones que restringen tokens.** El dueño de una organización puede bloquear los tokens clásicos o exigir aprobarlos (organización → *Settings* → *Personal access tokens*). Si es así, debe permitirlos o aprobar el token.

### Paso 2. Preparar el servidor (TI)

Instalar Docker y Git:

- **Ubuntu/Debian:** https://docs.docker.com/engine/install/ y `sudo apt install git`.
- **Windows / Mac:** Docker Desktop (https://www.docker.com/products/docker-desktop/) y Git (https://git-scm.com/).

Verificar:
```bash
docker --version
docker compose version
git --version
```

### Paso 3. Descargar la aplicación (TI)

```bash
git clone <URL-del-repositorio> gh-projects-viewer
cd gh-projects-viewer
cp .env.example .env
chmod 600 .env        # Linux/Mac: solo el dueño del archivo puede leerlo (contiene secretos)
```

Si el repositorio es privado, la cuenta que clona necesita acceso de lectura.

### Paso 4. Configurar `.env` (TI)

Abrir `.env` con un editor (`nano .env`, Bloc de notas, etc.) y completar **los valores obligatorios**:

```ini
# --- GitHub ---
GITHUB_MODE=api
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx      # el token del paso 1
GITHUB_OWNER_TYPE=auto                     # todas las organizaciones de la cuenta del token
GITHUB_OWNER=                              # vacío, o usuarios/organizaciones extra separados por coma

# --- Primer administrador de la aplicación ---
BOOTSTRAP_ADMIN_EMAIL=admin@empresa.com
BOOTSTRAP_ADMIN_PASSWORD=ClaveTemporal2026 # mínimo 12 caracteres, letras y números; se cambia al entrar

# --- Claves de MongoDB (inventar, largas, SIN los caracteres @ : / ? #) ---
MONGO_ROOT_PASSWORD=una-clave-larga-y-aleatoria-1
MONGO_APP_PASSWORD=una-clave-larga-y-aleatoria-2
```

Para generar claves aleatorias: `openssl rand -hex 24`.

Luego elegir el escenario de acceso:

| Escenario | `APP_PUBLISH` | `COOKIE_SECURE` | `TRUST_PROXY_HEADERS` |
|---|---|---|---|
| **Con HTTPS** (recomendado): proxy Caddy/Nginx delante (paso 7) | `127.0.0.1:8080` | `true` | `true` |
| **Solo red interna, sin HTTPS**: se abre como `http://IP-del-servidor:8080` | `8080` | `false` | `false` |

> Con `COOKIE_SECURE=true` y sin HTTPS, **el inicio de sesión no funciona** (el navegador no guarda la sesión y vuelve a la pantalla de ingreso).

El resto de variables tiene valores por defecto adecuados. La referencia completa está en la [sección 8](#8-configuración-referencia-completa-del-archivo-env).

**Sobre la contraseña del primer administrador:** la aplicación la usa **una sola vez**, al arrancar con la base de datos vacía, para crear el primer administrador. En el primer ingreso obliga a cambiarla, de modo que la contraseña real solo la conoce el administrador (la de `.env` la puede leer cualquiera con acceso al servidor). Después de ese primer arranque se puede borrar de `.env`.

### Paso 5. Iniciar (TI)

```bash
docker compose up -d --build
```

La primera vez tarda unos minutos: descarga las imágenes, **ejecuta las pruebas automáticas** y compila. Si alguna prueba falla, no se genera la imagen.

Verificar:
```bash
docker compose ps                     # "app" y "mongo" deben aparecer como "healthy"
docker compose logs app | tail -20    # debe decir "servidor escuchando"
docker compose run --rm app check     # diagnóstico completo
```

El diagnóstico `check` muestra:
1. Si la configuración es válida.
2. Si MongoDB responde y cuántos usuarios hay.
3. Si el token es válido, de qué cuenta es y qué permisos tiene.
4. Qué organizaciones detectó y cuántos proyectos tiene cada una.
5. Para los primeros proyectos: cuántas tareas, qué campos tienen y si se reconocen estado, prioridad, estimación y fechas.

Si aparece ❌ o ⚠️, el mensaje indica qué corregir.

### Paso 6. Primer ingreso (administrador de la aplicación)

1. Abrir la dirección de la aplicación (`https://proyectos.empresa.com` o `http://IP-del-servidor:8080`).
2. Ingresar con `BOOTSTRAP_ADMIN_EMAIL` y `BOOTSTRAP_ADMIN_PASSWORD`.
3. Cambiar la contraseña.
4. Crear los usuarios (ver [sección 11](#11-guía-del-administrador-de-la-aplicación)).

### Paso 7. HTTPS (TI, recomendado)

Sin HTTPS, las contraseñas viajan sin cifrar por la red. La forma más sencilla es **Caddy**, que obtiene y renueva el certificado automáticamente (requiere un dominio que apunte al servidor y los puertos 80 y 443 abiertos).

1. Instalar Caddy: https://caddyserver.com/docs/install
2. En `/etc/caddy/Caddyfile`:
   ```
   proyectos.empresa.com {
       reverse_proxy 127.0.0.1:8080
   }
   ```
3. `sudo systemctl reload caddy`
4. En `.env`: `APP_PUBLISH=127.0.0.1:8080`, `COOKIE_SECURE=true`, `TRUST_PROXY_HEADERS=true`.
5. `docker compose up -d`

Con Nginx es equivalente: un `server` con certificado que haga `proxy_pass http://127.0.0.1:8080;` y pase las cabeceras `X-Forwarded-For` y `X-Forwarded-Proto`.

---

## 7. Instalación local para probar (Mac, Linux o Windows)

### Opción A: modo demo (2 minutos, sin GitHub ni base de datos)

Requiere Go 1.24+.

```bash
go mod tidy
GITHUB_MODE=demo STORE=memory COOKIE_SECURE=false \
BOOTSTRAP_ADMIN_EMAIL=admin@empresa.com BOOTSTRAP_ADMIN_PASSWORD=CambiarEsta2026 \
go run ./cmd/server
```
(o `make demo`). En Windows PowerShell:
```powershell
$env:GITHUB_MODE="demo"; $env:STORE="memory"; $env:COOKIE_SECURE="false"
$env:BOOTSTRAP_ADMIN_EMAIL="admin@empresa.com"; $env:BOOTSTRAP_ADMIN_PASSWORD="CambiarEsta2026"
go run ./cmd/server
```

Abrir http://localhost:8080 e ingresar con `admin@empresa.com` / `CambiarEsta2026`. Se ven 3 proyectos ficticios de 2 organizaciones. Con `STORE=memory` los usuarios se borran al detener el servidor.

### Opción B: GitHub real + MongoDB en el equipo (ejemplo en Mac)

1. Instalar Homebrew (si no está): https://brew.sh
2. Instalar Go y MongoDB:
   ```bash
   brew install go
   brew tap mongodb/brew
   brew install mongodb-community
   brew services start mongodb-community
   ```
   MongoDB queda corriendo en segundo plano (solo accesible desde el equipo, sin contraseña) y arranca solo al iniciar sesión.
3. En la carpeta del proyecto:
   ```bash
   go mod tidy
   go test ./...                 # todo debe decir "ok"
   make test-mongo               # prueba contra MongoDB real: debe decir "PASS"
   cp .env.example .env
   chmod 600 .env
   ```
4. En `.env`: `GITHUB_TOKEN`, `GITHUB_OWNER_TYPE=auto` (y `GITHUB_OWNER=tu-usuario` si quieres ver también tus Projects personales), `STORE=mongo`, `MONGO_URI=mongodb://localhost:27017`, `COOKIE_SECURE=false`, `APP_ADDR=127.0.0.1:8080`, y el administrador inicial.
5. `go run ./cmd/server check` y luego `go run ./cmd/server`.
6. Abrir http://localhost:8080.

En Linux, MongoDB se instala según https://www.mongodb.com/docs/manual/administration/install-on-linux/. En Windows, con el instalador de https://www.mongodb.com/try/download/community.

### Opción C: Docker en el equipo

Igual que en producción ([sección 6](#6-instalación-en-un-servidor-producción)) con Docker Desktop, usando `APP_PUBLISH=127.0.0.1:8080` y `COOKIE_SECURE=false`. Abrir http://localhost:8080.

---

## 8. Configuración: referencia completa del archivo `.env`

La aplicación lee `.env` automáticamente al iniciar (y con Docker Compose, Compose lo pasa al contenedor). Las variables de entorno del sistema tienen prioridad sobre `.env`. **No hay que configurar nada en otro lugar**: los usuarios se gestionan desde la aplicación y los proyectos viven en GitHub.

### GitHub

| Variable | Por defecto | Descripción |
|---|---|---|
| `GITHUB_MODE` | `api` | `api` = GitHub real. `demo` = datos ficticios, sin token. |
| `GITHUB_TOKEN` | — | **Obligatorio en modo api.** Token de solo lectura (ver [paso 1](#paso-1-generar-el-token-de-github-dueño-de-github)). |
| `GITHUB_OWNER_TYPE` | `auto` en `.env.example` y Docker | `auto` = todas las organizaciones a las que pertenece la cuenta del token (se revisan cada 5 minutos). `org` / `user` = solo los dueños listados en `GITHUB_OWNER`. |
| `GITHUB_OWNER` | vacío | Organizaciones o usuarios separados por coma, tal como aparecen en la URL (`github.com/orgs/ESTE/...`). Con `auto` es opcional y **agrega** dueños extra (por ejemplo, un usuario personal). Con `org`/`user` es obligatorio y son los **únicos** que se muestran. |
| `GITHUB_API_URL` | `https://api.github.com/graphql` | Solo cambia con GitHub Enterprise Server. Debe ser `https`. |
| `CACHE_TTL` | `2m` | Cuánto tiempo se reutiliza la información antes de volver a consultar GitHub (`30s`, `5m`, `1h`…). |

### Nombres de los campos del Project

La aplicación reconoce cinco datos por el **nombre** del campo en GitHub. Cada variable acepta varios nombres separados por coma (sin distinguir mayúsculas).

| Variable | Por defecto | Para qué se usa |
|---|---|---|
| `FIELD_STATUS` | `Status,Estado` | Columnas del tablero, filtro y resumen por estado. |
| `FIELD_PRIORITY` | `Priority,Prioridad` | Filtro y resumen por prioridad. |
| `FIELD_ESTIMATE` | `Estimate,Estimación,Estimacion,Size` | Estimación y "estimación completada". |
| `FIELD_START_DATE` | `Start date,Start Date,Fecha de inicio,Fecha inicio` | Fecha de inicio. |
| `FIELD_DUE_DATE` | `Target date,Due date,End date,Fecha de entrega,Fecha entrega` | Fecha de entrega, vencidas y por vencer. |
| `DONE_STATUSES` | `Done,Hecho,Terminado,Completado,Finalizado` | Valores de estado que significan "terminada". |

Si alguien renombra un campo en GitHub (p. ej. "Status" → "Etapa"), hay que agregar el nombre nuevo (`FIELD_STATUS=Status,Estado,Etapa`). El comando `check` avisa cuando un dato no se reconoce.

### Base de datos

| Variable | Por defecto | Descripción |
|---|---|---|
| `STORE` | `mongo` | `mongo` = MongoDB. `memory` = en memoria (se borra al reiniciar; solo pruebas). Con Docker Compose siempre es `mongo`. |
| `MONGO_URI` | — | Conexión a MongoDB fuera de Docker (p. ej. `mongodb://localhost:27017`). Con Docker Compose se arma sola. |
| `MONGO_DB` | `gh_projects_viewer` | Nombre de la base de datos. |
| `MONGO_ROOT_USER` | `root` | Solo Docker: usuario administrador de MongoDB. La aplicación no lo usa. |
| `MONGO_ROOT_PASSWORD` | — | Solo Docker: **obligatorio**, inventar. Sin `@ : / ? #`. |
| `MONGO_APP_USER` | `gpv_app` | Solo Docker: usuario de la aplicación, con permisos solo sobre su base. |
| `MONGO_APP_PASSWORD` | — | Solo Docker: **obligatorio**, inventar. Sin `@ : / ? #`. |

### Servidor y seguridad

| Variable | Por defecto | Descripción |
|---|---|---|
| `APP_ADDR` | `:8080` | Dirección de escucha fuera de Docker. `127.0.0.1:8080` = solo el mismo equipo. |
| `APP_PUBLISH` | `127.0.0.1:8080` | Solo Docker: dónde se publica la aplicación. `8080` = toda la red. |
| `COOKIE_SECURE` | `true` (`false` en `.env.example`) | `true` con HTTPS. `false` solo para `http://` (pruebas o red interna). |
| `TRUST_PROXY_HEADERS` | `false` | `true` solo si hay un proxy de confianza (Caddy/Nginx) delante; así se registra la IP real del usuario. |
| `SESSION_IDLE_TIMEOUT` | `30m` | La sesión se cierra tras este tiempo sin actividad. |
| `SESSION_MAX_LIFETIME` | `8h` | Duración máxima de una sesión, aunque haya actividad. |
| `BOOTSTRAP_ADMIN_EMAIL` | — | Correo del primer administrador. Solo se usa si la base de datos no tiene usuarios. |
| `BOOTSTRAP_ADMIN_PASSWORD` | — | Su contraseña temporal (12–128 caracteres, letras y números). Solo se usa esa primera vez. |
| `LOG_FORMAT` | texto | `json` para registros en formato JSON (Docker lo usa por defecto). |

Después de cambiar `.env`: con Docker, `docker compose up -d`; sin Docker, detener (Ctrl+C) e iniciar de nuevo.

---

## 9. Preparar los Projects en GitHub

La aplicación muestra lo que haya en los Projects. Para que el resumen y los filtros sean útiles:

### Campos recomendados

| Campo | Tipo en GitHub | Valores sugeridos |
|---|---|---|
| **Status** | Single select (ya existe) | Todo, In Progress, Done (se pueden agregar más, p. ej. Ready, In Review) |
| **Priority** | Single select | P0, P1, P2 (o Alta, Media, Baja) |
| **Estimate** | Number | Puntos u horas |
| **Start date** | Date | — |
| **Target date** | Date | — |

Para crearlos: en el Project, vista de tabla → botón **+** al final de las columnas, o **⋯ → Settings → Custom fields**.

### Cómo se comportan los cambios en GitHub

| Cambio en GitHub | Efecto en la aplicación |
|---|---|
| Agregar una opción de estado (p. ej. "Ready" antes de "Todo") | Aparece sola como columna del tablero, en el mismo orden. Cuenta como tarea abierta. |
| Agregar un estado **final** (p. ej. "Entregado") | Aparece solo, pero hay que agregarlo a `DONE_STATUSES`; si no, se cuenta como abierta y puede salir como vencida. |
| Crear un campo nuevo (texto, número, fecha, selección, iteración) | Aparece solo en el detalle de cada tarea y como columna del CSV. |
| Renombrar Status, Priority, Estimate o las fechas | Hay que agregar el nombre nuevo en `FIELD_*`. |
| Agregar issues de otro repositorio al Project | Aparecen solos; se ve el repositorio de cada tarea. |
| Crear un Project nuevo en la organización | Aparece solo en la lista. |
| La cuenta del token entra a una organización nueva | Sus Projects aparecen en unos 5 minutos (modo `auto`). |
| Cerrar un Project | Deja de verse salvo que se marque "Mostrar cerrados". |

### Agregar tareas automáticamente

Para que cada issue nuevo de un repositorio entre solo al Project: en el Project, **⋯ → Workflows → Auto-add to project** → elegir el repositorio y un filtro (p. ej. `is:issue`) → activar.

### Repositorios privados y Projects privados

- Si una tarea es de un repositorio al que el token no tiene acceso, aparece como **"(Elemento sin acceso)"**.
- Los Projects privados de la organización solo se ven si la cuenta del token tiene permiso de lectura sobre ellos.

---

## 10. Guía del usuario (rol Consulta)

### 10.1 Ingresar

1. Abrir la dirección que les compartió el administrador.
2. Escribir correo y contraseña.
3. **Primer ingreso:** la aplicación pide cambiar la contraseña temporal. La nueva debe tener **entre 12 y 128 caracteres, con letras y números**. Hasta que no se cambie, no se pueden ver datos.

Datos importantes:
- Si se escribe mal la contraseña **5 veces seguidas**, la cuenta se bloquea **15 minutos**.
- La sesión se cierra sola tras **30 minutos sin actividad** o **8 horas** en total.
- Para salir: menú superior → **Salir**.
- Para cambiar la contraseña en cualquier momento: menú superior → **Cambiar contraseña**. Al cambiarla se cierran las demás sesiones abiertas de ese usuario.
- ¿Olvidó la contraseña? Pedirle al administrador que la restablezca (la aplicación no envía correos).

### 10.2 Lista de proyectos

- Los proyectos aparecen **agrupados por organización**, con el nombre de la organización como título.
- **Buscar proyecto:** filtra por nombre o descripción.
- **Todas las organizaciones:** filtra por una organización (solo aparece si hay más de una).
- **Mostrar cerrados:** incluye proyectos cerrados en GitHub (aparecen atenuados con la etiqueta *Cerrado*).
- Cada tarjeta muestra número, nombre, descripción y fecha de última actualización. Clic para abrirlo.

### 10.3 Vista de un proyecto

Arriba se ve **la organización**, el nombre del proyecto, su descripción y la hora en que se consultaron los datos. Botones:
- **Actualizar desde GitHub:** trae los datos al instante, sin esperar la caché de 2 minutos (máximo 6 veces por minuto por usuario).
- **Ver en GitHub:** abre el proyecto en GitHub (solo funciona para quien tenga cuenta y acceso).

### 10.4 Resumen

| Indicador | Cómo se calcula |
|---|---|
| **Tareas** | Total de tareas del proyecto. |
| **Completadas** | Tareas cuyo estado está en `DONE_STATUSES` (Done, Hecho, Terminado…). Si una tarea no tiene estado, cuenta como completada si el issue está cerrado o el pull request fusionado. |
| **Abiertas** | Total menos completadas. |
| **Vencidas** (rojo) | Abiertas con fecha de entrega **anterior a hoy**. |
| **Vencen en 7 días** (amarillo) | Abiertas con fecha de entrega entre hoy y dentro de 7 días. |
| **Sin responsable** | Tareas sin ninguna persona asignada. |
| **Avance** | Completadas ÷ total, en porcentaje (redondeado hacia abajo). |
| **Estimación completada** | Suma de la estimación de las tareas completadas frente al total (solo si hay estimaciones). |
| **Por estado / responsable / prioridad** | Cantidad de tareas en cada valor, con barras. Una tarea con dos responsables cuenta para ambos. |

"Hoy" es la fecha del servidor (su zona horaria).

### 10.5 Tablero y Tabla

- **Tablero:** una columna por estado, en el orden de GitHub, más "Sin estado" si hay tareas sin estado. Cada tarjeta muestra título, número, prioridad, responsables y fecha de entrega. Las **vencidas tienen borde rojo**.
- **Tabla:** clic en el encabezado de una columna para ordenar; otro clic invierte el orden. Las celdas vacías van al final.

### 10.6 Filtros

| Filtro | Qué hace |
|---|---|
| **Buscar** | Texto libre en título, **descripción**, número (`#12`), repositorio, etiquetas y responsables. |
| **Estado** | Un estado específico. |
| **Prioridad** | Una prioridad, o "Sin prioridad". |
| **Responsable** | Una persona, o "Sin responsable". |
| **Solo vencidas** | Solo las tareas vencidas. |
| **Limpiar filtros** | Quita todos los filtros. |

Los filtros se combinan entre sí y se aplican al tablero, a la tabla y al CSV. El contador muestra cuántas tareas coinciden.

### 10.7 Detalle de una tarea

Clic en una tarea (tarjeta del tablero o título en la tabla). Se ve:

- **Título y descripción completa** (como se escribió en GitHub; el formato Markdown se ve como texto).
- Tipo (Issue, Pull request, Borrador o Restringido), repositorio y número.
- Estado en el proyecto y estado en GitHub (abierto, cerrado, fusionado).
- Prioridad, estimación, fecha de inicio, fecha de entrega, iteración y última actualización.
- Responsables y etiquetas.
- **Todos los demás campos del proyecto**, en el orden de GitHub, con "—" si no tienen valor: Milestone, Issue padre, Progreso de sub-issues, Pull requests vinculados, Revisores, Creado, Cerrado y cualquier campo personalizado.
- **Abrir en GitHub** (para quien tenga acceso).

Cerrar con la **X**, clic fuera o tecla **Esc**.

### 10.8 Exportar a CSV

1. Aplicar los filtros que se quieran (o ninguno para exportar todo el proyecto).
2. **Exportar CSV**.
3. Se descarga `organizacion-proyecto-AAAA-MM-DD.csv`.

Contenido: **solo las tareas que coinciden con los filtros**, con las columnas Organización, Proyecto, Título, Descripción, Tipo, Repositorio, Número, Estado, Prioridad, Responsables, Estimación, Inicio, Entrega, Etiquetas, URL y **una columna por cada campo adicional del proyecto** (Milestone, campos personalizados, etc.).

- Se abre directamente en Excel, Numbers o Google Sheets, con tildes y eñes correctas (UTF-8).
- Las filas salen en el orden de GitHub, no en el orden elegido en la tabla.
- Por seguridad, celdas que empiezan con `=`, `+`, `-` o `@` llevan un apóstrofo delante para que Excel no las interprete como fórmulas.

---

## 11. Guía del administrador de la aplicación

El administrador puede hacer todo lo del usuario de consulta, y además gestionar usuarios y revisar la bitácora en el menú **Administración**.

### 11.1 Permisos por rol

| Acción | Consulta | Administrador |
|---|:---:|:---:|
| Ver proyectos, resumen, tablero, tabla y detalle | ✅ | ✅ |
| Filtrar y exportar CSV | ✅ | ✅ |
| Actualizar desde GitHub | ✅ | ✅ |
| Cambiar su propia contraseña | ✅ | ✅ |
| Ver el menú Administración | ❌ | ✅ |
| Crear usuarios, cambiar roles, activar/desactivar, restablecer contraseñas | ❌ | ✅ |
| Ver la bitácora de auditoría | ❌ | ✅ |

Los permisos se verifican en el servidor: aunque un usuario de consulta escriba la dirección de administración, recibe "acceso denegado".

### 11.2 Crear un usuario

1. **Administración → Crear usuario**.
2. Completar **Nombre**, **Correo** y **Rol** (*Consulta* o *Administrador*).
3. **Contraseña temporal:** escribir una o pulsar **Generar** para crear una aleatoria. Debe cumplir la política (12+ caracteres, letras y números).
4. **Crear**.
5. Entregar el correo y la contraseña temporal a la persona **por un canal seguro** (en persona, mensaje directo, gestor de contraseñas). En su primer ingreso deberá cambiarla.

El correo no se puede repetir. Los correos se guardan en minúsculas.

### 11.3 Gestionar usuarios

La tabla **Usuarios** muestra nombre, correo, rol, estado (*Activo*, *Activo (clave temporal)* o *Inactivo*) y último ingreso. Acciones por usuario:

| Botón | Qué hace |
|---|---|
| **Hacer admin / Hacer consulta** | Cambia el rol. **Cierra las sesiones abiertas** de ese usuario para que el cambio aplique de inmediato. |
| **Desactivar / Activar** | Un usuario inactivo no puede ingresar. Al desactivarlo, **su sesión se cierra al instante**. No se borra (queda en la bitácora). |
| **Restablecer clave** | Genera una contraseña temporal nueva y la muestra **una vez** para copiarla. El usuario deberá cambiarla al ingresar. |

Protecciones: un administrador **no puede quitarse su propio rol ni desactivarse** (para que nunca quede la aplicación sin administradores). Para eso, otro administrador debe hacerlo.

Recomendaciones:
- Tener **al menos dos administradores**, por si uno olvida su contraseña o deja la empresa.
- Desactivar a quien deje la empresa el mismo día.
- Revisar cada cierto tiempo los usuarios que nunca han ingresado.

### 11.4 Bitácora de auditoría

**Administración → Bitácora de auditoría** muestra los últimos 100 eventos: fecha, acción, usuario, detalle, IP y resultado. Se conserva **180 días** y luego se borra automáticamente.

| Acción | Cuándo se registra |
|---|---|
| **Ingreso** | Inicio de sesión correcto. |
| **Ingreso fallido** | Contraseña incorrecta, usuario inexistente o inactivo. |
| **Ingreso bloqueado** | Intento durante un bloqueo por demasiados fallos. |
| **Salida** | Cierre de sesión. |
| **Cambio de contraseña** | Un usuario cambió su contraseña. |
| **Creación de usuario** | Un administrador creó un usuario. |
| **Modificación de usuario** | Cambio de rol, activación/desactivación o restablecimiento de contraseña. |

### 11.5 Si el administrador olvida su contraseña

- Si hay **otro administrador**: que use **Restablecer clave**.
- Si **no hay otro**: TI ejecuta en el servidor
  ```bash
  docker compose run --rm -e ADMIN_EMAIL=admin@empresa.com -e ADMIN_PASSWORD='NuevaTemporal2026' app create-admin
  ```
  (sin Docker: `ADMIN_EMAIL=... ADMIN_PASSWORD=... go run ./cmd/server create-admin`). Si el correo existe, le pone esa contraseña temporal, lo reactiva y lo deja como administrador; si no existe, lo crea.

Cambiar `BOOTSTRAP_ADMIN_PASSWORD` en `.env` **no** sirve para esto: solo se usa cuando la base de datos está vacía.

---

## 12. Guía de TI: operación y mantenimiento

### 12.1 Tareas periódicas

| Tarea | Frecuencia | Cómo |
|---|---|---|
| **Renovar el token de GitHub** | Antes de que venza (p. ej. cada 90 días) | El dueño genera uno nuevo ([paso 1](#paso-1-generar-el-token-de-github-dueño-de-github)) → pegarlo en `GITHUB_TOKEN` → `docker compose up -d` → `docker compose run --rm app check`. Revocar el anterior en GitHub. |
| **Respaldo de la base de datos** | Diario (programado) | Ver 12.3. |
| **Actualizar la aplicación** | Cuando haya cambios | `git pull && docker compose up -d --build` |
| **Revisar la bitácora y los usuarios** | Mensual | Desde Administración. |
| **Actualizar el servidor y Docker** | Mensual | Según la política de TI. |

### 12.2 Agregar o quitar organizaciones

- **Agregar:** el dueño de la organización invita a la cuenta del token (organización → **People** → **Invite member**); la cuenta acepta la invitación; si la organización usa SSO, se autoriza el token. Sus Projects aparecen solos en unos 5 minutos. **No hay que tocar el servidor.**
- **Quitar:** sacar a la cuenta del token de la organización, o pasar a `GITHUB_OWNER_TYPE=org` con la lista explícita de organizaciones en `GITHUB_OWNER`.

### 12.3 Respaldos y restauración

Respaldo (genera un archivo con usuarios, sesiones y bitácora):
```bash
docker compose exec -T mongo sh -c 'mongodump --archive -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD"' > respaldo-$(date +%F).archive
```

Ejemplo de respaldo diario con cron (Linux, a las 2:00 a. m., conservando 30 días):
```cron
0 2 * * * cd /ruta/gh-projects-viewer && docker compose exec -T mongo sh -c 'mongodump --archive -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD"' > /respaldos/gpv-$(date +\%F).archive && find /respaldos -name 'gpv-*.archive' -mtime +30 -delete
```

Restauración (reemplaza los datos actuales):
```bash
docker compose exec -T mongo sh -c 'mongorestore --archive --drop -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD"' < respaldo-AAAA-MM-DD.archive
```

Guardar los respaldos fuera del servidor. Si se pierde la base sin respaldo, solo hay que volver a crear los usuarios: los proyectos siguen en GitHub.

### 12.4 Comandos de operación

| Necesidad | Comando |
|---|---|
| Estado de los contenedores | `docker compose ps` |
| Ver registros en vivo | `docker compose logs -f app` |
| Diagnóstico | `docker compose run --rm app check` |
| Reiniciar la aplicación | `docker compose restart app` |
| Aplicar cambios de `.env` | `docker compose up -d` |
| Detener todo (conserva datos) | `docker compose down` |
| Iniciar todo | `docker compose up -d` |
| Consola de MongoDB | `docker compose exec mongo mongosh -u root -p` |

> ⚠️ `docker compose down -v` **borra la base de datos** (usuarios y bitácora). No usarlo salvo que se quiera empezar de cero.

### 12.5 Monitoreo

- `GET /healthz` responde `200` si el servidor está funcionando. Sirve para Uptime Kuma, Zabbix, etc.
- Docker revisa la salud de la aplicación cada 30 segundos (comando interno `healthcheck`) y de MongoDB cada 10 segundos.
- Los registros (en JSON con Docker) incluyen cada petición (método, ruta, código, tiempo, IP), sin contraseñas ni contenidos. Los errores de GitHub se registran sin el token.

### 12.6 Cambiar la cuenta del token

Si cambia la persona o la cuenta de servicio: generar el token con la nueva cuenta (debe ser miembro de las organizaciones), reemplazar `GITHUB_TOKEN`, `docker compose up -d` y `check`. Los usuarios de la aplicación no se ven afectados.

---

## 13. Guía técnica para desarrolladores

### 13.1 Tecnologías

| Capa | Tecnología |
|---|---|
| Backend | Go 1.24 (biblioteca estándar + driver oficial de MongoDB v2) |
| Frontend | HTML5, CSS3 y JavaScript sin frameworks, **embebido en el binario** |
| Base de datos | MongoDB 7 |
| Integración | API GraphQL de GitHub (ProjectsV2), solo consultas |
| Despliegue | Docker multi-etapa (imagen distroless, sin shell, usuario no root) + Docker Compose |

### 13.2 Estructura del código

```
cmd/server/
  main.go              Arranque: configuración, store, proveedor GitHub, caché, servidor HTTP, admin inicial
  commands.go          Comandos check, create-admin y healthcheck
internal/config/       Lectura de variables de entorno y .env, validación
internal/github/
  client.go            Cliente GraphQL: organizaciones, proyectos, tareas (solo consultas)
  mapper.go            Convierte la respuesta de GitHub en Task (campos, roles de campos)
  models.go            Project, Task, ProjectDetail, Provider, FieldMapping
  cache.go             Caché en memoria con TTL (CachedProvider)
  demo.go              Proveedor de datos ficticios (modo demo)
internal/auth/         Hash de contraseñas (PBKDF2), política, tokens, limitador de intentos
internal/store/        Persistencia: interfaz Store, MongoDB y memoria
internal/httpapi/
  server.go            Rutas, archivos estáticos, auditoría
  middleware.go        Cabeceras de seguridad, sesión, CSRF, origen, roles, logs
  handlers_auth.go     Login, logout, me, cambio de contraseña
  handlers_projects.go Lista, detalle (con resumen) y refresco de proyectos
  handlers_admin.go    Usuarios y bitácora
web/public/            login.html, index.html, static/css/app.css, static/js/{api,app,login}.js
deploy/mongo-init.js   Crea el usuario de MongoDB de la aplicación (mínimo privilegio)
Dockerfile, docker-compose.yml, Makefile, .env.example, SEGURIDAD.md
```

### 13.3 Flujo de datos

1. `GET /api/projects` → `CachedProvider.ListProjects` → `Client.Owners` (organizaciones del token en modo `auto`, en caché 5 min) → `Client.ListOwnerProjects` por cada dueño, con la consulta `repositoryOwner(login)` (sirve para organizaciones y usuarios). Si un dueño falla, se registra y se omite; solo hay error si fallan todos.
2. `GET /api/projects/{id}` → valida el formato del ID y que esté en la lista permitida → `Client.GetProject` (pagina de 100 en 100 hasta 2000 tareas; verifica que el dueño del proyecto esté entre los permitidos) → `mapItem` por tarea → `summarize` calcula el resumen → JSON con `project`, `tasks`, `statusOptions`, `priorityOptions`, `fieldNames`, `fieldRoles`, `summary`, `doneStatuses`.
3. El frontend (`app.js`) dibuja resumen, tablero, tabla, filtros y detalle a partir de ese JSON; el CSV se genera en el navegador.

**Cómo se leen los campos (`mapper.go`):**
- Datos del issue/PR/borrador: título, descripción (`body`), número, URL, repositorio, estado, responsables, etiquetas, fechas de creación y cierre, issue padre y resumen de sub-issues.
- Valores de campos del Project por tipo: selección única, texto, número, fecha, iteración, milestone, pull requests vinculados y revisores. Todos quedan en `Task.Fields[nombreDelCampo]`.
- Los campos cuyo nombre coincide con `FIELD_*` se copian además a `Status`, `Priority`, `Estimate`, `StartDate`, `DueDate`; los de tipo iteración a `Iteration`. `ProjectDetail.FieldRoles` indica qué campo cumple cada rol, para que el frontend no los repita.
- El campo interno `Title` se omite (repite el título).

### 13.4 API

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| GET | `/healthz` | público | Estado del servicio |
| POST | `/api/auth/login` | público | Iniciar sesión (`{"email","password"}`) |
| POST | `/api/auth/logout` | sesión | Cerrar sesión |
| GET | `/api/auth/me` | sesión | Usuario actual y token CSRF |
| POST | `/api/auth/password` | sesión | Cambiar contraseña |
| GET | `/api/projects` | sesión | Lista de proyectos (con `owner` y `ownerName`) |
| GET | `/api/projects/{id}` | sesión | Proyecto, tareas, campos y resumen |
| POST | `/api/projects/{id}/refresh` | sesión | Vaciar la caché del proyecto (6/min por usuario) |
| GET / POST | `/api/admin/users` | admin | Listar / crear usuarios |
| PATCH | `/api/admin/users/{id}` | admin | Cambiar rol, estado o contraseña temporal |
| GET | `/api/admin/audit` | admin | Últimos eventos de la bitácora |

Las peticiones POST/PATCH requieren la cabecera `X-CSRF-Token` (la entregan el login y `/api/auth/me`) y un `Origin` del mismo sitio. Mientras el usuario tenga contraseña temporal, solo puede usar `me`, `password` y `logout`.

Ejemplo con curl:
```bash
curl -c cookies.txt -H "Content-Type: application/json" \
  -d '{"email":"admin@empresa.com","password":"SuClave2026xx"}' http://localhost:8080/api/auth/login
curl -b cookies.txt http://localhost:8080/api/projects
```

### 13.5 Pruebas

```bash
go vet ./...
go test ./...                                   # unitarias e integración con GitHub simulado
go test -cover ./...                            # cobertura
make test-mongo                                 # capa de datos contra MongoDB real local
```

| Paquete | Qué verifica |
|---|---|
| `config` | Lectura de `.env` y validación |
| `store` | Usuarios, sesiones y auditoría en memoria y, con `MONGO_TEST_URI`, en MongoDB real |
| `auth` | Hash, política de contraseñas, tokens, bloqueo por intentos |
| `github` | Mapeo de todos los tipos de campo, descripción, milestone, issue padre, sub-issues, borradores y elementos restringidos, errores parciales, 401/403/SSO/NOT_FOUND/500, paginación, varias organizaciones y modo `auto` (una organización con error no tumba a las demás), rechazo de proyectos de dueños ajenos, caché, y que **nunca se envíen mutaciones** |
| `httpapi` | Acceso sin sesión, login, cookies, cambio obligatorio de contraseña, CSRF, logout, roles, fuerza bruta, cabeceras de seguridad, origen, path traversal, campos desconocidos, expiración, gestión de usuarios y auditoría |

El build de Docker ejecuta `go test ./...`: si una prueba falla, no se genera la imagen.

### 13.6 Cómo extender

| Quiero… | Dónde |
|---|---|
| Leer un dato nuevo de GitHub | Agregarlo a `projectItemsQuery` (`client.go`), a `rawContent`/`rawFieldValue` y a `mapItem` (`mapper.go`); probarlo en `client_test.go`. Validar la consulta antes en el explorador GraphQL de GitHub o con `gh api graphql`. |
| Un indicador nuevo en el resumen | `Summary` y `summarize` en `handlers_projects.go`; dibujarlo en `summaryView` (`app.js`). |
| Una columna nueva en la tabla o el CSV | `columns` y `exportCSV` en `app.js`. |
| Un rol nuevo | `store.go` (constantes), middleware de roles y pantalla de Administración. |
| Otra fuente de datos | Implementar la interfaz `github.Provider` (`ListProjects`, `GetProject`). |

Convenciones: comentarios y mensajes en español; solo biblioteca estándar salvo el driver de MongoDB; el frontend nunca usa `innerHTML` (todo con `textContent` mediante el helper `h()`); toda consulta a GitHub es de lectura.

### 13.7 Desarrollo local rápido

```bash
make demo        # modo demo en http://localhost:8080
make test        # vet + pruebas
make check       # diagnóstico con .env
make run         # con la configuración de .env
make build       # binario en bin/server
```

---

## 14. Base de datos

### Qué guarda

| Colección | Contenido | Retención |
|---|---|---|
| `users` | Correo, nombre, rol, activo, si debe cambiar la contraseña, fechas de creación/actualización/último ingreso y el **hash** de la contraseña (PBKDF2, nunca la contraseña en texto). | Permanente |
| `sessions` | Sesiones abiertas: usuario, IP, navegador, fechas. El identificador se guarda como **hash** SHA-256 (un volcado de la base no permite suplantar sesiones). | Se borran solas al vencer (índice TTL) |
| `audit_log` | Eventos de la bitácora (sección 11.4). | 180 días (índice TTL) |

**No guarda** proyectos, tareas, descripciones ni el token de GitHub.

### Dónde vive

- **Docker (producción):** contenedor `mongo`, datos en el volumen `mongo-data`. No se expone fuera de la red interna de Docker; solo la aplicación lo alcanza, con un usuario que solo tiene permisos sobre su base.
- **Instalación local con Homebrew:** `/opt/homebrew/var/mongodb` (Mac con chip Apple), escuchando solo en `localhost:27017`.

### ¿Hay que configurarla?

No. Con `docker compose up`: se crea la base, el usuario de la aplicación (`deploy/mongo-init.js`, solo la primera vez), las colecciones y los índices (`ensureIndexes`), y el primer administrador. Solo hay que inventar `MONGO_ROOT_PASSWORD` y `MONGO_APP_PASSWORD`.

Consultas útiles (en `mongosh`):
```javascript
use gh_projects_viewer
db.users.find({}, {password_hash: 0})          // usuarios, sin el hash
db.sessions.countDocuments()                   // sesiones activas
db.audit_log.find().sort({at: -1}).limit(10)   // últimos eventos
```

---

## 15. Seguridad

Resumen; el detalle está en [SEGURIDAD.md](SEGURIDAD.md).

- **Token de GitHub:** solo en el servidor (`.env`, excluido de Git); nunca llega al navegador; integración de solo lectura (una prueba verifica que no existan mutaciones); solo se muestran proyectos de los dueños permitidos.
- **Contraseñas:** PBKDF2-HMAC-SHA256 con 600.000 iteraciones y sal aleatoria; política de 12–128 caracteres con letras y números; cambio obligatorio de contraseñas temporales.
- **Fuerza bruta:** 5 fallos por cuenta o 10 por IP en 15 minutos → bloqueo temporal; mensajes genéricos que no revelan si el correo existe.
- **Sesiones:** token aleatorio de 256 bits, cookie `HttpOnly` + `SameSite=Strict` + `Secure` (con HTTPS); expiración por inactividad y absoluta; logout, cambio de contraseña, desactivación y cambio de rol revocan sesiones.
- **Web:** protección contra XSS (sin `innerHTML`, Content-Security-Policy estricta), CSRF (token + `SameSite` + verificación de origen), clickjacking, inyección NoSQL/GraphQL, inyección de fórmulas en CSV y path traversal; límites de tamaño y tiempo en peticiones.
- **Contenedores:** imagen distroless sin shell, usuario no root, sistema de archivos de solo lectura, sin privilegios; MongoDB no expuesto.
- **Trazabilidad:** bitácora de 180 días; registros sin contraseñas ni contenidos.

Responsabilidades de operación: usar HTTPS, proteger `.env`, rotar el token, hacer respaldos, mantener actualizado el servidor y revisar la bitácora.

---

## 16. Límites conocidos y posibles mejoras

### Límites actuales

| Límite | Valor |
|---|---|
| Proyectos por organización | 100 |
| Tareas por proyecto | 2000 (se avisa si hay más) |
| Campos por proyecto | 50 |
| Responsables y etiquetas por tarea | 10 |
| Pull requests vinculados y revisores por tarea | 10 |
| Organizaciones detectadas en modo `auto` | 100 |
| Frecuencia de datos | Caché de 2 minutos (configurable) |
| Token | Uno solo para todas las organizaciones. Si una organización solo admite tokens *fine-grained* (que valen para una sola organización), hoy no se puede combinar con otras. |
| Límite de la API de GitHub | 5.000 puntos por hora por token; con la caché, sobra para el uso normal. |

### Posibles mejoras

- Permisos por organización (que cada usuario vea solo ciertas organizaciones).
- Varios tokens (uno por organización) para organizaciones que exigen tokens *fine-grained*.
- Usar una **GitHub App** en lugar de un token personal.
- Inicio de sesión con Google/Microsoft de la empresa.
- Recuperación de contraseña por correo.
- Mostrar la descripción con formato (Markdown).
- Exportar en el orden de la tabla; exportar varios proyectos a la vez; exportar a Excel (.xlsx).
- Filtros por campos personalizados y por milestone.
- Integración continua (GitHub Actions con `go vet`, `go test` y `govulncheck`).

---

## 17. Solución de problemas

Primero, siempre: `docker compose run --rm app check` (o `go run ./cmd/server check`). Indica la causa de la mayoría de los problemas.

### Ingreso y sesión

| Síntoma | Causa y solución |
|---|---|
| El login es correcto pero vuelve a la pantalla de ingreso | `COOKIE_SECURE=true` sin HTTPS. Usar `false` o configurar HTTPS. |
| "Correo o contraseña incorrectos" | Datos mal escritos, usuario inactivo o inexistente. El administrador puede revisar el estado y restablecer la clave. |
| "Demasiados intentos" | 5 fallos seguidos: esperar 15 minutos o pedir al administrador que restablezca la clave. |
| Nadie puede ingresar (base nueva) | Falta `BOOTSTRAP_ADMIN_EMAIL` cuando la base estaba vacía. Usar `create-admin` (sección 11.5). |
| Se cierra la sesión sola | Pasaron 30 min sin actividad u 8 horas. Ajustable con `SESSION_IDLE_TIMEOUT` y `SESSION_MAX_LIFETIME`. |

### GitHub

| Síntoma | Causa y solución |
|---|---|
| "El token de GitHub no es válido o expiró" | Token mal copiado, vencido o revocado. Generar uno nuevo. |
| "No tiene permisos suficientes" | Falta `read:project`; la organización exige SSO (autorizar el token) o bloquea tokens clásicos (el dueño debe permitirlos). |
| No aparece ninguna organización | La cuenta del token no es miembro de ninguna, o falta `read:org`. |
| Falta una organización | Invitar a la cuenta del token; autorizar SSO. `check` dice cuál falla. |
| No aparece un proyecto | Es personal de un miembro (no de la organización), está cerrado ("Mostrar cerrados") o es privado y la cuenta del token no tiene acceso. |
| No aparece un repositorio | La aplicación muestra Projects, no repositorios: agregar sus issues a un Project. |
| Proyecto sin tareas | Las tareas deben estar agregadas al Project, no solo al repositorio. |
| Tareas "(Elemento sin acceso)" | Repositorio privado sin acceso: agregar `repo` al token o dar acceso a la cuenta. |
| Estado, prioridad o fechas vacíos | El campo tiene otro nombre: ajustar `FIELD_*` (`check` muestra los nombres reales). |
| Un estado final cuenta como abierto o vencido | Agregarlo a `DONE_STATUSES`. |
| "GitHub limitó temporalmente las consultas" | Límite de la API o permisos. Esperar unos minutos; revisar el token. |
| Un cambio en GitHub no se ve | Esperar 2 minutos o pulsar "Actualizar desde GitHub". |

### Servidor

| Síntoma | Causa y solución |
|---|---|
| No se puede abrir desde otros equipos | `APP_PUBLISH=8080` (sin HTTPS) o proxy HTTPS; revisar el firewall. |
| `address already in use` | Otro programa usa el puerto 8080: cambiar `APP_PUBLISH` (Docker) o `APP_ADDR` (sin Docker). |
| "no fue posible conectar con MongoDB" | MongoDB detenido o `MONGO_URI` incorrecta. Con Docker: `docker compose ps`. |
| El build de Docker falla en las pruebas | Hay un cambio de código que rompe algo: revisar la salida de `go test`. |
| `defina MONGO_ROOT_PASSWORD en .env` | Faltan las claves de MongoDB en `.env`. |

---

## 18. Glosario

| Término | Significado |
|---|---|
| **Borrador (draft)** | Tarea que existe solo en el Project, sin issue en un repositorio. |
| **Caché** | Copia temporal en memoria de los datos de GitHub (2 minutos). |
| **CSV** | Archivo de valores separados por comas que abren Excel y similares. |
| **Dueño (owner)** | Organización o usuario al que pertenece un Project. |
| **Issue** | Tarea dentro de un repositorio de GitHub. |
| **Milestone** | Hito o versión del repositorio a la que se asocia un issue. |
| **Pull request (PR)** | Propuesta de cambio de código; puede estar en un Project como tarea. |
| **Restringido** | Tarea cuyo contenido el token no puede ver. |
| **SSO** | Inicio de sesión corporativo único que algunas organizaciones exigen. |
| **Sub-issue / issue padre** | Jerarquía de tareas en GitHub: un issue puede dividirse en sub-issues. |
| **Token** | Llave de acceso de solo lectura a GitHub. |
| **TTL** | Tiempo tras el cual algo se borra o expira automáticamente. |

---

## 19. Referencia rápida de comandos

```bash
# Producción (Docker)
docker compose up -d --build                  # instalar / actualizar
docker compose run --rm app check             # diagnóstico
docker compose logs -f app                    # registros
docker compose restart app                    # reiniciar
docker compose down                           # detener (conserva datos)
docker compose run --rm -e ADMIN_EMAIL=a@b.com -e ADMIN_PASSWORD='Temporal2026xx' app create-admin

# Local (Go)
go run ./cmd/server                           # iniciar con .env
go run ./cmd/server check                     # diagnóstico
make demo                                     # modo demo
make test                                     # pruebas
make test-mongo                               # pruebas contra MongoDB real
```
