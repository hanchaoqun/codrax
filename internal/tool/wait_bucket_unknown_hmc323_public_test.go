package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Raw native events, not a hand-authored observation, prove that the residual
// D bucket includes both known-zero and unknown IO markers. The display must
// not turn the shared bucket into evidence that an IO mechanism is absent.
func TestHMC323NativeDMarkerKnowledgeSurvivesPublicDisplay(t *testing.T) {
	for _, tc := range []struct {
		name, marker, detail string
		d, io                int
	}{
		{"known_false", "0", "0", 1, 0},
		{"known_true", "1", "1", 0, 1},
		{"malformed", "unknown", "not recorded", 1, 0},
		{"absent", "", "not recorded", 1, 0},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				trace := traceWaitBucketNativeCycle(1, "D", 0)
				if tc.marker == "" {
					var lines []string
					for _, line := range strings.Split(trace, "\n") {
						if !strings.Contains(line, "sched_blocked_reason:") {
							lines = append(lines, line)
						}
					}
					trace = strings.Join(lines, "\n")
				} else {
					trace = strings.Replace(trace, "iowait=0", "iowait="+tc.marker, 1)
				}
				dir := t.TempDir()
				start, end := 1.0, 1.004
				rm := types.RequestModel{RawRequest: "Report reader-77 states and waits", Language: lang,
					Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
					RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Thread: "reader-77", Source: "user_explicit", Confidence: 1}},
					RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "reader-77", Confidence: 1},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.004", Confidence: 1},
					RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}, SourceQuote: "states and waits", Confidence: 1},
				}
				mu := types.NewMutableState(rm.RawRequest)
				mu.SetRequestModel(rm)
				bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu,
					AnalysisIR:      &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
					AttachedHitrace: trace, AttachedHitraceSource: "harmony_hitrace"}
				params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "thread_timeline", "pid": 77, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
				result, err := (&TraceQuery{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("native query: %v %s", err, result.Summary)
				}
				mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
				ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
				waits := types.BuildTraceTargetWaitSummaryAuthorities(ledger, &rm)
				if len(waits) != 1 || waits[0].DStateOccurrences != tc.d || waits[0].IOWaitOccurrences != tc.io || waits[0].Count != 1 || fmt.Sprintf("%.3f", waits[0].WallClockMS) != "1.000" {
					t.Fatalf("state, count or measure changed: %+v", waits)
				}
				found := false
				for _, record := range result.Observations {
					if record.Predicate == "target_sleep_interval" && record.Value == "1.000" {
						found = true
						if !strings.Contains(record.Summary, "kernel IO marker="+tc.detail+";") {
							t.Errorf("known-false/unknown provenance lost: %s", record.Summary)
						}
					}
				}
				if !found {
					t.Fatal("native sleep inventory missing")
				}
				before, _ := json.Marshal([]any{result, ledger, types.CompileTraceCausalProjectionSet(ledger)})
				const model = "Model-owned business conclusion."
				payload, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "model", "kind": "summary", "text": model}}})
				emitted, err := (&EmitAnswerDocument{}).Execute(bus, payload)
				if err != nil || !emitted.Success {
					t.Fatalf("public emit: %v %s", err, emitted.Summary)
				}
				doc := mu.AnswerDocumentV2()
				text := render.RenderAnswerDocument(doc, lang)
				label := "D-state without an IO-wait mark"
				if lang == "zh" {
					label = "D 状态（未标记为 IO 等待）"
				}
				if !strings.Contains(text, label) || !strings.Contains(text, model) {
					t.Errorf("public display must preserve model text and knowledge-neutral D label %q:\n%s", label, text)
				}
				after, _ := json.Marshal([]any{result, ledger, types.CompileTraceCausalProjectionSet(ledger)})
				if string(before) != string(after) {
					t.Fatal("display changed raw facts or causal projection")
				}
			})
		}
	}
}
