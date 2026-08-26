package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
)

type CaseListQuery struct {
	States           []casework.State
	Severities       []string
	AntennaID        string
	ObservationStart *time.Time
	ObservationEnd   *time.Time
	HasBlockers      *bool
	Sort             string
	PageSize         int
	Cursor           string
}

type CaseQueueItem struct {
	CaseID          string         `json:"case_id"`
	Revision        int            `json:"revision"`
	Stage           casework.State `json:"stage"`
	Severity        string         `json:"severity"`
	AntennaID       string         `json:"antenna_id"`
	NextAction      string         `json:"next_action,omitempty"`
	BlockerSummary  []string       `json:"blocker_summary"`
	LatestAuditTime time.Time      `json:"latest_audit_time"`
}

type CaseFacets struct {
	States     map[casework.State]int `json:"states"`
	Severities map[string]int         `json:"severities"`
}

type CaseListPage struct {
	Items      []CaseQueueItem `json:"items"`
	Facets     CaseFacets      `json:"facets"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type caseCursor struct {
	FilterHash string `json:"filter_hash"`
	AuditTime  string `json:"audit_time"`
	CaseID     string `json:"case_id"`
	Revision   int    `json:"revision"`
}

type signedCursor struct {
	Position caseCursor `json:"position"`
	Checksum string     `json:"checksum"`
}

var validStates = map[casework.State]bool{
	casework.StateDetected: true, casework.StateTriaged: true, casework.StatePlanned: true,
	casework.StateEvidence: true, casework.StateHypothesis: true, casework.StateMitigated: true,
	casework.StateVerified: true, casework.StateClosed: true,
}

var validSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

func (s *Service) ListCases(q CaseListQuery) (*CaseListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := normalizeListQuery(&q); err != nil {
		return nil, err
	}
	filterHash := listFilterHash(q)
	position, err := decodeCaseCursor(q.Cursor, filterHash)
	if err != nil {
		return nil, err
	}
	items := []CaseQueueItem{}
	for _, c := range s.repo.Cases() {
		if !matchesCase(c, q) {
			continue
		}
		progress := buildProgress(c)
		lastAudit := c.CreatedAt
		events := ordered(s.repo.Audit(c.ID))
		if len(events) > 0 {
			lastAudit = events[len(events)-1].OccurredAt
		}
		nextAction := ""
		if len(progress.NextActions) > 0 {
			nextAction = progress.NextActions[0]
		}
		items = append(items, CaseQueueItem{CaseID: c.ID, Revision: c.Revision, Stage: c.State, Severity: c.Severity, AntennaID: c.AntennaID, NextAction: nextAction, BlockerSummary: append([]string(nil), progress.Blockers...), LatestAuditTime: lastAudit})
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].LatestAuditTime.Equal(items[j].LatestAuditTime) {
			if q.Sort == "recent_audit_asc" {
				return items[i].LatestAuditTime.Before(items[j].LatestAuditTime)
			}
			return items[i].LatestAuditTime.After(items[j].LatestAuditTime)
		}
		return items[i].CaseID < items[j].CaseID
	})
	facets := newCaseFacets()
	for _, item := range items {
		facets.States[item.Stage]++
		if item.Severity != "" {
			facets.Severities[item.Severity]++
		}
	}
	start := 0
	if position != nil {
		found := false
		for i, item := range items {
			if item.CaseID == position.CaseID && item.Revision == position.Revision && item.LatestAuditTime.UTC().Format(time.RFC3339Nano) == position.AuditTime {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return nil, casework.Invalid("cursor 位置已失效")
		}
	}
	end := start + q.PageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := append([]CaseQueueItem(nil), items[start:end]...)
	next := ""
	if end < len(items) && len(pageItems) > 0 {
		last := pageItems[len(pageItems)-1]
		next = encodeCaseCursor(caseCursor{FilterHash: filterHash, AuditTime: last.LatestAuditTime.UTC().Format(time.RFC3339Nano), CaseID: last.CaseID, Revision: last.Revision})
	}
	return &CaseListPage{Items: pageItems, Facets: facets, NextCursor: next}, nil
}

func normalizeListQuery(q *CaseListQuery) error {
	q.AntennaID = strings.TrimSpace(q.AntennaID)
	q.Sort = strings.TrimSpace(q.Sort)
	if q.Sort == "" {
		q.Sort = "recent_audit_desc"
	}
	if q.Sort == "recent_audit" || q.Sort == "latest_audit" || q.Sort == "latest_audit_desc" {
		q.Sort = "recent_audit_desc"
	}
	if q.Sort == "latest_audit_asc" {
		q.Sort = "recent_audit_asc"
	}
	if q.Sort != "recent_audit_desc" && q.Sort != "recent_audit_asc" {
		return casework.Invalid("sort 仅允许 recent_audit_desc 或 recent_audit_asc")
	}
	if q.PageSize == 0 {
		q.PageSize = 50
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		return casework.Invalid("page_size 必须在 1 到 100 之间")
	}
	states := map[casework.State]bool{}
	for _, state := range q.States {
		state = casework.State(strings.ToUpper(strings.TrimSpace(string(state))))
		if !validStates[state] {
			return casework.Invalid("state 包含未知案件状态")
		}
		states[state] = true
	}
	q.States = q.States[:0]
	for state := range states {
		q.States = append(q.States, state)
	}
	sort.Slice(q.States, func(i, j int) bool { return q.States[i] < q.States[j] })
	severities := map[string]bool{}
	for _, severity := range q.Severities {
		severity = strings.ToUpper(strings.TrimSpace(severity))
		if !validSeverities[severity] {
			return casework.Invalid("severity 包含未知严重度")
		}
		severities[severity] = true
	}
	q.Severities = q.Severities[:0]
	for severity := range severities {
		q.Severities = append(q.Severities, severity)
	}
	sort.Strings(q.Severities)
	if q.ObservationStart != nil && q.ObservationEnd != nil && !q.ObservationStart.Before(*q.ObservationEnd) {
		return casework.Invalid("观测时段开始时间必须早于结束时间")
	}
	if q.HasBlockers != nil && *q.HasBlockers && len(q.States) == 1 && q.States[0] == casework.StateClosed {
		return casework.Invalid("CLOSED 状态与 has_blockers=true 互相矛盾")
	}
	return nil
}

func matchesCase(c *casework.InterferenceCase, q CaseListQuery) bool {
	if len(q.States) == 0 {
		if c.State == casework.StateClosed {
			return false
		}
	} else if !containsState(q.States, c.State) {
		return false
	}
	if len(q.Severities) > 0 && !containsString(q.Severities, c.Severity) {
		return false
	}
	if q.AntennaID != "" && !strings.EqualFold(q.AntennaID, c.AntennaID) {
		return false
	}
	if q.ObservationStart != nil && !c.ObservationWindow.End.After(*q.ObservationStart) {
		return false
	}
	if q.ObservationEnd != nil && !c.ObservationWindow.Start.Before(*q.ObservationEnd) {
		return false
	}
	if q.HasBlockers != nil && (len(buildProgress(c).Blockers) > 0) != *q.HasBlockers {
		return false
	}
	return true
}

func containsState(states []casework.State, wanted casework.State) bool {
	for _, state := range states {
		if state == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func newCaseFacets() CaseFacets {
	states := map[casework.State]int{}
	for state := range validStates {
		states[state] = 0
	}
	severities := map[string]int{}
	for severity := range validSeverities {
		severities[severity] = 0
	}
	return CaseFacets{States: states, Severities: severities}
}

func listFilterHash(q CaseListQuery) string {
	payload := struct {
		States           []casework.State `json:"states"`
		Severities       []string         `json:"severities"`
		AntennaID        string           `json:"antenna_id"`
		ObservationStart string           `json:"observation_start"`
		ObservationEnd   string           `json:"observation_end"`
		HasBlockers      *bool            `json:"has_blockers"`
		Sort             string           `json:"sort"`
	}{States: q.States, Severities: q.Severities, AntennaID: strings.ToLower(q.AntennaID), HasBlockers: q.HasBlockers, Sort: q.Sort}
	if q.ObservationStart != nil {
		payload.ObservationStart = q.ObservationStart.UTC().Format(time.RFC3339Nano)
	}
	if q.ObservationEnd != nil {
		payload.ObservationEnd = q.ObservationEnd.UTC().Format(time.RFC3339Nano)
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func encodeCaseCursor(position caseCursor) string {
	b, _ := json.Marshal(position)
	sum := sha256.Sum256(b)
	envelope, _ := json.Marshal(signedCursor{Position: position, Checksum: hex.EncodeToString(sum[:])})
	return base64.RawURLEncoding.EncodeToString(envelope)
}

func decodeCaseCursor(raw, expectedFilterHash string) (*caseCursor, error) {
	if raw == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, casework.Invalid("cursor 损坏")
	}
	var envelope signedCursor
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, casework.Invalid("cursor 损坏")
	}
	positionBytes, _ := json.Marshal(envelope.Position)
	sum := sha256.Sum256(positionBytes)
	if envelope.Checksum != hex.EncodeToString(sum[:]) || envelope.Position.CaseID == "" || envelope.Position.Revision < 1 {
		return nil, casework.Invalid("cursor 损坏")
	}
	if envelope.Position.FilterHash != expectedFilterHash {
		return nil, casework.Invalid("cursor 与当前筛选条件不匹配")
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Position.AuditTime); err != nil {
		return nil, casework.Invalid("cursor 损坏")
	}
	return &envelope.Position, nil
}
