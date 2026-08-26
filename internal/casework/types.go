package casework

import (
	"encoding/json"
	"time"
)

type State string

const (
	StateDetected   State = "DETECTED"
	StateTriaged    State = "TRIAGED"
	StatePlanned    State = "PLANNED"
	StateEvidence   State = "EVIDENCE_COLLECTED"
	StateHypothesis State = "HYPOTHESIS_CONFIRMED"
	StateMitigated  State = "MITIGATED"
	StateVerified   State = "VERIFIED"
	StateClosed     State = "CLOSED"
)

type ObservationWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type SimilarityCandidate struct {
	CaseID           string  `json:"case_id"`
	Score            float64 `json:"score"`
	AntennaScore     float64 `json:"antenna_score"`
	FrequencyOverlap float64 `json:"frequency_overlap"`
	WindowScore      float64 `json:"window_score"`
	FeatureScore     float64 `json:"feature_score"`
	State            State   `json:"state"`
}
type AssociationDecision struct {
	Disposition   string                `json:"disposition"`
	RelatedCaseID string                `json:"related_case_id,omitempty"`
	Candidates    []SimilarityCandidate `json:"candidates,omitempty"`
}

type ScoreItem struct {
	Score       int    `json:"score"`
	Explanation string `json:"explanation"`
}
type TriageScore struct {
	AffectedObservations ScoreItem `json:"affected_observations"`
	BandwidthRatio       ScoreItem `json:"bandwidth_ratio"`
	Persistence          ScoreItem `json:"persistence"`
	Total                int       `json:"total"`
}
type Triage struct {
	AffectedObservations int         `json:"affected_observations"`
	OccupiedBandwidthHz  float64     `json:"occupied_bandwidth_hz"`
	PersistenceMinutes   int         `json:"persistence_minutes"`
	Rationale            string      `json:"rationale"`
	BandwidthRatio       float64     `json:"bandwidth_ratio,omitempty"`
	ScoreBreakdown       TriageScore `json:"score_breakdown"`
	Severity             string      `json:"severity,omitempty"`
	TriagedBy            string      `json:"triaged_by,omitempty"`
}

type PlanItem struct {
	ID               string            `json:"plan_item_id"`
	MeasurementSite  string            `json:"measurement_site"`
	EquipmentID      string            `json:"equipment_id"`
	TimeWindow       ObservationWindow `json:"time_window"`
	Owner            string            `json:"owner"`
	FrequencyRangeHz [2]float64        `json:"frequency_range_hz"`
}
type PlanCoverageSummary struct {
	TotalItems               int  `json:"total_items"`
	CoveredItems             int  `json:"covered_items"`
	CriticalFrequencyCovered bool `json:"critical_frequency_covered"`
}
type ResourceValidation struct {
	Checked   bool               `json:"checked"`
	Conflicts []ResourceConflict `json:"conflicts"`
}
type ResourceConflict struct {
	CaseID      string `json:"case_id"`
	PlanItemID  string `json:"plan_item_id"`
	EquipmentID string `json:"equipment_id"`
}
type InvestigationPlan struct {
	ID                 string              `json:"id"`
	CaseID             string              `json:"case_id"`
	MeasurementSites   []string            `json:"measurement_sites"`
	EquipmentIDs       []string            `json:"equipment_ids"`
	TimeWindow         ObservationWindow   `json:"time_window"`
	Owner              string              `json:"owner"`
	StopConditions     []string            `json:"stop_conditions"`
	Revision           int                 `json:"revision"`
	Items              []PlanItem          `json:"plan_items"`
	Coverage           PlanCoverageSummary `json:"coverage_summary"`
	ResourceValidation ResourceValidation  `json:"resource_validation"`
	ReplacementReason  string              `json:"replacement_reason,omitempty"`
}

const (
	EvidenceSpectrum    = "spectrum"
	EvidenceReading     = "device_reading"
	EvidenceObservation = "field_observation"
)

type EvidenceRecord struct {
	ID           string             `json:"id"`
	CaseID       string             `json:"case_id"`
	PlanItemID   string             `json:"plan_item_id"`
	Kind         string             `json:"kind"`
	CapturedAt   time.Time          `json:"captured_at"`
	SourceDevice string             `json:"source_device"`
	ContentHash  string             `json:"content_hash"`
	Metrics      map[string]float64 `json:"metrics"`
	Notes        string             `json:"notes"`
}
type EvidenceWithdrawal struct {
	Evidence           EvidenceRecord `json:"evidence"`
	WithdrawnBy        string         `json:"withdrawn_by"`
	CorrectionReason   string         `json:"correction_reason"`
	WithdrawalRevision int            `json:"withdrawal_revision"`
}
type EvidenceGap struct {
	PlanItemID   string   `json:"plan_item_id"`
	MissingKinds []string `json:"missing_kinds"`
}
type EvidenceCoverage struct {
	Complete  bool                `json:"complete"`
	PlanItems map[string][]string `json:"plan_items"`
	Gaps      []EvidenceGap       `json:"gaps"`
}

type ControlledTest struct {
	ID              string             `json:"test_id"`
	Window          ObservationWindow  `json:"window"`
	BaselineMetrics map[string]float64 `json:"baseline_metrics"`
	ActiveMetrics   map[string]float64 `json:"active_metrics"`
	ChangeMagnitude float64            `json:"change_magnitude"`
	Direction       int                `json:"direction"`
}
type SourceCandidate struct {
	ID               string           `json:"id"`
	CandidateSource  string           `json:"candidate_source"`
	Tests            []ControlledTest `json:"tests,omitempty"`
	CorrelationScore float64          `json:"correlation_score"`
	ExclusionNotes   string           `json:"exclusion_notes,omitempty"`
	Decision         string           `json:"decision"`
	DecisionReason   string           `json:"decision_reason,omitempty"`
	ConfirmedBy      string           `json:"confirmed_by,omitempty"`
	ConfirmedAt      *time.Time       `json:"confirmed_at,omitempty"`
}
type SourceHypothesis struct {
	ID               string              `json:"id"`
	CaseID           string              `json:"case_id"`
	CandidateSource  string              `json:"candidate_source"`
	TestWindows      []ObservationWindow `json:"test_windows"`
	BaselineMetrics  map[string]float64  `json:"baseline_metrics"`
	ActiveMetrics    map[string]float64  `json:"active_metrics"`
	CorrelationScore float64             `json:"correlation_score"`
	Repeatable       bool                `json:"repeatable"`
	ExclusionNotes   string              `json:"exclusion_notes"`
	Decision         string              `json:"decision"`
	ConfirmedAt      *time.Time          `json:"confirmed_at,omitempty"`
}

type VerificationResult struct {
	Window            ObservationWindow  `json:"window"`
	Thresholds        map[string]float64 `json:"thresholds"`
	ObservedMetrics   map[string]float64 `json:"observed_metrics"`
	Differences       map[string]float64 `json:"differences"`
	Result            string             `json:"result"`
	Reviewer          string             `json:"reviewer"`
	RemediationReason string             `json:"remediation_reason,omitempty"`
}
type MitigationAttempt struct {
	ID                 string               `json:"attempt_id"`
	PreviousAttemptID  string               `json:"previous_attempt_id,omitempty"`
	MeasureType        string               `json:"measure_type"`
	MeasureDescription string               `json:"measure_description"`
	ImplementedAt      time.Time            `json:"implemented_at"`
	ImplementedBy      string               `json:"implemented_by"`
	Status             string               `json:"status"`
	Verifications      []VerificationResult `json:"verifications,omitempty"`
}
type MitigationVerification struct {
	ID                 string             `json:"id,omitempty"`
	AttemptID          string             `json:"attempt_id,omitempty"`
	PreviousAttemptID  string             `json:"previous_attempt_id,omitempty"`
	CaseID             string             `json:"case_id,omitempty"`
	MeasureType        string             `json:"measure_type,omitempty"`
	MeasureDescription string             `json:"measure_description,omitempty"`
	ImplementedAt      time.Time          `json:"implemented_at,omitempty"`
	VerificationWindow ObservationWindow  `json:"verification_window,omitempty"`
	Thresholds         map[string]float64 `json:"thresholds,omitempty"`
	ObservedMetrics    map[string]float64 `json:"observed_metrics,omitempty"`
	Result             string             `json:"result,omitempty"`
	Reviewer           string             `json:"reviewer,omitempty"`
	RemediationReason  string             `json:"remediation_reason,omitempty"`
}

type ClosureItem struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}
type ClosureReview struct {
	ClosedAt  time.Time     `json:"closed_at"`
	Reviewer  string        `json:"reviewer"`
	Checklist []ClosureItem `json:"checklist"`
}

type InterferenceCase struct {
	ID                   string                   `json:"id"`
	State                State                    `json:"state"`
	Revision             int                      `json:"revision"`
	ObservationWindow    ObservationWindow        `json:"observation_window"`
	FrequencyRangeHz     [2]float64               `json:"frequency_range_hz"`
	AntennaID            string                   `json:"antenna_id"`
	InitialFeature       string                   `json:"initial_feature"`
	Fingerprint          string                   `json:"fingerprint"`
	Severity             string                   `json:"severity"`
	CreatedAt            time.Time                `json:"created_at"`
	ClosedAt             *time.Time               `json:"closed_at,omitempty"`
	Association          AssociationDecision      `json:"association"`
	Impact               *Triage                  `json:"impact,omitempty"`
	Plan                 *InvestigationPlan       `json:"plan,omitempty"`
	Evidence             []EvidenceRecord         `json:"evidence,omitempty"`
	EvidenceWithdrawals  []EvidenceWithdrawal     `json:"evidence_withdrawals,omitempty"`
	EvidenceCoverage     EvidenceCoverage         `json:"evidence_coverage"`
	Hypothesis           *SourceHypothesis        `json:"hypothesis,omitempty"`
	SourceCandidates     []SourceCandidate        `json:"source_candidates,omitempty"`
	ConfirmedCandidateID string                   `json:"confirmed_candidate_id,omitempty"`
	Mitigations          []MitigationVerification `json:"mitigations,omitempty"`
	MitigationAttempts   []MitigationAttempt      `json:"mitigation_attempts,omitempty"`
	Closure              *ClosureReview           `json:"closure,omitempty"`
}

type AuditEvent struct {
	EventID        string          `json:"event_id"`
	CaseID         string          `json:"case_id"`
	RequestID      string          `json:"request_id"`
	Actor          string          `json:"actor"`
	EventType      string          `json:"event_type"`
	FromState      State           `json:"from_state,omitempty"`
	ToState        State           `json:"to_state,omitempty"`
	Revision       int             `json:"revision"`
	OccurredAt     time.Time       `json:"occurred_at"`
	PayloadSummary json.RawMessage `json:"payload_summary,omitempty"`
	PayloadDigest  string          `json:"payload_digest"`
}
