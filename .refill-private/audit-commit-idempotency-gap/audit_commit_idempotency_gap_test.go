package auditcommitidempotencygap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestAuditWriteFailureDoesNotDuplicateRequest(t *testing.T) {
	dir := t.TempDir()
	repo, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/full", filepath.Join(dir, "audit.jsonl")); err != nil {
		t.Fatal(err)
	}
	svc := service.New(repo)
	now := time.Now().UTC()
	meta := service.Meta{Actor: "duty:registrar", RequestID: "audit-failure-retry"}
	in := service.CreateInput{
		ObservationWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)},
		FrequencyRangeHz:  [2]float64{100, 200},
		AntennaID:         "ANT-AUDIT",
		InitialFeature:    "burst",
	}
	if _, err := svc.Create(meta, in); err == nil {
		t.Fatal("审计介质失效时创建应返回错误")
	}
	ghostCases := len(repo.Cases())
	if err := os.Remove(filepath.Join(dir, "audit.jsonl")); err != nil {
		t.Fatal(err)
	}
	retry, err := svc.Create(meta, in)
	if err != nil {
		t.Fatal(err)
	}
	finalCases := len(repo.Cases())
	if retry == nil || ghostCases != 0 || finalCases != 1 {
		t.Fatalf("审计失败必须原子回滚且重试只创建一次: retry=%v ghost_cases=%d final_cases=%d", retry, ghostCases, finalCases)
	}
}
