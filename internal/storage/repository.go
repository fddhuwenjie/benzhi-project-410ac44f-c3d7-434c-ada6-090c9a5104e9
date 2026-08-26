package storage

import "github.com/benzhi/relay-survey/internal/casework"

type Repository interface {
	GetCase(string) (*casework.InterferenceCase, bool)
	Cases() []*casework.InterferenceCase
	PutCase(*casework.InterferenceCase) error
	Idempotent(string) ([]byte, bool)
	SaveResult(string, any) error
	AppendAudit(casework.AuditEvent) error
	Commit(*casework.InterferenceCase, casework.AuditEvent, string, any) error
	Audit(string) []casework.AuditEvent
	AuditIntegrity(string) bool
}
