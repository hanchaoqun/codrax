package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/textfmt"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func checkpointPreRead250(t *testing.T) (*types.BusContext, string, []byte) {
	t.Helper()
	bus := traceCapabilitiesDiscoveryBus(t, false)
	bus.WorkDir = t.TempDir()
	bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
	body := "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(filepath.Join(bus.RepoRoot, "capture.systrace"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	seed, err := (&toolpkg.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"window_stats","pid":42,"time_start":1,"time_end":1.02},{"view":"window_stats","pid":43,"time_start":1,"time_end":1.02}]}`))
	if err != nil || !seed.Success {
		t.Fatal(err, seed.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	var first tracecatalog.QueryRecord
	for _, q := range c.Snapshot().Queries {
		if q.Object.ID == "42" {
			first = q
		}
	}
	raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": first.ID}})
	queried, err := (&toolpkg.TraceQuery{}).Execute(bus, raw)
	if err != nil || !queried.Success {
		t.Fatal(err, queried.Summary)
	}
	raw, _ = json.Marshal(map[string]string{"action": "status", "catalog_id": c.ID()})
	status, err := (&toolpkg.TraceCatalog{}).Execute(bus, raw)
	if err != nil || !status.Success {
		t.Fatal(err, status.Summary)
	}
	var page struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(status.Summary), &page); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(page.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "checkpoint.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	bus.Mutable = types.NewMutableState("Continue the capture checks recorded in checkpoint.json.")
	bus.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{Complexity: types.ComplexityModerate,
			AnalyzerHints: types.AnalyzerHints{Kind: "mechanism", Entities: []string{"checkpoint.json"}, Keywords: []string{"checkpoint.json"}}},
		EvidencePlan: types.EvidencePlan{RequiredFiles: []string{"checkpoint.json"}},
	}
	return bus, path, data
}

func TestHMC250CheckpointPreReadBoundaries(t *testing.T) {
	for _, kind := range []string{"complete", "truncated", "excluded", "no_file_budget", "ordinary_json", "malformed", "duplicate_key", "unknown_version", "foreign_root", "foreign_checkpoint_symlink", "cancelled", "escaped_path", "long_path"} {
		t.Run(kind, func(t *testing.T) {
			bus, path, data := checkpointPreRead250(t)
			maxLines, maxFiles := 1000, 1
			var excluded map[string]bool
			switch kind {
			case "truncated":
				maxLines = len(textfmt.PhysicalLines(string(data))) - 1
			case "excluded":
				excluded = map[string]bool{"checkpoint.json": true}
			case "no_file_budget":
				maxFiles = 0
			case "ordinary_json":
				data = []byte(`{"action":"resume","root":"/tmp","success":true}`)
			case "malformed":
				data = data[:len(data)/2]
			case "duplicate_key":
				data = append([]byte(`{"navigation_only":true,`), data[1:]...)
			case "unknown_version":
				data = []byte(strings.Replace(string(data), `"schema_version": 1`, `"schema_version": 999`, 1))
			case "foreign_root":
				bus.RepoRoot = t.TempDir()
				path = filepath.Join(bus.RepoRoot, "checkpoint.json")
			case "foreign_checkpoint_symlink":
				foreign := filepath.Join(t.TempDir(), "checkpoint.json")
				if err := os.WriteFile(foreign, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, path); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				bus.Ctx = ctx
			case "escaped_path":
				path = filepath.Join(bus.RepoRoot, "state \"quoted\"\\view.json")
			case "long_path":
				path = filepath.Join(bus.RepoRoot, strings.Repeat("segment", 18), strings.Repeat("nested", 18), "checkpoint.json")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if kind != "foreign_checkpoint_symlink" {
				// Preserve a final LF in the raw digest, not a reconstructed gutter.
				data = append(data, '\n')
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			rel, err := filepath.Rel(bus.RepoRoot, path)
			if err != nil {
				t.Fatal(err)
			}
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
			got := preReadRequiredFilesTracked(ctx, bus.RepoRoot, []string{rel}, maxFiles, maxLines, excluded)
			const prefix = "Checkpoint navigation (not query evidence): "
			var line string
			for _, v := range strings.Split(got, "\n") {
				if strings.HasPrefix(v, prefix) {
					line = v
				}
			}
			wantNav := kind == "complete" || kind == "foreign_root" || kind == "foreign_checkpoint_symlink" || kind == "escaped_path" || kind == "long_path"
			if (line != "") != wantNav {
				t.Fatalf("navigation eligibility %s: %s", kind, got)
			}
			if kind == "truncated" && !strings.Contains(got, "first ") {
				t.Fatal("not an actual truncated display")
			}
			if !wantNav {
				return
			}
			var nav struct {
				NextCall       map[string]string `json:"next_call"`
				SHA            string            `json:"snapshot_sha256"`
				NavigationOnly bool              `json:"navigation_only"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &nav); err != nil {
				t.Fatal(err)
			}
			if !nav.NavigationOnly || nav.SHA != fmt.Sprintf("%x", sha256.Sum256(data)) {
				t.Fatal("navigation lost exact original bytes/role")
			}
			wantAction := kind != "foreign_root" && kind != "foreign_checkpoint_symlink"
			if (nav.NextCall != nil) != wantAction {
				t.Fatalf("authorization mismatch: %s", line)
			}
			if wantAction {
				canonical, err := filepath.EvalSymlinks(path)
				if err != nil {
					t.Fatal(err)
				}
				if nav.NextCall["snapshot_path"] != canonical {
					t.Fatal("path was truncated or reparsed")
				}
				if kind == "long_path" && strings.Contains(got, `preferred_tool="trace_catalog"`) {
					t.Fatal("long path entered capped generic parameters")
				}
			}
			if kind == "complete" {
				raw, _ := json.Marshal(map[string]string{"path": path})
				read, err := (&toolpkg.ReadFile{}).Execute(bus, raw)
				var readLine string
				for _, candidate := range strings.Split(read.Summary, "\n") {
					if strings.HasPrefix(candidate, prefix) {
						readLine = candidate
					}
				}
				if err != nil || !read.Success || readLine != line {
					t.Fatalf("read_file and pre-read navigation diverged: %v %s", err, read.Summary)
				}
			}
			if len(ctx.Mutable.DispatchToolResults()) != 0 || len(ctx.Mutable.TraceCatalogs()) != 0 {
				t.Fatal("display installed authority")
			}
			if !strings.Contains(got, "historical_results_not_current_execution_evidence") {
				t.Fatal("lost historical qualification")
			}
		})
	}
}

func TestHMC250CheckpointPreReadCancellationAtPublication(t *testing.T) {
	bus, _, _ := checkpointPreRead250(t)
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus.Ctx = cancelCtx
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	got := preReadRequiredFilesWithObserver(bus.RepoRoot, []string{"checkpoint.json"}, 1, 1000, nil,
		func(string, int, []string) { cancel() }, preReadCatalogNavigation(ctx))
	if !strings.Contains(got, "full file") {
		t.Fatal("file was not shown before cancellation")
	}
	if strings.Contains(got, "Checkpoint navigation (not query evidence):") {
		t.Fatal("cancelled publication issued navigation")
	}
}

func TestHMC250CheckpointAutomaticPreReadDoesNotNavigateHiddenTail(t *testing.T) {
	bus, path, data := checkpointPreRead250(t)
	data = append(data, []byte(strings.Repeat("\n", 250))...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture truncated pre-read")}
	_, err := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual dispatch: %v", err)
	}
	var prompt strings.Builder
	for _, m := range capture.messages {
		prompt.WriteString(m.Content)
	}
	if !strings.Contains(prompt.String(), "first 200 of") {
		t.Fatal("actual pre-read cap did not trigger")
	}
	if strings.Contains(prompt.String(), "Checkpoint navigation (not query evidence):") {
		t.Fatal("hidden bytes published an executable action")
	}
}

func TestHMC250CheckpointAutomaticPreReadActualExplorer(t *testing.T) {
	bus, _, _ := checkpointPreRead250(t)
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture actual automatic pre-read")}
	explorer := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := explorer.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("actual dispatch: %v calls=%d", err, capture.calls)
	}
	var prompt strings.Builder
	for _, m := range capture.messages {
		prompt.WriteString(m.Content)
		prompt.WriteString("\n")
	}
	text := prompt.String()
	if !strings.Contains(text, "Pre-read File Content") || !strings.Contains(text, "#### `checkpoint.json`") || strings.Contains(text, "showing first") {
		t.Fatalf("fixture did not fully pre-read checkpoint: %s", text)
	}
	const prefix = "Checkpoint navigation (not query evidence): "
	var next json.RawMessage
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			var nav struct {
				NextCall         json.RawMessage `json:"next_call"`
				CurrentExecution string          `json:"current_execution"`
				Historical       struct {
					Outcomes map[string]int `json:"plan_outcomes"`
				} `json:"historical_snapshot"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &nav); err != nil {
				t.Fatal(err)
			}
			if nav.CurrentExecution != "not_restored" || nav.Historical.Outcomes["success"] != 1 {
				t.Fatalf("historical/current roles: %s", line)
			}
			next = nav.NextCall
		}
	}
	if len(next) == 0 {
		t.Fatal("full automatic pre-read omitted the shared typed checkpoint navigation")
	}
	if !strings.Contains(text, "historical_results_not_current_execution_evidence") {
		t.Fatal("pre-read lost historical evidence ceiling")
	}
	if len(ctx.Mutable.TraceCatalogs()) != 0 || len(ctx.Mutable.DispatchToolResults()) != 0 || ctx.Mutable.TraceQueryRuntimeObservationCount() != 0 {
		t.Fatal("pre-read manufactured execution/evidence")
	}
	if hasToolSchema(capture.tools, "trace_query") {
		t.Fatal("pre-read installed a runtime source before normal resume admission")
	}
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, explorer.(*ExplorerAgent).slotFor(ctx).eval)
	sk := traceTeachingSkill(t, "explore-skill")
	schemas := base.buildToolSchemas(sk, ctx)
	resumed, mcp := base.executeTool(ctx, llm.ToolCall{Name: "trace_catalog", Params: next}, toolSchemaNameSet(schemas))
	if mcp != nil || resumed == nil || !resumed.Success {
		t.Fatalf("current schema resume: %v %+v", mcp, resumed)
	}
	c := ctx.Mutable.TraceCatalogs()[0]
	if len(c.Snapshot().Queries) != 2 {
		t.Fatal("resume lost member plans")
	}
	for _, q := range c.Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 || c.IsDeclaredQuery(q.ID) {
			t.Fatalf("history promoted: %+v", q)
		}
	}
	var page struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(resumed.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Calls) != 2 {
		t.Fatal("not all previous plans were offered")
	}
	for _, raw := range page.Calls {
		schemas = base.buildToolSchemas(sk, ctx)
		got, err := base.executeTool(ctx, llm.ToolCall{Name: "trace_query", Params: raw}, toolSchemaNameSet(schemas))
		if err != nil || got == nil || !got.Success {
			t.Fatalf("native current execution: %v %+v", err, got)
		}
	}
	for _, q := range c.Snapshot().Queries {
		if len(q.Attempts) != 1 || q.Outcome == tracecatalog.OutcomeNotExecuted {
			t.Fatalf("current execution missing: %+v", q)
		}
	}
}
