package storage

import "github.com/benzhi/relay-survey/internal/casework"

type Repository interface {
	GetCase(string) (*casework.InterferenceCase, bool)
	Cases() []*casework.InterferenceCase
	PutCase(*casework.InterferenceCase) error
	Idempotent(req string) (result []byte, fingerprint string, ok bool)
	SaveResult(req, fingerprint string, v any) error
	AppendAudit(casework.AuditEvent) error
	Commit(c *casework.InterferenceCase, e casework.AuditEvent, req, fingerprint string, result any) error
	Audit(string) []casework.AuditEvent
	AuditIntegrity(string) bool
}
