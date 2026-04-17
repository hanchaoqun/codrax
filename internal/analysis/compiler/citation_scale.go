package compiler

import (
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// citation_scale.go adapts the per-template citation thresholds to
// RequestModel.Complexity and len(SubTopics). Without it, every
// architecture_explain question — whether "what does repomap do?" or
// "compare the observability stack of Datadog vs NewRelic across
// 15 services" — demanded TmplCitationCountHigh (3) citations. The
// former is legitimately answerable with 2 tightly-grounded cites;
// forcing 3 drains the retry budget on prompts the LLM can't fill.
//
// Adaptation rule (intentionally conservative):
//
//   base        = template's hardcoded citation count (1 / 2 / 3)
//   simple      = base - 1   (one lookup / single file)
//   moderate    = base - 1   when base >= 3 (signature drop for
//                              "explain one 3-5 file component" —
//                              the common case that drove this fix)
//   moderate    = base       otherwise (don't cut low-base scenarios
//                              below their gate)
//   complex     = base       (keep the authors' ceiling; multi-file
//                              investigations genuinely need more
//                              citation breadth)
//
// Multi-topic boost: questions that covered N independent sub-topics
// must cite at least min(N, 3) times so each sub-topic has at least
// one visible anchor. This is additive on top of the complexity rule:
// a simple+3-subtopic question clamps to 3, not 0.
//
// Hard floor 1, hard ceiling 6.
//
// scaleCitationCount is the single computation — every threshold
// lever (TaskNode.SuccessCriteria, AnswerContract.AcceptanceTests,
// CitationReq.MinCitations) routes through it so they stay in sync.
func scaleCitationCount(base int, complexity types.Complexity, subTopics int) int {
	n := base
	switch complexity {
	case types.ComplexitySimple:
		n -= 1
	case types.ComplexityModerate:
		if base >= 3 {
			n -= 1
		}
	// types.ComplexityComplex or empty → no change.
	}
	if subTopics > 1 {
		boost := subTopics
		if boost > 3 {
			boost = 3
		}
		if n < boost {
			n = boost
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 6 {
		n = 6
	}
	return n
}

// applyAdaptiveCitationThresholds rewrites every CritCitationCountGE
// criterion in the TaskGraph + AnswerContract and the MinCitations
// field on CitationReq. Called from Compile right after the template
// function runs so every caller benefits without changing the
// templates themselves — they keep their "author intent" base values
// as named constants and this pass does the scaling.
//
// No-op when complexity is empty AND subTopics <= 1: the defaults
// match the pre-adaptation behaviour for legacy tests that build
// RequestModel literals without setting Complexity.
func applyAdaptiveCitationThresholds(out *Output, rm types.RequestModel) {
	if out == nil {
		return
	}
	if rm.Complexity == "" && len(rm.SubTopics) <= 1 {
		return
	}
	complexity := rm.Complexity
	subTopics := len(rm.SubTopics)

	// TaskNode SuccessCriteria on finalize / validate / reconcile.
	for i := range out.TaskGraph.Nodes {
		n := &out.TaskGraph.Nodes[i]
		for j := range n.SuccessCriteria {
			c := &n.SuccessCriteria[j]
			if c.Kind != types.CritCitationCountGE {
				continue
			}
			base, err := strconv.Atoi(c.Expr)
			if err != nil {
				continue
			}
			c.Expr = strconv.Itoa(scaleCitationCount(base, complexity, subTopics))
		}
	}

	// AnswerContract.AcceptanceTests.
	for i := range out.AnswerContract.AcceptanceTests {
		a := &out.AnswerContract.AcceptanceTests[i]
		if a.Kind != types.CritCitationCountGE {
			continue
		}
		base, err := strconv.Atoi(a.Expr)
		if err != nil {
			continue
		}
		a.Expr = strconv.Itoa(scaleCitationCount(base, complexity, subTopics))
	}

	// AnswerContract.CitationReq.MinCitations.
	if out.AnswerContract.CitationReq.MinCitations > 0 {
		out.AnswerContract.CitationReq.MinCitations = scaleCitationCount(
			out.AnswerContract.CitationReq.MinCitations, complexity, subTopics)
	}
}
