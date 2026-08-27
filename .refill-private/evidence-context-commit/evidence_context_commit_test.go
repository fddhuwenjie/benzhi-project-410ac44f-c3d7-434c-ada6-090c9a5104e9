package evidencecontextcommit_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/httpapi"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

type blockingRepository struct {
	storage.Repository
	commitEntered chan struct{}
	releaseCommit chan struct{}
}

func (r *blockingRepository) Commit(c *casework.InterferenceCase, event casework.AuditEvent, requestID string, result any) error {
	r.waitForRelease()
	return r.Repository.Commit(c, event, requestID, result)
}

func (r *blockingRepository) CommitContext(ctx context.Context, c *casework.InterferenceCase, event casework.AuditEvent, requestID string, result any) error {
	r.waitForRelease()
	repository := r.Repository.(interface {
		CommitContext(context.Context, *casework.InterferenceCase, casework.AuditEvent, string, any) error
	})
	return repository.CommitContext(ctx, c, event, requestID, result)
}

func (r *blockingRepository) waitForRelease() {
	close(r.commitEntered)
	<-r.releaseCommit
}

func TestCanceledEvidenceDoesNotCrossCommitBoundary(t *testing.T) {
	base, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	item := casework.PlanItem{
		ID:               "plan-item-1",
		MeasurementSite:  "site-a",
		EquipmentID:      "spectrum-1",
		TimeWindow:       casework.ObservationWindow{Start: now, End: now.Add(time.Hour)},
		Owner:            "engineer:owner",
		FrequencyRangeHz: [2]float64{100, 200},
	}
	original := &casework.InterferenceCase{
		ID:                "case-cancel-evidence",
		State:             casework.StatePlanned,
		Revision:          3,
		CreatedAt:         now.Add(-time.Hour),
		ObservationWindow: casework.ObservationWindow{Start: now.Add(-time.Hour), End: now.Add(2 * time.Hour)},
		FrequencyRangeHz:  [2]float64{100, 200},
		AntennaID:         "ANT-1",
		Plan:              &casework.InvestigationPlan{ID: "plan-1", CaseID: "case-cancel-evidence", Revision: 1, Items: []casework.PlanItem{item}},
	}
	if err := base.PutCase(original); err != nil {
		t.Fatal(err)
	}

	repo := &blockingRepository{
		Repository:    base,
		commitEntered: make(chan struct{}),
		releaseCommit: make(chan struct{}),
	}
	handler := httpapi.New(service.New(repo)).Handler()
	body := fmt.Sprintf(`{"id":"evidence-1","plan_item_id":"plan-item-1","kind":"spectrum","captured_at":%q,"source_device":"spectrum-1","content_hash":%q}`, now.Add(time.Minute).Format(time.RFC3339Nano), strings.Repeat("a", 64))
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/interference-cases/case-cancel-evidence/evidence", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set(httpapi.HeaderActor, "engineer:collector")
	request.Header.Set(httpapi.HeaderRequestID, "request-cancel-evidence")
	request.Header.Set(httpapi.HeaderExpectedRevision, "3")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	<-repo.commitEntered
	cancel()
	close(repo.releaseCommit)
	<-done

	stored, ok := base.GetCase(original.ID)
	if !ok {
		t.Fatal("案件在取消后丢失")
	}
	if stored.Revision != original.Revision || len(stored.Evidence) != 0 || len(base.Audit(original.ID)) != 0 {
		t.Fatalf("已取消的证据请求越过提交边界: revision=%d evidence=%d audit=%d status=%d", stored.Revision, len(stored.Evidence), len(base.Audit(original.ID)), response.Code)
	}
}
