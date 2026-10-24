package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var mapping = FieldMapping{
	Status:    []string{"Status"},
	Priority:  []string{"Priority"},
	Estimate:  []string{"Estimate"},
	StartDate: []string{"Start date"},
	DueDate:   []string{"Target date"},
}

const projectsResp = `{"data":{"repositoryOwner":{"login":"acme","name":"Acme S.A.","projectsV2":{"pageInfo":{"hasNextPage":false,"endCursor":null},
 "nodes":[{"id":"PVT_1","number":1,"title":"Proyecto A","shortDescription":"desc","url":"https://github.com/orgs/acme/projects/1","closed":false,"updatedAt":"2026-10-01T10:00:00Z"}]}}}}`

const itemsResp = `{"data":{"node":{
 "id":"PVT_1","number":1,"title":"Proyecto A","shortDescription":"","url":"u","closed":false,"updatedAt":"2026-10-01T10:00:00Z",
 "owner":{"login":"acme","name":"Acme S.A."},
 "fields":{"nodes":[{"name":"Status","dataType":"SINGLE_SELECT","options":[{"name":"Todo"},{"name":"Done"}]},
                    {"name":"Priority","dataType":"SINGLE_SELECT","options":[{"name":"P0"},{"name":"P1"}]},
                    {"name":"Title","dataType":"TITLE"}]},
 "items":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[
  {"id":"PVTI_1","type":"ISSUE","updatedAt":"2026-10-02T10:00:00Z",
   "content":{"title":"Login","body":"Formulario de acceso\ncon validación","number":7,"url":"https://github.com/acme/web/issues/7","issueState":"OPEN",
     "repository":{"nameWithOwner":"acme/web"},
     "assignees":{"nodes":[{"login":"ana","name":"Ana","avatarUrl":"https://avatars.githubusercontent.com/u/1"}]},
     "labels":{"nodes":[{"name":"frontend"}]},
     "createdAt":"2026-09-30T08:00:00Z","closedAt":null,
     "parent":{"title":"Épica de acceso","number":1},
     "subIssuesSummary":{"total":4,"completed":1,"percentCompleted":25}},
   "fieldValues":{"nodes":[
     {"text":"Login","field":{"name":"Title"}},
     {"name":"Todo","field":{"name":"Status"}},
     {"name":"P0","field":{"name":"Priority"}},
     {"number":3.5,"field":{"name":"Estimate"}},
     {"date":"2026-10-01","field":{"name":"Start date"}},
     {"date":"2026-10-20","field":{"name":"Target date"}},
     {"title":"Sprint 4","field":{"name":"Iteration"}},
     {"milestone":{"title":"Versión 1.0"},"field":{"name":"Milestone"}},
     {"pullRequests":{"nodes":[{"number":9},{"number":12}]},"field":{"name":"Linked pull requests"}},
     {"reviewers":{"nodes":[{"login":"ana"},{"name":"Equipo QA"}]},"field":{"name":"Reviewers"}},
     {}
   ]}},
  {"id":"PVTI_2","type":"DRAFT_ISSUE","updatedAt":"2026-10-02T10:00:00Z",
   "content":{"title":"Borrador","assignees":{"nodes":[]}},
   "fieldValues":{"nodes":[]}},
  {"id":"PVTI_3","type":"REDACTED","updatedAt":"2026-10-02T10:00:00Z","content":null,"fieldValues":{"nodes":[]}}
 ]}}}}`

func newTestServer(t *testing.T, handler func(query string) (int, string)) (*Client, *int) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("falta token de autorización")
		}
		body, _ := io.ReadAll(r.Body)
		var req gqlRequest
		_ = json.Unmarshal(body, &req)
		if strings.Contains(req.Query, "mutation") {
			t.Errorf("el cliente nunca debe enviar mutaciones")
		}
		code, resp := handler(req.Query)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "secret", []string{"acme"}, false, mapping), &calls
}

func TestListProjects(t *testing.T) {
	c, _ := newTestServer(t, func(string) (int, string) { return 200, projectsResp })
	ps, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != "PVT_1" || ps[0].Title != "Proyecto A" || ps[0].Owner != "acme" || ps[0].OwnerName != "Acme S.A." {
		t.Fatalf("resultado inesperado: %+v", ps)
	}
}

func TestGetProjectMapsFields(t *testing.T) {
	c, _ := newTestServer(t, func(string) (int, string) { return 200, itemsResp })
	d, err := c.GetProject(context.Background(), "PVT_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tasks) != 3 {
		t.Fatalf("esperaba 3 tareas, hay %d", len(d.Tasks))
	}
	tk := d.Tasks[0]
	if tk.Status != "Todo" || tk.Priority != "P0" || tk.Estimate != "3.5" ||
		tk.StartDate != "2026-10-01" || tk.DueDate != "2026-10-20" || tk.Iteration != "Sprint 4" {
		t.Fatalf("mapeo de campos incorrecto: %+v", tk)
	}
	if d.Project.Owner != "acme" || d.Project.OwnerName != "Acme S.A." {
		t.Fatalf("dueño del proyecto incorrecto: %+v", d.Project)
	}
	want := map[string]string{
		"Milestone": "Versión 1.0", "Linked pull requests": "#9, #12", "Reviewers": "ana, Equipo QA",
		"Parent issue": "#1 Épica de acceso", "Sub-issues progress": "1/4 (25 %)", "Created": "2026-09-30", "Updated": "2026-10-02",
	}
	for k, v := range want {
		if tk.Fields[k] != v {
			t.Errorf("campo %q = %q, esperaba %q", k, tk.Fields[k], v)
		}
	}
	if _, ok := tk.Fields["Closed"]; ok {
		t.Error("una tarea abierta no debe tener fecha de cierre")
	}
	if d.FieldRoles["Status"] != "status" || d.FieldRoles["Priority"] != "priority" || d.FieldRoles["Title"] != "" {
		t.Errorf("roles de campos incorrectos: %v", d.FieldRoles)
	}
	if tk.Description != "Formulario de acceso\ncon validación" {
		t.Fatalf("descripción incorrecta: %q", tk.Description)
	}
	if tk.State != "OPEN" {
		t.Fatalf("estado GitHub no mapeado: %q", tk.State)
	}
	if strings.Contains(projectItemsQuery, "url state") {
		t.Fatal("state debe consultarse con alias (conflicto de tipos IssueState/PullRequestState)")
	}
	if tk.Repository != "acme/web" || len(tk.Assignees) != 1 || tk.Assignees[0].Login != "ana" || tk.Labels[0] != "frontend" {
		t.Fatalf("contenido incorrecto: %+v", tk)
	}
	if _, ok := tk.Fields["Title"]; ok {
		t.Fatal("el campo Title no debe duplicarse en Fields")
	}
	if d.Tasks[1].Title != "Borrador" || d.Tasks[2].Title != "(Elemento sin acceso)" {
		t.Fatalf("títulos inesperados: %q %q", d.Tasks[1].Title, d.Tasks[2].Title)
	}
	if strings.Join(d.StatusOptions, ",") != "Todo,Done" || strings.Join(d.PriorityOption, ",") != "P0,P1" {
		t.Fatalf("opciones incorrectas: %v %v", d.StatusOptions, d.PriorityOption)
	}
}

func TestGetProjectRejectsOtherOwner(t *testing.T) {
	other := strings.Replace(itemsResp, `"login":"acme","name":"Acme S.A."`, `"login":"intruso","name":""`, 1)
	c, _ := newTestServer(t, func(string) (int, string) { return 200, other })
	if _, err := c.GetProject(context.Background(), "PVT_1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, obtuvo %v", err)
	}
}

func TestErrors(t *testing.T) {
	c, _ := newTestServer(t, func(string) (int, string) { return 403, `{"message":"API rate limit exceeded"}` })
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("403 por límite => ErrRateLimited, obtuvo %v", err)
	}
	c, _ = newTestServer(t, func(string) (int, string) {
		return 403, `{"message":"Resource protected by organization SAML enforcement."}`
	})
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrPermission) {
		t.Fatalf("403 por SSO => ErrPermission, obtuvo %v", err)
	}
	c, _ = newTestServer(t, func(string) (int, string) {
		return 200, `{"data":{"node":null},"errors":[{"type":"NOT_FOUND","message":"x"}]}`
	})
	if _, err := c.GetProject(context.Background(), "PVT_x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("NOT_FOUND => ErrNotFound, obtuvo %v", err)
	}
	c, _ = newTestServer(t, func(string) (int, string) { return 500, `boom` })
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrUpstream) {
		t.Fatalf("500 => ErrUpstream, obtuvo %v", err)
	}
}

func TestCache(t *testing.T) {
	c, calls := newTestServer(t, func(string) (int, string) { return 200, projectsResp })
	cp := NewCachedProvider(c, time.Minute)
	now := time.Now()
	cp.now = func() time.Time { return now }
	_, _ = cp.ListProjects(context.Background())
	_, _ = cp.ListProjects(context.Background())
	if *calls != 1 {
		t.Fatalf("la caché debería evitar la segunda llamada, hubo %d", *calls)
	}
	now = now.Add(2 * time.Minute)
	_, _ = cp.ListProjects(context.Background())
	if *calls != 2 {
		t.Fatalf("tras expirar debe volver a consultar, hubo %d", *calls)
	}
	cp.Invalidate("PVT_1")
	_, _ = cp.ListProjects(context.Background())
	if *calls != 3 {
		t.Fatalf("Invalidate debe forzar consulta, hubo %d", *calls)
	}
}

func TestDemoProvider(t *testing.T) {
	d := NewDemoProvider()
	ps, _ := d.ListProjects(context.Background())
	if len(ps) != 3 {
		t.Fatalf("demo debe tener 3 proyectos, tiene %d", len(ps))
	}
	det, err := d.GetProject(context.Background(), ps[0].ID)
	if err != nil || len(det.Tasks) == 0 {
		t.Fatal("demo debe devolver tareas")
	}
}

func TestPartialErrorsDoNotBreakProject(t *testing.T) {
	// Una tarea de un repositorio privado sin acceso: GitHub devuelve content=null y un error anidado.
	partial := strings.Replace(itemsResp, `"type":"DRAFT_ISSUE","updatedAt":"2026-10-02T10:00:00Z",
   "content":{"title":"Borrador","assignees":{"nodes":[]}}`, `"type":"ISSUE","updatedAt":"2026-10-02T10:00:00Z",
   "content":null`, 1)
	if partial == itemsResp {
		t.Fatal("el reemplazo del fixture no se aplicó")
	}
	partial = strings.TrimSuffix(strings.TrimSpace(partial), "}") +
		`,"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a node","path":["node","items","nodes",1,"content"]}]}`
	c, _ := newTestServer(t, func(string) (int, string) { return 200, partial })
	d, err := c.GetProject(context.Background(), "PVT_1")
	if err != nil {
		t.Fatalf("un error anidado no debe descartar el proyecto: %v", err)
	}
	if len(d.Tasks) != 3 || d.Tasks[1].Title != "(Elemento sin acceso)" {
		t.Fatalf("tarea sin acceso mal representada: %+v", d.Tasks[1])
	}
	if len(d.FieldNames) != 3 {
		t.Fatalf("debe reportar los nombres de campos: %v", d.FieldNames)
	}
}

func TestAuthAndPermissionErrors(t *testing.T) {
	c, _ := newTestServer(t, func(string) (int, string) { return 401, `{"message":"Bad credentials"}` })
	if _, err := c.Viewer(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 => ErrUnauthorized, obtuvo %v", err)
	}
	c, _ = newTestServer(t, func(string) (int, string) {
		return 200, `{"data":{"organization":null},"errors":[{"type":"INSUFFICIENT_SCOPES","message":"needs read:project","path":["organization"]}]}`
	})
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrPermission) {
		t.Fatalf("INSUFFICIENT_SCOPES => ErrPermission, obtuvo %v", err)
	}
	c, _ = newTestServer(t, func(string) (int, string) { return 200, `{"data":{"viewer":{"login":"wmendoza"}}}` })
	if l, err := c.Viewer(context.Background()); err != nil || l != "wmendoza" {
		t.Fatalf("viewer: %q %v", l, err)
	}
	c, _ = newTestServer(t, func(string) (int, string) { return 200, `{"data":{"repositoryOwner":null}}` })
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("organización inexistente => ErrNotFound, obtuvo %v", err)
	}
}

// Respuestas de un servidor GitHub simulado con dos organizaciones del usuario del token.
func multiOwnerHandler(query string, vars map[string]any) (int, string) {
	switch {
	case strings.Contains(query, "organizations"):
		return 200, `{"data":{"viewer":{"organizations":{"nodes":[{"login":"acme"},{"login":"globex"},{"login":"sso-org"}]}}}}`
	case strings.Contains(query, "repositoryOwner"):
		switch vars["login"] {
		case "acme":
			return 200, projectsResp
		case "globex":
			return 200, `{"data":{"repositoryOwner":{"login":"globex","name":"Globex","projectsV2":{"pageInfo":{"hasNextPage":false,"endCursor":null},
 "nodes":[{"id":"PVT_2","number":4,"title":"Proyecto G","shortDescription":"","url":"u","closed":false,"updatedAt":"2026-10-01T10:00:00Z"}]}}}}`
		default: // organización que exige SSO: no debe tumbar a las demás
			return 403, `{"message":"Resource protected by organization SAML enforcement."}`
		}
	case strings.Contains(query, "node(id"):
		if vars["id"] == "PVT_2" {
			return 200, strings.Replace(itemsResp, `"login":"acme","name":"Acme S.A."`, `"login":"globex","name":"Globex"`, 1)
		}
		return 200, strings.Replace(itemsResp, `"login":"acme","name":"Acme S.A."`, `"login":"intruso","name":""`, 1)
	}
	return 500, `boom`
}

func newMultiOwnerClient(t *testing.T, owners []string, auto bool) (*Client, *int) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req gqlRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		code, resp := multiOwnerHandler(req.Query, req.Variables)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "secret", owners, auto, mapping), &calls
}

func TestAutoDiscoversOrganizations(t *testing.T) {
	c, calls := newMultiOwnerClient(t, nil, true)
	ps, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("una organización con SSO no debe impedir listar las demás: %v", err)
	}
	if len(ps) != 2 || ps[0].Owner != "acme" || ps[1].Owner != "globex" || ps[1].OwnerName != "Globex" {
		t.Fatalf("proyectos inesperados: %+v", ps)
	}
	before := *calls
	if _, err := c.GetProject(context.Background(), "PVT_2"); err != nil {
		t.Fatalf("proyecto de una organización descubierta: %v", err)
	}
	if *calls != before+1 {
		t.Fatalf("las organizaciones deben reutilizarse desde la caché, hubo %d llamadas", *calls-before)
	}
	if _, err := c.GetProject(context.Background(), "PVT_otro"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("un proyecto de un dueño ajeno debe rechazarse, obtuvo %v", err)
	}
}

func TestExplicitOwnersList(t *testing.T) {
	c, _ := newMultiOwnerClient(t, []string{"acme", "globex"}, false)
	ps, err := c.ListProjects(context.Background())
	if err != nil || len(ps) != 2 {
		t.Fatalf("lista de dueños: %+v %v", ps, err)
	}
	c, _ = newMultiOwnerClient(t, []string{"sso-org"}, false)
	if _, err := c.ListProjects(context.Background()); !errors.Is(err, ErrPermission) {
		t.Fatalf("si ningún dueño responde debe devolverse el error, obtuvo %v", err)
	}
}
