package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wmendoza/gh-projects-viewer/internal/github"
)

// Los IDs de ProjectV2 de GitHub tienen la forma "PVT_xxx". Se valida el formato antes de usarlo.
var projectIDRe = regexp.MustCompile(`^[A-Za-z0-9_\-=]{1,100}$`)

func (s *Server) providerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, github.ErrNotFound):
		slog.Warn("no encontrado en GitHub", "err", err)
		writeError(w, http.StatusNotFound, "not_found", "Proyecto no encontrado")
	case errors.Is(err, github.ErrUnauthorized):
		slog.Error("token de GitHub inválido", "err", err)
		writeError(w, http.StatusBadGateway, "github_token", "El token de GitHub no es válido o expiró. Avise al administrador.")
	case errors.Is(err, github.ErrPermission):
		slog.Error("token de GitHub sin permisos", "err", err)
		writeError(w, http.StatusBadGateway, "github_permission", "El token de GitHub no tiene permisos para leer los proyectos. Avise al administrador.")
	case errors.Is(err, github.ErrRateLimited):
		writeError(w, http.StatusServiceUnavailable, "rate_limited", "GitHub limitó temporalmente las consultas. Intente en unos minutos.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeError(w, http.StatusGatewayTimeout, "timeout", "GitHub tardó demasiado en responder")
	default:
		slog.Error("error del proveedor", "err", err)
		writeError(w, http.StatusBadGateway, "upstream", "No fue posible consultar GitHub")
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	projects, err := s.provider.ListProjects(ctx)
	if err != nil {
		s.providerError(w, err)
		return
	}
	if projects == nil {
		projects = []github.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// projectAllowed confirma que el ID pertenece a la lista de proyectos del dueño configurado.
func (s *Server) projectAllowed(ctx context.Context, id string) (bool, error) {
	projects, err := s.provider.ListProjects(ctx)
	if err != nil {
		return false, err
	}
	for _, p := range projects {
		if p.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !projectIDRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, "bad_request", "Identificador inválido")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	ok, err := s.projectAllowed(ctx, id)
	if err != nil {
		s.providerError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Proyecto no encontrado")
		return
	}
	detail, err := s.provider.GetProject(ctx, id)
	if err != nil {
		s.providerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project":         detail.Project,
		"tasks":           detail.Tasks,
		"statusOptions":   detail.StatusOptions,
		"priorityOptions": detail.PriorityOption,
		"fetchedAt":       detail.FetchedAt,
		"truncated":       detail.Truncated,
		"fieldNames":      detail.FieldNames,
		"fieldRoles":      detail.FieldRoles,
		"summary":         s.summarize(detail.Tasks),
		"doneStatuses":    s.opts.DoneStatuses,
	})
}

func (s *Server) handleRefreshProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !projectIDRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, "bad_request", "Identificador inválido")
		return
	}
	u := userFrom(r)
	if !s.refreshLimiter.Allow(u.ID) {
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "Espere un momento antes de volver a actualizar")
		return
	}
	if s.cache != nil {
		s.cache.Invalidate(id)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- resumen ----------

type countItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Summary struct {
	Total        int         `json:"total"`
	Done         int         `json:"done"`
	Open         int         `json:"open"`
	Overdue      int         `json:"overdue"`
	DueSoon      int         `json:"dueSoon"` // vencen en los próximos 7 días
	Unassigned   int         `json:"unassigned"`
	NoDueDate    int         `json:"noDueDate"`
	ProgressPct  int         `json:"progressPct"`
	ByStatus     []countItem `json:"byStatus"`
	ByPriority   []countItem `json:"byPriority"`
	ByAssignee   []countItem `json:"byAssignee"`
	EstimateDone float64     `json:"estimateDone"`
	EstimateAll  float64     `json:"estimateAll"`
}

func (s *Server) isDone(t github.Task) bool {
	for _, d := range s.opts.DoneStatuses {
		if strings.EqualFold(t.Status, d) {
			return true
		}
	}
	if t.Status == "" && (t.State == "CLOSED" || t.State == "MERGED") {
		return true
	}
	return false
}

func sortedCounts(m map[string]int) []countItem {
	out := make([]countItem, 0, len(m))
	for k, v := range m {
		out = append(out, countItem{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func parseFloat(s string) (float64, bool) {
	var f float64
	var sawDigit bool
	frac := 0.0
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			sawDigit = true
			if frac > 0 {
				f += float64(c-'0') * frac
				frac /= 10
			} else {
				f = f*10 + float64(c-'0')
			}
		case (c == '.' || c == ',') && frac == 0:
			frac = 0.1
		default:
			return 0, false
		}
	}
	return f, sawDigit
}

func (s *Server) summarize(tasks []github.Task) Summary {
	today := s.now().Format("2006-01-02")
	soon := s.now().AddDate(0, 0, 7).Format("2006-01-02")
	sum := Summary{Total: len(tasks)}
	byStatus, byPrio, byAssignee := map[string]int{}, map[string]int{}, map[string]int{}
	for _, t := range tasks {
		done := s.isDone(t)
		if done {
			sum.Done++
		} else {
			sum.Open++
		}
		st := t.Status
		if st == "" {
			st = "Sin estado"
		}
		byStatus[st]++
		pr := t.Priority
		if pr == "" {
			pr = "Sin prioridad"
		}
		byPrio[pr]++
		if len(t.Assignees) == 0 {
			sum.Unassigned++
			byAssignee["Sin responsable"]++
		}
		for _, a := range t.Assignees {
			byAssignee[a.Login]++
		}
		if t.DueDate == "" {
			sum.NoDueDate++
		} else if !done {
			if t.DueDate < today {
				sum.Overdue++
			} else if t.DueDate <= soon {
				sum.DueSoon++
			}
		}
		if f, ok := parseFloat(t.Estimate); ok {
			sum.EstimateAll += f
			if done {
				sum.EstimateDone += f
			}
		}
	}
	if sum.Total > 0 {
		sum.ProgressPct = sum.Done * 100 / sum.Total
	}
	sum.ByStatus, sum.ByPriority, sum.ByAssignee = sortedCounts(byStatus), sortedCounts(byPrio), sortedCounts(byAssignee)
	return sum
}
