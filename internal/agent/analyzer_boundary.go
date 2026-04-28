package agent

import (
	"fmt"
	"slices"
	"strings"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// reconcileEnumerationBoundaryScope narrows analyzer-emitted breadth
// when the user explicitly declared a bounded principal set on a
// single exact owner (for example "the 7 checks" on one function).
//
// The analyzer LLM is still free to discover extra adjacent facts, but
// those facts must not be promoted into top-level entities/sub-topics,
// otherwise every downstream stage treats them as independent topics
// and drowns the user-declared principal set.
func reconcileEnumerationBoundaryScope(rm types.RequestModel, graph *repomap.Graph) (types.RequestModel, string) {
	if rm.EnumerationBoundary == nil || rm.EnumerationBoundary.DeclaredCount <= 0 {
		return rm, ""
	}
	if rm.Predicates.IsCountQuestion || rm.Predicates.IsCrossComponent || rm.Predicates.IsRelationalLookup {
		return rm, ""
	}
	kind := strings.ToLower(strings.TrimSpace(rm.AnalyzerHints.Kind))
	if kind != "mechanism" && kind != "enumeration" && kind != "call_chain" && kind != "conditional" {
		return rm, ""
	}
	owner := types.RequestedEnumerationBoundaryOwner(rm)
	if owner == "" {
		return rm, ""
	}
	focus := []string{owner}
	if graph != nil {
		if len(exactEntityAnchors(graph, focus)) > 1 {
			return rm, ""
		}
	}

	changed := false
	var reasons []string

	if len(rm.SubTopics) > 0 {
		rm.SubTopics = nil
		changed = true
		reasons = append(reasons, "dropped LLM sub_topics so a single-owner bounded set stays one topic")
	}
	if !equalFoldSlice(rm.AnalyzerHints.Entities, focus) {
		rm.AnalyzerHints.Entities = append([]string(nil), focus...)
		changed = true
		reasons = append(reasons, "narrowed entities to the single request-mentioned owner")
	}
	if !equalFoldSlice(rm.AnalyzerHints.PrimaryEntities, focus) {
		rm.AnalyzerHints.PrimaryEntities = append([]string(nil), focus...)
		changed = true
		reasons = append(reasons, "narrowed primary_entities to the same owner")
	}

	wantShape := strings.TrimSpace(rm.AnalyzerHints.Shape)
	switch kind {
	case "mechanism", "call_chain", "conditional":
		wantShape = string(types.ShapeStepList)
	case "enumeration":
		wantShape = string(types.ShapeListOfSymbols)
	}
	if wantShape != "" && !strings.EqualFold(strings.TrimSpace(rm.AnalyzerHints.Shape), wantShape) {
		rm.AnalyzerHints.Shape = wantShape
		changed = true
		reasons = append(reasons, fmt.Sprintf("restored boundary-compatible shape %s", wantShape))
	}
	if !changed {
		return rm, ""
	}
	return rm, strings.Join(reasons, "; ")
}

func equalFoldSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	for i := range ac {
		ac[i] = strings.ToLower(strings.TrimSpace(ac[i]))
	}
	for i := range bc {
		bc[i] = strings.ToLower(strings.TrimSpace(bc[i]))
	}
	slices.Sort(ac)
	slices.Sort(bc)
	return slices.Equal(ac, bc)
}
