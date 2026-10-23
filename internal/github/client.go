package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Límites defensivos para no descargar volúmenes desproporcionados.
const (
	maxProjects      = 100 // por dueño
	ownersTTL        = 5 * time.Minute
	maxItems         = 2000
	itemsPageSize    = 100
	maxResponseBytes = 20 << 20 // 20 MB
)

var (
	ErrNotFound     = errors.New("proyecto no encontrado")
	ErrUpstream     = errors.New("error consultando GitHub")
	ErrRateLimited  = errors.New("límite de consultas de GitHub alcanzado")
	ErrUnauthorized = errors.New("el token de GitHub no es válido o expiró")
	ErrPermission   = errors.New("el token de GitHub no tiene permisos suficientes")
)

// Client consulta la API GraphQL de GitHub en modo SOLO LECTURA.
// Solo ejecuta las consultas definidas en este archivo (no hay mutaciones).
type Client struct {
	apiURL  string
	token   string
	owners  []string // dueños configurados (organizaciones o usuarios)
	auto    bool     // además, descubrir las organizaciones del usuario del token
	mapping FieldMapping
	http    *http.Client

	mu            sync.Mutex
	lastScopes    string
	autoOwners    []string
	autoOwnersExp time.Time
}

// NewClient crea el cliente. Con auto=true se agregan a owners las organizaciones
// a las que pertenece la cuenta dueña del token.
func NewClient(apiURL, token string, owners []string, auto bool, mapping FieldMapping) *Client {
	return &Client{
		apiURL:  apiURL,
		token:   token,
		owners:  owners,
		auto:    auto,
		mapping: mapping,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type gqlError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Path    []any  `json:"path"`
}

func (c *Client) do(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-projects-viewer")

	resp, err := c.http.Do(req)
	if err != nil {
		slog.Error("github: fallo de red", "err", err)
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if s := resp.Header.Get("X-OAuth-Scopes"); s != "" || resp.Header.Get("X-Accepted-OAuth-Scopes") != "" {
		c.mu.Lock()
		c.lastScopes = s
		c.mu.Unlock()
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return ErrUpstream
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		slog.Error("github: token inválido o expirado (401)")
		return ErrUnauthorized
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		var body struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &body)
		msg := strings.ToLower(body.Message)
		if resp.StatusCode == http.StatusTooManyRequests || resp.Header.Get("X-RateLimit-Remaining") == "0" ||
			strings.Contains(msg, "rate limit") {
			slog.Warn("github: límite de consultas alcanzado", "status", resp.StatusCode)
			return ErrRateLimited
		}
		// Por ejemplo: la organización exige SSO/SAML o no permite tokens personales.
		slog.Error("github: acceso denegado", "status", resp.StatusCode, "msg", body.Message)
		return fmt.Errorf("%w: %s", ErrPermission, body.Message)
	case resp.StatusCode != http.StatusOK:
		// No se registra el cuerpo completo para evitar filtrar datos; solo el código.
		slog.Error("github: respuesta inesperada", "status", resp.StatusCode)
		return fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ErrUpstream
	}
	for _, e := range envelope.Errors {
		// Errores "anidados" (p. ej. una tarea de un repositorio privado sin acceso) son parciales:
		// se registran y se continúa con el resto de la información.
		if len(e.Path) > 1 {
			slog.Warn("github: dato parcial omitido", "type", e.Type, "msg", e.Message)
			continue
		}
		slog.Error("github: error GraphQL", "type", e.Type, "msg", e.Message)
		switch e.Type {
		case "NOT_FOUND":
			return fmt.Errorf("%w: %s", ErrNotFound, e.Message)
		case "RATE_LIMITED":
			return ErrRateLimited
		case "INSUFFICIENT_SCOPES", "FORBIDDEN":
			return fmt.Errorf("%w: %s", ErrPermission, e.Message)
		default:
			return fmt.Errorf("%w: %s", ErrUpstream, e.Message)
		}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return ErrUpstream
	}
	return json.Unmarshal(envelope.Data, out)
}

// Scopes devuelve los permisos del token clásico informados por GitHub (vacío en tokens fine-grained).
func (c *Client) Scopes() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastScopes
}

// Viewer devuelve el usuario dueño del token (sirve para verificar la conexión).
func (c *Client) Viewer(ctx context.Context) (string, error) {
	var data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.do(ctx, `query { viewer { login } }`, map[string]any{}, &data); err != nil {
		return "", err
	}
	return data.Viewer.Login, nil
}

// Owners devuelve los dueños (organizaciones o usuarios) cuyos proyectos se muestran.
// En modo automático incluye las organizaciones de la cuenta del token (se guardan unos minutos).
func (c *Client) Owners(ctx context.Context) ([]string, error) {
	if !c.auto {
		return c.owners, nil
	}
	c.mu.Lock()
	if c.autoOwners != nil && time.Now().Before(c.autoOwnersExp) {
		v := c.autoOwners
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	var data struct {
		Viewer struct {
			Organizations struct {
				Nodes []*struct {
					Login string `json:"login"`
				} `json:"nodes"`
			} `json:"organizations"`
		} `json:"viewer"`
	}
	if err := c.do(ctx, `query { viewer { organizations(first: 100) { nodes { login } } } }`, map[string]any{}, &data); err != nil {
		return nil, err
	}
	owners := append([]string{}, c.owners...)
	for _, o := range data.Viewer.Organizations.Nodes {
		if o != nil && o.Login != "" && !containsFold(owners, o.Login) {
			owners = append(owners, o.Login)
		}
	}
	c.mu.Lock()
	c.autoOwners, c.autoOwnersExp = owners, time.Now().Add(ownersTTL)
	c.mu.Unlock()
	return owners, nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// repositoryOwner resuelve tanto organizaciones como usuarios, así no hace falta indicar el tipo.
const projectsQuery = `query($login: String!, $cursor: String) {
  repositoryOwner(login: $login) {
    login
    ... on Organization { name }
    ... on User { name }
    ... on ProjectV2Owner {
      projectsV2(first: 50, after: $cursor, orderBy: {field: UPDATED_AT, direction: DESC}) {
        pageInfo { hasNextPage endCursor }
        nodes { id number title shortDescription url closed updatedAt }
      }
    }
  }
}`

// ListProjects devuelve los proyectos de todos los dueños configurados.
// Si un dueño falla (p. ej. la organización exige SSO) se omite y se continúa con los demás;
// solo se devuelve error si ninguno respondió.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	owners, err := c.Owners(ctx)
	if err != nil {
		return nil, err
	}
	all := []Project{}
	var firstErr error
	answered := 0
	for _, o := range owners {
		ps, err := c.ListOwnerProjects(ctx, o)
		if err != nil {
			slog.Warn("github: no se pudieron listar los proyectos de un dueño", "owner", o, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered++
		all = append(all, ps...)
	}
	if answered == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

// ListOwnerProjects devuelve los proyectos de una organización o usuario.
func (c *Client) ListOwnerProjects(ctx context.Context, owner string) ([]Project, error) {
	var all []Project
	var cursor *string
	for len(all) < maxProjects {
		var data struct {
			RepositoryOwner *struct {
				Login      string `json:"login"`
				Name       string `json:"name"`
				ProjectsV2 struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []*Project `json:"nodes"`
				} `json:"projectsV2"`
			} `json:"repositoryOwner"`
		}
		vars := map[string]any{"login": owner, "cursor": cursor}
		if err := c.do(ctx, projectsQuery, vars, &data); err != nil {
			return nil, err
		}
		ro := data.RepositoryOwner
		if ro == nil {
			return nil, fmt.Errorf("%w: %q no existe o el token no tiene acceso", ErrNotFound, owner)
		}
		for _, p := range ro.ProjectsV2.Nodes {
			if p != nil {
				p.Owner, p.OwnerName = ro.Login, ro.Name
				all = append(all, *p)
			}
		}
		if !ro.ProjectsV2.PageInfo.HasNextPage {
			break
		}
		next := ro.ProjectsV2.PageInfo.EndCursor
		cursor = &next
	}
	return all, nil
}

const projectItemsQuery = `query($id: ID!, $cursor: String) {
  node(id: $id) {
    ... on ProjectV2 {
      id number title shortDescription url closed updatedAt
      owner { ... on Organization { login name } ... on User { login name } }
      fields(first: 50) {
        nodes {
          ... on ProjectV2FieldCommon { name dataType }
          ... on ProjectV2SingleSelectField { name options { name } }
        }
      }
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id type updatedAt
          content {
            ... on Issue {
              title body number url issueState: state
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login name avatarUrl } }
              labels(first: 10) { nodes { name } }
            }
            ... on PullRequest {
              title body number url prState: state
              repository { nameWithOwner }
              assignees(first: 10) { nodes { login name avatarUrl } }
              labels(first: 10) { nodes { name } }
            }
            ... on DraftIssue {
              title body
              assignees(first: 10) { nodes { login name avatarUrl } }
            }
          }
          fieldValues(first: 30) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue { name field { ... on ProjectV2FieldCommon { name } } }
              ... on ProjectV2ItemFieldTextValue { text field { ... on ProjectV2FieldCommon { name } } }
              ... on ProjectV2ItemFieldNumberValue { number field { ... on ProjectV2FieldCommon { name } } }
              ... on ProjectV2ItemFieldDateValue { date field { ... on ProjectV2FieldCommon { name } } }
              ... on ProjectV2ItemFieldIterationValue { title field { ... on ProjectV2FieldCommon { name } } }
            }
          }
        }
      }
    }
  }
}`

// Nota: "state" se consulta con alias porque Issue.state (IssueState) y
// PullRequest.state (PullRequestState) son tipos distintos y GraphQL no permite fusionarlos.

// GetProject obtiene un proyecto con todas sus tareas (paginando).
// Verifica que el proyecto pertenezca a uno de los dueños configurados.
func (c *Client) GetProject(ctx context.Context, projectID string) (*ProjectDetail, error) {
	owners, err := c.Owners(ctx)
	if err != nil {
		return nil, err
	}
	var detail *ProjectDetail
	var cursor *string
	for {
		var data struct {
			Node *rawProject `json:"node"`
		}
		vars := map[string]any{"id": projectID, "cursor": cursor}
		if err := c.do(ctx, projectItemsQuery, vars, &data); err != nil {
			return nil, err
		}
		if data.Node == nil || data.Node.ID == "" {
			return nil, ErrNotFound
		}
		if !containsFold(owners, data.Node.Owner.Login) {
			// Defensa en profundidad: nunca mostrar proyectos de otros dueños.
			return nil, ErrNotFound
		}
		if detail == nil {
			detail = mapProjectHeader(data.Node, c.mapping)
			detail.Project.Owner, detail.Project.OwnerName = data.Node.Owner.Login, data.Node.Owner.Name
		}
		for _, it := range data.Node.Items.Nodes {
			if it != nil {
				detail.Tasks = append(detail.Tasks, mapItem(it, c.mapping))
			}
		}
		if !data.Node.Items.PageInfo.HasNextPage {
			break
		}
		if len(detail.Tasks) >= maxItems {
			detail.Truncated = true
			break
		}
		next := data.Node.Items.PageInfo.EndCursor
		cursor = &next
	}
	detail.FetchedAt = time.Now().UTC()
	return detail, nil
}
