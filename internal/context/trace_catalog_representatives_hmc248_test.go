package context_test

import (
	"bytes"
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

func TestHMC248RepresentativeActualQueryComparisonContext(t *testing.T) {
	root, work := t.TempDir(), t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.data", "two.data", "three.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	bus := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("Compare representative captures"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	discovered, err := (&tool.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}]}`))
	if err != nil || !discovered.Success {
		t.Fatalf("discover: %v %s", err, discovered.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	raw, _ := json.Marshal(map[string]any{"action": "representatives", "catalog_id": c.ID(), "representatives": map[string]any{"per_cluster": 1}})
	selected, err := (&tool.TraceCatalog{}).Execute(bus, raw)
	if err != nil || !selected.Success {
		t.Fatalf("selection: %v %s", err, selected.Summary)
	}
	var page struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(selected.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Calls) != 2 {
		t.Fatal("selected calls missing")
	}
	var results []types.ToolResult
	for _, call := range page.Calls {
		r, e := (&tool.TraceQuery{}).Execute(bus, call)
		if e != nil || !r.Success {
			t.Fatalf("query: %v %s", e, r.Summary)
		}
		results = append(results, r)
	}
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	before, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0)))
	current, _ := c.Representatives()
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		pc := ctxbuilder.BuildPromptContext(ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage), &skill.Config{Name: "representative-test"})
		section := artifactDenialSection(t, pc, ctxbuilder.SectionRuntimeArtifactChoice)
		for _, want := range []string{current.SelectionID, `"members":6`, `"selected":2`, `"budget_not_selected":4`, `"success":1`, `"empty":1`, `"not_executed":4`, `"kind":"thread"`, `"start_ns":10000000000`, `"end_ns":10050000000`, "Representative comparison", "representative_status"} {
			if !strings.Contains(section, want) {
				t.Errorf("%s comparison lost %q: %s", stage, want, section)
			}
		}
		for _, g := range current.Clusters {
			for _, m := range g.Members {
				if m.Selected {
					for _, want := range []string{m.QueryID, m.Path, m.ArtifactRevision, m.LatestAttempt.PayloadRef, m.LatestAttempt.SourceRevision} {
						if want == "" || !strings.Contains(section, want) {
							t.Errorf("%s source-local current member lost %q", stage, want)
						}
					}
				}
			}
		}
		if strings.Contains(section, "query_records_displayed=") || strings.Contains(section, "ImageCache::reserve") {
			t.Fatal("comparison duplicated flat rows or manufactured runtime values")
		}
	}
	after, _ := json.Marshal(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0)))
	if !bytes.Equal(before, after) {
		t.Fatal("representative context mutated measurement ledger")
	}
}

func TestHMC248RepresentativeMultipleCatalogContextBudget(t *testing.T) {
	bus := &types.BusContext{RepoRoot: t.TempDir(), Mutable: types.NewMutableState("compare navigation")}
	for i := 0; i < 8; i++ {
		root := filepath.Join(bus.RepoRoot, fmt.Sprintf("capture-%03d", i))
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "capture.data"), []byte("source"), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := tracecatalog.Discover(context.Background(), root, tracecatalog.DiscoverOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range c.Snapshot().Artifacts {
			for j := 0; j < 5; j++ {
				parameters, _ := json.Marshal(map[string]any{"view": "event_search", "pid": 101 + j, "pattern": strings.Repeat("long-legitimate-filter", 50)})
				if _, err := c.RegisterPlannedQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "thread", ID: fmt.Sprint(101 + j)}, View: "event_search", Parameters: parameters}); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := c.SelectRepresentatives(context.Background(), tracecatalog.RepresentativeOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := bus.Mutable.InstallTraceCatalog(c); err != nil {
			t.Fatal(err)
		}
	}
	pc := ctxbuilder.BuildPromptContext(ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize), &skill.Config{Name: "representative-budget-test"})
	section := artifactDenialSection(t, pc, ctxbuilder.SectionRuntimeArtifactChoice)
	if len(section) > 24<<10 || !strings.Contains(section, "Additional catalog navigation omitted") || !strings.Contains(section, "Representative comparison") {
		t.Fatalf("multi-catalog selection escaped context budget: bytes=%d tail=%s", len(section), section[max(0, len(section)-600):])
	}
	if strings.Count(section, "catalog_id=") >= 8 {
		t.Fatal("navigation silently claimed all catalogs shown")
	}
}
