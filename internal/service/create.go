package service

import (
	"sort"
	"strings"
	"time"

	"github.com/benzhi/relay-survey/internal/casework"
)

type AssociationInput struct {
	Disposition   string `json:"disposition"`
	RelatedCaseID string `json:"related_case_id"`
}
type CreateInput struct {
	ObservationWindow      casework.ObservationWindow `json:"observation_window"`
	FrequencyRangeHz       [2]float64                 `json:"frequency_range_hz"`
	AntennaID              string                     `json:"antenna_id"`
	InitialFeature         string                     `json:"initial_feature"`
	AssociationDisposition string                     `json:"association_disposition,omitempty"`
	RelatedCaseID          string                     `json:"related_case_id,omitempty"`
	Association            *AssociationInput          `json:"association,omitempty"`
}

func (s *Service) Create(m Meta, in CreateInput) (*casework.InterferenceCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	in = NormalizeCreate(in)
	if err := s.check(m); err != nil {
		return nil, err
	}
	if in.Association != nil {
		if in.AssociationDisposition == "" {
			in.AssociationDisposition = in.Association.Disposition
		}
		if in.RelatedCaseID == "" {
			in.RelatedCaseID = strings.TrimSpace(in.Association.RelatedCaseID)
		}
	}
	candidates := []casework.SimilarityCandidate{}
	now := time.Now().UTC()
	for _, old := range s.repo.Cases() {
		if old.State == casework.StateClosed && (old.ClosedAt == nil || now.Sub(*old.ClosedAt) > 30*24*time.Hour) {
			continue
		}
		match := casework.Similarity(*old, in.AntennaID, in.FrequencyRangeHz, in.ObservationWindow, in.InitialFeature)
		if match.Score >= .55 {
			candidates = append(candidates, match)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].CaseID < candidates[j].CaseID
		}
		return candidates[i].Score > candidates[j].Score
	})
	if raw, ok := s.repo.Idempotent(m.RequestID); ok {
		return decodeResult(raw)
	}
	disposition := strings.ToUpper(strings.TrimSpace(in.AssociationDisposition))
	if disposition == "" {
		disposition = "INDEPENDENT"
	}
	if disposition != "INDEPENDENT" && disposition != "LINK" {
		return nil, casework.Invalid("association_disposition 仅允许 LINK 或 INDEPENDENT")
	}
	if disposition == "LINK" {
		if in.RelatedCaseID == "" {
			return nil, casework.Invalid("LINK 处置必须指定 related_case_id")
		}
		if _, ok := s.repo.GetCase(in.RelatedCaseID); !ok {
			return nil, casework.NotFound(in.RelatedCaseID)
		}
	} else if in.RelatedCaseID != "" {
		return nil, casework.Invalid("INDEPENDENT 处置不得指定 related_case_id")
	}
	c, err := casework.NewCase(newID(), in.AntennaID, in.FrequencyRangeHz, in.ObservationWindow, in.InitialFeature)
	if err != nil {
		return nil, err
	}
	if in.RelatedCaseID == c.ID {
		return nil, casework.Invalid("案件不得关联自身")
	}
	c.Association = casework.AssociationDecision{Disposition: disposition, RelatedCaseID: in.RelatedCaseID, Candidates: candidates}
	summary := map[string]any{"fingerprint": c.Fingerprint, "candidate_matches": candidates, "association_disposition": disposition, "related_case_id": in.RelatedCaseID}
	e := makeEvent(m, c, "", "CASE_CREATED", summary)
	if err = s.repo.Commit(c, e, m.RequestID, c); err != nil {
		return nil, err
	}
	return c, nil
}
