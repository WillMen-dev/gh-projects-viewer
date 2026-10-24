package github

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Estructuras que reflejan la respuesta GraphQL cruda.

type rawPeople struct {
	Nodes []*Person `json:"nodes"`
}

type rawLabels struct {
	Nodes []*struct {
		Name string `json:"name"`
	} `json:"nodes"`
}

type rawContent struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	Number     int    `json:"number"`
	URL        string `json:"url"`
	IssueState string `json:"issueState"`
	PRState    string `json:"prState"`
	Repository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Assignees *rawPeople `json:"assignees"`
	Labels    *rawLabels `json:"labels"`
	CreatedAt string     `json:"createdAt"`
	ClosedAt  string     `json:"closedAt"`
	Parent    *struct {
		Title  string `json:"title"`
		Number int    `json:"number"`
	} `json:"parent"`
	SubIssuesSummary *struct {
		Total            int `json:"total"`
		Completed        int `json:"completed"`
		PercentCompleted int `json:"percentCompleted"`
	} `json:"subIssuesSummary"`
}

type rawFieldValue struct {
	Name   *string  `json:"name"`
	Text   *string  `json:"text"`
	Number *float64 `json:"number"`
	Date   *string  `json:"date"`
	Title  *string  `json:"title"`
	// Campos propios de GitHub (no creados por el usuario).
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
	PullRequests *struct {
		Nodes []*struct {
			Number int `json:"number"`
		} `json:"nodes"`
	} `json:"pullRequests"`
	Reviewers *struct {
		Nodes []*struct {
			Login string `json:"login"`
			Name  string `json:"name"`
		} `json:"nodes"`
	} `json:"reviewers"`
	Field *struct {
		Name string `json:"name"`
	} `json:"field"`
}

type rawItem struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	Content     *rawContent `json:"content"`
	FieldValues struct {
		Nodes []*rawFieldValue `json:"nodes"`
	} `json:"fieldValues"`
}

type rawProject struct {
	Project
	Owner struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	} `json:"owner"`
	Fields struct {
		Nodes []*struct {
			Name     string `json:"name"`
			DataType string `json:"dataType"`
			Options  []struct {
				Name string `json:"name"`
			} `json:"options"`
		} `json:"nodes"`
	} `json:"fields"`
	Items struct {
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
		Nodes []*rawItem `json:"nodes"`
	} `json:"items"`
}

func matches(name string, aliases []string) bool {
	for _, a := range aliases {
		if strings.EqualFold(strings.TrimSpace(name), a) {
			return true
		}
	}
	return false
}

func mapProjectHeader(p *rawProject, m FieldMapping) *ProjectDetail {
	d := &ProjectDetail{Project: p.Project, Tasks: []Task{}, StatusOptions: []string{}, PriorityOption: []string{}, FieldNames: []string{}, FieldRoles: map[string]string{}}
	for _, f := range p.Fields.Nodes {
		if f != nil && f.Name != "" {
			d.FieldNames = append(d.FieldNames, f.Name)
			if role := fieldRole(f.Name, f.DataType, m); role != "" {
				d.FieldRoles[f.Name] = role
			}
		}
		if f == nil || len(f.Options) == 0 {
			continue
		}
		var opts []string
		for _, o := range f.Options {
			opts = append(opts, o.Name)
		}
		switch {
		case matches(f.Name, m.Status):
			d.StatusOptions = opts
		case matches(f.Name, m.Priority):
			d.PriorityOption = opts
		}
	}
	return d
}

// fieldRole indica qué dato de la tarea representa un campo del proyecto, o "" si es un campo más.
func fieldRole(name, dataType string, m FieldMapping) string {
	switch {
	case matches(name, m.Status):
		return "status"
	case matches(name, m.Priority):
		return "priority"
	case matches(name, m.Estimate):
		return "estimate"
	case matches(name, m.StartDate):
		return "startDate"
	case matches(name, m.DueDate):
		return "dueDate"
	case dataType == "ITERATION":
		return "iteration"
	}
	return ""
}

// datePart devuelve la fecha (YYYY-MM-DD) de una marca de tiempo RFC 3339, o "" si viene vacía.
func datePart(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ""
}

func formatNumber(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func mapItem(it *rawItem, m FieldMapping) Task {
	t := Task{
		ID:        it.ID,
		Type:      it.Type,
		Assignees: []Person{},
		Labels:    []string{},
		Fields:    map[string]string{},
		UpdatedAt: it.UpdatedAt,
	}
	if c := it.Content; c != nil {
		t.Title = c.Title
		t.Description = c.Body
		t.Number = c.Number
		t.URL = c.URL
		t.State = c.IssueState
		if c.PRState != "" {
			t.State = c.PRState
		}
		if c.Repository != nil {
			t.Repository = c.Repository.NameWithOwner
		}
		if c.Assignees != nil {
			for _, p := range c.Assignees.Nodes {
				if p != nil {
					t.Assignees = append(t.Assignees, *p)
				}
			}
		}
		if c.Labels != nil {
			for _, l := range c.Labels.Nodes {
				if l != nil {
					t.Labels = append(t.Labels, l.Name)
				}
			}
		}
		// Campos propios de GitHub que vienen del issue/PR y no de los valores del proyecto.
		if c.Parent != nil {
			t.Fields["Parent issue"] = "#" + strconv.Itoa(c.Parent.Number) + " " + c.Parent.Title
		}
		if s := c.SubIssuesSummary; s != nil && s.Total > 0 {
			t.Fields["Sub-issues progress"] = fmt.Sprintf("%d/%d (%d %%)", s.Completed, s.Total, s.PercentCompleted)
		}
		if d := datePart(c.CreatedAt); d != "" {
			t.Fields["Created"] = d
		}
		if d := datePart(c.ClosedAt); d != "" {
			t.Fields["Closed"] = d
		}
	}
	if !it.UpdatedAt.IsZero() {
		t.Fields["Updated"] = it.UpdatedAt.Format("2006-01-02")
	}
	if t.Title == "" {
		if it.Type == "REDACTED" || (it.Content == nil && it.Type != "DRAFT_ISSUE") {
			t.Title = "(Elemento sin acceso)"
		} else {
			t.Title = "(Sin título)"
		}
	}

	for _, fv := range it.FieldValues.Nodes {
		if fv == nil || fv.Field == nil || fv.Field.Name == "" {
			continue
		}
		var val string
		switch {
		case fv.Name != nil:
			val = *fv.Name
		case fv.Text != nil:
			val = *fv.Text
		case fv.Number != nil:
			val = formatNumber(*fv.Number)
		case fv.Date != nil:
			val = *fv.Date
		case fv.Title != nil:
			val = *fv.Title
		case fv.Milestone != nil:
			val = fv.Milestone.Title
		case fv.PullRequests != nil:
			var prs []string
			for _, pr := range fv.PullRequests.Nodes {
				if pr != nil {
					prs = append(prs, "#"+strconv.Itoa(pr.Number))
				}
			}
			val = strings.Join(prs, ", ")
		case fv.Reviewers != nil:
			var revs []string
			for _, r := range fv.Reviewers.Nodes {
				if r != nil && r.Login != "" {
					revs = append(revs, r.Login)
				} else if r != nil && r.Name != "" {
					revs = append(revs, r.Name)
				}
			}
			val = strings.Join(revs, ", ")
		default:
			continue
		}
		name := fv.Field.Name
		// "Title" es un campo interno de GitHub que repite el título; se omite.
		if strings.EqualFold(name, "Title") {
			continue
		}
		t.Fields[name] = val
		switch {
		case matches(name, m.Status):
			t.Status = val
		case matches(name, m.Priority):
			t.Priority = val
		case matches(name, m.Estimate):
			t.Estimate = val
		case matches(name, m.StartDate):
			t.StartDate = val
		case matches(name, m.DueDate):
			t.DueDate = val
		case fv.Title != nil:
			t.Iteration = val
		}
	}
	return t
}
