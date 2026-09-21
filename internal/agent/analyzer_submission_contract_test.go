package agent

import (
	"encoding/json"
	"strings"
	"testing"

	agentctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnalyzerSubmissionGateSeparatesAttemptsFromAcceptedWrites(t *testing.T) {
	for _, tc := range []struct {
		name      string
		strict    bool
		results   []types.ToolResult
		attempts  int
		accepted  int
		wantError bool
	}{
		{"strict_repair", true, []types.ToolResult{emitResult(false), emitResult(true)}, 2, 1, false},
		{"warning_repair", false, []types.ToolResult{emitResult(false), emitResult(true)}, 2, 1, false},
		{"strict_all_failed", true, []types.ToolResult{emitResult(false), emitResult(false)}, 2, 0, true},
		{"warning_all_failed", false, []types.ToolResult{emitResult(false), emitResult(false)}, 2, 0, true},
		{"strict_two_writes", true, []types.ToolResult{emitResult(true), emitResult(false), emitResult(true)}, 3, 2, true},
		{"warning_two_writes", false, []types.ToolResult{emitResult(true), emitResult(false), emitResult(true)}, 3, 2, false},
		{"failed_last_attempt_does_not_erase_write", true, []types.ToolResult{emitResult(true), emitResult(false)}, 2, 1, false},
		{"other_tool_success_is_not_a_write", true, []types.ToolResult{{ToolName: "grep", Success: true}, emitResult(false)}, 1, 0, true},
		{"zero_attempts", true, nil, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restoreAnalysisLimits(t)
			tool.SetAnalysisLimits(tool.AnalysisLimits{RejectMultipleEmit: tc.strict})
			mu := types.NewMutableState("explain analyzer dispatch")
			// Keep a populated model even on the all-failed arm: a prior write
			// must not let this dispatch certify an unaccepted classification.
			mu.SetRequestModel(types.RequestModel{Intent: types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain, Complexity: types.ComplexityModerate,
				AnalyzerHints: types.AnalyzerHints{Keywords: []string{"dispatch", "classify", "emit", "request", "model", "scope", "retry", "accept"}, Kind: "mechanism"}})
			data, out := parseWithToolResults(t, mu, tc.results)
			if (out.Error != "") != tc.wantError {
				t.Errorf("error=%q, want error=%v", out.Error, tc.wantError)
			}
			if got := getFloat(t, data, "analysis_emit_calls"); got != float64(tc.attempts) {
				t.Errorf("attempt telemetry=%g want %d", got, tc.attempts)
			}
			if got := getFloat(t, data, "analysis_emit_successes"); got != float64(tc.accepted) {
				t.Errorf("accepted telemetry=%g want %d", got, tc.accepted)
			}
			if (out.AnalysisIR != nil) != (tc.accepted > 0) {
				t.Errorf("IR present=%v, accepted=%d", out.AnalysisIR != nil, tc.accepted)
			}
		})
	}
}

func TestAnalyzerSubmissionPromptUsesSchemaAndAllowsRejectedRepair(t *testing.T) {
	for _, retry := range []int{0, 1} {
		for _, attached := range []bool{false, true} {
			ctx := &types.AgentContext{AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
				Objective: "explain request classification", EmitStageRetryAttempt: retry,
				Mutable: types.NewMutableState("explain request classification")}
			if attached {
				ctx.AttachedHitrace = "app-100 (100) [001] .... 2.000000: sched_switch: prev_comm=app prev_pid=100 prev_state=S ==> next_comm=idle/1 next_pid=0"
			}
			sk := skill.BuildAnalysisSkill()
			eval := &analyzerEvaluator{}
			initial := eval.BuildInitialInstruction(ctx, sk)
			if attached && !strings.Contains(initial, "Attached Runtime Artifact Classification Shortcut") {
				t.Fatal("attached-runtime prompt arm was not exercised")
			}
			messages := agentctx.ToMessages(agentctx.BuildPromptContext(ctx, sk))
			var b strings.Builder
			for _, msg := range messages {
				b.WriteString(msg.Content)
			}
			b.WriteString(initial)
			// These dynamic hints constrain ONE response, not all attempts in a
			// dispatch. Keep their terminal tool boundary alongside repair teaching.
			b.WriteString(analyzerTerminalEmitOnlyHint())
			b.WriteString(analyzerExplicitRuntimeArtifactPathEmitOnlyHint())
			b.WriteString((&tool.EmitAnalysis{}).Description())
			b.Write((&tool.EmitAnalysis{}).Parameters())
			prompt := b.String()
			for _, banned := range []string{"every field in emit_analysis is REQUIRED", "Call emit_analysis EXACTLY ONCE",
				"call emit_analysis EXACTLY ONCE", "then call emit_analysis exactly once", "Call at most once per dispatch",
				"Required fields: intent", "The required fields are:", "Optional fields: sub_topics",
				"object with eight required booleans", "runtime_question_profile — object with scope",
				"files that you have READ", "file content in the prescan output", "READ a candidate file in pre-scan"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("retry=%d assembled prompt carries retired teaching %q", retry, banned)
				}
			}
			for _, want := range []string{"one successful emit_analysis", "one complete corrected call",
				"schema is the authority for JSON field names, types, required fields, and conditional fields",
				"Condition-dependent implementation/tool/path choice across implementations remains declared once in the separate required `runtime_selection_profile`",
				"Omit fact_families for causal_diagnosis, relation_analysis, system_overview, unspecified, and not_applicable",
				"already-returned, allowed navigation metadata", "do not open source content merely to populate this field"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("retry=%d assembled prompt lost %q", retry, want)
				}
			}
			eval.terminalEmitOnlyInstructionIssued = true
			failed := emitResult(false)
			sig := eval.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &failed,
				CurrentToolResults: []types.ToolResult{failed}, AllToolResults: []types.ToolResult{failed}})
			if sig.StopRequested || sig.HintRequested {
				t.Fatalf("failed terminal emit must reach its own validation repair, retry=%d attached=%v: %+v", retry, attached, sig)
			}
		}
	}
	var schema struct {
		Properties map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&tool.EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	predicates := schema.Properties["predicates"]
	if len(predicates.Required) != 9 || len(predicates.Properties) != 9 {
		t.Fatalf("live schema must retain nine explicit predicates: %+v", predicates)
	}
}
