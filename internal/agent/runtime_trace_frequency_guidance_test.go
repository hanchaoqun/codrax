package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceGuidanceCarriesDirectFrequencyLimitWitnesses(t *testing.T) {
	mut := types.NewMutableState("分析显式窗口内 CPU 供给")
	witness0 := types.TraceFrequencyLimitAuthority{
		CPU: 0, MinFrequencyKHz: 418000, MaxFrequencyKHz: 1530000,
		LimitRowCount: 16, WitnessLine: 8048, WitnessTs: 13762.861720,
		WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		Authority: "direct_in_window_policy_limit",
	}
	witness4 := types.TraceFrequencyLimitAuthority{
		CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000,
		LimitRowCount: 28, WitnessLine: 17113, WitnessTs: 13762.940114,
		WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		Authority: "direct_in_window_policy_limit",
	}
	for i := 0; i < 2; i++ {
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View:                    "window_stats",
				FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{witness0, witness4},
			},
			Observations: []types.ObservationRecord{
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "running_time", Subject: "app-17267", Object: "running", Value: "35.960", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898", "cpu=4", "freq=558000"},
				},
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "running_time", Subject: "app-17267", Object: "running", Value: "96.081", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898", "cpu=12", "freq=2075000"},
				},
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "running_time", Subject: "app-17267", Object: "running", Value: "999.000", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=1.000000..2.000000", "cpu=4", "freq=9999999"},
				},
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "target_cpu_running", Subject: "app-17267", Object: "cpu=4", Value: "35.960", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898", types.TraceNoteKeyTargetCPURunningCPU + "=4", types.TraceNoteKeyTargetCPURunningRosterStatus + "=complete"},
				},
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "target_cpu_running", Subject: "app-17267", Object: "cpu=12", Value: "96.081", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=13762.791708..13763.024898", types.TraceNoteKeyTargetCPURunningCPU + "=12", types.TraceNoteKeyTargetCPURunningRosterStatus + "=complete"},
				},
				{
					Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
					Predicate: "target_cpu_running", Subject: "app-17267", Object: "cpu=7", Value: "999.000", Unit: "ms",
					RichNotes: []string{types.TraceNoteKeySelectedWindow + "=1.000000..2.000000", types.TraceNoteKeyTargetCPURunningCPU + "=7", types.TraceNoteKeyTargetCPURunningRosterStatus + "=complete"},
				},
			},
		})
	}
	ctx := &types.AgentContext{Mutable: mut}

	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Runtime direct frequency-limit authority",
		"`cpu=0 min=418000kHz max=1530000kHz limit_rows=16 witness_line=8048 witness_ts=13762.861720 window=13762.791708..13763.024898 authority=direct_in_window_policy_limit`",
		"the policy ceiling is present in the selected window",
		"whether that ceiling restricted this target/workload is unproven",
		"`policy ceiling present; target binding unproven`",
		"do not turn the pair into an affirmative `the target was frequency-restricted`",
		"actual/average/residency frequency below that ceiling does not negate the policy limit",
		"`min_frequency_khz` is the lower policy bound and `max_frequency_khz` is the upper policy ceiling",
		"must not be described as equal to the maximum",
		"requires a target-running-slice overlap or another target-to-policy binding carrier",
		"neither that lower observed frequency nor the limit row proves that the workload hit the ceiling",
		"does not by itself identify the lower-frequency cause or prove governance binding",
		"Normalize every frequency comparison to one unit",
		"`running_scope=full_window_all_cpu`",
		"`runnable_scope=full_window_off_cpu_wait`",
		"never CPU execution or occupancy",
		"Do not add runnable to running and call the sum CPU occupancy",
		"`value_scope=running_plus_runnable_state_time_not_cpu_occupancy`",
		"`cpu_scope=dominant_state_slice_representative_not_exclusive`",
		"Never call that CPU the target's only CPU",
		"enumerate every available target-owned CPU row",
		"A policy row for one CPU binds only to target running evidence on that same CPU",
		"Runtime target/CPU policy comparison matrix (typed identity alignment only; the model still owns the restriction verdict)",
		"Every frequency and policy value is owned by that row's exact CPU; never compare, copy, or combine values across rows",
		"| `app-17267` | `13762.791708..13763.024898` | `0` | `absent_in_complete_roster` | `absent` | `present:min=418000kHz,max=1530000kHz,rows=16` | `not_comparable_missing_same_cpu_pair` |",
		"| `app-17267` | `13762.791708..13763.024898` | `4` | `35.960ms` | `558000kHz(CPU-owned running-bucket representative; not target-slice/policy overlap proof)` | `present:min=558000kHz,max=2100000kHz,rows=28` | `target_effect_unproven_no_slice_binding` |",
		"| `app-17267` | `13762.791708..13763.024898` | `12` | `96.081ms` | `2075000kHz(CPU-owned running-bucket representative; not target-slice/policy overlap proof)` | `absent` | `not_comparable_missing_same_cpu_pair` |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("frequency guidance missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "cpu=0 min=418000kHz max=1530000kHz") != 1 {
		t.Fatalf("duplicate typed witness was not deduplicated:\n%s", got)
	}
	if strings.Contains(got, "target=app-17267 window=1.000000..2.000000") || strings.Contains(got, "target_running=999.000ms") {
		t.Fatalf("target CPU rows from another exploratory window must not join this policy witness:\n%s", got)
	}
	if strings.Contains(got, "9999999kHz") {
		t.Fatalf("a frequency from another exploratory window must not join this policy witness:\n%s", got)
	}
}

func TestRuntimeTraceFrequencyCPUJoinEscapesMarkdownCellSeparators(t *testing.T) {
	mut := types.NewMutableState("bounded runtime frequency")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{{
			CPU: 4, MinFrequencyKHz: 500000, MaxFrequencyKHz: 2000000, LimitRowCount: 1,
			WindowStartTs: 1, WindowEndTs: 2, Authority: "direct_in_window_policy_limit",
		}}},
		Observations: []types.ObservationRecord{
			{
				Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				Predicate: "target_cpu_running", Subject: "app|worker", Object: "cpu=4", Value: "1.000", Unit: "ms",
				RichNotes: []string{types.TraceNoteKeySelectedWindow + "=1.000000..2.000000", types.TraceNoteKeyTargetCPURunningCPU + "=4", types.TraceNoteKeyTargetCPURunningRosterStatus + "=complete"},
			},
		},
	})
	got := renderAnswerDocRuntimeTraceAnswerGuidance(&types.AgentContext{Mutable: mut})
	if !strings.Contains(got, "`app¦worker`") || strings.Contains(got, "`app|worker`") {
		t.Fatalf("matrix cell did not escape markdown separator:\n%s", got)
	}
}

func TestRuntimeTraceGuidanceDoesNotInventFrequencyLimitWithoutTypedWitness(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("分析 trace")}
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	if strings.Contains(got, "Runtime direct frequency-limit authority") {
		t.Fatalf("frequency-limit guidance appeared without typed authority:\n%s", got)
	}
}
