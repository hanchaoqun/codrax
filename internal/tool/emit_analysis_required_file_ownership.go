package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_analysis_required_file_ownership.go — the required_files ↔
// requested_answer_dimensions ownership lane (V4-4, colleague_merge_audit
// §40.22).
//
// Ruling ③: an analyze-stage hard gate may judge only the INTERNAL
// CONSISTENCY of what the model declared; it may never judge the
// COMPLETENESS of what the model did not declare. Analysis does not read file
// bodies, so "which file implements dimension 2" is frequently legitimate
// model uncertainty. The former responsibility gate rejected the whole
// emission for an unclassified high-confidence file or an owner-less
// dimension and taught the only escape as "lower confidence below 0.8" —
// teaching the model to misreport its confidence. That gate is retired:
//
//   - HARD (validateRequiredFileDimensionContradictions): exactly the two
//     precise contradictions inside the declared content — one file declared
//     both operation-owner and navigation-only; an index outside the model's
//     own declared dimension set.
//   - SOFT (types.CompileDimensionOwnerUnresolved): an unresolved owner or an
//     undeclared file role becomes AnalyzerHints.DimensionOwnerUnresolved, a
//     typed marker disclosed to the exploration stage, whose completion gate
//     already demands a grounded operation row per dimension index.
//   - SOFT (normalizeRequiredFileDimensionOwnership): declared-but-ineligible
//     indices (low confidence, non-ownership role) keep the existing drop
//     with a warning.

// normalizeRequiredFileDimensionOwnership keeps the optional file-scope
// contract precise. Only high-confidence required files may own the required
// explanation-operation dimensions compiled from the analyzer's typed
// dimension profile. Invalid or inapplicable indices are dropped fail-soft so
// this optional carrier never causes an analyzer retry storm. No rationale or
// request/answer prose participates.
func normalizeRequiredFileDimensionOwnership(in []types.RequiredFileHint, profile *types.RequestedAnswerDimensionProfile, val *analysisValidationResult) []types.RequiredFileHint {
	if len(in) == 0 {
		return in
	}
	allowed := map[int]bool{}
	for _, dimension := range types.RequestedExplanationOperationOwnershipDimensions(profile) {
		allowed[dimension.Index] = true
	}
	dropped := 0
	for i := range in {
		if len(in[i].RequestedDimensionIndices) == 0 {
			continue
		}
		if in[i].Confidence < 0.8 {
			dropped += len(in[i].RequestedDimensionIndices)
			in[i].RequestedDimensionIndices = nil
			continue
		}
		filtered := make([]int, 0, len(in[i].RequestedDimensionIndices))
		for _, index := range in[i].RequestedDimensionIndices {
			if allowed[index] {
				filtered = append(filtered, index)
			} else {
				dropped++
			}
		}
		in[i].RequestedDimensionIndices = filtered
	}
	if dropped > 0 && val != nil {
		val.Warnings = append(val.Warnings, fmt.Sprintf(
			"required_files: dropped %d requested_dimension_indices that were low-confidence or not required explanation-operation dimensions",
			dropped,
		))
	}
	return in
}

// declaredRequestedDimensionIndices returns the model's DECLARED dimension
// index set from the raw requested_answer_dimensions carrier, applying the
// same positional default (i+1 when index ≤ 0) as the profile normalizer.
// The hard index arm must judge this set, not the normalized profile: the
// normalizer drops a dimension whose quote is unanchored, and a system-side
// drop must never turn into a "model contradiction". nil when the model
// declared no dimension set (is_dimensioned_answer=false or absent).
func declaredRequestedDimensionIndices(p *emitRequestedAnswerDimensionsParam) map[int]bool {
	if p == nil || p.IsDimensionedAnswer == nil || !*p.IsDimensionedAnswer || len(p.Dimensions) == 0 {
		return nil
	}
	declared := make(map[int]bool, len(p.Dimensions))
	for i, dimension := range p.Dimensions {
		index := dimension.Index
		if index <= 0 {
			index = i + 1
		}
		declared[index] = true
	}
	return declared
}

// validateRequiredFileDimensionContradictions is the HARD ownership arm. It
// reads only the raw typed required_files entries (path, confidence,
// requested_dimension_indices, requested_dimension_navigation_only) plus the
// declared dimension index set; rationale, request prose, answer prose, and
// path keywords are never read.
//
// Arm 1 (owner ∧ navigation-only): one resolved path that, at confidence
// ≥0.8 (the only band where either field has effect), carries a non-empty
// requested_dimension_indices AND requested_dimension_navigation_only=true.
// Active only when the profile compiles ownership dimensions, because below
// that shape both fields are inert and a reject would be a needless retry.
//
// Arm 2 (index ∉ declared set): a requested_dimension_indices value at
// confidence ≥0.8 that is not one of the model's own declared dimension
// indices. Active only when the model declared a dimension set — with no set
// there is nothing to contradict and the existing soft drop applies.
//
// Both arms report every offender in one correction; neither arm judges
// whether an owner is missing or a file is unclassified (§40.22 ③).
func validateRequiredFileDimensionContradictions(
	ctx *types.BusContext,
	raw []emitRequiredFileParam,
	profile *types.RequestedAnswerDimensionProfile,
	declaredIndices map[int]bool,
) string {
	if len(raw) == 0 {
		return ""
	}
	ownershipActive := len(types.RequestedExplanationOperationOwnershipDimensions(profile)) > 0
	type declaration struct {
		owner, navigationOnly bool
	}
	byPath := make(map[string]*declaration)
	var pathOrder []string
	undeclared := make(map[int]bool)
	for _, entry := range raw {
		if entry.Confidence != entry.Confidence || entry.Confidence < 0.8 {
			continue
		}
		path, _, ok := resolveRequiredFileHintPath(ctx, entry.Path)
		if !ok || path == "" {
			continue
		}
		path = types.CanonicalRequestedDimensionSource(path)
		decl := byPath[path]
		if decl == nil {
			decl = &declaration{}
			byPath[path] = decl
			pathOrder = append(pathOrder, path)
		}
		for _, index := range entry.RequestedDimensionIndices {
			if index <= 0 {
				continue
			}
			decl.owner = true
			if declaredIndices != nil && !declaredIndices[index] {
				undeclared[index] = true
			}
		}
		if entry.RequestedDimensionNavigationOnly != nil && *entry.RequestedDimensionNavigationOnly {
			decl.navigationOnly = true
		}
	}
	var parts []string
	if ownershipActive {
		var conflicting []string
		for _, path := range pathOrder {
			if decl := byPath[path]; decl.owner && decl.navigationOnly {
				conflicting = append(conflicting, path)
			}
		}
		if len(conflicting) > 0 {
			sort.Strings(conflicting)
			parts = append(parts, "files declared both operation-owner and navigation-only=["+strings.Join(conflicting, ", ")+"]; a file either owns the listed dimensions or is a navigation aid, never both")
		}
	}
	if len(undeclared) > 0 {
		indices := make([]int, 0, len(undeclared))
		for index := range undeclared {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		declared := make([]int, 0, len(declaredIndices))
		for index := range declaredIndices {
			declared = append(declared, index)
		}
		sort.Ints(declared)
		parts = append(parts, fmt.Sprintf("requested_dimension_indices reference index %v outside the declared requested_answer_dimensions index set %v; use only indices you declared", indices, declared))
	}
	if len(parts) == 0 {
		return ""
	}
	return "required_files contradicts its own declarations: " + strings.Join(parts, "; ") +
		". Ownership you cannot settle is not a contradiction: leave that file's requested_dimension_indices empty and do not mark it navigation-only."
}

// requiredFileDimensionOwnerUnresolvedWarning renders the soft marker as one
// summary warning so the model sees the disclosure without a retry.
func requiredFileDimensionOwnerUnresolvedWarning(unresolved *types.DimensionOwnerUnresolved) string {
	if unresolved == nil {
		return ""
	}
	var parts []string
	if len(unresolved.DimensionIndices) > 0 {
		parts = append(parts, fmt.Sprintf("explanation-operation dimension(s) without a declared high-confidence file owner=%v", unresolved.DimensionIndices))
	}
	if len(unresolved.UnclassifiedFiles) > 0 {
		parts = append(parts, "high-confidence file(s) without a declared role=["+strings.Join(unresolved.UnclassifiedFiles, ", ")+"]")
	}
	return "required_files ownership left unresolved for exploration: " + strings.Join(parts, "; ") +
		"; the implementing operation is read and bound to its dimension index during exploration"
}
