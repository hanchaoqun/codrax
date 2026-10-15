package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func prepareFixture(t *testing.T) (string, preparation) {
	t.Helper()
	for _, executable := range []string{"git", "sqlite3"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " unavailable")
		}
	}
	out := filepath.Join(t.TempDir(), "evaluation")
	receipt, err := prepare(context.Background(), out, "../fixtures/hmosperf_catalog_objects")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(receipt.Workspace), receipt
}

func TestOperatorCheckpointUsesActualQueryAndSeparateOracle(t *testing.T) {
	out, receipt := prepareFixture(t)
	if receipt.ModelCalls != 0 || receipt.SeedOutcomes[tracecatalog.OutcomeSuccess] != 1 || receipt.SeedOutcomes[tracecatalog.OutcomeNotExecuted] != 3 {
		t.Fatalf("not a bounded zero-model checkpoint: %+v", receipt)
	}
	snapshot, err := tracecatalog.Load(context.Background(), receipt.Checkpoint)
	if err != nil || len(snapshot.Queries) != 4 || len(snapshot.Artifacts) != 2 {
		t.Fatalf("snapshot: %v %+v", err, snapshot)
	}
	var result types.ToolResult
	body, err := os.ReadFile(filepath.Join(out, "operator", "seed-tool-result.json"))
	if err != nil || json.Unmarshal(body, &result) != nil || !result.Success || result.RawRef == "" {
		t.Fatalf("missing actual result: %v", err)
	}
	for rel, want := range receipt.SourceSHA {
		body, err := os.ReadFile(filepath.Join(receipt.Workspace, rel))
		if err != nil || hash(body) != want {
			t.Fatalf("source mutated: %s %v", rel, err)
		}
	}
	entries, err := os.ReadDir(receipt.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case ".git", "captures", "checkpoint.json":
		default:
			t.Fatalf("operator oracle leaked into model repository: %s", entry.Name())
		}
	}
	if _, err := prepare(context.Background(), out, "../fixtures/hmosperf_catalog_objects"); err == nil {
		t.Fatal("old evaluation overwritten")
	}
	if err := audit(context.Background(), out, receipt.Checkpoint); err == nil {
		t.Fatal("historical partial checkpoint passed as current completion")
	}
}

func TestFreshPublicResumeQueriesAllMembersWithoutOldEvidence(t *testing.T) {
	out, receipt := prepareFixture(t)
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: receipt.Workspace, WorkDir: filepath.Join(out, "fresh"), Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "fresh", "prepared")})}
	seed, err := tracecatalog.Load(context.Background(), receipt.Checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	status, _ := json.Marshal(map[string]any{"action": "status", "catalog_id": seed.ID})
	if got, _ := (&tool.TraceCatalog{}).Execute(bus, status); got.Success {
		t.Fatal("old ID regained live access before resume")
	}
	params, _ := json.Marshal(map[string]any{"action": "resume", "root": "captures", "snapshot_path": "checkpoint.json"})
	got, err := (&tool.TraceCatalog{}).Execute(bus, params)
	if err != nil || !got.Success {
		t.Fatalf("public resume: %v %s", err, got.Summary)
	}
	if len(got.Observations) != 0 || bus.Mutable.TraceQueryRuntimeObservationCount() != 0 || len(bus.Mutable.TraceQueryBlobRefs()) != 0 {
		t.Fatal("restore imported historical evidence")
	}
	catalogs := bus.Mutable.TraceCatalogs()
	if len(catalogs) != 1 {
		t.Fatalf("catalogs: %d", len(catalogs))
	}
	catalog := catalogs[0]
	for _, q := range catalog.Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 {
			t.Fatalf("history signed current execution: %+v", q)
		}
		params, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]any{"catalog_id": catalog.ID(), "query_id": q.ID}})
		result, err := (&tool.TraceQuery{}).Execute(bus, params)
		if err != nil || !result.Success {
			t.Fatalf("public referenced query: %v %s", err, result.Summary)
		}
		artifactPath := ""
		for _, artifact := range catalog.Snapshot().Artifacts {
			if artifact.ID == q.ArtifactID {
				artifactPath = artifact.Path
			}
		}
		matched := false
		for _, observation := range result.Observations {
			p, ok := tool.DecodeTraceResourceStack(observation)
			if !ok {
				continue
			}
			frames := 0
			for _, event := range p.Events {
				frames += len(event.Frames)
			}
			for _, want := range expected {
				if artifactPath == filepath.Join(receipt.Workspace, want.Path) && p.TargetPID == want.TID {
					matched = p.MatchedEvents == want.Events && frames == want.Frames && p.OmittedEvents == 0 && p.Window.StartTs == 10 && p.Window.EndTs == 10.05 && !p.Window.EndInclusive
				}
			}
		}
		if !matched {
			t.Fatalf("typed current facts differ from offline oracle for %+v", q)
		}
	}
	// Supply the precise emitted index path, never select a newest file.
	status, _ = json.Marshal(map[string]any{"action": "status", "catalog_id": catalog.ID()})
	got, err = (&tool.TraceCatalog{}).Execute(bus, status)
	if err != nil || !got.Success {
		t.Fatalf("status: %v %s", err, got.Summary)
	}
	var page struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(got.Summary), &page); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), out, page.SavedIndex); err != nil {
		t.Fatal(err)
	}
	// Independent current-source tampering must fail the operator audit, not
	// rewrite a fresh receipt to green.
	if err := os.Chmod(filepath.Join(receipt.Workspace, "captures", "capture.data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(receipt.Workspace, "captures", "capture.data"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), out, page.SavedIndex); err == nil || !strings.Contains(err.Error(), "capture changed") {
		t.Fatalf("mutation audit: %v", err)
	}
}

func TestPublicResumeDoesNotAdmitForeignRootOrQueryOverrides(t *testing.T) {
	out, receipt := prepareFixture(t)
	outside := t.TempDir()
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: outside, WorkDir: filepath.Join(out, "foreign"), Mutable: types.NewMutableState(question)}
	raw, _ := json.Marshal(map[string]any{"action": "resume", "root": filepath.Join(receipt.Workspace, "captures"), "snapshot_path": receipt.Checkpoint})
	if got, _ := (&tool.TraceCatalog{}).Execute(bus, raw); got.Success || len(bus.Mutable.TraceCatalogs()) != 0 {
		t.Fatal("historical path authorized an outside root")
	}
	bus.RepoRoot = receipt.Workspace
	raw, _ = json.Marshal(map[string]any{"action": "resume", "root": "captures", "snapshot_path": "checkpoint.json"})
	if got, err := (&tool.TraceCatalog{}).Execute(bus, raw); err != nil || !got.Success {
		t.Fatalf("resume: %v %s", err, got.Summary)
	}
	catalog := bus.Mutable.TraceCatalogs()[0]
	q := catalog.Snapshot().Queries[0]
	for _, override := range []map[string]any{{"pid": 999}, {"view": "event_search"}, {"time_end": 11}, {"source": "path", "path": receipt.Checkpoint}} {
		override["catalog_query_ref"] = map[string]any{"catalog_id": catalog.ID(), "query_id": q.ID}
		raw, _ = json.Marshal(override)
		if got, _ := (&tool.TraceQuery{}).Execute(bus, raw); got.Success {
			t.Fatalf("reference accepted free override: %s", raw)
		}
	}
	for _, q := range catalog.Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 {
			t.Fatal("rejected ref override executed or completed a plan")
		}
	}
}

// This adapter only drives deterministic plumbing. It is never used by the
// documented real CLI evaluation and is not a model acceptance result.
type schemaProbe struct {
	ctx   *types.AgentContext
	calls int
	stop  error
}

func (p *schemaProbe) Chat(_ context.Context, _ []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	p.calls++
	var call llm.ToolCall
	switch p.calls {
	case 1:
		call = llm.ToolCall{ID: "resume", Name: "trace_catalog", Params: json.RawMessage(`{"action":"resume","root":"captures","snapshot_path":"checkpoint.json"}`)}
	case 2:
		visible := false
		for _, schema := range schemas {
			if schema.Name == "trace_query" {
				visible = true
			}
		}
		if !visible {
			return llm.Response{}, errors.New("actual query schema is absent after resume")
		}
		// Resolve only IDs actually delivered by the tool, not a fixture's
		// privileged mutable roster that the model could not have observed.
		var catalogID, queryID string
		for _, result := range p.ctx.Mutable.DispatchToolResults() {
			if result.ToolName != "trace_catalog" || !result.Success {
				continue
			}
			var page struct {
				CatalogID string            `json:"catalog_id"`
				Records   []json.RawMessage `json:"records"`
			}
			if json.Unmarshal([]byte(result.Summary), &page) != nil {
				continue
			}
			for _, raw := range page.Records {
				var query tracecatalog.QueryRecord
				if json.Unmarshal(raw, &query) == nil && query.View != "" && query.ID != "" {
					catalogID, queryID = page.CatalogID, query.ID
					break
				}
			}
		}
		if catalogID == "" || queryID == "" {
			return llm.Response{}, errors.New("actual delivered catalog omitted executable current query identities")
		}
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]any{"catalog_id": catalogID, "query_id": queryID}})
		call = llm.ToolCall{ID: "referenced", Name: "trace_query", Params: raw}
	default:
		return llm.Response{}, p.stop
	}
	return llm.Response{ToolCalls: []llm.ToolCall{call}, StopReason: "tool_use"}, nil
}
func (*schemaProbe) ModelID() string               { return "deterministic-schema-probe-not-live" }
func (*schemaProbe) MaxContextTokens() int         { return 128000 }
func (*schemaProbe) MaxOutputTokens() int          { return 4096 }
func (*schemaProbe) RequestTimeout() time.Duration { return 0 }
func (*schemaProbe) RetryMaxAttempts() int         { return 0 }

func TestActualExplorerAcceptsRefOnlyAgainstPresentedSchema(t *testing.T) {
	out, receipt := prepareFixture(t)
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: receipt.Workspace, WorkDir: filepath.Join(out, "agent"), Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "agent", "prepared")})}
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	probe := &schemaProbe{ctx: ctx, stop: errors.New("captured actual schema-validated query result")}
	registry := tool.NewRegistry()
	tool.RegisterDefaults(registry)
	skills := skill.NewRegistry()
	skill.RegisterDefaults(skills)
	config, err := skills.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	explorer := agent.NewExplorerAgent(&agent.Dependencies{LLM: probe, Tools: registry, MaxIterations: 3, AgentSettings: types.DefaultAgentSettings()})
	_, err = explorer.Execute(ctx, config)
	if !errors.Is(err, probe.stop) || probe.calls != 3 {
		t.Fatalf("actual agent loop: calls=%d err=%v", probe.calls, err)
	}
	count := 0
	for _, result := range ctx.Mutable.DispatchToolResults() {
		if result.ToolName != "trace_query" {
			continue
		}
		count++
		if !result.Success || result.RawRef == "" {
			t.Fatalf("reference-only call failed actual schema/execution: %+v", result)
		}
	}
	if count != 1 {
		t.Fatalf("actual native queries=%d", count)
	}
}
