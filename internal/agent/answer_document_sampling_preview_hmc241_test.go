package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingReferencePreviewDoesNotHideExactHotspotsHMC241(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	line := strings.Split(ctx.AttachedHitrace, "\n")[1]
	ctx.AttachedHitrace = strings.Replace(ctx.AttachedHitrace, line+"\n", strings.Repeat(line+"\n", 7), 1)
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"window_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !result.Success {
		t.Fatalf("public sampling query: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	omitted := false
	for _, r := range ledger.Records {
		if p, valid := tool.DecodeTracePerfSamplingTree(r); valid {
			for _, c := range p.Cohorts {
				for _, n := range c.Nodes {
					omitted = omitted || n.OmittedReferences > 0
				}
			}
		}
	}
	if !omitted {
		t.Fatal("fixture must have a bounded reference preview, not a fully expanded tree")
	}
	var publication answerDocNativePresentationReceipt
	renderAnswerDocPerfSamplingTrees(ctx, ledger, &publication)
	text := renderAnswerDocPerfMemberFacts(ledger, &publication)
	for _, exact := range []string{`"samples=8"`, `"sample_weight=75"`, `"event=cycles"`} {
		if !strings.Contains(text, exact) {
			t.Errorf("exact native flat member was lost behind reference preview: %s", exact)
		}
	}
	if publication.completeFor(ctx) {
		t.Fatal("publishing hotspot fields must not declare omitted tree references complete")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "### Recorded sampling hotspot fields") || !strings.Contains(prompt, `"samples=8"`) {
		t.Fatal("actual bounded named-target finalizer lost native hotspot fields")
	}
	for _, name := range []string{"different_query", "different_window", "different_source", "no_tree"} {
		t.Run(name, func(t *testing.T) {
			var changed types.ObservationLedger
			for _, r := range ledger.Records {
				if r.Predicate != "perf_sample_top_symbol" {
					continue
				}
				switch name {
				case "different_query":
					r.SourceRef.QueryScopeID += ":other"
				case "different_window":
					r.SourceRef.QueryWindowStartTs += .1
				case "different_source":
					r.SourceRef.Path += ".other"
				}
				changed.Records = append(changed.Records, r)
			}
			var denied answerDocNativePresentationReceipt
			if name != "no_tree" {
				renderAnswerDocPerfSamplingTrees(ctx, ledger, &denied)
			}
			if got := renderAnswerDocPerfMemberFacts(changed, &denied); got != "" {
				t.Fatal("unqualified source/query/window borrowed a sampling selection")
			}
		})
	}
}
