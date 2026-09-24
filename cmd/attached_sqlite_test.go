package cmd

import (
	"context"
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

func TestLoadPreparedAttachedSQLiteQueriesCompleteMaterial(t *testing.T) {
	resetPreparedTraceFlags(t)
	dir := t.TempDir()
	body, err := os.ReadFile("../eval/fixtures/hmosperf_existing_sqlite/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "trace.capture")
	if err := os.WriteFile(source, body, 0o400); err != nil {
		t.Fatal(err)
	}
	prepareAttachedTraceInput = func(ctx context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
		opts.RuntimeAnchor, opts.RuntimeAnchorFallback = filepath.Join(dir, "runtime"), ""
		return traceinput.Prepare(ctx, opts)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "no-converter"))
	flagAttachHitrace, maxAttachedTraceBytes = []string{source}, 640
	loaded, err := loadPreparedAttachedTrace(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.material == nil || loaded.material.SourcePath() != source || len(loaded.body) > 640 || strings.Contains(loaded.body, "tail-business-marker") {
		t.Fatalf("bad prepared envelope: %+v", loaded)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: loaded.body, AttachedTraceMaterial: loaded.material, Mutable: types.NewMutableState("read trace")}
	projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
	result, err := (&tool.TraceQuery{}).Execute(projected, json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"tail-business-marker","time_start":2.98,"time_end":3.0,"limit":10}`))
	if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-business-marker") {
		t.Fatalf("public SQLite query failed: %+v %v", result, err)
	}
}
