package audit_writer_rotation_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestAuditWriterReopensAfterAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	repo, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(repo)
	now := time.Now().UTC()
	c, err := svc.Create(service.Meta{Actor: "duty:registrar", RequestID: "rotation-create"}, service.CreateInput{
		ObservationWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)},
		FrequencyRangeHz:  [2]float64{100, 200},
		AntennaID:         "ANT-ROTATION",
		InitialFeature:    "受控脉冲",
	})
	if err != nil {
		t.Fatal(err)
	}

	auditPath := filepath.Join(dir, "audit.jsonl")
	contents, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(dir, "audit.recovered")
	if err := os.WriteFile(replacement, contents, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, auditPath); err != nil {
		t.Fatal(err)
	}

	c, err = svc.Triage(service.Meta{Actor: "duty:leader", RequestID: "rotation-triage", ExpectedRevision: c.Revision}, c.ID, casework.Triage{
		AffectedObservations: 4,
		OccupiedBandwidthHz:  25,
		PersistenceMinutes:   30,
		Rationale:            "持续影响排程观测",
	})
	if err != nil {
		t.Fatalf("替换审计文件后的写请求意外失败: %v", err)
	}
	if c.Revision != 2 {
		t.Fatalf("研判修订号为 %d，期望 2", c.Revision)
	}

	reopened, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := reopened.GetCase(c.ID)
	if !ok || stored.Revision != 2 {
		t.Fatalf("重启后的快照未保存研判: %#v", stored)
	}
	events := reopened.Audit(c.ID)
	if len(events) != 2 || events[1].Revision != 2 || events[1].EventType != "CASE_TRIAGED" {
		t.Fatalf("TestAuditWriterReopensAfterAtomicReplacement: 重启后审计丢失已成功提交的修订: %#v", events)
	}
}
