package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The unchanged offline operator validates actual normal-status output, not an
// action name or a snapshot picked from disk. This is deterministic public-tool
// integration only; it does not seed a live model's preparation or selection.
func TestHMC293OrdinaryStatusPassesOriginalRepresentativeAudit(t *testing.T) {
	out, prep := fixture(t)
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: prep.Workspace, WorkDir: filepath.Join(out, "normal-status-run"), Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "prepared")})}
	initial := invokeCatalog(t, bus, map[string]any{"root": "captures", "queries": []map[string]any{
		{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05},
		{"view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05},
	}, "representatives": map[string]any{"per_cluster": 2, "max_clusters": 2}})
	if len(initial.Calls) != 4 {
		t.Fatalf("expected four actual selected calls: %d", len(initial.Calls))
	}
	if err := audit(context.Background(), auditInput{out, initial.SavedIndex, initial.Selection, initial.SelectionSHA}); err == nil {
		t.Fatal("unexecuted selection passed original completion checker")
	}
	ids := make([]string, 0, len(initial.Calls))
	for id := range initial.Calls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result, err := (&tool.TraceQuery{}).Execute(bus, initial.Calls[id])
		if err != nil || !result.Success {
			t.Fatalf("selected native query: %v %s", err, result.Summary)
		}
	}
	current := invokeCatalog(t, bus, map[string]any{"action": "status", "catalog_id": initial.CatalogID})
	if current.Selection == "" || current.SelectionSHA == "" || current.SelectionID != initial.SelectionID {
		t.Fatal("ordinary status lacks current fixed selection delivery")
	}
	if err := audit(context.Background(), auditInput{out, current.SavedIndex, initial.Selection, initial.SelectionSHA}); err == nil {
		t.Fatal("initial pending selection paired with final index passed original checker")
	}
	index, err := tracecatalog.Load(context.Background(), current.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(current.Selection)
	if err != nil {
		t.Fatal(err)
	}
	var selection selectionDocument
	if err := json.Unmarshal(body, &selection); err != nil {
		t.Fatal(err)
	}
	if len(index.Queries) != 8 || len(selection.Clusters) != 2 || selection.Coverage.Selected != 4 || selection.Coverage.BudgetNotSelected != 4 {
		t.Fatal("original exact 8/2/4/4 selection matrix changed")
	}
	if err := auditSnapshots(prep, index, selection); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), auditInput{out, current.SavedIndex, current.Selection, current.SelectionSHA}); err != nil {
		t.Fatalf("actual normal status pair rejected by unchanged offline checker: %v", err)
	}
}
