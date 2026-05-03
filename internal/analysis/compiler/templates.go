package compiler

import (
	"fmt"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Template-level constants for DAG node thresholds. Named so a single
// grep surfaces every node budget, and changing a value doesn't
// require reading the full template function.
const (
	TmplEvidenceCountHigh   = 3  // evidence_count for moderate/complex templates
	TmplEvidenceCountMedium = 2  // evidence_count for structural trace templates
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

// subTopicRetryBudgetBoost returns the extra retry budget for multi-
// topic questions. The boost is len(SubTopics)/2, capped so the total
// never exceeds 5.
func subTopicRetryBudgetBoost(rm types.RequestModel, base int) int {
	n := len(rm.SubTopics)
	if n <= 1 {
		return base
	}
	boost := n / 2
	if boost < 1 {
		boost = 1
	}
	adjusted := base + boost
	if adjusted > 5 {
		adjusted = 5
	}
	return adjusted
}

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

// EvidenceNodeSpec carries the per-template fields expandEvidenceNodes
// needs to build evidence-type TaskNodes. Every template supplies its
// own IDPrefix / Inputs / Outputs / default Objective / default
// Hints / EvidenceCount floor so the helper can handle both the
// single-topic and multi-sub-topic cases uniformly.
type EvidenceNodeSpec struct {
	IDPrefix      string            // node ID prefix; multi-topic appends "_tN"
	Inputs        []string          // default inputs (used by every emitted node)
	Outputs       []string          // default outputs (used by every emitted node)
	Objective     string            // used when len(SubTopics)<=1
	Hints         types.SearchHints // used when len(SubTopics)<=1
	EvidenceCount int               // floor for CritEvidenceCount success criterion
}

// expandEvidenceNodes returns one evidence node per sub-topic when
// rm.SubTopics is non-empty, or a single evidence node otherwise.
// The per-sub-topic path uses the sub-topic's Summary as its
// Objective and the sub-topic's Entities as SearchHints — every
// other field is inherited from the spec.
//
// Every non-generic template now routes evidence node creation
// through this helper so the multi-topic branch works identically
// across scenarios; before this was only wired for templateGeneric
// and sub-topic breakdowns silently degenerated to a single evidence
// node in architecture_explain / root_cause / config_trace /
// performance_bottleneck.
func expandEvidenceNodes(rm types.RequestModel, spec EvidenceNodeSpec) []types.TaskNode {
	evCount := spec.EvidenceCount
	if evCount <= 0 {
		evCount = TmplEvidenceCountLow
	}
	if len(rm.SubTopics) <= 1 {
		return []types.TaskNode{{
			ID:          spec.IDPrefix,
			Type:        types.NodeEvidence,
			Objective:   spec.Objective,
			Inputs:      spec.Inputs,
			Outputs:     spec.Outputs,
			SearchHints: spec.Hints,
			MaxRetries:  TmplEvidenceMaxRetries,
			SuccessCriteria: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(evCount)},
			},
		}}
	}
	nodes := make([]types.TaskNode, len(rm.SubTopics))
	for i, st := range rm.SubTopics {
		h := types.SearchHints{}
		for _, e := range st.Entities {
			h.EntityIDs = append(h.EntityIDs, e)
		}
		nodes[i] = types.TaskNode{
			ID:          fmt.Sprintf("%s_t%d", spec.IDPrefix, i),
			Type:        types.NodeEvidence,
			Objective:   st.Summary,
			Inputs:      spec.Inputs,
			Outputs:     spec.Outputs,
			SearchHints: h,
			MaxRetries:  TmplEvidenceMaxRetries,
			SuccessCriteria: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(evCount)},
			},
		}
	}
	return nodes
}

// evidenceChain wires probe → every evidence node → next as
// hard-dependency edges. Replaces the `chain(probe.ID, ev.ID, ...)`
// pattern for templates that use expandEvidenceNodes — chain() only
// supports a single evidence id in the sequence.
func evidenceChain(probeID string, evNodes []types.TaskNode, downstream ...string) []types.TaskEdge {
	var edges []types.TaskEdge
	for _, ev := range evNodes {
		edges = append(edges, types.TaskEdge{
			From: probeID, To: ev.ID, EdgeType: types.EdgeHardDependency,
		})
		if len(downstream) > 0 {
			edges = append(edges, types.TaskEdge{
				From: ev.ID, To: downstream[0], EdgeType: types.EdgeHardDependency,
			})
		}
	}
	for i := 1; i < len(downstream); i++ {
		edges = append(edges, types.TaskEdge{
			From: downstream[i-1], To: downstream[i], EdgeType: types.EdgeHardDependency,
		})
	}
	return edges
}

// evidenceCriticalPath returns probe → first-evidence → downstream…
// Keeps the critical path single-threaded (picking one evidence node
// representative) regardless of fan-out on the sub-topic axis.
func evidenceCriticalPath(probeID string, evNodes []types.TaskNode, downstream ...string) []string {
	path := []string{probeID}
	if len(evNodes) > 0 {
		path = append(path, evNodes[0].ID)
	}
	path = append(path, downstream...)
	return path
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
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"file_candidates", "symbol_table"},
		Outputs:       []string{"evidence_items", "answer_chains"},
		Objective:     "Collect evidence for each architectural component the user asked about.",
		Hints:         hints,
		EvidenceCount: TmplEvidenceCountHigh,
	})
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
	edges := evidenceChain(probe.ID, evNodes, val.ID, reconcile.ID, final.ID)
	// validation_feedback edges fan out to every evidence node so a
	// rejected validate pass can re-trigger any sub-topic's evidence
	// collection, not just the first.
	for _, ev := range evNodes {
		edges = append(edges, types.TaskEdge{
			From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
			Guard: "symbol_not_covered",
		})
	}
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, val, reconcile, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetMedium),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, val.ID, final.ID),
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
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountHigh},
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
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"failing_component", "repro_sites"},
		Outputs:       []string{"evidence_items", "hypothesis_bindings"},
		Objective:     "Collect call sites, control flow, and recent changes that could explain the symptom.",
		Hints:         hints,
		EvidenceCount: TmplEvidenceCountHigh,
	})
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
	edges := evidenceChain(probe.ID, evNodes, val.ID, reconcile.ID, final.ID)
	for _, ev := range evNodes {
		edges = append(edges, types.TaskEdge{
			From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
			Guard: "any_hypothesis_unknown",
		})
	}
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, val, reconcile, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetVeryHigh),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, val.ID, final.ID),
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
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountMedium},
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
		Objective:   "Locate the exact config key in source and config files; if it is absent, preserve that absence as the primary finding before considering related keys.",
		Inputs:      []string{"user_question", "config_key_hint"},
		Outputs:     []string{"definition_sites"},
		SearchHints: hints,
	}
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"definition_sites"},
		Outputs:       []string{"evidence_items", "resolution_chain"},
		Objective:     "Trace default value, override precedence, and effective-value resolution for the exact key only; nearby keys are contextual leads, not substitutes.",
		Hints:         hints,
		EvidenceCount: TmplEvidenceCountLow,
	})
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Confirm the traced chain matches the user's exact config key; reject substituting a nearby key when the requested key is absent.",
		Inputs:    []string{"resolution_chain"},
		Outputs:   []string{"validation_report"},
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Report the concrete value with key path and file:line when the exact key exists; otherwise report the exact-key absence and clearly separate related knobs.",
		Inputs:    []string{"validation_report"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
	}
	edges := evidenceChain(probe.ID, evNodes, val.ID, final.ID)
	for _, ev := range evNodes {
		edges = append(edges, types.TaskEdge{
			From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
			Guard: "chain_incomplete",
		})
	}
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, val, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetLow),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, final.ID),
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
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountLow},
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
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"hot_path_candidates"},
		Outputs:       []string{"evidence_items", "ranked_bottlenecks"},
		Objective:     "Collect loop structures, allocation sites, and any profiling hooks.",
		Hints:         hints,
		EvidenceCount: TmplCitationCountMedium,
	})
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
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, val, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: evidenceChain(probe.ID, evNodes, val.ID, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetMedium),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, val.ID, final.ID),
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
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountMedium},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

// ── generic fallback ────────────────────────────────────────────

func templateTraceWalkthrough(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Locate the concrete entry points and linear path segments relevant to the requested trace.",
		Inputs:      []string{"user_question", "term_graph"},
		Outputs:     []string{"file_candidates", "symbol_table"},
		SearchHints: hints,
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=" + strconv.Itoa(TmplEvidenceCountLow)},
		},
	}
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"file_candidates", "symbol_table"},
		Outputs:       []string{"evidence_items", "answer_chains"},
		Objective:     "Collect grounded steps for the requested call / flow / registration walkthrough.",
		Hints:         hints,
		EvidenceCount: TmplEvidenceCountMedium,
	})
	val := types.TaskNode{
		ID: nodeID(2, "validate"), Type: types.NodeValidate,
		Objective: "Check that the traced steps are grounded, ordered, and terminate on the requested target.",
		Inputs:    []string{"evidence_items", "answer_chains"},
		Outputs:   []string{"validation_report"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritAnswerSetBounded, Expr: "<=" + strconv.Itoa(TmplAnswerSetMaxSize)},
		},
	}
	final := types.TaskNode{
		ID: nodeID(3, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the traced explanation with citations.",
		Inputs:    []string{"validation_report", "answer_chains"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
	}
	edges := evidenceChain(probe.ID, evNodes, val.ID, final.ID)
	for _, ev := range evNodes {
		edges = append(edges, types.TaskEdge{
			From: val.ID, To: ev.ID, EdgeType: types.EdgeValidationFeedback,
			Guard: "symbol_not_covered",
		})
	}
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, val, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: edges,
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetMedium),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, val.ID, final.ID),
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
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountMedium},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountMedium)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}

func templateGeneric(rm types.RequestModel) Output {
	hints := hintsFromRM(rm)
	probe := types.TaskNode{
		ID: nodeID(0, "probe"), Type: types.NodeProbe,
		Objective:   "Breadth scan for relevant files.",
		Inputs:      []string{"user_question"},
		Outputs:     []string{"file_candidates"},
		SearchHints: hints,
	}
	evNodes := expandEvidenceNodes(rm, EvidenceNodeSpec{
		IDPrefix:      nodeID(1, "evidence"),
		Inputs:        []string{"user_question"},
		Outputs:       []string{"evidence_items"},
		Objective:     "Collect evidence that answers the user's request.",
		Hints:         hints,
		EvidenceCount: TmplEvidenceCountLow,
	})
	finalIdx := 1 + len(evNodes)
	final := types.TaskNode{
		ID: nodeID(finalIdx, "finalize"), Type: types.NodeFinalize,
		Objective: "Render the answer.",
		Inputs:    []string{"evidence_items"},
		Outputs:   []string{"answer_document"},
		SuccessCriteria: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
	}
	nodes := []types.TaskNode{probe}
	nodes = append(nodes, evNodes...)
	nodes = append(nodes, final)
	graph := types.TaskGraph{
		Nodes: nodes,
		Edges: evidenceChain(probe.ID, evNodes, final.ID),
		ExecutionPolicy: types.ExecutionPolicy{
			MaxParallelism: 1, RetryBudget: subTopicRetryBudgetBoost(rm, TmplRetryBudgetLow),
			CriticalPath: evidenceCriticalPath(probe.ID, evNodes, final.ID),
		},
	}
	plan := types.EvidencePlan{
		SourceMix: map[string]int{"grep": 35, "repomap": 30, "read": 35},
		StopConditions: []types.StopCondition{
			{Kind: types.CritContractSatisfied},
			{Kind: types.CritBudgetExhausted},
		},
	}
	contract := types.AnswerContract{
		CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: TmplCitationCountLow},
		AcceptanceTests: []types.Criterion{
			{Kind: types.CritCitationCountGE, Expr: strconv.Itoa(TmplCitationCountLow)},
		},
		Language: rm.Language,
	}
	return Output{TaskGraph: graph, EvidencePlan: plan, AnswerContract: contract}
}
