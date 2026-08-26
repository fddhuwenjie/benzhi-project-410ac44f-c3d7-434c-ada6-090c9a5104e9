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
