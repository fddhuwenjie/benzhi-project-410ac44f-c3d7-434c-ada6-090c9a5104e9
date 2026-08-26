package casework

import (
	"testing"
	"time"
)

func TestSeverityAndVerification(t *testing.T) {
	if Severity(Triage{AffectedObservations: 6, OccupiedBandwidthHz: 2e6, PersistenceMinutes: 80}) != "CRITICAL" {
		t.Fatal("严重度计算错误")
	}
	v := MitigationVerification{Thresholds: map[string]float64{"noise": 10}, ObservedMetrics: map[string]float64{"noise": 3}}
	if !VerificationPass(v) {
		t.Fatal("合格复测被拒绝")
	}
}

func TestAggregateFlow(t *testing.T) {
	now := time.Now().UTC()
	c, err := NewCase("c1", "a1", [2]float64{1, 2}, ObservationWindow{Start: now.Add(-time.Hour), End: now.Add(time.Hour)}, "burst")
	if err != nil || c.State != StateDetected {
		t.Fatal(err)
	}
	if err := c.Triage(Triage{AffectedObservations: 1, OccupiedBandwidthHz: 1, PersistenceMinutes: 1, Rationale: "影响"}); err != nil {
		t.Fatal(err)
	}
}
