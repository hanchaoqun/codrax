package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryToolSchemaLazyRuntimeExposure(t *testing.T) {
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.TraceQuery{})
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, nil)
	sk := &skill.Config{Name: "explore-skill", ToolSuggestions: []string{"trace_query"}}

	codeOnly := &types.AgentContext{Stage: types.StageExplore, Objective: "explain internal/tool grep implementation"}
	if hasToolSchema(base.buildToolSchemas(sk, codeOnly), "trace_query") {
		t.Fatal("trace_query must not be exposed for ordinary code-only exploration")
	}

	missingPathCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Objective: "explain why missing_capture.systrace path handling is tricky"}
	if hasToolSchema(base.buildToolSchemas(sk, missingPathCtx), "trace_query") {
		t.Fatal("trace_query must not be exposed solely because a missing token has a trace-like suffix")
	}

	traceCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "analyze sample.systrace", AttachedHitrace: "app-1 (1) [000] .... 1.0: sched_switch: prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0"}
	if !hasToolSchema(base.buildToolSchemas(sk, traceCtx), "trace_query") {
		t.Fatal("trace_query should be exposed when an attached trace exists")
	}

	dir := t.TempDir()
	systrace := filepath.Join(dir, "record_trace.systrace")
	if err := os.WriteFile(systrace, []byte("app-1 (1) [000] .... 1.0: sched_switch: prev_comm=app prev_pid=1 prev_state=S ==> next_comm=idle next_pid=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawPathOnlyCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, WorkDir: dir, Objective: "analyze record_trace.systrace for wakeup chain"}
	if hasToolSchema(base.buildToolSchemas(sk, rawPathOnlyCtx), "trace_query") {
		t.Fatal("post-analyzer trace_query exposure must not re-parse raw objective path tokens")
	}

	preflightPathCtx := &types.AgentContext{
		Stage:     types.StageExplore,
		RepoRoot:  dir,
		WorkDir:   dir,
		Objective: "analyze record_trace.systrace for wakeup chain",
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "record_trace.systrace",
				Carrier: "request_path",
			}},
		}),
	}
	if !hasToolSchema(base.buildToolSchemas(sk, preflightPathCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for typed runtime-artifact preflight trace paths")
	}
	if !traceQueryToolVisible(preflightPathCtx) {
		t.Fatal("typed trace preflight should open the trace_query tool surface")
	}
	if traceQueryToolAvailable(preflightPathCtx) {
		t.Fatal("typed trace preflight must not become a strong trace-query hard-gate carrier")
	}
	if explorerTraceQueryFirstRequired(preflightPathCtx, true) {
		t.Fatal("typed trace preflight alone must not arm trace-query-first hard blocking")
	}

	logPreflightCtx := &types.AgentContext{
		Stage:     types.StageExplore,
		RepoRoot:  dir,
		WorkDir:   dir,
		Objective: "analyze app.log for crash reason",
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "log",
				Source:  "app.log",
				Carrier: "request_path",
			}},
		}),
	}
	if hasToolSchema(base.buildToolSchemas(sk, logPreflightCtx), "trace_query") {
		t.Fatal("log-only runtime preflight must not expose trace_query")
	}

	pathCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, WorkDir: dir, Objective: "analyze record_trace.systrace for wakeup chain", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "record_trace.systrace", Confidence: 0.9}}},
	}}}
	if !hasToolSchema(base.buildToolSchemas(sk, pathCtx), "trace_query") {
		t.Fatal("trace_query should be exposed when analyzer emits a typed trace artifact path")
	}

	perftrace := filepath.Join(dir, "capture.perftrace")
	if err := os.WriteFile(perftrace, []byte("app-1 (1) [000] .... 1.0: perf_sample: cpu=0 pid=1 tid=1 period=1 symbol=main source=test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	perfTraceCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, WorkDir: dir, Objective: "analyze capture.perftrace for sampled CPU hotspots", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "capture.perftrace", Confidence: 0.9}}},
	}}}
	if !hasToolSchema(base.buildToolSchemas(sk, perfTraceCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for typed perftrace paths")
	}

	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(bundle, []byte(`{"version":"test","systrace":"record_trace.systrace","artifacts":[{"type":"systrace","path":"record_trace.systrace"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bundleCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, WorkDir: dir, Objective: "analyze capture.tracebundle.json for trace plus perf context", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "capture.tracebundle.json", Confidence: 0.9}}},
	}}}
	if !hasToolSchema(base.buildToolSchemas(sk, bundleCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for typed tracebundle paths")
	}

	perfData := filepath.Join(dir, "perf.data")
	if err := os.WriteFile(perfData, []byte("PERFILE2\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	perfDataCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, Objective: "analyze ./perf.data for running hotspots", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "./perf.data", Confidence: 0.9}}},
	}}}
	if !hasToolSchema(base.buildToolSchemas(sk, perfDataCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for a typed existing perf.data path")
	}

	perfDataCodeCtx := &types.AgentContext{Stage: types.StageExplore, Objective: "explain the raw perf.data parser implementation"}
	if hasToolSchema(base.buildToolSchemas(sk, perfDataCodeCtx), "trace_query") {
		t.Fatal("bare perf.data source-code discussion must not expose trace_query")
	}

	simpleperfProto := filepath.Join(dir, "simpleperf_proto_without_suffix")
	if err := os.WriteFile(simpleperfProto, []byte("SIMPLEPERF\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	simpleperfRawOnlyCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, Objective: "analyze simpleperf_proto_without_suffix for sampled CPU hotspots"}
	if hasToolSchema(base.buildToolSchemas(sk, simpleperfRawOnlyCtx), "trace_query") {
		t.Fatal("post-analyzer trace_query exposure must not re-parse raw suffixless objective path tokens")
	}
	simpleperfCtx := &types.AgentContext{Stage: types.StageExplore, RepoRoot: dir, Objective: "analyze simpleperf_proto_without_suffix for sampled CPU hotspots", AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: "simpleperf_proto_without_suffix", Confidence: 0.9}}},
	}}}
	if !hasToolSchema(base.buildToolSchemas(sk, simpleperfCtx), "trace_query") {
		t.Fatal("trace_query should be exposed for a typed existing suffixless SIMPLEPERF artifact path")
	}
}

func hasToolSchema(schemas []llm.ToolSchema, name string) bool {
	for _, schema := range schemas {
		if schema.Name == name {
			return true
		}
	}
	return false
}

// TestTraceQueryViewTeachingsMatchToolSchemaEnum pins the shared prompt
// teaching table against the tool's view enum: every schema view has exactly
// one teaching row, in the same order, so prompt sites rendered from the
// table can never teach a view the tool rejects (or silently skip one it
// accepts).
func TestTraceQueryViewTeachingsMatchToolSchemaEnum(t *testing.T) {
	var params struct {
		Properties struct {
			View struct {
				Enum []string `json:"enum"`
			} `json:"view"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&toolpkg.TraceQuery{}).Parameters(), &params); err != nil {
		t.Fatalf("unmarshal trace_query parameters: %v", err)
	}
	enum := params.Properties.View.Enum
	if len(enum) == 0 {
		t.Fatal("trace_query schema view enum is empty")
	}
	rows := skill.TraceQueryViewTeachings()
	if len(rows) != len(enum) {
		t.Fatalf("teaching table has %d rows, schema enum has %d views:\ntable=%v\nenum=%v", len(rows), len(enum), rows, enum)
	}
	for i, view := range enum {
		if rows[i].View != view {
			t.Fatalf("teaching table row %d is %q, schema enum has %q — keep the table in schema enum order", i, rows[i].View, view)
		}
	}
}

func TestTraceQueryToolSchemaDocumentsWakeupChainDefaultDepth(t *testing.T) {
	var params struct {
		Properties struct {
			MaxDepth struct {
				Description string `json:"description"`
			} `json:"max_depth"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&toolpkg.TraceQuery{}).Parameters(), &params); err != nil {
		t.Fatalf("unmarshal trace_query parameters: %v", err)
	}
	if !strings.Contains(params.Properties.MaxDepth.Description, "default 10") {
		t.Fatalf("max_depth schema description = %q, want default 10", params.Properties.MaxDepth.Description)
	}
}
