package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC303CLIPhysicalPCAPPreparationAndDefaultQuery(t *testing.T) {
	resetPreparedTraceFlags(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "normal.capture")
	var b bytes.Buffer
	for _, v := range []any{uint32(0xa1b2c3d4), uint16(2), uint16(4), int32(0), uint32(0), uint32(65535), uint32(147), uint32(5), uint32(2), uint32(3), uint32(5)} {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	b.Write([]byte{1, 2, 3})
	original := b.Bytes()
	if err := os.WriteFile(path, original, 0400); err != nil {
		t.Fatal(err)
	}
	calls := 0
	prepareAttachedTraceInput = func(ctx context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
		calls++
		opts.RuntimeAnchor = filepath.Join(dir, "runtime")
		opts.RuntimeAnchorFallback = ""
		return traceinput.Prepare(ctx, opts)
	}
	flagAttachHitrace = []string{path}
	loaded, err := loadPreparedAttachedTrace(t.Context(), nil)
	if err != nil || loaded.material == nil || calls != 1 || loaded.material.QueryPath() != path || !strings.Contains(loaded.body, "packet_inventory") || !strings.Contains(loaded.body, "epoch") {
		t.Fatalf("CLI did not retain original packet input: %+v %v calls=%d", loaded, err, calls)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: loaded.body, AttachedTraceMaterial: loaded.material, Mutable: types.NewMutableState("inspect packet capture")}
	projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	r, err := (&tool.TraceQuery{}).Execute(projected, json.RawMessage(`{"view":"packet_inventory"}`))
	if err != nil || !r.Success || len(r.Observations) != 1 || r.Observations[0].SourceRef.TimeDomain != "pcap_epoch_seconds" {
		t.Fatalf("default source lost CLI original: %+v %v", r, err)
	}
	projected.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	choices := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInputFromBusContext(projected, 0)).Choices()
	if len(choices) != 3 {
		t.Fatal("CLI native selectors absent", choices)
	}
	for _, choice := range choices {
		if !choice.HasNativeRawFacts() {
			t.Fatal("CLI raw display not source bound")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("CLI modified original capture")
	}
}
