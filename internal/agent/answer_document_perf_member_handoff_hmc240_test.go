package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfMemberPublicationHMC240SeparateFromPrefixTree(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"window_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !r.Success {
		t.Fatalf("query: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	ledger := answerDocObservationLedger(ctx)
	var publication answerDocNativePresentationReceipt
	renderAnswerDocPerfSamplingTrees(ctx, ledger, &publication)
	if publication.completeFor(ctx) {
		t.Fatal("prefix tree cannot publish flat weights including opaque stacks")
	}
	text := renderAnswerDocPerfMemberFacts(ledger, &publication)
	if publication.completeFor(ctx) {
		t.Fatal("tree and flat fields cannot publish the new fixed fact-table family")
	}
	renderAnswerDocRuntimeMeasurementChoices(ctx, &publication)
	if !publication.completeFor(ctx) {
		t.Fatal("independently published full flat fields, tree and fact tables must cover this sampling-only query")
	}
	for _, row := range ledger.Records {
		if row.Predicate != "perf_sample_top_symbol" {
			continue
		}
		for _, exact := range append([]string{row.Subject, row.Object, row.Value}, row.RichNotes...) {
			encoded, _ := json.Marshal(exact)
			if !strings.Contains(text, string(encoded)) {
				t.Fatalf("native flat field lost or rederived: %s", exact)
			}
		}
	}
	for _, name := range []string{"member_budget", "byte_budget", "foreign_family"} {
		t.Run(name, func(t *testing.T) {
			copyLedger := ledger
			copyLedger.Records = append([]types.ObservationRecord(nil), ledger.Records...)
			for _, row := range ledger.Records {
				if row.Predicate != "perf_sample_top_symbol" {
					continue
				}
				switch name {
				case "byte_budget":
					row.RichNotes = append(append([]string(nil), row.RichNotes...), strings.Repeat("x", 40<<10))
					copyLedger.Records = append(copyLedger.Records, row)
				case "member_budget":
					for n := 0; n < 33; n++ {
						row.Span.LineStart = 1000 + n
						copyLedger.Records = append(copyLedger.Records, row)
					}
				case "foreign_family":
					row.Predicate = "file_io_by_inode"
					copyLedger.Records = []types.ObservationRecord{row}
				}
				break
			}
			var partial answerDocNativePresentationReceipt
			renderAnswerDocPerfSamplingTrees(ctx, ledger, &partial)
			renderAnswerDocPerfMemberFacts(copyLedger, &partial)
			renderAnswerDocRuntimeMeasurementChoices(ctx, &partial)
			if partial.completeFor(ctx) {
				t.Fatal("omitted or unrelated flat member publication declared complete")
			}
		})
	}
}
