// Package gate implements the Analyzer v3 deterministic quality
// gate. It runs a fixed list of checks against an AnalysisIR and
// returns a GateReport whose Rejected/Retryable flags tell the
// analyzer agent whether to accept the IR or retry.
//
// Every check is independent and the gate always runs all of them
// so the returned report lists every defect at once. The gate is
// observational only — fixes happen in the compiler (deterministic)
// or via LLM retry (non-deterministic).
//
// IsHardRejectingCheck is the single source of truth for whether a
// failed GateCheck should reject the analyzer output and spend an LLM
// retry. Keep this list conservative: hard checks must be precise,
// runtime-consuming invariants. Format-hygiene checks for future or
// advisory fields may fail their individual GateCheck for telemetry,
// but they must not flip GateReport.Rejected.
package gate

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/analysis/priority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RunOptions carries optional, non-Threshold inputs to the quality
// gate. Resolver, when non-nil, lets coherence checks query the
// repomap symbol table directly so sub-topic entity claims can be
// validated against repo ground truth (R1.4 axis_collapse + R1.5
// entity_unresolvable). Nil resolver is always safe — checks that
// require it become no-ops, preserving pre-RunOptions behaviour for
// tests / write mode / any caller without a graph.
type RunOptions struct {
	Resolver normalizer.SymbolResolver
}

// Thresholds control the numeric gate cutoffs.
type Thresholds struct {
	CoverageMin       float32         // default 0.9
	CoverageWeights   CoverageWeights // default {Symbol:1.0, Config:0.7, Concept:0.4}
	BudgetMinFiles    int             // default 5
	BudgetMinIters    int             // default 4
	HypothesisMinPrio int             // default 50 (human-scale 0-100)
}

// CoverageWeights assigns per-term-kind weight to the coverage
// check. Symbols weigh most because they are code identifiers that
// MUST appear in a search hint; config strings weigh second; pure
// concepts weigh least because they are mostly natural language.
type CoverageWeights struct {
	Symbol  float32
	Config  float32
	Concept float32
}

// globalThresholds is the runtime-installed gate cutoffs, set by
// cmd/root.go from codrax.yaml / CLI flags via SetGlobalThresholds.
// Zero value → withDefaults fills package-level defaults.
var globalThresholds Thresholds

// SetGlobalThresholds installs the runtime-supplied thresholds.
// Called from cmd/root.go alongside tool.SetAnalysisLimits.
func SetGlobalThresholds(t Thresholds) { globalThresholds = t }

// GlobalThresholds returns the installed thresholds (zero-value
// safe: withDefaults fills defaults on first use).
func GlobalThresholds() Thresholds { return globalThresholds }

func (t Thresholds) withDefaults() Thresholds {
	if t.CoverageMin == 0 {
		t.CoverageMin = 0.9
	}
	if t.CoverageWeights.Symbol == 0 {
		t.CoverageWeights.Symbol = 1.0
	}
	if t.CoverageWeights.Config == 0 {
		t.CoverageWeights.Config = 0.7
	}
	if t.CoverageWeights.Concept == 0 {
		t.CoverageWeights.Concept = 0.4
	}
	if t.BudgetMinFiles == 0 {
		t.BudgetMinFiles = 5
	}
	if t.BudgetMinIters == 0 {
		t.BudgetMinIters = 4
	}
	if t.HypothesisMinPrio == 0 {
		// Priorities are produced by priority.Score and unpacked via
		// priority.Raw into a 0-100 range. The default floor is 30:
		// a baseline Explain/Trace hypothesis on a 2-term graph
		// scores ~35 and should pass, whereas a hypothesis rejected
		// at every dimension (~0) must fail.
		t.HypothesisMinPrio = 30
	}
	return t
}

// Run evaluates every gate check against ir and returns a
// GateReport. The caller — typically the analyzer agent's
// ParseOutput — should set the IR's QualityGate field to the
// returned report and, if Rejected, act on Retryable.
//
// mode mirrors BusContext.Mode at the time Run() is called.
// When mode is anything other than "" / "read" (i.e. "plan" /
// "apply" / "verify"), the read-mode-specific quality checks
// (`hypothesis_coverage`, `contract_complete`) are skipped —
// they gate AnswerDocument quality and read-mode investigation
// depth, neither of which applies to the write pipeline. The
// planner doesn't consume the HypothesisSet and the
// AnswerContract is replaced by the write graph's criterion
// suite (CritPlanReady / CritPatchApplies / CritTestsPass /
// CritNoRegression).
//
// Without this gating, a "create from scratch" write request
// like "用 python 写一个猜数字游戏" reliably fails the analyzer's
// quality gate because no codebase entity to investigate exists →
// HypothesisSet either empty or all priorities ≤ 0 → check fails →
// retry budget burns trying to manufacture hypotheses for code
// that doesn't exist yet. Skipping the read-mode checks lets the
// analyzer finish classifying intent and hand off to the planner.
func Run(ir *types.AnalysisIR, th Thresholds, mode string) types.GateReport {
	return RunWith(ir, th, mode, RunOptions{})
}

// RunWith is the resolver-aware variant of Run. Existing callers that
// don't yet have a graph keep using Run (which forwards a zero-value
// RunOptions); the analyzer agent threads its already-built repomap
// resolver through opts.Resolver so checkSubtopicCoherence can validate
// sub-topic entities against repo ground truth.
func RunWith(ir *types.AnalysisIR, th Thresholds, mode string, opts RunOptions) types.GateReport {
	th = th.withDefaults()
	if ir == nil {
		return types.GateReport{
			Passed: false, Rejected: true, Retryable: false,
			Checks: []types.GateCheck{{Name: "nil_ir", Passed: false, Detail: "analyzer returned nil AnalysisIR"}},
		}
	}
	isWrite := mode != "" && mode != "read"

	var checks []types.GateCheck
	checks = append(checks, checkCoverage(ir, th))
	checks = append(checks, checkDAGClosure(ir))
	checks = append(checks, checkBudgetSanity(ir, th))
	if !isWrite {
		// Read-mode-only quality checks. The write pipeline carries
		// its own AnswerContract substitute (write_graph criterion
		// suite) and does not consume HypothesisSet at all, so these
		// checks would gate on irrelevant data.
		checks = append(checks, checkContractComplete(ir, th))
		checks = append(checks, checkHypothesisCoverage(ir, th))
		// Cross-signal coherence gates. Up-stream root-cause guards
		// for the multi-topic / shape-vs-subject mis-classification
		// patterns the downstream explorer / extractor / finalizer
		// layers historically had to clean up after the fact. Both
		// purely structural (no keyword tables) — they compare LLM-
		// emitted IR fields against each other and against the
		// repomap-verified TermGraph domains. R1.4 / R1.5 additionally
		// query the resolver in opts.Resolver to validate sub-topic
		// entity claims against repo ground truth; nil resolver makes
		// those sub-rules no-op (test mode / no graph).
		checks = append(checks, checkSubtopicCoherence(ir, opts.Resolver))
		checks = append(checks, checkShapeSubjectCoherence(ir))
	}
	checks = append(checks, checkCriterionResolvable(ir))
	checks = append(checks, checkPendingFieldsWellformed(ir))

	hardRejected := false
	for _, c := range checks {
		if IsHardRejectingCheck(c) {
			hardRejected = true
		}
	}
	report := types.GateReport{
		Passed: !hardRejected, Rejected: hardRejected, Retryable: hardRejected, Checks: checks,
	}
	// B6 (2026-05-10): stamp a fingerprint of the rejection shape
	// onto the report so the orchestrator's analyze retry loop can
	// detect a retry storm — same failure SHAPE repeated across
	// attempts means the LLM is stuck in a loop, not converging.
	// Empty fingerprint when not rejected; never affects passing
	// reports' wire shape (omitempty in GateReport struct).
	// Design doc: docs/design/multirepo_entity_scope_separation.md §4.6.
	if hardRejected {
		report.Fingerprint = computeGateFingerprint(report)
	}
	return report
}

// IsHardRejectingCheck reports whether this individual check should
// reject the analyzer IR. It intentionally reads only the typed check
// identity and pass bit, never user text or model prose.
//
// Keep this as an explicit allowlist. Analyzer gates often mix hard
// structural facts with advisory quality signals; a newly-added check
// must prove it is safe to spend an LLM retry before it joins this list.
func IsHardRejectingCheck(c types.GateCheck) bool {
	if c.Passed {
		return false
	}
	switch c.Name {
	case "nil_ir",
		"dag_closure",
		"budget_sanity",
		"contract_complete",
		"hypothesis_coverage",
		"subtopic_coherence",
		"shape_subject_coherence",
		"criterion_resolvable":
		return true
	default:
		return false
	}
}

// ── individual checks ──────────────────────────────────────────

func checkCoverage(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	totalWeight := float32(0)
	covered := float32(0)
	hinted := make(map[string]bool)
	for _, n := range ir.TaskGraph.Nodes {
		for _, id := range n.SearchHints.KeywordIDs {
			hinted[id] = true
		}
		for _, id := range n.SearchHints.EntityIDs {
			hinted[id] = true
		}
	}
	weightFor := func(k types.TermKind) float32 {
		switch k {
		case types.TermSymbol:
			return th.CoverageWeights.Symbol
		case types.TermConfig:
			return th.CoverageWeights.Config
		case types.TermConcept:
			return th.CoverageWeights.Concept
		}
		return 0
	}
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		w := weightFor(c.Kind)
		if w == 0 {
			continue
		}
		totalWeight += w
		if hinted[c.ID] {
			covered += w
		}
	}
	if totalWeight == 0 {
		return types.GateCheck{
			Name: "coverage", Passed: true, Score: 1.0, Threshold: th.CoverageMin,
			Detail: "no weighted terms to cover",
		}
	}
	score := covered / totalWeight
	return types.GateCheck{
		Name:      "coverage",
		Passed:    score >= th.CoverageMin,
		Score:     score,
		Threshold: th.CoverageMin,
		Detail: fmt.Sprintf("weighted coverage %.2f/%.2f (Symbol=%.1f Config=%.1f Concept=%.1f)",
			covered, totalWeight, th.CoverageWeights.Symbol, th.CoverageWeights.Config, th.CoverageWeights.Concept),
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
	if c.Language == "" {
		return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "language missing"}
	}
	if c.CitationReq.Required && c.CitationReq.MinCitations < 0 {
		return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "negative min_citations"}
	}
	if c.ExactResolution != nil {
		if len(c.ExactResolution.Targets) == 0 {
			return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "exact_resolution.targets missing"}
		}
		if strings.TrimSpace(c.ExactResolution.TargetLabel) == "" {
			return types.GateCheck{Name: "contract_complete", Passed: false, Detail: "exact_resolution.target_label missing"}
		}
		if policy := c.ExactResolution.RelatedContextPolicy; policy != "" && !policy.IsValid() {
			return types.GateCheck{Name: "contract_complete", Passed: false,
				Detail: fmt.Sprintf("invalid exact_resolution.related_context_policy %q", policy)}
		}
	}
	return types.GateCheck{Name: "contract_complete", Passed: true, Score: 1.0, Threshold: 1.0}
}

func checkHypothesisCoverage(ir *types.AnalysisIR, th Thresholds) types.GateCheck {
	highPrio := 0
	for _, h := range ir.HypothesisSet {
		if priority.Raw(h.Priority) >= th.HypothesisMinPrio {
			highPrio++
		}
	}
	if len(ir.HypothesisSet) > 0 && highPrio == 0 {
		return types.GateCheck{Name: "hypothesis_coverage", Passed: true,
			Score: 0.7, Threshold: 1.0,
			Detail: "hypothesis_priority_floor (advisory): no hypothesis at or above min priority"}
	}
	if miss := hdp.Validate(ir.TaskGraph); len(miss) > 0 {
		return types.GateCheck{Name: "hypothesis_coverage", Passed: false,
			Detail: fmt.Sprintf("%d unbound node(s): %v", len(miss), miss)}
	}
	return types.GateCheck{Name: "hypothesis_coverage", Passed: true, Score: 1.0, Threshold: 1.0}
}

// checkCriterionResolvable scans every Criterion-bearing field in
// the IR and fails hard if any Kind is not registered in the
// criterion package. This is the static guarantee the scheduler /
// extractor rely on to panic on unknown-kind observation at
// runtime.
func checkCriterionResolvable(ir *types.AnalysisIR) types.GateCheck {
	var unknown []string
	visit := func(c types.Criterion, where string) {
		if c.Kind == "" {
			unknown = append(unknown, fmt.Sprintf("%s: empty Kind", where))
			return
		}
		if !criterion.IsRegistered(criterion.Kind(c.Kind)) {
			unknown = append(unknown, fmt.Sprintf("%s: %q", where, c.Kind))
		}
	}
	for _, n := range ir.TaskGraph.Nodes {
		for _, c := range n.EntryConditions {
			visit(c, "node "+n.ID+".entry")
		}
		for _, c := range n.SuccessCriteria {
			visit(c, "node "+n.ID+".success")
		}
	}
	for _, h := range ir.HypothesisSet {
		for _, c := range h.RequiredEvidence {
			visit(c, "hyp "+h.ID+".required_evidence")
		}
		visit(h.FalsificationCondition, "hyp "+h.ID+".falsification")
	}
	for _, sc := range ir.EvidencePlan.StopConditions {
		visit(types.Criterion{Kind: sc.Kind, Expr: sc.Expr}, "stop_condition")
	}
	for _, a := range ir.AnswerContract.AcceptanceTests {
		visit(a, "acceptance_test")
	}
	if len(unknown) > 0 {
		joined := ""
		for i, u := range unknown {
			if i > 0 {
				joined += "; "
			}
			joined += u
		}
		return types.GateCheck{
			Name: "criterion_resolvable", Passed: false,
			Detail: fmt.Sprintf("%d unregistered kind(s): %s", len(unknown), joined),
		}
	}
	return types.GateCheck{Name: "criterion_resolvable", Passed: true, Score: 1.0, Threshold: 1.0}
}

var pendingSlotName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// checkPendingFieldsWellformed is the SOFT gate check for the
// artifact-exchange pending fields. It verifies:
//
//   - TaskNode.Inputs / Outputs / ExitArtifacts entries match
//     snake_case_identifier so a future typed-artifact milestone
//     can adopt them without a cleanup pass.
//   - EvidencePlan.SourceMix values sum to ~100 (±5) when the
//     template declared any, so the ratio semantics stay honest.
//
// Failures here produce a warning rather than a hard reject —
// pending fields have no runtime consumer.
func checkPendingFieldsWellformed(ir *types.AnalysisIR) types.GateCheck {
	var issues []string
	for _, n := range ir.TaskGraph.Nodes {
		for _, slot := range n.Inputs {
			if !pendingSlotName.MatchString(slot) {
				issues = append(issues, fmt.Sprintf("node %s input %q not snake_case", n.ID, slot))
			}
		}
		for _, slot := range n.Outputs {
			if !pendingSlotName.MatchString(slot) {
				issues = append(issues, fmt.Sprintf("node %s output %q not snake_case", n.ID, slot))
			}
		}
		for _, slot := range n.ExitArtifacts {
			if !pendingSlotName.MatchString(slot) {
				issues = append(issues, fmt.Sprintf("node %s exit_artifact %q not snake_case", n.ID, slot))
			}
		}
	}
	if mix := ir.EvidencePlan.SourceMix; len(mix) > 0 {
		sum := 0
		for _, v := range mix {
			sum += v
		}
		if sum < 95 || sum > 105 {
			issues = append(issues, fmt.Sprintf("source_mix sums to %d, expected 100±5", sum))
		}
	}
	if len(issues) > 0 {
		joined := ""
		for i, s := range issues {
			if i > 0 {
				joined += "; "
			}
			joined += s
		}
		return types.GateCheck{
			Name: "pending_fields_wellformed", Passed: false,
			Detail: joined,
		}
	}
	return types.GateCheck{Name: "pending_fields_wellformed", Passed: true, Score: 1.0, Threshold: 1.0}
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

func findCycle(g types.TaskGraph) string {
	adj := make(map[string][]string, len(g.Nodes))
	for _, e := range g.Edges {
		if e.EdgeType != types.EdgeHardDependency {
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
	}
	color := make(map[string]int)
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
