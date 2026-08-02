package agent

// triage_segment_floor_test.go — EVALFIX-2B 类2 形态B pins (2026-07-30):
// the two-step Step-B loops apply the stage-level MinBytes admission
// floor at SEGMENT granularity in BOTH triage channels. A degenerate
// segment below the floor gets a deterministic skip + StageReport
// disclosure instead of a dedicated LLM dispatch.
//
// MUTATION self-check: with the segment floor removed, the scripted
// adapters below observe one EXTRA Chat call (the degenerate segment's
// extraction dispatch) — the exact-call-count assertions red on that
// mutation, so the pins are decisive for the wiring, not just for a
// helper predicate.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// perfSegFloorScriptedLLM drives the perf two-step flow: call 1 emits the
// segmentation (one healthy extractable segment + one degenerate
// extractable tail), every later call emits a minimal valid perf bundle.
type perfSegFloorScriptedLLM struct{ calls int }

func (l *perfSegFloorScriptedLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{
			ID:   "seg",
			Name: "emit_perf_segmentation",
			Params: json.RawMessage(`{"segments":[
				{"byte_start":0,"byte_end":900,"kind":"jank_region"},
				{"byte_start":900,"byte_end":1000,"kind":"thread_run"}]}`),
		}}}, nil
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{
		ID:     "extract",
		Name:   "emit_perf_trace",
		Params: json.RawMessage(`{"meta":{"source":"hitrace"},"observations":[{"subject":"segment","summary":"one hot region"}]}`),
	}}}, nil
}

func (*perfSegFloorScriptedLLM) ModelID() string               { return "perf-seg-floor-scripted" }
func (*perfSegFloorScriptedLLM) MaxContextTokens() int         { return 128000 }
func (*perfSegFloorScriptedLLM) MaxOutputTokens() int          { return 4096 }
func (*perfSegFloorScriptedLLM) RequestTimeout() time.Duration { return 0 }
func (*perfSegFloorScriptedLLM) RetryMaxAttempts() int         { return 0 }

func TestPerfTriageTwoStep_DegenerateSegmentSkipsLLMDispatch(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitPerfSegmentation{})
	registry.Register(&tool.EmitPerfTrace{})
	skills := skill.NewRegistry()
	skills.Register(&skill.Config{Name: "perf-segmentation-skill", ToolSuggestions: []string{"emit_perf_segmentation"}})
	skills.Register(&skill.Config{Name: "perf-triage-skill", ToolSuggestions: []string{"emit_perf_trace"}})
	adapter := &perfSegFloorScriptedLLM{}
	deps := &Dependencies{
		LLM:    adapter,
		Tools:  registry,
		Skills: skills,
		Emit:   func(render.Event) {},
	}
	settings := PerfTriageSettings{
		Enabled:         true,
		MinBytes:        200,
		MaxRetries:      1,
		TwoStepEnabled:  true,
		TwoStepBytes:    1000, // == trace size → straight to two-step
		TwoStepCoverage: 0.3,
		MaxLLMCalls:     12,
		LLMMaxBytes:     512 * 1024,
	}
	a := NewPerfTriagerAgent(deps, settings)

	ctx := &types.AgentContext{
		AgentName:       types.AgentPerfTriager,
		Stage:           types.StagePerfTriage,
		AttachedHitrace: strings.Repeat("x", 1000),
		Mutable:         types.NewMutableState("perf segment floor"),
	}
	out, err := a.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil StageOutput")
	}
	// 1 segmentation dispatch + 1 extraction dispatch for the 900-byte
	// segment; the 100-byte tail (< min_bytes=200) must NOT buy a round.
	if adapter.calls != 2 {
		t.Fatalf("expected exactly 2 LLM calls (segmentation + one healthy segment), got %d — the degenerate segment bought an LLM dispatch", adapter.calls)
	}
	if !strings.Contains(out.StageReport, "skipped 1 degenerate segments (< min_bytes=200)") {
		t.Fatalf("StageReport must disclose the degenerate skip, got: %q", out.StageReport)
	}
	if ctx.Mutable.PerfTrace() == nil {
		t.Fatal("healthy segment must still produce a merged bundle")
	}
	// ≥ MinBytes segments keep dispatching: the healthy segment's round
	// is asserted by the call count above (2, not 1).
}

// logSegFloorScriptedLLM is the log-channel twin.
type logSegFloorScriptedLLM struct{ calls int }

func (l *logSegFloorScriptedLLM) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	if l.calls == 1 {
		return llm.Response{ToolCalls: []llm.ToolCall{{
			ID:   "seg",
			Name: "emit_log_segmentation",
			Params: json.RawMessage(`{"segments":[
				{"byte_start":0,"byte_end":900,"kind":"stack"},
				{"byte_start":900,"byte_end":1000,"kind":"stack"}]}`),
		}}}, nil
	}
	return llm.Response{ToolCalls: []llm.ToolCall{{
		ID:     "extract",
		Name:   "emit_log_triage",
		Params: json.RawMessage(`{"meta":{"lang":"go","signals":[]},"errors":[],"observations":[{"kind":"runtime_event","subject":"segment","summary":"observed segment payload","evidence":"y","diagnostic":false,"confidence":0.9}]}`),
	}}}, nil
}

func (*logSegFloorScriptedLLM) ModelID() string               { return "log-seg-floor-scripted" }
func (*logSegFloorScriptedLLM) MaxContextTokens() int         { return 128000 }
func (*logSegFloorScriptedLLM) MaxOutputTokens() int          { return 4096 }
func (*logSegFloorScriptedLLM) RequestTimeout() time.Duration { return 0 }
func (*logSegFloorScriptedLLM) RetryMaxAttempts() int         { return 0 }

func TestLogTriageTwoStep_DegenerateSegmentSkipsLLMDispatch(t *testing.T) {
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitLogSegmentation{})
	registry.Register(&tool.EmitLogTriage{})
	skills := skill.NewRegistry()
	skills.Register(&skill.Config{Name: "log-segmentation-skill", ToolSuggestions: []string{"emit_log_segmentation"}})
	adapter := &logSegFloorScriptedLLM{}
	deps := &Dependencies{
		LLM:    adapter,
		Tools:  registry,
		Skills: skills,
		Emit:   func(render.Event) {},
	}
	settings := LogTriageSettings{
		Enabled:         true,
		MinBytes:        200,
		MaxRetries:      1,
		TwoStepEnabled:  true,
		TwoStepBytes:    1000, // == log size → straight to two-step
		TwoStepCoverage: 0.3,
		MaxLLMCalls:     12,
	}
	a := NewLogTriagerAgent(deps, settings)

	ctx := &types.AgentContext{
		AgentName:   types.AgentLogTriager,
		Stage:       types.StageLogTriage,
		AttachedLog: strings.Repeat("y", 1000),
		Mutable:     types.NewMutableState("log segment floor"),
	}
	out, err := a.Execute(ctx, &skill.Config{Name: "log-triage-skill", ToolSuggestions: []string{"emit_log_triage"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == nil {
		t.Fatal("Execute returned nil StageOutput")
	}
	if adapter.calls != 2 {
		t.Fatalf("expected exactly 2 LLM calls (segmentation + one healthy segment), got %d — the degenerate segment bought an LLM dispatch", adapter.calls)
	}
	if !strings.Contains(out.StageReport, "skipped 1 degenerate segments (< min_bytes=200)") {
		t.Fatalf("StageReport must disclose the degenerate skip, got: %q", out.StageReport)
	}
	if ctx.Mutable.LogTriage() == nil {
		t.Fatal("healthy segment must still produce a merged bundle")
	}
}
