package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wmendoza/gh-projects-viewer/internal/auth"
	"github.com/wmendoza/gh-projects-viewer/internal/config"
	"github.com/wmendoza/gh-projects-viewer/internal/github"
	"github.com/wmendoza/gh-projects-viewer/internal/store"
)

// check verifica configuración, GitHub y MongoDB, e imprime un diagnóstico legible.
// No modifica nada (solo lectura). Devuelve 0 si todo está bien.
func check() int {
	ok := true
	pass := func(f string, a ...any) { fmt.Printf("  ✅ "+f+"\n", a...) }
	fail := func(f string, a ...any) { ok = false; fmt.Printf("  ❌ "+f+"\n", a...) }
	warn := func(f string, a ...any) { fmt.Printf("  ⚠️  "+f+"\n", a...) }
	info := func(f string, a ...any) { fmt.Printf("     "+f+"\n", a...) }

	fmt.Println("\n1) Configuración")
	cfg, err := config.Load()
	if err != nil {
		fail("%v", err)
		fmt.Println("\nCorrija el archivo .env y vuelva a ejecutar: go run ./cmd/server check")
		return 1
	}
	pass("Variables de entorno válidas (GITHUB_MODE=%s, STORE=%s)", cfg.GitHubMode, cfg.StoreKind)
	if !cfg.CookieSecure {
		warn("COOKIE_SECURE=false: correcto para http://localhost, NO para producción")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("\n2) MongoDB")
	if cfg.StoreKind == "memory" {
		warn("STORE=memory: no se usa MongoDB (los usuarios se pierden al reiniciar)")
	} else {
		st, err := openStore(ctx, cfg)
		if err != nil {
			fail("%v", err)
			info("En Mac: brew services start mongodb-community   (o revise MONGO_URI)")
		} else {
			n, err := st.CountUsers(ctx)
			if err != nil {
				fail("Conectado, pero no se pudo leer la colección de usuarios: %v", err)
			} else {
				pass("Conectado a la base %q (%d usuario(s) registrados)", cfg.MongoDB, n)
				if n == 0 && cfg.BootstrapAdminEmail == "" {
					warn("No hay usuarios y BOOTSTRAP_ADMIN_EMAIL está vacío: nadie podrá ingresar")
				} else if n == 0 {
					info("Al iniciar se creará el administrador %s", cfg.BootstrapAdminEmail)
				}
			}
			_ = st.Close(context.Background())
		}
	}

	fmt.Println("\n3) GitHub")
	if cfg.GitHubMode == "demo" {
		warn("GITHUB_MODE=demo: se muestran datos ficticios. Para GitHub real use GITHUB_MODE=api")
	} else {
		c := newGitHubClient(cfg)
		login, err := c.Viewer(ctx)
		if err != nil {
			fail("No se pudo consultar GitHub con el token: %v", err)
			switch {
			case errors.Is(err, github.ErrUnauthorized):
				info("Revise GITHUB_TOKEN (¿copiado completo? ¿expiró? ¿fue revocado?)")
			case errors.Is(err, github.ErrPermission):
				info("Si la organización usa SSO: en GitHub > Settings > Tokens, pulse 'Configure SSO' y autorice el token.")
				info("Si es token fine-grained: un admin de la organización debe aprobarlo.")
			default:
				info("Revise su conexión a internet y GITHUB_API_URL")
			}
		} else {
			pass("Token válido. Pertenece al usuario de GitHub: %s", login)
			if sc := c.Scopes(); sc != "" {
				info("Token clásico con permisos: %s", sc)
				if !strings.Contains(sc, "project") {
					fail("Falta el permiso 'read:project' en el token clásico")
				}
			} else {
				info("Token fine-grained (los permisos se validan al consultar)")
			}

			projects, err := c.ListProjects(ctx)
			switch {
			case err != nil:
				fail("No se pudieron listar los proyectos de %s %q: %v", cfg.GitHubOwnerType, cfg.GitHubOwner, err)
				if errors.Is(err, github.ErrNotFound) {
					info("¿Es correcto GITHUB_OWNER? ¿GITHUB_OWNER_TYPE debe ser 'user' en vez de 'org' (o al revés)?")
				}
				if errors.Is(err, github.ErrPermission) {
					info("El token necesita permiso de lectura de Projects (read:project o 'Projects: Read-only')")
				}
			case len(projects) == 0:
				warn("Conexión correcta, pero %s %q no tiene proyectos visibles para este token", cfg.GitHubOwnerType, cfg.GitHubOwner)
				info("Cree un proyecto en GitHub (pestaña Projects) o revise los permisos del token")
			default:
				pass("%d proyecto(s) encontrados en %s", len(projects), cfg.GitHubOwner)
				for i, p := range projects {
					if i >= 3 {
						info("… y %d más", len(projects)-3)
						break
					}
					d, err := c.GetProject(ctx, p.ID)
					if err != nil {
						fail("Proyecto #%d %q: error al leer tareas: %v", p.Number, p.Title, err)
						continue
					}
					pass("Proyecto #%d %q: %d tarea(s)", p.Number, p.Title, len(d.Tasks))
					info("Campos del proyecto: %s", strings.Join(d.FieldNames, ", "))
					report := func(concept string, aliases []string, get func(github.Task) string) {
						count := 0
						for _, t := range d.Tasks {
							if get(t) != "" {
								count++
							}
						}
						if count > 0 {
							info("  %-15s reconocido en %d tarea(s)", concept, count)
						} else if len(d.Tasks) > 0 {
							warn("  %-15s sin datos. Si existe con otro nombre, agréguelo a la variable (actual: %s)", concept, strings.Join(aliases, ","))
						}
					}
					report("Estado", cfg.FieldStatus, func(t github.Task) string { return t.Status })
					report("Prioridad", cfg.FieldPriority, func(t github.Task) string { return t.Priority })
					report("Estimación", cfg.FieldEstimate, func(t github.Task) string { return t.Estimate })
					report("Fecha inicio", cfg.FieldStartDate, func(t github.Task) string { return t.StartDate })
					report("Fecha entrega", cfg.FieldDueDate, func(t github.Task) string { return t.DueDate })
				}
			}
		}
	}

	fmt.Println()
	if ok {
		fmt.Println("Todo listo. Inicie la aplicación con: go run ./cmd/server")
		return 0
	}
	fmt.Println("Hay problemas que corregir (ver ❌ arriba).")
	return 1
}

// createAdmin crea un administrador o restablece la contraseña de uno existente.
// Útil si se olvidó la contraseña del administrador o para crear el primero manualmente.
func createAdmin() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	pw := os.Getenv("ADMIN_PASSWORD")
	if email == "" || pw == "" {
		return errors.New("defina ADMIN_EMAIL y ADMIN_PASSWORD, por ejemplo:\n  ADMIN_EMAIL=yo@empresa.com ADMIN_PASSWORD='ClaveTemporal2026' go run ./cmd/server create-admin")
	}
	if err := auth.ValidatePassword(pw); err != nil {
		return err
	}
	if cfg.StoreKind != "mongo" {
		return errors.New("create-admin solo tiene sentido con STORE=mongo (la memoria se borra al terminar)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close(context.Background())

	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	u, err := st.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		u = &store.User{ID: auth.NewID(), Email: email, Name: "Administrador", Role: store.RoleAdmin,
			PasswordHash: hash, Active: true, MustChangePassword: true, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateUser(ctx, u); err != nil {
			return err
		}
		fmt.Printf("Administrador %s creado. Deberá cambiar la contraseña al ingresar.\n", email)
	case err != nil:
		return err
	default:
		u.PasswordHash, u.Role, u.Active, u.MustChangePassword, u.UpdatedAt = hash, store.RoleAdmin, true, true, now
		if err := st.UpdateUser(ctx, u); err != nil {
			return err
		}
		_ = st.DeleteUserSessions(ctx, u.ID)
		fmt.Printf("Contraseña de %s restablecida (rol administrador, activo). Deberá cambiarla al ingresar.\n", email)
	}
	_ = st.AddAudit(ctx, &store.AuditEntry{ID: auth.NewID(), At: now, Action: "admin_cli", Target: email, IP: "consola", Success: true})
	return nil
}
