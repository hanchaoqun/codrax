package compiler

import (
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Template-level constants for DAG node thresholds. Named so a single
// grep surfaces every node budget, and changing a value doesn't
// require reading the full template function.
const (
	TmplEvidenceCountHigh   = 3  // evidence_count for moderate/complex templates
	TmplEvidenceCountLow    = 1  // evidence_count for simple/generic templates
	TmplCitationCountHigh   = 3  // architecture_explain, root_cause
	TmplCitationCountMedium = 2  // config_trace, performance_bottleneck
	TmplCitationCountLow    = 1  // generic
	TmplAnswerSetMaxSize    = 50 // answer_set_bounded
	TmplEvidenceMaxRetries  = 2  // evidence node MaxRetries
	TmplRetryBudgetHigh     = 3  // RetryBudget for complex templates
	TmplRetryBudgetVeryHigh = 4  // RetryBudget for root_cause (hardest)
	TmplRetryBudgetMedium   = 3  // RetryBudget for mid-complexity
	TmplRetryBudgetLow      = 2  // RetryBudget for simple/generic
)

// Each template builds the same three artifacts so call sites are
// interchangeable. The node skeletons are hand-tuned per scenario
// and kept small (3–5 nodes) so the scheduler has clear checkpoints
// without drowning in micro-stages.
//
// Every non-finalize node declares EntryConditions + SuccessCriteria
// as typed []Criterion values; the criterion package evaluates them
// at runtime. The pending artifact-exchange fields (Inputs, Outputs,
// ExitArtifacts) carry snake_case slot names that will become a
// typed data contract in a future milestone; gate only enforces
// format hygiene on them.

// critEntry builds an EntryCondition that waits on a named signal.
func critEntry(signal string) []types.Criterion {
	return []types.Criterion{{Kind: types.CritSignalPresent, Expr: signal}}
}


// ── architecture_explain ────────────────────────────────────────

func templateArchitectureExplain(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Breadth scan to locate relevant modules and entry points.",
		Inputs:      []string{"user_question", "term_graph"},
		Outputs:     []string{"file_candidates", "symbol_table"},
		SearchHints: hints,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountLow)},
		},
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective:   "Collect evidence for each architectural component the user asked about.",
		Inputs:      []string{"file_candidates", "symbol_table"},
		Outputs:     []string{"evidence_items", "answer_chains"},
		SearchHints: hints,
		MaxRetries:  TmplEvidenceMaxRetries,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountHigh)},
		},
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Check that every claimed symbol is backed by evidence and that answer chains terminate cleanly.",
		Inputs:    []string{"evidence_items", "answer_chains"},
		Outputs:   []string{"validation_report"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritAnswerSetBounded, Expr: "<=" + strconv.Itoa(TmplAnswerSetMaxSize)},
		},
	}
	reconcile := types.TaskNode{
		ID: nodeID(3, "reconcile"), Type: types.NodeReconcile,
		Objective:       "Reconcile evidence into a coherent architectural explanation.",
		Inputs:          []string{"validation_report", "answer_chains"},
		Outputs:         []string{"reconciled_story"},
		EntryConditions: critEntry("has_enough_facts"),
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the explanation with citations.",
		Inputs:    []string{"reconciled_story"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountHigh)},
		},
	}
	edges := chain(probe.ID, ev.ID, val.ID, reconcile.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "symbol_not_covered",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, reconcile, final},
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: TmplRetryBudgetMedium,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID},
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 30, "repomap": 40, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: types.CritContractSatisfied},
			{Kind: types.CritBudgetExhausted},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeExplanation,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountHigh},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountHigh)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── root_cause ──────────────────────────────────────────────────

func templateRootCause(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Locate failing component and reproduce the observed symptom in the codebase.",
		Inputs:      []string{"user_question", "symptom_description"},
		Outputs:     []string{"failing_component", "repro_sites"},
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective:   "Collect call sites, control flow, and recent changes that could explain the symptom.",
		Inputs:      []string{"failing_component", "repro_sites"},
		Outputs:     []string{"evidence_items", "hypothesis_bindings"},
		SearchHints: hints,
		MaxRetries:  TmplEvidenceMaxRetries,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountHigh)},
		},
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective:       "Falsify every hypothesis about root cause against the collected evidence.",
		Inputs:          []string{"evidence_items", "hypothesis_bindings"},
		Outputs:         []string{"validation_verdicts"},
		EntryConditions: critEntry("has_enough_facts"),
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritAllHypothesesDecided},
		},
	}
	reconcile := types.TaskNode{
		ID: nodeID(3, "reconcile"), Type: types.NodeReconcile,
		Objective: "Select the hypothesis with the strongest supporting evidence.",
		Inputs:    []string{"validation_verdicts"},
		Outputs:   []string{"root_cause_story"},
	}
	final := types.TaskNode{
		ID: nodeID(4, "finalize"), Type: types.NodeFinalize,
		Objective: "Present the root cause as a numbered step list with file:line citations.",
		Inputs:    []string{"root_cause_story"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
	}
	edges := chain(probe.ID, ev.ID, val.ID, reconcile.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "any_hypothesis_unknown",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, reconcile, final},
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: TmplRetryBudgetVeryHigh,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID},
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 40, "repomap": 25, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: types.CritAllHypothesesDecided},
			{Kind: types.CritBudgetExhausted},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeStepList,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountMedium},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── config_trace ────────────────────────────────────────────────

func templateConfigTrace(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Locate the config key in source and config files.",
		Inputs:      []string{"user_question", "config_key_hint"},
		Outputs:     []string{"definition_sites"},
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective:   "Trace default value, override precedence, and effective-value resolution.",
		Inputs:      []string{"definition_sites"},
		Outputs:     []string{"evidence_items", "resolution_chain"},
		SearchHints: hints,
		MaxRetries:  1,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountLow)},
		},
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Confirm the traced chain matches the user's scenario.",
		Inputs:    []string{"resolution_chain"},
		Outputs:   []string{"validation_report"},
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Report the concrete value with key path and file:line.",
		Inputs:    []string{"validation_report"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
	}
	edges := chain(probe.ID, ev.ID, val.ID, final.ID)
	edges = append(edges, types.TaskEdge{
		From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
		Guard: "chain_incomplete",
	})
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: TmplRetryBudgetLow,
			CriticalPath: []string{probe.ID, ev.ID, final.ID},
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 50, "repomap": 20, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: types.CritContractSatisfied},
			{Kind: types.CritBudgetExhausted},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeConfigValue,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountLow},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── performance_bottleneck ──────────────────────────────────────

func templatePerformanceBottleneck(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Locate the hot paths and measurement points relevant to the user's concern.",
		Inputs:      []string{"user_question", "perf_hints"},
		Outputs:     []string{"hot_path_candidates"},
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective:   "Collect loop structures, allocation sites, and any profiling hooks.",
		Inputs:      []string{"hot_path_candidates"},
		Outputs:     []string{"evidence_items", "ranked_bottlenecks"},
		SearchHints: hints,
		MaxRetries:  TmplEvidenceMaxRetries,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplCitationCountMedium)},
		},
	}
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Rank candidate bottlenecks by evidence weight.",
		Inputs:    []string{"ranked_bottlenecks"},
		Outputs:   []string{"final_ranking"},
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Return a short list of the highest-impact symbols to investigate.",
		Inputs:    []string{"final_ranking"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
	}
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, val, final},
		Edges: chain(probe.ID, ev.ID, val.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: TmplRetryBudgetMedium,
			CriticalPath: []string{probe.ID, ev.ID, val.ID, final.ID},
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 35, "repomap": 35, "read": 30},
		StopConditions: []types.StopCondition{
			{Kind: types.CritContractSatisfied},
			{Kind: types.CritBudgetExhausted},
		},
	}
	contract := types.AnswerContract{
		RequiredAnswerShape: types.ShapeListOfSymbols,
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountMedium},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── generic fallback ────────────────────────────────────────────

func templateGeneric(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Breadth scan for relevant files.",
		Inputs:      []string{"user_question"},
		Outputs:     []string{"file_candidates"},
		SearchHints: hints,
	}
	ev := types.TaskNode{
		ID: nodeID(1, "evidence"), Type: types.NodeEvidence,
		Objective:   "Collect evidence that answers the user's request.",
		Inputs:      []string{"file_candidates"},
		Outputs:     []string{"evidence_items"},
		SearchHints: hints,
		MaxRetries:  TmplEvidenceMaxRetries,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountLow)},
		},
	}
	final := types.TaskNode{
		ID: nodeID(2, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the answer.",
		Inputs:    []string{"evidence_items"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
	}
	graph := types.TaskGraph{
		Nodes: []types.TaskNode{probe, ev, final},
		Edges: chain(probe.ID, ev.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: TmplRetryBudgetLow,
			CriticalPath: []string{probe.ID, ev.ID, final.ID},
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 35, "repomap": 30, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: types.CritContractSatisfied},
			{Kind: types.CritBudgetExhausted},
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
		CitationReq:         types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountLow},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}
