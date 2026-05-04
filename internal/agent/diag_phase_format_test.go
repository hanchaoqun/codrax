package agent

import (
	"os"
	"strings"
	"testing"
)

// diag_phase_format_test.go — R12 lock test (post-shape 残留 audit,
// 2026-05-04). The orchestrator's eval/run.sh writes per-agent
// dispatch + per-iteration LLM-turn metrics by grepping diag-line
// substrings. Two invariants are load-bearing:
//
//  1. The "[diag <agent>] iter=N ASSISTANT content_len=" line is the
//     1:1 LLM-turn signal — must NOT carry phase=… (B6-F5 metric
//     contract).
//  2. Every other diag-line that includes "iter=N" MUST carry a
//     phase=<sub> token so operators grepping `iter=` get a
//     disambiguatable stream (R5.2 / R12 fix; before the rewrite
//     `iter=40` was misread as a single ReAct loop when it was 4
//     dispatches × ~10 iters).
//
// We pin the source-text contract directly (cheap, deterministic,
// platform-independent) rather than spinning up the agent loop.

func TestDiagPhaseFormat_AgentSourceContract(t *testing.T) {
	src, err := os.ReadFile("agent.go")
	if err != nil {
		t.Fatalf("read agent.go: %v", err)
	}
	body := string(src)

	// Required phase= tags: at least one diag-line for each sub-phase
	// must exist.
	requiredPhases := []string{
		"phase=cancel",
		"phase=prune",
		"phase=toolcall",
		"phase=embedded_correction",
		"phase=softstop_signal",
		"phase=softstop_inject",
		"phase=toolresult",
		"phase=midloop_signal",
		"phase=midloop_inject",
		"phase=midloop_force_stop",
	}
	for _, p := range requiredPhases {
		if !strings.Contains(body, p) {
			t.Errorf("agent.go missing required diag tag %q — operators rely on phase=<sub> to disambiguate iter=N grep streams (R5.2/R12 contract)", p)
		}
	}

	// ASSISTANT content_len line is the LLM-turn metric (B6-F5).
	// Must NOT carry phase= — eval/run.sh metric assumes 1:1.
	assistantLine := `logging.Debug("[diag %s] iter=%d ASSISTANT content_len=%d tool_calls=%d",`
	if !strings.Contains(body, assistantLine) {
		t.Fatalf("agent.go missing canonical ASSISTANT diag line — B6-F5 metric contract broken")
	}
	// Sniff: no `phase=` token within 40 bytes after the assistant
	// header substring (precludes accidental tagging).
	idx := strings.Index(body, assistantLine)
	tail := body[idx : idx+len(assistantLine)+40]
	if strings.Contains(tail, "phase=") {
		t.Errorf("ASSISTANT content_len diag line MUST NOT carry phase=… (load-bearing for B6-F5 LLM-turn metric)")
	}
}

func TestDiagPhaseFormat_OrchestratorDispatchMarker(t *testing.T) {
	src, err := os.ReadFile("../orchestrator/orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	body := string(src)
	want := `logging.Debug("[diag %s] DISPATCH stage=%s attempt=%d",`
	if !strings.Contains(body, want) {
		t.Errorf("orchestrator.go missing per-dispatch DISPATCH marker — eval/run.sh's <agent>_dispatches metric depends on this exact format (R12 contract)")
	}
}
