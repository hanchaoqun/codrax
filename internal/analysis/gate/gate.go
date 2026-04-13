// Package gate implements the Analyzer v3 deterministic quality
// gate. It runs a fixed list of checks against an AnalysisIR and
// returns a GateReport whose Rejected/Retryable flags tell the
// analyzer agent whether to accept the IR, retry the analyze stage,
// or surface a hard error to the user.
//
// The checks are intentionally cheap and side-effect-free so the
// gate can be re-run cheaply after every analyze retry. Each check
// is independent: a later check is still evaluated even if an
// earlier check failed, so the final GateReport lists every defect
// at once rather than one at a time.
//
// The gate is never responsible for *fixing* an IR — only for
// observing it. Fixes happen either in the scenario compiler
// (deterministic) or via an LLM retry (non-deterministic).
package gate

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Thresholds control the numeric gate cutoffs. Zero value produces
// the defaults documented in the refactor plan §4.7.
type Thresholds struct {
	CoverageMin       float32 // default 0.9
	BudgetMinFiles    int     // default 5
	BudgetMinIters    int     // default 4
	HypothesisMinPrio int     // default 50
}

func (t Thresholds) withDefaults() Thresholds {
	if t.CoverageMin == 0 {
		t.CoverageMin = 0.9
	}
	if t.BudgetMinFiles == 0 {
		t.BudgetMinFiles = 5
	}
	if t.BudgetMinIters == 0 {
		t.BudgetMinIters = 4
	}
	if t.HypothesisMinPrio == 0 {
		t.HypothesisMinPrio = 50
	}
	return t
}

// Run evaluates every gate check against ir and returns a
// GateReport. The caller — typically the analyzer agent's
// ParseOutput — should set the IR's QualityGate field to the
// returned report and, if Rejected, act on Retryable.
func Run(ir *types.AnalysisIR, th Thresholds) types.GateReport {
	th = th.withDefaults()
	if ir == nil {
		return types.GateReport{
			Passed:    false,
			Rejected:  true,
			Retryable: false,
			Checks: []types.GateCheck{
				{Name: "nil_ir", Passed: false, Detail: "analyzer returned nil AnalysisIR"},
			},
		}
	}

	var checks []types.GateCheck
	checks = append(checks, checkCoverage(ir, th))
	checks = append(checks, checkDAGClosure(ir))
	checks = append(checks, checkBudgetSanity(ir, th))
	checks = append(checks, checkContractComplete(ir, th))
	checks = append(checks, checkHypothesisCoverage(ir, th))
	checks = append(checks, checkRiskConsistency(ir))

	passed := true
	for _, c := range checks {
		if !c.Passed && c.Name != "risk_consistency" {
			passed = false
		}
	}

	return types.GateReport{
		Passed:   passed,
		Rejected: !passed,
		// All checks but risk_consistency are retryable on their own
		// since they could plausibly be fixed by a second analyze
		// pass. risk_consistency is a warning only.
		Retryable: !passed,
		Checks:    checks,
	}
}

// ── individual checks ──────────────────────────────────────────

func checkCoverage(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	// Coverage proxy: every canonical symbol / config term in the
	// request must appear in at least one task node's SearchHints.
	// Concept-only terms don't count toward coverage because the
	// normalizer may emit them for context rather than search.
	important := 0
	hinted := make(map[string]bool)
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		if c.Kind != types.TermSymbol && c.Kind != types.TermConfig {
			continue
		}
		important++
	}
	if important == 0 {
		return types.GateCheck{Name: "coverage", Passed: true, Score: 1.0, Threshold: th.CoverageMin,
			Detail: "no critical terms to cover"}
	}
	for _, n := range ir.TaskGraph.Nodes {
		for _, id := range n.SearchHints.KeywordIDs {
			hinted[id] = true
		}
		for _, id := range n.SearchHints.EntityIDs {
			hinted[id] = true
		}
	}
	covered := 0
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		if c.Kind != types.TermSymbol && c.Kind != types.TermConfig {
			continue
		}
		if hinted[c.ID] {
			covered++
		}
	}
	score := float32(covered) / float32(important)
	return types.GateCheck{
		Name:      "coverage",
		Passed:    score >= th.CoverageMin,
		Score:     score,
		Threshold: th.CoverageMin,
		Detail:    fmt.Sprintf("%d/%d critical terms appear in node search hints", covered, important),
	}
}

func checkDAGClosure(ir *types.AnalysisIR) types.GateCheck {
	g := ir.TaskGraph
	if len(g.Nodes) == 0 {
		return types.GateCheck{Name: "dag_closure", Passed: false, Detail: "empty task graph"}
	}
	idSet := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		if idSet[n.ID] {
			return types.GateCheck{Name: "dag_closure", Passed: false,
				Detail: fmt.Sprintf("duplicate node id %q", n.ID)}
		}
		idSet[n.ID] = true
	}
	for _, e := range g.Edges {
		if !idSet[e.From] || !idSet[e.To] {
			return types.GateCheck{Name: "dag_closure", Passed: false,
				Detail: fmt.Sprintf("dangling edge %s→%s", e.From, e.To)}
		}
	}
	if cyc := findCycle(g); cyc != "" {
		return types.GateCheck{Name: "dag_closure", Passed: false,
			Detail: fmt.Sprintf("cycle detected through %s", cyc)}
	}
	if findFirstByType(g, types.NodeFinalize) == "" {
		return types.GateCheck{Name: "dag_closure", Passed: false, Detail: "no finalize node"}
	}
	return types.GateCheck{Name: "dag_closure", Passed: true, Score: 1.0, Threshold: 1.0}
}

func checkBudgetSanity(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	b := ir.EvidencePlan.Budget
	if b.MaxFiles < th.BudgetMinFiles || b.MaxReactIters < th.BudgetMinIters {
		return types.GateCheck{
			Name: "budget_sanity", Passed: false,
			Detail: fmt.Sprintf("budget too tight: max_files=%d (need ≥%d), max_react_iters=%d (need ≥%d)",
				b.MaxFiles, th.BudgetMinFiles, b.MaxReactIters, th.BudgetMinIters),
		}
	}
	// Upper sanity bound: refuse absurdly large budgets so a rogue
	// LLM cannot DoS the pipeline.
	if b.MaxFiles > 1000 || b.MaxReactIters > 200 {
		return types.GateCheck{
			Name: "budget_sanity", Passed: false,
			Detail: fmt.Sprintf("budget absurdly high: max_files=%d, max_react_iters=%d", b.MaxFiles, b.MaxReactIters),
		}
	}
	return types.GateCheck{Name: "budget_sanity", Passed: true, Score: 1.0, Threshold: 1.0}
}

func checkContractComplete(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	_ = th
	c := ir.AnswerContract
	if c.RequiredAnswerShape == "" {
		return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "required_answer_shape missing"}
	}
	if !isKnownShape(c.RequiredAnswerShape) {
		return types.GateCheck{Name: "contract_complete", Passed: false,
			Detail: fmt.Sprintf("unknown answer shape %q", c.RequiredAnswerShape)}
	}
	if c.Language == "" {
		return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "language missing"}
	}
	if c.CitationReq.Required && c.CitationReq.MinCitations < 0 {
		return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "negative min_citations"}
	}
	return types.GateCheck{Name: "contract_complete", Passed: true, Score: 1.0, Threshold: 1.0}
}

func checkHypothesisCoverage(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	// Rule 1: at least one high-priority hypothesis exists.
	highPrio := 0
	for _, h := range ir.HypothesisSet {
		if h.Priority >= th.HypothesisMinPrio {
			highPrio++
		}
	}
	if len(ir.HypothesisSet) > 0 && highPrio == 0 {
		return types.GateCheck{Name: "hypothesis_coverage", Passed: false,
			Detail: "no hypothesis at or above min priority"}
	}
	// Rule 2: every binding-eligible node must have ≥1 hypothesis.
	if miss := hdp.Validate(ir.TaskGraph); len(miss) > 0 {
		return types.GateCheck{Name: "hypothesis_coverage", Passed: false,
			Detail: fmt.Sprintf("%d unbound node(s): %v", len(miss), miss)}
	}
	return types.GateCheck{Name: "hypothesis_coverage", Passed: true, Score: 1.0, Threshold: 1.0}
}

func checkRiskConsistency(ir *types.AnalysisIR) types.GateCheck {
	// Warning-only: writing=true but every risk dim is zero.
	// The policy designer left this as non-fatal because the
	// analyzer may legitimately decide a change is risk-free.
	if !ir.RunPolicy.Writing {
		return types.GateCheck{Name: "risk_consistency", Passed: true, Score: 1.0, Threshold: 1.0}
	}
	rm := ir.RequestModel.RiskMatrix
	sum := rm.Security.Level + rm.DataIntegrity.Level + rm.Compatibility.Level +
		rm.Performance.Level + rm.Ops.Level + rm.Compliance.Level
	if sum == 0 {
		return types.GateCheck{Name: "risk_consistency", Passed: true, // warn only
			Detail: "writing=true with all-zero risk matrix (warning)"}
	}
	return types.GateCheck{Name: "risk_consistency", Passed: true, Score: 1.0, Threshold: 1.0}
}

// ── helpers ────────────────────────────────────────────────────

func findFirstByType(g types.TaskGraph, t types.TaskNodeType) string {
	for _, n := range g.Nodes {
		if n.Type == t {
			return n.ID
		}
	}
	return ""
}

func isKnownShape(s types.AnswerShape) bool {
	switch s {
	case types.ShapeListOfSymbols, types.ShapeStepList, types.ShapeValue,
		types.ShapeBoolean, types.ShapeConfigValue, types.ShapeExplanation, types.ShapeNone:
		return true
	}
	return false
}

// findCycle returns a node ID involved in a hard-dependency cycle,
// or "" if the graph is acyclic. Only hard_dependency edges count —
// validation_feedback edges are by design cycles back to earlier
// stages, so including them would flag every root_cause graph.
func findCycle(g types.TaskGraph) string {
	adj := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		if e.EdgeType != types.EdgeHardDependency {
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
	}
	color := make(map[string]int) // 0=unseen,1=in-stack,2=done
	var visit func(id string) string
	visit = func(id string) string {
		if color[id] == 1 {
			return id
		}
		if color[id] == 2 {
			return ""
		}
		color[id] = 1
		for _, next := range adj[id] {
			if found := visit(next); found != "" {
				return found
			}
		}
		color[id] = 2
		return ""
	}
	for _, n := range g.Nodes {
		if found := visit(n.ID); found != "" {
			return found
		}
	}
	return ""
}
