package service

import (
	"sort"
	"strings"

	"github.com/benzhi/relay-survey/internal/casework"
)

func (s *Service) Triage(m Meta, id string, t casework.Triage) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleDuty); err != nil {
		return nil, err
	}
	return s.mutate(m, id, "CASE_TRIAGED", func(c *casework.InterferenceCase) error { return c.Triage(t, m.Actor) }, func(c *casework.InterferenceCase) any {
		return map[string]any{"input": c.Impact, "score_breakdown": c.Impact.ScoreBreakdown, "actor": m.Actor}
	})
}

func (s *Service) Plan(m Meta, id string, p casework.InvestigationPlan) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleEngineer); err != nil {
		return nil, err
	}
	s.planMu.Lock()
	defer s.planMu.Unlock()
	p = NormalizePlan(p)
	reason := p.ReplacementReason
	p.ID = prefixedID("plan")
	eventType := "PLAN_CREATED"
	if reason != "" {
		eventType = "PLAN_REPLACED"
	}
	var oldPlan *casework.InvestigationPlan
	return s.mutate(m, id, eventType, func(c *casework.InterferenceCase) error {
		if c.State == casework.StateTriaged && reason != "" {
			return casework.Invalid("首版调查计划不得填写 replacement_reason")
		}
		if c.State == casework.StatePlanned {
			if c.Plan == nil {
				return casework.Invalid("当前案件缺少生效调查计划")
			}
			old := *c.Plan
			oldPlan = &old
			p.Revision = c.Plan.Revision + 1
		} else {
			p.Revision = 1
		}
		built, err := casework.BuildPlan(p, *c)
		if err != nil {
			return err
		}
		conflicts := []casework.ResourceConflict{}
		for _, other := range s.repo.Cases() {
			if other.ID == c.ID || other.State == casework.StateClosed || other.Plan == nil {
				continue
			}
			for _, a := range built.Items {
				for _, b := range other.Plan.Items {
					if a.EquipmentID == b.EquipmentID && casework.WindowsOverlap(a.TimeWindow, b.TimeWindow) {
						conflicts = append(conflicts, casework.ResourceConflict{CaseID: other.ID, PlanItemID: b.ID, EquipmentID: b.EquipmentID})
					}
				}
			}
		}
		if len(conflicts) > 0 {
			return casework.ResourceBusy(conflicts)
		}
		built.ResourceValidation = casework.ResourceValidation{Checked: true, Conflicts: []casework.ResourceConflict{}}
		if c.State == casework.StateTriaged {
			return c.SetPlan(built)
		}
		return c.ReplacePlan(built, reason)
	}, func(c *casework.InterferenceCase) any {
		out := map[string]any{"plan_id": c.Plan.ID, "plan_revision": c.Plan.Revision, "plan_items": c.Plan.Items, "coverage_summary": c.Plan.Coverage, "resource_validation": c.Plan.ResourceValidation}
		if oldPlan != nil {
			added, removed := equipmentDiff(oldPlan.EquipmentIDs, c.Plan.EquipmentIDs)
			out["replacement_reason"] = reason
			out["old_plan_id"] = oldPlan.ID
			out["new_plan_id"] = c.Plan.ID
			out["added_equipment_ids"] = added
			out["removed_equipment_ids"] = removed
		}
		return out
	})
}

func equipmentDiff(oldIDs, newIDs []string) ([]string, []string) {
	oldSet, newSet := map[string]bool{}, map[string]bool{}
	for _, id := range oldIDs {
		oldSet[id] = true
	}
	for _, id := range newIDs {
		newSet[id] = true
	}
	added, removed := []string{}, []string{}
	for id := range newSet {
		if !oldSet[id] {
			added = append(added, id)
		}
	}
	for id := range oldSet {
		if !newSet[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func (s *Service) Evidence(m Meta, id string, e casework.EvidenceRecord) (*casework.InterferenceCase, error) {
	return s.EvidenceBatch(m, id, []casework.EvidenceRecord{e})
}
func (s *Service) EvidenceBatch(m Meta, id string, batch []casework.EvidenceRecord) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleEngineer); err != nil {
		return nil, err
	}
	if len(batch) > 50 {
		return nil, casework.Invalid("证据批次最多 50 条")
	}
	for i := range batch {
		if strings.TrimSpace(batch[i].ID) == "" {
			batch[i].ID = prefixedID("ev")
		}
	}
	return s.mutate(m, id, "EVIDENCE_BATCH_ACCEPTED", func(c *casework.InterferenceCase) error { return c.AddEvidenceBatch(batch) }, func(c *casework.InterferenceCase) any {
		return map[string]any{"batch_size": len(batch), "evidence_ids": evidenceIDs(batch), "coverage": c.EvidenceCoverage}
	})
}
func evidenceIDs(v []casework.EvidenceRecord) []string {
	out := make([]string, len(v))
	for i := range v {
		out[i] = v[i].ID
	}
	return out
}

func (s *Service) WithdrawEvidence(m Meta, id string, evidenceIDs []string, reason string) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleEngineer); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	return s.mutate(m, id, "EVIDENCE_WITHDRAWN", func(c *casework.InterferenceCase) error {
		return c.WithdrawEvidence(evidenceIDs, m.Actor, reason)
	}, func(c *casework.InterferenceCase) any {
		return map[string]any{"evidence_ids": evidenceIDs, "correction_reason": reason, "coverage": c.EvidenceCoverage, "active_evidence_count": len(c.Evidence)}
	})
}

type HypothesisCommand struct {
	Action          string                     `json:"action"`
	CandidateID     string                     `json:"candidate_id,omitempty"`
	CandidateSource string                     `json:"candidate_source,omitempty"`
	Test            *HypothesisTestInput       `json:"test,omitempty"`
	TestWindow      casework.ObservationWindow `json:"test_window,omitempty"`
	BaselineMetrics map[string]float64         `json:"baseline_metrics,omitempty"`
	ActiveMetrics   map[string]float64         `json:"active_metrics,omitempty"`
	ExclusionNotes  string                     `json:"exclusion_notes,omitempty"`
	Reason          string                     `json:"reason,omitempty"`
}
type HypothesisTestInput struct {
	Window          casework.ObservationWindow `json:"window"`
	BaselineMetrics map[string]float64         `json:"baseline_metrics"`
	ActiveMetrics   map[string]float64         `json:"active_metrics"`
}

func (s *Service) HypothesisAction(m Meta, id string, in HypothesisCommand) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleInvestigator); err != nil {
		return nil, err
	}
	action := strings.ToUpper(strings.TrimSpace(in.Action))
	eventType := "HYPOTHESIS_UPDATED"
	if action == "CONFIRM" {
		eventType = "HYPOTHESIS_CONFIRMED"
	}
	return s.mutate(m, id, eventType, func(c *casework.InterferenceCase) error {
		switch action {
		case "REGISTER", "ADD_CANDIDATE":
			return c.RegisterCandidate(prefixedID("hyp"), in.CandidateSource)
		case "ADD_TEST", "TEST":
			x := candidatesFind(c, in.CandidateID)
			if x == nil {
				return casework.Invalid("候选不存在")
			}
			w, b, a := in.TestWindow, in.BaselineMetrics, in.ActiveMetrics
			if in.Test != nil {
				w, b, a = in.Test.Window, in.Test.BaselineMetrics, in.Test.ActiveMetrics
			}
			test, err := casework.BuildTest(prefixedID("test"), w, b, a, x.Tests)
			if err != nil {
				return err
			}
			return c.AddCandidateTest(in.CandidateID, test)
		case "EXCLUDE":
			return c.ExcludeCandidate(in.CandidateID, firstNonBlank(in.Reason, in.ExclusionNotes))
		case "CONFIRM":
			return c.ConfirmCandidate(in.CandidateID, in.ExclusionNotes, m.Actor)
		default:
			return casework.Invalid("hypothesis action 仅允许 REGISTER、ADD_TEST、EXCLUDE 或 CONFIRM")
		}
	}, func(c *casework.InterferenceCase) any {
		return map[string]any{"action": action, "candidate_id": in.CandidateID, "candidates": c.SourceCandidates}
	})
}
func candidatesFind(c *casework.InterferenceCase, id string) *casework.SourceCandidate {
	for i := range c.SourceCandidates {
		if c.SourceCandidates[i].ID == id || c.SourceCandidates[i].CandidateSource == id {
			return &c.SourceCandidates[i]
		}
	}
	return nil
}
func firstNonBlank(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
func (s *Service) Hypothesis(m Meta, id string, h casework.SourceHypothesis) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleInvestigator); err != nil {
		return nil, err
	}
	return s.mutate(m, id, "HYPOTHESIS_CONFIRMED", func(c *casework.InterferenceCase) error { return c.ConfirmHypothesis(h) }, nil)
}

func (s *Service) Mitigation(m Meta, id string, v casework.MitigationVerification) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleEngineer); err != nil {
		return nil, err
	}
	attempt := casework.MitigationAttempt{ID: prefixedID("attempt"), PreviousAttemptID: v.PreviousAttemptID, MeasureType: v.MeasureType, MeasureDescription: strings.TrimSpace(v.MeasureDescription), ImplementedAt: v.ImplementedAt, ImplementedBy: m.Actor}
	return s.mutate(m, id, "MITIGATION_ATTEMPTED", func(c *casework.InterferenceCase) error { return c.AddAttempt(attempt) }, func(c *casework.InterferenceCase) any {
		return map[string]any{"attempt": c.MitigationAttempts[len(c.MitigationAttempts)-1], "previous_attempt_id": attempt.PreviousAttemptID}
	})
}
func (s *Service) Verification(m Meta, id string, v casework.MitigationVerification) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleReviewer); err != nil {
		return nil, err
	}
	if strings.TrimSpace(v.AttemptID) == "" {
		return nil, casework.Invalid("verification 必须指定 attempt_id")
	}
	return s.mutate(m, id, "VERIFICATION_RECORDED", func(c *casework.InterferenceCase) error { return c.VerifyAttempt(v.AttemptID, m.Actor, v) }, func(c *casework.InterferenceCase) any {
		attempt := c.MitigationAttempts[len(c.MitigationAttempts)-1]
		verification := attempt.Verifications[len(attempt.Verifications)-1]
		return map[string]any{"attempt_id": v.AttemptID, "previous_attempt_id": attempt.PreviousAttemptID, "conclusion": verification.Result, "metric_differences": verification.Differences, "attempts": c.MitigationAttempts}
	})
}

func (s *Service) Close(m Meta, id string) (*casework.InterferenceCase, error) {
	if err := requireRole(m.Actor, casework.RoleReviewer); err != nil {
		return nil, err
	}
	return s.mutate(m, id, "CASE_CLOSED", func(c *casework.InterferenceCase) error { return c.Close(m.Actor, s.repo.AuditIntegrity(id)) }, func(c *casework.InterferenceCase) any { return c.Closure })
}
