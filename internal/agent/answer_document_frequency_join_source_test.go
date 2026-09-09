package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// JSON construction keeps the negative test executable against the pre-receipt
// baseline: that baseline ignores source_ref, and incorrectly joins the rows.
func b1631JoinResult(t *testing.T, path, payload, query, running string, maximum int64) types.ToolResult {
	t.Helper()
	var ref types.ObservationSourceRef
	raw, _ := json.Marshal(map[string]any{"kind": types.ObservationSourceRuntimeArtifact,
		"path": path, "payload_ref": payload, "raw_ref": payload, "query_scope_id": query})
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	var witness types.TraceFrequencyLimitAuthority
	raw, _ = json.Marshal(map[string]any{"cpu": 4, "min_frequency_khz": 500000,
		"max_frequency_khz": maximum, "limit_row_count": 1, "witness_line": 3,
		"witness_ts": 10.001, "window_start_ts": 10, "window_end_ts": 10.006,
		"authority": "direct_in_window_policy_limit", "source_ref": ref, "observed_at": "2026-09-08T20:00:00Z"})
	if err := json.Unmarshal(raw, &witness); err != nil {
		t.Fatal(err)
	}
	return types.ToolResult{ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{View: "window_stats", FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{witness}},
		Observations: []types.ObservationRecord{{ID: "target:" + query + ":" + running,
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", SourceRef: ref, ObservedAt: "2026-09-08T20:00:00Z",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "target_cpu_running", Subject: "target-41", Object: "cpu=4", Value: running, Unit: "ms",
			RichNotes: []string{"selected_window=10.000000..10.006000", "target_cpu_running_cpu=4", "target_cpu_running_roster_status=complete"}}}}
}

func b1631JoinPrompt(t *testing.T, lang string, results ...types.ToolResult) (*types.AgentContext, string) {
	t.Helper()
	ctx := boundedRuntimeReaderHandoffTestContext()
	ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx, (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
}

func b1631JoinRows(prompt string) []string {
	var rows []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "  | ") && strings.Contains(line, "`target-41`") {
			rows = append(rows, line)
		}
	}
	return rows
}

func TestB1631FrequencyQueryCohortsSurviveBothPromptReaders(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				a := b1631JoinResult(t, "/captures/same.ftrace", "result.json", "child-a", "1.000", 2100000)
				b := b1631JoinResult(t, "/captures/same.ftrace", "result.json", "child-b", "2.000", 1800000)
				results := []types.ToolResult{a, b}
				if reverse {
					results[0], results[1] = results[1], results[0]
				}
				before, _ := json.Marshal(results)
				ctx, prompt := b1631JoinPrompt(t, lang, results...)
				rows := b1631JoinRows(prompt)
				if len(rows) != 2 {
					t.Fatalf("separate result children must retain both CPU observations: %q", rows)
				}
				for _, pair := range [][2]string{{"1.000ms", "max=2100000kHz"}, {"2.000ms", "max=1800000kHz"}} {
					found := false
					for _, row := range rows {
						if strings.Contains(row, pair[0]) && strings.Contains(row, pair[1]) && strings.Contains(row, "/captures/same.ftrace") {
							found = true
						}
					}
					if !found {
						t.Errorf("lost source-local pair %v in %q", pair, rows)
					}
				}
				readerMarker := "- Per-CPU target running and frequency comparison"
				if lang == "zh" {
					readerMarker = "- 目标线程的逐 CPU 运行与频率对照"
				}
				_, reader, ok := strings.Cut(prompt, readerMarker)
				if !ok {
					t.Fatal("actual reader missing")
				}
				for _, pair := range [][2]string{{"1.000ms", "500000–2100000 kHz"}, {"2.000ms", "500000–1800000 kHz"}} {
					found := false
					for _, line := range strings.Split(reader, "\n") {
						if strings.HasPrefix(line, "  - ") && strings.Contains(line, pair[0]) && strings.Contains(line, pair[1]) && strings.Contains(line, "/captures/same.ftrace") {
							found = true
						}
					}
					if !found {
						t.Errorf("reader lost own pair %v", pair)
					}
				}
				after, _ := json.Marshal(results)
				if string(before) != string(after) || prompt != (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil) {
					t.Fatal("display mutated original inputs or is not idempotent")
				}
			})
		}
	}
}

func TestB1631FrequencyConflictingPoliciesDoNotSelectFirst(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	b := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 1800000)
	for _, reverse := range []bool{false, true} {
		results := []types.ToolResult{a, b}
		if reverse {
			results[0], results[1] = results[1], results[0]
		}
		_, prompt := b1631JoinPrompt(t, "en", results...)
		rows := b1631JoinRows(prompt)
		if len(rows) != 1 || !strings.Contains(rows[0], "not_comparable_ambiguous") || strings.Contains(rows[0], "present:min=") {
			t.Fatalf("conflicting same-source policies cannot choose a first winner: %q", rows)
		}
		for _, value := range []string{"max=2100000kHz", "max=1800000kHz", "1.000ms"} {
			if !strings.Contains(prompt, value) {
				t.Errorf("original independent measurement %q disappeared", value)
			}
		}
	}
}

func TestB1631FrequencyUnknownSourcesRemainVisibleButCannotJoin(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "", "", "1.000", 2100000)
	_, prompt := b1631JoinPrompt(t, "en", a)
	for _, row := range b1631JoinRows(prompt) {
		if strings.Contains(row, "target_effect_unproven_no_slice_binding") {
			t.Fatalf("missing receipt must not mint a target/policy pair: %s", row)
		}
	}
	for _, want := range []string{"1.000ms", "max=2100000kHz", "/captures/a.ftrace"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("unknown original fact %q disappeared", want)
		}
	}
}

func TestB1631FrequencyConflictingTargetRowsDoNotSelectLast(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	b := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "2.000", 2100000)
	for _, reverse := range []bool{false, true} {
		results := []types.ToolResult{a, b}
		if reverse {
			results[0], results[1] = results[1], results[0]
		}
		ctx, prompt := b1631JoinPrompt(t, "en", results...)
		rows := b1631JoinRows(prompt)
		_ = ctx
		if len(rows) != 2 {
			t.Fatalf("conflicting source-local targets must remain two independent values: %q", rows)
		}
		for _, row := range rows {
			if !strings.Contains(row, "not_comparable_ambiguous_source_rows") || strings.Contains(row, "present:min=") {
				t.Fatalf("conflicting target selected a last winner: %s", row)
			}
		}
		if !strings.Contains(strings.Join(rows, "\n"), "1.000ms") || !strings.Contains(strings.Join(rows, "\n"), "2.000ms") {
			t.Fatal("conflict disclosure lost an original target value")
		}
	}
}

func TestB1631FrequencyRosterAndBucketCannotBorrowAnotherResult(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	a.TraceEvidenceAuthority.FrequencyLimitWitnesses[0].CPU = 0
	a.Observations[0].RichNotes[2] = "target_cpu_running_roster_status=truncated"
	b := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-b", "2.000", 1800000)
	freq := b.Observations[0]
	freq.ID, freq.Predicate = "frequency-b", "running_time"
	freq.RichNotes = []string{"selected_window=10.000000..10.006000", "cpu=4", "freq=9999999"}
	b.Observations = append(b.Observations, freq)
	for _, reverse := range []bool{false, true} {
		results := []types.ToolResult{a, b}
		if reverse {
			results[0], results[1] = results[1], results[0]
		}
		_, prompt := b1631JoinPrompt(t, "en", results...)
		var policyOnly, ownTarget bool
		for _, row := range b1631JoinRows(prompt) {
			if strings.Contains(row, "max=2100000kHz") {
				policyOnly = true
				if strings.Contains(row, "absent_in_complete_roster") || !strings.Contains(row, "not_observed_in_emitted_roster") {
					t.Fatalf("another query's complete roster borrowed into CPU 0: %s", row)
				}
			}
			if strings.Contains(row, "1.000ms") {
				ownTarget = true
				if strings.Contains(row, "9999999") {
					t.Fatalf("another result's bucket borrowed into the target: %s", row)
				}
			}
		}
		if !policyOnly || !ownTarget {
			t.Fatalf("same-source target or independent CPU-policy face disappeared")
		}
	}
}

func TestB1631FrequencyDisplayCapCannotProvePolicyUniqueness(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	first := a.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	for cpu := 10; cpu < 17; cpu++ {
		w := first
		w.CPU = cpu
		a.TraceEvidenceAuthority.FrequencyLimitWitnesses = append(a.TraceEvidenceAuthority.FrequencyLimitWitnesses, w)
	}
	conflict := first
	conflict.MaxFrequencyKHz = 1800000
	a.TraceEvidenceAuthority.FrequencyLimitWitnesses = append(a.TraceEvidenceAuthority.FrequencyLimitWitnesses, conflict)
	ctx, prompt := b1631JoinPrompt(t, "en", a)
	if got := len(answerDocRuntimeTraceGuidanceView(ctx).FrequencyLimitWitnesses); got != 8 {
		t.Fatalf("old policy display budget changed: %d", got)
	}
	for _, row := range b1631JoinRows(prompt) {
		if strings.Contains(row, "1.000ms") && (!strings.Contains(row, "not_comparable_ambiguous") || strings.Contains(row, "present:min=")) {
			t.Fatalf("ninth conflicting witness was hidden by preview before uniqueness check: %s", row)
		}
	}
}

func TestB1631FrequencyLegacyUnknownDedupRemainsIndependent(t *testing.T) {
	w := types.TraceFrequencyLimitAuthority{CPU: 4, MaxFrequencyKHz: 2100000, LimitRowCount: 1, WindowStartTs: 10, WindowEndTs: 10.006}
	if got := answerDocDedupFrequencyLimitWitnesses([]types.TraceFrequencyLimitAuthority{w, w}, 8); len(got) != 2 {
		t.Fatalf("equal legacy numbers do not prove a duplicate source: %+v", got)
	}
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	w = a.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	if got := answerDocDedupFrequencyLimitWitnesses([]types.TraceFrequencyLimitAuthority{w, w}, 8); len(got) != 1 {
		t.Fatalf("complete identical receipts must still deduplicate: %+v", got)
	}
}

func TestB1631FrequencyCanonicalCaptureEnrichmentRequiresUniqueCapture(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("conflict=%t/reverse=%t", conflict, reverse), func(t *testing.T) {
				a := b1631JoinResult(t, "/materialized/trace.ftrace", "result.json", "child-a", "1.000", 2100000)
				a.Observations[0].SourceRef.CaptureIdentityPath = "/captures/a/trace.ftrace"
				results := []types.ToolResult{a}
				if conflict {
					b := b1631JoinResult(t, "/materialized/trace.ftrace", "result.json", "child-a", "1.000", 2100000)
					b.Observations[0].ID = "target-other-capture"
					b.Observations[0].SourceRef.CaptureIdentityPath = "/captures/b/trace.ftrace"
					results = append(results, b)
					if reverse {
						results[0], results[1] = results[1], results[0]
					}
				}
				_, prompt := b1631JoinPrompt(t, "en", results...)
				rows := b1631JoinRows(prompt)
				if len(rows) != len(results) {
					t.Fatalf("independent capture observations disappeared: %q", rows)
				}
				for _, row := range rows {
					if conflict {
						if !strings.Contains(row, "not_comparable_ambiguous") || strings.Contains(row, "present:min=") {
							t.Fatalf("unbound policy borrowed one of two proven captures: %s", row)
						}
					} else if !strings.Contains(row, "present:min=500000kHz,max=2100000kHz") {
						t.Fatalf("single compatible preflight enrichment must remain valid: %s", row)
					}
				}
			})
		}
	}
}

func TestB1631FrequencySuffixedDeterministicProducerRetainsJoin(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	a.Observations[0].Producer = "trace_query:run2"
	_, prompt := b1631JoinPrompt(t, "en", a)
	rows := b1631JoinRows(prompt)
	if len(rows) != 1 || !strings.Contains(rows[0], "present:min=500000kHz,max=2100000kHz") {
		t.Fatalf("existing deterministic producer classifier was narrowed: %q", rows)
	}
}

func TestB1631FrequencyDifferentReceiptMetadataIsNotSilentlyCollapsed(t *testing.T) {
	a := b1631JoinResult(t, "/captures/a.ftrace", "result.json", "child-a", "1.000", 2100000)
	w := types.CloneTraceFrequencyLimitAuthority(a.TraceEvidenceAuthority.FrequencyLimitWitnesses[0])
	w.SourceRef.ClockAlignment = "separately_reported"
	a.TraceEvidenceAuthority.FrequencyLimitWitnesses = append(a.TraceEvidenceAuthority.FrequencyLimitWitnesses, w)
	ctx, prompt := b1631JoinPrompt(t, "en", a)
	if len(answerDocRuntimeTraceGuidanceView(ctx).FrequencyLimitWitnesses) != 2 {
		t.Fatal("different typed receipts were collapsed by equal policy numbers")
	}
	for _, row := range b1631JoinRows(prompt) {
		if !strings.Contains(row, "not_comparable_ambiguous") {
			t.Fatalf("receipt metadata differences are conservatively independent, not a selected merged policy: %s", row)
		}
	}
}
