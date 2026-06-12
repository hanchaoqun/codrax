package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestTypedDenials_ToolStampsReachOrchestratorBus pins the
// negative-knowledge channel's write-side contract end-to-end through
// the PRODUCTION projection chain, which per-tool unit tests bypass
// (they pass the orchestrator-shaped BusContext directly to Execute):
//
//	orchestrator BusContext
//	  → BuildAgentContext            (AgentContext.TypedDenials wired to bus)
//	  → types.ToolBusContext         (per-tool-call narrowed BusContext)
//	  → EmitLogTriage.Execute        (stamps a denial for the cleared frame)
//	  → assert bus-level visibility  (L3 contract_check reads the bus set)
//	  → types.ToolBusContext again   (next tool call in the same dispatch)
//	  → ReadFile.Execute             (L1 gate must refuse the denied path)
//
// The contract being pinned is the one already documented on
// BuildAgentContext ("when a tool call mid-dispatch stamps a fresh
// denial, subsequent calls in the loop see it") and on the
// BusContext.TypedDenials field doc (orchestrator-owned channel that
// the L1/L2/L3 enforcement surfaces consume).
func TestTypedDenials_ToolStampsReachOrchestratorBus(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "internal", "svc", "handler.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package svc\n\nfunc Handler() error { return nil }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bus := &types.BusContext{
		RepoRoot:      dir,
		PipelineStage: types.StageLogTriage,
		// Mirrors orchestrator.Run's Run-entry allocation — the
		// single Run-level set every projection shares by pointer.
		TypedDenials: types.NewTypedDenialSet(),
		Mutable:      types.NewMutableState("why does svc.Handler panic"),
		AttachedLog: "panic: runtime error\n\n" +
			"goroutine 1 [running]:\nsvc.Handler(\"/x\")\n" +
			"\tinternal/svc/handler.go:42 +0x1e\n" +
			"fake.Nope()\n\tinternal/fake/nope.go:10 +0x2a\n",
	}

	agentCtx := promptctx.BuildAgentContext(bus, types.AgentName("log_triager"), types.StageLogTriage)
	toolBus := types.ToolBusContext(agentCtx, types.AgentName("log_triager"))

	params, err := json.Marshal(emitLogTriageParams{
		Meta: emitLogTriageMeta{
			Lang:    "go",
			Signals: []string{"panic"},
			Summary: "runtime error in svc.Handler",
		},
		Errors: []emitLogTriageError{
			{
				Type: "runtime error: invalid memory address",
				Frames: []emitLogTriageFrame{
					{File: "internal/svc/handler.go", Line: 42,
						Func: "svc.Handler", Raw: "real frame", Confidence: 0.9},
					// This frame's File does not exist in the repo: the
					// validate pass clears it, ValidateBundleWithDenials
					// reports it as a ClearedFrame, and Execute stamps a
					// TypedDenialExternalLogFrameUnresolved denial.
					{File: "internal/fake/nope.go", Line: 10,
						Func: "fake.Nope", Raw: "fake frame", Confidence: 0.9},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	emitTool := &EmitLogTriage{}
	res, err := emitTool.Execute(toolBus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("emit_log_triage failed: %s", res.Summary)
	}

	// Control for the bug class: Mutable is a shared POINTER through
	// the projection, so the bundle write-through must always work.
	// If this fails the fixture itself is broken, not the channel.
	if bus.Mutable.LogTriage() == nil {
		t.Fatal("fixture broken: LogBundle did not reach the orchestrator bus through Mutable")
	}

	// The write-side contract: the denial stamped during Execute must
	// be visible on the ORCHESTRATOR-level set — that is what the L3
	// answer validator (contract_check → runDeniedTokenAnswerCheck)
	// and every later BuildAgentContext mirror read.
	if got := bus.TypedDenials.Len(); got == 0 {
		t.Fatalf("tool-stamped typed denial never reached the orchestrator BusContext: "+
			"the per-call ToolBusContext projection dropped it (value-copy boundary); "+
			"L1/L2/L3 enforcement surfaces all read an empty set (len=%d)", got)
	}
	if !bus.TypedDenials.IsPathDenied("internal/fake/nope.go") {
		t.Fatalf("orchestrator bus denial set does not deny the cleared frame path; set=%+v",
			bus.TypedDenials.PathTokens())
	}

	// L1 leg — "subsequent calls in the same loop see it": the next
	// tool call's fresh projection must refuse a read of the denied
	// path with the external-log refusal prose, not a plain not-found.
	toolBus2 := types.ToolBusContext(agentCtx, types.AgentName("log_triager"))
	readTool := &ReadFile{}
	res2, err := readTool.Execute(toolBus2, json.RawMessage(`{"path":"internal/fake/nope.go"}`))
	if err != nil {
		t.Fatalf("read_file returned transport error instead of typed refusal: %v", err)
	}
	if res2.Success {
		t.Fatal("read_file succeeded on a denied path — L1 gate did not fire")
	}
	if !strings.Contains(res2.Summary, "attached runtime log") {
		t.Errorf("read_file refusal is not the typed external-log refusal; summary=%q", res2.Summary)
	}
}
