package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingFactPublicationContextHMC243(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	ctx := samplingSelectionAnalysisContext(t)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"window_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %s", err, r.Summary)
	}
	return ctx, r
}

func samplingFactRecordsHMC243(ctx *types.AgentContext) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, r := range answerDocObservationLedger(ctx).Records {
		if r.Predicate == types.RuntimeSamplingMeasurementPredicate {
			out = append(out, r)
		}
	}
	return out
}

func TestHMC243SamplingFactPublicationRawCompiledBridge(t *testing.T) {
	for _, mode := range []string{"raw", "preflight", "supplement", "json_copy", "source", "query", "clock", "conflicting_raw_id", "different_compiled_record"} {
		t.Run(mode, func(t *testing.T) {
			ctx, r := samplingFactPublicationContextHMC243(t)
			want := mode == "raw" || mode == "preflight" || mode == "supplement"
			var path string
			for i := range r.Observations {
				row := &r.Observations[i]
				if row.Predicate != types.RuntimeSamplingMeasurementPredicate {
					continue
				}
				path = row.SourceRef.Path
				switch mode {
				case "json_copy":
					data, _ := json.Marshal(row)
					var copied types.ObservationRecord
					if err := json.Unmarshal(data, &copied); err != nil {
						t.Fatal(err)
					}
					*row = copied
				case "source":
					row.SourceRef.Path += ".unrelated"
				case "query":
					row.SourceRef.QueryScopeID += "-changed"
				case "clock":
					offset := 9.0
					row.SourceRef.ClockOffsetSec = &offset
				}
			}
			results := []types.ToolResult{{ToolName: "unrelated", Success: true}, r}
			if mode == "conflicting_raw_id" {
				conflict := r
				conflict.Observations = append([]types.ObservationRecord(nil), r.Observations...)
				for i := range conflict.Observations {
					if conflict.Observations[i].Predicate == types.RuntimeSamplingMeasurementPredicate {
						conflict.Observations[i].SourceRef.Path += ".conflict"
					}
				}
				results = append(results, conflict)
			}
			if mode == "preflight" || mode == "supplement" {
				ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}}
			}
			if mode == "supplement" {
				ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{}, results)
			} else {
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			}
			before, _ := json.Marshal(answerDocObservationLedger(ctx))
			bridge := samplingFactPublicationBridge(ctx)
			rows := samplingFactRecordsHMC243(ctx)
			if len(rows) < 2 {
				t.Fatalf("expected same-query independent groups, got %d", len(rows))
			}
			var receipt answerDocNativePresentationReceipt
			renderAnswerDocRuntimeMeasurementChoices(ctx, &receipt)
			for _, row := range rows {
				if mode == "different_compiled_record" {
					row.SourceRef.CaptureIdentityPath += ".changed"
				}
				_, found := bridge[traceEventPublicationRecordKey(row)]
				if found != want || receipt.queries[nativePresentationKey(row)] != want {
					t.Fatalf("bridge=%t receipt=%t want=%t for %s", found, receipt.queries[nativePresentationKey(row)], want, row.ID)
				}
				if (mode == "preflight" || mode == "supplement") && row.SourceRef.CaptureIdentityPath == "" {
					t.Fatal("fixture missed actual preflight enrichment")
				}
			}
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			if string(before) != string(after) {
				t.Fatal("presentation bridge changed accepted ledger")
			}
		})
	}
}

func TestHMC243SamplingFactPublicationRequiresEveryViewAndGroup(t *testing.T) {
	for _, mode := range []string{"complete", "missing_all", "one_group", "summary_only", "partial_quality", "missing_note", "changed_cell"} {
		t.Run(mode, func(t *testing.T) {
			ctx, r := samplingFactPublicationContextHMC243(t)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			var shown samplingFactRosterPublication
			first := ""
			for _, table := range view.RuntimeMeasurementContract.Choices() {
				if !table.HasNativeSamplingFacts() {
					continue
				}
				if first == "" {
					first = table.ObservationID
				}
				if mode == "missing_all" || mode == "one_group" && table.ObservationID != first || mode == "summary_only" && table.View != types.RuntimeMeasurementSummary {
					continue
				}
				rows := len(table.Rows)
				if mode == "partial_quality" && table.View == types.RuntimeMeasurementDistribution {
					rows--
				}
				if mode == "changed_cell" {
					table.Rows = append([][]string(nil), table.Rows...)
					table.Rows[0] = append([]string(nil), table.Rows[0]...)
					table.Rows[0][0] = "changed"
				}
				shown.table(table, rows)
				for i, note := range table.Notes {
					if mode != "missing_note" || i != 0 {
						shown.note(table, note)
					}
				}
			}
			var receipt answerDocNativePresentationReceipt
			shown.publish(ctx, &receipt)
			rows := samplingFactRecordsHMC243(ctx)
			if len(rows) < 2 {
				t.Fatal("fixture lacks same-query siblings")
			}
			for _, row := range rows {
				want := mode == "complete" || mode == "one_group" && row.ID == first
				if receipt.queries[nativePresentationKey(row)] != want {
					t.Errorf("%s complete=%t want=%t", row.ID, receipt.queries[nativePresentationKey(row)], want)
				}
			}
			if receipt.completeFor(ctx) {
				t.Fatal("fact-table family cannot publish independent tree/flat observations")
			}
		})
	}
}

func TestHMC243SamplingFactPublicationActualRosterBudgets(t *testing.T) {
	for _, queries := range []int{8, 18} {
		t.Run(map[int]string{8: "quality_row_budget", 18: "group_budget"}[queries], func(t *testing.T) {
			ctx, first := samplingFactPublicationContextHMC243(t)
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			results := []types.ToolResult{first}
			for i := 1; i < queries; i++ {
				params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "window_stats", "pid": 11, "time_start": 1 + float64(i)/1000, "time_end": 2})
				r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
				if err != nil || !r.Success {
					t.Fatalf("public query %d: %v %s", i, err, r.Summary)
				}
				results = append(results, r)
			}
			const synthesis = "CURRENT_SYNTHESIS_RETAIN_WHEN_FACT_TABLES_PARTIAL"
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, InvestigationNotes: []string{synthesis, "independent note"}})
			ctx.Mutable.SetInvestigationComplete(synthesis)
			ctx.Mutable.SetInvestigationResultKind("resolved")
			var receipt answerDocNativePresentationReceipt
			text := renderAnswerDocRuntimeMeasurementChoices(ctx, &receipt)
			choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
			shown, omitted := runtimeMeasurementHandoffRoster(choices, 32)
			counts := runtimeMeasurementHandoffPreviewRows(shown)
			partialQuality := false
			for i, table := range shown {
				if table.HasNativeSamplingFacts() && table.View == types.RuntimeMeasurementDistribution && counts[i] < len(table.Rows) {
					partialQuality = true
				}
			}
			if !partialQuality || queries == 18 && omitted == 0 || !strings.Contains(text, "preview rows=128/128") {
				t.Fatalf("fixture missed actual budgets: partial quality=%t omitted=%d", partialQuality, omitted)
			}
			incomplete := 0
			for _, row := range samplingFactRecordsHMC243(ctx) {
				if !receipt.queries[nativePresentationKey(row)] {
					incomplete++
				}
			}
			if incomplete == 0 || receipt.completeFor(ctx) {
				t.Fatal("actual preview omission signed complete fact families")
			}
			if prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil); !strings.Contains(prompt, synthesis) {
				t.Fatal("actual finalizer removed synthesis despite partial fact tables")
			}
		})
	}
}
