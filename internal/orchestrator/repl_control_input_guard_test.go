package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRun_RefusesREPLControlInput locks the contract that a literal
// slash-command request reaches the orchestrator at most once and
// fail-louds before any LLM call. The customer trigger was a CLI
// invocation that piped `/approve plan-XXX` as --request: the
// analyzer iterated to its budget cap rejecting its own emit_analysis
// call (`IsREPLControlInput` true on every retry) before SIGINT
// killed the run.
//
// Two assertions:
//   1. err is non-nil and names the REPL-control-input shape.
//   2. The agent registry's analyzer was NEVER dispatched — observed
//      via a counter wired into the analyzer fn.
func TestRun_RefusesREPLControlInput(t *testing.T) {
	for _, slashed := range []string{
		"/approve plan-1777259041968496495-1896504",
		"/verify plan-XXX",
		"/merge --branch=main",
		"/plan",
		"\\reject some reason",
	} {
		analyzerCalls := 0
		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
				analyzerCalls++
				return &agent.StageOutput{}, nil
			},
		}
		ar, sr, sar := buildRegistries(agentFns)
		o := New(types.PipelineSettings{}, ar, sr, sar)

		_, err := o.Run(slashed, "/tmp/probe-repo", "main")

		if err == nil {
			t.Errorf("input %q: expected fail-loud; got nil error", slashed)
			continue
		}
		if !strings.Contains(err.Error(), "REPL control command") {
			t.Errorf("input %q: error should name 'REPL control command'; got: %v",
				slashed, err)
		}
		if analyzerCalls != 0 {
			t.Errorf("input %q: analyzer dispatched %d times; want 0 (guard fires before any agent dispatch)",
				slashed, analyzerCalls)
		}
	}
}

// TestRun_AcceptsSyntheticDispatchPhrasing verifies the synthesized
// request phrasings that the REPL handlers emit (approveDispatchRequest
// and verifyDispatchRequest) DO pass the guard. Without this check
// a regex-shaped guard could incorrectly false-fire on legit dispatch
// strings like "Apply approved plan plan-XXX: ...".
func TestRun_AcceptsSyntheticDispatchPhrasing(t *testing.T) {
	for _, legit := range []string{
		"Apply approved plan plan-1234: refactor handler.go",
		"Verify applied plan plan-1234: regression check",
		"how does explorer.ShouldStop work?",
		"用 python 写一个猜数字游戏",
	} {
		agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
			types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
				return &agent.StageOutput{}, nil
			},
		}
		ar, sr, sar := buildRegistries(agentFns)
		o := New(types.PipelineSettings{}, ar, sr, sar)
		_, err := o.Run(legit, "/tmp/probe-repo", "main")
		// Run will fail downstream (no real analyzer IR), but the
		// REPL-control-input guard must NOT be the cause. Discriminate
		// by looking at the error text.
		if err != nil && strings.Contains(err.Error(), "REPL control command") {
			t.Errorf("input %q: legit dispatch phrasing wrongly flagged as REPL control input", legit)
		}
	}
}
