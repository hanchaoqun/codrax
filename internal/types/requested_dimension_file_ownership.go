package types

import (
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
