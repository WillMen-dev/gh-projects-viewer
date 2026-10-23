// Package config carga la configuración de la aplicación desde variables de entorno.
// Ningún secreto se escribe en el código: todo llega por entorno o archivo .env (no versionado).
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr string // dirección de escucha, p.ej. ":8080"

	// GitHub
	GitHubMode      string // "api" (real) o "demo" (datos de ejemplo)
	GitHubToken     string
	GitHubOwners    []string // organizaciones o usuarios dueños de los proyectos
	GitHubOwnerType string   // "org", "user" o "auto" (organizaciones del usuario del token)
	GitHubAPIURL    string
	CacheTTL        time.Duration

	// Nombres de campos personalizados en GitHub Projects (admite alias separados por coma)
	FieldStatus    []string
	FieldPriority  []string
	FieldEstimate  []string
	FieldStartDate []string
	FieldDueDate   []string
	DoneStatuses   []string // valores de estado que significan "terminado"

	// Persistencia
	StoreKind string // "mongo" o "memory"
	MongoURI  string
	MongoDB   string

	// Seguridad
	CookieSecure       bool
	SessionIdleTimeout time.Duration
	SessionMaxLifetime time.Duration
	TrustProxyHeaders  bool // solo true si hay un proxy inverso de confianza delante

	// Administrador inicial (solo se usa si no existe ningún usuario)
	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

// ownerRe valida nombres de usuario u organización de GitHub.
var ownerRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func list(key, def string) []string {
	raw := env(key, def)
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolean(key string, def bool) (bool, error) {
	v := env(key, strconv.FormatBool(def))
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s debe ser true/false", key)
	}
	return b, nil
}

func duration(key string, def time.Duration) (time.Duration, error) {
	v := env(key, def.String())
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s debe ser una duración válida (ej. 30m, 8h)", key)
	}
	return d, nil
}

// Load lee y valida la configuración.
func Load() (*Config, error) {
	c := &Config{
		Addr:            env("APP_ADDR", ":8080"),
		GitHubMode:      strings.ToLower(env("GITHUB_MODE", "api")),
		GitHubToken:     os.Getenv("GITHUB_TOKEN"),
		GitHubOwners:    list("GITHUB_OWNER", ""),
		GitHubOwnerType: strings.ToLower(env("GITHUB_OWNER_TYPE", "org")),
		GitHubAPIURL:    env("GITHUB_API_URL", "https://api.github.com/graphql"),

		FieldStatus:    list("FIELD_STATUS", "Status,Estado"),
		FieldPriority:  list("FIELD_PRIORITY", "Priority,Prioridad"),
		FieldEstimate:  list("FIELD_ESTIMATE", "Estimate,Estimación,Estimacion,Size"),
		FieldStartDate: list("FIELD_START_DATE", "Start date,Start Date,Fecha de inicio,Fecha inicio"),
		FieldDueDate:   list("FIELD_DUE_DATE", "Target date,Due date,End date,Fecha de entrega,Fecha entrega"),
		DoneStatuses:   list("DONE_STATUSES", "Done,Hecho,Terminado,Completado,Finalizado"),

		StoreKind: strings.ToLower(env("STORE", "mongo")),
		MongoURI:  os.Getenv("MONGO_URI"),
		MongoDB:   env("MONGO_DB", "gh_projects_viewer"),

		BootstrapAdminEmail:    strings.ToLower(env("BOOTSTRAP_ADMIN_EMAIL", "")),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}

	var err error
	if c.CacheTTL, err = duration("CACHE_TTL", 2*time.Minute); err != nil {
		return nil, err
	}
	if c.CookieSecure, err = boolean("COOKIE_SECURE", true); err != nil {
		return nil, err
	}
	if c.TrustProxyHeaders, err = boolean("TRUST_PROXY_HEADERS", false); err != nil {
		return nil, err
	}
	if c.SessionIdleTimeout, err = duration("SESSION_IDLE_TIMEOUT", 30*time.Minute); err != nil {
		return nil, err
	}
	if c.SessionMaxLifetime, err = duration("SESSION_MAX_LIFETIME", 8*time.Hour); err != nil {
		return nil, err
	}

	switch c.GitHubMode {
	case "api":
		if c.GitHubToken == "" {
			return nil, errors.New("GITHUB_TOKEN es obligatorio cuando GITHUB_MODE=api")
		}
		if c.GitHubOwnerType != "org" && c.GitHubOwnerType != "user" && c.GitHubOwnerType != "auto" {
			return nil, errors.New("GITHUB_OWNER_TYPE debe ser 'org', 'user' o 'auto'")
		}
		if len(c.GitHubOwners) == 0 && c.GitHubOwnerType != "auto" {
			return nil, errors.New("GITHUB_OWNER es obligatorio cuando GITHUB_MODE=api (o use GITHUB_OWNER_TYPE=auto)")
		}
		for _, o := range c.GitHubOwners {
			if !ownerRe.MatchString(o) {
				return nil, fmt.Errorf("GITHUB_OWNER contiene un nombre inválido: %q", o)
			}
		}
		if !strings.HasPrefix(c.GitHubAPIURL, "https://") {
			return nil, errors.New("GITHUB_API_URL debe usar https")
		}
	case "demo":
	default:
		return nil, errors.New("GITHUB_MODE debe ser 'api' o 'demo'")
	}

	switch c.StoreKind {
	case "mongo":
		if c.MongoURI == "" {
			return nil, errors.New("MONGO_URI es obligatorio cuando STORE=mongo")
		}
	case "memory":
	default:
		return nil, errors.New("STORE debe ser 'mongo' o 'memory'")
	}

	if c.BootstrapAdminEmail != "" && len(c.BootstrapAdminPassword) < 12 {
		return nil, errors.New("BOOTSTRAP_ADMIN_PASSWORD debe tener al menos 12 caracteres")
	}
	return c, nil
}
