// Punto de entrada de la aplicación web para consulta de proyectos de GitHub Projects.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gh-projects-viewer/internal/auth"
	"gh-projects-viewer/internal/config"
	"gh-projects-viewer/internal/github"
	"gh-projects-viewer/internal/httpapi"
	"gh-projects-viewer/internal/store"
	"gh-projects-viewer/web"
)

const usage = `Uso: server [comando]

Comandos:
  (sin comando)   Inicia el servidor web
  check           Verifica la configuración, la conexión con GitHub y con MongoDB
  create-admin    Crea un administrador o restablece su contraseña
                  (usa ADMIN_EMAIL y ADMIN_PASSWORD del entorno)
  healthcheck     Usado por Docker para comprobar que el servidor responde
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	// "server healthcheck" lo usa Docker (la imagen distroless no trae curl).
	if cmd == "healthcheck" {
		os.Exit(healthcheck())
	}

	// Carga .env de la carpeta actual si existe (las variables del sistema tienen prioridad).
	envPath := os.Getenv("ENV_FILE")
	if envPath == "" {
		envPath = ".env"
	}
	loaded, err := config.LoadDotEnv(envPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error leyendo", envPath+":", err)
		os.Exit(1)
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))
	if loaded {
		slog.Info("configuración cargada desde archivo", "archivo", envPath)
	}

	switch cmd {
	case "":
		err = run()
	case "check":
		os.Exit(check())
	case "create-admin":
		err = createAdmin()
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprint(os.Stderr, "Comando desconocido: "+cmd+"\n\n"+usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "\nError:", err)
		os.Exit(1)
	}
}

// openStore abre el almacenamiento configurado.
func openStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	if cfg.StoreKind == "mongo" {
		m, err := store.NewMongo(ctx, cfg.MongoURI, cfg.MongoDB)
		if err != nil {
			return nil, fmt.Errorf("no fue posible conectar con MongoDB (revise MONGO_URI y que MongoDB esté iniciado): %w", err)
		}
		return m, nil
	}
	return store.NewMemory(), nil
}

func newGitHubClient(cfg *config.Config) *github.Client {
	return github.NewClient(cfg.GitHubAPIURL, cfg.GitHubToken, cfg.GitHubOwners, cfg.GitHubOwnerType == "auto", github.FieldMapping{
		Status: cfg.FieldStatus, Priority: cfg.FieldPriority, Estimate: cfg.FieldEstimate,
		StartDate: cfg.FieldStartDate, DueDate: cfg.FieldDueDate,
	})
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Persistencia
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.StoreKind == "mongo" {
		slog.Info("almacenamiento: MongoDB", "db", cfg.MongoDB)
	} else {
		slog.Warn("almacenamiento: MEMORIA (los usuarios se pierden al reiniciar; solo para pruebas)")
	}
	defer st.Close(context.Background())

	if err := bootstrapAdmin(ctx, st, cfg); err != nil {
		return err
	}

	// Fuente de datos
	var provider github.Provider
	if cfg.GitHubMode == "demo" {
		provider = github.NewDemoProvider()
		slog.Warn("GitHub: MODO DEMO con datos ficticios")
	} else {
		provider = newGitHubClient(cfg)
		slog.Info("GitHub: API GraphQL", "owners", strings.Join(cfg.GitHubOwners, ","), "type", cfg.GitHubOwnerType)
	}
	provider = github.NewCachedProvider(provider, cfg.CacheTTL)

	if !cfg.CookieSecure {
		slog.Warn("COOKIE_SECURE=false: use esto SOLO en desarrollo local sin HTTPS")
	}

	srv := httpapi.New(st, provider, web.Static(), httpapi.Options{
		CookieSecure:       cfg.CookieSecure,
		SessionIdleTimeout: cfg.SessionIdleTimeout,
		SessionMaxLifetime: cfg.SessionMaxLifetime,
		TrustProxyHeaders:  cfg.TrustProxyHeaders,
		DoneStatuses:       cfg.DoneStatuses,
		DemoMode:           cfg.GitHubMode == "demo",
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	errCh := make(chan error, 1)
	go func() {
		host := cfg.Addr
		if strings.HasPrefix(host, ":") {
			host = "localhost" + host
		}
		slog.Info("servidor escuchando", "addr", cfg.Addr, "abrir", "http://"+host)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("apagando servidor...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func healthcheck() int {
	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// bootstrapAdmin crea el primer administrador SOLO si la base de datos no tiene usuarios.
// Se le obliga a cambiar la contraseña en el primer inicio de sesión.
func bootstrapAdmin(ctx context.Context, st store.Store, cfg *config.Config) error {
	n, err := st.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.BootstrapAdminEmail == "" {
		slog.Warn("no hay usuarios y BOOTSTRAP_ADMIN_EMAIL no está definido: nadie podrá iniciar sesión")
		return nil
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	u := &store.User{
		ID: auth.NewID(), Email: cfg.BootstrapAdminEmail, Name: "Administrador", Role: store.RoleAdmin,
		PasswordHash: hash, Active: true, MustChangePassword: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.CreateUser(ctx, u); err != nil {
		return err
	}
	slog.Info("administrador inicial creado; debe cambiar la contraseña al ingresar", "email", u.Email)
	return nil
}
