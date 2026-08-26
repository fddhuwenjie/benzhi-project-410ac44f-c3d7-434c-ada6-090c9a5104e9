package casework

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func Fingerprint(antenna string, freq [2]float64, w ObservationWindow, feature string) string {
	s := fmt.Sprintf("%s|%.3f|%.3f|%s|%s", antenna, freq[0], freq[1], w.Start.UTC().Format(time.RFC3339), normalizeFeature(feature))
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func Similarity(existing InterferenceCase, antenna string, freq [2]float64, w ObservationWindow, feature string) SimilarityCandidate {
	a := 0.0
	if strings.EqualFold(strings.TrimSpace(existing.AntennaID), strings.TrimSpace(antenna)) {
		a = 1
	}
	intersection := math.Max(0, math.Min(existing.FrequencyRangeHz[1], freq[1])-math.Max(existing.FrequencyRangeHz[0], freq[0]))
	denom := math.Min(existing.FrequencyRangeHz[1]-existing.FrequencyRangeHz[0], freq[1]-freq[0])
	overlap := 0.0
	if denom > 0 {
		overlap = intersection / denom
	}
	gap := windowGap(existing.ObservationWindow, w)
	window := 0.0
	if gap == 0 {
		window = 1
	} else if gap <= 10*time.Minute {
		window = .9
	} else if gap <= 24*time.Hour {
		window = math.Max(0, 1-gap.Hours()/24)
	}
	f := textSimilarity(existing.InitialFeature, feature)
	score := round4(.35*a + .30*overlap + .20*window + .15*f)
	return SimilarityCandidate{CaseID: existing.ID, Score: score, AntennaScore: a, FrequencyOverlap: round4(overlap), WindowScore: round4(window), FeatureScore: round4(f), State: existing.State}
}
func normalizeFeature(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func textSimilarity(a, b string) float64 {
	aa := strings.Fields(normalizeFeature(a))
	bb := strings.Fields(normalizeFeature(b))
	if len(aa) == 0 || len(bb) == 0 {
		return 0
	}
	set := map[string]int{}
	for _, v := range aa {
		set[v] |= 1
	}
	for _, v := range bb {
		set[v] |= 2
	}
	inter, union := 0, 0
	for _, v := range set {
		union++
		if v == 3 {
			inter++
		}
	}
	return float64(inter) / float64(union)
}
func windowGap(a, b ObservationWindow) time.Duration {
	if a.End.Before(b.Start) {
		return b.Start.Sub(a.End)
	}
	if b.End.Before(a.Start) {
		return a.Start.Sub(b.End)
	}
	return 0
}
func WindowsOverlap(a, b ObservationWindow) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func Severity(t Triage) string {
	score := 0
	if t.AffectedObservations >= 5 {
		score += 2
	} else if t.AffectedObservations > 0 {
		score++
	}
	if t.OccupiedBandwidthHz >= 1e6 {
		score += 2
	} else if t.OccupiedBandwidthHz > 0 {
		score++
	}
	if t.PersistenceMinutes >= 60 {
		score += 2
	} else if t.PersistenceMinutes >= 10 {
		score++
	}
	if score >= 5 {
		return "CRITICAL"
	}
	if score >= 3 {
		return "HIGH"
	}
	if score >= 1 {
		return "MEDIUM"
	}
	return "LOW"
}
func ScoreTriage(t Triage, caseBandwidth float64) (TriageScore, string, error) {
	if t.AffectedObservations < 0 || t.OccupiedBandwidthHz < 0 || t.OccupiedBandwidthHz > caseBandwidth || t.PersistenceMinutes <= 0 || strings.TrimSpace(t.Rationale) == "" {
		return TriageScore{}, "", Invalid("研判指标超出案件观测事实或缺少研判理由")
	}
	obs := 0
	if t.AffectedObservations > 0 {
		obs = 1
	}
	if t.AffectedObservations >= 5 {
		obs = 2
	}
	ratio := t.OccupiedBandwidthHz / caseBandwidth
	bw := 0
	if ratio > 0 {
		bw = 1
	}
	if ratio >= .2 {
		bw = 2
	}
	persist := 0
	if t.PersistenceMinutes > 0 {
		persist = 1
	}
	if t.PersistenceMinutes >= 60 {
		persist = 2
	}
	total := obs + bw + persist
	sev := "LOW"
	if total >= 5 {
		sev = "CRITICAL"
	} else if total >= 3 {
		sev = "HIGH"
	} else if total >= 1 {
		sev = "MEDIUM"
	}
	return TriageScore{AffectedObservations: ScoreItem{obs, fmt.Sprintf("受影响观测 %d 次", t.AffectedObservations)}, BandwidthRatio: ScoreItem{bw, fmt.Sprintf("占案件频段 %.1f%%", ratio*100)}, Persistence: ScoreItem{persist, fmt.Sprintf("持续 %d 分钟", t.PersistenceMinutes)}, Total: total}, sev, nil
}

func BuildPlan(p InvestigationPlan, c InterferenceCase) (InvestigationPlan, error) {
	if len(p.MeasurementSites) == 0 || len(p.EquipmentIDs) == 0 || len(p.MeasurementSites) != len(p.EquipmentIDs) || strings.TrimSpace(p.Owner) == "" || len(p.StopConditions) == 0 {
		return p, Invalid("位置、设备必须非空并一一对应，且责任人与停止条件齐备")
	}
	if !p.TimeWindow.Start.Before(p.TimeWindow.End) {
		return p, Invalid("计划结束时间必须晚于开始时间")
	}
	if p.TimeWindow.Start.Before(c.ObservationWindow.Start.Add(-24 * time.Hour)) {
		return p, Invalid("计划开始时间早于允许边界")
	}
	sites, equipment := map[string]bool{}, map[string]bool{}
	p.Items = nil
	for i := range p.MeasurementSites {
		site := strings.TrimSpace(p.MeasurementSites[i])
		device := strings.TrimSpace(p.EquipmentIDs[i])
		if site == "" || device == "" {
			return p, Invalid("计划项不得为空")
		}
		if sites[site] {
			return p, Invalid("测量位置不得重复")
		}
		if equipment[device] {
			return p, Invalid("设备不得重复")
		}
		sites[site] = true
		equipment[device] = true
		p.MeasurementSites[i] = site
		p.EquipmentIDs[i] = device
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s", c.ID, p.ID, p.Revision, site, device, p.TimeWindow.Start.UTC().Format(time.RFC3339Nano), p.TimeWindow.End.UTC().Format(time.RFC3339Nano))))
		id := "pi-" + hex.EncodeToString(sum[:8])
		p.Items = append(p.Items, PlanItem{ID: id, MeasurementSite: site, EquipmentID: device, TimeWindow: p.TimeWindow, Owner: strings.TrimSpace(p.Owner), FrequencyRangeHz: c.FrequencyRangeHz})
	}
	for _, v := range p.StopConditions {
		if strings.TrimSpace(v) == "" {
			return p, Invalid("停止条件不得为空")
		}
	}
	p.Coverage = PlanCoverageSummary{TotalItems: len(p.Items), CoveredItems: len(p.Items), CriticalFrequencyCovered: true}
	p.ResourceValidation = ResourceValidation{Checked: true, Conflicts: []ResourceConflict{}}
	p.ReplacementReason = ""
	return p, nil
}
func ValidatePlan(p InvestigationPlan, c InterferenceCase) error {
	_, err := BuildPlan(p, c)
	return err
}

func ValidateEvidence(e EvidenceRecord, p InvestigationPlan) error {
	var item *PlanItem
	for i := range p.Items {
		if p.Items[i].ID == e.PlanItemID {
			item = &p.Items[i]
			break
		}
	}
	if item == nil {
		return Invalid("plan_item_id 不属于调查计划")
	}
	if e.ID == "" || e.PlanItemID == "" || e.SourceDevice == "" {
		return Invalid("证据元数据不完整")
	}
	if e.SourceDevice != item.EquipmentID {
		return Invalid("source_device 与计划设备不一致")
	}
	if e.CapturedAt.Before(item.TimeWindow.Start) || e.CapturedAt.After(item.TimeWindow.End) {
		return Invalid("证据采集时间不在计划项窗口")
	}
	if e.Kind != EvidenceSpectrum && e.Kind != EvidenceReading && e.Kind != EvidenceObservation {
		return Invalid("证据 kind 不在允许集合")
	}
	if len(e.ContentHash) != 64 {
		return Invalid("content_hash 必须是 64 位十六进制 SHA-256 摘要")
	}
	if _, err := hex.DecodeString(e.ContentHash); err != nil {
		return Invalid("content_hash 必须是合法十六进制摘要")
	}
	return nil
}
func Coverage(c InterferenceCase) EvidenceCoverage {
	out := EvidenceCoverage{PlanItems: map[string][]string{}, Gaps: []EvidenceGap{}}
	if c.Plan == nil {
		return out
	}
	kinds := []string{EvidenceSpectrum, EvidenceReading, EvidenceObservation}
	for _, item := range c.Plan.Items {
		seen := map[string]bool{}
		for _, e := range c.Evidence {
			if e.PlanItemID == item.ID {
				seen[e.Kind] = true
			}
		}
		got := []string{}
		missing := []string{}
		for _, k := range kinds {
			if seen[k] {
				got = append(got, k)
			} else {
				missing = append(missing, k)
			}
		}
		out.PlanItems[item.ID] = got
		if len(missing) > 0 {
			out.Gaps = append(out.Gaps, EvidenceGap{item.ID, missing})
		}
	}
	out.Complete = len(c.Plan.Items) > 0 && len(out.Gaps) == 0
	return out
}
func EvidenceComplete(c InterferenceCase) bool { return Coverage(c).Complete }

func BuildTest(id string, w ObservationWindow, baseline, active map[string]float64, existing []ControlledTest) (ControlledTest, error) {
	if !w.Start.Before(w.End) || len(baseline) == 0 || len(baseline) != len(active) {
		return ControlledTest{}, Invalid("受控测试窗口或指标无效")
	}
	for _, t := range existing {
		if WindowsOverlap(w, t.Window) {
			return ControlledTest{}, Invalid("受控测试窗口不得重叠")
		}
	}
	keys := make([]string, 0, len(baseline))
	for k, b := range baseline {
		a, ok := active[k]
		if !ok || math.IsNaN(a) || math.IsInf(a, 0) || math.IsNaN(b) || math.IsInf(b, 0) {
			return ControlledTest{}, Invalid("基线与激活指标键集合必须一致且数值有限")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	total := 0.0
	direction := 0
	for _, k := range keys {
		d := active[k] - baseline[k]
		den := math.Abs(baseline[k])
		if den < 1e-9 {
			den = 1
		}
		total += math.Min(1, math.Abs(d)/den)
		if math.Abs(d) > 1e-9 {
			cur := 1
			if d < 0 {
				cur = -1
			}
			if direction == 0 {
				direction = cur
			} else if direction != cur {
				direction = 2
			}
		}
	}
	if direction == 2 {
		direction = 0
	}
	return ControlledTest{ID: id, Window: w, BaselineMetrics: baseline, ActiveMetrics: active, ChangeMagnitude: round4(total / float64(len(keys))), Direction: direction}, nil
}
func CandidateAssessment(c *SourceCandidate) {
	if len(c.Tests) == 0 {
		c.CorrelationScore = 0
		return
	}
	sum := 0.0
	for _, t := range c.Tests {
		sum += t.ChangeMagnitude
	}
	c.CorrelationScore = round4(sum / float64(len(c.Tests)))
}
func CandidateMissing(c SourceCandidate) []string {
	m := []string{}
	if len(c.Tests) < 2 {
		m = append(m, "至少两轮测试")
	}
	if len(c.Tests) >= 2 {
		d := c.Tests[0].Direction
		for _, t := range c.Tests[1:] {
			if d == 0 || t.Direction != d {
				m = append(m, "测试变化方向一致")
				break
			}
		}
	}
	if c.CorrelationScore < .7 {
		m = append(m, "累计相关性达到 0.7")
	}
	if strings.TrimSpace(c.ExclusionNotes) == "" {
		m = append(m, "排他说明")
	}
	return m
}
func Confirmable(h SourceHypothesis) bool {
	return h.CorrelationScore >= .7 && h.Repeatable && strings.TrimSpace(h.ExclusionNotes) != ""
}

func VerificationPass(v MitigationVerification) bool {
	if len(v.Thresholds) == 0 {
		return false
	}
	for k, t := range v.Thresholds {
		x, ok := v.ObservedMetrics[k]
		if !ok || math.IsNaN(x) || math.IsInf(x, 0) || x > t {
			return false
		}
	}
	return true
}
