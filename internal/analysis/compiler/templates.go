package compiler

import "github.com/hanchaoqun/codrax/internal/types"

// Each template builds the same three artifacts so call sites are
// interchangeable. The node skeletons are hand-tuned per scenario
// and kept small (3–5 nodes) so the orchestrator's DAG scheduler has
// clear checkpoints without drowning in micro-stages.
//
// Node naming convention:
//
//	probe       → fast breadth scan (keyword search + repo_map)
//	evidence    → deep evidence collection per hypothesis
//	validate    → falsification check against hypotheses
//	design      → plan artifact (writing only)
//	implement   → code patch (writing only)
//	review      → design/code review (writing only)
//	verify      → tests/build (writing only)
//	reconcile   → counterfactual branch collapse
//	finalize    → answer synthesis

// ── architecture_explain ────────────────────────────────────────

func templateArchitectureExplain(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Breadth scan to locate relevant modules and entry points.",
		Outputs:   []string{"file_candidates", "symbol_table"},
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Collect evidence for each architectural component the user asked about.",
		SearchHints: hints,
		ExitArtifacts: []string{"evidence_items", "answer_chains"},
		MaxRetries: 2,
	}
	// validate node (review P2-1): when B5 removes the finalizer's
	// S3 in-loop symbol validation, the DAG must carry the check so
	// the answer chain's symbols stay closed under evidence.
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Check that every claimed symbol is backed by evidence and that answer chains terminate cleanly.",
	}
	reconcile := types.TaskNode{
		ID: nodeID(3, "reconcile"), Type: types.NodeReconcile,
		Objective: "Reconcile evidence into a coherent architectural explanation.",
		EntryConditions: []string{"has_enough_facts"},
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the explanation with citations.",
	}
	edges := chain(probe.ID, ev.ID, val.ID, reconcile.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "symbol_not_covered",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, reconcile, final},
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 3,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 40, 20),
		SourceMix: map[string]int{"grep": 30, "repomap": 40, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: "contract_satisfied"},
			{Kind: "budget_exhausted"},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 3},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── root_cause ──────────────────────────────────────────────────

func templateRootCause(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Locate failing component and reproduce the observed symptom in the codebase.",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Collect call sites, control flow, and recent changes that could explain the symptom.",
		SearchHints: hints, MaxRetries: 2,
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Falsify every hypothesis about root cause against the collected evidence.",
		EntryConditions: []string{"has_enough_facts"},
	}
	reconcile := types.TaskNode{
		ID: nodeID(3, "reconcile"), Type: types.NodeReconcile,
		Objective: "Select the hypothesis with the strongest supporting evidence.",
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "Present the root cause as a numbered step list with file:line citations.",
	}
	edges := chain(probe.ID, ev.ID, val.ID, reconcile.ID, final.ID)
	// validation_feedback from validate → evidence so a rejected
	// hypothesis forces another evidence pass instead of finalize.
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "any_hypothesis_unknown",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, reconcile, final}, Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 4,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 50, 24),
		SourceMix: map[string]int{"grep": 40, "repomap": 25, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: "all_hypotheses_decided"},
			{Kind: "budget_exhausted"},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeStepList,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── security_audit ──────────────────────────────────────────────

func templateSecurityAudit(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Enumerate entry points (HTTP handlers, RPC endpoints, auth middleware, deserialization paths).",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "For each attack surface, collect code that validates, sanitizes, or bypasses checks.",
		SearchHints: hints, MaxRetries: 3,
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Check each hypothesized vulnerability against the collected evidence; reject the ones that hold.",
	}
	review := types.TaskNode{
		ID: nodeID(3, "review"), Type: types.NodeReview,
		Objective: "Second-pass review of findings for false positives and severity grading.",
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "List confirmed vulnerabilities with CWE-style classification and file:line.",
	}
	edges := chain(probe.ID, ev.ID, val.ID, review.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "any_hypothesis_unknown",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, review, final}, Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 5,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, review.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 60, 26),
		SourceMix: map[string]int{"grep": 45, "repomap": 20, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: "all_hypotheses_decided"},
			{Kind: "budget_exhausted"},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line_range", MinCitations: 1},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── refactor_design ─────────────────────────────────────────────

func templateRefactorDesign(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Map the modules and public interfaces touched by the proposed refactor.",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Collect current usage sites, tests, and coupling points.",
		SearchHints: hints, MaxRetries: 2,
	}
	design := types.TaskNode{
		ID: nodeID(2, "design"), Type: types.NodeDesign,
		Objective: "Produce a batched migration plan with explicit invariants and rollback strategy.",
	}
	review := types.TaskNode{
		ID: nodeID(3, "review"), Type: types.NodeReview,
		Objective: "Review the plan for coverage, reversibility, and test impact.",
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the final design document.",
	}
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, design, review, final},
		Edges: chain(probe.ID, ev.ID, design.ID, review.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 4,
			CriticalPath: []string{probe.ID, ev.ID, design.ID, review.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 50, 22),
		SourceMix: map[string]int{"grep": 30, "repomap": 40, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: "contract_satisfied"},
			{Kind: "budget_exhausted"},
		},
	}
	// No hardcoded acceptance test (review P2-3): not every refactor
	// requires a rollback — rename-only refactors or deliberate
	// no-back-compat rewrites (like Analyzer v3 itself) are legal
	// answers too. Acceptance tests must come from the LLM's own
	// RequestModel when warranted.
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── config_trace ────────────────────────────────────────────────

func templateConfigTrace(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Locate the config key in source and config files.",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Trace default value, override precedence, and effective-value resolution.",
		SearchHints: hints, MaxRetries: 1,
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Confirm the traced chain matches the user's scenario.",
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Report the concrete value with key path and file:line.",
	}
	// validation_feedback (review P2-2): a broken resolution chain
	// should send us back to collect more evidence, not straight to
	// finalize with a gap.
	edges := chain(probe.ID, ev.ID, val.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "chain_incomplete",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 2,
			CriticalPath: []string{probe.ID, ev.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 25, 14),
		SourceMix: map[string]int{"grep": 50, "repomap": 20, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: "contract_satisfied"},
			{Kind: "budget_exhausted"},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeConfigValue,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── performance_bottleneck ──────────────────────────────────────

func templatePerformanceBottleneck(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Locate the hot paths and measurement points relevant to the user's concern.",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Collect loop structures, allocation sites, and any profiling hooks.",
		SearchHints: hints, MaxRetries: 2,
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Rank candidate bottlenecks by evidence weight.",
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Return a short list of the highest-impact symbols to investigate.",
	}
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: chain(probe.ID, ev.ID, val.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 3,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 35, 18),
		SourceMix: map[string]int{"grep": 35, "repomap": 35, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: "contract_satisfied"},
			{Kind: "budget_exhausted"},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── generic fallback ────────────────────────────────────────────

func templateGeneric(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective: "Breadth scan for relevant files.",
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective: "Collect evidence that answers the user's request.",
		SearchHints: hints, MaxRetries: 2,
	}
	final := types.TaskNode{
		ID: nodeID(2, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the answer.",
	}
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, final},
		Edges: chain(probe.ID, ev.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{MaxParallelism: 1, RetryBudget: 2,
			CriticalPath: []string{probe.ID, ev.ID, final.ID}},
	}
	plan := types.EvidencePlan{
		Budget:    defaultBudget(rm, 30, 16),
		SourceMix: map[string]int{"grep": 35, "repomap": 30, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: "contract_satisfied"},
			{Kind: "budget_exhausted"},
		},
	}
	shape := types.ShapeExplanation
	if rm.Intent == types.IntentReturnValue {
		shape = types.ShapeValue
	} else if rm.Intent == types.IntentEnumerate {
		shape = types.ShapeListOfSymbols
	} else if rm.Intent == types.IntentConfigQuery {
		shape = types.ShapeConfigValue
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: shape,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
		Language:            rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}
