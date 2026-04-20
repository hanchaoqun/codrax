package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestOrchestrator_AttachedLogSetterGetter pins the log-triage entry
// point: SetAttachedLog stores the payload, AttachedLog reads it back,
// and nothing else is touched. REPL /log and CLI --log both rely on
// this round-trip to deliver the user's log excerpt to Run.
func TestOrchestrator_AttachedLogSetterGetter(t *testing.T) {
	ar := agent.NewRegistry()
	sr := skill.NewRegistry()
	sar := agent.NewSubAgentRegistry()
	o := New(types.PipelineSettings{}, ar, sr, sar)

	if got := o.AttachedLog(); got != "" {
		t.Errorf("fresh orchestrator AttachedLog = %q, want empty", got)
	}
	payload := "panic: test\ngoroutine 1 [running]:\n"
	o.SetAttachedLog(payload)
	if got := o.AttachedLog(); got != payload {
		t.Errorf("after SetAttachedLog: got %q, want %q", got, payload)
	}
	// Empty string clears.
	o.SetAttachedLog("")
	if got := o.AttachedLog(); got != "" {
		t.Errorf("after clearing: got %q, want empty", got)
	}
}
