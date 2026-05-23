package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/amplifier"
	"github.com/hanchaoqun/codrax/internal/analysis/binder"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/counterfactual"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/types"
)

// rebuildAnalysisIRCompiledArtifactsAfterAutoCorrect keeps the analyzer IR
// internally consistent after an accepted structural auto-correction mutates
// RequestModel. The analyzer compiles TaskGraph/EvidencePlan/AnswerContract
// before the quality gate runs; if the orchestrator later corrects
// RequestModel and only re-runs the gate, downstream scheduling can still use
// stale evidence lanes from the pre-correction graph.
//
// This helper reuses the same deterministic compiler pieces as buildAnalysisIR
// and deliberately avoids any model/user prose inspection.
func rebuildAnalysisIRCompiledArtifactsAfterAutoCorrect(ir *types.AnalysisIR) error {
	if ir == nil {
		return fmt.Errorf("nil analysis IR")
	}
	rm := ir.RequestModel
	if rm.Scenario == "" {
		rm.Scenario = compiler.InferScenario(rm)
	}
	oldContract := ir.AnswerContract
	oldRequiredFiles := append([]string(nil), ir.EvidencePlan.RequiredFiles...)

	sig := budget.BudgetSignals{
		Complexity:      rm.Complexity,
		TermCount:       len(rm.TermGraph.Canonical),
		HypothesisCount: 1,
	}
	out := compiler.Compile(rm, sig)
	if out.AnswerContract.Language == "" {
		out.AnswerContract.Language = rm.Language
	}
	if out.AnswerContract.Language == "" {
		out.AnswerContract.Language = oldContract.Language
	}

	hypotheses := hdp.Plan(rm)
	sig.HypothesisCount = len(hypotheses)
	compiler.RecomputeBudget(&out, rm, sig)

	amplifier.AmplifyPostCompile(rm, &out.AnswerContract)

	if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {
		return fmt.Errorf("binder: %w", err)
	}
	if counterfactual.ShouldExpand(rm) {
		expanded, newIDs := counterfactual.Expand(
			out.TaskGraph, rm,
			counterfactual.Options{Enabled: true, MaxBranches: 1},
		)
		out.TaskGraph = expanded
		if len(newIDs) > 0 {
			if err := binder.BindByRelevance(&out.TaskGraph, hypotheses, binder.Options{}); err != nil {
				return fmt.Errorf("binder (counterfactual): %w", err)
			}
		}
	}

	preserveAnalyzerOnlyPostCompileContract(&out, oldContract)
	out.AnswerContract.ExactResolution = types.BuildExactResolutionContract(rm)
	out.EvidencePlan.RequiredFiles = mergeRequiredFileHints(out.EvidencePlan.RequiredFiles, oldRequiredFiles)

	ir.RequestModel = rm
	ir.TaskGraph = out.TaskGraph
	ir.EvidencePlan = out.EvidencePlan
	ir.AnswerContract = out.AnswerContract
	ir.HypothesisSet = hypotheses
	ir.QualityGate = types.GateReport{}
	return nil
}

func preserveAnalyzerOnlyPostCompileContract(out *compiler.Output, old types.AnswerContract) {
	if out == nil {
		return
	}
	if old.Diagram != nil {
		diagram := *old.Diagram
		out.AnswerContract.Diagram = &diagram
	}
	if old.CurrentStatusDiagnostic != nil && out.AnswerContract.CurrentStatusDiagnostic == nil {
		current := *old.CurrentStatusDiagnostic
		out.AnswerContract.CurrentStatusDiagnostic = &current
	}
	if old.CitationReq.Required || old.CitationReq.MinCitations != 0 || old.CitationReq.Granularity == "" {
		return
	}
	out.AnswerContract.CitationReq = old.CitationReq
	out.AnswerContract.AcceptanceTests = dropCitationCountGEContract(out.AnswerContract.AcceptanceTests)
	for i := range out.TaskGraph.Nodes {
		out.TaskGraph.Nodes[i].SuccessCriteria = dropCitationCountGEContract(out.TaskGraph.Nodes[i].SuccessCriteria)
	}
}

func mergeRequiredFileHints(first, second []string) []string {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	out := make([]string, 0, len(first)+len(second))
	seen := make(map[string]bool, len(first)+len(second))
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, v := range first {
		add(v)
	}
	for _, v := range second {
		add(v)
	}
	return out
}
