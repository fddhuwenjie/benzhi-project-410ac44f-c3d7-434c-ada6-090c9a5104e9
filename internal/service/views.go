package service

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/benzhi/relay-survey/internal/casework"
)

type ProgressView struct {
	CompletedStages []casework.State `json:"completed_stages"`
	CurrentStage    casework.State   `json:"current_stage"`
	NextActions     []string         `json:"next_actions"`
	Blockers        []string         `json:"blockers"`
	Percent         int              `json:"percent"`
}
type CaseSummary struct {
	EvidenceCoverage casework.EvidenceCoverage   `json:"evidence_coverage"`
	CandidateCounts  map[string]int              `json:"candidate_counts"`
	LatestMitigation *casework.MitigationAttempt `json:"latest_mitigation,omitempty"`
	ClosurePrecheck  []casework.ClosureItem      `json:"closure_precheck"`
}
type CaseView struct {
	*casework.InterferenceCase
	Progress        ProgressView         `json:"progress"`
	Summary         CaseSummary          `json:"summary"`
	MitigationTrend MitigationTrendView  `json:"mitigation_trend"`
	AuditCount      int                  `json:"audit_count"`
	LastEvent       *casework.AuditEvent `json:"last_event,omitempty"`
}

var stages = []casework.State{casework.StateDetected, casework.StateTriaged, casework.StatePlanned, casework.StateEvidence, casework.StateHypothesis, casework.StateMitigated, casework.StateVerified, casework.StateClosed}

func (s *Service) Detail(id string) (*CaseView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	events := ordered(s.repo.Audit(id))
	progress := buildProgress(c)
	counts := map[string]int{}
	for _, x := range c.SourceCandidates {
		counts[x.Decision]++
	}
	summary := CaseSummary{EvidenceCoverage: c.EvidenceCoverage, CandidateCounts: counts, ClosurePrecheck: c.ClosureChecklist(s.repo.AuditIntegrity(id))}
	if len(c.MitigationAttempts) > 0 {
		last := c.MitigationAttempts[len(c.MitigationAttempts)-1]
		summary.LatestMitigation = &last
	}
	view := &CaseView{InterferenceCase: c, Progress: progress, Summary: summary, MitigationTrend: buildMitigationTrend(c.MitigationAttempts), AuditCount: len(events)}
	if len(events) > 0 {
		view.LastEvent = &events[len(events)-1]
	}
	return view, nil
}

func buildProgress(c *casework.InterferenceCase) ProgressView {
	idx := 0
	for i, v := range stages {
		if v == c.State {
			idx = i
		}
	}
	completed := append([]casework.State(nil), stages[:idx]...)
	next := []string{}
	blockers := []string{}
	switch c.State {
	case casework.StateDetected:
		next = []string{"triage"}
		blockers = []string{"等待影响研判"}
	case casework.StateTriaged:
		next = []string{"plan"}
		blockers = []string{"等待调查计划"}
	case casework.StatePlanned:
		next = []string{"evidence"}
		for _, g := range c.EvidenceCoverage.Gaps {
			blockers = append(blockers, fmt.Sprintf("计划项 %s 缺少 %s", g.PlanItemID, strings.Join(g.MissingKinds, ",")))
		}
	case casework.StateEvidence:
		next = []string{"hypothesis"}
		blockers = []string{"等待确认唯一来源"}
	case casework.StateHypothesis:
		next = []string{"mitigation"}
		blockers = []string{"等待抑制措施"}
	case casework.StateMitigated:
		if len(c.MitigationAttempts) > 0 && c.MitigationAttempts[len(c.MitigationAttempts)-1].Status == "FAILED" {
			next = []string{"mitigation"}
			blockers = []string{"最近一次复测未通过，等待后续整改措施"}
		} else {
			next = []string{"verification"}
			blockers = []string{"等待独立复测通过"}
		}
	case casework.StateVerified:
		next = []string{"close"}
	case casework.StateClosed:
		completed = append(completed, casework.StateClosed)
	}
	return ProgressView{CompletedStages: completed, CurrentStage: c.State, NextActions: next, Blockers: blockers, Percent: (idx * 100) / (len(stages) - 1)}
}

type MitigationMetricView struct {
	ObservedDifference float64 `json:"observed_difference"`
	Exceedance         float64 `json:"exceedance"`
	ComplianceMargin   float64 `json:"compliance_margin"`
}
type MitigationTrendNode struct {
	AttemptID          string                          `json:"attempt_id"`
	PreviousAttemptID  string                          `json:"previous_attempt_id,omitempty"`
	MeasureType        string                          `json:"measure_type"`
	MeasureDescription string                          `json:"measure_description"`
	Status             string                          `json:"status"`
	Reviewer           string                          `json:"reviewer,omitempty"`
	RemediationReason  string                          `json:"remediation_reason,omitempty"`
	Metrics            map[string]MitigationMetricView `json:"metrics,omitempty"`
}
type MitigationComparison struct {
	FromAttemptID string             `json:"from_attempt_id"`
	ToAttemptID   string             `json:"to_attempt_id"`
	Comparable    bool               `json:"comparable"`
	Reason        string             `json:"reason,omitempty"`
	Improvements  map[string]float64 `json:"improvements,omitempty"`
}
type MitigationTrendView struct {
	Attempts    []MitigationTrendNode  `json:"attempts"`
	Comparisons []MitigationComparison `json:"comparisons"`
	Overall     string                 `json:"overall"`
}

func buildMitigationTrend(attempts []casework.MitigationAttempt) MitigationTrendView {
	out := MitigationTrendView{Attempts: []MitigationTrendNode{}, Comparisons: []MitigationComparison{}, Overall: "NO_DATA"}
	for _, attempt := range attempts {
		node := MitigationTrendNode{AttemptID: attempt.ID, PreviousAttemptID: attempt.PreviousAttemptID, MeasureType: attempt.MeasureType, MeasureDescription: attempt.MeasureDescription, Status: attempt.Status}
		if len(attempt.Verifications) > 0 {
			v := attempt.Verifications[0]
			node.Reviewer, node.RemediationReason = v.Reviewer, v.RemediationReason
			node.Metrics = map[string]MitigationMetricView{}
			for metric, difference := range v.Differences {
				exceedance, margin := 0.0, 0.0
				if difference > 0 {
					exceedance = difference
				} else {
					margin = -difference
				}
				node.Metrics[metric] = MitigationMetricView{ObservedDifference: difference, Exceedance: exceedance, ComplianceMargin: margin}
			}
		}
		out.Attempts = append(out.Attempts, node)
	}
	comparable, positive, negative := 0, 0, 0
	for i := 1; i < len(out.Attempts); i++ {
		prev, current := out.Attempts[i-1], out.Attempts[i]
		comparison := MitigationComparison{FromAttemptID: prev.AttemptID, ToAttemptID: current.AttemptID, Improvements: map[string]float64{}}
		for metric, before := range prev.Metrics {
			if after, ok := current.Metrics[metric]; ok {
				improvement := before.ObservedDifference - after.ObservedDifference
				comparison.Improvements[metric] = improvement
				if improvement > 0 {
					positive++
				} else if improvement < 0 {
					negative++
				}
			}
		}
		comparison.Comparable = len(comparison.Improvements) > 0
		if !comparison.Comparable {
			comparison.Reason = "相邻尝试缺少共同指标"
			comparison.Improvements = nil
		} else {
			comparable++
		}
		out.Comparisons = append(out.Comparisons, comparison)
	}
	if len(attempts) > 0 {
		out.Overall = "INSUFFICIENT_ATTEMPTS"
	}
	if len(attempts) > 1 && comparable == 0 {
		out.Overall = "NOT_COMPARABLE"
	} else if comparable > 0 && positive > 0 && negative == 0 {
		out.Overall = "IMPROVING"
	} else if comparable > 0 && negative > 0 && positive == 0 {
		out.Overall = "WORSENING"
	} else if comparable > 0 && positive == 0 && negative == 0 {
		out.Overall = "UNCHANGED"
	} else if comparable > 0 {
		out.Overall = "MIXED"
	}
	return out
}

type AuditQuery struct {
	FromRevision int
	EventType    string
	PageSize     int
	Cursor       string
}
type IntegrityView struct {
	Continuous   bool   `json:"continuous"`
	DigestsValid bool   `json:"digests_valid"`
	Status       string `json:"status"`
}
type AuditPage struct {
	Events     []casework.AuditEvent `json:"events"`
	NextCursor string                `json:"next_cursor,omitempty"`
	Integrity  IntegrityView         `json:"integrity"`
}

func (s *Service) AuditPage(id string, q AuditQuery) (*AuditPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Get(id); err != nil {
		return nil, err
	}
	if q.PageSize == 0 {
		q.PageSize = 50
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		return nil, casework.Invalid("page_size 必须在 1 到 100 之间")
	}
	offset := 0
	if q.Cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(q.Cursor)
		if err != nil {
			return nil, casework.Invalid("cursor 无效")
		}
		offset, err = strconv.Atoi(string(b))
		if err != nil || offset < 0 {
			return nil, casework.Invalid("cursor 无效")
		}
	}
	all := ordered(s.repo.Audit(id))
	filtered := []casework.AuditEvent{}
	for _, e := range all {
		if e.Revision < q.FromRevision {
			continue
		}
		if q.EventType != "" && e.EventType != q.EventType {
			continue
		}
		filtered = append(filtered, e)
	}
	if offset > len(filtered) {
		return nil, casework.Invalid("cursor 已过期")
	}
	end := offset + q.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	next := ""
	if end < len(filtered) {
		next = base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	}
	ok := s.repo.AuditIntegrity(id)
	status := "OK"
	if !ok {
		status = "ANOMALY"
	}
	return &AuditPage{Events: filtered[offset:end], NextCursor: next, Integrity: IntegrityView{Continuous: ok, DigestsValid: ok, Status: status}}, nil
}
func (s *Service) Audit(id string) ([]casework.AuditEvent, error) {
	p, err := s.AuditPage(id, AuditQuery{PageSize: 100})
	if err != nil {
		return nil, err
	}
	return p.Events, nil
}
func ordered(events []casework.AuditEvent) []casework.AuditEvent {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Revision != events[j].Revision {
			return events[i].Revision < events[j].Revision
		}
		if !events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].OccurredAt.Before(events[j].OccurredAt)
		}
		return events[i].EventID < events[j].EventID
	})
	return events
}
