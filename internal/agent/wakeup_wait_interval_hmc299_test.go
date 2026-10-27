package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Two sleeps separated by runnable and running intervals reproduce the real
// handoff failure without asking a model to generate a particular wrong answer.
func TestHMC299WakeupMeasuredIntervalActualFinalizer(t *testing.T) {
	trace := `worker-52 (52) [001] .... 1.999000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=52 next_prio=120
target-41 (41) [000] .... 2.000000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
worker-52 (52) [001] .... 2.002978: sched_wakeup: comm=target pid=41 prio=120 target_cpu=0
idle-0 (0) [000] .... 2.002992: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... 2.003237: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
worker-52 (52) [001] .... 2.011467: sched_wakeup: comm=target pid=41 prio=120 target_cpu=0
idle-0 (0) [000] .... 2.011478: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [000] .... 2.012000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120
worker-52 (52) [001] .... 2.013000: cpu_frequency: state=1200000 cpu_id=1
`
	for _, tc := range []struct {
		name     string
		start    float64
		interval string
		duration string
	}{
		{"whole", 2, "measured_wait=[2.003237000,2.011467000)s", "8.230"},
		{"clipped", 2.005, "measured_wait=[2.005000000,2.011467000)s", "6.467"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "capture.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			end := 2.012
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "wakeup_chain", "pid": 41, "time_start": tc.start, "time_end": end})
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public producer: %v %s", err, result.Summary)
			}
			var edge *types.ObservationRecord
			for i := range result.Observations {
				r := &result.Observations[i]
				if r.Predicate == "wakeup_chain_edge" && r.Object == "target-41" && r.Value == tc.duration {
					edge = r
					break
				}
			}
			if edge == nil {
				t.Fatalf("fixture missing exact native wait %s: %s", tc.duration, result.Summary)
			}
			wait, valid := types.DecodeTraceWakeupMeasuredWait(*edge)
			if !valid {
				t.Errorf("native optional wait did not bind to same physical event: %+v", edge)
			} else {
				if wait.ActualStartTs == nil || *wait.ActualStartTs != 2.003237 || wait.SegmentState != "s_sleep" || wait.StartLine != 5 {
					t.Errorf("native segment metadata: %+v", wait)
				}
			}
			if edge.Span.StartTs != 2.011467 || edge.Span.EndTs != 2.011467 || edge.Span.LineStart != 6 || edge.Span.LineEnd != 6 {
				t.Fatalf("wake credential is not original point: %+v", edge.Span)
			}
			ctx := tracePrincipalValueAuthorityTestContext("target-41", 41, result.Observations)
			ctx.RepoRoot, ctx.WorkDir, ctx.Language = dir, dir, "zh"
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &tc.start, TimeEnd: &end}
			ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentFinalizer), types.AgentFinalizer, types.StageFinalize)
			before, _ := json.Marshal(result)
			projectionBefore := hmc218CausalProjection(t, ctx)
			messages := dependencyObservationMessages(t, ctx)
			for face, text := range map[string]string{"public_tool": result.Summary, "native_observation": edge.Summary, "actual_finalizer": messages} {
				if !strings.Contains(text, tc.interval) {
					t.Errorf("%s lost exact producer-selected measured interval %q (duration=%s)", face, tc.interval, tc.duration)
				}
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, hmc218CausalProjection(t, ctx)) {
				t.Fatal("display changed original results or causal projection")
			}
			if valid {
				hmc299OptionalWaitNegativeControls(t, *edge)
			}
		})
	}
}

func hmc299OptionalWaitNegativeControls(t *testing.T, original types.ObservationRecord) {
	t.Helper()
	originalEvent, ok := types.DecodeRuntimeWakeupEvent(original)
	if !ok {
		t.Fatal("fixture lacks independent wake credential")
	}
	eventBytes, _ := json.Marshal(originalEvent)
	// Display binding judges time operands, not source line order. This is a
	// decoder contract check, not a claim that the parser accepts every raw
	// out-of-order capture format.
	lineOrder := original
	lineOrder.RichNotes = append([]string(nil), original.RichNotes...)
	for i, note := range lineOrder.RichNotes {
		if value, found := strings.CutPrefix(note, types.TraceNoteKeyWakeupMeasuredWait+"="); found {
			var p types.TraceWakeupMeasuredWaitPublication
			if err := json.Unmarshal([]byte(value), &p); err != nil {
				t.Fatal(err)
			}
			p.Wait.StartLine = p.WakeupLine + 10
			wire, _ := json.Marshal(p)
			lineOrder.RichNotes[i] = types.TraceNoteKeyWakeupMeasuredWait + "=" + string(wire)
		}
	}
	if _, valid := types.DecodeTraceWakeupMeasuredWait(lineOrder); !valid {
		t.Fatal("display inferred event ordering from source line ordering")
	}
	for _, name := range []string{"legacy", "other_source", "other_query", "other_window", "other_wake", "running_segment", "missing_start_line", "before_query", "wrong_caliber", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			r := original
			r.RichNotes = nil
			for _, note := range original.RichNotes {
				if name == "wrong_caliber" && strings.HasPrefix(note, types.TraceNoteKeyWakeupLatencyCaliber+"=") {
					note = types.TraceNoteKeyWakeupLatencyCaliber + "=post_wakeup_scheduling"
				}
				if value, found := strings.CutPrefix(note, types.TraceNoteKeyWakeupMeasuredWait+"="); found {
					if name == "legacy" {
						continue
					}
					var p types.TraceWakeupMeasuredWaitPublication
					if err := json.Unmarshal([]byte(value), &p); err != nil {
						t.Fatal(err)
					}
					switch name {
					case "other_source":
						p.Path += ".other"
					case "other_query":
						p.QueryScopeID += ".other"
					case "other_window":
						p.QueryStartTs -= 1
					case "other_wake":
						p.WakeupLine++
					case "running_segment":
						p.Wait.SegmentState = "running"
					case "missing_start_line":
						p.Wait.StartLine = 0
					case "before_query":
						p.Wait.StartTs = p.QueryStartTs - 1
					}
					wire, _ := json.Marshal(p)
					note = types.TraceNoteKeyWakeupMeasuredWait + "=" + string(wire)
					if name == "duplicate" {
						r.RichNotes = append(r.RichNotes, note)
					}
				}
				r.RichNotes = append(r.RichNotes, note)
			}
			if _, valid := types.DecodeTraceWakeupMeasuredWait(r); valid {
				t.Fatal("foreign/missing optional measurement qualified")
			}
			event, valid := types.DecodeRuntimeWakeupEvent(r)
			got, _ := json.Marshal(event)
			if !valid || !bytes.Equal(eventBytes, got) {
				t.Fatal("optional measurement altered/revoked the independent physical wake event")
			}
		})
	}
}
