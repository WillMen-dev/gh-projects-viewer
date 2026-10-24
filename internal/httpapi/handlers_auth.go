package httpapi

import (
	"net/http"
	"strings"
	"time"

	"gh-projects-viewer/internal/auth"
	"gh-projects-viewer/internal/store"
)

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,                    // inaccesible desde JavaScript (mitiga robo por XSS)
		Secure:   s.opts.CookieSecure,     // solo por HTTPS en producción
		SameSite: http.SameSiteStrictMode, // no se envía en peticiones de otros sitios (CSRF)
		MaxAge:   int(s.opts.SessionMaxLifetime.Seconds()),
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

type meResponse struct {
	User      *store.User `json:"user"`
	CSRFToken string      `json:"csrfToken"`
	DemoMode  bool        `json:"demoMode"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	ip := s.clientIP(r)
	if len(email) > 254 || len(in.Password) > auth.MaxPasswordLen {
		writeError(w, http.StatusBadRequest, "bad_request", "Solicitud inválida")
		return
	}

	if s.loginIPLimiter.Blocked(ip) || s.loginEmailLimiter.Blocked(email) {
		s.audit(r, nil, "login_blocked", email, false)
		writeError(w, http.StatusTooManyRequests, "too_many_attempts",
			"Demasiados intentos fallidos. Intente de nuevo en 15 minutos.")
		return
	}

	ctx := r.Context()
	user, err := s.store.GetUserByEmail(ctx, email)
	valid := false
	if err == nil {
		valid = auth.CheckPassword(in.Password, user.PasswordHash) && user.Active
	} else {
		auth.DummyCheck(in.Password) // mismo tiempo de respuesta exista o no el usuario
	}
	if !valid {
		s.loginIPLimiter.Hit(ip)
		s.loginEmailLimiter.Hit(email)
		s.audit(r, nil, "login_failed", email, false)
		// Mensaje genérico: no revela si el correo existe.
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Correo o contraseña incorrectos")
		return
	}
	s.loginEmailLimiter.Reset(email)

	token, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	csrf, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	now := s.now()
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	sess := &store.Session{
		TokenHash: auth.HashToken(token), UserID: user.ID, CSRFToken: csrf,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(s.opts.SessionMaxLifetime),
		IP: ip, UserAgent: ua,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	user.LastLoginAt = now.UTC()
	_ = s.store.UpdateUser(ctx, user)
	s.audit(r, user, "login", user.Email, true)

	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, meResponse{User: user, CSRFToken: csrf, DemoMode: s.opts.DemoMode})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := sessionFrom(r); sess != nil {
		_ = s.store.DeleteSession(r.Context(), sess.TokenHash)
	}
	s.audit(r, userFrom(r), "logout", "", true)
	s.clearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, meResponse{User: userFrom(r), CSRFToken: sessionFrom(r).CSRFToken, DemoMode: s.opts.DemoMode})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	user := userFrom(r)
	if !auth.CheckPassword(in.Current, user.PasswordHash) {
		s.audit(r, user, "password_change", user.Email, false)
		writeError(w, http.StatusBadRequest, "invalid_password", "La contraseña actual es incorrecta")
		return
	}
	if err := auth.ValidatePassword(in.New); err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	if in.New == in.Current {
		writeError(w, http.StatusBadRequest, "weak_password", "La nueva contraseña debe ser diferente")
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	user.UpdatedAt = time.Now().UTC()
	ctx := r.Context()
	if err := s.store.UpdateUser(ctx, user); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	// Cierra las demás sesiones del usuario y conserva la actual.
	current := sessionFrom(r)
	_ = s.store.DeleteUserSessions(ctx, user.ID)
	_ = s.store.CreateSession(ctx, current)
	s.audit(r, user, "password_change", user.Email, true)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
