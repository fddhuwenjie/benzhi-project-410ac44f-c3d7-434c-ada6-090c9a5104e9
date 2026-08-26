package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestReady(t *testing.T) {
	r, _ := storage.Open(t.TempDir())
	s := New(service.New(r))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("readiness 状态为 %d", w.Code)
	}
}

func TestCollectionAndEvidenceWithdrawalRoutes(t *testing.T) {
	r, _ := storage.Open(t.TempDir())
	now := time.Now().UTC()
	item := casework.PlanItem{ID: "pi-1", MeasurementSite: "S", EquipmentID: "D", TimeWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)}, Owner: "engineer:o", FrequencyRangeHz: [2]float64{100, 200}}
	c := &casework.InterferenceCase{ID: "case-1", State: casework.StatePlanned, Revision: 3, CreatedAt: now, ObservationWindow: casework.ObservationWindow{Start: now.Add(-time.Hour), End: now.Add(2 * time.Hour)}, FrequencyRangeHz: [2]float64{100, 200}, AntennaID: "A", Severity: "HIGH", Plan: &casework.InvestigationPlan{ID: "plan-1", CaseID: "case-1", Revision: 1, Items: []casework.PlanItem{item}}, Evidence: []casework.EvidenceRecord{{ID: "ev-1", CaseID: "case-1", PlanItemID: "pi-1", Kind: casework.EvidenceSpectrum, CapturedAt: now.Add(time.Minute), SourceDevice: "D", ContentHash: strings.Repeat("1", 64)}}, EvidenceCoverage: casework.EvidenceCoverage{PlanItems: map[string][]string{"pi-1": {casework.EvidenceSpectrum}}, Gaps: []casework.EvidenceGap{{PlanItemID: "pi-1", MissingKinds: []string{casework.EvidenceReading, casework.EvidenceObservation}}}}}
	if err := r.PutCase(c); err != nil {
		t.Fatal(err)
	}
	s := New(service.New(r))
	list := httptest.NewRecorder()
	s.Handler().ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/interference-cases?state=PLANNED&page_size=1", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("集合查询状态为 %d: %s", list.Code, list.Body.String())
	}
	var page service.CaseListPage
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Items[0].CaseID != c.ID || page.Items[0].NextAction != "evidence" {
		t.Fatalf("集合查询响应不正确: %v %s", err, list.Body.String())
	}
	withdraw := httptest.NewRequest(http.MethodPost, "/api/v1/interference-cases/case-1/evidence", strings.NewReader(`{"action":"WITHDRAW","evidence_ids":["ev-1"],"correction_reason":"设备标注更正"}`))
	withdraw.Header.Set(HeaderActor, "engineer:collector")
	withdraw.Header.Set(HeaderRequestID, "withdraw-http")
	withdraw.Header.Set(HeaderExpectedRevision, "3")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, withdraw)
	if w.Code != http.StatusOK {
		t.Fatalf("撤回命令状态为 %d: %s", w.Code, w.Body.String())
	}
	stored, _ := r.GetCase(c.ID)
	if stored.Revision != 4 || len(stored.Evidence) != 0 || len(stored.EvidenceWithdrawals) != 1 || len(r.Audit(c.ID)) != 1 {
		t.Fatalf("撤回命令未原子保存: %#v", stored)
	}
}
