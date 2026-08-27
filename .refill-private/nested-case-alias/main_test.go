package nestedcasealias

import (
	"testing"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/service"
	"github.com/benzhi/relay-survey/internal/storage"
)

func TestFailedConfirmationDoesNotMutateStoredCandidate(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(store)
	seed := &casework.InterferenceCase{
		ID:       "case-alias",
		State:    casework.StateEvidence,
		Revision: 1,
		SourceCandidates: []casework.SourceCandidate{{
			ID:              "candidate-1",
			CandidateSource: "射电发射机",
			Decision:        "PENDING",
		}},
	}
	if err := store.PutCase(seed); err != nil {
		t.Fatal(err)
	}

	_, err = svc.HypothesisAction(service.Meta{
		Actor:            "investigator:reviewer",
		RequestID:        "confirm-invalid",
		ExpectedRevision: 1,
	}, "case-alias", service.HypothesisCommand{
		Action:         "CONFIRM",
		CandidateID:    "candidate-1",
		ExclusionNotes: "确认前留下的说明",
	})
	if err == nil {
		t.Fatal("缺少受控测试的来源确认不应成功")
	}

	got, ok := store.GetCase("case-alias")
	if !ok {
		t.Fatal("案件被意外删除")
	}
	if got.Revision != 1 || len(store.Audit("case-alias")) != 0 {
		t.Fatalf("失败请求不应改变修订或审计: revision=%d audit=%d", got.Revision, len(store.Audit("case-alias")))
	}
	if got.SourceCandidates[0].ExclusionNotes != "" {
		t.Fatalf("失败确认泄漏了候选修改: %q", got.SourceCandidates[0].ExclusionNotes)
	}
}
