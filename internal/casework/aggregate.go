package casework

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func NewCase(id, antenna string, freq [2]float64, w ObservationWindow, feature string) (*InterferenceCase, error) {
	if id == "" || strings.TrimSpace(antenna) == "" || !w.Start.Before(w.End) || freq[0] >= freq[1] || strings.TrimSpace(feature) == "" {
		return nil, Invalid("案件登记参数无效")
	}
	return &InterferenceCase{ID: id, State: StateDetected, Revision: 1, ObservationWindow: w, FrequencyRangeHz: freq, AntennaID: strings.TrimSpace(antenna), InitialFeature: normalizeFeature(feature), Fingerprint: Fingerprint(antenna, freq, w, feature), CreatedAt: time.Now().UTC(), EvidenceCoverage: EvidenceCoverage{PlanItems: map[string][]string{}, Gaps: []EvidenceGap{}}}, nil
}

func (c *InterferenceCase) Triage(t Triage, actor ...string) error {
	if c.State != StateDetected {
		return Invalid("当前状态不可研判")
	}
	score, severity, err := ScoreTriage(t, c.FrequencyRangeHz[1]-c.FrequencyRangeHz[0])
	if err != nil {
		return err
	}
	t.Rationale = strings.TrimSpace(t.Rationale)
	t.BandwidthRatio = t.OccupiedBandwidthHz / (c.FrequencyRangeHz[1] - c.FrequencyRangeHz[0])
	t.ScoreBreakdown = score
	t.Severity = severity
	if len(actor) > 0 {
		t.TriagedBy = actor[0]
	}
	c.Impact = &t
	c.Severity = severity
	c.State = StateTriaged
	c.Revision++
	return nil
}

func (c *InterferenceCase) SetPlan(p InvestigationPlan) error {
	if c.State != StateTriaged {
		return Invalid("案件尚未完成研判")
	}
	p.Revision = 1
	built, err := BuildPlan(p, *c)
	if err != nil {
		return err
	}
	built.CaseID = c.ID
	c.Plan = &built
	c.State = StatePlanned
	c.Revision++
	c.EvidenceCoverage = Coverage(*c)
	return nil
}

func (c *InterferenceCase) ReplacePlan(p InvestigationPlan, reason string) error {
	if c.State != StatePlanned || c.Plan == nil {
		return Invalid("当前状态不可改版调查计划")
	}
	if len(c.Evidence) > 0 || len(c.EvidenceWithdrawals) > 0 {
		return Invalid("案件已有证据记录，调查计划不可改版")
	}
	if strings.TrimSpace(reason) == "" {
		return Invalid("计划改版必须填写 replacement_reason")
	}
	p.Revision = c.Plan.Revision + 1
	built, err := BuildPlan(p, *c)
	if err != nil {
		return err
	}
	built.CaseID = c.ID
	c.Plan = &built
	c.EvidenceCoverage = Coverage(*c)
	c.Revision++
	return nil
}

func (c *InterferenceCase) AddEvidence(e EvidenceRecord) error {
	return c.AddEvidenceBatch([]EvidenceRecord{e})
}
func (c *InterferenceCase) AddEvidenceBatch(batch []EvidenceRecord) error {
	if c.State != StatePlanned && c.State != StateEvidence {
		return Invalid("当前状态不可取证")
	}
	if c.Plan == nil {
		return Invalid("缺少调查计划")
	}
	if len(batch) == 0 {
		return Invalid("证据批次不得为空")
	}
	ids := map[string]bool{}
	hashes := map[string]bool{}
	for _, old := range c.Evidence {
		ids[old.ID] = true
		hashes[strings.ToLower(old.ContentHash)] = true
	}
	for _, withdrawn := range c.EvidenceWithdrawals {
		ids[withdrawn.Evidence.ID] = true
		hashes[strings.ToLower(withdrawn.Evidence.ContentHash)] = true
	}
	for i := range batch {
		if batch[i].ID == "" {
			return InvalidDetails("证据 ID 不得为空", map[string]any{"batch_index": i})
		}
		if err := ValidateEvidence(batch[i], *c.Plan); err != nil {
			return InvalidDetails(err.Error(), map[string]any{"batch_index": i})
		}
		h := strings.ToLower(batch[i].ContentHash)
		if ids[batch[i].ID] || hashes[h] {
			return InvalidDetails("证据 ID 或 content_hash 重复", map[string]any{"batch_index": i})
		}
		ids[batch[i].ID] = true
		hashes[h] = true
		batch[i].ContentHash = h
		batch[i].CaseID = c.ID
	}
	c.Evidence = append(c.Evidence, batch...)
	c.EvidenceCoverage = Coverage(*c)
	if c.EvidenceCoverage.Complete {
		c.State = StateEvidence
	}
	c.Revision++
	return nil
}

func (c *InterferenceCase) WithdrawEvidence(ids []string, actor, reason string) error {
	if c.State != StatePlanned && c.State != StateEvidence {
		return Invalid("当前状态不可撤回证据")
	}
	if len(c.SourceCandidates) > 0 || c.Hypothesis != nil {
		return Invalid("来源研判已经开始，证据不可撤回")
	}
	if len(ids) < 1 || len(ids) > 50 {
		return Invalid("evidence_ids 必须包含 1 到 50 个标识")
	}
	if strings.TrimSpace(reason) == "" {
		return Invalid("证据撤回必须填写 correction_reason")
	}
	positions := make(map[string]int, len(c.Evidence))
	for i := range c.Evidence {
		positions[c.Evidence[i].ID] = i
	}
	seen := map[string]bool{}
	selected := make([]EvidenceRecord, 0, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return Invalid("evidence_ids 包含空值或重复标识")
		}
		seen[id] = true
		pos, ok := positions[id]
		if !ok {
			return InvalidDetails("证据不存在、已撤回或不属于本案", map[string]any{"evidence_id": id})
		}
		selected = append(selected, c.Evidence[pos])
	}
	kept := make([]EvidenceRecord, 0, len(c.Evidence)-len(selected))
	for _, evidence := range c.Evidence {
		if !seen[evidence.ID] {
			kept = append(kept, evidence)
		}
	}
	revision := c.Revision + 1
	for _, evidence := range selected {
		c.EvidenceWithdrawals = append(c.EvidenceWithdrawals, EvidenceWithdrawal{Evidence: evidence, WithdrawnBy: actor, CorrectionReason: strings.TrimSpace(reason), WithdrawalRevision: revision})
	}
	c.Evidence = kept
	c.EvidenceCoverage = Coverage(*c)
	if !c.EvidenceCoverage.Complete {
		c.State = StatePlanned
	}
	c.Revision++
	return nil
}

func (c *InterferenceCase) RegisterCandidate(id, source string) error {
	if c.State != StateEvidence {
		return Invalid("证据尚未齐备")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return Invalid("候选来源不得为空")
	}
	for _, v := range c.SourceCandidates {
		if strings.EqualFold(v.CandidateSource, source) {
			return Invalid("候选来源已存在")
		}
	}
	c.SourceCandidates = append(c.SourceCandidates, SourceCandidate{ID: id, CandidateSource: source, Decision: "PENDING"})
	c.Revision++
	return nil
}
func (c *InterferenceCase) AddCandidateTest(candidateID string, test ControlledTest) error {
	if c.State != StateEvidence {
		return Invalid("当前状态不可追加来源测试")
	}
	x := c.candidate(candidateID)
	if x == nil || x.Decision != "PENDING" {
		return Invalid("候选不存在或已结束")
	}
	x.Tests = append(x.Tests, test)
	CandidateAssessment(x)
	c.Revision++
	return nil
}
func (c *InterferenceCase) ExcludeCandidate(candidateID, reason string) error {
	if c.State != StateEvidence {
		return Invalid("当前状态不可排除候选")
	}
	x := c.candidate(candidateID)
	if x == nil {
		return Invalid("候选不存在")
	}
	if strings.TrimSpace(reason) == "" {
		return Invalid("排除原因不得为空")
	}
	x.Decision = "EXCLUDED"
	x.DecisionReason = strings.TrimSpace(reason)
	c.Revision++
	return nil
}
func (c *InterferenceCase) ConfirmCandidate(candidateID, notes, actor string) error {
	if c.State != StateEvidence {
		return Invalid("证据尚未齐备")
	}
	x := c.candidate(candidateID)
	if x == nil || x.Decision != "PENDING" {
		return Invalid("候选不存在或已排除")
	}
	x.ExclusionNotes = strings.TrimSpace(notes)
	missing := CandidateMissing(*x)
	if len(missing) > 0 {
		return InvalidDetails("来源确认条件尚未齐备", map[string]any{"missing_conditions": missing})
	}
	x.Decision = "CONFIRMED"
	x.ConfirmedBy = actor
	now := time.Now().UTC()
	x.ConfirmedAt = &now
	c.ConfirmedCandidateID = x.ID
	for i := range c.SourceCandidates {
		if c.SourceCandidates[i].ID != x.ID && c.SourceCandidates[i].Decision == "PENDING" {
			c.SourceCandidates[i].Decision = "NOT_SELECTED"
			c.SourceCandidates[i].DecisionReason = "其他候选已确认"
		}
	}
	c.Hypothesis = &SourceHypothesis{ID: x.ID, CaseID: c.ID, CandidateSource: x.CandidateSource, CorrelationScore: x.CorrelationScore, Repeatable: true, ExclusionNotes: x.ExclusionNotes, Decision: "CONFIRMED", ConfirmedAt: &now}
	for _, t := range x.Tests {
		c.Hypothesis.TestWindows = append(c.Hypothesis.TestWindows, t.Window)
	}
	c.State = StateHypothesis
	c.Revision++
	return nil
}
func (c *InterferenceCase) ConfirmHypothesis(h SourceHypothesis) error {
	if c.State != StateEvidence {
		return Invalid("证据尚未齐备")
	}
	if !Confirmable(h) {
		return Invalid("来源判定未满足相关性、重复性和排他性")
	}
	h.CaseID = c.ID
	h.Decision = "CONFIRMED"
	c.Hypothesis = &h
	c.ConfirmedCandidateID = h.ID
	c.State = StateHypothesis
	c.Revision++
	return nil
}
func (c *InterferenceCase) candidate(id string) *SourceCandidate {
	for i := range c.SourceCandidates {
		if c.SourceCandidates[i].ID == id || c.SourceCandidates[i].CandidateSource == id {
			return &c.SourceCandidates[i]
		}
	}
	return nil
}

func (c *InterferenceCase) AddAttempt(a MitigationAttempt) error {
	if c.State != StateHypothesis && c.State != StateMitigated {
		return Invalid("尚未确认来源")
	}
	allowed := map[string]bool{"屏蔽": true, "移频": true, "时段协调": true, "shielding": true, "frequency_shift": true, "schedule_coordination": true}
	if !allowed[a.MeasureType] || strings.TrimSpace(a.MeasureDescription) == "" || a.ImplementedBy == "" {
		return Invalid("抑制措施类型或信息无效")
	}
	confirmedAt := time.Time{}
	if c.Hypothesis != nil {
		if c.Hypothesis.ConfirmedAt != nil {
			confirmedAt = *c.Hypothesis.ConfirmedAt
		}
		for _, w := range c.Hypothesis.TestWindows {
			if w.End.After(confirmedAt) {
				confirmedAt = w.End
			}
		}
	}
	if !confirmedAt.IsZero() && a.ImplementedAt.Before(confirmedAt) {
		return Invalid("措施实施时间不得早于来源确认测试")
	}
	if a.ImplementedAt.IsZero() {
		return Invalid("实施时间不能为空")
	}
	if len(c.MitigationAttempts) == 0 {
		if a.PreviousAttemptID != "" {
			return Invalid("首次抑制尝试不得填写 previous_attempt_id")
		}
	} else {
		latest := c.MitigationAttempts[len(c.MitigationAttempts)-1]
		if latest.Status == "PENDING_VERIFICATION" {
			return Invalid("存在待复测尝试，不得并行提交新措施")
		}
		if latest.Status != "FAILED" || a.PreviousAttemptID == "" || a.PreviousAttemptID != latest.ID {
			return Invalid("后续措施必须引用本案最新失败尝试")
		}
	}
	a.Status = "PENDING_VERIFICATION"
	c.MitigationAttempts = append(c.MitigationAttempts, a)
	c.Mitigations = append(c.Mitigations, MitigationVerification{ID: a.ID, AttemptID: a.ID, MeasureType: a.MeasureType, MeasureDescription: a.MeasureDescription, ImplementedAt: a.ImplementedAt, Reviewer: a.ImplementedBy})
	c.State = StateMitigated
	c.Revision++
	return nil
}
func (c *InterferenceCase) AddMitigation(m MitigationVerification) error {
	return c.AddAttempt(MitigationAttempt{ID: m.ID, PreviousAttemptID: m.PreviousAttemptID, MeasureType: m.MeasureType, MeasureDescription: m.MeasureDescription, ImplementedAt: m.ImplementedAt, ImplementedBy: m.Reviewer})
}
func (c *InterferenceCase) VerifyAttempt(attemptID, reviewer string, v MitigationVerification) error {
	if c.State != StateMitigated {
		return Invalid("尚未实施抑制措施")
	}
	a := c.attempt(attemptID)
	if a == nil {
		return Invalid("抑制尝试不存在")
	}
	if a.Status != "PENDING_VERIFICATION" {
		return Invalid("抑制尝试已经复测，结论不可改写")
	}
	if len(c.MitigationAttempts) == 0 || c.MitigationAttempts[len(c.MitigationAttempts)-1].ID != a.ID {
		return Invalid("仅最新待复测尝试可以复测")
	}
	if reviewer == a.ImplementedBy {
		return Forbidden("复测人与措施实施人必须分离")
	}
	if !v.VerificationWindow.Start.Before(v.VerificationWindow.End) || !v.VerificationWindow.Start.After(a.ImplementedAt) {
		return Invalid("复测窗口必须晚于措施实施时间")
	}
	if c.Hypothesis != nil {
		for _, w := range c.Hypothesis.TestWindows {
			if WindowsOverlap(w, v.VerificationWindow) {
				return Invalid("复测窗口与来源确认测试窗口重叠")
			}
		}
	}
	for _, attempt := range c.MitigationAttempts {
		for _, old := range attempt.Verifications {
			if WindowsOverlap(old.Window, v.VerificationWindow) {
				return Invalid("复测窗口与既有复测窗口重叠")
			}
		}
	}
	if len(v.Thresholds) == 0 {
		return Invalid("复测阈值不得为空")
	}
	diff := map[string]float64{}
	pass := true
	for k, t := range v.Thresholds {
		x, ok := v.ObservedMetrics[k]
		if !ok || math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(t) || math.IsInf(t, 0) {
			return Invalid("观测指标必须完整且为有限数值")
		}
		diff[k] = x - t
		if x > t {
			pass = false
		}
	}
	result := "PASS"
	if !pass {
		result = "FAIL"
		if strings.TrimSpace(v.RemediationReason) == "" {
			return Invalid("不合格复测必须填写整改原因")
		}
		a.Status = "FAILED"
	} else {
		a.Status = "PASSED"
		c.State = StateVerified
	}
	a.Verifications = append(a.Verifications, VerificationResult{Window: v.VerificationWindow, Thresholds: v.Thresholds, ObservedMetrics: v.ObservedMetrics, Differences: diff, Result: result, Reviewer: reviewer, RemediationReason: strings.TrimSpace(v.RemediationReason)})
	c.Mitigations = append(c.Mitigations, MitigationVerification{AttemptID: a.ID, VerificationWindow: v.VerificationWindow, Thresholds: v.Thresholds, ObservedMetrics: v.ObservedMetrics, Result: result, Reviewer: reviewer, RemediationReason: v.RemediationReason})
	c.Revision++
	return nil
}
func (c *InterferenceCase) Verify(m MitigationVerification) error {
	return c.VerifyAttempt(m.AttemptID, m.Reviewer, m)
}
func (c *InterferenceCase) attempt(id string) *MitigationAttempt {
	if id == "" && len(c.MitigationAttempts) > 0 {
		return &c.MitigationAttempts[len(c.MitigationAttempts)-1]
	}
	for i := range c.MitigationAttempts {
		if c.MitigationAttempts[i].ID == id {
			return &c.MitigationAttempts[i]
		}
	}
	return nil
}

func (c *InterferenceCase) ClosureChecklist(auditContinuous bool) []ClosureItem {
	confirmed := c.Hypothesis != nil && c.Hypothesis.Decision == "CONFIRMED"
	validAttempt, passed := false, false
	for _, a := range c.MitigationAttempts {
		if a.ImplementedBy != "" {
			validAttempt = true
		}
		if a.Status == "PASSED" {
			passed = true
		}
	}
	return []ClosureItem{{"evidence_coverage", c.EvidenceCoverage.Complete, "所有计划项三类证据齐备"}, {"confirmed_source", confirmed, "存在唯一已确认来源"}, {"valid_mitigation", validAttempt, "存在有效抑制尝试"}, {"independent_verification", passed, "存在通过的独立复测"}, {"audit_continuity", auditContinuous, "审计修订连续且摘要一致"}}
}
func (c *InterferenceCase) Close(actor string, auditOK ...bool) error {
	if c.State == StateClosed {
		return Terminal("案件已封存")
	}
	if c.State != StateVerified {
		return Invalid("复测尚未通过")
	}
	if !CanClose(actor) {
		return Forbidden("仅 reviewer 角色可以封存案件")
	}
	ok := true
	if len(auditOK) > 0 {
		ok = auditOK[0]
	}
	items := c.ClosureChecklist(ok)
	missing := []string{}
	for _, x := range items {
		if !x.Passed {
			missing = append(missing, x.Name)
		}
	}
	if len(missing) > 0 {
		return InvalidDetails("封存材料不齐备", map[string]any{"missing_items": missing, "checklist": items})
	}
	conflicts := []string{}
	if c.Hypothesis != nil {
		for _, x := range c.SourceCandidates {
			if x.ID == c.ConfirmedCandidateID && x.ConfirmedBy == actor {
				conflicts = append(conflicts, "source_confirmation")
			}
		}
	}
	if len(c.MitigationAttempts) > 0 && c.MitigationAttempts[len(c.MitigationAttempts)-1].ImplementedBy == actor {
		conflicts = append(conflicts, "last_mitigation")
	}
	for _, a := range c.MitigationAttempts {
		for _, v := range a.Verifications {
			if v.Result == "PASS" && v.Reviewer == actor {
				conflicts = append(conflicts, "passed_verification")
			}
		}
	}
	if len(conflicts) > 0 {
		return Forbidden(fmt.Sprintf("封存职责冲突: %s", strings.Join(conflicts, ",")))
	}
	now := time.Now().UTC()
	c.State = StateClosed
	c.ClosedAt = &now
	c.Closure = &ClosureReview{ClosedAt: now, Reviewer: actor, Checklist: items}
	c.Revision++
	return nil
}
