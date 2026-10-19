package context_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc259CatalogBus(t *testing.T) *types.BusContext {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.data", "two.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	bus := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("Inspect capture members"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	out, err := (&tool.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !out.Success {
		t.Fatalf("discover: %v %s", err, out.Summary)
	}
	return bus
}

func hmc259CatalogContext(t *testing.T, bus *types.BusContext, stage types.PipelineStage) string {
	t.Helper()
	ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
	pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "catalog-handoff-test"})
	for _, section := range pc.UserSections {
		if section.Title == ctxbuilder.SectionRuntimeArtifactChoice {
			return section.Content
		}
	}
	return ""
}

func hmc259Calls(t *testing.T, content string) map[string]json.RawMessage {
	t.Helper()
	calls := map[string]json.RawMessage{}
	for _, line := range strings.Split(content, "\n") {
		var row struct {
			Query struct {
				ID string `json:"id"`
			} `json:"query"`
			Member struct {
				ID string `json:"query_id"`
			} `json:"selected_member"`
			Call json.RawMessage `json:"query_call"`
		}
		if json.Unmarshal([]byte(line), &row) != nil || len(row.Call) == 0 {
			continue
		}
		id := row.Query.ID
		if id == "" {
			id = row.Member.ID
		}
		calls[id] = row.Call
	}
	return calls
}

func TestHMC259LongAttemptHistoryDoesNotHidePendingQueryHandoff(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	queries := c.Snapshot().Queries
	// Valid producer failure history is deliberately larger than the whole
	// context budget. The saved history must survive, but not evict other work.
	for i := 0; i < 40; i++ {
		_, err := c.Complete(context.Background(), queries[0].ID, tracecatalog.Completion{Outcome: tracecatalog.OutcomeFailure, Error: &tracecatalog.Issue{Code: "decoder_failed", Message: strings.Repeat("x", 3000)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	before, _ := json.Marshal(c.Snapshot())
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if !strings.Contains(content, queries[3].ID) {
			t.Fatalf("%s: legal retry history evicted later pending query %s", stage, queries[3].ID)
		}
		if len(content) > 24<<10 {
			t.Fatalf("context exceeded unchanged budget: %d", len(content))
		}
		if len(hmc259Calls(t, content)) != 4 || !strings.Contains(content, `"attempt_count":40`) || !strings.Contains(content, `"not_executed":3`) {
			t.Fatalf("lost current exact calls/counts: %s", content)
		}
		if strings.Contains(content, strings.Repeat("x", 3000)) {
			t.Fatal("full failed-history message repeated into navigation")
		}
	}
	after, _ := json.Marshal(c.Snapshot())
	if string(before) != string(after) {
		t.Fatal("projection mutated failure history")
	}
}

func TestHMC259ContextQueryCallsExecuteExactCurrentNativePlans(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	content := hmc259CatalogContext(t, bus, types.StageExplore)
	calls := hmc259Calls(t, content)
	if len(calls) != 4 {
		t.Fatalf("current plans lost executable handoff: %s", content)
	}
	for _, q := range c.Snapshot().Queries {
		out, err := (&tool.TraceQuery{}).Execute(bus, calls[q.ID])
		if err != nil || !out.Success {
			t.Fatalf("native query: %v %s", err, out.Summary)
		}
		if got, _ := c.Query(q.ID); len(got.Attempts) != 1 || got.Outcome == tracecatalog.OutcomeNotExecuted {
			t.Fatal("handoff did not execute exact query")
		}
	}
	content = hmc259CatalogContext(t, bus, types.StageFinalize)
	for _, want := range []string{`"attempts":4`, `"success":2`, `"empty":2`, `"query_origin":"declared_current_plan"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %s: %s", want, content)
		}
	}
	// A copied current call never restores private live state in another run.
	bus.Mutable = types.NewMutableState("unrelated request")
	for _, call := range calls {
		out, err := (&tool.TraceQuery{}).Execute(bus, call)
		if err != nil || out.Success {
			t.Fatalf("foreign reference gained authority: %v %+v", err, out)
		}
		break
	}
	if got := hmc259CatalogContext(t, bus, types.StageFinalize); strings.Contains(got, "current_execution=") {
		t.Fatal("history became current catalog")
	}
}

func TestHMC259OversizedWholePlanDoesNotStarveOtherCurrentWork(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	a := c.Snapshot().Artifacts[0]
	params, _ := json.Marshal(map[string]any{"view": "event_search", "pattern": strings.Repeat("long", 7800)})
	large, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "capture", ID: "all"}, View: "event_search", Parameters: params})
	if err != nil {
		t.Fatal(err)
	}
	content := hmc259CatalogContext(t, bus, types.StageExplore)
	if len(content) > 24<<10 || len(hmc259Calls(t, content)) != 4 {
		t.Fatalf("large legitimate plan starved small plans: %s", content)
	}
	if strings.Contains(content, large.ID) || strings.Contains(content, strings.Repeat("long", 20)) {
		t.Fatal("oversized row was partially published")
	}
	for _, want := range []string{`"declared_plans":5`, "query_records_displayed=4 omitted=1", "execution_groups_displayed=2 omitted=1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing complete omission accounting %s: %s", want, content)
		}
	}
	if q, _ := c.Query(large.ID); string(q.Parameters) != string(params) {
		t.Fatal("display clipped stored plan")
	}
}

func TestHMC259RepresentativeHandoffKeepsUnselectedAndLaterPlansSeparate(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	selection, err := c.SelectRepresentatives(context.Background(), tracecatalog.RepresentativeOptions{PerCluster: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := c.Snapshot().Artifacts[0]
	extra, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "thread", ID: "202"}, View: "resource_stack", Parameters: json.RawMessage(`{"view":"resource_stack","pid":202,"time_start":10,"time_end":10.05}`)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if len(hmc259Calls(t, content)) != 3 || !strings.Contains(content, extra.ID) || !strings.Contains(content, `"new_queries_not_in_selection":1`) || !strings.Contains(content, "unselected_member_records_displayed=2 omitted=0") {
			t.Fatalf("lost independent work/state: %s", content)
		}
		for _, g := range selection.Clusters {
			for _, m := range g.Members {
				if !strings.Contains(content, m.QueryID) {
					t.Fatalf("member hidden: %s", m.QueryID)
				}
			}
		}
		if strings.Contains(content, "query_records_displayed=") {
			t.Fatal("flat full roster duplicated representative projection")
		}
	}
	current, _ := c.Representatives()
	if current.SelectionID != selection.SelectionID || current.Coverage.Selected != 2 {
		t.Fatal("context changed fixed selection")
	}
	for _, stage := range []types.PipelineStage{types.StageAnalyze, types.StagePlan} {
		if strings.Contains(hmc259CatalogContext(t, bus, stage), c.ID()) {
			t.Fatal("navigation expanded analyzer/write task surface")
		}
	}
}

func TestHMC259DenseRepresentativePreviewReservesIndependentPendingWork(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	for _, a := range c.Snapshot().Artifacts {
		for i := 0; i < 24; i++ {
			params, _ := json.Marshal(map[string]any{"view": "event_search", "pattern": fmt.Sprint(i) + strings.Repeat("large-filter", 80)})
			_, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "capture", ID: "all"}, View: "event_search", Parameters: params})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	selection, err := c.SelectRepresentatives(context.Background(), tracecatalog.RepresentativeOptions{PerCluster: 1, MaxClusters: 50})
	if err != nil {
		t.Fatal(err)
	}
	a := c.Snapshot().Artifacts[0]
	extra, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "thread", ID: "202"}, View: "resource_stack", Parameters: json.RawMessage(`{"view":"resource_stack","pid":202}`)})
	if err != nil {
		t.Fatal(err)
	}
	content := hmc259CatalogContext(t, bus, types.StageExplore)
	if len(content) > 24<<10 || !strings.Contains(content, extra.ID) || !strings.Contains(content, "outside_selection_queries_displayed=1 omitted=0") {
		t.Fatalf("representative preview starved new work (%d bytes): %s", len(content), content)
	}
	if !strings.Contains(content, `"members":52`) || !strings.Contains(content, `"selected":26`) || !strings.Contains(content, `"declared_plans":53`) {
		t.Fatal("full and preview populations mixed")
	}
	current, _ := c.Representatives()
	if current.SelectionID != selection.SelectionID {
		t.Fatal("projection reselected members")
	}
}

func TestHMC259ContextHandleRevalidatesChangedSourceAndOmitsStaleCall(t *testing.T) {
	bus := hmc259CatalogBus(t)
	c := bus.Mutable.TraceCatalogs()[0]
	q := c.Snapshot().Queries[0]
	calls := hmc259Calls(t, hmc259CatalogContext(t, bus, types.StageExplore))
	var path string
	for _, a := range c.Snapshot().Artifacts {
		if a.ID == q.ArtifactID {
			path = a.Path
		}
	}
	if err := os.WriteFile(path, []byte("source changed"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := (&tool.TraceQuery{}).Execute(bus, calls[q.ID])
	if err != nil || out.Success {
		t.Fatalf("navigation bypassed generation admission: %v %+v", err, out)
	}
	content := hmc259CatalogContext(t, bus, types.StageFinalize)
	if _, present := hmc259Calls(t, content)[q.ID]; present {
		t.Fatal("stale source offered current executable navigation")
	}
	if !strings.Contains(content, q.ID) || !strings.Contains(content, `"source_status":"stale"`) {
		t.Fatal("stale status disappeared instead of being disclosed")
	}
}
