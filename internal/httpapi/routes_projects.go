package httpapi

import (
	"net/http"

	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store"
)

type environmentDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Position  int64  `json:"position"`
	CreatedAt string `json:"createdAt"`
}

func toEnvDTO(e store.Environment) environmentDTO {
	return environmentDTO{ID: e.ID, Name: e.Name, Slug: e.Slug, Position: e.Position, CreatedAt: isoTime(e.CreatedAt)}
}

func toEnvDTOs(list []store.Environment) []environmentDTO {
	out := make([]environmentDTO, len(list))
	for i, e := range list {
		out[i] = toEnvDTO(e)
	}
	return out
}

type projectDTO struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Slug         string           `json:"slug"`
	Role         *string          `json:"role"` // null when not a member
	CreatedAt    string           `json:"createdAt"`
	UpdatedAt    string           `json:"updatedAt"`
	Environments []environmentDTO `json:"environments"`
}

func toProjectDTO(v service.ProjectView) projectDTO {
	d := projectDTO{ID: v.Project.ID, Name: v.Project.Name, Slug: v.Project.Slug, CreatedAt: isoTime(v.Project.CreatedAt),
		UpdatedAt: isoTime(v.Project.UpdatedAt), Environments: toEnvDTOs(v.Environments)}
	if v.Role != "" {
		r := string(v.Role)
		d.Role = &r
	}
	return d
}

type memberDTO struct {
	UserID    string `json:"userId"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func toMemberDTO(m store.Member) memberDTO {
	return memberDTO{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: string(m.Role), CreatedAt: isoTime(m.CreatedAt)}
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListProjects(r.Context(), principal(r))
	if err != nil {
		return err
	}
	out := make([]projectDTO, len(list))
	for i, v := range list {
		out[i] = toProjectDTO(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Name, Slug string }
	if err := decode(r, &body); err != nil {
		return err
	}
	v, err := h.svc.CreateProject(r.Context(), principal(r), service.CreateProjectInput{Name: body.Name, Slug: body.Slug})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, toProjectDTO(v))
	return nil
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) error {
	v, err := h.svc.GetProject(r.Context(), principal(r), r.PathValue("p"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toProjectDTO(v))
	return nil
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Name, Slug *string }
	if err := decode(r, &body); err != nil {
		return err
	}
	v, err := h.svc.UpdateProject(r.Context(), principal(r), r.PathValue("p"), service.UpdateProjectInput{Name: body.Name, Slug: body.Slug})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toProjectDTO(v))
	return nil
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Confirm string }
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := h.svc.DeleteProject(r.Context(), principal(r), r.PathValue("p"), body.Confirm); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListMembers(r.Context(), principal(r), r.PathValue("p"))
	if err != nil {
		return err
	}
	out := make([]memberDTO, len(list))
	for i, m := range list {
		out[i] = toMemberDTO(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) error {
	var body struct{ UserID, Email, Role string }
	if err := decode(r, &body); err != nil {
		return err
	}
	m, err := h.svc.AddMember(r.Context(), principal(r), r.PathValue("p"),
		service.AddMemberInput{UserID: body.UserID, Email: body.Email, Role: store.Role(body.Role)})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, toMemberDTO(m))
	return nil
}

func (h *Handler) updateMember(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Role string }
	if err := decode(r, &body); err != nil {
		return err
	}
	m, err := h.svc.UpdateMember(r.Context(), principal(r), r.PathValue("p"), r.PathValue("userId"), store.Role(body.Role))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]string{"userId": m.UserID, "role": string(m.Role)})
	return nil
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) error {
	if err := h.svc.RemoveMember(r.Context(), principal(r), r.PathValue("p"), r.PathValue("userId")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listEnvironments(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListEnvironments(r.Context(), principal(r), r.PathValue("p"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": toEnvDTOs(list)})
	return nil
}

func (h *Handler) createEnvironment(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Name, Slug string }
	if err := decode(r, &body); err != nil {
		return err
	}
	e, err := h.svc.CreateEnvironment(r.Context(), principal(r), r.PathValue("p"), body.Name, body.Slug)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, toEnvDTO(e))
	return nil
}

func (h *Handler) updateEnvironment(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name, Slug *string
		Position   *int64
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	e, err := h.svc.UpdateEnvironment(r.Context(), principal(r), r.PathValue("p"), r.PathValue("env"),
		service.UpdateEnvironmentInput{Name: body.Name, Slug: body.Slug, Position: body.Position})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toEnvDTO(e))
	return nil
}

func (h *Handler) deleteEnvironment(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Confirm string }
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := h.svc.DeleteEnvironment(r.Context(), principal(r), r.PathValue("p"), r.PathValue("env"), body.Confirm); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) projectAudit(w http.ResponseWriter, r *http.Request) error {
	q, err := parseAuditQuery(r)
	if err != nil {
		return err
	}
	page, err := h.svc.ProjectAudit(r.Context(), principal(r), r.PathValue("p"), q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, auditPage(page))
	return nil
}
