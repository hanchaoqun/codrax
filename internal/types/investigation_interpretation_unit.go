package types

import "strings"

const MaxInvestigationInterpretationUnits = 16

// A model-authored explanation and its declared limits travel together. Refs
// locate proposed support only; they grant no measurement or causal authority.
type InvestigationInterpretationUnit struct {
	Text        string                            `json:"text"`
	Boundaries  []string                          `json:"boundaries,omitempty"`
	SupportRefs []string                          `json:"support_refs,omitempty"`
	NativeRefs  []InvestigationNativeRowReference `json:"native_refs,omitempty"`
}

const InvestigationInterpretationFormTeaching = "Provide non-empty interpretation_units and omit interpretation, or provide legacy interpretation as a string; an empty string explicitly means no independent interpretation. Do not omit both."

const InvestigationInterpretationUnitsTeaching = "Prefer concise interpretation_units for independent findings: keep each text with its boundaries and support_refs in one object. Each object is displayed whole or omitted whole under the existing shared narrative budget; a support_ref locates proposed support, not proof. " + InvestigationInterpretationFormTeaching + " When both are supplied, units replace the legacy narrative, which remains audit-only. Missing units retain the legacy interpretation path without an extra retry. Both forms stay model-authored advisory and cannot replace native facts or grant causal authority."

func (u InvestigationInterpretationUnit) Valid() bool {
	if strings.TrimSpace(u.Text) == "" || len(u.Boundaries) > 8 || len(u.SupportRefs) > 16 || len(u.NativeRefs) > MaxInvestigationNativeRowReferences {
		return false
	}
	valid := true
	for _, ref := range u.NativeRefs {
		valid = valid && ref.valid()
	}
	for _, fields := range [][]string{u.Boundaries, u.SupportRefs} {
		for _, field := range fields {
			valid = valid && strings.TrimSpace(field) != ""
		}
	}
	return valid
}

func cloneInvestigationInterpretationUnits(in []InvestigationInterpretationUnit) []InvestigationInterpretationUnit {
	if in == nil {
		return nil
	}
	out := make([]InvestigationInterpretationUnit, len(in))
	for i, unit := range in {
		out[i] = unit
		out[i].Boundaries = append([]string(nil), unit.Boundaries...)
		out[i].SupportRefs = append([]string(nil), unit.SupportRefs...)
		out[i].NativeRefs = append([]InvestigationNativeRowReference(nil), unit.NativeRefs...)
		for j, ref := range unit.NativeRefs {
			out[i].NativeRefs[j].RowIndices = append([]int(nil), ref.RowIndices...)
		}
	}
	return out
}
