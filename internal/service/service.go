package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/storage"
)

func newID() string                   { return fmt.Sprintf("%d", time.Now().UnixNano()) }
func prefixedID(prefix string) string { return prefix + "-" + newID() }

type Service struct {
	repo storage.Repository
	mu   sync.Mutex
}

func New(r storage.Repository) *Service { return &Service{repo: r} }

type Meta struct {
	Actor            string
	RequestID        string
	ExpectedRevision int
}

func (s *Service) check(m Meta) error {
	if strings.TrimSpace(m.Actor) == "" || strings.TrimSpace(m.RequestID) == "" {
		return casework.Invalid("缺少 actor 或 request_id")
	}
	return nil
}
func requireRole(actor string, role casework.Role) error {
	got, name := casework.ParseActorRole(actor)
	if got != role || strings.TrimSpace(name) == "" {
		return casework.Forbidden(fmt.Sprintf("仅 %s 角色可执行此操作", role))
	}
	return nil
}
func decodeResult(raw []byte) (*casework.InterferenceCase, error) {
	var c casework.InterferenceCase
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
func eventSummary(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func makeEvent(m Meta, c *casework.InterferenceCase, from casework.State, eventType string, summary any) casework.AuditEvent {
	e := casework.AuditEvent{EventID: prefixedID("evt"), CaseID: c.ID, RequestID: m.RequestID, Actor: m.Actor, EventType: eventType, FromState: from, ToState: c.State, Revision: c.Revision, OccurredAt: time.Now().UTC(), PayloadSummary: eventSummary(summary)}
	e.PayloadDigest = storage.AuditDigest(e)
	return e
}
func (s *Service) mutate(m Meta, id, eventType string, fn func(*casework.InterferenceCase) error, summary func(*casework.InterferenceCase) any) (*casework.InterferenceCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(m); err != nil {
		return nil, err
	}
	if raw, ok := s.repo.Idempotent(m.RequestID); ok {
		return decodeResult(raw)
	}
	c, ok := s.repo.GetCase(id)
	if !ok {
		return nil, casework.NotFound(id)
	}
	if c.State == casework.StateClosed {
		return nil, casework.Terminal("已关闭案件不可修改")
	}
	if m.ExpectedRevision <= 0 {
		return nil, casework.Invalid("已有案件写操作必须提供 X-Expected-Revision")
	}
	if c.Revision != m.ExpectedRevision {
		return nil, casework.Conflict(fmt.Sprintf("期望修订号 %d，实际为 %d", m.ExpectedRevision, c.Revision))
	}
	from := c.State
	if err := fn(c); err != nil {
		return nil, err
	}
	payload := any(map[string]any{"revision": c.Revision})
	if summary != nil {
		payload = summary(c)
	}
	e := makeEvent(m, c, from, eventType, payload)
	if err := s.repo.Commit(c, e, m.RequestID, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) Get(id string) (*casework.InterferenceCase, error) {
	c, ok := s.repo.GetCase(id)
	if !ok {
		return nil, casework.NotFound(id)
	}
	return c, nil
}
