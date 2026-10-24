package httpapi

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"gh-projects-viewer/internal/auth"
	"gh-projects-viewer/internal/store"
)

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxSession
)

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(ctxUser).(*store.User)
	return u
}

func sessionFrom(r *http.Request) *store.Session {
	s, _ := r.Context().Value(ctxSession).(*store.Session)
	return s
}

// securityHeaders aplica cabeceras de seguridad a todas las respuestas.
func securityHeaders(next http.Handler, https bool) http.Handler {
	csp := "default-src 'self'; script-src 'self'; style-src 'self'; " +
		"img-src 'self' data: https://avatars.githubusercontent.com; connect-src 'self'; " +
		"font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if https {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequests registra método, ruta, estado y duración. No registra query strings,
// cuerpos ni cabeceras (evita guardar contraseñas o tokens en los logs).
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"ms", time.Since(start).Milliseconds(), "ip", s.clientIP(r))
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic", "value", v, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "Error interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// csrfProtect: toda petición que modifica estado debe venir del mismo origen.
// Además, las rutas autenticadas validan el token CSRF de la sesión (ver authed).
func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// Sec-Fetch-Site lo envían todos los navegadores modernos y no se puede falsificar desde JS.
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			writeError(w, http.StatusForbidden, "csrf", "Origen no permitido")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			scheme := "http"
			if r.TLS != nil || (s.opts.TrustProxyHeaders && r.Header.Get("X-Forwarded-Proto") == "https") {
				scheme = "https"
			}
			if origin != scheme+"://"+r.Host {
				writeError(w, http.StatusForbidden, "csrf", "Origen no permitido")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// authed valida la cookie de sesión, los tiempos de expiración, que el usuario
// siga activo y (para métodos no seguros) el token CSRF.
func (s *Server) authed(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName())
		if err != nil || len(c.Value) != 64 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Sesión no válida")
			return
		}
		ctx := r.Context()
		hash := auth.HashToken(c.Value)
		sess, err := s.store.GetSession(ctx, hash)
		if err != nil {
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Sesión no válida")
			return
		}
		now := s.now()
		if now.After(sess.ExpiresAt) || now.Sub(sess.LastSeenAt) > s.opts.SessionIdleTimeout {
			_ = s.store.DeleteSession(ctx, hash)
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "session_expired", "La sesión expiró")
			return
		}
		user, err := s.store.GetUserByID(ctx, sess.UserID)
		if err != nil || !user.Active {
			_ = s.store.DeleteSession(ctx, hash)
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "unauthorized", "Sesión no válida")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.Header.Get("X-CSRF-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFToken)) != 1 {
				writeError(w, http.StatusForbidden, "csrf", "Token CSRF inválido")
				return
			}
		}
		if now.Sub(sess.LastSeenAt) > time.Minute {
			_ = s.store.TouchSession(ctx, hash, now)
		}
		ctx = context.WithValue(ctx, ctxUser, user)
		ctx = context.WithValue(ctx, ctxSession, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r); u == nil || u.Role != store.RoleAdmin {
			writeError(w, http.StatusForbidden, "forbidden", "Requiere rol administrador")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pwChanged bloquea el acceso a datos hasta que el usuario cambie su contraseña temporal.
func (s *Server) pwChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r); u != nil && u.MustChangePassword {
			writeError(w, http.StatusForbidden, "password_change_required", "Debe cambiar su contraseña")
			return
		}
		next.ServeHTTP(w, r)
	})
}
