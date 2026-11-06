package httpapi

import (
	"net/http"
	"time"

	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store"
)

type serviceTokenDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Permission   string   `json:"permission"`
	Environments []string `json:"environments"`
	CreatedAt    string   `json:"createdAt"`
	ExpiresAt    *string  `json:"expiresAt"`
	LastUsedAt   *string  `json:"lastUsedAt"`
	RevokedAt    *string  `json:"revokedAt"`
}

func toServiceTokenDTO(v service.ServiceTokenView) serviceTokenDTO {
	t := v.Token
	d := serviceTokenDTO{ID: t.ID, Name: t.Name, Permission: string(t.Permission), Environments: []string{},
		CreatedAt: isoTime(t.CreatedAt), ExpiresAt: isoPtr(t.ExpiresAt), LastUsedAt: isoPtr(t.LastUsedAt), RevokedAt: isoPtr(t.RevokedAt)}
	for _, e := range v.Environments {
		d.Environments = append(d.Environments, e.Slug)
	}
	return d
}

func (h *Handler) listServiceTokens(w http.ResponseWriter, r *http.Request) error {
	list, err := h.svc.ListServiceTokens(r.Context(), principal(r), r.PathValue("p"))
	if err != nil {
		return err
	}
	out := make([]serviceTokenDTO, len(list))
	for i, v := range list {
		out[i] = toServiceTokenDTO(v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) createServiceToken(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name         string     `json:"name"`
		Environments []string   `json:"environments"`
		Permission   string     `json:"permission"`
		ExpiresAt    *time.Time `json:"expiresAt"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	tok, v, err := h.svc.CreateServiceToken(r.Context(), principal(r), r.PathValue("p"), service.CreateServiceTokenInput{
		Name: body.Name, Environments: body.Environments, Permission: store.Permission(body.Permission), ExpiresAt: body.ExpiresAt})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": tok, "info": toServiceTokenDTO(v)})
	return nil
}

func (h *Handler) revokeServiceToken(w http.ResponseWriter, r *http.Request) error {
	if err := h.svc.RevokeServiceToken(r.Context(), principal(r), r.PathValue("p"), r.PathValue("id")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) tokenInfo(w http.ResponseWriter, r *http.Request) error {
	info, err := h.svc.TokenInfo(r.Context(), principal(r))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         info.Name,
		"project":      map[string]string{"id": info.Project.ID, "name": info.Project.Name, "slug": info.Project.Slug},
		"environments": toEnvDTOs(info.Environments),
		"permission":   string(info.Permission),
		"expiresAt":    isoPtr(info.ExpiresAt),
	})
	return nil
}
