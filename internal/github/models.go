package github

import (
	"context"
	"time"
)

// Project es la información resumida de un proyecto de GitHub Projects (v2).
type Project struct {
	ID               string    `json:"id"`
	Number           int       `json:"number"`
	Title            string    `json:"title"`
	ShortDescription string    `json:"shortDescription"`
	Owner            string    `json:"owner"`               // login de la organización o usuario dueño
	OwnerName        string    `json:"ownerName,omitempty"` // nombre visible del dueño
	URL              string    `json:"url"`
	Closed           bool      `json:"closed"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// Person es un responsable (assignee) de una tarea.
type Person struct {
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// Task es una tarea (item) del proyecto, normalizada para la interfaz.
type Task struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // ISSUE, PULL_REQUEST, DRAFT_ISSUE, REDACTED
	Title       string            `json:"title"`
	Description string            `json:"description"` // cuerpo del issue/PR/borrador (Markdown)
	Number      int               `json:"number,omitempty"`
	URL         string            `json:"url,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	State       string            `json:"state,omitempty"` // OPEN, CLOSED, MERGED
	Assignees   []Person          `json:"assignees"`
	Labels      []string          `json:"labels"`
	Status      string            `json:"status"`
	Priority    string            `json:"priority"`
	Estimate    string            `json:"estimate"`
	StartDate   string            `json:"startDate"` // YYYY-MM-DD o vacío
	DueDate     string            `json:"dueDate"`   // YYYY-MM-DD o vacío
	Iteration   string            `json:"iteration,omitempty"`
	Fields      map[string]string `json:"fields"` // todos los campos personalizados
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// ProjectDetail agrupa el proyecto, sus tareas y las opciones de los campos.
type ProjectDetail struct {
	Project        Project   `json:"project"`
	Tasks          []Task    `json:"tasks"`
	StatusOptions  []string  `json:"statusOptions"`
	PriorityOption []string  `json:"priorityOptions"`
	FetchedAt      time.Time `json:"fetchedAt"`
	Truncated      bool      `json:"truncated"`
	FieldNames     []string  `json:"fieldNames"` // nombres de todos los campos del proyecto
}

// Provider es la fuente de datos (GitHub real o datos de demostración).
type Provider interface {
	ListProjects(ctx context.Context) ([]Project, error)
	GetProject(ctx context.Context, projectID string) (*ProjectDetail, error)
}

// FieldMapping indica qué nombres de campo de GitHub corresponden a cada concepto.
type FieldMapping struct {
	Status    []string
	Priority  []string
	Estimate  []string
	StartDate []string
	DueDate   []string
}
