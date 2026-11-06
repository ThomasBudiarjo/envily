package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/thomasbudiarjo/envily/internal/service"
)

type actorDTO struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Label string `json:"label"`
}

func toActorDTO(a service.Actor) actorDTO { return actorDTO{Type: a.Type, ID: a.ID, Label: a.Label} }

type secretDTO struct {
	Key        string   `json:"key"`
	Version    int64    `json:"version"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt"`
	UpdatedBy  actorDTO `json:"updatedBy"`
	HasComment bool     `json:"hasComment"`
	Value      *string  `json:"value,omitempty"`
	Comment    *string  `json:"comment,omitempty"`
}

func toSecretDTO(s service.SecretItem) secretDTO {
	return secretDTO{Key: s.Key, Version: s.Version, CreatedAt: isoTime(s.CreatedAt), UpdatedAt: isoTime(s.UpdatedAt),
		UpdatedBy: toActorDTO(s.UpdatedBy), HasComment: s.HasComment, Value: s.Value, Comment: s.Comment}
}

func scope(r *http.Request) (project, env string) { return r.PathValue("p"), r.PathValue("env") }

func (h *Handler) listSecrets(w http.ResponseWriter, r *http.Request) error {
	p, e := scope(r)
	list, err := h.svc.ListSecrets(r.Context(), principal(r), p, e, r.URL.Query().Get("values") == "true")
	if err != nil {
		return err
	}
	out := make([]secretDTO, len(list))
	for i, s := range list {
		out[i] = toSecretDTO(s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) getSecret(w http.ResponseWriter, r *http.Request) error {
	p, e := scope(r)
	s, err := h.svc.GetSecret(r.Context(), principal(r), p, e, r.PathValue("key"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toSecretDTO(s))
	return nil
}

type upsertBody struct {
	Key             string  `json:"key"`
	Value           *string `json:"value"`
	Comment         *string `json:"comment"`
	ExpectedVersion *int64  `json:"expectedVersion"`
}

func (b upsertBody) toUpsert() (service.Upsert, error) {
	if b.Value == nil {
		return service.Upsert{}, badRequest("value is required")
	}
	return service.Upsert{Key: b.Key, Value: *b.Value, Comment: b.Comment, ExpectedVersion: b.ExpectedVersion}, nil
}

func (h *Handler) putSecret(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Value           *string `json:"value"`
		Comment         *string `json:"comment"`
		ExpectedVersion *int64  `json:"expectedVersion"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	u, err := upsertBody{Key: r.PathValue("key"), Value: body.Value, Comment: body.Comment, ExpectedVersion: body.ExpectedVersion}.toUpsert()
	if err != nil {
		return err
	}
	p, e := scope(r)
	s, created, err := h.svc.PutSecret(r.Context(), principal(r), p, e, u)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, toSecretDTO(s))
	return nil
}

func (h *Handler) renameSecret(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		NewKey string `json:"newKey"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	p, e := scope(r)
	if err := h.svc.RenameSecret(r.Context(), principal(r), p, e, r.PathValue("key"), body.NewKey); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) deleteSecret(w http.ResponseWriter, r *http.Request) error {
	p, e := scope(r)
	if err := h.svc.DeleteSecret(r.Context(), principal(r), p, e, r.PathValue("key")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) batchSecrets(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Upserts []upsertBody `json:"upserts"`
		Deletes []string     `json:"deletes"`
		Renames []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"renames"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	in := service.BatchInput{Deletes: body.Deletes}
	for _, u := range body.Upserts {
		up, err := u.toUpsert()
		if err != nil {
			return err
		}
		in.Upserts = append(in.Upserts, up)
	}
	for _, rn := range body.Renames {
		in.Renames = append(in.Renames, service.Rename{From: rn.From, To: rn.To})
	}
	p, e := scope(r)
	res, err := h.svc.Batch(r.Context(), principal(r), p, e, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": res.Created, "updated": res.Updated, "deleted": res.Deleted, "renamed": res.Renamed})
	return nil
}

func (h *Handler) importSecrets(w http.ResponseWriter, r *http.Request) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errTooLarge
		}
		return badRequest("cannot read request body")
	}
	entries, err := service.ParseImport(raw)
	if err != nil {
		return err
	}
	p, e := scope(r)
	res, err := h.svc.Import(r.Context(), principal(r), p, e, entries, r.URL.Query().Get("mode"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": res.Created, "updated": res.Updated,
		"unchanged": res.Unchanged, "deleted": res.Deleted})
	return nil
}

var exportTypes = map[string]string{
	"dotenv": "text/plain; charset=utf-8",
	"json":   "application/json; charset=utf-8",
	"yaml":   "application/yaml; charset=utf-8",
}

var exportExt = map[string]string{"dotenv": ".env", "json": ".json", "yaml": ".yaml"}

func (h *Handler) exportSecrets(w http.ResponseWriter, r *http.Request) error {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "dotenv"
	}
	p, e := scope(r)
	out, err := h.svc.Export(r.Context(), principal(r), p, e, format)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", exportTypes[format])
	w.Header().Set("Content-Disposition", `attachment; filename="`+p+"-"+e+exportExt[format]+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out) //nolint:gosec // G705: attachment with a non-HTML type and nosniff
	return nil
}

func (h *Handler) secretVersions(w http.ResponseWriter, r *http.Request) error {
	p, e := scope(r)
	list, err := h.svc.SecretVersions(r.Context(), principal(r), p, e, r.PathValue("key"), r.URL.Query().Get("values") == "true")
	if err != nil {
		return err
	}
	type versionDTO struct {
		Version   int64    `json:"version"`
		CreatedAt string   `json:"createdAt"`
		Actor     actorDTO `json:"actor"`
		Value     *string  `json:"value,omitempty"`
		Comment   *string  `json:"comment,omitempty"`
	}
	out := make([]versionDTO, len(list))
	for i, v := range list {
		out[i] = versionDTO{Version: v.Version, CreatedAt: isoTime(v.CreatedAt), Actor: toActorDTO(v.Actor), Value: v.Value, Comment: v.Comment}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
	return nil
}

func (h *Handler) rollbackSecret(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Version int64 `json:"version"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	p, e := scope(r)
	v, err := h.svc.Rollback(r.Context(), principal(r), p, e, r.PathValue("key"), body.Version)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]int64{"version": v})
	return nil
}

func (h *Handler) compare(w http.ResponseWriter, r *http.Request) error {
	var envs []string
	if v := r.URL.Query().Get("envs"); v != "" {
		envs = strings.Split(v, ",")
	}
	c, err := h.svc.Compare(r.Context(), principal(r), r.PathValue("p"), envs)
	if err != nil {
		return err
	}
	type cell struct {
		Exists    bool    `json:"exists"`
		Version   int64   `json:"version,omitempty"`
		UpdatedAt *string `json:"updatedAt,omitempty"`
	}
	keys := map[string]map[string]cell{}
	for k, row := range c.Keys {
		keys[k] = map[string]cell{}
		for env, x := range row {
			cl := cell{Exists: x.Exists, Version: x.Version}
			if x.Exists {
				t := isoTime(x.UpdatedAt)
				cl.UpdatedAt = &t
			}
			keys[k][env] = cl
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": toEnvDTOs(c.Environments), "keys": keys})
	return nil
}
