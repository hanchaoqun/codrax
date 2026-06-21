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
package compiler

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/sourcemix"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Output bundles the three artifacts the compiler produces.
type Output struct {
	TaskGraph      types.TaskGraph
	EvidencePlan   types.EvidencePlan
	AnswerContract types.AnswerContract
}

// Compile selects the template for rm.Scenario, runs it, then
// computes the real EvidenceBudget via the budget package and
// derives NodeBudgetHints from the template's SourceMix.
//
// The two-phase shape (template → budget compute) exists because
// the budget depends on len(hypothesis set), which is produced by
// hdp AFTER compile finishes. Callers (analyzer.buildAnalysisIR)
// re-run CompileFinalize with the final hypothesis count after hdp.
func Compile(rm types.RequestModel, sig budget.BudgetSignals) Output {
	t := pickTemplate(rm)
	out := t(rm)
	EnsureReadStageNodes(&out.TaskGraph)
	// Adapt citation thresholds to complexity + subtopic count so a
	// "simple single-lookup" question is not held to the same bar as
	// a "cross-component architecture survey". See
	// citation_scale.go:applyAdaptiveCitationThresholds for the rule.
	applyAdaptiveCitationThresholds(&out, rm)
	out.EvidencePlan.Budget = budget.Compute(rm, sig)
	out.EvidencePlan.NodeBudgetHints = sourcemix.FromTemplateMix(out.EvidencePlan.SourceMix, out.EvidencePlan.Budget)
	return out
}

// RecomputeBudget is called by the analyzer after hdp.Plan has run
// so the budget reflects the final hypothesis count. It overwrites
// Budget and NodeBudgetHints in place on the passed Output.
func RecomputeBudget(out *Output, rm types.RequestModel, sig budget.BudgetSignals) {
	if out == nil {
		return
	}
	out.EvidencePlan.Budget = budget.Compute(rm, sig)
	out.EvidencePlan.NodeBudgetHints = sourcemix.FromTemplateMix(out.EvidencePlan.SourceMix, out.EvidencePlan.Budget)
}

// templateFn is the contract every scenario implementation
// satisfies. It reads rm and returns a raw Output whose Budget /
// NodeBudgetHints are zero — Compile fills those in centrally.
type templateFn func(types.RequestModel) Output

func pickTemplate(rm types.RequestModel) templateFn {
	// Defense-in-depth: even if an upstream caller kept
	// architecture_explain/generic, single-topic structural trace
	// requests should still compile to the lighter trace walkthrough
	// DAG rather than the reconcile-heavy architecture template.
	if types.IsSingleTopicStructuralTrace(rm) &&
		(rm.Scenario == types.ScenarioGeneric || rm.Scenario == types.ScenarioArchitectureExplain) {
		return templateTraceWalkthrough
	}
	switch rm.Scenario {
	case types.ScenarioArchitectureExplain:
		return templateArchitectureExplain
	case types.ScenarioRootCause:
		return templateRootCause
	case types.ScenarioConfigTrace:
		return templateConfigTrace
	case types.ScenarioPerformanceBottleneck:
		return templatePerformanceBottleneck
	}
	return templateGeneric
}

// nodeID builds a deterministic node ID from an index and a short
// name.
func nodeID(idx int, name string) string {
	return fmt.Sprintf("n%d_%s", idx, name)
}

// chain connects a list of node IDs with hard_dependency edges in
// order.
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
// hints, then merges analyzer/profile subject lanes as soft guidance.
// AnalyzerHints are deliberately hints only: they improve recall for
// language forms the raw-text normalizer does not fully tokenize
// (C++ `::`, Cangjie / ArkTS qualified members, prior-resolved
// subjects), but never become hard gates by themselves.
func hintsFromRM(rm types.RequestModel) types.SearchHints {
	var hints types.SearchHints
	seenKeyword := map[string]bool{}
	seenEntity := map[string]bool{}
	addKeyword := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seenKeyword[strings.ToLower(id)] {
			return
		}
		seenKeyword[strings.ToLower(id)] = true
		hints.KeywordIDs = append(hints.KeywordIDs, id)
	}
	addEntity := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seenEntity[strings.ToLower(id)] {
			return
		}
		seenEntity[strings.ToLower(id)] = true
		hints.EntityIDs = append(hints.EntityIDs, id)
	}
	for _, c := range rm.TermGraph.Canonical {
		switch c.Kind {
		case types.TermSymbol:
			addEntity(c.ID)
			addKeyword(c.ID)
		case types.TermConcept, types.TermConfig:
			addKeyword(c.ID)
		}
	}
	for _, candidate := range rm.AnalyzerHints.Entities {
		addEntity(candidate)
		addKeyword(candidate)
	}
	for _, candidate := range rm.AnalyzerHints.DerivedEntities {
		addEntity(candidate)
		addKeyword(candidate)
	}
	if profile := rm.ArtifactObservationProfile; profile != nil {
		for _, candidate := range profile.SubjectCandidates {
			addEntity(candidate)
			addKeyword(candidate)
		}
	}
	if profile := rm.ConversationReferenceProfile; profile != nil {
		for _, candidate := range profile.SubjectCandidates() {
			addEntity(candidate)
			addKeyword(candidate)
		}
	}
	if profile := rm.ChangeImpactProfile; profile != nil {
		for _, candidate := range profile.SubjectCandidates() {
			addEntity(candidate)
			addKeyword(candidate)
		}
	}
	return hints
}
