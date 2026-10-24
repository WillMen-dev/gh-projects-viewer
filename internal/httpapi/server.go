// Package httpapi expone la API REST y sirve el frontend.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"gh-projects-viewer/internal/auth"
	"gh-projects-viewer/internal/github"
	"gh-projects-viewer/internal/store"
)

type Options struct {
	CookieSecure       bool
	SessionIdleTimeout time.Duration
	SessionMaxLifetime time.Duration
	TrustProxyHeaders  bool
	DoneStatuses       []string
	DemoMode           bool
}

type Server struct {
	store    store.Store
	provider github.Provider
	cache    *github.CachedProvider // puede ser nil
	opts     Options
	static   fs.FS

	loginIPLimiter    *auth.Limiter
	loginEmailLimiter *auth.Limiter
	refreshLimiter    *auth.Limiter
	now               func() time.Time
}

func New(st store.Store, provider github.Provider, static fs.FS, opts Options) *Server {
	s := &Server{
		store:    st,
		provider: provider,
		opts:     opts,
		static:   static,
		// 10 intentos fallidos por IP o 5 por cuenta en 15 minutos => bloqueo temporal.
		loginIPLimiter:    auth.NewLimiter(10, 15*time.Minute),
		loginEmailLimiter: auth.NewLimiter(5, 15*time.Minute),
		// Máximo 6 refrescos forzados por usuario por minuto (protege el límite de GitHub).
		refreshLimiter: auth.NewLimiter(6, time.Minute),
		now:            time.Now,
	}
	if c, ok := provider.(*github.CachedProvider); ok {
		s.cache = c
	}
	return s
}

func (s *Server) cookieName() string {
	// El prefijo __Host- obliga al navegador a exigir Secure, Path=/ y sin Domain.
	if s.opts.CookieSecure {
		return "__Host-gpv_session"
	}
	return "gpv_session"
}

// Handler construye el enrutador con todos los middlewares.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Salud (sin autenticación, sin datos sensibles)
	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Autenticación
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.Handle("POST /api/auth/logout", s.authed(http.HandlerFunc(s.handleLogout)))
	mux.Handle("GET /api/auth/me", s.authed(http.HandlerFunc(s.handleMe)))
	mux.Handle("POST /api/auth/password", s.authed(http.HandlerFunc(s.handleChangePassword)))

	// Consulta de proyectos (requiere contraseña ya cambiada)
	mux.Handle("GET /api/projects", s.authed(s.pwChanged(http.HandlerFunc(s.handleListProjects))))
	mux.Handle("GET /api/projects/{id}", s.authed(s.pwChanged(http.HandlerFunc(s.handleGetProject))))
	mux.Handle("POST /api/projects/{id}/refresh", s.authed(s.pwChanged(http.HandlerFunc(s.handleRefreshProject))))

	// Administración
	admin := func(h http.HandlerFunc) http.Handler { return s.authed(s.pwChanged(s.adminOnly(h))) }
	mux.Handle("GET /api/admin/users", admin(s.handleListUsers))
	mux.Handle("POST /api/admin/users", admin(s.handleCreateUser))
	mux.Handle("PATCH /api/admin/users/{id}", admin(s.handleUpdateUser))
	mux.Handle("GET /api/admin/audit", admin(s.handleListAudit))

	// API desconocida => 404 JSON
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Recurso no encontrado")
	})

	// Frontend estático
	mux.Handle("/", s.staticHandler())

	var h http.Handler = mux
	h = s.csrfProtect(h)
	h = securityHeaders(h, s.opts.CookieSecure)
	h = s.logRequests(h)
	h = recoverPanics(h)
	return h
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Método no permitido")
			return
		}
		p := r.URL.Path
		if p == "/" || p == "/index.html" || p == "/login" || p == "/login.html" {
			w.Header().Set("Cache-Control", "no-store")
			name := "index.html"
			if strings.HasPrefix(p, "/login") {
				name = "login.html"
			}
			data, err := fs.ReadFile(s.static, name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		if !strings.HasPrefix(p, "/static/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------- utilidades ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError nunca incluye detalles internos (trazas, consultas, tokens).
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

// decodeJSON limita el tamaño del cuerpo y rechaza campos desconocidos.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Se espera application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Solicitud inválida")
		return false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "Solicitud inválida")
		return false
	}
	return true
}

func (s *Server) clientIP(r *http.Request) string {
	if s.opts.TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			// El último valor lo agrega el proxy de confianza; los anteriores los controla el cliente.
			if ip := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) audit(r *http.Request, actor *store.User, action, target string, ok bool) {
	e := &store.AuditEntry{
		ID: auth.NewID(), At: s.now().UTC(), Action: action, Target: target,
		IP: s.clientIP(r), Success: ok,
	}
	if actor != nil {
		e.ActorID, e.ActorMail = actor.ID, actor.Email
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.AddAudit(ctx, e); err != nil {
		slog.Error("no se pudo registrar auditoría", "err", err)
	}
}
