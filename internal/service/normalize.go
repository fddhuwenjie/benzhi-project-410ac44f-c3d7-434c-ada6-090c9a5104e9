package service

import (
	"strings"

	"github.com/benzhi/relay-survey/internal/casework"
)

func NormalizeCreate(in CreateInput) CreateInput {
	in.AntennaID = strings.TrimSpace(in.AntennaID)
	in.InitialFeature = strings.Join(strings.Fields(in.InitialFeature), " ")
	return in
}

// normalizeCreateInput produces a canonical representation of a create request
// payload for request fingerprinting. It folds the association sub-object and
// top-level association fields into a single normalized form so that equivalent
// payloads share the same fingerprint regardless of which field carried them.
func normalizeCreateInput(in CreateInput) createInputFingerprint {
	out := createInputFingerprint{
		ObservationWindow:      in.ObservationWindow,
		FrequencyRangeHz:       in.FrequencyRangeHz,
		AntennaID:              in.AntennaID,
		InitialFeature:         in.InitialFeature,
		AssociationDisposition: strings.ToUpper(strings.TrimSpace(in.AssociationDisposition)),
		RelatedCaseID:          strings.TrimSpace(in.RelatedCaseID),
	}
	if in.Association != nil {
		if out.AssociationDisposition == "" {
			out.AssociationDisposition = strings.ToUpper(strings.TrimSpace(in.Association.Disposition))
		}
		if out.RelatedCaseID == "" {
			out.RelatedCaseID = strings.TrimSpace(in.Association.RelatedCaseID)
		}
	}
	if out.AssociationDisposition == "" {
		out.AssociationDisposition = "INDEPENDENT"
	}
	return out
}

type createInputFingerprint struct {
	ObservationWindow      casework.ObservationWindow `json:"observation_window"`
	FrequencyRangeHz       [2]float64                 `json:"frequency_range_hz"`
	AntennaID              string                     `json:"antenna_id"`
	InitialFeature         string                     `json:"initial_feature"`
	AssociationDisposition string                     `json:"association_disposition"`
	RelatedCaseID          string                     `json:"related_case_id"`
}

func NormalizePlan(plan casework.InvestigationPlan) casework.InvestigationPlan {
	plan.Owner = strings.TrimSpace(plan.Owner)
	plan.ReplacementReason = strings.TrimSpace(plan.ReplacementReason)
	for i := range plan.MeasurementSites {
		plan.MeasurementSites[i] = strings.TrimSpace(plan.MeasurementSites[i])
	}
	for i := range plan.EquipmentIDs {
		plan.EquipmentIDs[i] = strings.TrimSpace(plan.EquipmentIDs[i])
	}
	return plan
}
