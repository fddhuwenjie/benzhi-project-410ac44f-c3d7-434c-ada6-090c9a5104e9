package storage

import "github.com/benzhi/relay-survey/internal/casework"

func ValidateAudit(events []casework.AuditEvent) bool {
	for i, e := range events {
		if e.Revision != i+1 || e.PayloadDigest != AuditDigest(e) {
			return false
		}
	}
	return true
}
