package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// ---- DTOs ----

func isoTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func isoPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := isoTime(*t)
	return &s
}

type userDTO struct {
	ID                 string  `json:"id"`
	Email              string  `json:"email"`
	Name               string  `json:"name"`
	IsAdmin            bool    `json:"isAdmin"`
	MustChangePassword bool    `json:"mustChangePassword"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
	DisabledAt         *string `json:"disabledAt"`
}

func toUserDTO(u store.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin, MustChangePassword: u.MustChangePassword,
		CreatedAt: isoTime(u.CreatedAt), UpdatedAt: isoTime(u.UpdatedAt), DisabledAt: isoPtr(u.DisabledAt)}
}

type personalTokenDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"createdAt"`
	ExpiresAt  *string `json:"expiresAt"`
	LastUsedAt *string `json:"lastUsedAt"`
	RevokedAt  *string `json:"revokedAt"`
}

func toPersonalTokenDTO(t store.PersonalToken) personalTokenDTO {
	return personalTokenDTO{ID: t.ID, Name: t.Name, CreatedAt: isoTime(t.CreatedAt), ExpiresAt: isoPtr(t.ExpiresAt),
		LastUsedAt: isoPtr(t.LastUsedAt), RevokedAt: isoPtr(t.RevokedAt)}
}

type auditDTO struct {
	ID            string `json:"id"`
	At            string `json:"at"`
	ActorType     string `json:"actorType"`
	ActorID       string `json:"actorId"`
	ActorLabel    string `json:"actorLabel"`
	Action        string `json:"action"`
	ProjectID     string `json:"projectId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
	TargetType    string `json:"targetType"`
	TargetID      string `json:"targetId"`
	Metadata      any    `json:"metadata"`
	IP            string `json:"ip"`
	UserAgent     string `json:"userAgent"`
}

type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) { return []byte(r), nil }

func auditPage(page service.AuditPage) map[string]any {
	items := make([]auditDTO, len(page.Items))
	for i, l := range page.Items {
		items[i] = auditDTO{ID: l.ID, At: isoTime(l.At), ActorType: l.ActorType, ActorID: l.ActorID, ActorLabel: l.ActorLabel,
			Action: l.Action, ProjectID: l.ProjectID, EnvironmentID: l.EnvironmentID, TargetType: l.TargetType,
			TargetID: l.TargetID, Metadata: rawJSON(l.Metadata), IP: l.IP, UserAgent: l.UserAgent}
	}
	var next *string
	if page.NextCursor != "" {
		next = &page.NextCursor
	}
	return map[string]any{"items": items, "nextCursor": next}
}

func parseAuditQuery(r *http.Request) (service.AuditQuery, error) {
	q := r.URL.Query()
	aq := service.AuditQuery{Action: q.Get("action"), Actor: q.Get("actor"), Cursor: q.Get("cursor")}
	if v := q.Get("limit"); v != "" {
		n, err := parseInt(v)
		if err != nil {
			return aq, badRequest("limit must be a number")
		}
		aq.Limit = n
	}
	for name, dst := range map[string]*time.Time{"from": &aq.From, "to": &aq.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return aq, badRequest(name + " must be an RFC 3339 time")
			}
			*dst = t
		}
	}
	return aq, nil
}

// ---- handlers ----

func (h *Handler) setupStatus(w http.ResponseWriter, r *http.Request) error {
	needs, err := h.svc.SetupStatus(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsSetup": needs})
	return nil
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Email, Name, Password, SetupToken string
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if ok, wait := h.limits.loginIP.allow(h.clientIP(r)); !ok {
		return h.rateLimited(w, wait)
	}
	u, token, err := h.svc.Setup(r.Context(), service.SetupInput{Email: body.Email, Name: body.Name,
		Password: body.Password, SetupToken: body.SetupToken})
	if err != nil {
		return err
	}
	h.setSessionCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserDTO(u)})
	return nil
}

func (h *Handler) rateLimited(w http.ResponseWriter, wait time.Duration) error {
	w.Header().Set("Retry-After", retryAfter(wait))
	return &service.Error{Code: service.CodeRateLimited, Message: "too many attempts, try again later"}
}

// loginAllowed applies the login limits: per IP and per email.
func (h *Handler) loginAllowed(w http.ResponseWriter, r *http.Request, email string) error {
	if ok, wait := h.limits.loginIP.allow(h.clientIP(r)); !ok {
		return h.rateLimited(w, wait)
	}
	if ok, wait := h.limits.loginEmail.allow(strings.ToLower(strings.TrimSpace(email))); !ok {
		return h.rateLimited(w, wait)
	}
	return nil
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Email, Password string }
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := h.loginAllowed(w, r, body.Email); err != nil {
		return err
	}
	u, token, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		return err
	}
	h.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(u)})
	return nil
}

func (h *Handler) cliLogin(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Email, Password, TokenName string }
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := h.loginAllowed(w, r, body.Email); err != nil {
		return err
	}
	token, t, err := h.svc.CLILogin(r.Context(), body.Email, body.Password, body.TokenName)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "expiresAt": isoPtr(t.ExpiresAt)})
	return nil
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if err := h.svc.Logout(r.Context(), principal(r)); err != nil {
		return err
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	u, err := h.svc.Me(r.Context(), principal(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserDTO(u), "isAdmin": u.IsAdmin,
		"mustChangePassword": u.MustChangePassword})
	return nil
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) error {
	var body struct{ CurrentPassword, NewPassword string }
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := h.svc.ChangePassword(r.Context(), principal(r), body.CurrentPassword, body.NewPassword); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listMyTokens(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListPersonalTokens(r.Context(), principal(r))
	if err != nil {
		return err
	}
	out := make([]personalTokenDTO, len(list))
	for i, t := range list {
		out[i] = toPersonalTokenDTO(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) createMyToken(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name      string
		ExpiresAt *time.Time
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	token, t, err := h.svc.CreatePersonalToken(r.Context(), principal(r), body.Name, body.ExpiresAt)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "info": toPersonalTokenDTO(t)})
	return nil
}

func (h *Handler) revokeMyToken(w http.ResponseWriter, r *http.Request) error {
	p, id := principal(r), r.PathValue("id")
	if id == "current" {
		if p.Type != authz.ActorPersonalToken {
			return badRequest("only a personal token can revoke itself with \"current\"")
		}
		id = p.ID
	}
	if err := h.svc.RevokePersonalToken(r.Context(), p, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListUsers(r.Context(), principal(r))
	if err != nil {
		return err
	}
	out := make([]userDTO, len(list))
	for i, u := range list {
		out[i] = toUserDTO(u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Email, Name string
		IsAdmin     bool
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u, pw, err := h.svc.CreateUser(r.Context(), principal(r), service.CreateUserInput{Email: body.Email, Name: body.Name, IsAdmin: body.IsAdmin})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserDTO(u), "temporaryPassword": pw})
	return nil
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name          *string
		IsAdmin       *bool
		Disabled      *bool
		ResetPassword bool
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u, pw, err := h.svc.UpdateUser(r.Context(), principal(r), r.PathValue("id"), service.UpdateUserInput{
		Name: body.Name, IsAdmin: body.IsAdmin, Disabled: body.Disabled, ResetPassword: body.ResetPassword})
	if err != nil {
		return err
	}
	resp := map[string]any{"user": toUserDTO(u)}
	if pw != "" {
		resp["temporaryPassword"] = pw
	}
	writeJSON(w, http.StatusOK, resp)
	return nil
}

func (h *Handler) instanceAudit(w http.ResponseWriter, r *http.Request) error {
	q, err := parseAuditQuery(r)
	if err != nil {
		return err
	}
	page, err := h.svc.InstanceAudit(r.Context(), principal(r), q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, auditPage(page))
	return nil
}
