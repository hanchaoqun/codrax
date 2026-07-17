package agent

// TOOLWIN-FIX pins (§29.118 / §29.104.20, real_trace_campaign_20260705.md).
//
// Lesion (witness eval/results/real_trace_h3_iofam_one_seat-20260717-051341,
// intent=explain question_kind=enumeration scenario=architecture_explain, and
// eval/results/real_trace_h6_channel_mixed_display-20260715-062137,
// intent=root_cause question_kind=mechanism scenario=root_cause): an
// attached-trace run whose typed source_inventory_profile carried scheduling
// authority got a lens-first explore window whose tool surface was exactly
// {repo_map, emit_evidence, emit_investigation_complete}. The model's correct
// first action — trace_query — was structurally absent from the schema, the
// accepted completion inside that window auto-completed the remaining explore
// nodes, and the run shipped an honest all-anchors-missing absence answer.
// The driver is the typed profile, not the classification labels (the two
// witnesses share no label): the orchestrator-side census pin lives in
// internal/orchestrator/toolwin_fix_test.go.
//
// Invariant pinned here: when the run carries a typed runtime trace artifact,
// EVERY scheduler-owned explore window surface (the closed ExploreToolSurface
// enum) keeps trace_query exposed at schema level AND admitted at the
// validate arms. Runs without a trace attachment keep every surface
// byte-identical to the pre-fix sets (negative arms).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func toolwinAttachedTracePreflightForTest() types.RuntimeArtifactPreflightProfile {
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		Active:                   true,
		SourceNavigationOptional: true,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:    "trace",
			Source:  "eval/fixtures/real_traces/donghu.ftrace",
			Carrier: "attachment",
		}},
	})
}

func toolwinExploreCtxForTest(surface types.ExploreToolSurface, withTrace bool) *types.AgentContext {
	ctx := &types.AgentContext{
		Stage:              types.StageExplore,
		ExploreToolSurface: surface,
	}
	if withTrace {
		ctx.RuntimeArtifactPreflight = toolwinAttachedTracePreflightForTest()
	}
	return ctx
}

func toolwinFullExploreSchemasForTest() []llm.ToolSchema {
	return []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "list_files"},
		{Name: "grep"},
		{Name: "repo_map"},
		{Name: "exec_command"},
		{Name: "trace_query"},
		{Name: "emit_evidence"},
		{Name: "emit_investigation_complete"},
	}
}

// ① Witness-shape pin: the source-inventory lens window of an attached-trace
// run exposes trace_query (schema filter + direct surface + validate arm).
func TestToolwinFix_LensWindowKeepsTraceQueryWithAttachedTrace(t *testing.T) {
	eval := &explorerEvaluator{}
	ctx := toolwinExploreCtxForTest(types.ExploreToolSurfaceSourceInventoryLens, true)

	got := eval.FilterToolSchemas(ctx, toolwinFullExploreSchemasForTest())
	if gotNames := explorerSchemaNames(got); strings.Join(gotNames, ",") != "repo_map,trace_query,emit_evidence,emit_investigation_complete" {
		t.Fatalf("attached-trace lens surface must keep trace_query, got %v", gotNames)
	}

	surface := sourceInventoryLensToolSurface(ctx)
	if !surface["trace_query"] {
		t.Fatalf("sourceInventoryLensToolSurface must admit trace_query for an attached-trace run, got %v", surface)
	}

	call := llm.ToolCall{Name: "trace_query", Params: json.RawMessage(`{"view":"window_stats"}`)}
	if violation := validateExplorerSourceInventoryLensToolCall(ctx, eval, call); violation != nil {
		t.Fatalf("lens validate arm must admit trace_query on an attached-trace run, got rejection: %s", violation.Summary)
	}
}

// ① (second witness surface) follow-up probe window of an attached-trace run.
func TestToolwinFix_FollowupWindowKeepsTraceQueryWithAttachedTrace(t *testing.T) {
	eval := &explorerEvaluator{sourceInventoryLensSurfaceReleased: true}
	ctx := toolwinExploreCtxForTest(types.ExploreToolSurfaceSourceInventoryFollowup, true)
	ctx.SourceInventoryFollowupDebt = sourceInventoryFollowupDebtForExplorerTest("")

	got := eval.FilterToolSchemas(ctx, toolwinFullExploreSchemasForTest())
	if gotNames := explorerSchemaNames(got); strings.Join(gotNames, ",") != "repo_map,trace_query,emit_investigation_complete" {
		t.Fatalf("attached-trace follow-up surface must keep trace_query, got %v", gotNames)
	}

	surface := sourceInventoryFollowupToolSurface(ctx)
	if !surface["trace_query"] {
		t.Fatalf("sourceInventoryFollowupToolSurface must admit trace_query for an attached-trace run, got %v", surface)
	}

	call := llm.ToolCall{Name: "trace_query", Params: json.RawMessage(`{"view":"window_stats"}`)}
	if violation := validateExplorerSourceInventoryLensToolCall(ctx, eval, call); violation != nil {
		t.Fatalf("follow-up validate arm must admit trace_query on an attached-trace run, got rejection: %s", violation.Summary)
	}
}

// ② Closed-enum census: for EVERY ExploreToolSurface value, an attached-trace
// explore window keeps trace_query at schema level and both validate arms
// admit it. Together with the skill-suggestion arm below this makes the
// witness failure shape ("尝试不可用工具 trace_query" — a call to a tool absent
// from the dispatch schema) structurally unreachable in attached-trace runs:
// the renderer prints that line only when the called tool is missing from the
// iter-0 schema set, and every surface in the closed enum now retains it.
func TestToolwinFix_ExploreSurfaceCensus_AttachedTraceAlwaysAdmitsTraceQuery(t *testing.T) {
	surfaces := []types.ExploreToolSurface{
		types.ExploreToolSurfaceDefault,
		types.ExploreToolSurfaceSourceInventoryLens,
		types.ExploreToolSurfaceSourceInventoryFollowup,
	}
	for _, surface := range surfaces {
		eval := &explorerEvaluator{}
		ctx := toolwinExploreCtxForTest(surface, true)
		if surface.IsSourceInventoryFollowup() {
			ctx.SourceInventoryFollowupDebt = sourceInventoryFollowupDebtForExplorerTest("")
		}

		// Schema lane: the per-dispatch filter must keep trace_query.
		filtered := eval.FilterToolSchemas(ctx, toolwinFullExploreSchemasForTest())
		hasTraceQuery := false
		for _, schema := range filtered {
			if schema.Name == "trace_query" {
				hasTraceQuery = true
			}
		}
		if !hasTraceQuery {
			t.Fatalf("surface %q: attached-trace explore window lost trace_query at schema level: %v",
				surface, explorerSchemaNames(filtered))
		}

		// Skill-suggestion lane: the schema builder's per-tool blocker must
		// not hide trace_query for an attached-trace explore dispatch (this
		// is what feeds the iter-0 schema set the filter narrows).
		base := &BaseAgent{name: types.AgentExplorer}
		if base.skillToolSuggestionBlocked(ctx, "trace_query") {
			t.Fatalf("surface %q: skill suggestion blocker must not hide trace_query on an attached-trace run", surface)
		}

		// Validate lane: the source-inventory probe boundary admits the call.
		call := llm.ToolCall{Name: "trace_query", Params: json.RawMessage(`{"view":"window_stats"}`)}
		if violation := validateExplorerSourceInventoryLensToolCall(ctx, eval, call); violation != nil {
			t.Fatalf("surface %q: source-inventory validate arm rejected trace_query: %s", surface, violation.Summary)
		}
	}
}

// ③ Negative arm: without a trace attachment every probe surface stays
// byte-identical to the pre-fix sets, and the validate arm still rejects
// trace_query inside the lens probe.
func TestToolwinFix_NoAttachmentProbeSurfacesUnchanged(t *testing.T) {
	eval := &explorerEvaluator{}
	ctx := toolwinExploreCtxForTest(types.ExploreToolSurfaceSourceInventoryLens, false)

	lens := sourceInventoryLensToolSurface(ctx)
	wantLens := map[string]bool{
		"repo_map":                    true,
		"emit_evidence":               true,
		"emit_investigation_complete": true,
	}
	if len(lens) != len(wantLens) {
		t.Fatalf("no-attachment lens surface drifted: got %v want %v", lens, wantLens)
	}
	for name := range wantLens {
		if !lens[name] {
			t.Fatalf("no-attachment lens surface lost %q: %v", name, lens)
		}
	}

	followup := sourceInventoryFollowupToolSurface(ctx)
	wantFollowup := map[string]bool{
		"repo_map":                    true,
		"emit_investigation_complete": true,
	}
	if len(followup) != len(wantFollowup) {
		t.Fatalf("no-attachment follow-up surface drifted: got %v want %v", followup, wantFollowup)
	}
	for name := range wantFollowup {
		if !followup[name] {
			t.Fatalf("no-attachment follow-up surface lost %q: %v", name, followup)
		}
	}

	got := eval.FilterToolSchemas(ctx, toolwinFullExploreSchemasForTest())
	if gotNames := explorerSchemaNames(got); strings.Join(gotNames, ",") != "repo_map,emit_evidence,emit_investigation_complete" {
		t.Fatalf("no-attachment lens filter output drifted, got %v", gotNames)
	}

	call := llm.ToolCall{Name: "trace_query", Params: json.RawMessage(`{"view":"window_stats"}`)}
	violation := validateExplorerSourceInventoryLensToolCall(ctx, eval, call)
	if violation == nil {
		t.Fatal("no-attachment lens probe must still reject trace_query")
	}
	if strings.Contains(violation.Summary, "trace_query,") || strings.HasSuffix(violation.Summary, "trace_query") {
		t.Fatalf("no-attachment rejection must not advertise trace_query as available: %s", violation.Summary)
	}
}

// ④ Reject-surface honesty: when the lens window rejects an unrelated tool on
// an attached-trace run, the advertised available-tool list names trace_query.
func TestToolwinFix_LensRejectionAdvertisesTraceQueryWhenAttached(t *testing.T) {
	eval := &explorerEvaluator{}
	ctx := toolwinExploreCtxForTest(types.ExploreToolSurfaceSourceInventoryLens, true)
	call := llm.ToolCall{Name: "exec_command", Params: json.RawMessage(`{"command":"ls"}`)}
	violation := validateExplorerSourceInventoryLensToolCall(ctx, eval, call)
	if violation == nil {
		t.Fatal("exec_command must stay outside the lens probe surface")
	}
	if !strings.Contains(violation.Summary, "trace_query") {
		t.Fatalf("attached-trace lens rejection must advertise trace_query as available: %s", violation.Summary)
	}
}
