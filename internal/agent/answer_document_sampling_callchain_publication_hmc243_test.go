package agent

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC243RecordedCallchainPublicationIsIndependent(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"perf_stats","pid":11,"time_start":1,"time_end":2}`))
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %s", err, r.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	ledger := answerDocObservationLedger(ctx)
	var receipt answerDocNativePresentationReceipt
	renderAnswerDocPerfSamplingTrees(ctx, ledger, &receipt)
	renderAnswerDocRuntimeMeasurementChoices(ctx, &receipt)
	if receipt.completeFor(ctx) {
		t.Fatal("tree and fixed facts cannot certify recorded callchain labels")
	}
	text := renderAnswerDocPerfMemberFacts(ledger, &receipt)
	if count := samplingSourceDirectoryIntegrityHMC243(t, text, ledger); count != 1 {
		t.Fatalf("one exact source should be shared, got %d", count)
	}
	found, opaque, exact := 0, false, false
	for _, row := range ledger.Records {
		if row.Predicate != "perf_sample_top_callchain" {
			continue
		}
		found++
		opaque = opaque || strings.Contains(row.Summary, "callchain_form=opaque_flat_label")
		exact = exact || strings.Contains(row.Summary, "callchain_form=exact_frames")
		for _, original := range []string{row.Subject, row.Object, row.Summary} {
			encoded, _ := json.Marshal(original)
			if !strings.Contains(text, string(encoded)) {
				t.Errorf("lost complete native recorded callchain field %s", encoded)
			}
		}
		if !receipt.queries[nativePresentationKey(row)] {
			t.Error("actually published callchain family lacks independent receipt")
		}
	}
	if found == 0 || !opaque || !exact || !receipt.completeFor(ctx) {
		for _, row := range ledger.Records {
			if row.Predicate == "perf_sample_top_symbol" || row.Predicate == "perf_sample_top_callchain" {
				ref, _ := json.Marshal(row.SourceRef)
				notes, _ := json.Marshal(row.RichNotes)
				t.Logf("native row %s source_bytes=%d summary_bytes=%d notes_bytes=%d complete=%t", row.Predicate, len(ref), len(row.Summary), len(notes), receipt.queries[nativePresentationKey(row)])
			}
		}
		t.Logf("actually displayed bytes=%d", len(text))
		t.Fatalf("recorded callchains=%d exact=%t opaque=%t complete=%t", found, exact, opaque, receipt.completeFor(ctx))
	}
	for _, mode := range []string{"no_selected_query", "different_source", "byte_budget", "member_budget", "source_budget"} {
		t.Run(mode, func(t *testing.T) {
			copyLedger := ledger
			copyLedger.Records = append([]types.ObservationRecord(nil), ledger.Records...)
			for _, row := range ledger.Records {
				if row.Predicate != "perf_sample_top_callchain" {
					continue
				}
				switch mode {
				case "different_source":
					row.SourceRef.Path += ".other"
					copyLedger.Records = []types.ObservationRecord{row}
				case "byte_budget":
					row.Summary += strings.Repeat("x", 40<<10)
					copyLedger.Records = append(copyLedger.Records, row)
				case "source_budget":
					// Tool-call decoration is not a new query; its full source
					// still needs publication rather than silent omission.
					row.SourceRef.ToolCallID = strings.Repeat("x", 40<<10)
					copyLedger.Records = append(copyLedger.Records, row)
				case "member_budget":
					for n := 0; n < 33; n++ {
						row.Span.LineStart = n + 1000
						copyLedger.Records = append(copyLedger.Records, row)
					}
				}
				break
			}
			var partial answerDocNativePresentationReceipt
			if mode != "no_selected_query" {
				renderAnswerDocPerfSamplingTrees(ctx, ledger, &partial)
			}
			renderAnswerDocRuntimeMeasurementChoices(ctx, &partial)
			output := renderAnswerDocPerfMemberFacts(copyLedger, &partial)
			samplingSourceDirectoryIntegrityHMC243(t, output, copyLedger)
			if strings.Contains(output, strings.Repeat("x", 40<<10)) {
				t.Fatal("oversized member/source escaped the unchanged byte budget")
			}
			if partial.completeFor(ctx) || (mode == "no_selected_query" || mode == "different_source") && output != "" {
				t.Fatal("unselected or incompletely displayed callchains acquired publication")
			}
		})
	}
}

// Check the actually written local directory, not the renderer's internal map.
func samplingSourceDirectoryIntegrityHMC243(t *testing.T, text string, ledger types.ObservationLedger) int {
	t.Helper()
	allowed := map[string]bool{}
	for _, row := range ledger.Records {
		data, _ := json.Marshal(row.SourceRef)
		allowed[string(data)] = true
	}
	sources := map[string]string{}
	used := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if sourceLine, found := strings.CutPrefix(line, "native_source_ref="); found {
			encoded, source, ok := strings.Cut(sourceLine, " source=")
			id, err := strconv.Unquote(encoded)
			if !ok || err != nil || id == "" || sources[id] != "" || !allowed[source] {
				t.Fatalf("invalid or changed source directory: %s", line)
			}
			sources[id] = source
		} else if strings.HasPrefix(line, "{") {
			var row struct {
				Source string `json:"source_ref"`
			}
			if err := json.Unmarshal([]byte(line), &row); err != nil || sources[row.Source] == "" {
				t.Fatalf("member refers to absent source: %s", line)
			}
			used[row.Source] = true
		}
	}
	if len(sources) != len(used) {
		t.Fatal("source directory was written without a budget-admitted member")
	}
	return len(sources)
}

func TestHMC243RecordedSamplingSourcesAreExactAndLocal(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	var results []types.ToolResult
	for _, start := range []float64{1, 1.01} {
		input, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "perf_stats", "pid": 11, "time_start": start, "time_end": 2})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), input)
		if err != nil || !r.Success {
			t.Fatalf("public query: %v %s", err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	var receipt answerDocNativePresentationReceipt
	renderAnswerDocPerfSamplingTrees(ctx, ledger, &receipt)
	text := renderAnswerDocPerfMemberFacts(ledger, &receipt)
	if count := samplingSourceDirectoryIntegrityHMC243(t, text, ledger); count != 2 {
		t.Fatalf("different query windows cannot share a source directory entry, got %d", count)
	}
	if len(text) > 32<<10 || receipt.completeFor(ctx) {
		t.Fatal("source reuse enlarged byte budget or published unseen fact tables")
	}
}
