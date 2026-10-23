package httpapi

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wmendoza/gh-projects-viewer/internal/auth"
	"github.com/wmendoza/gh-projects-viewer/internal/store"
)

func validEmail(e string) bool {
	if len(e) > 254 {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e
}

func validName(n string) bool {
	l := utf8.RuneCountInString(n)
	return l >= 1 && l <= 100 && !strings.ContainsAny(n, "<>")
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		Role              string `json:"role"`
		TemporaryPassword string `json:"temporaryPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if !validEmail(in.Email) {
		writeError(w, http.StatusBadRequest, "invalid_email", "Correo inválido")
		return
	}
	if !validName(in.Name) {
		writeError(w, http.StatusBadRequest, "invalid_name", "Nombre inválido (1 a 100 caracteres)")
		return
	}
	if in.Role != store.RoleAdmin && in.Role != store.RoleViewer {
		writeError(w, http.StatusBadRequest, "invalid_role", "Rol inválido")
		return
	}
	if err := auth.ValidatePassword(in.TemporaryPassword); err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}
	hash, err := auth.HashPassword(in.TemporaryPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	now := time.Now().UTC()
	u := &store.User{
		ID: auth.NewID(), Email: in.Email, Name: in.Name, Role: in.Role, PasswordHash: hash,
		Active: true, MustChangePassword: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate", "Ya existe un usuario con ese correo")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	s.audit(r, userFrom(r), "user_create", u.Email+" ("+u.Role+")", true)
	writeJSON(w, http.StatusCreated, map[string]any{"user": u})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name              *string `json:"name"`
		Role              *string `json:"role"`
		Active            *bool   `json:"active"`
		TemporaryPassword *string `json:"temporaryPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	actor := userFrom(r)
	id := r.PathValue("id")
	if len(id) != 24 {
		writeError(w, http.StatusBadRequest, "bad_request", "Identificador inválido")
		return
	}
	ctx := r.Context()
	u, err := s.store.GetUserByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Usuario no encontrado")
		return
	}
	// Un administrador no puede quitarse a sí mismo el rol ni desactivarse (evita quedar sin admins).
	if u.ID == actor.ID && ((in.Role != nil && *in.Role != store.RoleAdmin) || (in.Active != nil && !*in.Active)) {
		writeError(w, http.StatusBadRequest, "self_lockout", "No puede quitarse su propio rol de administrador ni desactivarse")
		return
	}
	var changes []string
	revokeSessions := false
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if !validName(n) {
			writeError(w, http.StatusBadRequest, "invalid_name", "Nombre inválido")
			return
		}
		u.Name = n
		changes = append(changes, "name")
	}
	if in.Role != nil {
		if *in.Role != store.RoleAdmin && *in.Role != store.RoleViewer {
			writeError(w, http.StatusBadRequest, "invalid_role", "Rol inválido")
			return
		}
		u.Role = *in.Role
		changes = append(changes, "role="+u.Role)
		revokeSessions = true
	}
	if in.Active != nil {
		u.Active = *in.Active
		changes = append(changes, "active="+map[bool]string{true: "true", false: "false"}[u.Active])
		if !u.Active {
			revokeSessions = true
		}
	}
	if in.TemporaryPassword != nil {
		if err := auth.ValidatePassword(*in.TemporaryPassword); err != nil {
			writeError(w, http.StatusBadRequest, "weak_password", err.Error())
			return
		}
		hash, err := auth.HashPassword(*in.TemporaryPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Error interno")
			return
		}
		u.PasswordHash = hash
		u.MustChangePassword = true
		changes = append(changes, "password_reset")
		revokeSessions = true
	}
	if len(changes) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "No hay cambios")
		return
	}
	u.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateUser(ctx, u); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	if revokeSessions && u.ID != actor.ID {
		_ = s.store.DeleteUserSessions(ctx, u.ID)
	}
	s.audit(r, actor, "user_update", u.Email+" ["+strings.Join(changes, ",")+"]", true)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListAudit(r.Context(), 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Error interno")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
