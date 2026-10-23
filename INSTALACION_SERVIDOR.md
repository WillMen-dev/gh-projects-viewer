# Instalación en el servidor de la empresa

Guía para dejar la aplicación funcionando en un servidor (Linux, Windows o Mac) con Docker. La aplicación lee los GitHub Projects de **todas las organizaciones a las que pertenece el dueño del token** y los muestra a usuarios que no tienen GitHub (RRHH, administración, gerencia).

Tiempo estimado: 20–30 minutos.

| Quién | Qué hace |
|---|---|
| **Dueño de GitHub** (quien administra la organización) | Genera el token de solo lectura (paso 1) |
| **TI** | Instala y configura el servidor (pasos 2 a 5) |
| **Administrador de la app** | Entra, cambia su contraseña y crea los usuarios (paso 6) |

---

## Paso 1. Token de GitHub (lo hace el dueño de GitHub)

El token es una llave de **solo lectura** con la que la aplicación consulta GitHub. La aplicación nunca crea ni modifica nada.

1. Entrar a https://github.com con la cuenta dueña de la organización.
2. Foto (arriba a la derecha) → **Settings** → abajo a la izquierda **Developer settings** → **Personal access tokens** → **Tokens (classic)** → **Generate new token (classic)**.
3. *Note*: `consulta-proyectos`. *Expiration*: 90 días (anotar la fecha para renovarlo).
4. Marcar **solo** estos permisos:
   - `read:project` (leer los Projects)
   - `read:org` (saber a qué organizaciones pertenece)
   - `repo` (leer las tareas de repositorios privados)
5. **Generate token** y copiar el valor (`ghp_...`). GitHub no lo vuelve a mostrar.
6. Si la organización usa inicio de sesión corporativo (SSO), en la lista de tokens pulsar **Configure SSO** → **Authorize** en cada organización.
7. Entregar el token a TI por un medio seguro (no por correo ni chat).

> **Recomendado:** en lugar de la cuenta personal del dueño, usar una **cuenta de GitHub de servicio** (p. ej. `empresa-reportes`) invitada a cada organización. Así la aplicación no depende de una persona.

**¿Qué organizaciones se verán?** Todas aquellas de las que la cuenta del token sea miembro. Para agregar una organización nueva basta con invitar a esa cuenta (organización → *People* → *Invite member*); sus proyectos aparecen solos en unos minutos.

## Paso 2. Preparar el servidor (TI)

Requisitos: **Docker** con Docker Compose y **Git**.

- Linux (Ubuntu/Debian): https://docs.docker.com/engine/install/
- Windows / Mac: Docker Desktop, https://www.docker.com/products/docker-desktop/

Verificar:
```bash
docker --version
docker compose version
git --version
```

## Paso 3. Descargar la aplicación (TI)

```bash
git clone <URL-del-repositorio> gh-projects-viewer
cd gh-projects-viewer
cp .env.example .env
chmod 600 .env        # solo Linux/Mac: que solo el dueño del archivo pueda leerlo
```

> Si el repositorio es privado, la cuenta que clona necesita acceso de lectura a él.

## Paso 4. Configurar `.env` (TI)

Abrir `.env` con un editor y completar:

```ini
# GitHub
GITHUB_MODE=api
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx      # el token del paso 1
GITHUB_OWNER_TYPE=auto                     # todas las organizaciones del dueño del token
GITHUB_OWNER=                              # vacío (ver nota abajo)

# Primer administrador de la app (deberá cambiar la clave al entrar)
BOOTSTRAP_ADMIN_EMAIL=admin@empresa.com
BOOTSTRAP_ADMIN_PASSWORD=ClaveTemporal2026

# Claves de MongoDB: inventar dos claves largas, SIN los caracteres @ : / ? #
MONGO_ROOT_PASSWORD=una-clave-larga-y-aleatoria-1
MONGO_APP_PASSWORD=una-clave-larga-y-aleatoria-2
```

Para generar claves aleatorias: `openssl rand -hex 24`.

Según cómo se vaya a acceder a la app:

| Escenario | `APP_PUBLISH` | `COOKIE_SECURE` |
|---|---|---|
| **Con HTTPS** (recomendado), un proxy Caddy/Nginx delante (ver paso 7) | `127.0.0.1:8080` | `true` |
| **Solo red interna, sin HTTPS** (`http://IP-del-servidor:8080`) | `8080` | `false` |

> Con `COOKIE_SECURE=true` y sin HTTPS el inicio de sesión no funciona (la sesión no se guarda).

**Notas sobre `GITHUB_OWNER`:**
- Con `GITHUB_OWNER_TYPE=auto` puede quedar vacío. Si se escribe un nombre, se **agrega** a las organizaciones detectadas (por ejemplo el usuario personal del dueño, para ver también sus proyectos personales).
- Para mostrar **solo** algunas organizaciones: `GITHUB_OWNER_TYPE=org` y `GITHUB_OWNER=organizacion-a,organizacion-b`.

## Paso 5. Iniciar (TI)

```bash
docker compose up -d --build
```

La primera vez tarda unos minutos: descarga las imágenes, ejecuta las pruebas automáticas y compila. Si alguna prueba falla, no se inicia.

Verificar:
```bash
docker compose ps                          # app y mongo deben estar "healthy"
docker compose logs app | tail -20         # debe decir "servidor escuchando"
docker compose run --rm app check          # diagnóstico: token, organizaciones, proyectos y MongoDB
```

El diagnóstico `check` muestra qué organizaciones detectó, cuántos proyectos tiene cada una y si los campos (Status, Priority, fechas…) se reconocen. Si algo falla, el mismo mensaje indica qué corregir.

## Paso 6. Primer ingreso (administrador de la app)

1. Abrir la dirección de la aplicación (p. ej. `https://proyectos.empresa.com` o `http://IP-del-servidor:8080`).
2. Entrar con `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`.
3. Cambiar la contraseña (mínimo 12 caracteres, con letras y números).
4. **Administración** → crear los usuarios:
   - **Administrador**: ve todo y gestiona usuarios.
   - **Consulta**: solo ve proyectos (RRHH, gerencia, etc.).
5. Entregar a cada persona su contraseña temporal; la cambiará en su primer ingreso.

Después del primer inicio, TI puede borrar `BOOTSTRAP_ADMIN_PASSWORD` de `.env` (el administrador ya existe en la base de datos).

## Paso 7. HTTPS (TI, recomendado)

Sin HTTPS las contraseñas viajan sin cifrar por la red. Lo más sencillo es Caddy, que obtiene el certificado automáticamente (requiere un dominio que apunte al servidor):

```
proyectos.empresa.com {
    reverse_proxy 127.0.0.1:8080
}
```

En `.env`: `APP_PUBLISH=127.0.0.1:8080`, `COOKIE_SECURE=true` y `TRUST_PROXY_HEADERS=true`. Luego `docker compose up -d`.

---

## Mantenimiento

| Tarea | Cómo |
|---|---|
| **Renovar el token** (antes de que venza) | El dueño genera uno nuevo (paso 1) → TI lo pega en `GITHUB_TOKEN` de `.env` → `docker compose up -d` |
| **Agregar una organización** | Invitar a la cuenta del token a esa organización. No hay que tocar el servidor |
| **Actualizar la aplicación** | `git pull && docker compose up -d --build` |
| **Ver registros** | `docker compose logs -f app` |
| **Reiniciar** | `docker compose restart app` |
| **Detener** | `docker compose down` (los usuarios se conservan) |
| **Respaldo de la base de datos** | `docker compose exec mongo sh -c 'mongodump --archive -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD"' > respaldo-$(date +%F).archive` |
| **Olvido de la clave del administrador** | `docker compose run --rm -e ADMIN_EMAIL=admin@empresa.com -e ADMIN_PASSWORD='NuevaTemporal2026' app create-admin` |

> `docker compose down -v` **borra la base de datos** (usuarios y bitácora). No usarlo salvo que se quiera empezar de cero.

## Problemas comunes

| Síntoma | Solución |
|---|---|
| El login es correcto pero vuelve a la pantalla de ingreso | `COOKIE_SECURE=true` sin HTTPS. Use `false` o configure HTTPS |
| No aparece ninguna organización | La cuenta del token no es miembro de ninguna, o al token le falta `read:org` |
| Falta una organización | Invitar a la cuenta del token; si usa SSO, autorizar el token (paso 1.6). El `check` indica cuál falla |
| "El token de GitHub no es válido o expiró" | Generar un token nuevo y actualizar `.env` |
| Tareas "(Elemento sin acceso)" | Al token le falta `repo`, o la cuenta no tiene acceso a ese repositorio privado |
| Estado, prioridad o fechas vacíos | Los campos tienen otro nombre en GitHub: ajustar `FIELD_STATUS`, `FIELD_PRIORITY`, etc. en `.env` (el `check` muestra los nombres reales) |
| No se puede abrir desde otros equipos | `APP_PUBLISH=8080` (sin HTTPS) o configurar el proxy HTTPS; revisar el firewall del servidor |
