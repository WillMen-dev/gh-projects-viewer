package github

import (
	"context"
	"fmt"
	"time"
)

// DemoProvider entrega datos ficticios para probar la aplicación sin token de GitHub.
// Se activa con GITHUB_MODE=demo. Nunca usar en producción.
type DemoProvider struct {
	projects []Project
	details  map[string]*ProjectDetail
}

func NewDemoProvider() *DemoProvider {
	now := time.Now().UTC()
	day := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }

	people := map[string]Person{
		"ana":    {Login: "ana-dev", Name: "Ana Torres"},
		"carlos": {Login: "carlos-be", Name: "Carlos Ruiz"},
		"laura":  {Login: "laura-fe", Name: "Laura Gómez"},
		"pedro":  {Login: "pedro-qa", Name: "Pedro Díaz"},
	}
	statuses := []string{"Todo", "In Progress", "In Review", "Done"}
	priorities := []string{"P0", "P1", "P2"}

	type seed struct {
		title, status, prio, est string
		who                      []string
		start, due               int
		typ                      string
		labels                   []string
	}
	dpv := &DemoProvider{details: map[string]*ProjectDetail{}}
	mk := func(id string, num int, title, desc string, seeds []seed) {
		p := Project{
			ID: id, Number: num, Title: title, ShortDescription: desc,
			URL: fmt.Sprintf("https://github.com/orgs/demo-org/projects/%d", num), UpdatedAt: now.Add(-time.Duration(num) * time.Hour),
		}
		d := &ProjectDetail{Project: p, StatusOptions: statuses, PriorityOption: priorities, FetchedAt: now, Tasks: []Task{}}
		for i, s := range seeds {
			t := Task{
				ID: fmt.Sprintf("%s_item_%d", id, i+1), Type: s.typ, Title: s.title, Number: 100 + i,
				URL: fmt.Sprintf("https://github.com/demo-org/repo-%d/issues/%d", num, 100+i), Repository: fmt.Sprintf("demo-org/repo-%d", num),
				State: "OPEN", Status: s.status, Priority: s.prio, Estimate: s.est,
				Assignees: []Person{}, Labels: append([]string{}, s.labels...), Fields: map[string]string{},
				UpdatedAt: now.Add(-time.Duration(i*3) * time.Hour),
			}
			if s.typ == "DRAFT_ISSUE" {
				t.Number, t.URL, t.Repository, t.State = 0, "", "", ""
			}
			if s.status == "Done" && s.typ != "DRAFT_ISSUE" {
				t.State = "CLOSED"
			}
			if s.start != 0 {
				t.StartDate = day(s.start)
			}
			if s.due != 0 {
				t.DueDate = day(s.due)
			}
			for _, w := range s.who {
				t.Assignees = append(t.Assignees, people[w])
			}
			t.Fields["Status"], t.Fields["Priority"] = t.Status, t.Priority
			if t.Estimate != "" {
				t.Fields["Estimate"] = t.Estimate
			}
			if t.StartDate != "" {
				t.Fields["Start date"] = t.StartDate
			}
			if t.DueDate != "" {
				t.Fields["Target date"] = t.DueDate
			}
			d.Tasks = append(d.Tasks, t)
		}
		d.Project.UpdatedAt = d.Tasks[0].UpdatedAt
		dpv.projects = append(dpv.projects, d.Project)
		dpv.details[id] = d
	}

	mk("PVT_demo_portal", 1, "Portal de clientes", "Nueva versión del portal web de clientes", []seed{
		{"Diseñar pantalla de inicio de sesión", "Done", "P1", "3", []string{"laura"}, -20, -12, "ISSUE", []string{"frontend"}},
		{"API de autenticación", "Done", "P0", "5", []string{"carlos"}, -18, -10, "ISSUE", []string{"backend"}},
		{"Integrar pasarela de pagos", "In Progress", "P0", "8", []string{"carlos", "ana"}, -6, 4, "ISSUE", []string{"backend", "pagos"}},
		{"Pantalla de historial de pedidos", "In Progress", "P1", "5", []string{"laura"}, -4, -1, "ISSUE", []string{"frontend"}},
		{"Pruebas de carga del checkout", "Todo", "P1", "3", []string{"pedro"}, 3, 10, "ISSUE", []string{"qa"}},
		{"Revisión de accesibilidad", "Todo", "P2", "2", nil, 0, 15, "DRAFT_ISSUE", nil},
		{"Corregir error de sesión expirada", "In Review", "P0", "1", []string{"ana"}, -2, 1, "PULL_REQUEST", []string{"bug"}},
		{"Documentar endpoints públicos", "Todo", "P2", "2", []string{"carlos"}, 0, 0, "ISSUE", []string{"docs"}},
	})
	mk("PVT_demo_rrhh", 2, "Sistema interno RRHH", "Gestión de solicitudes internas", []seed{
		{"Formulario de solicitud de vacaciones", "Done", "P1", "3", []string{"laura"}, -30, -22, "ISSUE", []string{"frontend"}},
		{"Flujo de aprobación por jefe inmediato", "In Progress", "P0", "8", []string{"ana"}, -10, -3, "ISSUE", []string{"backend"}},
		{"Reporte mensual de ausencias", "Todo", "P1", "5", []string{"carlos"}, 2, 14, "ISSUE", []string{"reportes"}},
		{"Notificaciones por correo", "Todo", "P2", "3", nil, 0, 0, "DRAFT_ISSUE", nil},
		{"Pruebas de regresión", "In Review", "P1", "2", []string{"pedro"}, -1, 2, "ISSUE", []string{"qa"}},
	})
	mk("PVT_demo_infra", 3, "Infraestructura 2026", "Migración de servidores y monitoreo", []seed{
		{"Inventario de servidores actuales", "Done", "P1", "2", []string{"pedro"}, -40, -35, "ISSUE", []string{"infra"}},
		{"Configurar monitoreo centralizado", "In Progress", "P1", "5", []string{"ana"}, -5, 6, "ISSUE", []string{"infra"}},
		{"Plan de respaldo y recuperación", "Todo", "P0", "5", []string{"carlos"}, 1, 9, "ISSUE", []string{"infra", "seguridad"}},
		{"Renovar certificados TLS", "Done", "P0", "1", []string{"pedro"}, -8, -7, "PULL_REQUEST", []string{"seguridad"}},
	})
	return dpv
}

func (d *DemoProvider) ListProjects(context.Context) ([]Project, error) {
	return d.projects, nil
}

func (d *DemoProvider) GetProject(_ context.Context, id string) (*ProjectDetail, error) {
	p, ok := d.details[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	cp.FetchedAt = time.Now().UTC()
	return &cp, nil
}
