// Package compiler turns a RequestModel into a fully populated
// TaskGraph + EvidencePlan + AnswerContract according to a pluggable
// scenario template. It is the deterministic half of the Analyzer v3
// planning stage: the LLM only has to produce a high-quality
// RequestModel (intent + scenario + term graph + risk matrix), and
// the compiler does the rest.
//
// Templates are built into the binary and indexed by Scenario so
// tests and production run against the same data. Adding a scenario
// means adding a template function — no config file, no reflection.
//
// The compiler is intentionally conservative: when a RequestModel
// lacks a field the template relies on, it emits a generic fallback
// instead of returning an error. The analyzer quality gate (b4)
// catches degenerate IRs separately.
package compiler

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Output bundles the three artifacts the compiler produces. Keeping
// them together makes it obvious at call sites that every compile
// run emits all three — no "optional" pieces.
type Output struct {
	TaskGraph      types.TaskGraph
	EvidencePlan   types.EvidencePlan
	AnswerContract types.AnswerContract
}

// Compile selects the template for rm.Scenario and runs it. If the
// scenario is empty or unknown, the generic template is used. It
// never returns nil — callers can always inspect the returned Output.
func Compile(rm types.RequestModel) Output {
	t := pickTemplate(rm.Scenario)
	return t(rm)
}

// templateFn is the contract every scenario implementation satisfies.
// It may read any field of RequestModel but must not mutate it.
type templateFn func(types.RequestModel) Output

func pickTemplate(s types.Scenario) templateFn {
	switch s {
	case types.ScenarioArchitectureExplain:
		return templateArchitectureExplain
	case types.ScenarioRootCause:
		return templateRootCause
	case types.ScenarioSecurityAudit:
		return templateSecurityAudit
	case types.ScenarioRefactorDesign:
		return templateRefactorDesign
	case types.ScenarioConfigTrace:
		return templateConfigTrace
	case types.ScenarioPerformanceBottleneck:
		return templatePerformanceBottleneck
	}
	return templateGeneric
}

// nodeID builds a deterministic node ID from an index and a short
// name. Every template uses this helper so tests can assert stable
// IDs without caring about template internals.
func nodeID(idx int, name string) string {
	return fmt.Sprintf("n%d_%s", idx, name)
}

// chain connects a list of node IDs with hard_dependency edges in
// order. It is the most common edge topology across templates.
func chain(ids ...string) []types.TaskEdge {
	if len(ids) < 2 {
		return nil
	}
	edges := make([]types.TaskEdge, 0, len(ids)-1)
	for i := 1; i < len(ids); i++ {
		edges = append(edges, types.TaskEdge{
			From:     ids[i-1],
			To:       ids[i],
			EdgeType: types.EdgeHardDependency,
		})
	}
	return edges
}

// hintsFromRM copies every symbol-kind term into the node's search
// hints. Concept terms are included too because explorer resolves
// them through normalizer.Resolve at runtime. Called from every
// template so nodes get useful hints without hand-wiring.
func hintsFromRM(rm types.RequestModel) types.SearchHints {
	var hints types.SearchHints
	for _, c := range rm.TermGraph.Canonical {
		switch c.Kind {
		case types.TermSymbol:
			hints.EntityIDs = append(hints.EntityIDs, c.ID)
			hints.KeywordIDs = append(hints.KeywordIDs, c.ID)
		case types.TermConcept, types.TermConfig:
			hints.KeywordIDs = append(hints.KeywordIDs, c.ID)
		}
	}
	return hints
}

// defaultBudget scales the evidence budget by complexity. The
// numbers are deliberately small-round-numbers — they're tuning
// seeds, not measurements, and will be recalibrated against the
// analyzer v3 eval grid during b8.
func defaultBudget(rm types.RequestModel, maxFiles, maxIters int) types.EvidenceBudget {
	mult := 1.0
	switch rm.Complexity {
	case types.ComplexitySimple:
		mult = 0.6
	case types.ComplexityComplex:
		mult = 1.6
	}
	// MaxToolCalls is MaxReactIters × 4: each ReAct iteration may
	// dispatch 1-3 tool calls and the ×2 ratio used in the b3 draft
	// was too tight on complex questions (review item P2-4).
	return types.EvidenceBudget{
		MaxFiles:      int(float64(maxFiles) * mult),
		MaxBytes:      200_000,
		MaxReactIters: int(float64(maxIters) * mult),
		MaxToolCalls:  int(float64(maxIters*4) * mult),
	}
}
