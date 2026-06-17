package types

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ImpactAnalysisResult struct {
	ResultID            string                     `json:"result_id,omitempty"`
	PlanID              string                     `json:"plan_id,omitempty"`
	PatchEffectID       string                     `json:"patch_effect_id,omitempty"`
	Source              string                     `json:"source,omitempty"`
	ChangedSurfaces     []ImpactChangedSurface     `json:"changed_surfaces,omitempty"`
	Edges               []ImpactEdge               `json:"edges,omitempty"`
	VerificationTargets []ImpactVerificationTarget `json:"verification_targets,omitempty"`
	ObligationSet       *ImpactObligationSet       `json:"obligation_set,omitempty"`
	CreatedAt           time.Time                  `json:"created_at,omitempty"`
}

type ImpactChangedSurface struct {
	ID          string `json:"id,omitempty"`
	Kind        string `json:"kind"`
	Path        string `json:"path,omitempty"`
	Role        string `json:"role,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
	LineStart   int    `json:"line_start,omitempty"`
	LineEnd     int    `json:"line_end,omitempty"`
	Source      string `json:"source,omitempty"`
	Strength    string `json:"strength,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

type ImpactEdge struct {
	ID          string `json:"id,omitempty"`
	Kind        string `json:"kind"`
	Relation    string `json:"relation,omitempty"`
	FromPath    string `json:"from_path,omitempty"`
	ToPath      string `json:"to_path,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
	ContractRef string `json:"contract_ref,omitempty"`
	ProbeID     string `json:"probe_id,omitempty"`
	Source      string `json:"source,omitempty"`
	Strength    string `json:"strength,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

type ImpactVerificationTarget struct {
	ID             string `json:"id,omitempty"`
	Kind           string `json:"kind"`
	Path           string `json:"path,omitempty"`
	RelatedPath    string `json:"related_path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	ContractRef    string `json:"contract_ref,omitempty"`
	ProbeID        string `json:"probe_id,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	Strength       string `json:"strength,omitempty"`
	Source         string `json:"source,omitempty"`
	CoverageStatus string `json:"coverage_status,omitempty"`
	EvidenceRef    string `json:"evidence_ref,omitempty"`
}

func NormalizeImpactAnalysisResult(in ImpactAnalysisResult) ImpactAnalysisResult {
	in.ResultID = strings.TrimSpace(in.ResultID)
	in.PlanID = strings.TrimSpace(in.PlanID)
	in.PatchEffectID = strings.TrimSpace(in.PatchEffectID)
	in.Source = strings.TrimSpace(in.Source)
	if in.Source == "" && (len(in.ChangedSurfaces) > 0 || len(in.Edges) > 0 || len(in.VerificationTargets) > 0 || in.ObligationSet != nil) {
		in.Source = "impact_engine"
	}
	in.ChangedSurfaces = normalizeImpactChangedSurfaces(in.ChangedSurfaces)
	in.Edges = normalizeImpactEdges(in.Edges)
	in.VerificationTargets = normalizeImpactVerificationTargets(in.VerificationTargets)
	if in.ObligationSet != nil {
		set := NormalizeImpactObligationSet(*in.ObligationSet)
		if len(set.Obligations) == 0 {
			in.ObligationSet = nil
		} else {
			in.ObligationSet = &set
			if in.PlanID == "" {
				in.PlanID = set.PlanID
			}
		}
	}
	if in.ResultID == "" && (in.PlanID != "" || in.PatchEffectID != "") {
		in.ResultID = impactAnalysisResultID(in.PlanID, in.PatchEffectID)
	}
	return in
}

func ImpactAnalysisResultFromObligationSet(planID, patchEffectID string, set ImpactObligationSet, createdAt time.Time) ImpactAnalysisResult {
	set = NormalizeImpactObligationSet(set)
	result := ImpactAnalysisResult{
		PlanID:        firstNonEmptyImpactAnalysisString(planID, set.PlanID),
		PatchEffectID: strings.TrimSpace(patchEffectID),
		Source:        "impact_engine",
		ObligationSet: &set,
		CreatedAt:     createdAt,
	}
	for _, ob := range set.Obligations {
		if surface := impactChangedSurfaceFromObligation(ob); surface.Kind != "" {
			result.ChangedSurfaces = append(result.ChangedSurfaces, surface)
		}
		if edge := impactEdgeFromObligation(ob); edge.Kind != "" {
			result.Edges = append(result.Edges, edge)
		}
		if target := impactVerificationTargetFromObligation(ob); target.Kind != "" {
			result.VerificationTargets = append(result.VerificationTargets, target)
		}
	}
	return NormalizeImpactAnalysisResult(result)
}

func impactChangedSurfaceFromObligation(ob ImpactObligation) ImpactChangedSurface {
	ob = impactObligation(ob)
	switch ob.Kind {
	case "changed_file":
		return impactChangedSurface(ImpactChangedSurface{
			Kind:        "file",
			Path:        ob.SubjectPath,
			Source:      ob.Source,
			Strength:    string(ob.Strength),
			EvidenceRef: ob.EvidenceRef,
		})
	case "changed_symbol":
		return impactChangedSurface(ImpactChangedSurface{
			Kind:        "symbol",
			Path:        ob.SubjectPath,
			Symbol:      ob.SubjectSymbol,
			Source:      ob.Source,
			Strength:    string(ob.Strength),
			EvidenceRef: ob.EvidenceRef,
		})
	default:
		return ImpactChangedSurface{}
	}
}

func impactEdgeFromObligation(ob ImpactObligation) ImpactEdge {
	ob = impactObligation(ob)
	switch ob.Kind {
	case "dependency", "dependent", "test_surface":
		return impactEdge(ImpactEdge{
			Kind:        ob.Kind,
			Relation:    ob.Relation,
			FromPath:    ob.SubjectPath,
			ToPath:      ob.RelatedPath,
			Source:      ob.Source,
			Strength:    string(ob.Strength),
			EvidenceRef: ob.EvidenceRef,
		})
	case "behavior_contract":
		return impactEdge(ImpactEdge{
			Kind:        ob.Kind,
			Relation:    ob.Relation,
			ContractRef: ob.ContractRef,
			ProbeID:     ob.VerificationProbe,
			Source:      ob.Source,
			Strength:    string(ob.Strength),
			EvidenceRef: ob.EvidenceRef,
		})
	case "effect_followup":
		return impactEdge(ImpactEdge{
			Kind:        ob.Kind,
			Relation:    ob.Relation,
			FromPath:    ob.SubjectPath,
			Source:      ob.Source,
			Strength:    string(ob.Strength),
			EvidenceRef: ob.EvidenceRef,
		})
	default:
		return ImpactEdge{}
	}
}

func impactVerificationTargetFromObligation(ob ImpactObligation) ImpactVerificationTarget {
	ob = impactObligation(ob)
	priority := impactVerificationPriority(ob)
	if priority <= 0 {
		return ImpactVerificationTarget{}
	}
	return impactVerificationTarget(ImpactVerificationTarget{
		Kind:           ob.Kind,
		Path:           ob.SubjectPath,
		RelatedPath:    ob.RelatedPath,
		Symbol:         ob.SubjectSymbol,
		ContractRef:    ob.ContractRef,
		ProbeID:        ob.VerificationProbe,
		Priority:       priority,
		Strength:       string(ob.Strength),
		Source:         ob.Source,
		CoverageStatus: "unverified",
		EvidenceRef:    ob.EvidenceRef,
	})
}

func impactVerificationPriority(ob ImpactObligation) int {
	switch ob.Kind {
	case "behavior_contract":
		return 10
	case "changed_symbol":
		return 20
	case "changed_file":
		return 30
	case "dependent":
		return 40
	case "test_surface":
		return 50
	case "dependency":
		return 60
	case "effect_followup":
		return 70
	default:
		return 0
	}
}

func normalizeImpactChangedSurfaces(in []ImpactChangedSurface) []ImpactChangedSurface {
	out := make([]ImpactChangedSurface, 0, len(in))
	seen := map[string]bool{}
	for _, surface := range in {
		surface = impactChangedSurface(surface)
		if surface.Kind == "" {
			continue
		}
		if seen[surface.ID] {
			continue
		}
		seen[surface.ID] = true
		out = append(out, surface)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func normalizeImpactEdges(in []ImpactEdge) []ImpactEdge {
	out := make([]ImpactEdge, 0, len(in))
	seen := map[string]bool{}
	for _, edge := range in {
		edge = impactEdge(edge)
		if edge.Kind == "" {
			continue
		}
		if seen[edge.ID] {
			continue
		}
		seen[edge.ID] = true
		out = append(out, edge)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func normalizeImpactVerificationTargets(in []ImpactVerificationTarget) []ImpactVerificationTarget {
	out := make([]ImpactVerificationTarget, 0, len(in))
	seen := map[string]bool{}
	for _, target := range in {
		target = impactVerificationTarget(target)
		if target.Kind == "" {
			continue
		}
		if seen[target.ID] {
			continue
		}
		seen[target.ID] = true
		out = append(out, target)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func impactChangedSurface(in ImpactChangedSurface) ImpactChangedSurface {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Path = normalizeImpactObligationPath(in.Path)
	in.Role = strings.TrimSpace(in.Role)
	in.Symbol = strings.TrimSpace(in.Symbol)
	in.Source = strings.TrimSpace(in.Source)
	in.Strength = strings.TrimSpace(in.Strength)
	in.EvidenceRef = strings.TrimSpace(in.EvidenceRef)
	if in.LineStart < 0 {
		in.LineStart = 0
	}
	if in.LineEnd < 0 {
		in.LineEnd = 0
	}
	if in.LineEnd > 0 && in.LineStart > 0 && in.LineEnd < in.LineStart {
		in.LineEnd = in.LineStart
	}
	in.ID = impactChangedSurfaceID(in)
	return in
}

func impactEdge(in ImpactEdge) ImpactEdge {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Relation = strings.TrimSpace(in.Relation)
	in.FromPath = normalizeImpactObligationPath(in.FromPath)
	in.ToPath = normalizeImpactObligationPath(in.ToPath)
	in.Symbol = strings.TrimSpace(in.Symbol)
	in.ContractRef = strings.TrimSpace(in.ContractRef)
	in.ProbeID = strings.TrimSpace(in.ProbeID)
	in.Source = strings.TrimSpace(in.Source)
	in.Strength = strings.TrimSpace(in.Strength)
	in.EvidenceRef = strings.TrimSpace(in.EvidenceRef)
	in.ID = impactEdgeID(in)
	return in
}

func impactVerificationTarget(in ImpactVerificationTarget) ImpactVerificationTarget {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Path = normalizeImpactObligationPath(in.Path)
	in.RelatedPath = normalizeImpactObligationPath(in.RelatedPath)
	in.Symbol = strings.TrimSpace(in.Symbol)
	in.ContractRef = strings.TrimSpace(in.ContractRef)
	in.ProbeID = strings.TrimSpace(in.ProbeID)
	in.Strength = strings.TrimSpace(in.Strength)
	in.Source = strings.TrimSpace(in.Source)
	in.CoverageStatus = strings.TrimSpace(in.CoverageStatus)
	in.EvidenceRef = strings.TrimSpace(in.EvidenceRef)
	if in.Priority < 0 {
		in.Priority = 0
	}
	in.ID = impactVerificationTargetID(in)
	return in
}

func impactAnalysisResultID(planID, patchEffectID string) string {
	planID = strings.TrimSpace(planID)
	patchEffectID = strings.TrimSpace(patchEffectID)
	key := strings.Trim(strings.Join([]string{planID, patchEffectID}, ":"), ":")
	if key == "" {
		return ""
	}
	return "impact-analysis:" + key
}

func impactChangedSurfaceID(surface ImpactChangedSurface) string {
	key := strings.Join([]string{
		surface.Kind,
		surface.Path,
		surface.Symbol,
		fmt.Sprintf("%d", surface.LineStart),
		fmt.Sprintf("%d", surface.LineEnd),
		surface.EvidenceRef,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		return ""
	}
	return "impact-surface:" + key
}

func impactEdgeID(edge ImpactEdge) string {
	key := strings.Join([]string{
		edge.Kind,
		edge.Relation,
		edge.FromPath,
		edge.ToPath,
		edge.Symbol,
		edge.ContractRef,
		edge.ProbeID,
		edge.EvidenceRef,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		return ""
	}
	return "impact-edge:" + key
}

func impactVerificationTargetID(target ImpactVerificationTarget) string {
	key := strings.Join([]string{
		target.Kind,
		target.Path,
		target.RelatedPath,
		target.Symbol,
		target.ContractRef,
		target.ProbeID,
		target.EvidenceRef,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		return ""
	}
	return "impact-target:" + key
}

func firstNonEmptyImpactAnalysisString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
