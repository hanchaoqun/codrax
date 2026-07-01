package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDefaultPerfTriageSettingsCapsLLMPreTriageBytes(t *testing.T) {
	got := DefaultPerfTriageSettings()
	if got.LLMMaxBytes != 512*1024 {
		t.Fatalf("LLMMaxBytes = %d, want 512 KiB", got.LLMMaxBytes)
	}
}

func TestPerfTriagerDefaultSkipsMultiMiBTraceBeforeLLMDispatch(t *testing.T) {
	a := &perfTriager{
		settings: DefaultPerfTriageSettings(),
	}
	ctx := &types.AgentContext{
		AttachedHitrace: strings.Repeat("x", 1024*1024),
		Mutable:         types.NewMutableState("analyze attached trace"),
	}

	out, err := a.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected skipped StageOutput")
	}
	if !strings.Contains(out.StageReport, "exceeds perf_triage_llm_max_bytes") ||
		!strings.Contains(out.StageReport, "trace_query") {
		t.Fatalf("default multi-MiB trace should be delegated to trace_query, got %q", out.StageReport)
	}
}

func TestPerfTriagerSkipsOversizedTraceBeforeLLMDispatch(t *testing.T) {
	a := &perfTriager{
		settings: PerfTriageSettings{
			Enabled:     true,
			MinBytes:    1,
			LLMMaxBytes: 4,
		},
	}
	ctx := &types.AgentContext{
		AttachedHitrace: "12345",
		Mutable:         types.NewMutableState("analyze attached trace"),
	}

	out, err := a.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil {
		t.Fatal("expected skipped StageOutput")
	}
	if !strings.Contains(out.StageReport, "exceeds perf_triage_llm_max_bytes") ||
		!strings.Contains(out.StageReport, "trace_query") {
		t.Fatalf("oversized trace should be delegated to trace_query, got %q", out.StageReport)
	}
	if got := ctx.Mutable.PerfTrace(); got != nil {
		t.Fatalf("oversized skip must not synthesize a perf bundle: %+v", got)
	}
}
