package context_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a real captured trace through public Execute, typed observations and
// the finalizer's actual context builder, not a synthesized occurrence tuple.
func TestTraceBoardRepresentativeWindowPublicMixedOccurrence(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 34579.450627, 34579.520000
	dir := t.TempDir()
	params, _ := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "root_cause_rank", "pid": 59566,
		"time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace",
	})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("public query: %v %s", err, result.Summary)
	}
	payloadRef := ""
	for _, record := range result.Observations {
		if ref := record.SourceRef.PayloadRef; ref != "" {
			if payloadRef != "" && ref != payloadRef {
				t.Fatal("one public query unexpectedly has multiple native payloads")
			}
			payloadRef = ref
		}
	}
	if payloadRef == "" {
		t.Fatal("public query has no native payload reference")
	}
	data, err := os.ReadFile(payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	var payload tracequery.Result
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RootCauseRank == nil {
		t.Fatal("public root-cause result absent")
	}
	var mixed *tracequery.RootCauseRankItem
	for i := range payload.RootCauseRank.Items {
		item := &payload.RootCauseRank.Items[i]
		if item.Rank <= 0 || item.Rank > 8 || item.Source != "wakeup_chain.aggregated_impacts" || len(item.OccurrenceWindows) == 0 {
			continue
		}
		first := item.OccurrenceWindows[0]
		states := 0
		for _, value := range []float64{first.RunningMs, first.RunnableMs, first.SleepMs, first.DStateMs, first.IOWaitMs} {
			if value > 0 {
				states++
			}
		}
		if states > 1 && first.FragmentCount > 1 {
			mixed = item
			break
		}
	}
	if mixed == nil {
		t.Fatalf("real producer must publish a seated first occurrence with multiple state intervals: %+v", payload.RootCauseRank.Items)
	}
	first := mixed.OccurrenceWindows[0]
	window := fmt.Sprintf("%.6f..%.6f", first.Window.StartTs, first.Window.EndTs)
	subject := fmt.Sprintf("%s-%d", mixed.Thread.Comm, mixed.Thread.PID)
	t.Logf("public witness: %s rank=%d type=%s representative=%s fragments=%d running=%.6f runnable=%.6f sleep=%.6f d_state=%.6f io_wait=%.6f; aggregate_effective=%.6f occurrences=%d",
		subject, mixed.Rank, mixed.Type, window, first.FragmentCount, first.RunningMs, first.RunnableMs, first.SleepMs, first.DStateMs, first.IOWaitMs, mixed.EffectiveImpactMs, len(mixed.OccurrenceWindows))
	matchingNote := false
	for _, record := range result.Observations {
		if record.Subject != subject || record.Object != mixed.Type {
			continue
		}
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, types.TraceNoteKeyOccurrenceWindows+"="+window+",") {
				matchingNote = true
			}
		}
	}
	if !matchingNote {
		t.Fatal("public typed note lost the first native occurrence window")
	}
	before, _ := json.Marshal(result.Observations)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			mu := types.NewMutableState("inspect the selected runtime window")
			rm := types.RequestModel{
				Language: lang, Intent: types.IntentRootCause,
				RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
				RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 59566, Thread: "com.baidu.tieba-59566", Source: "user_explicit"}},
				RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end},
			}
			mu.SetRequestModel(rm)
			mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			ac := ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}, types.AgentFinalizer, types.StageFinalize)
			prompt := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "finalize-answer"})
			board := ""
			for _, section := range prompt.UserSections {
				if section.Title == ctxbuilder.SectionTraceRootCauseBoard {
					board = section.Content
				}
			}
			row := ""
			rowPrefix := fmt.Sprintf("- #%d root-cause seat — %s · %s ·", mixed.Rank, subject, mixed.Type)
			for _, line := range strings.Split(board, "\n") {
				if strings.HasPrefix(line, rowPrefix) {
					if row != "" {
						t.Fatal("public board has ambiguous duplicate witness rows")
					}
					row = line
				}
			}
			if !strings.Contains(row, "representative_window="+window) ||
				!strings.Contains(row, fmt.Sprintf("%.3fms (effective attribution)", mixed.EffectiveImpactMs)) {
				t.Fatalf("same board row lost or changed the native ordinal/type/subject/range/aggregate value: %q\n%s", row, board)
			}
			for _, want := range []string{
				"first published occurrence record's measurement window",
				"One record may include multiple state intervals",
				"does not prove a continuous dominant-state occurrence",
				"The row's aggregate value is not this window's duration",
			} {
				if !strings.Contains(board, want) {
					t.Errorf("actual finalizer board missing %q", want)
				}
			}
			if strings.Contains(board, "ONE occurrence among several") {
				t.Error("mixed dependency record is still taught as one occurrence without a state-interval boundary")
			}
		})
	}
	after, _ := json.Marshal(result.Observations)
	if string(before) != string(after) {
		t.Fatal("context rendering changed native observations or aggregate values")
	}
}
