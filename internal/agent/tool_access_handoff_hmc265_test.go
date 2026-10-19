package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC265PublicHandoffSeparatesReadDiscoveryAndUnavailable(t *testing.T) {
	ctx := boundedRuntimeReaderHandoffTestContext()
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "capture.systrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\nworker-1 (1) [000] .... 1.000000: tracing_mark_write: I|1|one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	bus := types.ToolBusContext(ctx, types.AgentExplorer)
	args, _ := json.Marshal(map[string]any{"path": path})
	read, err := (&tool.ReadFile{}).Execute(bus, args)
	if err != nil || !read.Success || read.RuntimeArtifactRead == nil {
		t.Fatalf("read fixture: %v %+v", err, read)
	}
	args, _ = json.Marshal(map[string]any{"root": outside})
	discovery, err := (&tool.TraceCatalog{}).Execute(bus, args)
	if err != nil || discovery.Success {
		t.Fatalf("discovery fixture: %v %+v", err, discovery)
	}
	unavailable := unknownToolResult(ctx, llm.ToolCall{Name: "trace_query"})
	results := []types.ToolResult{read, discovery, *unavailable}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, HandoffCarriers: types.ToolHandoffCarriersFromTurnAInputs(results, nil, nil)})
	for name, prompt := range map[string]string{
		"finalizer": hmc265ReadinessFinalizerPrompt(t, ctx),
		"extractor": (&extractorEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{"Historical tool access observations", "artifact_bytes_read", path, "tool_not_executed", "directory_scope_refused", "not file permission or decoder failures", "not current availability"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("shared boundary missing %q", want)
				}
			}
		})
	}
}

func TestHMC265ClosureAbsenceIsAdvisoryAndNotDuplicated(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("inspect captures"), AnalysisIR: &types.AnalysisIR{}}
	reason := "model statement: material unavailable"
	ctx.Mutable.SetInvestigationComplete(reason)
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.SetAbsenceJustification(reason)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{AcceptedClosureReason: reason, AcceptedResultKind: "absence"})
	for name, prompt := range map[string]string{
		"finalizer": renderAnswerDocAcceptedClosure(ctx),
		"extractor": renderExtractorAcceptedClosure(ctx, ctx.Mutable.TurnAArtifacts()),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(prompt, "accepted bounded absence / no-hit") {
				t.Fatal("model absence upgraded to verified no-hit")
			}
			if strings.Count(prompt, reason) != 1 {
				t.Fatalf("repeated advisory narrative: %s", prompt)
			}
			if !strings.Contains(prompt, "advisory only") || !strings.Contains(prompt, "not verified no-hit") {
				t.Fatalf("absence lacks explicit scope: %s", prompt)
			}
		})
	}
	if ctx.Mutable.StableInvestigationResultKind() != "absence" || ctx.Mutable.StableAbsenceJustification() != reason {
		t.Fatal("display mutated completion state")
	}
}
