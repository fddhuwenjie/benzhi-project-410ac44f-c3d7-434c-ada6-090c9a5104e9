package audit_integrity_stale_cache_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/httpapi"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestAuditIntegrityCacheInvalidation(t *testing.T) {
	dir := t.TempDir()
	repo, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	created, err := service.New(repo).Create(service.Meta{
		Actor:     "duty:operator",
		RequestID: "create-for-integrity-cache",
	}, service.CreateInput{
		ObservationWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)},
		FrequencyRangeHz:  [2]float64{1.40e9, 1.41e9},
		AntennaID:         "ANT-CACHE",
		InitialFeature:    "稳定窄带载波",
	})
	if err != nil {
		t.Fatal(err)
	}

	handler := httpapi.New(service.New(repo)).Handler()
	first := auditResponse(t, handler, created.ID)
	if first.Integrity.Status != "OK" {
		t.Fatalf("首次完整性状态为 %q", first.Integrity.Status)
	}

	f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("{\"truncated_audit_event\":\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	second := auditResponse(t, handler, created.ID)
	if second.Integrity.Status != "ANOMALY" || second.Integrity.Continuous || second.Integrity.DigestsValid {
		t.Fatalf("审计文件失效后仍复用缓存结论: %+v", second.Integrity)
	}
}

func auditResponse(t *testing.T, handler http.Handler, caseID string) service.AuditPage {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/interference-cases/"+caseID+"/audit", strings.NewReader(""))
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("审计查询状态为 %d: %s", w.Code, w.Body.String())
	}
	var page service.AuditPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}
