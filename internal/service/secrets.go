package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/dotenv"
	"github.com/thomasbudiarjo/envily/internal/store"
)

const (
	maxValueSize   = 64 << 10
	maxCommentSize = 1 << 10
	maxBatchItems  = 1000
	maxVersions    = 100
)

var errDecrypt = newErr(CodeDecryptionFailed, "a stored value could not be decrypted")

// Actor identifies who wrote a secret version.
type Actor struct {
	Type, ID, Label string
}

// SecretItem is a live secret. Value and Comment are set only when values
// were requested.
type SecretItem struct {
	Key        string
	Version    int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	UpdatedBy  Actor
	HasComment bool
	Value      *string
	Comment    *string
}

// ---- encryption helpers (the only callers of crypto.Seal/Open for secrets) ----

// unwrapDEK returns the project's data key; callers must crypto.Zero it.
func (s *Service) unwrapDEK(pc projectCtx) ([]byte, error) {
	dek, err := s.kr.UnwrapDEK(pc.project.ID, pc.project.DEKRootVersion, pc.project.WrappedDEK)
	if err != nil {
		s.log.Error("cannot unwrap project key", "project_id", pc.project.ID)
		return nil, errDecrypt
	}
	return dek, nil
}

func (s *Service) open(dek []byte, ct, aad []byte, secretID string, version int64) (string, error) {
	plain, err := crypto.Open(dek, ct, aad)
	if err != nil {
		s.log.Error("cannot decrypt secret", "secret_id", secretID, "version", version)
		return "", errDecrypt
	}
	return string(plain), nil
}

func (s *Service) openValue(dek []byte, secretID string, version int64, ct []byte) (string, error) {
	return s.open(dek, ct, crypto.SecretAAD(secretID, version), secretID, version)
}

func (s *Service) openComment(dek []byte, secretID string, version int64, ct []byte) (string, error) {
	if ct == nil {
		return "", nil
	}
	return s.open(dek, ct, crypto.CommentAAD(secretID, version), secretID, version)
}

func sealVersion(dek []byte, secretID string, version int64, value, comment string) (valueCT, commentCT []byte, err error) {
	if valueCT, err = crypto.Seal(dek, []byte(value), crypto.SecretAAD(secretID, version)); err != nil {
		return nil, nil, err
	}
	if comment != "" {
		if commentCT, err = crypto.Seal(dek, []byte(comment), crypto.CommentAAD(secretID, version)); err != nil {
			return nil, nil, err
		}
	}
	return valueCT, commentCT, nil
}

// ---- validation ----

func checkKey(v validation, field, key string) {
	if !ValidKey(key) {
		v.add(field, "must start with a letter or underscore and contain only letters, digits, and underscores (max 255)")
	}
}

func checkValue(v validation, field, value string) {
	if len(value) > maxValueSize {
		v.add(field, "must be at most 64 KiB")
	} else if !utf8.ValidString(value) {
		v.add(field, "must be valid UTF-8")
	}
}

func checkComment(v validation, field string, comment *string) {
	if comment != nil && len(*comment) > maxCommentSize {
		v.add(field, "must be at most 1 KiB")
	}
}

// ---- resolution ----

// secretScope resolves project and environment and checks perm.
func (s *Service) secretScope(ctx context.Context, st store.Store, p *authz.Principal, projectSlug, envSlug string, perm authz.Permission) (projectCtx, store.Environment, error) {
	pc, err := s.resolveProject(ctx, st, p, projectSlug)
	if err != nil {
		return pc, store.Environment{}, err
	}
	env, err := s.resolveEnv(ctx, st, p, pc, envSlug)
	if err != nil {
		return pc, env, err
	}
	return pc, env, s.require(p, pc, perm, env.ID)
}

// actorOf is the author recorded on secret versions: personal tokens act as
// their user.
func actorOf(p *authz.Principal) (string, string) {
	if p.Type == authz.ActorPersonalToken {
		return string(authz.ActorUser), p.UserID
	}
	return string(p.Type), p.ID
}

// actorLabels resolves version authors to display labels.
func (s *Service) actorLabels(ctx context.Context, projectID string) (map[string]string, error) {
	labels := map[string]string{}
	users, err := s.db.Users().List(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		labels["user:"+u.ID] = u.Email
	}
	tokens, err := s.db.Tokens().ListService(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, t := range tokens {
		labels["service_token:"+t.ID] = t.Name
	}
	return labels, nil
}

func actor(labels map[string]string, typ, id string) Actor {
	return Actor{Type: typ, ID: id, Label: labels[typ+":"+id]}
}

// ---- reading ----

// ListSecrets lists an environment's live secrets. With values set the
// values and comments are decrypted and the read is audited.
func (s *Service) ListSecrets(ctx context.Context, p *authz.Principal, projectSlug, envSlug string, values bool) ([]SecretItem, error) {
	perm := authz.ViewProject
	if values {
		perm = authz.ReadSecrets
	}
	pc, env, err := s.secretScope(ctx, s.db, p, projectSlug, envSlug, perm)
	if err != nil {
		return nil, err
	}
	live, err := s.db.Secrets().ListLive(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	labels, err := s.actorLabels(ctx, pc.project.ID)
	if err != nil {
		return nil, err
	}
	var dek []byte
	if values {
		if dek, err = s.unwrapDEK(pc); err != nil {
			return nil, err
		}
		defer crypto.Zero(dek)
	}
	out := make([]SecretItem, len(live))
	for i, x := range live {
		out[i] = SecretItem{Key: x.Key, Version: x.CurrentVersion, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt,
			UpdatedBy: actor(labels, x.ActorType, x.ActorID), HasComment: x.CommentCT != nil}
		if values {
			v, err := s.openValue(dek, x.ID, x.CurrentVersion, x.ValueCT)
			if err != nil {
				return nil, err
			}
			c, err := s.openComment(dek, x.ID, x.CurrentVersion, x.CommentCT)
			if err != nil {
				return nil, err
			}
			out[i].Value, out[i].Comment = &v, &c
		}
	}
	if values {
		if err := s.audit(ctx, s.db, p, auditEntry{action: "secrets.read_all", projectID: pc.project.ID, envID: env.ID,
			targetType: "environment", targetID: env.ID, meta: map[string]any{"count": len(out)}}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// GetSecret returns one secret with its value and comment.
func (s *Service) GetSecret(ctx context.Context, p *authz.Principal, projectSlug, envSlug, key string) (SecretItem, error) {
	pc, env, err := s.secretScope(ctx, s.db, p, projectSlug, envSlug, authz.ReadSecrets)
	if err != nil {
		return SecretItem{}, err
	}
	sec, err := s.db.Secrets().GetLive(ctx, env.ID, key)
	if err != nil {
		return SecretItem{}, err
	}
	ver, err := s.db.Secrets().GetVersion(ctx, sec.ID, sec.CurrentVersion)
	if err != nil {
		return SecretItem{}, err
	}
	dek, err := s.unwrapDEK(pc)
	if err != nil {
		return SecretItem{}, err
	}
	defer crypto.Zero(dek)
	value, err := s.openValue(dek, sec.ID, ver.Version, ver.ValueCT)
	if err != nil {
		return SecretItem{}, err
	}
	comment, err := s.openComment(dek, sec.ID, ver.Version, ver.CommentCT)
	if err != nil {
		return SecretItem{}, err
	}
	labels, err := s.actorLabels(ctx, pc.project.ID)
	if err != nil {
		return SecretItem{}, err
	}
	if err := s.audit(ctx, s.db, p, auditEntry{action: "secret.read", projectID: pc.project.ID, envID: env.ID,
		targetType: "secret", targetID: sec.ID, meta: map[string]any{"key": key, "version": sec.CurrentVersion}}); err != nil {
		return SecretItem{}, err
	}
	return SecretItem{Key: sec.Key, Version: sec.CurrentVersion, CreatedAt: sec.CreatedAt, UpdatedAt: sec.UpdatedAt,
		UpdatedBy: actor(labels, ver.ActorType, ver.ActorID), HasComment: ver.CommentCT != nil, Value: &value, Comment: &comment}, nil
}

// ---- writing ----

// Upsert is one secret write. A nil Comment keeps the current comment.
// ExpectedVersion, when set, must equal the current version (0 = must not
// exist yet).
type Upsert struct {
	Key             string
	Value           string
	Comment         *string
	ExpectedVersion *int64
}

type Rename struct{ From, To string }

type BatchInput struct {
	Upserts []Upsert
	Deletes []string
	Renames []Rename
}

type BatchResult struct {
	Created, Updated, Deleted, Renamed int
}

// writer applies secret changes inside one transaction.
type writer struct {
	s         *Service
	ctx       context.Context
	tx        store.Store
	p         *authz.Principal
	pc        projectCtx
	env       store.Environment
	dek       []byte
	now       time.Time
	conflicts map[string]string
}

func (s *Service) withWriter(ctx context.Context, p *authz.Principal, projectSlug, envSlug string, fn func(w *writer) error) error {
	return s.db.WithTx(ctx, func(tx store.Store) error {
		pc, env, err := s.secretScope(ctx, tx, p, projectSlug, envSlug, authz.WriteSecrets)
		if err != nil {
			return err
		}
		dek, err := s.unwrapDEK(pc)
		if err != nil {
			return err
		}
		defer crypto.Zero(dek)
		w := &writer{s: s, ctx: ctx, tx: tx, p: p, pc: pc, env: env, dek: dek, now: s.now(), conflicts: map[string]string{}}
		if err := fn(w); err != nil {
			return err
		}
		if len(w.conflicts) > 0 {
			return &Error{Code: CodeConflict, Message: "some secrets were changed by someone else", Details: w.conflicts}
		}
		return tx.Projects().Touch(ctx, pc.project.ID, w.now)
	})
}

func (w *writer) audit(action, secretID string, meta map[string]any) error {
	return w.s.audit(w.ctx, w.tx, w.p, auditEntry{action: action, projectID: w.pc.project.ID, envID: w.env.ID,
		targetType: "secret", targetID: secretID, meta: meta})
}

// writeVersion stores a new version of sec (already created or updated in
// memory) with the given value and comment.
func (w *writer) writeVersion(sec store.Secret, value, comment string) error {
	valueCT, commentCT, err := sealVersion(w.dek, sec.ID, sec.CurrentVersion, value, comment)
	if err != nil {
		return err
	}
	actorType, actorID := actorOf(w.p)
	if err := w.tx.Secrets().CreateVersion(w.ctx, store.SecretVersion{ID: newID(), SecretID: sec.ID, Version: sec.CurrentVersion,
		ValueCT: valueCT, CommentCT: commentCT, ActorType: actorType, ActorID: actorID, CreatedAt: w.now}); err != nil {
		return err
	}
	if sec.CurrentVersion > maxVersions {
		return w.tx.Secrets().PruneVersions(w.ctx, sec.ID, sec.CurrentVersion-maxVersions+1)
	}
	return nil
}

// current returns the decrypted value and comment of sec's current version.
func (w *writer) current(sec store.Secret) (string, string, error) {
	ver, err := w.tx.Secrets().GetVersion(w.ctx, sec.ID, sec.CurrentVersion)
	if err != nil {
		return "", "", err
	}
	value, err := w.s.openValue(w.dek, sec.ID, ver.Version, ver.ValueCT)
	if err != nil {
		return "", "", err
	}
	comment, err := w.s.openComment(w.dek, sec.ID, ver.Version, ver.CommentCT)
	return value, comment, err
}

// upsert writes one secret. It reports created=true for new secrets and
// changed=false when the value and comment are unchanged (no new version).
func (w *writer) upsert(u Upsert, skipUnchanged bool) (created, changed bool, err error) {
	sec, err := w.tx.Secrets().GetLive(w.ctx, w.env.ID, u.Key)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if u.ExpectedVersion != nil && *u.ExpectedVersion != 0 {
			w.conflicts[u.Key] = "the secret no longer exists"
			return false, false, nil
		}
		sec = store.Secret{ID: newID(), EnvironmentID: w.env.ID, Key: u.Key, CurrentVersion: 1, CreatedAt: w.now, UpdatedAt: w.now}
		if err := w.tx.Secrets().Create(w.ctx, sec); err != nil {
			return false, false, err
		}
		comment := ""
		if u.Comment != nil {
			comment = *u.Comment
		}
		if err := w.writeVersion(sec, u.Value, comment); err != nil {
			return false, false, err
		}
		return true, true, w.audit("secret.created", sec.ID, map[string]any{"key": u.Key, "version": 1})
	case err != nil:
		return false, false, err
	}
	if u.ExpectedVersion != nil && *u.ExpectedVersion != sec.CurrentVersion {
		w.conflicts[u.Key] = fmt.Sprintf("expected version %d, current version is %d", *u.ExpectedVersion, sec.CurrentVersion)
		return false, false, nil
	}
	oldValue, oldComment, err := w.current(sec)
	if err != nil {
		return false, false, err
	}
	comment := oldComment
	if u.Comment != nil {
		comment = *u.Comment
	}
	if skipUnchanged && oldValue == u.Value && oldComment == comment {
		return false, false, nil
	}
	sec.CurrentVersion++
	sec.UpdatedAt = w.now
	if err := w.tx.Secrets().Update(w.ctx, sec); err != nil {
		return false, false, err
	}
	if err := w.writeVersion(sec, u.Value, comment); err != nil {
		return false, false, err
	}
	return false, true, w.audit("secret.updated", sec.ID, map[string]any{"key": u.Key, "version": sec.CurrentVersion})
}

func (w *writer) rename(from, to string) error {
	sec, err := w.tx.Secrets().GetLive(w.ctx, w.env.ID, from)
	if err != nil {
		return err
	}
	sec.Key, sec.UpdatedAt = to, w.now
	if err := w.tx.Secrets().Update(w.ctx, sec); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return &Error{Code: CodeConflict, Message: "a secret with that key already exists", Details: map[string]string{to: "already exists"}}
		}
		return err
	}
	return w.audit("secret.renamed", sec.ID, map[string]any{"from": from, "to": to, "version": sec.CurrentVersion})
}

func (w *writer) remove(key string) error {
	sec, err := w.tx.Secrets().GetLive(w.ctx, w.env.ID, key)
	if err != nil {
		return err
	}
	sec.DeletedAt, sec.UpdatedAt = &w.now, w.now
	if err := w.tx.Secrets().Update(w.ctx, sec); err != nil {
		return err
	}
	return w.audit("secret.deleted", sec.ID, map[string]any{"key": key, "version": sec.CurrentVersion})
}

func validateUpsert(v validation, field string, u Upsert) {
	checkKey(v, field+".key", u.Key)
	checkValue(v, field+".value", u.Value)
	checkComment(v, field+".comment", u.Comment)
}

// PutSecret creates or updates one secret and returns its new state.
func (s *Service) PutSecret(ctx context.Context, p *authz.Principal, projectSlug, envSlug string, u Upsert) (SecretItem, bool, error) {
	v := validation{}
	checkKey(v, "key", u.Key)
	checkValue(v, "value", u.Value)
	checkComment(v, "comment", u.Comment)
	if err := v.err(); err != nil {
		return SecretItem{}, false, err
	}
	var item SecretItem
	var created bool
	err := s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error {
		var err error
		if created, _, err = w.upsert(u, false); err != nil || len(w.conflicts) > 0 {
			return err
		}
		sec, err := w.tx.Secrets().GetLive(ctx, w.env.ID, u.Key)
		if err != nil {
			return err
		}
		actorType, actorID := actorOf(p)
		item = SecretItem{Key: sec.Key, Version: sec.CurrentVersion, CreatedAt: sec.CreatedAt, UpdatedAt: sec.UpdatedAt,
			UpdatedBy: Actor{Type: actorType, ID: actorID, Label: p.Label}}
		return nil
	})
	return item, created, err
}

// RenameSecret changes a secret's key, keeping its versions.
func (s *Service) RenameSecret(ctx context.Context, p *authz.Principal, projectSlug, envSlug, from, to string) error {
	v := validation{}
	checkKey(v, "newKey", to)
	if err := v.err(); err != nil {
		return err
	}
	return s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error { return w.rename(from, to) })
}

// DeleteSecret soft-deletes a secret; its key can be reused.
func (s *Service) DeleteSecret(ctx context.Context, p *authz.Principal, projectSlug, envSlug, key string) error {
	return s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error { return w.remove(key) })
}

// Batch applies deletes, then renames, then upserts, atomically.
func (s *Service) Batch(ctx context.Context, p *authz.Principal, projectSlug, envSlug string, in BatchInput) (BatchResult, error) {
	v := validation{}
	if len(in.Upserts)+len(in.Deletes)+len(in.Renames) > maxBatchItems {
		v.add("batch", fmt.Sprintf("may contain at most %d changes", maxBatchItems))
	}
	if len(in.Upserts)+len(in.Deletes)+len(in.Renames) == 0 {
		v.add("batch", "contains no changes")
	}
	seen := map[string]bool{}
	for i, u := range in.Upserts {
		validateUpsert(v, fmt.Sprintf("upserts[%d]", i), u)
		if seen[u.Key] {
			v.add(fmt.Sprintf("upserts[%d].key", i), "appears more than once")
		}
		seen[u.Key] = true
	}
	for i, r := range in.Renames {
		checkKey(v, fmt.Sprintf("renames[%d].to", i), r.To)
	}
	if err := v.err(); err != nil {
		return BatchResult{}, err
	}
	var res BatchResult
	err := s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error {
		for _, key := range in.Deletes {
			if err := w.remove(key); err != nil {
				return notFoundKey(err, key)
			}
			res.Deleted++
		}
		for _, r := range in.Renames {
			if err := w.rename(r.From, r.To); err != nil {
				return notFoundKey(err, r.From)
			}
			res.Renamed++
		}
		for _, u := range in.Upserts {
			created, changed, err := w.upsert(u, false)
			if err != nil {
				return err
			}
			switch {
			case created:
				res.Created++
			case changed:
				res.Updated++
			}
		}
		return nil
	})
	return res, err
}

func notFoundKey(err error, key string) error {
	if errors.Is(err, store.ErrNotFound) {
		return &Error{Code: CodeNotFound, Message: "secret not found", Details: map[string]string{key: "not found"}}
	}
	return err
}

// ---- import and export ----

type ImportResult struct {
	Created, Updated, Unchanged, Deleted int
}

// ParseImport reads .env text or a JSON object of strings.
func ParseImport(body []byte) ([]dotenv.Entry, error) {
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		var m map[string]any
		if err := json.Unmarshal(trimmed, &m); err != nil {
			return nil, invalid("body", "is not a valid JSON object")
		}
		out := make([]dotenv.Entry, 0, len(m))
		for k, val := range m {
			s, ok := val.(string)
			if !ok {
				return nil, invalid(k, "value must be a string")
			}
			out = append(out, dotenv.Entry{Key: k, Value: s})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
		return out, nil
	}
	entries, err := dotenv.Parse(string(body))
	if err != nil {
		return nil, invalid("body", err.Error())
	}
	return entries, nil
}

// Import writes entries into an environment. In "overwrite" mode, live keys
// missing from the input are deleted; "merge" (the default) keeps them.
func (s *Service) Import(ctx context.Context, p *authz.Principal, projectSlug, envSlug string, entries []dotenv.Entry, mode string) (ImportResult, error) {
	if mode == "" {
		mode = "merge"
	}
	v := validation{}
	if mode != "merge" && mode != "overwrite" {
		v.add("mode", "must be merge or overwrite")
	}
	if len(entries) > maxBatchItems {
		v.add("body", fmt.Sprintf("may contain at most %d secrets", maxBatchItems))
	}
	for _, e := range entries {
		if !ValidKey(e.Key) {
			v.add(e.Key, "is not a valid key")
		} else {
			checkValue(v, e.Key, e.Value)
		}
	}
	if err := v.err(); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult
	err := s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error {
		keep := map[string]bool{}
		for _, e := range entries {
			keep[e.Key] = true
			created, changed, err := w.upsert(Upsert{Key: e.Key, Value: e.Value}, true)
			if err != nil {
				return err
			}
			switch {
			case created:
				res.Created++
			case changed:
				res.Updated++
			default:
				res.Unchanged++
			}
		}
		if mode == "overwrite" {
			live, err := w.tx.Secrets().ListLive(ctx, w.env.ID)
			if err != nil {
				return err
			}
			for _, x := range live {
				if !keep[x.Key] {
					if err := w.remove(x.Key); err != nil {
						return err
					}
					res.Deleted++
				}
			}
		}
		return w.s.audit(ctx, w.tx, p, auditEntry{action: "secrets.imported", projectID: w.pc.project.ID, envID: w.env.ID,
			targetType: "environment", targetID: w.env.ID, meta: map[string]any{"mode": mode, "created": res.Created,
				"updated": res.Updated, "unchanged": res.Unchanged, "deleted": res.Deleted}})
	})
	return res, err
}

// Export renders an environment's secrets as dotenv, json, or yaml.
func (s *Service) Export(ctx context.Context, p *authz.Principal, projectSlug, envSlug, format string) ([]byte, error) {
	if format == "" {
		format = "dotenv"
	}
	if format != "dotenv" && format != "json" && format != "yaml" {
		return nil, invalid("format", "must be dotenv, json, or yaml")
	}
	pc, env, err := s.secretScope(ctx, s.db, p, projectSlug, envSlug, authz.ReadSecrets)
	if err != nil {
		return nil, err
	}
	entries, err := s.liveValues(ctx, pc, env)
	if err != nil {
		return nil, err
	}
	if err := s.audit(ctx, s.db, p, auditEntry{action: "secrets.export", projectID: pc.project.ID, envID: env.ID,
		targetType: "environment", targetID: env.ID, meta: map[string]any{"format": format, "count": len(entries)}}); err != nil {
		return nil, err
	}
	return FormatSecrets(entries, format), nil
}

// FormatSecrets renders key/value pairs (sorted by key) as dotenv, json, or yaml.
func FormatSecrets(entries []dotenv.Entry, format string) []byte {
	switch format {
	case "json":
		m := make(map[string]string, len(entries))
		for _, e := range entries {
			m[e.Key] = e.Value
		}
		b, _ := json.MarshalIndent(m, "", "  ")
		return append(b, '\n')
	case "yaml":
		var b strings.Builder
		for _, e := range entries {
			// A JSON string is a valid YAML double-quoted scalar.
			q, _ := json.Marshal(e.Value)
			fmt.Fprintf(&b, "%s: %s\n", e.Key, q)
		}
		return []byte(b.String())
	}
	return []byte(dotenv.Format(entries))
}

func (s *Service) liveValues(ctx context.Context, pc projectCtx, env store.Environment) ([]dotenv.Entry, error) {
	live, err := s.db.Secrets().ListLive(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	dek, err := s.unwrapDEK(pc)
	if err != nil {
		return nil, err
	}
	defer crypto.Zero(dek)
	out := make([]dotenv.Entry, len(live))
	for i, x := range live {
		v, err := s.openValue(dek, x.ID, x.CurrentVersion, x.ValueCT)
		if err != nil {
			return nil, err
		}
		out[i] = dotenv.Entry{Key: x.Key, Value: v}
	}
	return out, nil
}

// ---- history ----

type VersionItem struct {
	Version   int64
	CreatedAt time.Time
	Actor     Actor
	Value     *string
	Comment   *string
}

// SecretVersions lists a live secret's versions, newest first. Values are
// included only when requested by an editor or owner.
func (s *Service) SecretVersions(ctx context.Context, p *authz.Principal, projectSlug, envSlug, key string, values bool) ([]VersionItem, error) {
	perm := authz.ViewHistory
	if values {
		perm = authz.ReadHistoryValues
	}
	pc, env, err := s.secretScope(ctx, s.db, p, projectSlug, envSlug, perm)
	if err != nil {
		return nil, err
	}
	sec, err := s.db.Secrets().GetLive(ctx, env.ID, key)
	if err != nil {
		return nil, err
	}
	versions, err := s.db.Secrets().ListVersions(ctx, sec.ID)
	if err != nil {
		return nil, err
	}
	labels, err := s.actorLabels(ctx, pc.project.ID)
	if err != nil {
		return nil, err
	}
	var dek []byte
	if values {
		if dek, err = s.unwrapDEK(pc); err != nil {
			return nil, err
		}
		defer crypto.Zero(dek)
	}
	out := make([]VersionItem, len(versions))
	for i, ver := range versions {
		out[i] = VersionItem{Version: ver.Version, CreatedAt: ver.CreatedAt, Actor: actor(labels, ver.ActorType, ver.ActorID)}
		if values {
			val, err := s.openValue(dek, sec.ID, ver.Version, ver.ValueCT)
			if err != nil {
				return nil, err
			}
			c, err := s.openComment(dek, sec.ID, ver.Version, ver.CommentCT)
			if err != nil {
				return nil, err
			}
			out[i].Value, out[i].Comment = &val, &c
		}
	}
	if values {
		if err := s.audit(ctx, s.db, p, auditEntry{action: "secret.read", projectID: pc.project.ID, envID: env.ID,
			targetType: "secret", targetID: sec.ID, meta: map[string]any{"key": key, "history": true}}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Rollback writes a new version holding the value and comment of version.
func (s *Service) Rollback(ctx context.Context, p *authz.Principal, projectSlug, envSlug, key string, version int64) (int64, error) {
	var newVersion int64
	err := s.withWriter(ctx, p, projectSlug, envSlug, func(w *writer) error {
		if err := w.s.require(p, w.pc, authz.Rollback, ""); err != nil {
			return err
		}
		sec, err := w.tx.Secrets().GetLive(ctx, w.env.ID, key)
		if err != nil {
			return err
		}
		old, err := w.tx.Secrets().GetVersion(ctx, sec.ID, version)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("version", "does not exist")
		} else if err != nil {
			return err
		}
		value, err := w.s.openValue(w.dek, sec.ID, old.Version, old.ValueCT)
		if err != nil {
			return err
		}
		comment, err := w.s.openComment(w.dek, sec.ID, old.Version, old.CommentCT)
		if err != nil {
			return err
		}
		sec.CurrentVersion++
		sec.UpdatedAt = w.now
		newVersion = sec.CurrentVersion
		if err := w.tx.Secrets().Update(ctx, sec); err != nil {
			return err
		}
		if err := w.writeVersion(sec, value, comment); err != nil {
			return err
		}
		return w.audit("secret.rolled_back", sec.ID, map[string]any{"key": key, "from": version, "version": newVersion})
	})
	return newVersion, err
}

// ---- compare ----

type CompareCell struct {
	Exists    bool
	Version   int64
	UpdatedAt time.Time
}

type Comparison struct {
	Environments []store.Environment
	Keys         map[string]map[string]CompareCell // key -> env slug -> cell
}

// Compare shows which keys exist in which environments, without values.
// envSlugs selects environments; empty means all visible ones.
func (s *Service) Compare(ctx context.Context, p *authz.Principal, projectSlug string, envSlugs []string) (Comparison, error) {
	view, err := s.GetProject(ctx, p, projectSlug)
	if err != nil {
		return Comparison{}, err
	}
	pc := projectCtx{project: view.Project, role: view.Role}
	if err := s.require(p, pc, authz.ViewProject, ""); err != nil {
		return Comparison{}, err
	}
	envs := view.Environments
	if len(envSlugs) > 0 {
		envs = nil
		for _, slug := range envSlugs {
			i := slices.IndexFunc(view.Environments, func(e store.Environment) bool { return e.Slug == slug })
			if i < 0 {
				return Comparison{}, invalid("envs", "unknown environment "+slug)
			}
			envs = append(envs, view.Environments[i])
		}
	}
	keys, err := s.db.Secrets().ListProjectKeys(ctx, pc.project.ID)
	if err != nil {
		return Comparison{}, err
	}
	slugByID := map[string]string{}
	for _, e := range envs {
		slugByID[e.ID] = e.Slug
	}
	out := Comparison{Environments: envs, Keys: map[string]map[string]CompareCell{}}
	for _, k := range keys {
		slug, ok := slugByID[k.EnvironmentID]
		if !ok {
			continue
		}
		row := out.Keys[k.Key]
		if row == nil {
			row = map[string]CompareCell{}
			for _, e := range envs {
				row[e.Slug] = CompareCell{}
			}
			out.Keys[k.Key] = row
		}
		row[slug] = CompareCell{Exists: true, Version: k.CurrentVersion, UpdatedAt: k.UpdatedAt}
	}
	return out, nil
}
