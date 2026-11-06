package service

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// AuditQuery filters an audit log listing. Cursor is the opaque value from a
// previous page.
type AuditQuery struct {
	Action, Actor string
	From, To      time.Time
	Limit         int
	Cursor        string
}

type AuditPage struct {
	Items      []store.AuditLog
	NextCursor string // empty on the last page
}

func encodeCursor(c *store.AuditCursor) string {
	if c == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(store.Millis(c.At), 10) + ":" + c.ID))
}

func decodeCursor(s string) (*store.AuditCursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, invalid("cursor", "is invalid")
	}
	at, id, ok := strings.Cut(string(raw), ":")
	ms, perr := strconv.ParseInt(at, 10, 64)
	if !ok || perr != nil || id == "" {
		return nil, invalid("cursor", "is invalid")
	}
	return &store.AuditCursor{At: store.FromMillis(ms), ID: id}, nil
}

// ClampLimit applies the API's pagination limits (default 50, max 200).
func ClampLimit(n int) int {
	switch {
	case n <= 0:
		return 50
	case n > 200:
		return 200
	}
	return n
}

func (s *Service) listAudit(ctx context.Context, projectID string, q AuditQuery) (AuditPage, error) {
	after, err := decodeCursor(q.Cursor)
	if err != nil {
		return AuditPage{}, err
	}
	items, next, err := s.db.Audit().List(ctx, store.AuditFilter{
		ProjectID: projectID, Action: q.Action, ActorID: q.Actor, From: q.From, To: q.To,
		After: after, Limit: ClampLimit(q.Limit),
	})
	return AuditPage{Items: items, NextCursor: encodeCursor(next)}, err
}

// InstanceAudit lists the whole instance's audit log (admins only).
func (s *Service) InstanceAudit(ctx context.Context, p *authz.Principal, q AuditQuery) (AuditPage, error) {
	if err := requireAdmin(p); err != nil {
		return AuditPage{}, err
	}
	return s.listAudit(ctx, "", q)
}

// PruneAudit deletes audit rows older than the retention period. It is a
// no-op when retention is 0 (keep forever).
func (s *Service) PruneAudit(ctx context.Context) (int64, error) {
	if s.opts.AuditRetentionDays <= 0 {
		return 0, nil
	}
	return s.db.Audit().DeleteBefore(ctx, s.now().AddDate(0, 0, -s.opts.AuditRetentionDays))
}

// RunMaintenance prunes the audit log and expired sessions once a day until
// ctx is cancelled.
func (s *Service) RunMaintenance(ctx context.Context) {
	run := func() {
		if n, err := s.PruneAudit(ctx); err != nil {
			s.log.Error("audit retention failed", "error", err)
		} else if n > 0 {
			s.log.Info("pruned audit log", "rows", n)
		}
		if err := s.db.Sessions().DeleteExpired(ctx, s.now()); err != nil {
			s.log.Error("session cleanup failed", "error", err)
		}
	}
	run()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
