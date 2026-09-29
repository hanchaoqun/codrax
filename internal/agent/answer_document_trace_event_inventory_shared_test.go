package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceEventInventoryOverlappingPublicQueriesKeepCompletionEndpoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlap.systrace")
	var source strings.Builder
	for n := 0; n < 7; n++ {
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: B|42|AppStartup\n", 10+n*10)
		fmt.Fprintf(&source, "app-42 [000] .... 1.%03d000: tracing_mark_write: E|42\n", 14+n*10)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = dir, dir
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	var results []types.ToolResult
	for n := 0; n < 9; n++ {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 1.0, "time_end": 1.1 + float64(n)/100, "limit": 40})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 9 {
		t.Fatalf("receipts=%d", len(views))
	}
	for _, view := range views {
		var original types.ObservationRecord
		for _, record := range ledger.Records {
			if record.ID == view.ObservationID {
				original = record
			}
		}
		if original.EventSearchInventory == nil || !reflect.DeepEqual(view.Source, original.SourceRef) ||
			!reflect.DeepEqual(view.Inventory, original.EventSearchInventory) || view.PromptRowsOmitted != 0 || view.PromptRowsShown != 14 {
			t.Fatalf("overlap lost complete member list / completion endpoint: %+v", view)
		}
	}
	if !strings.Contains(prompt, "prompt_member_rows=14/32") || len(prompt) > traceEventInventoryPromptByteLimit {
		t.Fatal("repeated members consumed display budget")
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("display sharing changed accepted receipts/causality")
	}
}

func TestTraceEventInventoryDisplaySharingKeepsProvenanceAndFullContent(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	base.EventSearchInventory.Rows = base.EventSearchInventory.Rows[:1]
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"carrier":     func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"capture":     func(r *types.ObservationRecord) { r.SourceRef.CaptureIdentityPath = "other" },
		"commit":      func(r *types.ObservationRecord) { r.SourceRef.Commit = "other" },
		"clock":       func(r *types.ObservationRecord) { r.SourceRef.TimeDomain = "other" },
		"offset":      func(r *types.ObservationRecord) { offset := 1.0; r.SourceRef.ClockOffsetSec = &offset },
		"source line": func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].LocalLine++ },
		"typed field": func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].EmitterTID++ },
		"raw tail":    func(r *types.ObservationRecord) { r.EventSearchInventory.Rows[0].Raw += "different tail" },
	} {
		t.Run(name, func(t *testing.T) {
			original := base
			original.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
			original.EventSearchInventory.Rows[0].Raw = strings.Repeat("x", 600)
			changed := original
			changed.EventSearchInventory = types.CloneTraceEventSearchInventory(original.EventSearchInventory)
			mutate(&changed)
			if got := traceEventInventoryMembers([]types.ObservationRecord{original, changed}); len(got.rows) != 2 {
				t.Fatal("nonidentical evidence shared a display object")
			}
		})
	}
	// Identical occurrences inside one result remain two members, even though
	// their display bytes can be shared. This must not become deduped counting.
	base.EventSearchInventory.Rows = append(base.EventSearchInventory.Rows, base.EventSearchInventory.Rows[0])
	members := traceEventInventoryMembers([]types.ObservationRecord{base})
	if len(members.rows) != 1 || len(members.refs[0]) != 2 || members.refs[0][0] != members.refs[0][1] {
		t.Fatal("sharing changed membership multiplicity")
	}
}
