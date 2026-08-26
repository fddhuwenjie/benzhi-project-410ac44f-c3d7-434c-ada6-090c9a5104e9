package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/benzhi/relay-survey/internal/casework"
)

func AuditDigest(event casework.AuditEvent) string {
	b, _ := json.Marshal(event.PayloadSummary)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func AuditContinuous(events []casework.AuditEvent) bool {
	if len(events) == 0 {
		return true
	}
	for i := range events {
		if events[i].Revision != i+1 || events[i].PayloadDigest != AuditDigest(events[i]) {
			return false
		}
		if i > 0 && events[i].OccurredAt.Before(events[i-1].OccurredAt) {
			return false
		}
	}
	return true
}
