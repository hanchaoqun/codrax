package types

import (
	"sort"
	"strconv"
	"strings"
)

// RequestedExplanationOperationNeed is one exact operation-evidence seat for
// a required explanation dimension. Source is empty for the legacy unscoped
// contract; a non-empty source comes only from an explicit high-confidence
// RequiredFileHint.RequestedDimensionIndices binding.
type RequestedExplanationOperationNeed struct {
	Dimension RequestedAnswerDimension
	Source    string
}

// RequestedExplanationOperationNeeds compiles the typed ownership contract.
// A dimension with one or more explicit file bindings requires an operation
// row from every bound file. Dimensions without bindings retain the legacy
// any-source operation seat. Rationale, request prose, answer prose, and path
// keywords are deliberately not inspected.
func RequestedExplanationOperationNeeds(profile *RequestedAnswerDimensionProfile, hints []RequiredFileHint) []RequestedExplanationOperationNeed {
	dimensions := RequestedExplanationOperationOwnershipDimensions(profile)
	if len(dimensions) == 0 {
		return nil
	}
	dimensionByIndex := make(map[int]RequestedAnswerDimension, len(dimensions))
	for _, dimension := range dimensions {
		dimensionByIndex[dimension.Index] = dimension
	}
	sourcesByIndex := make(map[int][]string, len(dimensions))
	seen := make(map[string]bool)
	for _, hint := range hints {
		if hint.Confidence < 0.8 {
			continue
		}
		source := CanonicalRequestedDimensionSource(hint.Path)
		if source == "" {
			continue
		}
		for _, index := range hint.RequestedDimensionIndices {
			if _, ok := dimensionByIndex[index]; !ok {
				continue
			}
			key := requestedExplanationOperationNeedKey(index, source)
			if seen[key] {
				continue
			}
			seen[key] = true
			sourcesByIndex[index] = append(sourcesByIndex[index], source)
		}
	}
	needs := make([]RequestedExplanationOperationNeed, 0, len(dimensions))
	for _, dimension := range dimensions {
		sources := sourcesByIndex[dimension.Index]
		if len(sources) == 0 {
			needs = append(needs, RequestedExplanationOperationNeed{Dimension: dimension})
			continue
		}
		for _, source := range sources {
			needs = append(needs, RequestedExplanationOperationNeed{Dimension: dimension, Source: source})
		}
	}
	return needs
}

// CanonicalRequestedDimensionSource keeps file ownership matching exact and
// platform-neutral without resolving or guessing paths.
func CanonicalRequestedDimensionSource(source string) string {
	source = strings.TrimSpace(strings.ReplaceAll(source, "\\", "/"))
	for strings.HasPrefix(source, "./") {
		source = strings.TrimPrefix(source, "./")
	}
	return source
}

func RequestedExplanationOperationNeedCovered(need RequestedExplanationOperationNeed, item EvidenceItem) bool {
	if item.GroundingStatus != GroundingGrounded && item.GroundingStatus != GroundingRecovered {
		return false
	}
	if !EvidenceCarriesExplanationOperation(item) || !evidenceOwnsRequestedDimension(item, need.Dimension.Index) {
		return false
	}
	return need.Source == "" || CanonicalRequestedDimensionSource(item.Source) == need.Source
}

func RequestedExplanationOperationNeedKey(need RequestedExplanationOperationNeed) string {
	return requestedExplanationOperationNeedKey(need.Dimension.Index, need.Source)
}

func requestedExplanationOperationNeedKey(index int, source string) string {
	return strings.Join([]string{strconv.Itoa(index), CanonicalRequestedDimensionSource(source)}, "\x00")
}

func evidenceOwnsRequestedDimension(item EvidenceItem, index int) bool {
	for _, candidate := range item.RequestedDimensionIndices {
		if candidate == index {
			return true
		}
	}
	return false
}

// DimensionOwnerUnresolved is the typed SOFT marker (V4-4, colleague_merge_audit
// §40.22) recording that the analyzer asserted high-confidence file
// responsibility but did not settle every required explanation-operation
// dimension. DimensionIndices lists the ownership dimensions with no
// high-confidence owner; UnclassifiedFiles lists the high-confidence files
// that declared neither an owned index nor navigation-only. The marker is
// disclosure for the exploration stage (which settles ownership by reading
// the implementing operation) and never a reject reason: analysis does not
// read file bodies, so this uncertainty is legitimate model uncertainty.
type DimensionOwnerUnresolved struct {
	DimensionIndices  []int    `json:"dimension_indices,omitempty"`
	UnclassifiedFiles []string `json:"unclassified_files,omitempty"`
}

// CompileDimensionOwnerUnresolved reads only schema-validated typed state:
// dimension Required/Role/Index, hint Origin, Confidence,
// RequestedDimensionIndices, and the persisted
// RequestedDimensionNavigationOnly flag. It returns nil when there are no
// ownership dimensions (fewer than two required
// function_or_purpose/branch_behavior dimensions), when no model-declared
// high-confidence (≥0.8) hint exists (the legacy any-source contract), or
// when every dimension has an owner and every model-declared high-confidence
// file declared a role. Hints with a system Origin (runtime-artifact path,
// prescan candidate, scope promotion, user pin) are not the model's
// declarations and never enter the marker (§40.47 fold-in A0). Rationale,
// path names, request prose, and answer prose are not inspected.
func CompileDimensionOwnerUnresolved(profile *RequestedAnswerDimensionProfile, hints []RequiredFileHint) *DimensionOwnerUnresolved {
	dimensions := RequestedExplanationOperationOwnershipDimensions(profile)
	if len(dimensions) == 0 {
		return nil
	}
	required := make(map[int]bool, len(dimensions))
	for _, dimension := range dimensions {
		required[dimension.Index] = true
	}
	owned := make(map[int]bool, len(dimensions))
	seenFile := make(map[string]bool, len(hints))
	var unclassified []string
	hasHighConfidence := false
	for _, hint := range hints {
		if !hint.ModelDeclared() || hint.Confidence < 0.8 {
			continue
		}
		path := CanonicalRequestedDimensionSource(hint.Path)
		if path == "" {
			continue
		}
		hasHighConfidence = true
		ownsOperation := false
		for _, index := range hint.RequestedDimensionIndices {
			if required[index] {
				owned[index] = true
				ownsOperation = true
			}
		}
		if !ownsOperation && !hint.RequestedDimensionNavigationOnly && !seenFile[path] {
			seenFile[path] = true
			unclassified = append(unclassified, path)
		}
	}
	if !hasHighConfidence {
		return nil
	}
	var missing []int
	for _, dimension := range dimensions {
		if !owned[dimension.Index] {
			missing = append(missing, dimension.Index)
		}
	}
	if len(missing) == 0 && len(unclassified) == 0 {
		return nil
	}
	sort.Strings(unclassified)
	return &DimensionOwnerUnresolved{DimensionIndices: missing, UnclassifiedFiles: unclassified}
}

// Clone returns a nil-safe deep copy so degraded-recovery IR rebuilds carry
// the marker without aliasing the partial IR's slices.
func (m *DimensionOwnerUnresolved) Clone() *DimensionOwnerUnresolved {
	if m == nil {
		return nil
	}
	return &DimensionOwnerUnresolved{
		DimensionIndices:  append([]int(nil), m.DimensionIndices...),
		UnclassifiedFiles: append([]string(nil), m.UnclassifiedFiles...),
	}
}
