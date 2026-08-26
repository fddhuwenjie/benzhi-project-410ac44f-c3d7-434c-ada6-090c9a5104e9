package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
	"github.com/benzhi/relay-survey/internal/storage"
)

func testService(t *testing.T) (*Service, *storage.Store) {
	t.Helper()
	r, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(r), r
}

func createPlannedCase(t *testing.T, s *Service, requestPrefix, device string, now time.Time) *casework.InterferenceCase {
	t.Helper()
	c, err := s.Create(Meta{Actor: "duty:registrar", RequestID: requestPrefix + "-create"}, CreateInput{ObservationWindow: casework.ObservationWindow{Start: now.Add(-time.Hour), End: now.Add(4 * time.Hour)}, FrequencyRangeHz: [2]float64{100, 200}, AntennaID: "ANT-" + requestPrefix, InitialFeature: "burst"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Triage(Meta{Actor: "duty:leader", RequestID: requestPrefix + "-triage", ExpectedRevision: c.Revision}, c.ID, casework.Triage{AffectedObservations: 5, OccupiedBandwidthHz: 20, PersistenceMinutes: 60, Rationale: "影响观测"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Plan(Meta{Actor: "engineer:planner", RequestID: requestPrefix + "-plan", ExpectedRevision: c.Revision}, c.ID, casework.InvestigationPlan{MeasurementSites: []string{"S1"}, EquipmentIDs: []string{device}, TimeWindow: casework.ObservationWindow{Start: now, End: now.Add(2 * time.Hour)}, Owner: "engineer:owner", StopConditions: []string{"恢复基线"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreateSimilarityAndIdempotency(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	in := CreateInput{ObservationWindow: casework.ObservationWindow{Start: now, End: now.Add(time.Hour)}, FrequencyRangeHz: [2]float64{100, 200}, AntennaID: "ANT-1", InitialFeature: "脉冲 噪声", AssociationDisposition: "INDEPENDENT"}
	first, err := s.Create(Meta{Actor: "duty:a", RequestID: "create-1"}, in)
	if err != nil {
		t.Fatal(err)
	}
	in.ObservationWindow = casework.ObservationWindow{Start: now.Add(time.Hour + 10*time.Minute), End: now.Add(2 * time.Hour)}
	in.FrequencyRangeHz = [2]float64{105, 195}
	second, err := s.Create(Meta{Actor: "duty:b", RequestID: "create-2"}, in)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || len(second.Association.Candidates) != 1 || second.Association.Candidates[0].CaseID != first.ID {
		t.Fatalf("相似候选未关联: %#v", second.Association.Candidates)
	}
	replayed, err := s.Create(Meta{Actor: "duty:b", RequestID: "create-2"}, in)
	if err != nil || replayed.ID != second.ID || len(r.Audit(second.ID)) != 1 {
		t.Fatalf("创建重放不稳定: %v", err)
	}
}

func TestTriageBoundaryAndEvidenceBatchAtomicity(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	c, err := s.Create(Meta{Actor: "duty:a", RequestID: "c"}, CreateInput{ObservationWindow: casework.ObservationWindow{Start: now.Add(-time.Hour), End: now.Add(3 * time.Hour)}, FrequencyRangeHz: [2]float64{100, 110}, AntennaID: "A", InitialFeature: "burst"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Triage(Meta{Actor: "duty:a", RequestID: "bad", ExpectedRevision: 1}, c.ID, casework.Triage{AffectedObservations: 6, OccupiedBandwidthHz: 11, PersistenceMinutes: 90, Rationale: "越界"})
	if err == nil {
		t.Fatal("越界带宽被接受")
	}
	unchanged, _ := r.GetCase(c.ID)
	if unchanged.Revision != 1 || len(r.Audit(c.ID)) != 1 {
		t.Fatal("失败研判改变了快照")
	}
	c, err = s.Triage(Meta{Actor: "duty:a", RequestID: "tri", ExpectedRevision: 1}, c.ID, casework.Triage{AffectedObservations: 6, OccupiedBandwidthHz: 2, PersistenceMinutes: 90, Rationale: "  影响主观测  "})
	if err != nil {
		t.Fatal(err)
	}
	plan := casework.InvestigationPlan{MeasurementSites: []string{"S"}, EquipmentIDs: []string{"D"}, TimeWindow: casework.ObservationWindow{Start: now, End: now.Add(2 * time.Hour)}, Owner: "owner", StopConditions: []string{"恢复"}}
	c, err = s.Plan(Meta{Actor: "engineer:p", RequestID: "plan", ExpectedRevision: c.Revision}, c.ID, plan)
	if err != nil {
		t.Fatal(err)
	}
	item := c.Plan.Items[0].ID
	hash := func(ch string) string { return strings.Repeat(ch, 64) }
	batch := []casework.EvidenceRecord{{ID: "e1", PlanItemID: item, Kind: casework.EvidenceSpectrum, CapturedAt: now.Add(time.Minute), SourceDevice: "D", ContentHash: hash("1")}, {ID: "e2", PlanItemID: item, Kind: casework.EvidenceReading, CapturedAt: now.Add(time.Minute), SourceDevice: "wrong", ContentHash: hash("2")}}
	_, err = s.EvidenceBatch(Meta{Actor: "engineer:e", RequestID: "ev-bad", ExpectedRevision: c.Revision}, c.ID, batch)
	if err == nil {
		t.Fatal("设备不匹配批次被接受")
	}
	unchanged, _ = r.GetCase(c.ID)
	if len(unchanged.Evidence) != 0 || unchanged.Revision != c.Revision {
		t.Fatal("失败批次留下了部分证据")
	}
}

func TestPlanReplacementAndReplay(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	c := createPlannedCase(t, s, "replace", "D1", now)
	oldID, oldItem, oldRevision := c.Plan.ID, c.Plan.Items[0].ID, c.Revision
	replacement := casework.InvestigationPlan{MeasurementSites: []string{"S2"}, EquipmentIDs: []string{"D2"}, TimeWindow: casework.ObservationWindow{Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)}, Owner: "engineer:new", StopConditions: []string{"恢复基线"}, ReplacementReason: "设备排期变化"}
	c, err := s.Plan(Meta{Actor: "engineer:planner", RequestID: "replace-v2", ExpectedRevision: oldRevision}, c.ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != casework.StatePlanned || c.Plan.Revision != 2 || c.Plan.ID == oldID || c.Plan.Items[0].ID == oldItem || c.Plan.EquipmentIDs[0] != "D2" {
		t.Fatalf("改版结果不正确: %#v", c.Plan)
	}
	events := r.Audit(c.ID)
	last := events[len(events)-1]
	if last.EventType != "PLAN_REPLACED" || !strings.Contains(string(last.PayloadSummary), "设备排期变化") || !strings.Contains(string(last.PayloadSummary), "D1") || !strings.Contains(string(last.PayloadSummary), "D2") {
		t.Fatalf("改版审计摘要不完整: %s", last.PayloadSummary)
	}
	replayed, err := s.Plan(Meta{Actor: "engineer:planner", RequestID: "replace-v2", ExpectedRevision: oldRevision}, c.ID, replacement)
	if err != nil || replayed.Plan.ID != c.Plan.ID || len(r.Audit(c.ID)) != len(events) {
		t.Fatalf("改版重放不稳定: %v", err)
	}
}

func TestEvidenceWithdrawalRecalculatesCoverageAndBansHash(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	c := createPlannedCase(t, s, "withdraw", "D1", now)
	item := c.Plan.Items[0].ID
	batch := []casework.EvidenceRecord{
		{ID: "ev-spectrum", PlanItemID: item, Kind: casework.EvidenceSpectrum, CapturedAt: now.Add(time.Minute), SourceDevice: "D1", ContentHash: strings.Repeat("1", 64)},
		{ID: "ev-reading", PlanItemID: item, Kind: casework.EvidenceReading, CapturedAt: now.Add(time.Minute), SourceDevice: "D1", ContentHash: strings.Repeat("2", 64)},
		{ID: "ev-observation", PlanItemID: item, Kind: casework.EvidenceObservation, CapturedAt: now.Add(time.Minute), SourceDevice: "D1", ContentHash: strings.Repeat("3", 64)},
	}
	c, err := s.EvidenceBatch(Meta{Actor: "engineer:collector", RequestID: "withdraw-add", ExpectedRevision: c.Revision}, c.ID, batch)
	if err != nil || c.State != casework.StateEvidence {
		t.Fatalf("证据未齐备: %v", err)
	}
	c, err = s.WithdrawEvidence(Meta{Actor: "engineer:collector", RequestID: "withdraw-one", ExpectedRevision: c.Revision}, c.ID, []string{"ev-observation"}, "现场摘要标注错误")
	if err != nil {
		t.Fatal(err)
	}
	if c.State != casework.StatePlanned || c.EvidenceCoverage.Complete || len(c.EvidenceWithdrawals) != 1 || len(c.EvidenceCoverage.Gaps) != 1 {
		t.Fatalf("撤回后覆盖未回算: %#v", c.EvidenceCoverage)
	}
	revision, auditCount := c.Revision, len(r.Audit(c.ID))
	_, err = s.Evidence(Meta{Actor: "engineer:collector", RequestID: "withdraw-reuse", ExpectedRevision: revision}, c.ID, casework.EvidenceRecord{ID: "ev-corrected", PlanItemID: item, Kind: casework.EvidenceObservation, CapturedAt: now.Add(time.Minute), SourceDevice: "D1", ContentHash: strings.Repeat("3", 64)})
	if err == nil {
		t.Fatal("已撤回 content_hash 被再次接受")
	}
	unchanged, _ := r.GetCase(c.ID)
	if unchanged.Revision != revision || len(r.Audit(c.ID)) != auditCount {
		t.Fatal("失败更正改变了案件或审计")
	}
}

func TestMitigationChainAndTrend(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	c := &casework.InterferenceCase{ID: "trend-case", State: casework.StateHypothesis, Revision: 1, CreatedAt: now, Hypothesis: &casework.SourceHypothesis{Decision: "CONFIRMED", ConfirmedAt: timePointer(now)}}
	if err := r.PutCase(c); err != nil {
		t.Fatal(err)
	}
	c, err := s.Mitigation(Meta{Actor: "engineer:e1", RequestID: "m1", ExpectedRevision: 1}, c.ID, casework.MitigationVerification{MeasureType: "屏蔽", MeasureDescription: "加装屏蔽", ImplementedAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	firstID := c.MitigationAttempts[0].ID
	beforeRevision := c.Revision
	if _, err = s.Mitigation(Meta{Actor: "engineer:e2", RequestID: "parallel", ExpectedRevision: beforeRevision}, c.ID, casework.MitigationVerification{MeasureType: "移频", MeasureDescription: "并行措施", ImplementedAt: now.Add(2 * time.Minute)}); err == nil {
		t.Fatal("待复测时接受了并行措施")
	}
	c, err = s.Verification(Meta{Actor: "reviewer:r1", RequestID: "v1", ExpectedRevision: beforeRevision}, c.ID, casework.MitigationVerification{AttemptID: firstID, VerificationWindow: casework.ObservationWindow{Start: now.Add(3 * time.Minute), End: now.Add(4 * time.Minute)}, Thresholds: map[string]float64{"noise": 10}, ObservedMetrics: map[string]float64{"noise": 14}, RemediationReason: "仍超阈值"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Mitigation(Meta{Actor: "engineer:e2", RequestID: "m2", ExpectedRevision: c.Revision}, c.ID, casework.MitigationVerification{PreviousAttemptID: firstID, MeasureType: "移频", MeasureDescription: "迁移频点", ImplementedAt: now.Add(5 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	secondID := c.MitigationAttempts[1].ID
	c, err = s.Verification(Meta{Actor: "reviewer:r2", RequestID: "v2", ExpectedRevision: c.Revision}, c.ID, casework.MitigationVerification{AttemptID: secondID, VerificationWindow: casework.ObservationWindow{Start: now.Add(6 * time.Minute), End: now.Add(7 * time.Minute)}, Thresholds: map[string]float64{"noise": 10}, ObservedMetrics: map[string]float64{"noise": 7}})
	if err != nil || c.State != casework.StateVerified {
		t.Fatalf("第二次复测未通过: %v", err)
	}
	view, err := s.Detail(c.ID)
	if err != nil || len(view.MitigationTrend.Attempts) != 2 || len(view.MitigationTrend.Comparisons) != 1 || !view.MitigationTrend.Comparisons[0].Comparable || view.MitigationTrend.Comparisons[0].Improvements["noise"] != 7 || view.MitigationTrend.Overall != "IMPROVING" {
		encoded, _ := json.Marshal(view.MitigationTrend)
		t.Fatalf("趋势计算错误: %v %s", err, encoded)
	}
}

func timePointer(v time.Time) *time.Time { return &v }

func TestCaseListCursorFiltersAndReadOnly(t *testing.T) {
	s, r := testService(t)
	now := time.Now().UTC()
	ids := []string{}
	for i := 0; i < 5; i++ {
		c, err := s.Create(Meta{Actor: "duty:a", RequestID: "list-" + string(rune('a'+i))}, CreateInput{ObservationWindow: casework.ObservationWindow{Start: now.Add(time.Duration(i) * time.Minute), End: now.Add(time.Hour + time.Duration(i)*time.Minute)}, FrequencyRangeHz: [2]float64{100, 200}, AntennaID: "A", InitialFeature: "burst"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	revisions, audits := map[string]int{}, map[string]int{}
	for _, id := range ids {
		c, _ := r.GetCase(id)
		revisions[id], audits[id] = c.Revision, len(r.Audit(id))
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, err := s.ListCases(CaseListQuery{PageSize: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			if seen[item.CaseID] {
				t.Fatal("分页出现重复案件")
			}
			seen[item.CaseID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatalf("分页遗漏案件: %d", len(seen))
	}
	first, _ := s.ListCases(CaseListQuery{PageSize: 2})
	if _, err := s.ListCases(CaseListQuery{PageSize: 2, Severities: []string{"HIGH"}, Cursor: first.NextCursor}); err == nil {
		t.Fatal("跨筛选条件游标被接受")
	}
	if _, err := s.ListCases(CaseListQuery{States: []casework.State{"UNKNOWN"}}); err == nil {
		t.Fatal("未知状态被接受")
	}
	for _, id := range ids {
		c, _ := r.GetCase(id)
		if c.Revision != revisions[id] || len(r.Audit(id)) != audits[id] {
			t.Fatal("集合查询改写了案件或审计")
		}
	}
}
