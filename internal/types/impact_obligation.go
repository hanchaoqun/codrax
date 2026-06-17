package types

import (
	"fmt"
	"sort"
	"strings"
)

type ImpactObligationStrength string

const (
	ImpactObligationStrengthPrecise  ImpactObligationStrength = "precise"
	ImpactObligationStrengthDeclared ImpactObligationStrength = "declared"
	ImpactObligationStrengthInferred ImpactObligationStrength = "inferred"
)

type ImpactObligation struct {
	ID                string                   `json:"id,omitempty"`
	Kind              string                   `json:"kind"`
	Relation          string                   `json:"relation,omitempty"`
	Obligation        string                   `json:"obligation"`
	SubjectPath       string                   `json:"subject_path,omitempty"`
	RelatedPath       string                   `json:"related_path,omitempty"`
	SubjectSymbol     string                   `json:"subject_symbol,omitempty"`
	ContractRef       string                   `json:"contract_ref,omitempty"`
	VerificationProbe string                   `json:"verification_probe,omitempty"`
	SliceID           string                   `json:"slice_id,omitempty"`
	Source            string                   `json:"source,omitempty"`
	Strength          ImpactObligationStrength `json:"strength,omitempty"`
	EvidenceRef       string                   `json:"evidence_ref,omitempty"`
}

type ImpactObligationSet struct {
	SetID       string             `json:"set_id,omitempty"`
	PlanID      string             `json:"plan_id,omitempty"`
	Source      string             `json:"source,omitempty"`
	Obligations []ImpactObligation `json:"obligations,omitempty"`
}

func ImpactObligationSetFromChangePlan(plan *ChangePlan) ImpactObligationSet {
	set := ImpactObligationSet{Source: "change_plan"}
	if plan == nil {
		return set
	}
	set.PlanID = strings.TrimSpace(plan.ID)
	set.SetID = "impact-obligations:" + set.PlanID
	for _, change := range plan.Changes {
		path := normalizeImpactObligationPath(change.Path)
		if path != "" {
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:        "changed_file",
				Relation:    "change",
				Obligation:  "verify_changed_file",
				SubjectPath: path,
				Source:      "change_plan",
				Strength:    ImpactObligationStrengthPrecise,
				EvidenceRef: path,
			}))
		}
		newPath := normalizeImpactObligationPath(change.NewPath)
		if newPath != "" && newPath != path {
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:        "changed_file",
				Relation:    "rename_target",
				Obligation:  "verify_changed_file",
				SubjectPath: newPath,
				RelatedPath: path,
				Source:      "change_plan",
				Strength:    ImpactObligationStrengthPrecise,
				EvidenceRef: newPath,
			}))
		}
		for _, dep := range change.DependsOn {
			dep = normalizeImpactObligationPath(dep)
			if path == "" || dep == "" {
				continue
			}
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:        "dependency",
				Relation:    "depends_on",
				Obligation:  "verify_dependency_order",
				SubjectPath: path,
				RelatedPath: dep,
				Source:      "change_plan",
				Strength:    ImpactObligationStrengthDeclared,
				EvidenceRef: path + "->" + dep,
			}))
		}
	}
	for _, probe := range plan.VerificationProbes {
		probeID := strings.TrimSpace(probe.ID)
		for _, ref := range probe.ContractRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:              "behavior_contract",
				Relation:          "contract_ref",
				Obligation:        "verify_contract_ref",
				ContractRef:       ref,
				VerificationProbe: probeID,
				Source:            "verification_probe",
				Strength:          ImpactObligationStrengthDeclared,
				EvidenceRef:       ref,
			}))
		}
		for _, ref := range probe.ChangedSymbolRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:              "changed_symbol",
				Relation:          "symbol_ref",
				Obligation:        "verify_changed_symbol",
				SubjectSymbol:     ref,
				VerificationProbe: probeID,
				Source:            "verification_probe",
				Strength:          ImpactObligationStrengthDeclared,
				EvidenceRef:       ref,
			}))
		}
	}
	for _, slice := range plan.Slices {
		for _, ref := range slice.ContractRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:        "behavior_contract",
				Relation:    "slice_contract_ref",
				Obligation:  "verify_contract_ref",
				ContractRef: ref,
				SliceID:     strings.TrimSpace(slice.ID),
				Source:      "change_plan_slice",
				Strength:    ImpactObligationStrengthDeclared,
				EvidenceRef: ref,
			}))
		}
		for _, ref := range slice.ChangedSymbolRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			set.Obligations = append(set.Obligations, impactObligation(ImpactObligation{
				Kind:          "changed_symbol",
				Relation:      "slice_symbol_ref",
				Obligation:    "verify_changed_symbol",
				SubjectSymbol: ref,
				SliceID:       strings.TrimSpace(slice.ID),
				Source:        "change_plan_slice",
				Strength:      ImpactObligationStrengthDeclared,
				EvidenceRef:   ref,
			}))
		}
	}
	set.Obligations = normalizeImpactObligations(set.Obligations)
	return set
}

func NormalizeImpactObligationSet(in ImpactObligationSet) ImpactObligationSet {
	in.SetID = strings.TrimSpace(in.SetID)
	in.PlanID = strings.TrimSpace(in.PlanID)
	in.Source = strings.TrimSpace(in.Source)
	in.Obligations = normalizeImpactObligations(in.Obligations)
	if in.SetID == "" && in.PlanID != "" {
		in.SetID = "impact-obligations:" + in.PlanID
	}
	return in
}

func impactObligation(in ImpactObligation) ImpactObligation {
	in.Kind = strings.TrimSpace(in.Kind)
	in.Relation = strings.TrimSpace(in.Relation)
	in.Obligation = strings.TrimSpace(in.Obligation)
	in.SubjectPath = normalizeImpactObligationPath(in.SubjectPath)
	in.RelatedPath = normalizeImpactObligationPath(in.RelatedPath)
	in.SubjectSymbol = strings.TrimSpace(in.SubjectSymbol)
	in.ContractRef = strings.TrimSpace(in.ContractRef)
	in.VerificationProbe = strings.TrimSpace(in.VerificationProbe)
	in.SliceID = strings.TrimSpace(in.SliceID)
	in.Source = strings.TrimSpace(in.Source)
	in.EvidenceRef = strings.TrimSpace(in.EvidenceRef)
	if in.Strength == "" {
		in.Strength = ImpactObligationStrengthInferred
	}
	in.ID = impactObligationID(in)
	return in
}

func normalizeImpactObligations(in []ImpactObligation) []ImpactObligation {
	out := make([]ImpactObligation, 0, len(in))
	seen := map[string]bool{}
	for _, ob := range in {
		ob = impactObligation(ob)
		if ob.Kind == "" || ob.Obligation == "" {
			continue
		}
		if seen[ob.ID] {
			continue
		}
		seen[ob.ID] = true
		out = append(out, ob)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func impactObligationID(ob ImpactObligation) string {
	key := strings.Join([]string{
		ob.Kind,
		ob.Relation,
		ob.Obligation,
		ob.SubjectPath,
		ob.RelatedPath,
		ob.SubjectSymbol,
		ob.ContractRef,
		ob.VerificationProbe,
		ob.SliceID,
	}, "|")
	key = strings.Trim(key, "|")
	if key == "" {
		return ""
	}
	return fmt.Sprintf("impact:%s", key)
}

func normalizeImpactObligationPath(raw string) string {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
