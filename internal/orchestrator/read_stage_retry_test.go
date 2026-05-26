package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildExploreTransientRetryCheckpointHintIncludesTypedObservationOrigins(t *testing.T) {
	mut := types.NewMutableState("count generated tool files")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary: "[exec_command: $ find internal/tool -name '*.go' | wc -l]\n" +
			"[exec_command: evidence_origin=command_measurement measurement=count]\n" +
			"140\n",
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}

	got := o.buildExploreTransientRetryCheckpointHint()
	for _, want := range []string{
		"Checkpoint summary",
		"typed observation origins=command_measurement:1",
		"successful tool results=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("checkpoint hint missing %q:\n%s", want, got)
		}
	}
}
