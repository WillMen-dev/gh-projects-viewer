package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wmendoza/gh-projects-viewer/internal/auth"
	"github.com/wmendoza/gh-projects-viewer/internal/github"
	"github.com/wmendoza/gh-projects-viewer/internal/store"
)

type env struct {
	t   *testing.T
	srv *Server
	h   http.Handler
	st  *store.Memory
}

func newEnv(t *testing.T) *env {
	st := store.NewMemory()
	static := fstest.MapFS{
		"index.html":       {Data: []byte("<html>app</html>")},
		"login.html":       {Data: []byte("<html>login</html>")},
		"static/js/app.js": {Data: []byte("console.log(1)")},
	}
	srv := New(st, github.NewCachedProvider(github.NewDemoProvider(), time.Minute), static, Options{
		CookieSecure: false, SessionIdleTimeout: 30 * time.Minute, SessionMaxLifetime: 8 * time.Hour,
		DoneStatuses: []string{"Done"}, DemoMode: true,
	})
	return &env{t: t, srv: srv, h: srv.Handler(), st: st}
}

func (e *env) addUser(email, pw, role string, mustChange bool) *store.User {
	h, err := auth.HashPassword(pw)
	if err != nil {
		e.t.Fatal(err)
	}
	u := &store.User{ID: auth.NewID(), Email: email, Name: "Test", Role: role, PasswordHash: h, Active: true, MustChangePassword: mustChange}
	if err := e.st.CreateUser(context.Background(), u); err != nil {
		e.t.Fatal(err)
	}
	return u
}

type client struct {
	e      *env
	cookie *http.Cookie
	csrf   string
}

func (c *client) do(method, path string, body any, headers ...string) (*httptest.ResponseRecorder, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func (e *env) login(email, pw string) (*client, *httptest.ResponseRecorder) {
	c := &client{e: e}
	rec, out := c.do("POST", "/api/auth/login", map[string]string{"email": email, "password": pw})
	if rec.Code == 200 {
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == "gpv_session" {
				c.cookie = ck
			}
		}
		c.csrf, _ = out["csrfToken"].(string)
	}
	return c, rec
}

func TestUnauthenticatedIsRejected(t *testing.T) {
	e := newEnv(t)
	c := &client{e: e}
	for _, p := range []string{"/api/projects", "/api/auth/me", "/api/admin/users"} {
		if rec, _ := c.do("GET", p, nil); rec.Code != 401 {
			t.Errorf("%s sin sesión debería ser 401, fue %d", p, rec.Code)
		}
	}
	c.cookie = &http.Cookie{Name: "gpv_session", Value: strings.Repeat("a", 64)}
	if rec, _ := c.do("GET", "/api/projects", nil); rec.Code != 401 {
		t.Errorf("cookie falsa debería ser 401, fue %d", rec.Code)
	}
}

func TestLoginFlowAndPasswordChange(t *testing.T) {
	e := newEnv(t)
	e.addUser("admin@empresa.com", "ClaveInicial2026", store.RoleAdmin, true)

	if _, rec := e.login("admin@empresa.com", "incorrecta123"); rec.Code != 401 {
		t.Fatalf("login incorrecto debe ser 401, fue %d", rec.Code)
	}
	if _, rec := e.login("noexiste@empresa.com", "incorrecta123"); rec.Code != 401 ||
		!strings.Contains(rec.Body.String(), "Correo o contraseña incorrectos") {
		t.Fatalf("usuario inexistente debe recibir el mismo mensaje genérico")
	}

	c, rec := e.login("ADMIN@empresa.com ", "ClaveInicial2026")
	if rec.Code != 200 || c.cookie == nil {
		t.Fatalf("login correcto falló: %d %s", rec.Code, rec.Body.String())
	}
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("la cookie debe ser HttpOnly y SameSite=Strict")
	}
	if strings.Contains(rec.Body.String(), "password") && strings.Contains(rec.Body.String(), "pbkdf2") {
		t.Fatal("la respuesta nunca debe incluir el hash de contraseña")
	}

	// Debe cambiar la contraseña antes de ver datos
	if rec, out := c.do("GET", "/api/projects", nil); rec.Code != 403 || out["error"] != "password_change_required" {
		t.Fatalf("esperaba password_change_required, obtuvo %d %v", rec.Code, out)
	}

	// Sin CSRF => 403
	csrf := c.csrf
	c.csrf = ""
	if rec, _ := c.do("POST", "/api/auth/password", map[string]string{"currentPassword": "ClaveInicial2026", "newPassword": "NuevaClave2026x"}); rec.Code != 403 {
		t.Fatalf("sin CSRF debe ser 403, fue %d", rec.Code)
	}
	c.csrf = csrf
	if rec, _ := c.do("POST", "/api/auth/password", map[string]string{"currentPassword": "ClaveInicial2026", "newPassword": "debil"}); rec.Code != 400 {
		t.Fatalf("contraseña débil debe ser 400, fue %d", rec.Code)
	}
	if rec, _ := c.do("POST", "/api/auth/password", map[string]string{"currentPassword": "ClaveInicial2026", "newPassword": "NuevaClave2026x"}); rec.Code != 200 {
		t.Fatalf("cambio de contraseña debe ser 200, fue %d", rec.Code)
	}

	rec, out := c.do("GET", "/api/projects", nil)
	if rec.Code != 200 {
		t.Fatalf("listar proyectos: %d", rec.Code)
	}
	projects := out["projects"].([]any)
	if len(projects) != 3 {
		t.Fatalf("esperaba 3 proyectos demo, hay %d", len(projects))
	}

	// Logout invalida la sesión en el servidor
	if rec, _ := c.do("POST", "/api/auth/logout", nil); rec.Code != 200 {
		t.Fatalf("logout: %d", rec.Code)
	}
	if rec, _ := c.do("GET", "/api/projects", nil); rec.Code != 401 {
		t.Fatalf("tras logout debe ser 401, fue %d", rec.Code)
	}
}

func TestProjectDetailAndValidation(t *testing.T) {
	e := newEnv(t)
	e.addUser("rrhh@empresa.com", "ClaveRRHH20261", store.RoleViewer, false)
	c, _ := e.login("rrhh@empresa.com", "ClaveRRHH20261")

	rec, out := c.do("GET", "/api/projects/PVT_demo_portal", nil)
	if rec.Code != 200 {
		t.Fatalf("detalle: %d", rec.Code)
	}
	sum := out["summary"].(map[string]any)
	if sum["total"].(float64) != 8 || sum["done"].(float64) != 2 {
		t.Fatalf("resumen inesperado: %v", sum)
	}
	if sum["overdue"].(float64) < 1 {
		t.Fatalf("el demo tiene al menos una tarea vencida: %v", sum)
	}
	if rec, _ := c.do("GET", "/api/projects/PVT_no_existe", nil); rec.Code != 404 {
		t.Fatalf("proyecto inexistente debe ser 404, fue %d", rec.Code)
	}
	if rec, _ := c.do("GET", "/api/projects/bad%20id$", nil); rec.Code != 400 {
		t.Fatalf("id inválido debe ser 400, fue %d", rec.Code)
	}
	// Un visualizador no accede a administración
	if rec, _ := c.do("GET", "/api/admin/users", nil); rec.Code != 403 {
		t.Fatalf("viewer en admin debe ser 403, fue %d", rec.Code)
	}
	// Refrescar está limitado
	codes := []int{}
	for i := 0; i < 8; i++ {
		r, _ := c.do("POST", "/api/projects/PVT_demo_portal/refresh", nil)
		codes = append(codes, r.Code)
	}
	if codes[0] != 200 || codes[7] != 429 {
		t.Fatalf("refresh debe limitarse: %v", codes)
	}
}

func TestBruteForceLockout(t *testing.T) {
	e := newEnv(t)
	e.addUser("ana@empresa.com", "ClaveCorrecta2026", store.RoleViewer, false)
	for i := 0; i < 5; i++ {
		e.login("ana@empresa.com", "mala-clave-000")
	}
	if _, rec := e.login("ana@empresa.com", "ClaveCorrecta2026"); rec.Code != 429 {
		t.Fatalf("tras 5 fallos la cuenta debe bloquearse temporalmente, fue %d", rec.Code)
	}
}

func TestSecurityHeadersAndCSRFOrigin(t *testing.T) {
	e := newEnv(t)
	c := &client{e: e}
	rec, _ := c.do("GET", "/", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("index: %d", rec.Code)
	}
	for _, h := range []string{"Content-Security-Policy", "X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("falta cabecera %s", h)
		}
	}
	if rec, _ := c.do("POST", "/api/auth/login", map[string]string{"email": "a@b.co", "password": "x"}, "Origin", "https://malicioso.com"); rec.Code != 403 {
		t.Fatalf("POST desde otro origen debe ser 403, fue %d", rec.Code)
	}
	if rec, _ := c.do("POST", "/api/auth/login", map[string]string{"email": "a@b.co", "password": "x"}, "Sec-Fetch-Site", "cross-site"); rec.Code != 403 {
		t.Fatalf("Sec-Fetch-Site cross-site debe ser 403, fue %d", rec.Code)
	}
	for _, p := range []string{"/../../etc/passwd", "/static/../../etc/passwd", "/static/%2e%2e/%2e%2e/etc/passwd"} {
		if rec, _ := c.do("GET", p, nil); rec.Code == 200 || strings.Contains(rec.Body.String(), "root:") {
			t.Fatalf("path traversal no debe servir archivos (%s => %d)", p, rec.Code)
		}
	}
	// Campos desconocidos se rechazan
	if rec, _ := c.do("POST", "/api/auth/login", map[string]any{"email": "a@b.co", "password": "x", "role": "admin"}); rec.Code != 400 {
		t.Fatalf("campos desconocidos deben rechazarse, fue %d", rec.Code)
	}
}

func TestSessionIdleTimeout(t *testing.T) {
	e := newEnv(t)
	e.addUser("ana@empresa.com", "ClaveCorrecta2026", store.RoleViewer, false)
	c, _ := e.login("ana@empresa.com", "ClaveCorrecta2026")
	e.srv.now = func() time.Time { return time.Now().Add(31 * time.Minute) }
	if rec, out := c.do("GET", "/api/projects", nil); rec.Code != 401 || out["error"] != "session_expired" {
		t.Fatalf("sesión inactiva debe expirar: %d %v", rec.Code, out)
	}
}

func TestAdminUserManagement(t *testing.T) {
	e := newEnv(t)
	admin := e.addUser("admin@empresa.com", "ClaveAdmin20261", store.RoleAdmin, false)
	c, _ := e.login("admin@empresa.com", "ClaveAdmin20261")

	rec, out := c.do("POST", "/api/admin/users", map[string]string{
		"email": "rrhh@empresa.com", "name": "Recursos Humanos", "role": "viewer", "temporaryPassword": "Temporal20261x",
	})
	if rec.Code != 201 {
		t.Fatalf("crear usuario: %d %v", rec.Code, out)
	}
	newID := out["user"].(map[string]any)["id"].(string)
	if rec, _ := c.do("POST", "/api/admin/users", map[string]string{
		"email": "rrhh@empresa.com", "name": "Dup", "role": "viewer", "temporaryPassword": "Temporal20261x",
	}); rec.Code != 409 {
		t.Fatalf("duplicado debe ser 409, fue %d", rec.Code)
	}
	if rec, _ := c.do("POST", "/api/admin/users", map[string]string{
		"email": "x@empresa.com", "name": "X", "role": "superuser", "temporaryPassword": "Temporal20261x",
	}); rec.Code != 400 {
		t.Fatalf("rol inválido debe ser 400, fue %d", rec.Code)
	}

	// El nuevo usuario inicia sesión y luego es desactivado: su sesión se revoca
	viewer, rec2 := e.login("rrhh@empresa.com", "Temporal20261x")
	if rec2.Code != 200 {
		t.Fatalf("login nuevo usuario: %d", rec2.Code)
	}
	if rec, _ := c.do("PATCH", "/api/admin/users/"+newID, map[string]any{"active": false}); rec.Code != 200 {
		t.Fatalf("desactivar: %d", rec.Code)
	}
	if rec, _ := viewer.do("GET", "/api/auth/me", nil); rec.Code != 401 {
		t.Fatalf("usuario desactivado debe perder la sesión, fue %d", rec.Code)
	}
	if _, rec := e.login("rrhh@empresa.com", "Temporal20261x"); rec.Code != 401 {
		t.Fatalf("usuario desactivado no debe poder ingresar, fue %d", rec.Code)
	}
	// Auto-bloqueo prohibido
	if rec, _ := c.do("PATCH", "/api/admin/users/"+admin.ID, map[string]any{"role": "viewer"}); rec.Code != 400 {
		t.Fatalf("admin no debe quitarse su propio rol, fue %d", rec.Code)
	}
	// Auditoría registrada
	rec, out = c.do("GET", "/api/admin/audit", nil)
	if rec.Code != 200 || len(out["entries"].([]any)) < 3 {
		t.Fatalf("auditoría: %d %v", rec.Code, out)
	}
}
