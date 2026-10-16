package context_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC250ObservedQueriesDoNotBecomeExpectedExecutionRoster(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(root, "capture.data"), filepath.Join(root, "nested", "capture.data")}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	bus := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("inspect captures"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	call := func(params map[string]any) types.ToolResult {
		t.Helper()
		raw, _ := json.Marshal(params)
		r, err := (&tool.TraceCatalog{}).Execute(bus, raw)
		if err != nil || !r.Success {
			t.Fatalf("catalog: %v %s", err, r.Summary)
		}
		return r
	}
	call(map[string]any{"action": "discover"})
	for _, pair := range []struct {
		path string
		pid  int
	}{{paths[0], 101}, {paths[1], 101}, {paths[1], 999}} {
		raw, _ := json.Marshal(map[string]any{"source": "path", "path": pair.path, "view": "resource_stack", "pid": pair.pid, "time_start": 10, "time_end": 10.05})
		r, err := (&tool.TraceQuery{}).Execute(bus, raw)
		if err != nil || !r.Success {
			t.Fatalf("native query: %v %s", err, r.Summary)
		}
	}
	check := func(want []string) {
		t.Helper()
		for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
			ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
			pc := ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "execution-roster-test"})
			section := artifactDenialSection(t, pc, ctxbuilder.SectionRuntimeArtifactChoice)
			for _, value := range want {
				if !strings.Contains(section, value) {
					t.Errorf("%s missing %s: %s", stage, value, section)
				}
			}
			if strings.Contains(section, "ImageCache::reserve") || strings.Contains(section, "UIFrame::render") {
				t.Fatal("execution navigation acquired native names")
			}
		}
	}
	checkPage := func(origin, kind string, registered int) types.ToolResult {
		t.Helper()
		status := call(map[string]any{"action": "status", "catalog_id": bus.Mutable.TraceCatalogs()[0].ID()})
		var page struct {
			Registered int            `json:"registered_query_count"`
			Execution  map[string]any `json:"current_execution"`
			Records    []struct {
				Kind   string `json:"record_kind"`
				Origin string `json:"query_origin"`
			} `json:"records"`
		}
		if err := json.Unmarshal([]byte(status.Summary), &page); err != nil {
			t.Fatal(err)
		}
		if page.Registered != registered || page.Execution == nil {
			t.Fatalf("missing execution view: %s", status.Summary)
		}
		count := 0
		for _, row := range page.Records {
			if row.Origin == "" {
				continue
			}
			count++
			if row.Origin != origin || row.Kind != kind {
				t.Fatalf("query classification differs: %+v", row)
			}
		}
		if count != registered {
			t.Fatalf("query page lost rows: %d / %d", count, registered)
		}
		return status
	}
	check([]string{`"coverage_basis":"observed_queries_only"`, `"declared_plans":0`, `"resumed_plans":0`, `"observed_queries":3`, `"attempts":3`, `"query_origin":"observed_query_only"`, paths[0], paths[1]})
	checkPage("observed_query_only", "observed_query", 3)
	// Current explicit plans can include already observed calls, but the one
	// unqueried source/object remains pending, not empty or historical success.
	call(map[string]any{"action": "discover", "queries": []map[string]any{{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05}, {"view": "resource_stack", "pid": 999, "time_start": 10, "time_end": 10.05}}})
	check([]string{`"coverage_basis":"registered_current_plans"`, `"declared_plans":4`, `"resumed_plans":0`, `"observed_queries":0`, `"attempts":3`, `"not_executed":1`, `"query_origin":"declared_current_plan"`})
	// A new run may read the navigation snapshot but cannot acquire declaration
	// permission or current results from its historical execution counters.
	status := checkPage("declared_current_plan", "current_query_plan", 4)
	var saved struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(status.Summary), &saved); err != nil {
		t.Fatal(err)
	}
	bus.Mutable = types.NewMutableState("continue captures")
	bus.UserPinnedFiles = []string{saved.SavedIndex}
	call(map[string]any{"action": "resume", "root": root, "snapshot_path": saved.SavedIndex})
	check([]string{`"coverage_basis":"registered_current_plans"`, `"declared_plans":0`, `"resumed_plans":4`, `"observed_queries":0`, `"attempts":0`, `"not_executed":4`, `"query_origin":"resumed_current_plan"`})
	checkPage("resumed_current_plan", "current_query_plan", 4)
}
