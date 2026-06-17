package impact

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type SymbolRef struct {
	ID      string
	Name    string
	File    string
	Line    int
	EndLine int
}

type GraphProvider interface {
	Imports(path string) []string
	ReverseImports(path string) []string
	RelatedTests(path string) []string
	SymbolsInFile(path string) []SymbolRef
}

type Input struct {
	Plan        *types.ChangePlan
	PatchEffect *types.PatchEffectRecord
	Graph       GraphProvider
}

func BuildObligationSet(in Input) types.ImpactObligationSet {
	set := types.ImpactObligationSetFromChangePlan(in.Plan)
	if in.Plan != nil && in.PatchEffect == nil {
		in.PatchEffect = in.Plan.PatchEffect
	}
	if in.Plan != nil {
		set.PlanID = strings.TrimSpace(in.Plan.ID)
	}
	set.Source = "impact_engine"
	set.Obligations = append(set.Obligations, patchEffectObligations(in.PatchEffect)...)
	set.Obligations = append(set.Obligations, graphObligations(in.PatchEffect, in.Graph)...)
	return types.NormalizeImpactObligationSet(set)
}

func patchEffectObligations(effect *types.PatchEffectRecord) []types.ImpactObligation {
	if effect == nil {
		return nil
	}
	var out []types.ImpactObligation
	for _, file := range effect.Files {
		path := normalizeImpactPath(file.Path)
		if path == "" {
			path = normalizeImpactPath(file.OldPath)
		}
		if path == "" {
			continue
		}
		out = append(out, types.ImpactObligation{
			Kind:        "changed_file",
			Relation:    "actual_diff",
			Obligation:  "verify_changed_file",
			SubjectPath: path,
			RelatedPath: normalizeImpactPath(file.OldPath),
			Source:      "patch_effect",
			Strength:    types.ImpactObligationStrengthPrecise,
			EvidenceRef: patchEffectEvidenceRef(effect, path),
		})
		for _, event := range file.Events {
			code := strings.TrimSpace(event.Code)
			if code == "" {
				continue
			}
			eventPath := normalizeImpactPath(event.Path)
			if eventPath == "" {
				eventPath = path
			}
			out = append(out, types.ImpactObligation{
				Kind:        "effect_followup",
				Relation:    code,
				Obligation:  "review_patch_effect_event",
				SubjectPath: eventPath,
				Source:      "patch_effect",
				Strength:    types.ImpactObligationStrengthPrecise,
				EvidenceRef: patchEffectEvidenceRef(effect, eventPath),
			})
		}
	}
	return out
}

func graphObligations(effect *types.PatchEffectRecord, graph GraphProvider) []types.ImpactObligation {
	if effect == nil || graph == nil {
		return nil
	}
	var out []types.ImpactObligation
	for _, file := range effect.Files {
		path := normalizeImpactPath(file.Path)
		if path == "" {
			path = normalizeImpactPath(file.OldPath)
		}
		if path == "" || file.PathRole != "" && types.SourcePathRoleIsAuxiliary(file.PathRole) {
			continue
		}
		for _, dep := range graph.Imports(path) {
			dep = normalizeImpactPath(dep)
			if dep == "" {
				continue
			}
			out = append(out, types.ImpactObligation{
				Kind:        "dependency",
				Relation:    "imports",
				Obligation:  "verify_import_dependency",
				SubjectPath: path,
				RelatedPath: dep,
				Source:      "impact_engine",
				Strength:    types.ImpactObligationStrengthInferred,
				EvidenceRef: path + "->" + dep,
			})
		}
		for _, dependent := range graph.ReverseImports(path) {
			dependent = normalizeImpactPath(dependent)
			if dependent == "" {
				continue
			}
			out = append(out, types.ImpactObligation{
				Kind:        "dependent",
				Relation:    "reverse_import",
				Obligation:  "verify_dependent",
				SubjectPath: path,
				RelatedPath: dependent,
				Source:      "impact_engine",
				Strength:    types.ImpactObligationStrengthInferred,
				EvidenceRef: dependent + "->" + path,
			})
		}
		for _, test := range graph.RelatedTests(path) {
			test = normalizeImpactPath(test)
			if test == "" {
				continue
			}
			out = append(out, types.ImpactObligation{
				Kind:        "test_surface",
				Relation:    "related_test",
				Obligation:  "run_related_test",
				SubjectPath: path,
				RelatedPath: test,
				Source:      "impact_engine",
				Strength:    types.ImpactObligationStrengthInferred,
				EvidenceRef: test,
			})
		}
		for _, symbol := range touchedSymbols(file, graph.SymbolsInFile(path)) {
			out = append(out, types.ImpactObligation{
				Kind:          "changed_symbol",
				Relation:      "patch_hunk",
				Obligation:    "verify_changed_symbol",
				SubjectPath:   path,
				SubjectSymbol: symbolImpactName(symbol),
				Source:        "impact_engine",
				Strength:      types.ImpactObligationStrengthPrecise,
				EvidenceRef:   symbolImpactEvidenceRef(symbol),
			})
		}
	}
	return out
}

func touchedSymbols(file types.PatchEffectFile, symbols []SymbolRef) []SymbolRef {
	if len(file.Hunks) == 0 || len(symbols) == 0 {
		return nil
	}
	var out []SymbolRef
	for _, sym := range symbols {
		if sym.Line <= 0 {
			continue
		}
		end := sym.EndLine
		if end <= 0 || end < sym.Line {
			end = sym.Line
		}
		for _, hunk := range file.Hunks {
			if hunk.NewStart <= 0 {
				continue
			}
			hunkEnd := hunk.NewStart + hunk.NewLines - 1
			if hunk.NewLines == 0 {
				hunkEnd = hunk.NewStart
			}
			if rangesOverlap(sym.Line, end, hunk.NewStart, hunkEnd) {
				out = append(out, sym)
				break
			}
		}
	}
	return out
}

func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart <= bEnd && bStart <= aEnd
}

func symbolImpactName(sym SymbolRef) string {
	if strings.TrimSpace(sym.ID) != "" {
		return strings.TrimSpace(sym.ID)
	}
	return strings.TrimSpace(sym.Name)
}

func symbolImpactEvidenceRef(sym SymbolRef) string {
	if sym.File == "" {
		return symbolImpactName(sym)
	}
	if sym.Line > 0 {
		return fmt.Sprintf("%s:%d", normalizeImpactPath(sym.File), sym.Line)
	}
	return normalizeImpactPath(sym.File)
}

func patchEffectEvidenceRef(effect *types.PatchEffectRecord, path string) string {
	if effect == nil || strings.TrimSpace(effect.RecordID) == "" {
		return path
	}
	if path == "" {
		return strings.TrimSpace(effect.RecordID)
	}
	return strings.TrimSpace(effect.RecordID) + ":" + path
}

func normalizeImpactPath(raw string) string {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
