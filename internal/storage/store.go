package storage

import (
	"encoding/json"
	"errors"
	"github.com/benzhi/relay-survey/internal/casework"
	"os"
	"path/filepath"
	"sync"
)

type snapshot struct {
	Cases   map[string]*casework.InterferenceCase `json:"cases"`
	Results map[string]json.RawMessage            `json:"results"`
}
type Store struct {
	mu    sync.RWMutex
	dir   string
	data  snapshot
	audit []casework.AuditEvent
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &Store{
		dir:  dir,
		data: snapshot{Cases: map[string]*casework.InterferenceCase{}, Results: map[string]json.RawMessage{}},
	}
	b, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if err == nil {
		if json.Unmarshal(b, &s.data) != nil {
			return nil, errors.New("快照损坏")
		}
	}
	if s.data.Cases == nil {
		s.data.Cases = map[string]*casework.InterferenceCase{}
	}
	if s.data.Results == nil {
		s.data.Results = map[string]json.RawMessage{}
	}
	ab, err := os.ReadFile(filepath.Join(dir, "audit.jsonl"))
	if err == nil {
		for _, line := range splitLines(ab) {
			var e casework.AuditEvent
			if json.Unmarshal(line, &e) == nil {
				s.audit = append(s.audit, e)
			}
		}
	}
	return s, nil
}
func cloneCase(c *casework.InterferenceCase) *casework.InterferenceCase {
	b, _ := json.Marshal(c)
	var cp casework.InterferenceCase
	_ = json.Unmarshal(b, &cp)
	return &cp
}
func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
func (s *Store) persist() error {
	b, _ := json.MarshalIndent(s.data, "", "  ")
	tmp := filepath.Join(s.dir, "snapshot.tmp")
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, "snapshot.json"))
}
func (s *Store) GetCase(id string) (*casework.InterferenceCase, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Cases[id]
	if !ok {
		return nil, false
	}
	b, _ := json.Marshal(c)
	var cp casework.InterferenceCase
	json.Unmarshal(b, &cp)
	return &cp, true
}
func (s *Store) PutCase(c *casework.InterferenceCase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Cases[c.ID] = cloneCase(c)
	return s.persist()
}
func (s *Store) Cases() []*casework.InterferenceCase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*casework.InterferenceCase, 0, len(s.data.Cases))
	for _, c := range s.data.Cases {
		out = append(out, cloneCase(c))
	}
	return out
}
func (s *Store) Idempotent(req string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.data.Results[req]
	return []byte(r), ok
}
func (s *Store) SaveResult(req string, v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(v)
	s.data.Results[req] = b
	return s.persist()
}
func (s *Store) AppendAudit(e casework.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(e)
	f, err := os.OpenFile(filepath.Join(s.dir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	s.audit = append(s.audit, e)
	return nil
}
func (s *Store) Commit(c *casework.InterferenceCase, e casework.AuditEvent, req string, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	oldCase, hadCase := s.data.Cases[c.ID]
	oldResult, hadResult := s.data.Results[req]
	s.data.Cases[c.ID] = cloneCase(c)
	s.data.Results[req] = resultBytes
	if err = s.persist(); err != nil {
		if hadCase {
			s.data.Cases[c.ID] = oldCase
		} else {
			delete(s.data.Cases, c.ID)
		}
		if hadResult {
			s.data.Results[req] = oldResult
		} else {
			delete(s.data.Results, req)
		}
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.dir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	s.audit = append(s.audit, e)
	return nil
}
func (s *Store) Audit(id string) []casework.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []casework.AuditEvent{}
	for _, e := range s.audit {
		if e.CaseID == id {
			out = append(out, e)
		}
	}
	return out
}
func (s *Store) AuditIntegrity(id string) bool {
	dir := s.dir
	if b, err := os.ReadFile(filepath.Join(dir, "audit.jsonl")); err == nil {
		events := []casework.AuditEvent{}
		for _, line := range splitLines(b) {
			var e casework.AuditEvent
			if json.Unmarshal(line, &e) != nil {
				return false
			}
			if e.CaseID == id {
				events = append(events, e)
			}
		}
		if !ValidateAudit(events) {
			return false
		}
		return matchesInMemoryAudit(events, s.Audit(id))
	}
	return len(s.Audit(id)) == 0
}

func matchesInMemoryAudit(disk, mem []casework.AuditEvent) bool {
	if len(disk) != len(mem) {
		return false
	}
	for i := range disk {
		if disk[i].Revision != mem[i].Revision || disk[i].EventID != mem[i].EventID || disk[i].PayloadDigest != mem[i].PayloadDigest {
			return false
		}
	}
	return true
}
