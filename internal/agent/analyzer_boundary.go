package agent

import (
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

	// AnswerShape "restoration" retired with the shape enum. The
	// pre-shape variant of this block forced shape=step_list /
	// list_of_symbols when the LLM had picked something else for a
	// bounded-owner question; the V2 carrier derives the same effect
	// from QuestionFamily + AnswerSubject + bounded-owner narrowing
	// upstream, without needing a shape side-channel.
	if !changed {
		return rm, ""
	}
	return rm, strings.Join(reasons, "; ")
}

// reconcileScalarRoleLocateScope keeps a scalar role-locate request as
// one principal question even when the analyzer LLM emitted exploratory
// sub-topics. Cross-file or cross-repo search breadth is still available
// through entities and search hints; sub_topics are only removed so they
// do not become independent answer sections.
func reconcileScalarRoleLocateScope(rm types.RequestModel) (types.RequestModel, string) {
	if !rm.Predicates.IsRoleLocateLookup ||
		!rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		signalsExplicitMultiAxis(rm.Predicates) ||
		len(rm.SubTopics) == 0 {
		return rm, ""
	}
	rm.SubTopics = nil
	return rm, "dropped exploratory sub_topics for scalar role-locate lookup; breadth remains search context, not principal answer topics"
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
