package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1631WriteFrequencyCapture(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func b1631FrequencySlice(max int, start, end float64) string {
	return fmt.Sprintf(`policy-10 (10) [004] .... %.6f: cpu_frequency_limits: min=500000 max=%d cpu_id=4
idle-0 (0) [004] .... %.6f: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [004] .... %.6f: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
policy-10 (10) [004] .... %.6f: cpu_frequency: state=1000000 cpu_id=4
`, start-0.0001, max, start, end, end+0.0001)
}

func b1631RunFrequencyQuery(t *testing.T, dir, path string, first, last int) types.ToolResult {
	t.Helper()
	params := map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 41,
		"time_start": 10.0, "time_end": 10.006}
	if first > 0 {
		params["line_start"], params["line_end"] = first, last
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, raw)
	if err != nil || !result.Success {
		t.Fatalf("actual trace_query failed: %v; %s", err, result.Summary)
	}
	return result
}

func b1631AssertFrequencyQuery(t *testing.T, result types.ToolResult, maximum int64, running string) {
	t.Helper()
	if result.TraceEvidenceAuthority == nil || len(result.TraceEvidenceAuthority.FrequencyLimitWitnesses) != 1 {
		t.Fatalf("fixture must publish one positive policy witness: %+v", result.TraceEvidenceAuthority)
	}
	w := result.TraceEvidenceAuthority.FrequencyLimitWitnesses[0]
	if w.CPU != 4 || w.MaxFrequencyKHz != maximum || w.MinFrequencyKHz != 500000 || w.LimitRowCount != 1 || w.WitnessLine <= 0 || w.WindowStartTs != 10 || w.WindowEndTs != 10.006 {
		t.Fatalf("fixture policy premise failed: %+v", w)
	}
	var rows []types.ObservationRecord
	for _, record := range result.Observations {
		if record.Predicate == "target_cpu_running" {
			rows = append(rows, record)
		}
	}
	if len(rows) != 1 || rows[0].Subject != "target-41" || rows[0].Object != "cpu=4" || rows[0].Value != running || rows[0].Unit != "ms" || rows[0].SourceRef.Path == "" || rows[0].SourceRef.PayloadRef == "" {
		t.Fatalf("fixture must publish its own exact target/CPU/source row, want %sms: %+v", running, rows)
	}
	t.Logf("actual input path=%s payload=%s target=%s CPU=4 running=%sms max=%d count=%d witness_line=%d", rows[0].SourceRef.Path, rows[0].SourceRef.PayloadRef, rows[0].Subject, rows[0].Value, w.MaxFrequencyKHz, w.LimitRowCount, w.WitnessLine)
}

func b1631FrequencyFinalContext(t *testing.T, dir string, results []types.ToolResult) (*types.AgentContext, string) {
	t.Helper()
	start, end := 10.0, 10.006
	rm := types.RequestModel{Intent: types.IntentTrace, Language: "en",
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactFrequencyResidency}},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
	}
	mut := types.NewMutableState("Compare the supplied target CPU and policy observations in the selected query range.")
	mut.SetRequestModel(rm)
	for _, result := range results {
		mut.AppendDispatchToolResult(result)
	}
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Mutable: mut, Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	ledger := answerDocObservationLedger(ctx)
	var rows []types.ObservationRecord
	for _, record := range ledger.Records {
		if record.Predicate == "target_cpu_running" {
			rows = append(rows, record)
		}
	}
	if len(rows) != len(results) {
		t.Fatalf("dispatch/ledger premise failed: %d target rows from %d results: %+v", len(rows), len(results), rows)
	}
	return ctx, (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
}

func b1631FrequencyMatrixRows(t *testing.T, prompt string) []string {
	t.Helper()
	const marker = "Runtime target/CPU policy comparison matrix"
	_, section, found := strings.Cut(prompt, marker)
	if !found {
		t.Fatalf("real finalizer prompt has no policy comparison matrix")
	}
	var rows []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "  | ") && strings.Contains(line, "`target-41`") {
			rows = append(rows, line)
		} else if len(rows) > 0 {
			break
		}
	}
	return rows
}

func b1631AssertOwnFrequencyPairs(t *testing.T, dir string, a, b types.ToolResult) {
	t.Helper()
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			results := []types.ToolResult{a, b}
			if reverse {
				results[0], results[1] = results[1], results[0]
			}
			before, _ := json.Marshal(results)
			ctx, prompt := b1631FrequencyFinalContext(t, dir, results)
			rows := b1631FrequencyMatrixRows(t, prompt)
			t.Logf("actual finalizer matrix rows: %q", rows)
			if len(rows) != 2 {
				t.Errorf("distinct source/query cohorts must retain two own target/policy pairs, got %d:\n%s", len(rows), strings.Join(rows, "\n"))
			}
			for _, pair := range []struct{ run, max string }{{"1.000ms", "max=2100000kHz"}, {"2.000ms", "max=1800000kHz"}} {
				matches := 0
				for _, row := range rows {
					if strings.Contains(row, pair.run) && strings.Contains(row, pair.max) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("target running and policy must come from their own result exactly once: run=%s %s, matches=%d", pair.run, pair.max, matches)
				}
			}
			// The late reader-ready restatement is a second production join,
			// not evidence that the earlier matrix alone was repaired.
			_, reader, found := strings.Cut(prompt, "- Per-CPU target running and frequency comparison")
			if !found {
				t.Fatal("real finalizer prompt has no per-CPU reader-ready comparison")
			}
			var readerRows []string
			for _, line := range strings.Split(reader, "\n") {
				if strings.HasPrefix(line, "  - ") && strings.Contains(line, "target-41") {
					readerRows = append(readerRows, line)
				} else if len(readerRows) > 0 {
					break
				}
			}
			t.Logf("actual finalizer reader rows: %q", readerRows)
			if len(readerRows) != 2 {
				t.Errorf("reader restatement must retain both source/query cohorts, got %d", len(readerRows))
			}
			for _, pair := range []struct{ run, bounds string }{{"1.000ms", "500000–2100000 kHz"}, {"2.000ms", "500000–1800000 kHz"}} {
				matches := 0
				for _, row := range readerRows {
					if strings.Contains(row, pair.run) && strings.Contains(row, pair.bounds) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("reader restatement borrowed another result: run=%s bounds=%s, matches=%d", pair.run, pair.bounds, matches)
				}
			}
			after, _ := json.Marshal(results)
			if string(before) != string(after) || prompt != (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil) {
				t.Fatal("rendering changed the producer results or was not idempotent")
			}
		})
	}
}

func TestB1631ActualFrequencyCaptureIsolation(t *testing.T) {
	dir := t.TempDir()
	pathA := b1631WriteFrequencyCapture(t, dir, "a/same.ftrace", b1631FrequencySlice(2100000, 10.0002, 10.0012))
	pathB := b1631WriteFrequencyCapture(t, dir, "b/same.ftrace", b1631FrequencySlice(1800000, 10.0022, 10.0042))
	a, b := b1631RunFrequencyQuery(t, dir, pathA, 0, 0), b1631RunFrequencyQuery(t, dir, pathB, 0, 0)
	b1631AssertFrequencyQuery(t, a, 2100000, "1.000")
	b1631AssertFrequencyQuery(t, b, 1800000, "2.000")
	b1631AssertOwnFrequencyPairs(t, dir, a, b)
}

func TestB1631ActualFrequencyLineQueryIsolation(t *testing.T) {
	dir := t.TempDir()
	path := b1631WriteFrequencyCapture(t, dir, "same.ftrace", b1631FrequencySlice(2100000, 10.0002, 10.0012)+b1631FrequencySlice(1800000, 10.0022, 10.0042))
	a, b := b1631RunFrequencyQuery(t, dir, path, 1, 4), b1631RunFrequencyQuery(t, dir, path, 5, 8)
	b1631AssertFrequencyQuery(t, a, 2100000, "1.000")
	b1631AssertFrequencyQuery(t, b, 1800000, "2.000")
	b1631AssertOwnFrequencyPairs(t, dir, a, b)
}
