package recommend

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestParseLLMRecommendations_HappyPath verifies the LLM
// emit_env_recommendation tool call → []Recommendation conversion
// for a 2-candidate response. Strategies parsed correctly,
// !-prefix preserved, priority assigned 1-indexed.
func TestParseLLMRecommendations_HappyPath(t *testing.T) {
	call := llm.ToolCall{
		Name: "emit_env_recommendation",
		Params: json.RawMessage(`{
		  "recommendations": [
		    {
		      "strategy": "venv_install",
		      "command": "!python3 -m venv .venv && .venv/bin/pip install pytest",
		      "why": "project-isolated venv keeps system clean",
		      "needs_network": true,
		      "estimated_sec": 30
		    },
		    {
		      "strategy": "user_install",
		      "command": "!pip install --user pytest",
		      "why": "user-level install fallback",
		      "needs_network": true,
		      "caveats": ["~/.local/bin must be on PATH"]
		    }
		  ]
		}`),
	}
	recs := parseLLMRecommendations(call)
	if len(recs) != 2 {
		t.Fatalf("expected 2 recs, got %d", len(recs))
	}
	if recs[0].Strategy != types.StrategyVenvInstall {
		t.Errorf("rec[0] strategy: want VenvInstall, got %v", recs[0].Strategy)
	}
	if recs[0].Priority != 1 {
		t.Errorf("rec[0] priority: want 1, got %d", recs[0].Priority)
	}
	if recs[1].Priority != 2 {
		t.Errorf("rec[1] priority: want 2, got %d", recs[1].Priority)
	}
	if len(recs[1].Caveats) != 1 {
		t.Errorf("rec[1] caveats lost in unmarshal; got %+v", recs[1].Caveats)
	}
}

// TestParseLLMRecommendations_RejectsNonBangCommand verifies the
// !-prefix invariant is enforced. The system prompt requires every
// command to start with `!` so users can paste into the codrax
// REPL; an LLM emission missing the prefix is dropped.
func TestParseLLMRecommendations_RejectsNonBangCommand(t *testing.T) {
	call := llm.ToolCall{
		Name: "emit_env_recommendation",
		Params: json.RawMessage(`{
		  "recommendations": [
		    {
		      "strategy": "venv_install",
		      "command": "python3 -m venv .venv",
		      "why": "missing the ! prefix"
		    }
		  ]
		}`),
	}
	recs := parseLLMRecommendations(call)
	if len(recs) != 0 {
		t.Errorf("non-! command must be rejected; got %+v", recs)
	}
}

// TestParseLLMRecommendations_WrongToolName guards against the LLM
// calling a sibling tool (e.g. emit_change_plan) by accident.
func TestParseLLMRecommendations_WrongToolName(t *testing.T) {
	call := llm.ToolCall{
		Name:   "emit_change_plan",
		Params: json.RawMessage(`{"recommendations": []}`),
	}
	recs := parseLLMRecommendations(call)
	if recs != nil {
		t.Errorf("wrong tool name must yield nil; got %+v", recs)
	}
}

// TestParseLLMRecommendations_UnknownStrategyDropped ensures the
// strategy enum gate works — an LLM that invents a new strategy
// label gets that recommendation silently skipped, but valid
// siblings survive.
func TestParseLLMRecommendations_UnknownStrategyDropped(t *testing.T) {
	call := llm.ToolCall{
		Name: "emit_env_recommendation",
		Params: json.RawMessage(`{
		  "recommendations": [
		    {"strategy": "magic_install", "command": "!magic install foo", "why": "invalid strategy"},
		    {"strategy": "user_install",  "command": "!pip install --user foo", "why": "valid"}
		  ]
		}`),
	}
	recs := parseLLMRecommendations(call)
	if len(recs) != 1 {
		t.Fatalf("expected 1 surviving rec; got %d (%+v)", len(recs), recs)
	}
	if recs[0].Strategy != types.StrategyUserInstall {
		t.Errorf("survivor should be user_install; got %v", recs[0].Strategy)
	}
}

// TestDocsLinkFallback_AllSupportedRunners locks the runnerDocsURL
// table so a missing entry breaks CI rather than silently emitting
// no recommendation. Every runner the codrax test pipeline knows
// about has a docs URL fallback.
func TestDocsLinkFallback_AllSupportedRunners(t *testing.T) {
	for _, runner := range []string{
		"python", "node", "rust", "go", "java", "ruby",
		"swift", "cmake", "meson", "make", "hvigor", "cjpm",
	} {
		t.Run(runner, func(t *testing.T) {
			d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: runner}
			recs := docsLinkFallback(d)
			if len(recs) == 0 {
				t.Fatalf("runner %s missing from runnerDocsURL", runner)
			}
			if recs[0].Strategy != types.StrategyDocsLink {
				t.Errorf("runner %s fallback should be DocsLink; got %v", runner, recs[0].Strategy)
			}
			if recs[0].Command == "" {
				t.Errorf("runner %s fallback command empty", runner)
			}
		})
	}
}

// TestDocsLinkFallback_UnknownRunner returns nil so the Renderer
// surfaces a generic "no actionable recommendation" footer.
func TestDocsLinkFallback_UnknownRunner(t *testing.T) {
	d := types.Diagnosis{Kind: types.DiagRunnerMissing, Runner: "unknown_xyz"}
	if recs := docsLinkFallback(d); recs != nil {
		t.Errorf("unknown runner should return nil; got %+v", recs)
	}
}
