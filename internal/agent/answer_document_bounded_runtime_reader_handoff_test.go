package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func boundedRuntimeReaderHandoffTestContext() *types.AgentContext {
	start, end := 13762.791708, 13763.024898
	zero := 0
	ref := types.ObservationSourceRef{
		Kind:       types.ObservationSourceRuntimeArtifact,
		ArtifactID: "trace-h4",
		Path:       "/tmp/h4.ftrace",
	}
	mut := types.NewMutableState("查询显式窗口内线程状态与频率")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "window_stats",
			FrequencyLimitWitnesses: []types.TraceFrequencyLimitAuthority{{
				CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000,
				LimitRowCount: 28, WindowStartTs: start, WindowEndTs: end,
				Authority: "direct_in_window_policy_limit",
			}},
		},
		Observations: []types.ObservationRecord{
			{
				ID: "state", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "target_window_states", ClaimKey: "target_window_states:.ugc.aweme.lite-17267",
				Subject: ".ugc.aweme.lite-17267", Object: "state_partition", Value: "233.190", Unit: "ms",
				RichNotes: []string{
					"selected_window=13762.791708..13763.024898", "running=157.248", "runnable=5.604",
					"sleep=70.338", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=0.000", "total=233.190",
				},
			},
			{
				ID: "wait", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "target_window_wait_occurrences", ClaimKey: "target_window_wait_occurrences:.ugc.aweme.lite-17267",
				Subject: ".ugc.aweme.lite-17267", Object: "complete", Value: "0", Unit: "occurrences",
				ResultCount: &zero,
				RichNotes: []string{
					"selected_window=13762.791708..13763.024898",
					"target_wait_occurrence_prompt=status=complete,emitted=0,total=0",
					"target_wait_occurrence_prompt_sum_ms=0.000",
				},
			},
			{
				ID: "cpu-running", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "target_cpu_running", Subject: ".ugc.aweme.lite-17267", Object: "cpu=4", Value: "35.960", Unit: "ms",
				RichNotes: []string{
					"selected_window=13762.791708..13763.024898", "target_cpu_running_cpu=4",
					"target_cpu_running_roster_status=complete",
				},
			},
			{
				ID: "cpu-frequency", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				Predicate: "running_time", Subject: ".ugc.aweme.lite-17267", Object: "running", Value: "35.960", Unit: "ms",
				RichNotes: []string{
					"selected_window=13762.791708..13763.024898", "cpu=4", "freq=558000",
				},
			},
		},
	}}})
	return &types.AgentContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "zh",
			Intent:   types.IntentTrace,
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
				Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{
					types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactCountOrDuration,
					types.RuntimeQuestionFactFrequencyResidency,
				},
			},
			RuntimeTargets: []types.RuntimeTarget{{
				Kind: types.RuntimeTargetKindThread, PID: 17267,
				Thread: ".ugc.aweme.lite-17267", Source: "user_explicit",
			}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
				RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart:      &start, TimeEnd: &end, SourceQuote: "13762.791708 到 13763.024898",
			},
		}},
	}
}

func TestBoundedRuntimeFinalReaderHandoffUsesNaturalLanguageWithoutWireEnums(t *testing.T) {
	got := renderAnswerDocBoundedRuntimeFinalReaderHandoff(boundedRuntimeReaderHandoffTestContext())
	for _, want := range []string{
		"有限窗口查询的读者事实卡（结论由模型给出）",
		"目标线程状态分布、次数或持续时间、CPU 频率驻留与策略上限",
		// EVOLUTION RECORD (V3-1, §40.20): the handoff prints the types-level
		// account sentence (types.FormatTargetStateAccount).
		"运行 157.248 毫秒，可运行但尚未获调度 5.604 毫秒，可中断睡眠 70.338 毫秒，不可中断等待 0.000 毫秒（其中调度器标记的 IO 等待 0.000 毫秒）",
		"没有匹配到由调度器标记的 D 状态或 IO 等待",
		"没有评估由 IO 完成事件闭合的 S 状态等待",
		"缺席表示未评估，不是测得为零",
		"调度器标记等待清单已完整覆盖所选窗口：共 0 次，合计 0.000 毫秒",
		"CPU 4",
		"策略范围为 558000–2100000 kHz",
		"是否限制了目标线程，仍需同一 CPU 上目标运行切片与策略的重叠",
		"目标线程的逐 CPU 运行与频率对照",
		"于 CPU 4 运行 35.960ms（完整目标运行 CPU 清单中的一项）",
		"代表频率 558000kHz",
		"不证明该频率覆盖了目标线程的具体运行切片",
		"两项出现在同一 CPU 仍不足以证明目标切片与策略重叠、目标受限或产生性能影响",
		"系统不检查或修改模型正文，也不代替模型给结论",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reader handoff missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"bounded_window_candidate", "target_window_states", "target_window_wait_occurrences",
		"status=complete", "full_window_all_cpu", "target_effect_unproven_no_slice_binding",
		"direct_in_window_policy_limit", "coverage_status", "unproven",
		"dominant_state_slice_representative", "CPU-owned", "target-slice",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("reader handoff leaked wire token %q:\n%s", forbidden, got)
		}
	}
}

func TestBuildInitialInstructionPinsBoundedRuntimeReaderHandoffAtFinalSeam(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(boundedRuntimeReaderHandoffTestContext(), nil)
	marker := "## 有限窗口查询的读者事实卡（结论由模型给出）"
	if strings.Count(prompt, marker) != 1 {
		t.Fatalf("bounded runtime reader handoff wiring count=%d, want 1", strings.Count(prompt, marker))
	}
	if strings.LastIndex(prompt, marker) < strings.LastIndex(prompt, "## Observation Ledger") {
		t.Fatalf("reader handoff must follow the raw observation ledger")
	}
	for _, want := range []string{
		"没有评估由 IO 完成事件闭合的 S 状态等待",
		"字段名、枚举值、状态码和机器键值只用于校验",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("wired prompt missing %q", want)
		}
	}
}
