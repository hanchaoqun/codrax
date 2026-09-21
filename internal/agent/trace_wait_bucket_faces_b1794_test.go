package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A native D/S cycle inside a real B/E marker exercises the five-bucket
// business account and the requested finite-state guidance independently of
// the final recap. No observation or measured duration is hand-authored.
func b1794WaitBucketContext(t *testing.T, state string, marker int, lang string) *types.AgentContext {
	t.Helper()
	trace := fmt.Sprintf(`idle-0 (0) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... 1.000100: tracing_mark_write: B|77|ReadWork
reader-77 (77) [000] .... 1.001000: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120
irq-9 (9) [001] .... 1.002000: sched_waking: comm=reader pid=77 prio=120 target_cpu=0
irq-9 (9) [001] .... 1.002001: sched_blocked_reason: pid=77 iowait=%d caller=wait_site
idle-0 (0) [000] .... 1.003000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=reader next_pid=77 next_prio=120
reader-77 (77) [000] .... 1.003900: tracing_mark_write: E|77
reader-77 (77) [000] .... 1.004000: sched_switch: prev_comm=reader prev_pid=77 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, state, marker)
	start, end := 1.0, 1.004
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: trace, AttachedHitraceSource: "harmony_hitrace"}
	params, _ := json.Marshal(map[string]any{"source": "attached_trace", "view": "window_stats", "pid": 77, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("public query failed: %v %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext("reader-77", 77, result.Observations)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent, rm.Language = types.IntentExplain, lang
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
		FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences}}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.004"}
	ctx.Mutable.SetRequestModel(*rm)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "The model owns the conclusion; retain 9.876 ms."}},
	})
	return ctxbuilder.BuildAgentContext(&types.BusContext{RepoRoot: dir, WorkDir: dir, Language: lang, Mutable: ctx.Mutable, AnalysisIR: ctx.AnalysisIR, AttachedHitrace: trace, AttachedHitraceSource: "harmony_hitrace"}, types.AgentFinalizer, types.StageFinalize)
}

func TestB1794WaitBucketsActualFinalizerMessages(t *testing.T) {
	for _, tc := range []struct {
		name, state       string
		marker            int
		d, io, sleep, sio string
	}{
		{"D_IO", "D", 1, "0.000", "1.000", "0.000", "0.000"},
		{"D_non_IO", "D", 0, "1.000", "0.000", "0.000", "0.000"},
		{"S_IO", "S", 1, "0.000", "0.000", "1.000", "1.000"},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				ctx := b1794WaitBucketContext(t, tc.state, tc.marker, lang)
				ledger := answerDocObservationLedger(ctx)
				facts := types.TraceBusinessSpanFacts(ledger, &ctx.AnalysisIR.RequestModel)
				if len(facts) != 1 || facts[0].Object != "ReadWork" || facts[0].Value != "3.800" {
					t.Fatalf("native marker premise failed: %+v", facts)
				}
				var states tracequery.TraceSpanSchedulerStates
				if err := json.Unmarshal([]byte(traceQueryObservationSupplementNoteValue(facts[0], types.TraceNoteKeyBusinessSpanSchedulerStates)), &states); err != nil {
					t.Fatal(err)
				}
				for name, values := range map[string]struct {
					got  float64
					want string
				}{
					"running": {states.RunningMs, "1.800"}, "runnable": {states.RunnableMs, "1.000"},
					"D": {states.DStateMs, tc.d}, "IO": {states.IOWaitMs, tc.io},
					"sleep": {states.SleepMs, tc.sleep}, "sleep_IO": {states.SleepIOWaitMs, tc.sio}, "accounted": {states.AccountedMs, "3.800"},
				} {
					if fmt.Sprintf("%.3f", values.got) != values.want {
						t.Fatalf("native %s premise got %.6f want %s", name, values.got, values.want)
					}
				}
				messages := dependencyObservationMessages(t, ctx)
				var finite, skillLine, business string
				for _, line := range strings.Split(messages, "\n") {
					if strings.Contains(line, "Runtime finite target-state caliber hint:") {
						finite = line
					}
					if strings.Contains(line, "STATE-DURATION CALIBER SEPARATION:") {
						skillLine = line
					}
					if strings.HasPrefix(line, "- 业务 \"ReadWork\"") || strings.HasPrefix(line, "- Work \"ReadWork\"") {
						business = line
					}
				}
				for name, face := range map[string]string{"finite": finite, "skill": skillLine} {
					if !strings.Contains(face, types.TraceSchedulerWaitPartitionTeaching) {
						t.Errorf("%s still lacks shared physical-state/account-bucket boundary", name)
					}
					for _, wrong := range []string{"Its IO-wait fields cover only D/explicit io_wait plus S", "io_wait/sleep_io_wait is only the scheduler-marked classifier: D or explicit io_wait", "Describe a zero as `no scheduler-marked D/IO-wait match", "Describe zero as no matching scheduler marker"} {
						if strings.Contains(face, wrong) {
							t.Errorf("%s retains contradictory teaching %q", name, wrong)
						}
					}
					if !strings.Contains(face, "completion-closed issuer-blocked IO") || !strings.Contains(face, "not assessed by") || !strings.Contains(face, "state partition") {
						t.Errorf("%s lost independent IO-mechanism ceiling", name)
					}
				}
				zh := lang == "zh"
				if !strings.Contains(business, tool.TraceStateNonIODStateWord(zh)+" "+tc.d) || !strings.Contains(business, "3.800") || !strings.Contains(business, "observation_id=") || !strings.Contains(business, "query_scope=") {
					t.Errorf("business raw D bucket uses the native-total label or lost its scope: %s", business)
				}
				if zh {
					if !strings.Contains(business, "睡眠中调度标记 IO 等待 "+tc.sio+" 毫秒为包含项，不另加") {
						t.Error("sleep IO overlay lost its included-only boundary")
					}
				} else if !strings.Contains(business, "scheduler-marked IO within sleep "+tc.sio+" ms is an included overlay, not an addend") {
					t.Error("sleep IO overlay lost its included-only boundary")
				}
			})
		}
	}
}

func TestB1794RawDNoteLabelsUseExistingExclusiveBucketWord(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, key := range []string{types.TraceNoteKeyDState, "peer_state_d_state", "subject_state_d_state"} {
			t.Run(fmt.Sprintf("%t/%s", zh, key), func(t *testing.T) {
				got, ok := traceQueryObservationSupplementNoteDisplay(key+"=1.250", zh)
				if !ok || !strings.Contains(got, tool.TraceStateNonIODStateWord(zh)) || !strings.HasSuffix(got, "1.250") {
					t.Errorf("raw D bucket must keep its value with the existing exclusive-bucket label: %q", got)
				}
			})
		}
	}
	for _, note := range []string{"unknown_wait_bucket=1.250", "d_state=", "d_state"} {
		if _, ok := traceQueryObservationSupplementNoteDisplay(note, true); ok {
			t.Errorf("invalid/unknown note newly admitted: %q", note)
		}
	}
}

func TestB1794FiniteWaitTeachingDoesNotActivateForUnrequestedState(t *testing.T) {
	for _, profile := range []*types.RuntimeQuestionProfile{nil, {Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactIOLatency}}} {
		ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeQuestionProfile: profile}}}
		if got := renderAnswerDocRuntimeFiniteTargetStateCaliberHint(ctx); got != "" {
			t.Fatalf("state teaching expanded an unrequested scope: %s", got)
		}
	}
}
