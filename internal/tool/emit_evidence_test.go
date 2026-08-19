package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func newEmitCtx() *types.BusContext {
	return &types.BusContext{Mutable: types.NewMutableState("")}
}

func seedReadFileHistory(ctx *types.BusContext, file string, startLine int, lines ...string) {
	if ctx == nil || ctx.Mutable == nil || len(lines) == 0 {
		return
	}
	endLine := startLine + len(lines) - 1
	var b strings.Builder
	fmt.Fprintf(&b, "[%s: showing lines %d-%d of 9999]\n", file, startLine, endLine)
	for i, line := range lines {
		fmt.Fprintf(&b, "  %d│ %s\n", startLine+i, line)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary:  b.String(),
		}},
	})
}

func TestEmitEvidence_AcceptsValidBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "Foo", "object": "bar", "source": "internal/agent/foo.go", "line_start": 12, "summary": "Foo registers bar", "anchor_kind": "call", "anchor_symbol": "Register"},
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	assertToolRuntimeTimingPhases(t, res.RuntimeTimings,
		"schema_compat_decode",
		"ground_context_cache_miss",
		"per_item_grounding_stabilize",
		"duplicate_amendment_merge",
		"summary_repair_render",
	)
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items in buffer, got %d", len(got))
	}
	for _, it := range got {
		if it.Producer != EmitEvidenceProducer {
			t.Errorf("producer = %q, want %q", it.Producer, EmitEvidenceProducer)
		}
		if it.ID == "" {
			t.Errorf("missing stable ID")
		}
		if it.LineEnd == 0 {
			t.Errorf("LineEnd should default to LineStart")
		}
	}
	if got[0].Kind != types.EvidenceRegistration {
		t.Errorf("kind = %q", got[0].Kind)
	}
	if got[0].Predicate != "registration" {
		t.Errorf("predicate default failed: %q", got[0].Predicate)
	}
}

func TestEmitEvidence_StructuredPayloadCompatRepairsStringItemsAndKeyAliases(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items":"[{\"kind\":\"direct\",\"source\":\"internal/agent/foo.go\",\"lineStart\":\"30\",\"summary\":\"isOK definition\",\"anchorKind\":\"definition\",\"anchorSymbol\":\"isOK\"}]"
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].LineStart != 30 || got[0].AnchorKind != types.AnchorDefinition || got[0].AnchorSymbol != "isOK" {
		t.Fatalf("structured payload compatibility did not preserve repaired item: %+v", got[0])
	}
}

func TestEmitEvidence_MCPURIStaysExternalObservationNotUngroundedSourceEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "pid=4242",
				"source": "mcp://fixture/trace/sleep-wakeup",
				"line_start": 7,
				"summary": "worker wakes target",
				"anchor_kind": "text_reference",
				"anchor_symbol": "pid=4242"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("external observation URI should produce an advisory success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("external observation URI should publish typed repair, got %+v", res.Repair)
	}
	if got := ctx.Mutable.EmittedEvidence(); len(got) != 0 {
		t.Fatalf("MCP URI rows must not enter current-source evidence buffer: %+v", got)
	}
	for _, want := range []string{
		"skipped 1 external observation item",
		"mcp://fixture/trace/sleep-wakeup:7",
		"do not retry emit_evidence",
		"emit_investigation_complete.reason",
		"aggregate_facts",
	} {
		if !strings.Contains(res.Summary, want) && (res.Repair == nil || !strings.Contains(res.Repair.Hint, want)) {
			t.Fatalf("summary/repair missing %q; summary=%s repair=%+v", want, res.Summary, res.Repair)
		}
	}
	if strings.Contains(res.Summary, "→ ungrounded") || strings.Contains(res.Summary, "Current actionable repair targets") {
		t.Fatalf("MCP URI rows should not be rendered as ungrounded source evidence:\n%s", res.Summary)
	}
}

func TestEmitEvidence_RuntimeUnresolvedSourceStaysExternalObservation(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "NoMethodError",
				"source": "runtime log (unresolved)",
				"summary": "attached log reports NoMethodError",
				"anchor_kind": "text_reference",
				"anchor_symbol": "NoMethodError"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime unresolved source should produce an advisory success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("runtime unresolved source should publish typed repair, got %+v", res.Repair)
	}
	if got := ctx.Mutable.EmittedEvidence(); len(got) != 0 {
		t.Fatalf("runtime rows must not enter current-source evidence buffer: %+v", got)
	}
	if !strings.Contains(res.Summary, "runtime_log external observation") ||
		!strings.Contains(res.Summary, "emit_investigation_complete.reason") {
		t.Fatalf("runtime unresolved summary should redirect to external observation closure:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "does not look like a repo-relative file path") ||
		strings.Contains(res.Summary, "→ ungrounded") {
		t.Fatalf("runtime unresolved row must not be treated as invalid current-source evidence:\n%s", res.Summary)
	}
}

func TestEmitEvidence_AttachedTraceBlobStaysExternalObservation(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "android.haitong-56023",
				"source": ".codrax/blob/20260630-091213-000-29296/attached_trace.txt",
				"line_start": 188556,
				"summary": "trace_query identified the on-chain runnable blocker",
				"anchor_kind": "text_reference",
				"anchor_symbol": "android.haitong-56023"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("attached trace blob should produce an advisory success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
		t.Fatalf("attached trace blob should publish typed repair, got %+v", res.Repair)
	}
	if got := ctx.Mutable.EmittedEvidence(); len(got) != 0 {
		t.Fatalf("attached trace blob rows must not enter current-source evidence buffer: %+v", got)
	}
	for _, want := range []string{
		"runtime_artifact external observation",
		".codrax/blob/20260630-091213-000-29296/attached_trace.txt:188556",
		"do not retry emit_evidence",
		"emit_investigation_complete.reason",
		"aggregate_facts",
	} {
		if !strings.Contains(res.Summary, want) && (res.Repair == nil || !strings.Contains(res.Repair.Hint, want)) {
			t.Fatalf("summary/repair missing %q; summary=%s repair=%+v", want, res.Summary, res.Repair)
		}
	}
	if strings.Contains(res.Summary, "→ ungrounded") || strings.Contains(res.Summary, "Current actionable repair targets") {
		t.Fatalf("attached trace blob rows should not be rendered as ungrounded source evidence:\n%s", res.Summary)
	}
}

func TestEmitEvidence_RepairsStringLoadBearingSummary(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 2278,
		"ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})",
	)
	params := json.RawMessage(`{
		"items": [{
			"kind": "direct",
			"subject": "buildAnalysisIR",
			"predicate": "calls",
			"object": "gate.RunWith",
			"source": "internal/agent/analyzer.go",
			"line_start": 2278,
			"summary": "buildAnalysisIR calls gate.RunWith.",
			"anchor_kind": "call",
			"anchor_symbol": "gate.RunWith",
			"load_bearing_summary": "The resolver-aware gate call is the terminal hop."
		}]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected string load_bearing_summary repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if !got[0].LoadBearingSummary {
		t.Fatalf("load_bearing_summary should be repaired to true: %+v", got[0])
	}
	if !strings.Contains(got[0].Summary, "terminal hop") {
		t.Fatalf("string payload should be preserved in summary, got %q", got[0].Summary)
	}
}

func TestEmitEvidence_MovesSingleTopLevelSalienceIntoOnlyItem(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [{
			"kind": "direct",
			"source": "internal/agent/foo.go",
			"line_start": 30,
			"summary": "principal proof",
			"anchor_kind": "definition",
			"anchor_symbol": "TargetSymbol"
		}],
		"salience": "exhaust_listed"
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("single-item top-level salience should be normalized into the item, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Salience != types.SalienceExhaustListed {
		t.Fatalf("salience was not preserved on the only item: %+v", got)
	}
}

func TestEmitEvidence_MergesAdjacentMetadataOnlyFragmentsIntoPriorItems(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{"kind":"direct","subject":"Alpha","source":"internal/agent/foo.go","line_start":12,"summary":"Alpha proof","anchor_kind":"definition","anchor_symbol":"Alpha"},
			{"salience":"load_bearing"},
			{"kind":"direct","subject":"Beta","source":"internal/agent/foo.go","line_start":30,"summary":"Beta proof","anchor_kind":"definition","anchor_symbol":"Beta"},
			{"salience":"supporting","context_role_hint":"related_context"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unambiguous adjacent metadata fragments should be repaired, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("metadata fragments must not become evidence rows, got %d: %+v", len(got), got)
	}
	if got[0].Salience != types.SalienceLoadBearing {
		t.Fatalf("first adjacent salience was not preserved: %+v", got[0])
	}
	if got[1].Salience != types.SalienceSupporting || got[1].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("second adjacent metadata fragment was not preserved: %+v", got[1])
	}
}

func TestRepairEmitEvidenceAdjacentMetadataFragmentsReportsOriginalIndexes(t *testing.T) {
	items := []map[string]json.RawMessage{
		{"kind": json.RawMessage(`"direct"`)},
		{"salience": json.RawMessage(`"load_bearing"`)},
		{"context_role_hint": json.RawMessage(`"related_context"`)},
	}
	got, paths := repairEmitEvidenceAdjacentItemMetadataFragments(items)
	if len(got) != 1 {
		t.Fatalf("two adjacent metadata fragments should merge into one substantive item: %+v", got)
	}
	joined := strings.Join(paths, ",")
	for _, want := range []string{
		"items[1].salience->items[0].salience",
		"items[2].context_role_hint->items[0].context_role_hint",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("repair audit paths missing %q: %v", want, paths)
		}
	}
}

func TestRepairEmitEvidenceAdjacentMetadataFragmentsFailsClosedOnAmbiguousOwner(t *testing.T) {
	items := []map[string]json.RawMessage{
		{"salience": json.RawMessage(`"context"`)},
		{"kind": json.RawMessage(`"direct"`), "salience": json.RawMessage(`"supporting"`)},
		{"salience": json.RawMessage(`"load_bearing"`)},
	}
	got, paths := repairEmitEvidenceAdjacentItemMetadataFragments(items)
	if len(got) != len(items) || len(paths) != 0 {
		t.Fatalf("leading or conflicting metadata fragments must stay fail-loud, got len=%d paths=%v", len(got), paths)
	}
}

func TestEmitEvidence_InfersMissingAnchorSymbolFromExactLineGraph(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "a.go", 10, "result := pkg.Run(ctx)")
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Relations: []repomap.Relation{
					{Kind: "call", File: "a.go", Line: 10, ToEP: repomap.RelationEndpoint{Name: "Run"}, Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "test_fixture"},
				},
			},
		},
	})
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"source":"a.go",
		"line_start":10,
		"summary":"pkg.Run dispatches the request",
		"anchor_kind":"call"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected missing anchor_symbol to be recovered, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted item, got %d", len(got))
	}
	if got[0].AnchorSymbol != "Run" {
		t.Fatalf("AnchorSymbol = %q, want Run", got[0].AnchorSymbol)
	}
	if !strings.Contains(res.Summary, "anchor_symbol was missing") {
		t.Fatalf("summary should disclose compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_InfersMissingAnchorSymbolFromQualifiedObjectReadLine(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 2278,
		"ir.QualityGate = gate.RunWith(ir, gate.GlobalThresholds(), mode, gate.RunOptions{Resolver: resolver})")
	params := json.RawMessage(`{"items":[{
		"kind":"relationship",
		"source":"internal/agent/analyzer.go",
		"line_start":2278,
		"subject":"buildAnalysisIR",
		"predicate":"calls",
		"object":"gate.RunWith",
		"summary":"buildAnalysisIR calls gate.RunWith for quality gating",
		"anchor_kind":"call"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected qualified object to provide a recoverable anchor_symbol, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted item, got %d", len(got))
	}
	if got[0].AnchorSymbol != "RunWith" {
		t.Fatalf("AnchorSymbol = %q, want RunWith", got[0].AnchorSymbol)
	}
	if !strings.Contains(res.Summary, "typed fields corroborated by the read line") {
		t.Fatalf("summary should disclose typed-field compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DoesNotInferMissingAnchorSymbolFromAmbiguousLineGraph(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "a.go", 10, "return left.Run() + right.Stop()")
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Relations: []repomap.Relation{
					{Kind: "call", File: "a.go", Line: 10, ToEP: repomap.RelationEndpoint{Name: "Run"}, Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "test_fixture"},
					{Kind: "call", File: "a.go", Line: 10, ToEP: repomap.RelationEndpoint{Name: "Stop"}, Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "test_fixture"},
				},
			},
		},
	})
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"source":"a.go",
		"line_start":10,
		"summary":"the line calls multiple helpers",
		"anchor_kind":"call"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("ambiguous missing anchor_symbol should remain rejected, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "anchor_symbol is required") {
		t.Fatalf("expected original anchor_symbol guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_InfersMissingSourceFromExactReadLineAnchor(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "a.go", 10, "result := pkg.Run(ctx)")
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"line_start":10,
		"summary":"pkg.Run dispatches the request",
		"anchor_kind":"call",
		"anchor_symbol":"Run"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected missing source to be recovered, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted item, got %d", len(got))
	}
	if got[0].Source != "a.go" {
		t.Fatalf("Source = %q, want a.go", got[0].Source)
	}
	if !strings.Contains(res.Summary, "source was missing") {
		t.Fatalf("summary should disclose source repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_InfersMissingSourceFromPathValuedSubject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/analysis/binder/binder.go", 72, "return Bind(input)")
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"subject":"internal/analysis/binder/binder.go",
		"object":"Bind",
		"line_start":72,
		"summary":"Bind is the entry point observed in this file",
		"anchor_kind":"call"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected path-valued subject source repair, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted item, got %d", len(got))
	}
	if got[0].Source != "internal/analysis/binder/binder.go" {
		t.Fatalf("Source = %q", got[0].Source)
	}
	if got[0].AnchorSymbol != "Bind" {
		t.Fatalf("AnchorSymbol = %q, want Bind", got[0].AnchorSymbol)
	}
	if !strings.Contains(res.Summary, "path-valued subject/object") {
		t.Fatalf("summary should disclose path-slot source repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DoesNotInferMissingSourceFromAmbiguousPathSlots(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 10-10 of 10]\n  10│ result := pkg.Run(ctx)\n"},
		{ToolName: "read_file", Success: true, Summary: "[b.go: showing lines 10-10 of 10]\n  10│ result := pkg.Run(ctx)\n"},
	}})
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"subject":"a.go",
		"object":"b.go",
		"line_start":10,
		"summary":"ambiguous path slots should not choose a source",
		"anchor_kind":"call"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("ambiguous path-valued slots should remain rejected, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "source is required") {
		t.Fatalf("expected source-required guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DoesNotInferMissingSourceFromAmbiguousReadLineAnchor(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 10-10 of 10]\n  10│ result := pkg.Run(ctx)\n"},
		{ToolName: "read_file", Success: true, Summary: "[b.go: showing lines 10-10 of 10]\n  10│ value := other.Run(ctx)\n"},
	}})
	params := json.RawMessage(`{"items":[{
		"kind":"mechanism",
		"line_start":10,
		"summary":"Run dispatches the request",
		"anchor_kind":"call",
		"anchor_symbol":"Run"
	}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("ambiguous missing source should remain rejected, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "source is required") {
		t.Fatalf("expected original source guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_MultiItemTopLevelSalienceGetsPreciseHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{"kind": "direct", "source": "a.go", "line_start": 1, "summary": "A", "anchor_kind": "definition", "anchor_symbol": "A"},
			{"kind": "direct", "source": "b.go", "line_start": 2, "summary": "B", "anchor_kind": "definition", "anchor_symbol": "B"}
		],
		"salience": "exhaust_listed"
	}`)

	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("multi-item top-level salience is ambiguous and must not be silently copied: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "items[i].salience") {
		t.Fatalf("ambiguous misplaced salience should produce an actionable item path hint, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DuplicateBatchIsNoProgress(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	first, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if !first.Success {
		t.Fatalf("first emit should succeed, got: %s", first.Summary)
	}
	second, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if !second.Success {
		t.Fatalf("duplicate emit should be a successful no-op, got: %s", second.Summary)
	}
	if second.Repair == nil || second.Repair.Code != EmitEvidenceDuplicateNoopCode {
		t.Fatalf("duplicate emit should expose %s repair metadata, got %+v", EmitEvidenceDuplicateNoopCode, second.Repair)
	}
	if got := strings.TrimSpace(second.Repair.Metadata["progress"]); got != "none" {
		t.Fatalf("duplicate emit progress metadata = %q, want none", got)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 1 {
		t.Fatalf("duplicate emit should not grow evidence buffer, got %d", got)
	}
	if !strings.Contains(second.Summary, "accepted 0 new item(s)") || !strings.Contains(second.Summary, "skipped 1 duplicate") {
		t.Fatalf("duplicate summary should tell the model no new evidence was recorded, got: %s", second.Summary)
	}
}

func TestEmitEvidence_SameStableIDMetadataCorrectionUpdatesSnapshot(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	first := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "ProviderAuth.api", "predicate": "defines", "source": "packages/opencode/src/auth.ts", "line_start": 42, "summary": "ProviderAuth.api participates in auth transport", "anchor_kind": "call", "anchor_symbol": "ProviderAuth"}
        ]
    }`)
	corrected := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "ProviderAuth.api", "predicate": "defines", "source": "packages/opencode/src/auth.ts", "line_start": 42, "summary": "ProviderAuth.api is the transport-facing definition used by auth.set", "anchor_kind": "definition", "anchor_symbol": "ProviderAuth", "snippet": "class ProviderAuth { api() {} }"}
        ]
    }`)
	if res, err := tool.Execute(ctx, first); err != nil || !res.Success {
		t.Fatalf("first emit failed: res=%+v err=%v", res, err)
	}
	res, err := tool.Execute(ctx, corrected)
	if err != nil {
		t.Fatalf("corrected emit: %v", err)
	}
	if !res.Success {
		t.Fatalf("corrected emit should succeed, got: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == EmitEvidenceDuplicateNoopCode {
		t.Fatalf("metadata correction must not be treated as no-progress duplicate: %+v", res.Repair)
	}
	if !strings.Contains(res.Summary, "Updated 1 existing evidence item") {
		raw, total := ctx.Mutable.EmittedEvidenceSince(0)
		t.Fatalf("correction summary should tell model the row was amended, total=%d raw=%+v got: %s", total, raw, res.Summary)
	}
	if !strings.Contains(res.Summary, "amendment direct") || strings.Contains(res.Summary, "duplicate direct") {
		t.Fatalf("amendment feedback must not contradict acceptance by labelling the row duplicate: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("same-ID correction should compact to one answer-grade row, got %d: %+v", len(got), got)
	}
	if got[0].AnchorKind != types.AnchorDefinition {
		t.Fatalf("anchor kind = %q, want %q", got[0].AnchorKind, types.AnchorDefinition)
	}
	if !strings.Contains(got[0].Summary, "auth transport") || !strings.Contains(got[0].Summary, "auth.set") {
		t.Fatalf("correction merge should preserve rich summaries, got %q", got[0].Summary)
	}
	if got[0].Snippet != "class ProviderAuth { api() {} }" {
		t.Fatalf("corrected snippet not merged: %q", got[0].Snippet)
	}
	tail, total := ctx.Mutable.EmittedEvidenceSince(1)
	if total != 2 || len(tail) != 1 || tail[0].AnchorKind != types.AnchorDefinition {
		t.Fatalf("raw evidence tail should expose correction event, total=%d tail=%+v", total, tail)
	}
}

func TestEmitEvidence_CommandScalarWithoutLineIsAdvisoryNoop(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "scope": "line",
            "source": "internal/tool",
            "line_start": 0,
            "anchor_kind": "text_reference",
            "anchor_symbol": "70693",
            "snippet": "70693 total",
            "summary": "70693 total lines across non-test .go files in internal/tool",
            "load_bearing_summary": true
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("command-derived scalar should be advisory no-op, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 0 {
		t.Fatalf("command scalar must not be recorded as source evidence, got %d item(s)", got)
	}
	if !strings.Contains(res.Summary, "aggregate_facts") ||
		!strings.Contains(res.Summary, "has no source file:line anchor") {
		t.Fatalf("summary should route the model to aggregate_facts, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "evidence_command_value_to_closure" {
		t.Fatalf("expected advisory repair code, got %+v", res.Repair)
	}
}

func TestEmitEvidence_HistoryMetadataWithoutLineIsAdvisoryNoop(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		},
	}
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "scope": "line",
            "source": "git log",
            "anchor_kind": "text_reference",
            "anchor_symbol": "af683718",
            "summary": "af683718 Tighten explorer guidance and evidence grounding 是最近一次合入的特性提交",
            "load_bearing_summary": true
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("history metadata should be advisory no-op, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 0 {
		t.Fatalf("history metadata must not be recorded as source evidence, got %d item(s)", got)
	}
	if !strings.Contains(res.Summary, "VCS/history metadata") ||
		!strings.Contains(res.Summary, "emit_investigation_complete.reason") {
		t.Fatalf("summary should route VCS metadata to closure reason/support facts, got: %s", res.Summary)
	}
}

func TestEmitEvidence_HistoryMetadataCrossfileIsAdvisoryNoop(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		},
	}
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "scope": "crossfile",
            "source": "git_log",
            "anchor_kind": "text_reference",
            "anchor_symbol": "3ae8465",
            "summary": "3ae8465 orchestrator: route retries through claim bindings 是最近一次提交",
            "crossfile_query": {
              "files": ["internal/orchestrator/orchestrator.go"],
              "pattern": "3ae8465"
            },
            "crossfile_assertion": {"kind": "exists"}
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("history crossfile metadata should be advisory no-op, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 0 {
		t.Fatalf("history metadata must not be recorded as source evidence, got %d item(s)", got)
	}
	if strings.Contains(res.Summary, "scope=crossfile assertion=exists FAILED") {
		t.Fatalf("VCS metadata must not be routed through source crossfile grounding, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "VCS/history metadata") ||
		!strings.Contains(res.Summary, "emit_investigation_complete.reason") {
		t.Fatalf("summary should route VCS metadata to closure reason/support facts, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DirectoryMeasurementFileScopeIsAdvisoryNoop(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "scope": "file",
            "source": "internal/tool",
            "anchor_kind": "definition",
            "anchor_symbol": "internal/tool",
            "summary": "internal/tool 目录包含 118 个非测试 .go 文件，总行数 70763"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("directory measurement should be advisory no-op, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 0 {
		t.Fatalf("directory measurement must not be recorded as file evidence, got %d item(s)", got)
	}
	if !strings.Contains(res.Summary, "aggregate_facts") ||
		!strings.Contains(res.Summary, "directory/file-set measurement") {
		t.Fatalf("summary should route directory measurements to aggregate_facts, got: %s", res.Summary)
	}
}

func TestEmitEvidence_MixedBatchRecordsOnlyNewEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	first := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	if res, err := tool.Execute(ctx, first); err != nil || !res.Success {
		t.Fatalf("seed emit failed: res=%+v err=%v", res, err)
	}
	mixed := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"},
          {"kind": "direct", "subject": "isReady", "source": "internal/agent/foo.go", "line_start": 42, "summary": "isReady returns true", "anchor_kind": "definition", "anchor_symbol": "isReady"}
        ]
    }`)
	res, err := tool.Execute(ctx, mixed)
	if err != nil {
		t.Fatalf("mixed execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("mixed emit should succeed, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 2 {
		t.Fatalf("mixed emit should record only the new item, got evidence buffer len %d", got)
	}
	if !strings.Contains(res.Summary, "emit_evidence accepted 1 item(s)") || !strings.Contains(res.Summary, "Skipped 1 duplicate") {
		t.Fatalf("mixed summary should report accepted and skipped counts, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsStringWrappedItemsArray(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": "[{\"kind\":\"direct\",\"subject\":\"isOK\",\"source\":\"internal/agent/foo.go\",\"line_start\":30,\"summary\":\"isOK returns true\",\"anchor_kind\":\"definition\",\"anchor_symbol\":\"isOK\"}]"
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Subject != "isOK" {
		t.Fatalf("subject = %q, want isOK", got[0].Subject)
	}
}

func TestEmitEvidence_AcceptsInitializerAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 6362,
		"CitationReq: types.CitationReq{Required: false},",
	)
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "subject": "CitationReq.Required",
            "object": "false",
            "source": "internal/orchestrator/orchestrator.go",
            "line_start": 6362,
            "summary": "CitationReq.Required is initialized to false",
            "snippet": "CitationReq: types.CitationReq{Required: false},",
            "anchor_kind": "initializer",
            "anchor_symbol": "CitationReq.Required"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorInitializer {
		t.Fatalf("anchor kind = %s, want initializer", got[0].AnchorKind)
	}
	if form := types.ClaimFormOf(got[0]); form != types.ClaimAssignmentFact {
		t.Fatalf("initializer claim form = %s, want assignment_fact", form)
	}
}

func TestEmitEvidence_RegistrationAnchorKindOnMemberInitializersNormalizesAcrossLanguages(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		line    int
		text    string
		subject string
		object  string
	}{
		{
			name:    "c designated initializer",
			path:    "kernel/bpf/hashtab.c",
			line:    2364,
			text:    "\t.map_update_elem = htab_map_update_elem,",
			subject: "bpf_map_ops_htab",
			object:  "htab_map_update_elem",
		},
		{
			name:    "go composite literal field",
			path:    "internal/router/routes.go",
			line:    42,
			text:    "\tUpdateElem: updateElemHandler,",
			subject: "RouteTable",
			object:  "updateElemHandler",
		},
		{
			name:    "typescript object member",
			path:    "packages/app/routes.ts",
			line:    17,
			text:    "\tupdateElem: updateElemHandler,",
			subject: "routes",
			object:  "updateElemHandler",
		},
		{
			name:    "arkts object member",
			path:    "entry/src/main/ets/pages/Index.ets",
			line:    31,
			text:    "\tupdateElem: updateElemHandler,",
			subject: "routes",
			object:  "updateElemHandler",
		},
		{
			name:    "cangjie named member",
			path:    "src/main.cj",
			line:    58,
			text:    "\tupdateElem: updateElemHandler,",
			subject: "routes",
			object:  "updateElemHandler",
		},
		{
			name:    "kotlin named argument style",
			path:    "src/main/kotlin/Routes.kt",
			line:    76,
			text:    "\tupdateElem = updateElemHandler,",
			subject: "RouteSpec",
			object:  "updateElemHandler",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			seedReadFileHistory(ctx, tc.path, tc.line, tc.text)
			params := json.RawMessage(fmt.Sprintf(`{
				"items": [{
					"scope": "line",
					"evidence_kind": "registration",
					"subject": %q,
					"predicate": "registers",
					"object": %q,
					"source": %q,
					"line_start": %d,
					"summary": "member initializer registration",
					"anchor_kind": "registration",
					"anchor_symbol": %q
				}]
			}`, tc.subject, tc.object, tc.path, tc.line, tc.object))
			res, err := tool.Execute(ctx, params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Success {
				t.Fatalf("expected success, got: %s", res.Summary)
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 {
				t.Fatalf("want 1 item, got %d", len(got))
			}
			if got[0].AnchorKind != types.AnchorInitializer {
				t.Fatalf("anchor kind = %s, want initializer; summary:\n%s", got[0].AnchorKind, res.Summary)
			}
			if got[0].LineStart != tc.line || got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
				t.Fatalf("grounding=%s/%s line=%d, want grounded/%s at %d; summary:\n%s",
					got[0].GroundingStatus, got[0].GroundingTier, got[0].LineStart, types.TierLineText, tc.line, res.Summary)
			}
			if !strings.Contains(got[0].GroundingNote, "semantic registration evidence was treated as an initializer anchor") {
				t.Fatalf("grounding note should record compatibility repair, got %q", got[0].GroundingNote)
			}
		})
	}
}

func TestEmitEvidence_AcceptsTextReferenceAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/types/analysis_ir.go", 1235,
		"// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
	)
	params := json.RawMessage(`{
        "items": [
          {
            "kind": "direct",
            "subject": "ShapeValue",
            "source": "internal/types/analysis_ir.go",
            "line_start": 1235,
            "summary": "comment text mentions ShapeValue as a retired shape label",
            "snippet": "// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
            "anchor_kind": "text_reference",
            "anchor_symbol": "ShapeValue"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("anchor kind = %s, want text_reference", got[0].AnchorKind)
	}
	if got[0].ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("intentional text_reference must not be auto-downgraded to illustrative_only")
	}
	if form := types.ClaimFormOf(got[0]); form != types.ClaimTextReferenceFact {
		t.Fatalf("text_reference claim form = %s, want text_reference_fact", form)
	}
}

func TestEmitEvidence_AcceptsStringLiteralAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/tool/emit_answer_document.go", 35,
		`func (t *EmitAnswerDocument) Name() string { return "emit_answer_document" }`,
	)
	params := json.RawMessage(`{
        "items": [
          {
            "scope": "line",
            "evidence_kind": "direct",
            "subject": "EmitAnswerDocument.Name",
            "object": "emit_answer_document",
            "source": "internal/tool/emit_answer_document.go",
            "line_start": 35,
            "summary": "Name returns the literal tool name",
            "anchor_kind": "string_literal",
            "anchor_symbol": "emit_answer_document"
          }
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorStringLiteral {
		t.Fatalf("anchor kind = %s, want string_literal", got[0].AnchorKind)
	}
	if got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("string literal should ground, got status=%s note=%q", got[0].GroundingStatus, got[0].GroundingNote)
	}
	if form := types.ClaimFormOf(got[0]); form != types.ClaimLiteralValueFact {
		t.Fatalf("string_literal claim form = %s, want literal_value_fact", form)
	}
}

// LoadBearingSummary opt-in surface (2026-05-08 add — u7a deep-dive).
// The flag tells the four EvidenceXxxSurfaceText helpers to append
// the trimmed summary to the typed surface line so finalize-stage
// rendering preserves scalars (hashes, version strings, counts) the
// answer cannot synthesise from typed fields alone.

func TestEmitEvidence_AcceptsLoadBearingSummary(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "first introduced in commit 0123456 with subject feat: shipping carrier", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "load_bearing_summary": true}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if !got[0].LoadBearingSummary {
		t.Errorf("LoadBearingSummary did not propagate from input to EvidenceItem")
	}
	if got[0].Summary == "" {
		t.Errorf("Summary lost during item construction")
	}
}

func TestEmitEvidence_AcceptsGroundedSurfaceTerms(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if got[0].Producer != EmitEvidenceProducer {
		t.Fatalf("first item should be model-authored evidence; got producer %q", got[0].Producer)
	}
	if strings.Join(got[0].SurfaceTerms, ",") != "Index.ets,@Entry" {
		t.Fatalf("surface terms not preserved: %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermsAcceptGrepObservedLines(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 1 matching lines]",
			"[grep params: pattern=extractSubAgentProposal path=internal/orchestrator/orchestrator.go context_lines=1]",
			"internal/orchestrator/orchestrator.go:6065:\tif proposal := extractSubAgentProposal(output, agentName); proposal != nil {",
		}, "\n"),
	})
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "dispatchStage", "predicate": "calls", "object": "extractSubAgentProposal", "source": "internal/orchestrator/orchestrator.go", "line_start": 6065, "summary": "dispatchStage checks the agent output for sub-agent proposals", "anchor_kind": "call", "anchor_symbol": "extractSubAgentProposal", "surface_terms": ["extractSubAgentProposal"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if strings.Join(got[0].SurfaceTerms, ",") != "extractSubAgentProposal" {
		t.Fatalf("surface terms not preserved: %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewPromptsModelAuthoredHeaderLabels(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["@Entry", "@Component"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("expected surface-term review repair, got %#v", res.Repair)
	}
	if !strings.Contains(res.Repair.Hint, "Index.ets") || !strings.Contains(res.Summary, "Index.ets") {
		t.Fatalf("review should name the grounded header label, repair=%q summary=%q", res.Repair.Hint, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if strings.Contains(strings.Join(got[0].SurfaceTerms, ","), "Index.ets") {
		t.Fatalf("tool must not auto-fill model-authored surface_terms; got %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewPromptsDecoratorCompanions(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 5, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("expected decorator surface-term review repair, got %#v", res.Repair)
	}
	if !strings.Contains(res.Repair.Hint, "@Component") || !strings.Contains(res.Summary, "@Component") {
		t.Fatalf("review should name missing decorator companion, repair=%q summary=%q", res.Repair.Hint, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) == 0 {
		t.Fatal("expected emitted evidence")
	}
	if strings.Contains(strings.Join(got[0].SurfaceTerms, ","), "@Component") {
		t.Fatalf("tool must not auto-fill decorator surface_terms; got %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_SurfaceTermReviewDoesNotPullParentDecoratorOntoNestedMethod(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", 1,
		"// Source: developer.huawei.com codelabs / @Builder decorator example",
		"// Surface: @Builder + @BuilderParam + UI element reuse mechanism",
		"",
		"@Component",
		"struct CardComponent {",
		"  @BuilderParam customHeader: () => void",
		"",
		"  @Builder defaultHeader() {",
		"    Text('Default Header').fontSize(20)",
		"  }",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "defaultHeader", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", "line_start": 8, "summary": "@Builder method defaultHeader", "anchor_kind": "definition", "anchor_symbol": "defaultHeader", "surface_terms": ["@Builder", "defaultHeader"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == EmitEvidenceSurfaceTermReviewCode &&
		(strings.Contains(res.Repair.Hint, "@Component") || strings.Contains(res.Repair.Hint, "@BuilderParam")) {
		t.Fatalf("nested method review should not require parent/sibling decorators, repair=%q", res.Repair.Hint)
	}
}

func TestEmitEvidence_RejectsRequestedDecoratorObjectMismatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"@Builder"}},
			SubTopics:     []types.SubTopic{{Summary: "list @Builder fragments", Entities: []string{"@Builder"}}},
		},
	}
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", 1,
		"// Source: openharmony @Styles and @Extend decorators",
		"// Surface: @Styles function + @Extend(Type) attribute method chains",
		"",
		"@Styles function commonCardStyle() {",
		"  .width('100%')",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "commonCardStyle", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", "line_start": 4, "summary": "@Styles function commonCardStyle", "anchor_kind": "definition", "anchor_symbol": "commonCardStyle", "surface_terms": ["@Styles", "commonCardStyle"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected decorator alignment rejection, got success summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "requested decorator \"@Builder\"") || !strings.Contains(res.Summary, "@Styles") {
		t.Fatalf("rejection should explain requested vs attached decorator, got %q", res.Summary)
	}
}

func TestEmitEvidence_DecoratorMismatchSkipsOnlyInvalidItem(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"@Builder"}},
			SubTopics:     []types.SubTopic{{Summary: "list @Builder fragments", Entities: []string{"@Builder"}}},
		},
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: strings.Join([]string{
					"[internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets: showing lines 1-6 of 9999]",
					"  1│ // Source: openharmony @Styles and @Extend decorators",
					"  2│ // Surface: @Styles function + @Extend(Type) attribute method chains",
					"  3│ ",
					"  4│ @Styles function commonCardStyle() {",
					"  5│   .width('100%')",
					"  6│ }",
				}, "\n"),
			},
			{
				ToolName: "read_file",
				Success:  true,
				Summary: strings.Join([]string{
					"[internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets: showing lines 1-3 of 9999]",
					"  1│ @Builder function defaultHeader() {",
					"  2│   Text('hello')",
					"  3│ }",
				}, "\n"),
			},
		},
	})
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "commonCardStyle", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", "line_start": 4, "summary": "@Styles function commonCardStyle", "anchor_kind": "definition", "anchor_symbol": "commonCardStyle", "surface_terms": ["@Styles", "commonCardStyle"]},
          {"kind": "registration", "subject": "defaultHeader", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets", "line_start": 1, "summary": "@Builder function defaultHeader", "anchor_kind": "definition", "anchor_symbol": "defaultHeader", "surface_terms": ["@Builder", "defaultHeader"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("mixed decorator batch should keep valid items and skip invalid ones: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "SKIPPED due to validation errors") ||
		!strings.Contains(res.Summary, "requested decorator \"@Builder\"") {
		t.Fatalf("summary should disclose skipped invalid decorator item, got %q", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("only valid decorator item should persist, got %d: %+v", len(got), got)
	}
	if got[0].Source != "internal/thirdparty/tree-sitter-arkts/corpus/sources/02_builder_decorator.ets" {
		t.Fatalf("persisted wrong evidence item: %+v", got[0])
	}
}

func TestEmitEvidence_DecoratorMismatchDoesNotHardRejectBareAnalyzerEntity(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"Builder"}},
			SubTopics:     []types.SubTopic{{Summary: "list builder fragments", Entities: []string{"Builder"}}},
		},
	}
	if requested := requestedDecoratorSurfaceTerms(ctx); len(requested) != 0 {
		t.Fatalf("bare analyzer entities must not become hard requested decorators, got %+v", requested)
	}
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", 1,
		"// Source: openharmony @Styles and @Extend decorators",
		"// Surface: @Styles function + @Extend(Type) attribute method chains",
		"",
		"@Styles function commonCardStyle() {",
		"  .width('100%')",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "registration", "subject": "commonCardStyle", "object": "Builder", "predicate": "registers", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", "line_start": 4, "summary": "@Styles function commonCardStyle", "anchor_kind": "definition", "anchor_symbol": "commonCardStyle", "surface_terms": ["@Styles", "commonCardStyle"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bare analyzer entity should stay advisory/search-only, got rejection: %s", res.Summary)
	}
}

func TestRequestedDecoratorSurfaceTermsConsumesExplicitTypedQuotes(t *testing.T) {
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"Builder"}},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				SourceQuotes:      []string{"explicit decorator @Builder"},
			},
		},
	}
	requested := requestedDecoratorSurfaceTerms(ctx)
	if len(requested) != 1 || !requested["@Builder"] {
		t.Fatalf("explicit typed source quote should carry requested decorator, got %+v", requested)
	}
}

func TestEmitEvidence_SurfaceTermReviewIgnoresUnrelatedSourcePathLabels(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1318,
		"// constructs graph metadata for internal/types/enumeration_boundary.go",
		"func buildAnalysisIR() {",
		"  graph := analyzerGraphForNormalize(ctx, rm)",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "buildAnalysisIR", "predicate": "calls", "object": "analyzerGraphForNormalize", "source": "internal/agent/analyzer.go", "line_start": 1320, "summary": "analyzerGraphForNormalize builds the graph used by normalization", "anchor_kind": "call", "anchor_symbol": "analyzerGraphForNormalize"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && strings.Contains(res.Repair.Hint, "internal/types/enumeration_boundary.go") {
		t.Fatalf("unrelated source-path label should not trigger surface-term review: %q", res.Repair.Hint)
	}
}

func TestEmitEvidence_SurfaceTermReviewSkipsSelfSourceBasenameLabel(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/contract_check.go", 23,
		"// contract_check.go is the orchestrator-side hook that runs the",
		"// Analyzer-v3 AnswerContract checker after the finalizer produces its",
		"// draft answer and before runTaskGraph marks the finalize node done.",
		"//",
		"// The checker itself lives in internal/analysis/contract; this file",
		"// only handles the orchestrator-facing glue: extracting citations",
		"// from the rendered answer text and translating contract.Result into",
		"// the orchestrator's backtrack signal.",
		"//",
		"// P1.3 design notes:",
		"//",
		"//   - Citations are extracted by a structural file:line regex, NOT",
		"//     a curated list of valid citations.",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "ContractCheck", "source": "internal/orchestrator/contract_check.go", "line_start": 34, "summary": "Citations are extracted by a structural file:line regex", "anchor_kind": "text_reference", "anchor_symbol": "Citations"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && strings.Contains(res.Repair.Hint, "contract_check.go") {
		t.Fatalf("source basename already represented by evidence source should not trigger review: %q", res.Repair.Hint)
	}
}

func TestEmitEvidence_SurfaceTermReviewSatisfiedWhenHeaderLabelAuthored(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"",
		"@Entry",
		"struct Index {",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 4, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index", "surface_terms": ["Index.ets", "@Entry"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == EmitEvidenceSurfaceTermReviewCode {
		t.Fatalf("surface-term review should not fire after model-authored label is present: %#v", res.Repair)
	}
}

func TestEmitEvidence_DropsUngroundedSurfaceTermsButKeepsEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/example/file.go", 10,
		"package example",
		"type Target struct{}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Target", "source": "internal/example/file.go", "line_start": 11, "summary": "Target is defined here", "anchor_kind": "definition", "anchor_symbol": "Target", "surface_terms": ["MissingAlias.ts"]}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected optional surface_terms drop to keep evidence, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped ungrounded optional term") || !strings.Contains(res.Summary, "MissingAlias.ts") {
		t.Fatalf("summary should name dropped surface_terms and the bad term, got %q", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence item should persist after optional term drop, got %d", len(got))
	}
	if len(got[0].SurfaceTerms) != 0 {
		t.Fatalf("ungrounded surface_terms should be dropped, got %#v", got[0].SurfaceTerms)
	}
}

func TestEmitEvidence_IgnoresLegacyFieldCompatButRejectsOtherUnknowns(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/example/file.go", 10,
		"package example",
		"type Target struct{}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "field":"call", "subject": "Target", "source": "internal/example/file.go", "line_start": 11, "summary": "Target is defined here", "anchor_kind": "definition", "anchor_symbol": "Target"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("legacy items[].field should be ignored before strict decode, got: %s", res.Summary)
	}

	ctx = newEmitCtx()
	seedReadFileHistory(ctx, "internal/example/file.go", 10,
		"package example",
		"type Target struct{}",
	)
	params = json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Target", "source": "internal/example/file.go", "line_start": 11, "summary": "Target is defined here", "anchor_kind": "definition", "anchor_symbol": "Target", "field_constraints": {"scope": "line"}}
        ]
    }`)
	res, err = tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("items[].field_constraints sidecar should be ignored before strict decode, got: %s", res.Summary)
	}

	ctx = newEmitCtx()
	seedReadFileHistory(ctx, "internal/example/file.go", 10,
		"package example",
		"type Target struct{}",
	)
	params = json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Target", "source": "internal/example/file.go", "line_start": 11, "summary": "Target is defined here", "anchor_kind": "definition", "anchor_symbol": "Target", "fieldConstraints": {"line_end": 11}}
        ]
    }`)
	res, err = tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("items[].fieldConstraints sidecar should promote known fields before strict decode, got: %s", res.Summary)
	}

	params = json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","note":"hi"}]}`)
	res, _ = tool.Execute(newEmitCtx(), params)
	if res.Success || !strings.Contains(res.Summary, "unknown field") {
		t.Fatalf("other unknown fields should remain fail-loud, got success=%v summary=%q", res.Success, res.Summary)
	}
}

func TestEmitEvidence_RejectsLoadBearingSummaryWithEmptySummary(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "load_bearing_summary": true}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure (empty summary + flag set), got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "load_bearing_summary=true requires a non-empty summary") {
		t.Errorf("expected targeted error message, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_LoadBearingSummaryDefaultsToFalse(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Do NOT pass the field at all — default false should propagate.
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "free-form rationale", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].LoadBearingSummary {
		t.Errorf("LoadBearingSummary should default to false when field omitted from input")
	}
}

func TestEmitEvidence_AcceptsSalience(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "principal proof", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "salience": "load_bearing"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Salience != types.SalienceLoadBearing {
		t.Fatalf("Salience = %q, want load_bearing", got[0].Salience)
	}
}

func TestEmitEvidence_RejectsUnknownSalience(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "principal proof", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol", "salience": "primary"}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "salience") || !strings.Contains(res.Summary, "load_bearing") {
		t.Fatalf("expected targeted salience enum error, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_SalienceDefaultsToUnset(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "TargetSymbol", "source": "internal/agent/foo.go", "line_start": 30, "summary": "ordinary proof", "anchor_kind": "definition", "anchor_symbol": "TargetSymbol"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Salience != types.SalienceUnset {
		t.Fatalf("Salience = %q, want unset", got[0].Salience)
	}
	if got[0].Salience.Resolve() != types.SalienceSupporting {
		t.Fatalf("unset Resolve = %q, want supporting", got[0].Salience.Resolve())
	}
}

func TestEmitEvidence_SalienceSchemaSyncedWithGo(t *testing.T) {
	var schema struct {
		Properties struct {
			Items struct {
				Items struct {
					Properties map[string]struct {
						Enum []string `json:"enum"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(emitEvidenceParametersSchema(), &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	got := schema.Properties.Items.Items.Properties["salience"].Enum
	if !reflect.DeepEqual(got, types.EvidenceSalienceStrings()) {
		t.Fatalf("schema salience enum = %v, want %v", got, types.EvidenceSalienceStrings())
	}
}

func TestEmitEvidence_ParametersUsesOnlyCanonicalSchema(t *testing.T) {
	got := (&EmitEvidence{}).Parameters()
	want := emitEvidenceParametersSchema()
	if !bytes.Equal(got, want) {
		t.Fatal("Parameters must return the canonical generated schema byte-for-byte")
	}
	var schema map[string]any
	if err := json.Unmarshal(got, &schema); err != nil {
		t.Fatalf("canonical schema is invalid JSON: %v", err)
	}
	props := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if _, stale := props["kind"]; stale {
		t.Fatal("retired legacy kind field leaked into the LLM-facing schema")
	}
	if _, ok := props["evidence_kind"]; !ok {
		t.Fatal("canonical evidence_kind field is missing")
	}
	for _, field := range []string{"negative_query", "negative_scope"} {
		if _, ok := props[field]; !ok {
			t.Fatalf("typed absence field %q is missing", field)
		}
	}
	itemSchema := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	allOf, ok := itemSchema["allOf"].([]any)
	if !ok || len(allOf) != 2 {
		t.Fatalf("relation endpoint conditional missing from canonical schema: %+v", itemSchema["allOf"])
	}
	branch := allOf[0].(map[string]any)
	thenRequired := branch["then"].(map[string]any)["required"].([]any)
	if !reflect.DeepEqual(thenRequired, []any{"subject", "object"}) {
		t.Fatalf("relation endpoint conditional requires %v, want subject+object", thenRequired)
	}
	conditionalBranch := allOf[1].(map[string]any)
	conditionalThen := conditionalBranch["then"].(map[string]any)
	conditionalRequired := conditionalThen["required"].([]any)
	if !reflect.DeepEqual(conditionalRequired, []any{"condition", "anchor_kind"}) {
		t.Fatalf("conditional evidence schema requires %v, want condition+anchor_kind", conditionalRequired)
	}
	anchorKind := conditionalThen["properties"].(map[string]any)["anchor_kind"].(map[string]any)["const"]
	if anchorKind != "condition" {
		t.Fatalf("conditional evidence anchor_kind const = %v, want condition", anchorKind)
	}
}

func TestEmitEvidenceSchema_SelectedContainerDefinitionDoesNotReplaceBindingRow(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitEvidence{}).Parameters(), &schema); err != nil {
		t.Fatalf("decode emit_evidence schema: %v", err)
	}
	props := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	description, _ := props["evidence_kind"].(map[string]any)["description"].(string)
	for _, want := range []string{
		"selected module/factory/container definition",
		"separate registration row in the same batch",
		"definition row never substitutes",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("evidence_kind schema missing atomic registration teaching %q: %q", want, description)
		}
	}
}

func TestEmitEvidenceToolDescription_NoSalienceInternalLeakage(t *testing.T) {
	desc := (&EmitEvidence{}).Description()
	for _, forbidden := range []string{"tier1_floor", "extractor", "finalizer", "producer rank", "display cap"} {
		if strings.Contains(desc, forbidden) {
			t.Fatalf("description leaks internal term %q:\n%s", forbidden, desc)
		}
	}
	if !strings.Contains(desc, "salience") || !strings.Contains(desc, "load_bearing") || !strings.Contains(desc, "exhaust_listed") {
		t.Fatalf("description should teach salience values:\n%s", desc)
	}
}

func TestEmitEvidenceToolDescription_PinsJSONCarrierAndSupportRefOwnership(t *testing.T) {
	desc := (&EmitEvidence{}).Description()
	for _, want := range []string{
		"`items` is one native JSON array",
		"never a quoted/escaped JSON string",
		"`support_refs` belongs to aggregate_facts on emit_investigation_complete",
		"is not an emit_evidence item field",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing JSON ownership lesson %q:\n%s", want, desc)
		}
	}
}

func TestEmitEvidenceRepairsOnlyRedundantItemSupportRefs(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "x.go"), []byte("package p\nfunc F() {}\nfunc G() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := newEmitCtx()
	ctx.RepoRoot = repo
	res, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{
		"items":[{
			"scope":"line",
			"evidence_kind":"direct",
			"anchor_kind":"definition",
			"anchor_symbol":"F",
			"source":"x.go",
			"line_start":2,
			"support_refs":["F: x.go:2"]
		}]
	}`))
	if err != nil || !res.Success {
		t.Fatalf("redundant same-row support_refs should be repaired: err=%v result=%+v", err, res)
	}
	if !strings.Contains(res.Summary, "accepted 1 item") {
		t.Fatalf("expected accepted evidence after lossless repair: %s", res.Summary)
	}

	ctx = newEmitCtx()
	ctx.RepoRoot = repo
	res, err = (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{
		"items":[{
			"scope":"line",
			"evidence_kind":"direct",
			"anchor_kind":"definition",
			"anchor_symbol":"F",
			"source":"x.go",
			"line_start":2,
			"support_refs":["G: x.go:3"]
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Summary, `unknown field "support_refs"`) {
		t.Fatalf("non-redundant support_refs must remain fail-loud: %+v", res)
	}
}

func TestEmitEvidenceToolDescription_BoundsEvidenceEntailmentByAnchorAndSpan(t *testing.T) {
	desc := (&EmitEvidence{}).Description()
	for _, want := range []string{
		"Evidence entailment is bounded by the typed anchor and source span",
		"call-site item proves only that the caller invokes the callee",
		"does NOT prove the callee's internal guards",
		"entry identity line proves only that identity",
		"anchor a requested field value at its exact initializer line",
		"one bounded line_range",
		"emit a separate evidence item",
		"Never combine sibling functions' behavior",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description missing entailment boundary %q:\n%s", want, desc)
		}
	}
}

func TestEmitEvidence_RejectsUnknownKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"speculation","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "unknown evidence_kind") {
		t.Errorf("error msg should mention unknown evidence_kind, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("buffer should not be touched on failure")
	}
}

func TestEmitEvidence_RejectsEvidenceKindValueInsideAnchorKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"evidence_kind":"direct","source":"x.go","line_start":12,"anchor_kind":"direct","anchor_symbol":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("expected failure, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "belongs to evidence_kind, not anchor_kind") {
		t.Fatalf("expected targeted anchor_kind guidance, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsEvidenceKindValueInsideAnchorKindWhenLineGroundable(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/tool/emit_evidence.go", 1367,
		"\tif preAnchorKindKey == \"\" {",
		"\t\treturn \"\", \"\", fmt.Errorf(\"items[%d]: anchor_kind is required\", index)",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line",
			"evidence_kind": "direct",
			"source": "internal/tool/emit_evidence.go",
			"line_start": 1367,
			"anchor_kind": "mechanism",
			"anchor_symbol": "preAnchorKindKey",
			"summary": "parseAnchorFields checks preAnchorKindKey before accepting anchor fields."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema compatibility repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceMechanism {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.EvidenceMechanism)
	}
	if got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("anchor_kind=%q, want %q", got[0].AnchorKind, types.AnchorTextReference)
	}
	if !strings.Contains(res.Summary, "schema-shape compatibility repair") {
		t.Fatalf("summary should mention compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsAnchorKindInEvidenceKindAndFileScopeLineAnchor(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/subagent_runtime.go", 14,
		"type SubAgentValidator struct {",
		"\tregistry *SubAgentRegistry",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "file",
			"evidence_kind": "definition",
			"subject": "SubAgentValidator",
			"source": "internal/agent/subagent_runtime.go",
			"line_start": 14,
			"summary": "SubAgentValidator is defined here.",
			"anchor_kind": "definition",
			"anchor_symbol": "SubAgentValidator"
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected compatibility repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceDirect {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.EvidenceDirect)
	}
	if got[0].Scope != types.ScopeLine {
		t.Fatalf("scope=%q, want %q", got[0].Scope, types.ScopeLine)
	}
	if got[0].LineStart != 14 || got[0].AnchorKind != types.AnchorDefinition || got[0].AnchorSymbol != "SubAgentValidator" {
		t.Fatalf("line anchor not preserved after repair: %+v", got[0])
	}
	if !strings.Contains(res.Summary, "schema-shape compatibility repair") {
		t.Fatalf("summary should mention compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsSemanticValueInScopeToLineScope(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 80,
		"func execute(req Request) error {",
		"\treturn runtime.Run(req)",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "mechanism",
			"evidence_kind": "mechanism",
			"subject": "execute",
			"predicate": "calls",
			"object": "Run",
			"source": "internal/agent/scope_repair.go",
			"line_start": 81,
			"anchor_kind": "call",
			"anchor_symbol": "Run",
			"summary": "execute calls Run at the anchored line."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected semantic scope repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Scope != types.ScopeLine {
		t.Fatalf("scope=%q, want %q", got[0].Scope, types.ScopeLine)
	}
	if got[0].Kind != types.EvidenceMechanism {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.EvidenceMechanism)
	}
	if !strings.Contains(res.Summary, "schema-shape compatibility repair") {
		t.Fatalf("summary should mention compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsAnchorKindValueInScopeAndFillsKind(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 20,
		"type ScopeThing struct {",
		"\tName string",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "definition",
			"subject": "ScopeThing",
			"source": "internal/agent/scope_repair.go",
			"line_start": 20,
			"anchor_symbol": "ScopeThing",
			"summary": "ScopeThing is defined on this line."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected anchor scope repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Scope != types.ScopeLine || got[0].Kind != types.EvidenceDirect || got[0].AnchorKind != types.AnchorDefinition {
		t.Fatalf("unexpected repaired evidence: %+v", got[0])
	}
}

func TestEmitEvidence_RepairsMissingEvidenceKindFromAnchorShape(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 40,
		"func outer() {",
		"\treturn inner()",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line",
			"subject": "outer",
			"predicate": "calls",
			"object": "inner",
			"source": "internal/agent/scope_repair.go",
			"line_start": 41,
			"anchor_kind": "call",
			"anchor_symbol": "inner",
			"summary": "outer calls inner at this line."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected missing evidence_kind repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceRelationship {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.EvidenceRelationship)
	}
	if !strings.Contains(res.Summary, "evidence_kind was missing") {
		t.Fatalf("summary should mention compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsDirectEvidenceKindFromAnchorShape(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 50,
		"func outer() {",
		"\tinner()",
		"\tsink()",
		"}",
	)
	params := json.RawMessage(`{
		"items": [
			{
				"scope": "line",
				"evidence_kind": "direct",
				"subject": "outer",
				"source": "internal/agent/scope_repair.go",
				"line_start": 51,
				"anchor_kind": "call",
				"anchor_symbol": "inner",
				"summary": "outer calls inner at this line."
			},
			{
				"scope": "line",
				"evidence_kind": "direct",
				"subject": "outer",
				"predicate": "calls",
				"object": "sink",
				"source": "internal/agent/scope_repair.go",
				"line_start": 52,
				"anchor_kind": "call",
				"anchor_symbol": "sink",
				"summary": "outer calls sink at this line."
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected direct evidence_kind repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 emitted evidence items, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceMechanism {
		t.Fatalf("first kind=%q, want %q", got[0].Kind, types.EvidenceMechanism)
	}
	if got[1].Kind != types.EvidenceRelationship {
		t.Fatalf("second kind=%q, want %q", got[1].Kind, types.EvidenceRelationship)
	}
	if !strings.Contains(res.Summary, "conflicted with anchor_kind") {
		t.Fatalf("summary should mention compatibility repair, got: %s", res.Summary)
	}
}

func TestEmitEvidence_RepairsLineRangeWithoutEndToLineScope(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 60,
		"func target() error {",
		"\treturn nil",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line_range",
			"evidence_kind": "direct",
			"subject": "target",
			"source": "internal/agent/scope_repair.go",
			"line_start": 60,
			"anchor_kind": "definition",
			"anchor_symbol": "target",
			"summary": "target is defined here."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected line_range-to-line repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Scope != types.ScopeLine || got[0].LineEnd != got[0].LineStart {
		t.Fatalf("unexpected repaired scope/lines: %+v", got[0])
	}
}

func TestEmitEvidence_DoesNotRepairSemanticScopeWithoutLineCoordinates(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [{
			"scope": "mechanism",
			"evidence_kind": "mechanism",
			"source": "internal/agent/scope_repair.go",
			"anchor_kind": "call",
			"anchor_symbol": "Run",
			"summary": "Missing line_start is not enough to infer a line scope."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure without line coordinates, got: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Fatalf("invalid item should not be emitted")
	}
}

func TestEmitEvidence_RepairsSemanticValueInScopeToLineRange(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/scope_repair.go", 100,
		"func outer() {",
		"\tinner()",
		"\tother()",
		"}",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "relationship",
			"evidence_kind": "relationship",
			"subject": "outer",
			"predicate": "calls",
			"object": "inner",
			"source": "internal/agent/scope_repair.go",
			"line_start": 100,
			"line_end": 103,
			"summary": "The function body shows the relationship."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected semantic scope repair to range to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Scope != types.ScopeLineRange {
		t.Fatalf("scope=%q, want %q", got[0].Scope, types.ScopeLineRange)
	}
	if got[0].Kind != types.EvidenceRelationship {
		t.Fatalf("kind=%q, want %q", got[0].Kind, types.EvidenceRelationship)
	}
}

func TestEmitEvidence_RepairsStringLiteralIdentifierAnchorToDefinition(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/types/enums.go", 24,
		"const (",
		"\tStageAnalyze  PipelineStage = \"analyze\"",
		"\tStageExplore  PipelineStage = \"explore\"",
		")",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line",
			"evidence_kind": "direct",
			"source": "internal/types/enums.go",
			"line_start": 25,
			"anchor_kind": "string_literal",
			"anchor_symbol": "StageAnalyze",
			"summary": "StageAnalyze names the analyze pipeline stage."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected identifier-anchor repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorDefinition {
		t.Fatalf("anchor_kind=%q, want %q", got[0].AnchorKind, types.AnchorDefinition)
	}
	if got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
		t.Fatalf("grounding=%s/%s, want grounded/%s; summary:\n%s",
			got[0].GroundingStatus, got[0].GroundingTier, types.TierLineText, res.Summary)
	}
	if !strings.Contains(got[0].GroundingNote, "treated as definition") {
		t.Fatalf("grounding note should record compatibility repair, got %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_RepairsStringLiteralIdentifierReferenceToTextReference(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "ressched/services/resschedservice/src/res_sched_service.cpp", 186,
		"        ResType::RES_TYPE_START_INPUT_METHOD_PROCESS,",
		"        ResType::RES_TYPE_START_UIEXTENSION_PROC,",
		"        ResType::RES_TYPE_SCHED_MODE_CHANGE,",
		"        ResType::RES_TYPE_AVCODE_HARDWARE_RESOURCES,",
		"        ResType::RES_TYPE_FIRST_FRAME_DRAWN,",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line",
			"evidence_kind": "direct",
			"source": "ressched/services/resschedservice/src/res_sched_service.cpp",
			"line_start": 189,
			"anchor_kind": "string_literal",
			"anchor_symbol": "RES_TYPE_AVCODE_HARDWARE_RESOURCES",
			"summary": "ALLOW_MULTI_UID_REPORT_RES集合中使用RES_TYPE_AVCODE_HARDWARE_RESOURCES"
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected identifier-reference repair to succeed, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("anchor_kind=%q, want %q", got[0].AnchorKind, types.AnchorTextReference)
	}
	if got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
		t.Fatalf("grounding=%s/%s, want grounded/%s; summary:\n%s",
			got[0].GroundingStatus, got[0].GroundingTier, types.TierLineText, res.Summary)
	}
	if !strings.Contains(got[0].GroundingNote, "treated as text_reference") {
		t.Fatalf("grounding note should record compatibility repair, got %q", got[0].GroundingNote)
	}
	if strings.Contains(res.Summary, "→ ungrounded") || strings.Contains(res.Summary, "Current actionable repair targets: ressched/services/resschedservice/src/res_sched_service.cpp") {
		t.Fatalf("summary should not ask for ungrounded repair after compatibility fix:\n%s", res.Summary)
	}
}

func TestEmitEvidence_DoesNotRepairStringLiteralIdentifierOnCallSite(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 7107,
		"output, err := ag.Execute(agentCtx, sk)",
	)
	params := json.RawMessage(`{
		"items": [{
			"scope": "line",
			"evidence_kind": "direct",
			"source": "internal/orchestrator/orchestrator.go",
			"line_start": 7107,
			"anchor_kind": "string_literal",
			"anchor_symbol": "Execute",
			"summary": "Execute is called here."
		}]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("tool should preserve the ungrounded item as repairable evidence, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].AnchorKind != types.AnchorStringLiteral {
		t.Fatalf("anchor_kind=%q, want %q", got[0].AnchorKind, types.AnchorStringLiteral)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding=%s, want ungrounded for call-site mismatch; summary:\n%s", got[0].GroundingStatus, res.Summary)
	}
}

func TestEmitEvidence_RepairsStringifiedItemsWithObjectFragment(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "ressched.gni", 1,
		"# gni",
		"",
		`ressched_path = "//foundation/resourceschedule"`,
		`ressched_libs = "${ressched_path}/ressched/libs"`,
		`ressched_services = "${ressched_path}/ressched/services"`,
		`ressched_plugins = "${ressched_path}/ressched/plugins"`,
		`ressched_resschedd = "${ressched_path}/ressched/resschedd"`,
		`ressched_serviceinterfaces = "${ressched_path}/ressched/interfaces"`,
	)
	items := `[` +
		`{"anchor_kind":"definition","anchor_symbol":"ressched_path","evidence_kind":"direct","line_start":3,"scope":"line","source":"ressched.gni","summary":"path root"}, ` +
		`"subject":"ressched.gni","object":"BUILD.gn","predicate":"maps","anchor_kind":"definition","anchor_symbol":"ressched_path","evidence_kind":"relationship","scope":"line_range","line_start":3,"line_end":8,"source":"ressched.gni","summary":"build path map"}, "load_bearing_summary":true}]`
	params, err := json.Marshal(map[string]string{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("stringified object-fragment items should be repaired, got: %s", res.Summary)
	}
	if got := len(ctx.Mutable.EmittedEvidence()); got != 2 {
		t.Fatalf("expected two repaired evidence items, got %d", got)
	}
}

func TestEmitEvidence_SelfRefLiteralEmitsTypedClusterKey(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{"explorer"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "direct",
				"subject": "explorer",
				"predicate": "returns",
				"object": "\"explorer\"",
				"snippet": "return \"explorer\"",
				"source": "internal/agent/sub_explorer.go",
				"line_start": 12,
				"summary": "Explorer.Name returns explorer",
				"anchor_kind": "definition",
				"anchor_symbol": "Explorer.Name"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	vs := ctx.Mutable.EvidenceClosure().ViolationsByKind(types.ViolSelfRefLiteral)
	if len(vs) != 1 {
		t.Fatalf("expected one self-ref violation, got %d", len(vs))
	}
	if got, want := vs[0].ClusterKey, "symbol:explorer|root:answer_subject.kind"; got != want {
		t.Fatalf("ClusterKey=%q, want %q", got, want)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 emitted evidence item, got %d", len(got))
	}
	if got[0].Confidence != 0 {
		t.Fatalf("self-ref evidence confidence=%v, want 0", got[0].Confidence)
	}
}

func TestEmitEvidence_ParametersExposeEvidenceKindNotLegacyKind(t *testing.T) {
	tool := &EmitEvidence{}
	schema := string(tool.Parameters())
	if !strings.Contains(schema, `"evidence_kind"`) {
		t.Fatalf("schema should expose evidence_kind: %s", schema)
	}
	if strings.Contains(schema, `"required":["kind"`) || strings.Contains(schema, `"required": ["kind"`) {
		t.Fatalf("schema should not require legacy kind field: %s", schema)
	}
}

func TestEmitEvidence_DemotesDocumentationStyleExactConfigMention(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "defines",
				"source": "docs/analysis_contract.md",
				"line_start": 367,
				"summary": "The token appears in a comment example only.",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "illustrative_only"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with demotion feedback, got: %s", res.Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("summary should steer the model away from repairing doc mentions, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should mark the mention non-repairable, got: %s", got[0].GroundingNote)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("drop-only illustrative mention must not queue structured repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_QueuesStructuredRepairTargetsForRecoveredEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "relationship",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "fullScan",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 148,
				"summary": "full scan branch",
				"anchor_kind": "call",
				"anchor_symbol": "fullScan"
			},
			{
				"kind": "relationship",
				"subject": "buildOrLoadGraph",
				"predicate": "calls",
				"object": "loadFromCache",
				"source": "internal/tool/repomap/tool.go",
				"line_start": 156,
				"summary": "cache branch",
				"anchor_kind": "call",
				"anchor_symbol": "loadFromCache"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with structured repair feedback, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceLineTextRepair {
		t.Fatalf("expected structured line-text repair metadata, got %+v", res.Repair)
	}
	if len(res.Repair.Targets) != 1 {
		t.Fatalf("expected one grouped repair target, got %+v", res.Repair.Targets)
	}
	target := res.Repair.Targets[0]
	if target.File != "internal/tool/repomap/tool.go" {
		t.Fatalf("repair target file = %q, want internal/tool/repomap/tool.go", target.File)
	}
	if !reflect.DeepEqual(target.Lines, []int{148, 156}) {
		t.Fatalf("repair target lines = %v, want [148 156]", target.Lines)
	}
	if target.Action != string(types.RepairReadFile) {
		t.Fatalf("repair target action = %q, want %q", target.Action, types.RepairReadFile)
	}
}

func TestEmitEvidenceRepairTargets_RecoveredLineDriftKeepsClaimedAndAdjustedLines(t *testing.T) {
	items := []types.EvidenceItem{{
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       7216,
		GroundingStatus: types.GroundingRecovered,
		AnchorSymbol:    "applyStageOutput",
		Subject:         "applyStageOutput",
	}}
	reports := []ground.Report{{OriginalLine: 7314, AdjustedLine: 7216}}
	targets := emitEvidenceRepairTargets(nil, items, reports)
	if len(targets) != 1 {
		t.Fatalf("expected one repair target, got %+v", targets)
	}
	if targets[0].File != "internal/orchestrator/orchestrator.go" {
		t.Fatalf("target file = %q", targets[0].File)
	}
	if !reflect.DeepEqual(targets[0].Lines, []int{7216, 7314}) {
		t.Fatalf("recovered repair target should keep both adjusted and claimed lines after sorting, got %v", targets[0].Lines)
	}
}

func TestRenderEmitSummary_RecoveredLineDriftExplainsAnchorChoice(t *testing.T) {
	items := []types.EvidenceItem{{
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       7216,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.GroundingTier("fqname_same_file"),
		Kind:            types.EvidenceDirect,
		AnchorSymbol:    "applyStageOutput",
		Summary:         "write-once AnalysisIR",
	}}
	reports := []ground.Report{{OriginalLine: 7314, AdjustedLine: 7216}}
	summary := renderEmitSummary(nil, items, reports, items)
	for _, want := range []string{
		"you claimed line 7314, adjusted to 7216",
		"if line 7314 is the intended proof, re-emit with an anchor_kind/anchor_symbol visible on that line",
		"if the recovered symbol definition is the proof, cite line 7216 instead",
		"Current actionable repair targets: internal/orchestrator/orchestrator.go near lines 7216, 7314.",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestEmitEvidenceRepairTargets_DropsRecoveredItemCoveredByGroundedSibling(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			GroundingStatus: types.GroundingGrounded,
			AnchorSymbol:    "buildAnalysisIR",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       860,
			GroundingStatus: types.GroundingRecovered,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
	}
	reports := []ground.Report{
		{},
		{AdjustedLine: 860},
	}
	if got := emitEvidenceRepairTargets(nil, items, reports); len(got) != 0 {
		t.Fatalf("recovered item already covered by a grounded sibling should not queue repair targets, got %+v", got)
	}
}

func TestEmitEvidenceRepairTargets_DropsExternalArtifactSources(t *testing.T) {
	items := []types.EvidenceItem{{
		Source:          "/dev/stdin",
		LineStart:       2,
		GroundingStatus: types.GroundingUngrounded,
		AnchorSymbol:    "LLMStreamTimeoutError",
		Subject:         "runtime log",
	}}
	reports := []ground.Report{{AdjustedLine: 2}}
	if got := emitEvidenceRepairTargets(nil, items, reports); len(got) != 0 {
		t.Fatalf("external artifact source must not become read_file repair target: %+v", got)
	}
}

func TestEmitEvidenceRepairTargets_RelativizesRepoAbsoluteSources(t *testing.T) {
	ctx := newEmitCtx()
	ctx.RepoRoot = "/repo"
	items := []types.EvidenceItem{{
		Source:          "/repo/internal/tool/emit_evidence.go",
		LineStart:       10,
		GroundingStatus: types.GroundingUngrounded,
		AnchorSymbol:    "EmitEvidence",
		Subject:         "EmitEvidence",
	}}
	reports := []ground.Report{{AdjustedLine: 10}}
	got := emitEvidenceRepairTargets(ctx, items, reports)
	if len(got) != 1 {
		t.Fatalf("expected one repo-local repair target, got %+v", got)
	}
	if got[0].File != "internal/tool/emit_evidence.go" {
		t.Fatalf("absolute repo source should be relativized, got %q", got[0].File)
	}
}

func TestBuildEmitEvidenceRepair_EmitsStructuredNoopEnvelopeForCoveredRecoveredItems(t *testing.T) {
	items := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       852,
			GroundingStatus: types.GroundingGrounded,
			AnchorSymbol:    "buildAnalysisIR",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       849,
			GroundingStatus: types.GroundingRecovered,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
	}
	reports := []ground.Report{
		{},
		{AdjustedLine: 852},
	}
	repair := buildEmitEvidenceRepair(nil, items, reports)
	if repair == nil {
		t.Fatal("expected structured repair envelope even when no actionable line-text repair remains")
	}
	if repair.Code != types.ToolRepairCodeEvidenceLineTextRepair {
		t.Fatalf("repair code=%q, want evidence_line_text_repair", repair.Code)
	}
	if len(repair.Targets) != 0 {
		t.Fatalf("covered recovered sibling should not produce actionable targets, got %+v", repair.Targets)
	}
	if got := repair.Metadata["repair_status"]; got != types.ToolRepairStatusSatisfiedOrNonActionable {
		t.Fatalf("repair_status=%q, want satisfied_or_non_actionable", got)
	}
}

func TestRenderEmitSummary_SplitsCurrentBatchFromCumulativeAudit(t *testing.T) {
	current := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       852,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   "line_text",
			Kind:            types.EvidenceDirect,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
	}
	all := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       852,
			GroundingStatus: types.GroundingUngrounded,
			Kind:            types.EvidenceDirect,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
		},
		current[0],
	}
	reports := []ground.Report{{AdjustedLine: 852}}

	summary := renderEmitSummary(nil, current, reports, all)
	for _, want := range []string{
		"Current batch: 1 grounded / 0 recovered / 0 ungrounded.",
		"Evidence buffer (audit, cumulative): 1 grounded / 0 recovered / 1 ungrounded across 1 file(s).",
		"Current actionable repair targets: none.",
		"Do not re-emit a full consolidated evidence set just to change the cumulative audit tally.",
		"Cumulative repair audit: 0 recovered / 0 ungrounded still visible in the buffer; 1 covered/non-actionable cumulative row(s) omitted from repair guidance.",
		"This is audit context; next-step repair guidance comes from current item rows above and structured ToolRepair targets only.",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "Active repair targets in buffer") {
		t.Fatalf("summary should not present cumulative audit rows as active repair targets:\n%s", summary)
	}
}

func TestRenderEmitSummary_SurfacesTextReferenceMechanismBoundaryInSameTurn(t *testing.T) {
	items := []types.EvidenceItem{{
		Source:          "internal/skill/defaults.go",
		LineStart:       1203,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   "line_text",
		Kind:            types.EvidenceMechanism,
		AnchorKind:      types.AnchorTextReference,
		AnchorSymbol:    "span parsing rule",
		Subject:         "span parsing rule",
	}}
	summary := renderEmitSummary(nil, items, []ground.Report{{AdjustedLine: 1203}}, items)
	for _, want := range []string{
		"claim_form: text_reference_fact",
		"source authority: source_shape_authority=visible_text_only executable_mechanism=unproven",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("same-turn authority feedback missing %q:\n%s", want, summary)
		}
	}
}

func TestEmitEvidence_PartialValidationFailureReturnsExactTypedRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{
			Kind:     types.DiagramArchitecture,
			Required: true,
		},
	}}
	seedReadFileHistory(ctx, "internal/demo.go", 1,
		"func Valid() {",
		"  BuildContext()",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"scope":"line","evidence_kind":"direct","source":"internal/demo.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"Valid","summary":"valid sibling"},
          {"scope":"line","evidence_kind":"relationship","subject":"Valid","predicate":"calls","object":"BuildContext","source":"internal/demo.go","anchor_kind":"call","anchor_symbol":"BuildContext","summary":"missing only line_start"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation {
		t.Fatalf("partial validation failure must keep valid siblings and return typed repair: %+v", res)
	}
	if got := res.Repair.Metadata["repair_status"]; got != types.ToolRepairStatusActionRequired {
		t.Fatalf("repair_status=%q, want action_required", got)
	}
	if got := res.Repair.Metadata["completion_blocking"]; got != "true" {
		t.Fatalf("completion_blocking=%q, want true for required typed relation diagram", got)
	}
	if len(res.Repair.Fields) != 1 || res.Repair.Fields[0] != "items[1].line_start" {
		t.Fatalf("repair fields=%v, want exact rejected field", res.Repair.Fields)
	}
	if strings.Contains(res.Summary, "Current actionable repair targets: none") ||
		!strings.Contains(res.Summary, "Current actionable repair targets: items[1].line_start") {
		t.Fatalf("summary must not contradict the typed validation repair:\n%s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorSymbol != "Valid" {
		t.Fatalf("accepted sibling should persist exactly once: %+v", got)
	}
}

func TestEmitEvidence_AllInvalidItemsReturnTypedRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope":"line","evidence_kind":"relationship","subject":"A","predicate":"calls","object":"B","source":"internal/demo.go","anchor_kind":"call","anchor_symbol":"B"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation {
		t.Fatalf("all-invalid batch must fail loud with typed local repair: %+v", res)
	}
	if len(res.Repair.Fields) != 1 || res.Repair.Fields[0] != "items[0].line_start" {
		t.Fatalf("repair fields=%v, want exact rejected field", res.Repair.Fields)
	}
}

func TestEmitEvidence_OptionalInvalidRelationDoesNotBlockCompletion(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{
			Kind:     types.DiagramArchitecture,
			Required: false,
		},
	}}
	params := json.RawMessage(`{
        "items": [
          {"scope":"line","evidence_kind":"relationship","subject":"A","predicate":"calls","object":"B","source":"internal/demo.go","anchor_kind":"call","anchor_symbol":"B"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation {
		t.Fatalf("optional malformed item should still receive a local repair: %+v", res)
	}
	if got := res.Repair.Metadata["completion_blocking"]; got != "" {
		t.Fatalf("optional relation evidence must not create completion debt, got %q", got)
	}
}

func TestCrossComponentCallChainInvalidRegistrationBlocksCompletionWithoutRequiredDiagram(t *testing.T) {
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqCallChain),
		},
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
	}}
	in := emitEvidenceItem{
		EvidenceKind: string(types.EvidenceRegistration),
		Scope:        string(types.ScopeLine),
		Source:       "bridge.rs",
		AnchorKind:   string(types.AnchorCall),
		AnchorSymbol: "add",
		Subject:      "registry",
	}
	if !emitEvidenceValidationFailureBlocksCompletion(ctx, in) {
		t.Fatal("an explicitly submitted cross-component binding row must remain repair debt even when the diagram is optional")
	}
	ctx.AnalysisIR.RequestModel.Predicates.IsCrossComponent = false
	if emitEvidenceValidationFailureBlocksCompletion(ctx, in) {
		t.Fatal("an optional single-component registration typo must keep the ordinary non-blocking policy")
	}
}

func TestRegistrationEndpointErrorTeachesActualBindingExpression(t *testing.T) {
	ctx := newEmitCtx()
	res, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"_fastlex","source":"bridge.rs","line_start":7,"anchor_kind":"definition","anchor_symbol":"_fastlex"}]}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"actual binding expression", "registry.add(wrapper(target))", "anchor_kind=call", "object=wrapper(target)"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("registration repair teaching missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestRenderEmitSummary_ListsOnlyCurrentActionableRepairTargets(t *testing.T) {
	current := []types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       100,
			GroundingStatus: types.GroundingUngrounded,
			Kind:            types.EvidenceDirect,
			AnchorSymbol:    "Missing",
			Subject:         "Missing",
		},
	}
	all := []types.EvidenceItem{
		{
			Source:          "internal/agent/old.go",
			LineStart:       22,
			GroundingStatus: types.GroundingUngrounded,
			Kind:            types.EvidenceDirect,
			AnchorSymbol:    "OldMissing",
			Subject:         "OldMissing",
		},
		current[0],
	}
	reports := []ground.Report{{AdjustedLine: 100}}

	summary := renderEmitSummary(nil, current, reports, all)
	for _, want := range []string{
		"Current actionable repair targets: internal/agent/analyzer.go near line 100.",
		"Treat these as audit candidates: repair them when current source confirms the row",
		"if the line proves stale or wrong-file, emit a grounded replacement or omit the row before widening scope.",
		"Cumulative repair audit: 0 recovered / 2 ungrounded still visible in the buffer.",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "internal/agent/old.go near line 22") {
		t.Fatalf("summary should not list old cumulative rows as current repair targets:\n%s", summary)
	}
}

func TestCumulativeEmitEvidenceRepairAuditTally_CountsOnlyActionableBufferRows(t *testing.T) {
	audit, auditOnly := cumulativeEmitEvidenceRepairAuditTally([]types.EvidenceItem{
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       100,
			GroundingStatus: types.GroundingUngrounded,
			AnchorSymbol:    "Missing",
			Subject:         "Missing",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       200,
			GroundingStatus: types.GroundingGrounded,
			AnchorSymbol:    "Recovered",
			Subject:         "Recovered",
		},
		{
			Source:          "internal/agent/analyzer.go",
			LineStart:       201,
			GroundingStatus: types.GroundingRecovered,
			AnchorSymbol:    "Recovered",
			Subject:         "Recovered",
		},
	})
	if audit.ungrounded != 1 || audit.recovered != 0 {
		t.Fatalf("audit tally = %+v, want 1 cumulative ungrounded and 0 recovered", audit)
	}
	if auditOnly != 1 {
		t.Fatalf("auditOnly = %d, want covered recovered row omitted from repair guidance", auditOnly)
	}
}

func TestEmitEvidence_PreservesValidatedDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "cmd/root.go", 1635,
		"if rs.ExploreMidLoopMinIteration != nil {",
		"	h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration",
	)
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "CLI override binding",
				"predicate": "overrides",
				"source": "cmd/root.go",
				"line_start": 1635,
				"summary": "CLI override applies when non-nil.",
				"anchor_kind": "condition",
				"anchor_symbol": "ExploreMidLoopMinIteration",
				"condition": "rs.ExploreMidLoopMinIteration != nil",
				"diagram_role_hint": "override"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("diagram role = %q, want override", got[0].DiagramRole)
	}
}

func TestEmitEvidence_PrefersMutableExactContextFilesForDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.RepoRoot = `C:\Users\ssccv\codrax`
	seedReadFileHistory(ctx, "internal/types/config.go", 876,
		"func DefaultExploreHeuristics() ExploreHeuristics {",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		EvidencePlan: types.EvidencePlan{
			RequiredFiles: []string{"internal/context/builder.go"},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "DefaultExploreHeuristics",
				"predicate": "defines",
				"source": "C:\\Users\\ssccv\\codrax\\internal\\types\\config.go",
				"line_start": 876,
				"summary": "code-default lineage anchor",
				"anchor_kind": "definition",
				"anchor_symbol": "DefaultExploreHeuristics",
				"context_role_hint": "related_context",
				"diagram_role_hint": "default"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Source != "internal/types/config.go" {
		t.Fatalf("source = %q, want canonical repo-relative path", got[0].Source)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleDefault {
		t.Fatalf("diagram role = %q, want default", got[0].DiagramRole)
	}
	if strings.Contains(got[0].GroundingNote, "outside the current same-scope required files") {
		t.Fatalf("grounding note should not treat the mutable exact-context file as out of scope, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_PreservesRequestedDiagramRoleWhenScopeRejectsValidation(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.RepoRoot = `C:\Users\ssccv\codrax`
	seedReadFileHistory(ctx, "cmd/root.go", 1635,
		"if rs.ExploreMidLoopMinIteration != nil {",
		"	h.MidLoopMinIteration = *rs.ExploreMidLoopMinIteration",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "CLI override binding",
				"predicate": "maps",
				"source": "C:\\Users\\ssccv\\codrax\\cmd\\root.go",
				"line_start": 1635,
				"summary": "CLI override binding layer",
				"anchor_kind": "condition",
				"anchor_symbol": "ExploreMidLoopMinIteration",
				"condition": "rs.ExploreMidLoopMinIteration != nil",
				"context_role_hint": "related_context",
				"diagram_role_hint": "override"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("diagram role = %q, want override when structurally-valid precedence evidence stays within grounded config context", got[0].DiagramRole)
	}
	if got[0].RequestedDiagramRole != types.EvidenceDiagramRoleOverride {
		t.Fatalf("requested diagram role = %q, want override", got[0].RequestedDiagramRole)
	}
	if strings.Contains(got[0].GroundingNote, "outside the current same-scope required files") {
		t.Fatalf("grounding note should no longer reject structurally-valid requested precedence evidence only because it lives outside the narrow same-scope file set, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_AcceptsLegacyYAMLDiagramRoleAliasForConfigFiles(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "codrax.yaml.example", 22,
		"#   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags",
	)
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 怎么生效？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want canonical config role", got[0].DiagramRole)
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("config precedence comment should stay related_context, got %q", got[0].ContextRole)
	}
}

func TestEmitEvidence_DropsConfigTraceDiagramRoleForOutOfScopeHelper(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 是怎么生效的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "collectConfigTraceDiagramAnchors",
				"predicate": "assembles",
				"source": "internal/context/builder.go",
				"line_start": 1763,
				"summary": "helper for answer-document prompt assembly",
				"anchor_kind": "definition",
				"anchor_symbol": "collectConfigTraceDiagramAnchors",
				"context_role_hint": "related_context",
				"diagram_role_hint": "runtime"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for out-of-scope helper evidence", got[0].DiagramRole)
	}
}

func TestEmitEvidence_DoesNotInferDiagramRoleWithoutHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Scenario:      types.ScenarioConfigTrace,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore",
				"predicate": "documents",
				"source": "codrax.yaml.example",
				"line_start": 20,
				"summary": "yaml precedence comment",
				"anchor_kind": "definition",
				"anchor_symbol": "explore"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown without explicit hint", got[0].DiagramRole)
	}
	if !strings.Contains(res.Summary, "Config-precedence task detected") {
		t.Fatalf("summary should nudge explicit diagram_role_hint usage, got: %s", res.Summary)
	}
}

func TestEmitEvidence_DropsInconsistentDiagramRoleHint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "LoadRuntimeConfig",
				"predicate": "binds",
				"source": "internal/config/runtime.go",
				"line_start": 194,
				"summary": "runtime binding layer",
				"anchor_kind": "assignment",
				"anchor_symbol": "resolved",
				"diagram_role_hint": "yaml"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want unknown for inconsistent hint", got[0].DiagramRole)
	}
	if !strings.Contains(got[0].GroundingNote, "diagram_role_hint=config was ignored") {
		t.Fatalf("grounding note should explain why the role hint was ignored, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(got[0].GroundingNote, "`default` / `runtime` / `override`") {
		t.Fatalf("grounding note should point at valid code-layer roles, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(res.Summary, "diagram_role_hint=config was ignored") {
		t.Fatalf("tool summary should surface the ignored-role guidance, got: %s", res.Summary)
	}
}

func TestStabilizeAssignmentEndpointAuthorityKeepsOnlyExactLineTuple(t *testing.T) {
	tests := []struct {
		name          string
		item          types.EvidenceItem
		wantChanged   bool
		wantNoteParts []string
	}{
		{
			name: "exact tuple keeps assignment authority",
			item: types.EvidenceItem{
				Scope:           types.ScopeLine,
				AnchorKind:      types.AnchorAssignment,
				Subject:         "busCtx.AnalysisIR",
				Object:          "output.AnalysisIR",
				Snippet:         `o.busCtx.AnalysisIR = output.AnalysisIR`,
				GroundingStatus: types.GroundingGrounded,
			},
		},
		{
			name: "enclosing function cannot replace lhs",
			item: types.EvidenceItem{
				Scope:           types.ScopeLine,
				AnchorKind:      types.AnchorAssignment,
				Subject:         "applyStageOutput",
				Predicate:       "passes",
				Object:          "output.AnalysisIR",
				OwnerSymbol:     "applyStageOutput",
				Snippet:         `o.busCtx.AnalysisIR = output.AnalysisIR`,
				GroundingStatus: types.GroundingGrounded,
			},
			wantChanged:   true,
			wantNoteParts: []string{`receiver="o.busCtx.AnalysisIR"`, `value="output.AnalysisIR"`},
		},
		{
			name: "ambiguous expression has no relation authority downstream without rewriting local fact",
			item: types.EvidenceItem{
				Scope:           types.ScopeLine,
				AnchorKind:      types.AnchorAssignment,
				Subject:         "state",
				Object:          "left",
				Snippet:         `state = left + right`,
				GroundingStatus: types.GroundingRecovered,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			note := stabilizeAssignmentEndpointAuthority(&tc.item)
			if got := note != ""; got != tc.wantChanged {
				t.Fatalf("changed=%t note=%q, want changed=%t", got, note, tc.wantChanged)
			}
			if !tc.wantChanged {
				if tc.item.AnchorKind != types.AnchorAssignment {
					t.Fatalf("exact tuple lost assignment authority: %+v", tc.item)
				}
				return
			}
			if tc.item.AnchorKind != types.AnchorTextReference || tc.item.Subject != "" || tc.item.Predicate != "" || tc.item.Object != "" {
				t.Fatalf("mismatched assignment retained relation authority: %+v", tc.item)
			}
			for _, want := range tc.wantNoteParts {
				if !strings.Contains(note, want) {
					t.Fatalf("note %q missing %q", note, want)
				}
			}
		})
	}
}

func TestEmitEvidence_DemotesGroundedAssignmentWithFalseDirectedEndpoints(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 8545,
		"o.busCtx.AnalysisIR = output.AnalysisIR",
	)
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"applyStageOutput","predicate":"passes","object":"output.AnalysisIR","source":"internal/orchestrator/orchestrator.go","line_start":8545,"summary":"stage output is stored on the bus context","anchor_kind":"assignment","anchor_symbol":"AnalysisIR"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("grounded source observation should remain citable, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d want 1: %+v", len(got), got)
	}
	if got[0].AnchorKind != types.AnchorTextReference || got[0].Subject != "" || got[0].Predicate != "" || got[0].Object != "" {
		t.Fatalf("false assignment endpoints retained directed authority: %+v", got[0])
	}
	for _, want := range []string{`receiver="o.busCtx.AnalysisIR"`, `value="output.AnalysisIR"`} {
		if !strings.Contains(got[0].GroundingNote, want) || !strings.Contains(res.Summary, want) {
			t.Fatalf("exact endpoint repair %q missing from item/result: item=%+v summary=%s", want, got[0], res.Summary)
		}
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("ordinary/optional evidence must keep endpoint correction advisory: %+v", res.Repair)
	}
}

func TestEmitEvidence_RequiredFlowPublishesExactAssignmentEndpointRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{
			Kind:     types.DiagramSequence,
			Required: true,
		},
	}}
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 2520,
		"o.busCtx.AnalysisIR = out.AnalysisIR",
	)
	wrong := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"runAnalyzePhase","predicate":"writes","object":"out.AnalysisIR","source":"internal/orchestrator/orchestrator.go","line_start":2520,"summary":"analysis output is written to the shared context","anchor_kind":"assignment","anchor_symbol":"AnalysisIR"}]}`)
	res, err := tool.Execute(ctx, wrong)
	if err != nil || !res.Success {
		t.Fatalf("mismatched source observation should remain citable, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceItemValidation ||
		res.Repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired ||
		res.Repair.Metadata["completion_blocking"] != "true" ||
		res.Repair.Metadata["repair_scope"] != "assignment_endpoint_identity" {
		t.Fatalf("required flow must publish typed endpoint repair debt: %+v", res.Repair)
	}
	wantFields := []string{"items[0].subject", "items[0].object"}
	if !reflect.DeepEqual(res.Repair.Fields, wantFields) {
		t.Fatalf("repair fields=%v want=%v", res.Repair.Fields, wantFields)
	}
	for _, want := range []string{
		`keep anchor_kind="assignment"`,
		"keeping the listed anchor_kind plus the same evidence_kind, anchor_symbol, predicate, source, line_start, scope, and snippet",
		`subject="o.busCtx.AnalysisIR"`,
		`object="out.AnalysisIR"`,
		"Current actionable repair targets: items[0].subject, items[0].object",
	} {
		if !strings.Contains(res.Repair.Hint+"\n"+res.Summary, want) {
			t.Fatalf("required repair/result missing %q: repair=%+v summary=%s", want, res.Repair, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "Current actionable repair targets: none") {
		t.Fatalf("summary contradicts action-required endpoint repair:\n%s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("wrong tuple must remain citable without directed authority: %+v", got)
	}

	correct := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"o.busCtx.AnalysisIR","predicate":"writes","object":"out.AnalysisIR","source":"internal/orchestrator/orchestrator.go","line_start":2520,"summary":"exact analysis result assignment","anchor_kind":"assignment","anchor_symbol":"AnalysisIR"}]}`)
	res, err = tool.Execute(ctx, correct)
	if err != nil || !res.Success {
		t.Fatalf("exact tuple re-emit failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("exact re-emit must clear endpoint repair debt: %+v", res.Repair)
	}
	got = ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || got[1].AnchorKind != types.AnchorAssignment ||
		!types.AssignmentEvidenceEndpointsMatch(got[1]) {
		t.Fatalf("exact re-emit did not publish typed assignment authority: %+v", got)
	}
}

func TestEmitEvidence_RequiredFlowAcceptsExactFullRHSExpressionAndPublishesCanonicalTuple(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
	}}
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 5692,
		"firstFinalizeDraft = strings.TrimSpace(out.FinalAnswer)",
	)
	params := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"firstFinalizeDraft","predicate":"assigns","object":"strings.TrimSpace(out.FinalAnswer)","source":"internal/orchestrator/orchestrator.go","line_start":5692,"summary":"finalizer output is trimmed into the first draft","anchor_kind":"assignment","anchor_symbol":"firstFinalizeDraft"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("exact full RHS expression failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("exact source RHS must not create contradictory canonical-endpoint repair: %+v", res.Repair)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorAssignment || !types.AssignmentEvidenceEndpointsMatch(got[0]) {
		t.Fatalf("full RHS expression did not retain typed assignment authority: %+v", got)
	}
	receiver, value, ok := types.AssignmentEvidenceEndpoints(got[0])
	if !ok || receiver != "firstFinalizeDraft" || value != "strings.TrimSpace" {
		t.Fatalf("canonical tuple=(%q,%q,%t), want (firstFinalizeDraft,strings.TrimSpace,true)", receiver, value, ok)
	}
}

func TestEmitEvidence_RequiredFlowRepairsMisclassifiedValueTransferAcrossLanguages(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		line        string
		participant string
		anchor      string
		wantKind    types.AnchorKind
		wantLHS     string
		wantRHS     string
	}{
		{name: "go initializer", source: "internal/demo/state.go", line: "Mutable: ctx.Mutable,", participant: "Mutable", anchor: "Mutable", wantKind: types.AnchorInitializer, wantLHS: "Mutable", wantRHS: "ctx.Mutable"},
		{name: "arkts initializer", source: "entry/src/main/ets/state.ets", line: "state: ctx.state,", participant: "state", anchor: "state", wantKind: types.AnchorInitializer, wantLHS: "state", wantRHS: "ctx.state"},
		{name: "cangjie assignment", source: "src/state.cj", line: "let state = ctx.state", participant: "state", anchor: "state", wantKind: types.AnchorAssignment, wantLHS: "state", wantRHS: "ctx.state"},
		{name: "cpp assignment", source: "src/state.cpp", line: "state = ctx.state;", participant: "state", anchor: "state", wantKind: types.AnchorAssignment, wantLHS: "state", wantRHS: "ctx.state"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
					Identity: tc.participant, Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			seedReadFileHistory(ctx, tc.source, 10, tc.line)
			params := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":%q,"source":%q,"line_start":10,"summary":"state carrier","anchor_kind":"definition","anchor_symbol":%q,"snippet":%q}]}`,
				tc.participant, tc.source, tc.anchor, tc.line))
			res, err := tool.Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("misclassified exact line must remain citable with repair, err=%v result=%+v", err, res)
			}
			if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "value_transfer_classification" ||
				res.Repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired ||
				res.Repair.Metadata["completion_blocking"] != "true" {
				t.Fatalf("missing typed classification repair: %+v", res.Repair)
			}
			if _, ok := decodeEmitEvidenceRelationRepairObligations(
				res.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey]); !ok {
				t.Fatalf("classification repair must publish a durable syntax obligation: %+v", res.Repair.Metadata)
			}
			if !strings.Contains(res.Summary, "Action-required typed evidence repair:") ||
				!strings.Contains(res.Summary, res.Repair.Hint) {
				t.Fatalf("same-turn model summary must expose the exact typed repair:\n%s", res.Summary)
			}
			for _, want := range []string{
				fmt.Sprintf(`anchor_kind=%q`, tc.wantKind),
				fmt.Sprintf(`subject=%q`, tc.wantLHS),
				fmt.Sprintf(`object=%q`, tc.wantRHS),
				"items[0].anchor_kind",
			} {
				if !strings.Contains(res.Repair.Hint+"\n"+strings.Join(res.Repair.Fields, "\n"), want) {
					t.Fatalf("repair missing %q: %+v", want, res.Repair)
				}
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 || got[0].AnchorKind != types.AnchorDefinition {
				t.Fatalf("system must not silently promote model evidence into a relation: %+v", got)
			}

			corrected := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":%q,"predicate":"assigns","object":%q,"source":%q,"line_start":10,"summary":"exact state transfer","anchor_kind":%q,"anchor_symbol":%q,"snippet":%q}]}`,
				tc.wantLHS, tc.wantRHS, tc.source, tc.wantKind, tc.wantLHS, tc.line))
			res, err = tool.Execute(ctx, corrected)
			if err != nil || !res.Success {
				t.Fatalf("corrected exact transfer rejected, err=%v result=%+v", err, res)
			}
			if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
				t.Fatalf("corrected exact transfer must clear current repair debt: %+v", res.Repair)
			}
			got = ctx.Mutable.EmittedEvidence()
			if len(got) != 2 || got[1].AnchorKind != tc.wantKind || !types.AssignmentEvidenceEndpointsMatch(got[1]) {
				t.Fatalf("corrected row did not publish exact typed transfer authority: %+v", got)
			}
		})
	}
}

func TestEmitEvidence_StampsExplicitMemberInitializerContainerIdentity(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	source := "internal/context/builder.go"
	line := "Mutable: bus.Mutable,"
	seedReadFileHistory(ctx, source, 59, line)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangGo,
			LineFeatures: map[int][]repomap.LineFeature{59: {repomap.LineFeatureMemberInitializer}},
			MemberInitializerBindings: map[int][]repomap.MemberInitializerBinding{59: {{
				Member: "Mutable", OwnerType: "AgentContext",
			}}},
		},
	}})
	params := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"AgentContext.Mutable","predicate":"assigns","object":"bus.Mutable","source":"internal/context/builder.go","line_start":59,"summary":"shared mutable state enters the agent context","anchor_kind":"initializer","anchor_symbol":"Mutable","snippet":"Mutable: bus.Mutable,"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("typed member initializer rejected, err=%v result=%+v", err, res)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].InitializerContainer != "AgentContext" {
		t.Fatalf("parser-owned initializer container missing: %+v", got)
	}
	receiver, value, ok := types.AssignmentEvidenceEndpoints(got[0])
	if !ok || receiver != "AgentContext.Mutable" || value != "bus.Mutable" ||
		!types.AssignmentEvidenceEndpointsMatch(got[0]) {
		t.Fatalf("typed initializer tuple=(%q,%q,%t), row=%+v", receiver, value, ok, got[0])
	}
}

func TestEmitEvidence_RequiredFlowRepairsASTAnchorKindMismatchAcrossLanguages(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		language    string
		line        string
		participant string
		wrongKind   types.AnchorKind
		feature     repomap.LineFeature
		wantKind    types.AnchorKind
		wantLHS     string
		wantRHS     string
	}{
		{name: "go member initializer mislabeled assignment", source: "internal/demo/state.go", language: repomap.LangGo, line: "Mutable: types.NewMutableState(request),", participant: "Mutable", wrongKind: types.AnchorAssignment, feature: repomap.LineFeatureMemberInitializer, wantKind: types.AnchorInitializer, wantLHS: "Mutable", wantRHS: "types.NewMutableState"},
		{name: "arkts member initializer mislabeled assignment", source: "entry/src/main/ets/state.ets", language: repomap.LangArkTS, line: "state: buildState(context),", participant: "state", wrongKind: types.AnchorAssignment, feature: repomap.LineFeatureMemberInitializer, wantKind: types.AnchorInitializer, wantLHS: "state", wantRHS: "buildState"},
		{name: "cangjie assignment mislabeled initializer", source: "src/state.cj", language: repomap.LangCangjie, line: "let state = context.state", participant: "state", wrongKind: types.AnchorInitializer, feature: repomap.LineFeatureAssignment, wantKind: types.AnchorAssignment, wantLHS: "state", wantRHS: "context.state"},
		{name: "cpp assignment mislabeled initializer", source: "src/state.cpp", language: repomap.LangCpp, line: "state = context.state;", participant: "state", wrongKind: types.AnchorInitializer, feature: repomap.LineFeatureAssignment, wantKind: types.AnchorAssignment, wantLHS: "state", wantRHS: "context.state"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
					Identity: tc.participant, Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			seedReadFileHistory(ctx, tc.source, 10, tc.line)
			ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
				tc.source: {
					RelPath: tc.source, Language: tc.language,
					LineFeatures: map[int][]repomap.LineFeature{10: {tc.feature}},
				},
			}})
			params := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":%q,"predicate":"assigns","object":%q,"source":%q,"line_start":10,"summary":"state transfer","anchor_kind":%q,"anchor_symbol":%q,"snippet":%q}]}`,
				tc.wantLHS, tc.wantRHS, tc.source, tc.wrongKind, tc.wantLHS, tc.line))
			res, err := tool.Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("wrong source-shape row should remain citable with repair, err=%v result=%+v", err, res)
			}
			if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "value_transfer_classification" ||
				res.Repair.Metadata["repair_status"] != types.ToolRepairStatusActionRequired ||
				res.Repair.Metadata["completion_blocking"] != "true" {
				t.Fatalf("missing typed AST anchor-kind repair: %+v", res.Repair)
			}
			for _, want := range []string{
				fmt.Sprintf(`anchor_kind=%q`, tc.wantKind),
				fmt.Sprintf(`subject=%q`, tc.wantLHS),
				fmt.Sprintf(`object=%q`, tc.wantRHS),
				"items[0].anchor_kind",
			} {
				if !strings.Contains(res.Repair.Hint+"\n"+strings.Join(res.Repair.Fields, "\n"), want) {
					t.Fatalf("repair missing %q: %+v", want, res.Repair)
				}
			}
			obligations, ok := decodeEmitEvidenceRelationRepairObligations(
				res.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
			if !ok || len(obligations) != 1 || obligations[0].AnchorKind != tc.wantKind {
				t.Fatalf("repair must persist the parser-owned anchor kind: %+v", res.Repair.Metadata)
			}

			corrected := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":%q,"predicate":"assigns","object":%q,"source":%q,"line_start":10,"summary":"exact state transfer","anchor_kind":%q,"anchor_symbol":%q,"snippet":%q}]}`,
				tc.wantLHS, tc.wantRHS, tc.source, tc.wantKind, tc.wantLHS, tc.line))
			res, err = tool.Execute(ctx, corrected)
			if err != nil || !res.Success {
				t.Fatalf("corrected parser-owned shape rejected, err=%v result=%+v", err, res)
			}
			if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
				t.Fatalf("corrected shape must clear current repair debt: %+v", res.Repair)
			}
		})
	}
}

func TestExactASTValueTransferTupleFailsOpenOnAmbiguousOrMissingParserShape(t *testing.T) {
	const source = "src/state.go"
	for _, tc := range []struct {
		name     string
		features []repomap.LineFeature
	}{
		{name: "missing"},
		{name: "ambiguous", features: []repomap.LineFeature{repomap.LineFeatureAssignment, repomap.LineFeatureMemberInitializer}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gc := &ground.Context{Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
				source: {RelPath: source, Language: repomap.LangGo, LineFeatures: map[int][]repomap.LineFeature{10: tc.features}},
			}}}
			if anchor, receiver, value, ok := exactASTValueTransferTuple(gc, source, 10, "Mutable: ctx.Mutable,"); ok {
				t.Fatalf("no unique parser shape must fail open: anchor=%q receiver=%q value=%q", anchor, receiver, value)
			}
		})
	}
}

func TestEmitEvidence_MisclassifiedValueTransferRepairDoesNotEnterTraceOrOptionalDiagram(t *testing.T) {
	for _, tc := range []struct {
		name     string
		intent   types.Intent
		required bool
	}{
		{name: "trace", intent: types.IntentTrace, required: true},
		{name: "optional", intent: types.IntentExplain, required: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent:        tc.intent,
				PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: tc.required, Participants: []types.DiagramParticipantHint{{
					Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			seedReadFileHistory(ctx, "internal/demo/state.go", 10, "Mutable: ctx.Mutable,")
			params := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"Mutable","source":"internal/demo/state.go","line_start":10,"summary":"state carrier","anchor_kind":"definition","anchor_symbol":"Mutable","snippet":"Mutable: ctx.Mutable,"}]}`)
			res, err := tool.Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("Execute: err=%v result=%+v", err, res)
			}
			if res.Repair != nil && res.Repair.Metadata["repair_scope"] == "value_transfer_classification" {
				t.Fatalf("classification repair escaped its required source-flow lane: %+v", res.Repair)
			}
		})
	}
}

func TestEmitEvidence_RequiredFlowMergesRejectedItemAndExactEndpointRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramSequence, Required: true},
	}}
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 2520,
		"o.busCtx.AnalysisIR = out.AnalysisIR",
	)
	params := json.RawMessage(`{"items":[
		{"scope":"line","evidence_kind":"relationship","subject":"runAnalyzePhase","predicate":"writes","object":"out.AnalysisIR","source":"internal/orchestrator/orchestrator.go","line_start":2520,"summary":"analysis output is written to shared context","anchor_kind":"assignment","anchor_symbol":"AnalysisIR"},
		{"scope":"line","evidence_kind":"relationship","subject":"producer","predicate":"writes","object":"value","source":"internal/orchestrator/orchestrator.go","summary":"missing required line","anchor_kind":"assignment"}
	]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("mixed repair call failed, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "item_validation+assignment_endpoint_identity" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("mixed repair must preserve both typed debts: %+v", res.Repair)
	}
	for _, want := range []string{"items[0].subject", "items[0].object", "items[1].line_start"} {
		if !slices.Contains(res.Repair.Fields, want) {
			t.Fatalf("mixed repair fields=%v missing %q", res.Repair.Fields, want)
		}
	}
	for _, want := range []string{
		"Validation reasons:",
		`subject="o.busCtx.AnalysisIR"`,
		`object="out.AnalysisIR"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("mixed repair hint missing %q: %s", want, res.Repair.Hint)
		}
	}
}

func TestRequiredFlowAssignmentEndpointRepairIsLanguageNeutralAndTraceIsolated(t *testing.T) {
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisFlow,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
	}}
	tests := []struct {
		name    string
		snippet string
		wantLHS string
		wantRHS string
	}{
		{name: "go", snippet: `o.busCtx.AnalysisIR = out.AnalysisIR`, wantLHS: "o.busCtx.AnalysisIR", wantRHS: "out.AnalysisIR"},
		{name: "arkts", snippet: `this.state = nextState`, wantLHS: "this.state", wantRHS: "nextState"},
		{name: "cangjie", snippet: `this.value = incoming`, wantLHS: "this.value", wantRHS: "incoming"},
		{name: "rust", snippet: `self.value = next;`, wantLHS: "self.value", wantRHS: "next"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := types.EvidenceItem{
				Scope: types.ScopeLine, Kind: types.EvidenceRelationship,
				AnchorKind: types.AnchorAssignment, Subject: "enclosingCallable", Object: tc.wantRHS,
				Snippet: tc.snippet, Source: "src/example", LineStart: i + 1,
				GroundingStatus: types.GroundingGrounded,
			}
			repair, ok := emitEvidenceRequiredFlowAssignmentEndpointRepair(ctx, item, i)
			if !ok || repair.receiver != tc.wantLHS || repair.value != tc.wantRHS {
				t.Fatalf("repair=%+v ok=%t, want %q <- %q", repair, ok, tc.wantLHS, tc.wantRHS)
			}
		})
	}

	traceCtx := newEmitCtx()
	traceCtx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		PredicateAxis: types.AxisFlow,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramFlow, Required: true},
	}}
	item := types.EvidenceItem{
		Scope: types.ScopeLine, Kind: types.EvidenceRelationship,
		AnchorKind: types.AnchorAssignment, Subject: "owner", Object: "next",
		Snippet: `state = next`, Source: "src/example", LineStart: 1,
		GroundingStatus: types.GroundingGrounded,
	}
	if repair, ok := emitEvidenceRequiredFlowAssignmentEndpointRepair(traceCtx, item, 0); ok {
		t.Fatalf("Trace causal lane must remain isolated from source-flow endpoint repair: %+v", repair)
	}
}

func TestEmitEvidence_RequiredRelationPublishesExactCallEndpointRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisRegister,
	}}
	fi := &repomap.FileInfo{
		RelPath: "pipeline/runner.py", Language: repomap.LangPython, Package: "pipeline",
		Symbols: []repomap.Symbol{{Name: "run_pipeline", Kind: "function", File: "pipeline/runner.py", Line: 1, EndLine: 3}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "pipeline/runner.py", Line: 2,
			FromEP:     repomap.RelationEndpoint{Name: "run_pipeline", File: "pipeline/runner.py", Line: 2},
			ToEP:       repomap.RelationEndpoint{Name: "handle", Receiver: "plugin", File: "pipeline/runner.py", Line: 2},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "python_call",
		}},
	}
	graph := callTargetTestGraph(fi)
	ctx.MultiGraph = &emitEvidenceSourceGraphStub{graph: graph, file: fi, source: "pipeline/runner.py"}
	seedReadFileHistory(ctx, "repo-b/pipeline/runner.py", 1,
		"def run_pipeline(payload):",
		"    return plugin.handle(payload)",
		"",
	)

	wrong := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"direct","subject":"run_pipeline","source":"repo-b/pipeline/runner.py","line_start":2,"summary":"pipeline invokes the plugin","anchor_kind":"call","anchor_symbol":"plugin.handle"}]}`)
	res, err := tool.Execute(ctx, wrong)
	if err != nil || !res.Success {
		t.Fatalf("call-shaped source observation should remain citable, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "call_endpoint_identity" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("required relation must publish typed exact-call repair: %+v", res.Repair)
	}
	for _, want := range []string{
		"items[0].evidence_kind", "items[0].subject", "items[0].predicate", "items[0].object", "items[0].anchor_symbol",
	} {
		if !slices.Contains(res.Repair.Fields, want) {
			t.Fatalf("call repair fields=%v missing %q", res.Repair.Fields, want)
		}
	}
	for _, want := range []string{
		`evidence_kind="relationship"`, `subject="run_pipeline"`, `predicate="calls"`,
		`object="handle"`, `anchor_symbol="handle"`, `unique qualified callee "plugin.handle"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("call repair hint missing %q: %s", want, res.Repair.Hint)
		}
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("wrong call tuple must remain citable without call authority: %+v", got)
	}

	correct := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"run_pipeline","predicate":"calls","object":"handle","source":"repo-b/pipeline/runner.py","line_start":2,"summary":"pipeline invokes the plugin","anchor_kind":"call","anchor_symbol":"handle"}]}`)
	res, err = tool.Execute(ctx, correct)
	if err != nil || !res.Success {
		t.Fatalf("exact call tuple re-emit failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("exact call re-emit must clear repair debt: %+v", res.Repair)
	}
	got = ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || got[1].AnchorKind != types.AnchorCall ||
		got[1].Subject != "run_pipeline" || got[1].Object != "plugin.handle" {
		t.Fatalf("exact re-emit did not publish call authority: %+v", got)
	}
}

func TestRequiredRelationCallEndpointRepairUsesTypedRelationAxisAndIsTraceIsolated(t *testing.T) {
	item := types.EvidenceItem{
		Scope: types.ScopeLine, Kind: types.EvidenceMechanism,
		AnchorKind: types.AnchorCall, Source: "src/example", LineStart: 3,
		GroundingStatus: types.GroundingGrounded,
	}
	base := types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisCall,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramCallDAG, Required: true},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: base}
	if _, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "A.run", "B.handle", true); !ok {
		t.Fatal("typed required source relation should request exact call repair")
	}
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "A.run", "B.handle", true); ok {
		t.Fatalf("Trace causal lane must remain isolated: %+v", repair)
	}

	for _, axis := range []types.PredicateAxis{
		types.AxisCall, types.AxisRegister, types.AxisConfigure, types.AxisFlow,
	} {
		ctx.AnalysisIR.RequestModel = types.RequestModel{
			Intent:        types.IntentExplain,
			PredicateAxis: axis,
		}
		if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "A.run", "B.handle", true); !ok {
			t.Fatalf("typed %q relation must preserve the exact model-submitted call tuple: %+v", axis, repair)
		}
	}

	ctx.AnalysisIR.RequestModel = base
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisCondition
	if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "A.run", "B.handle", true); ok {
		t.Fatalf("a supporting call must not block a different principal relation axis: %+v", repair)
	}

	ctx.AnalysisIR.RequestModel = types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqRegistration),
		},
	}
	if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "A.run", "B.handle", true); !ok {
		t.Fatalf("typed registration requirement must preserve the exact model-submitted call tuple: %+v", repair)
	}

	ctx.AnalysisIR.RequestModel = types.RequestModel{Intent: types.IntentExplain, PredicateAxis: types.AxisRegister}
	if repair, ok := emitEvidenceRequiredRelationCallEndpointRepair(ctx, item, 0, "", "B.handle", false); ok {
		t.Fatalf("no unique parser tuple must remain advisory and must not invent an edge: %+v", repair)
	}
}

func TestEmitEvidence_CallbackExpressionRequiresSeparateReceiverCallForTypedChain(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}
	fi := &repomap.FileInfo{
		RelPath: "pipeline/runner.py", Language: repomap.LangPython, Package: "pipeline",
		Symbols: []repomap.Symbol{{
			Name: "run_pipeline", Kind: "function", File: "pipeline/runner.py", Line: 1, EndLine: 3,
		}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "pipeline/runner.py", Line: 2,
			FromEP: repomap.RelationEndpoint{Name: "run_pipeline", File: "pipeline/runner.py", Line: 2},
			ToEP: repomap.RelationEndpoint{
				Name: "run_in_executor", Receiver: "loop", File: "pipeline/runner.py", Line: 2,
			},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "python_call",
		}},
	}
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(fi))
	seedReadFileHistory(ctx, "pipeline/runner.py", 1,
		"async def run_pipeline(kind, payload):",
		"    return await loop.run_in_executor(None, plugin.handle, payload)",
		"",
	)

	callbackOnly := json.RawMessage(`{"items":[{
		"scope":"line","evidence_kind":"relationship","subject":"run_pipeline",
		"predicate":"calls","object":"plugin.handle","source":"pipeline/runner.py","line_start":2,
		"summary":"the pipeline passes the plugin handler to the executor","anchor_kind":"call",
		"anchor_symbol":"plugin.handle","snippet":"return await loop.run_in_executor(None, plugin.handle, payload)"
	}]}`)
	res, err := tool.Execute(ctx, callbackOnly)
	if err != nil || !res.Success {
		t.Fatalf("callback evidence should be accepted with a sibling repair, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "callback_receiver_call_pair" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("typed chain must request the exact caller -> receiver sibling: %+v", res.Repair)
	}
	for _, want := range []string{
		"callback handoff", `subject="run_pipeline"`, `predicate="calls"`,
		`object="run_in_executor"`, `anchor_symbol="run_in_executor"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("callback pair hint missing %q: %s", want, res.Repair.Hint)
		}
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorCallback ||
		got[0].Subject != "loop.run_in_executor" || got[0].Object != "plugin.handle" {
		t.Fatalf("callback half must remain accepted without a system-authored sibling: %+v", got)
	}

	directReceiverCall := json.RawMessage(`{"items":[{
		"scope":"line","evidence_kind":"relationship","subject":"run_pipeline",
		"predicate":"calls","object":"run_in_executor","source":"pipeline/runner.py","line_start":2,
		"summary":"run_pipeline invokes the executor API","anchor_kind":"call",
		"anchor_symbol":"run_in_executor","snippet":"return await loop.run_in_executor(None, plugin.handle, payload)"
	}]}`)
	res, err = tool.Execute(ctx, directReceiverCall)
	if err != nil || !res.Success {
		t.Fatalf("model re-emitted receiver call failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("exact receiver call must not create another action-required repair: %+v", res.Repair)
	}
	got = ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || got[1].AnchorKind != types.AnchorCall ||
		got[1].Subject != "run_pipeline" || got[1].Object != "loop.run_in_executor" {
		t.Fatalf("model-authored direct receiver call was not preserved: %+v", got)
	}
}

func TestCallbackReceiverCallRepairIsSchemaBoundTraceIsolatedAndDuplicateAware(t *testing.T) {
	fi := &repomap.FileInfo{
		RelPath: "Runner.ets", Language: repomap.LangArkTS,
		Symbols: []repomap.Symbol{{Name: "run", Kind: "method", Parent: "Runner", Line: 1, EndLine: 3}},
		Relations: []repomap.Relation{{
			Kind: "call", File: "Runner.ets", Line: 2,
			FromEP:     repomap.RelationEndpoint{Name: "run", Receiver: "Runner", Line: 2},
			ToEP:       repomap.RelationEndpoint{Name: "submit", Receiver: "queue", Line: 2},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "arkts_call",
		}},
	}
	gc := &ground.Context{
		Graph: callTargetTestGraph(fi),
		LineIndex: map[string]map[int]string{
			"Runner.ets": {2: "queue.submit(worker.run, request);"},
		},
	}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "Runner.ets", LineStart: 2, LineEnd: 2,
		AnchorKind: types.AnchorCallback, AnchorSymbol: "worker.run",
		Subject: "queue.submit", Predicate: "passes callback", Object: "worker.run",
		GroundingStatus: types.GroundingGrounded,
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisCall,
	}}
	repair, ok := emitEvidenceRequiredCallbackReceiverCallRepair(ctx, item, 0, gc)
	if !ok || repair.caller != "Runner.run" || repair.callee != "queue.submit" || !repair.callbackReceiverPair {
		t.Fatalf("cross-language callback receiver tuple=%+v ok=%t", repair, ok)
	}

	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisCondition
	if got, ok := emitEvidenceRequiredCallbackReceiverCallRepair(ctx, item, 0, gc); ok {
		t.Fatalf("incidental callback must not block a non-relation question: %+v", got)
	}
	ctx.AnalysisIR.RequestModel = types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall}
	if got, ok := emitEvidenceRequiredCallbackReceiverCallRepair(ctx, item, 0, gc); ok {
		t.Fatalf("Runtime Trace causal lane must remain isolated: %+v", got)
	}

	direct := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "Runner.ets", LineStart: 2, LineEnd: 2,
		AnchorKind: types.AnchorCall, AnchorSymbol: "queue.submit",
		Subject: "Runner.run", Predicate: "calls", Object: "queue.submit",
		GroundingStatus: types.GroundingGrounded,
	}
	if pending := filterSatisfiedCallbackReceiverCallRepairs(
		[]emitEvidenceCallEndpointRepair{repair}, []types.EvidenceItem{item, direct},
	); len(pending) != 0 {
		t.Fatalf("an already model-authored sibling call must suppress duplicate repair: %+v", pending)
	}
}

func TestEmitEvidence_AcceptsExactArgumentFlowWithoutMintingCalleeUse(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 8026,
		"agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stage)")
	params := json.RawMessage(`{"items":[{
		"scope":"line","evidence_kind":"relationship","subject":"o.busCtx",
		"predicate":"passes argument","object":"ctxbuilder.BuildAgentContext",
		"source":"internal/orchestrator/orchestrator.go","line_start":8026,
		"summary":"the current bus context is passed into agent context construction",
		"anchor_kind":"argument","anchor_symbol":"o.busCtx"
	}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("argument evidence failed, err=%v result=%+v", err, res)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorArgument ||
		got[0].Subject != "o.busCtx" || got[0].Object != "ctxbuilder.BuildAgentContext" ||
		types.ClaimFormOf(got[0]) != types.ClaimArgumentFlow {
		t.Fatalf("exact argument tuple was not preserved: %+v", got)
	}
	if strings.Contains(strings.ToLower(got[0].Predicate), "stores") || strings.Contains(strings.ToLower(got[0].Predicate), "mutates") {
		t.Fatalf("tool minted unsupported callee-side semantics: %+v", got[0])
	}
}

func TestEmitEvidence_RequiredFlowCallPublishesTypedArgumentSiblingRepairAcrossLanguages(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		language     string
		line         string
		owner        string
		method       string
		binding      string
		argument     string
		callee       string
		calleeName   string
		calleeRecv   string
		declaredType string
	}{
		{name: "go", source: "pipeline.go", language: repomap.LangGo, line: "agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, agentName, stage)", owner: "Orchestrator", method: "dispatchStage", binding: "busCtx", argument: "o.busCtx", callee: "ctxbuilder.BuildAgentContext", calleeName: "BuildAgentContext", calleeRecv: "ctxbuilder", declaredType: "*types.BusContext"},
		{name: "python", source: "pipeline.py", language: repomap.LangPython, line: "agent_ctx = builder.build(self.bus_ctx, agent_name, stage)", owner: "Pipeline", method: "dispatch", binding: "bus_ctx", argument: "self.bus_ctx", callee: "builder.build", calleeName: "build", calleeRecv: "builder", declaredType: "BusContext"},
		{name: "arkts", source: "pipeline.ets", language: repomap.LangArkTS, line: "const agentCtx = builder.build(this.busContext, agentName, stage);", owner: "Pipeline", method: "dispatch", binding: "busContext", argument: "this.busContext", callee: "builder.build", calleeName: "build", calleeRecv: "builder", declaredType: "BusContext"},
		{name: "cangjie", source: "pipeline.cj", language: repomap.LangCangjie, line: "let agentCtx = builder.build(this.busContext, agentName, stage)", owner: "Pipeline", method: "dispatch", binding: "busContext", argument: "this.busContext", callee: "builder.build", calleeName: "build", calleeRecv: "builder", declaredType: "BusContext"},
		{name: "cpp", source: "pipeline.cpp", language: repomap.LangCpp, line: "auto agentCtx = builder.build(this->busContext_, agentName, stage);", owner: "Pipeline", method: "dispatch", binding: "busContext_", argument: "this->busContext_", callee: "builder.build", calleeName: "build", calleeRecv: "builder", declaredType: "BusContext*"},
		{name: "rust", source: "pipeline.rs", language: repomap.LangRust, line: "let agent_ctx = builder.build(&self.bus_context, agent_name, stage);", owner: "Pipeline", method: "dispatch", binding: "bus_context", argument: "&self.bus_context", callee: "builder.build", calleeName: "build", calleeRecv: "builder", declaredType: "BusContext"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
					Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			fi := &repomap.FileInfo{
				RelPath: tc.source, Language: tc.language,
				Symbols: []repomap.Symbol{
					{Name: tc.binding, Kind: "field", Parent: tc.owner, DeclaredType: tc.declaredType, Line: 5, EndLine: 5},
					{Name: tc.method, Kind: "method", Parent: tc.owner, Line: 10, EndLine: 30},
				},
				Relations: []repomap.Relation{{
					Kind: "call", File: tc.source, Line: 20,
					FromEP:     repomap.RelationEndpoint{Name: tc.method, Receiver: tc.owner, File: tc.source, Line: 20},
					ToEP:       repomap.RelationEndpoint{Name: tc.calleeName, Receiver: tc.calleeRecv, File: tc.source, Line: 20},
					Confidence: 1, Provenance: "tree_sitter", ResolvedBy: tc.language + "_call",
				}},
			}
			ctx.Mutable.SetSearchGraph(callTargetTestGraph(fi))
			seedReadFileHistory(ctx, tc.source, 20, tc.line)
			callPayload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":%q,"predicate":"calls","object":%q,"source":%q,"line_start":20,"summary":"dispatch invokes the context builder","anchor_kind":"call","anchor_symbol":%q,"snippet":%q}]}`,
				tc.owner+"."+tc.method, tc.callee, tc.source, tc.callee, tc.line))
			res, err := tool.Execute(ctx, callPayload)
			if err != nil || !res.Success {
				t.Fatalf("call row rejected, err=%v result=%+v", err, res)
			}
			if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "call_argument_flow_pair" ||
				res.Repair.Metadata["completion_blocking"] != "true" {
				t.Fatalf("missing typed argument sibling repair: %+v", res.Repair)
			}
			for _, want := range []string{`anchor_kind="argument"`, fmt.Sprintf(`subject=%q`, tc.argument), fmt.Sprintf(`object=%q`, tc.callee)} {
				if !strings.Contains(res.Repair.Hint, want) {
					t.Fatalf("repair missing %q: %s", want, res.Repair.Hint)
				}
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 || got[0].AnchorKind != types.AnchorCall {
				t.Fatalf("system must retain only the model-authored call before repair: %+v", got)
			}

			argumentPayload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":%q,"predicate":"passes argument","object":%q,"source":%q,"line_start":20,"summary":"the carrier is passed to the builder","anchor_kind":"argument","anchor_symbol":%q,"snippet":%q}]}`,
				tc.argument, tc.callee, tc.source, tc.argument, tc.line))
			res, err = tool.Execute(ctx, argumentPayload)
			if err != nil || !res.Success {
				t.Fatalf("argument sibling rejected, err=%v result=%+v", err, res)
			}
			if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
				t.Fatalf("exact argument sibling must close current repair: %+v", res.Repair)
			}
			got = ctx.Mutable.EmittedEvidence()
			if len(got) != 2 || got[1].AnchorKind != types.AnchorArgument || types.ClaimFormOf(got[1]) != types.ClaimArgumentFlow {
				t.Fatalf("model-authored argument sibling did not become typed authority: %+v", got)
			}
		})
	}
}

func TestEmitEvidence_RequiredFlowCallFindsUniqueReceiverFieldAcrossFiles(t *testing.T) {
	const (
		callSource = "internal/orchestrator/extract_work.go"
		declSource = "internal/orchestrator/orchestrator.go"
		line       = "ac := ctxbuilder.BuildAgentContext(o.busCtx, types.AgentExtractor, types.StageExtract)"
	)
	callFile := &repomap.FileInfo{
		RelPath: callSource, Language: repomap.LangGo, Package: "orchestrator",
		Symbols: []repomap.Symbol{
			{Name: "extractStageHasRequiredWork", Kind: "method", Receiver: "Orchestrator", Line: 10, EndLine: 35},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: callSource, Line: 15,
			FromEP:     repomap.RelationEndpoint{Name: "extractStageHasRequiredWork", Receiver: "Orchestrator", File: callSource, Line: 15},
			ToEP:       repomap.RelationEndpoint{Name: "BuildAgentContext", Receiver: "ctxbuilder", File: callSource, Line: 15},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call",
		}},
	}
	declarationFile := &repomap.FileInfo{
		RelPath: declSource, Language: repomap.LangGo, Package: "orchestrator",
		Symbols: []repomap.Symbol{{
			Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", File: declSource, Line: 55, EndLine: 55,
		}},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}}},
	}}
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(callFile, declarationFile))
	seedReadFileHistory(ctx, callSource, 15, line)

	payload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"Orchestrator.extractStageHasRequiredWork","predicate":"calls","object":"ctxbuilder.BuildAgentContext","source":%q,"line_start":15,"summary":"builds the stage context","anchor_kind":"call","anchor_symbol":"ctxbuilder.BuildAgentContext","snippet":%q}]}`,
		callSource, line))
	res, err := (&EmitEvidence{}).Execute(ctx, payload)
	if err != nil || !res.Success {
		t.Fatalf("cross-file operation evidence failed, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "call_argument_flow_pair" ||
		!strings.Contains(res.Repair.Hint, `subject="o.busCtx"`) ||
		!strings.Contains(res.Repair.Hint, `object="ctxbuilder.BuildAgentContext"`) {
		t.Fatalf("unique same-package receiver field must expose the exact argument sibling: %+v", res.Repair)
	}

	// A same-named receiver field from another namespace is not an identity
	// bridge, even when it is the only other declaration in the repository.
	declarationFile.Package = "other"
	ctx = newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}}},
	}}
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(callFile, declarationFile))
	seedReadFileHistory(ctx, callSource, 15, line)
	res, err = (&EmitEvidence{}).Execute(ctx, payload)
	if err != nil || !res.Success {
		t.Fatalf("cross-package negative evidence failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_scope"] == "call_argument_flow_pair" {
		t.Fatalf("cross-package homonym must stay fail-closed: %+v", res.Repair)
	}
}

func TestEmitEvidence_RequiredFlowCallRepairsCarrierAndVerifiedStageArgumentsTogether(t *testing.T) {
	const (
		callSource  = "internal/orchestrator/extract_work.go"
		declSource  = "internal/orchestrator/orchestrator.go"
		buildSource = "internal/context/builder.go"
		line        = "ac := ctxbuilder.BuildAgentContext(o.busCtx, types.AgentExtractor, types.StageExtract)"
	)
	callFile := &repomap.FileInfo{
		RelPath: callSource, Language: repomap.LangGo, Package: "orchestrator",
		Imports: []repomap.Import{{Path: "github.com/hanchaoqun/codrax/internal/context", Alias: "ctxbuilder"}},
		Symbols: []repomap.Symbol{{Name: "extractStageHasRequiredWork", Kind: "method", Receiver: "Orchestrator", Line: 10, EndLine: 35}},
		Relations: []repomap.Relation{{
			Kind: "call", File: callSource, Line: 15,
			FromEP:     repomap.RelationEndpoint{Name: "extractStageHasRequiredWork", Receiver: "Orchestrator", File: callSource, Line: 15},
			ToEP:       repomap.RelationEndpoint{Name: "BuildAgentContext", Receiver: "ctxbuilder", File: callSource, Line: 15},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call",
		}},
	}
	declarationFile := &repomap.FileInfo{
		RelPath: declSource, Language: repomap.LangGo, Package: "orchestrator",
		Symbols: []repomap.Symbol{{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", File: declSource, Line: 55, EndLine: 55}},
	}
	buildFile := &repomap.FileInfo{
		RelPath: buildSource, Language: repomap.LangGo, Package: "context",
		Symbols: []repomap.Symbol{{Name: "BuildAgentContext", Kind: "function", File: buildSource, Line: 26, EndLine: 80}},
	}
	graph := callTargetTestGraph(callFile, declarationFile, buildFile)
	graph.ResolvedImports = map[string][]repomap.ResolvedImportBinding{
		callSource: {{Import: callFile.Imports[0], Targets: []string{buildSource}}},
	}
	ctx := newEmitCtx()
	ctx.RepoRoot = "../.."
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: []types.DiagramParticipantHint{
			{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		}},
	}}
	ctx.Mutable.SetSearchGraph(graph)
	seedReadFileHistory(ctx, callSource, 15, line)
	payload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"Orchestrator.extractStageHasRequiredWork","predicate":"calls","object":"ctxbuilder.BuildAgentContext","source":%q,"line_start":15,"summary":"builds the extractor context","anchor_kind":"call","anchor_symbol":"ctxbuilder.BuildAgentContext","snippet":%q}]}`,
		callSource, line))
	res, err := (&EmitEvidence{}).Execute(ctx, payload)
	if err != nil || !res.Success {
		t.Fatalf("flow call failed, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "call_argument_flow_pair" {
		t.Fatalf("expected exact argument repair, got %+v", res.Repair)
	}
	for _, want := range []string{`subject="o.busCtx"`, `subject="types.AgentExtractor"`, `object="ctxbuilder.BuildAgentContext"`} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("combined carrier/stage repair missing %q: %s", want, res.Repair.Hint)
		}
	}
	if strings.Contains(res.Repair.Hint, `subject="types.StageExtract"`) {
		t.Fatalf("one verified stage participant should produce one exact argument repair: %s", res.Repair.Hint)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Object != "ctxbuilder.BuildAgentContext" {
		t.Fatalf("exact already-read source qualifier must survive a tail-only resolved target: %+v", got)
	}
}

func TestRequiredCallArgumentFlowRepairIsTypedParticipantAndTraceIsolated(t *testing.T) {
	const source = "pipeline.go"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "dispatchStage", Kind: "method", Parent: "Orchestrator", Line: 10, EndLine: 30},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 20,
			FromEP:     repomap.RelationEndpoint{Name: "dispatchStage", Receiver: "Orchestrator", Line: 20},
			ToEP:       repomap.RelationEndpoint{Name: "BuildAgentContext", Receiver: "ctxbuilder", Line: 20},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call",
		}},
	}
	gc := &ground.Context{
		Graph:     callTargetTestGraph(fi),
		LineIndex: map[string]map[int]string{source: {20: "agentCtx := ctxbuilder.BuildAgentContext(o.busCtx, stage)"}},
	}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 20,
		AnchorKind: types.AnchorCall, AnchorSymbol: "ctxbuilder.BuildAgentContext",
		Subject: "Orchestrator.dispatchStage", Predicate: "calls", Object: "ctxbuilder.BuildAgentContext",
		GroundingStatus: types.GroundingGrounded,
	}
	for _, tc := range []struct {
		name        string
		intent      types.Intent
		required    bool
		participant string
		want        bool
	}{
		{name: "exact typed carrier", intent: types.IntentExplain, required: true, participant: "BusContext", want: true},
		{name: "different participant", intent: types.IntentExplain, required: true, participant: "OtherContext"},
		{name: "optional diagram", intent: types.IntentExplain, participant: "BusContext"},
		{name: "trace isolation", intent: types.IntentTrace, required: true, participant: "BusContext"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: tc.intent, PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: tc.required, Participants: []types.DiagramParticipantHint{{
					Identity: tc.participant, Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			got := emitEvidenceRequiredCallArgumentFlowRepairs(ctx, item, 0, gc)
			if (len(got) > 0) != tc.want {
				t.Fatalf("repairs=%+v want=%t", got, tc.want)
			}
		})
	}
}

func TestRequiredAssignmentCallArgumentFlowRepairUsesUniqueParserCall(t *testing.T) {
	const source = "pipeline.go"
	line := "o.busCtx.EvidenceItems, changed = agent.MergeEvidenceItemsIfChanged(o.busCtx.EvidenceItems, output.EvidenceItems)"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "apply", Kind: "method", Parent: "Orchestrator", Line: 10, EndLine: 30},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 20,
			FromEP:     repomap.RelationEndpoint{Name: "apply", Receiver: "Orchestrator", Line: 20},
			ToEP:       repomap.RelationEndpoint{Name: "MergeEvidenceItemsIfChanged", Receiver: "agent", Line: 20},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call",
		}},
	}
	gc := &ground.Context{
		Graph:     callTargetTestGraph(fi),
		LineIndex: map[string]map[int]string{source: {20: line}},
	}
	item := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 20,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "o.busCtx.EvidenceItems",
		Subject: "o.busCtx", Predicate: "merges", Object: "output.EvidenceItems", Snippet: line,
		GroundingStatus: types.GroundingGrounded,
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}}},
	}}
	got := emitEvidenceRequiredAssignmentCallArgumentFlowRepairs(ctx, item, 0, gc)
	if len(got) != 1 || got[0].argument != "o.busCtx.EvidenceItems" ||
		got[0].receiver != "agent.MergeEvidenceItemsIfChanged" || !got[0].assignmentCallCompanion {
		t.Fatalf("assignment companion repairs=%+v", got)
	}
	repair := buildEmitEvidenceArgumentFlowRepair(got)
	if repair == nil || repair.Metadata["repair_scope"] != "assignment_call_argument_flow_pair" ||
		!strings.Contains(repair.Hint, "contains one unique parser-owned call") {
		t.Fatalf("assignment companion repair contract=%+v", repair)
	}

	fi.Relations = append(fi.Relations, repomap.Relation{
		Kind: "call", File: source, Line: 20,
		FromEP: repomap.RelationEndpoint{Name: "apply", Receiver: "Orchestrator", Line: 20},
		ToEP:   repomap.RelationEndpoint{Name: "Other", Receiver: "agent", Line: 20},
	})
	if got := emitEvidenceRequiredAssignmentCallArgumentFlowRepairs(ctx, item, 0, gc); len(got) != 0 {
		t.Fatalf("multiple same-line calls must fail open, got %+v", got)
	}

	ctx.AnalysisIR.RequestModel = types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Required: true}}
	fi.Relations = fi.Relations[:1]
	if got := emitEvidenceRequiredAssignmentCallArgumentFlowRepairs(ctx, item, 0, gc); len(got) != 0 {
		t.Fatalf("Trace causal lane must remain isolated: %+v", got)
	}
}

func TestRequiredCallResultAssignmentRepairContinuesSelectedValuePathAcrossLanguages(t *testing.T) {
	for _, language := range repomap.SupportedReadLanguages() {
		t.Run(string(language), func(t *testing.T) {
			const source = "src/dispatcher.source"
			const buildLine = "ctx = builder.Build(bus)"
			const executeLine = "output = agent.Execute(ctx)"
			fi := &repomap.FileInfo{
				RelPath: source, Language: language,
				Symbols: []repomap.Symbol{
					{Name: "bus", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 2},
					{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 30},
				},
				Relations: []repomap.Relation{
					{Kind: "call", File: source, Line: 10,
						FromEP: repomap.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 10},
						ToEP:   repomap.RelationEndpoint{Name: "Build", Receiver: "builder", Line: 10}},
					{Kind: "call", File: source, Line: 20,
						FromEP: repomap.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: 20},
						ToEP:   repomap.RelationEndpoint{Name: "Execute", Receiver: "agent", Line: 20}},
				},
				LineFeatures: map[int][]repomap.LineFeature{
					10: {repomap.LineFeatureAssignment},
					20: {repomap.LineFeatureAssignment},
				},
			}
			graph := callTargetTestGraph(fi)
			gc := &ground.Context{Graph: graph, LineIndex: map[string]map[int]string{
				source: {10: buildLine, 20: executeLine},
			}}
			ctx := newEmitCtx()
			ctx.Mutable.SetSearchGraph(graph)
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
				AnalyzerHints: types.AnalyzerHints{EntityProvenance: []types.EntityProvenance{{
					Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol,
					Resolved: true, UseForSearch: true, UseForShape: true,
				}}},
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
					Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
				}}},
			}}
			busArgument := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 10,
				AnchorKind: types.AnchorArgument, AnchorSymbol: "bus", Subject: "bus", Predicate: "passes argument", Object: "builder.Build",
				Snippet: buildLine, GroundingStatus: types.GroundingGrounded, Producer: EmitEvidenceProducer,
				DeclaredIdentityBindings: []types.EvidenceDeclaredIdentityBinding{{Binding: "bus", Type: "BusContext"}},
			}

			first := emitEvidenceRequiredCallResultAssignmentFlowRepairs(
				ctx, []types.EvidenceItem{busArgument}, []int{4}, []types.EvidenceItem{busArgument}, gc,
			)
			if len(first) != 1 || first[0].receiver != "ctx" || first[0].value != "builder.Build" ||
				first[0].anchor != types.AnchorAssignment || first[0].itemIndex != 4 {
				t.Fatalf("participant-selected call result repair=%+v", first)
			}

			ctxAssignment := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 10,
				AnchorKind: types.AnchorAssignment, AnchorSymbol: "ctx", Subject: "ctx", Predicate: "assigns", Object: "builder.Build",
				Snippet: buildLine, GroundingStatus: types.GroundingGrounded, Producer: EmitEvidenceProducer,
			}
			executeArgument := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 20,
				AnchorKind: types.AnchorArgument, AnchorSymbol: "ctx", Subject: "ctx", Predicate: "passes argument", Object: "agent.Execute",
				Snippet: executeLine, GroundingStatus: types.GroundingGrounded, Producer: EmitEvidenceProducer,
			}
			evidence := []types.EvidenceItem{busArgument, ctxAssignment, executeArgument}
			second := emitEvidenceRequiredCallResultAssignmentFlowRepairs(
				ctx, []types.EvidenceItem{executeArgument}, []int{7}, evidence, gc,
			)
			if len(second) != 1 || second[0].receiver != "output" || second[0].value != "agent.Execute" ||
				second[0].itemIndex != 7 {
				t.Fatalf("selected consumer result repair=%+v", second)
			}

			outputAssignment := types.EvidenceItem{
				Kind: types.EvidenceRelationship, Scope: types.ScopeLine, Source: source, LineStart: 20,
				AnchorKind: types.AnchorAssignment, AnchorSymbol: "output", Subject: "output", Predicate: "assigns", Object: "agent.Execute",
				Snippet: executeLine, GroundingStatus: types.GroundingGrounded, Producer: EmitEvidenceProducer,
			}
			if got := filterSatisfiedCallResultAssignmentRepairs(second, append(evidence, outputAssignment)); len(got) != 0 {
				t.Fatalf("model-authored exact assignment must satisfy companion debt: %+v", got)
			}
			contract := buildEmitEvidenceCallResultAssignmentRepair(second)
			if contract == nil || contract.Metadata["repair_scope"] != "selected_call_result_assignment_pair" ||
				!strings.Contains(contract.Hint, `subject="output"`) {
				t.Fatalf("call-result companion contract=%+v", contract)
			}
		})
	}
}

func TestRequiredCallResultAssignmentRepairKeepsTraceLaneIsolated(t *testing.T) {
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Required: true},
	}}
	if got := emitEvidenceRequiredCallResultAssignmentFlowRepairs(
		ctx, []types.EvidenceItem{{}}, []int{0}, nil, &ground.Context{},
	); len(got) != 0 {
		t.Fatalf("Trace causal lane must remain isolated: %+v", got)
	}
}

func TestEmitEvidencePublishesCallResultAssignmentCompanionWithoutMintingEdge(t *testing.T) {
	const source = "internal/pipeline.go"
	const lineNumber = 10
	const line = "ctx := builder.Build(bus)"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "bus", Kind: "field", Parent: "Pipeline", DeclaredType: "BusContext", Line: 2},
			{Name: "run", Kind: "method", Receiver: "Pipeline", Line: 1, EndLine: 20},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: lineNumber,
			FromEP: repomap.RelationEndpoint{Name: "run", Receiver: "Pipeline", Line: lineNumber},
			ToEP:   repomap.RelationEndpoint{Name: "Build", Receiver: "builder", Line: lineNumber},
		}},
		LineFeatures: map[int][]repomap.LineFeature{lineNumber: {repomap.LineFeatureAssignment}},
	}
	ctx := newEmitCtx()
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(fi))
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{EntityProvenance: []types.EntityProvenance{{
			Surface: "BusContext", ResolvedAs: "BusContext", Resolution: types.EntityResolutionSymbol,
			Resolved: true, UseForSearch: true, UseForShape: true,
		}}},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}}},
	}}
	seedReadFileHistory(ctx, source, lineNumber, line)
	params := json.RawMessage(fmt.Sprintf(`{"items":[{
		"scope":"line","evidence_kind":"relationship","subject":"bus","predicate":"passes argument",
		"object":"builder.Build","source":%q,"line_start":%d,"summary":"pass shared context",
		"anchor_kind":"argument","anchor_symbol":"bus"}]}`, source, lineNumber))
	res, err := (&EmitEvidence{}).Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("emit evidence failed: err=%v result=%+v", err, res)
	}
	if res.Repair == nil || !strings.Contains(res.Repair.Hint, `subject="ctx"`) ||
		!strings.Contains(res.Repair.Hint, `object="builder.Build"`) {
		t.Fatalf("tool must publish exact additional assignment recipe: %+v", res.Repair)
	}
	obligations, ok := decodeEmitEvidenceRelationRepairObligations(
		res.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey],
	)
	if !ok {
		t.Fatalf("missing durable companion obligation: %+v", res.Repair.Metadata)
	}
	found := false
	for _, obligation := range obligations {
		if obligation.AnchorKind == types.AnchorAssignment && obligation.Subject == "ctx" &&
			obligation.Object == "builder.Build" {
			found = true
		}
	}
	if !found {
		t.Fatalf("assignment companion obligation not wired: %+v", obligations)
	}
	for _, item := range ctx.Mutable.EmittedEvidence() {
		if item.AnchorKind == types.AnchorAssignment && item.Subject == "ctx" {
			t.Fatalf("system must not mint the missing edge: %+v", item)
		}
	}
}

func TestEmitEvidence_RequiredFlowRepairsMultiResultAssignmentAndNestedArgumentTogether(t *testing.T) {
	const source = "pipeline.go"
	line := "o.busCtx.EvidenceItems, changed = agent.MergeEvidenceItemsIfChanged(o.busCtx.EvidenceItems, output.EvidenceItems)"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{
			{Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext", Line: 5, EndLine: 5},
			{Name: "apply", Kind: "method", Parent: "Orchestrator", Line: 10, EndLine: 30},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 20,
			FromEP:     repomap.RelationEndpoint{Name: "apply", Receiver: "Orchestrator", Line: 20},
			ToEP:       repomap.RelationEndpoint{Name: "MergeEvidenceItemsIfChanged", Receiver: "agent", Line: 20},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_call",
		}},
	}
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: []types.DiagramParticipantHint{{
			Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
		}}},
	}}
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(fi))
	seedReadFileHistory(ctx, source, 20, line)

	res, err := tool.Execute(ctx, json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"o.busCtx","predicate":"merges","object":"output.EvidenceItems","source":%q,"line_start":20,"summary":"merge evidence into shared state","anchor_kind":"assignment","anchor_symbol":"o.busCtx.EvidenceItems","snippet":%q}]}`, source, line)))
	if err != nil || !res.Success {
		t.Fatalf("broad multi-result row failed unexpectedly, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "assignment_endpoint_identity+assignment_call_argument_flow_pair" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("multi-shape line must return both exact obligations: %+v", res.Repair)
	}
	for _, want := range []string{
		`subject="o.busCtx.EvidenceItems" (exact LHS receiver)`,
		`object="agent.MergeEvidenceItemsIfChanged" (exact RHS value/source)`,
		`anchor_kind="argument"`,
		`subject="o.busCtx.EvidenceItems"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("combined repair missing %q: %s", want, res.Repair.Hint)
		}
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorTextReference {
		t.Fatalf("system must not mint either directed edge before model re-emit: %+v", got)
	}

	corrected := json.RawMessage(fmt.Sprintf(`{"items":[
		{"scope":"line","evidence_kind":"relationship","subject":"o.busCtx.EvidenceItems","predicate":"assigns","object":"agent.MergeEvidenceItemsIfChanged","source":%q,"line_start":20,"summary":"merge result enters shared state","anchor_kind":"assignment","anchor_symbol":"o.busCtx.EvidenceItems","snippet":%q},
		{"scope":"line","evidence_kind":"relationship","subject":"o.busCtx.EvidenceItems","predicate":"passes argument","object":"agent.MergeEvidenceItemsIfChanged","source":%q,"line_start":20,"summary":"existing evidence enters merge operation","anchor_kind":"argument","anchor_symbol":"o.busCtx.EvidenceItems","snippet":%q}
	]}`, source, line, source, line))
	res, err = tool.Execute(ctx, corrected)
	if err != nil || !res.Success {
		t.Fatalf("corrected companion rows rejected, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("model-authored exact siblings must close combined debt: %+v", res.Repair)
	}
}

func TestEmitEvidence_ConfigCommentLineBecomesIllustrativeOnly(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[codrax.yaml.example: showing lines 22-22 of 801]\n" +
					"  22│ #   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags\n",
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context for config-file precedence comment", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want config for config-file precedence comment", got[0].DiagramRole)
	}
	if strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("config-file precedence comment should remain usable context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should not tell the model to drop config-file precedence comments, got: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigPrecedenceCommentCanGroundWithoutExactAnchorSymbol(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[codrax.yaml.example: showing lines 20-22 of 801]\n" +
					"  20│ # Precedence (lowest wins last):\n" +
					"  21│ #\n" +
					"  22│ #   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags\n",
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "codrax.yaml",
				"object": "command-line flags",
				"predicate": "precedes",
				"source": "codrax.yaml.example",
				"line_start": 22,
				"summary": "code defaults < codrax.yaml < command-line flags",
				"anchor_kind": "definition",
				"anchor_symbol": "exeDir",
				"context_role_hint": "related_context",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("grounding status = %q, want grounded", got[0].GroundingStatus)
	}
	if got[0].GroundingTier != types.TierLineText {
		t.Fatalf("grounding tier = %q, want line_text", got[0].GroundingTier)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleConfig {
		t.Fatalf("diagram role = %q, want config", got[0].DiagramRole)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("grounded config precedence comment must not queue repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_AuxiliarySourceDeclarationIsNotPathOnlyIllustrative(t *testing.T) {
	paths := []string{
		"tests/fixtures/entry.ets",
		"examples/entry.ets",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/entry.ets",
		"vendor/acme/entry.ets",
		"generated/entry.ets",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			seedReadFileHistory(ctx, path, 12, "export struct EntryComponent {")
			params := json.RawMessage(fmt.Sprintf(`{
				"items": [
					{
						"kind": "direct",
						"subject": "EntryComponent",
						"predicate": "defines",
						"source": %q,
						"line_start": 12,
						"summary": "EntryComponent is a real auxiliary source declaration.",
						"anchor_kind": "definition",
						"anchor_symbol": "EntryComponent",
						"context_role_hint": "defining"
					}
				]
			}`, path))
			res, err := tool.Execute(ctx, params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Success {
				t.Fatalf("expected success, got: %s", res.Summary)
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 {
				t.Fatalf("want 1 item in buffer, got %d", len(got))
			}
			if got[0].ContextRole != types.EvidenceContextRoleDefining {
				t.Fatalf("context role = %q, want defining for real auxiliary source declaration", got[0].ContextRole)
			}
			if got[0].Kind == types.EvidenceUnresolved || got[0].GroundingStatus == types.GroundingUngrounded {
				t.Fatalf("auxiliary source declaration must not be downgraded as illustrative path noise: kind=%q grounding=%q note=%q", got[0].Kind, got[0].GroundingStatus, got[0].GroundingNote)
			}
			if strings.Contains(strings.ToLower(got[0].GroundingNote), "illustrative") {
				t.Fatalf("grounding note should not mark real source declaration illustrative: %q", got[0].GroundingNote)
			}
		})
	}
}

func TestEmitEvidence_KeepsFreeformExactMentionAsRelatedContextWithoutAnchoredTargetWindow(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 42,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "context only") {
		t.Fatalf("freeform summary-only exact mention should stay non-defining context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "context only") {
		t.Fatalf("tool summary should still surface context-only guidance, got: %q", res.Summary)
	}
}

func TestEmitEvidence_PositiveAnchoredWindowDoesNotMintAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[internal/agent/router.go: showing lines 40-42 of 60]\n" +
					"  40│func buildFallbackHandlers() {\n" +
					"  41│    // FooHandler is absent from the runtime router on this branch.\n" +
					"  42│    _ = 0\n",
			},
		},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "FooHandler 在哪里定义？",
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FooHandler"},
				ExactTargets:    []string{"FooHandler"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectFunctionName,
				TargetLabel:  "symbol",
				Targets:      []string{"FooHandler"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "mechanism",
				"subject": "buildFallbackHandlers",
				"predicate": "describes",
				"source": "internal/agent/router.go",
				"line_start": 40,
				"summary": "The fallback path explains why FooHandler is absent from the runtime router.",
				"anchor_kind": "definition",
				"anchor_symbol": "buildFallbackHandlers",
				"context_role_hint": "related_context"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if strings.Contains(strings.ToLower(got[0].GroundingNote), "absence support") {
		t.Fatalf("positive anchored target mention must not be relabelled as absence support, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "positive related context") {
		t.Fatalf("grounding note should preserve the positive-occurrence boundary, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(res.Summary), "absence support") {
		t.Fatalf("tool summary must not teach an absence conclusion from a positive source occurrence, got: %q", res.Summary)
	}
}

func TestEmitEvidence_NegativeAbsenceProbeDoesNotQueueLineRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{
			{
				ToolName: "read_file",
				Success:  true,
				Summary: "[internal/types/config.go: showing lines 768-770 of 1188]\n" +
					"  768│ type ExploreHeuristics struct {\n" +
					"  769│ \t// --- Mid-loop thresholds ---\n" +
					"  770│ \n",
			},
		},
	})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"explore_mid_loop_hint_budget"},
				AllowAbsence: true,
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"object": "ExploreHeuristics",
				"predicate": "absent_from",
				"source": "internal/types/config.go",
				"line_start": 768,
				"summary": "explore_mid_loop_hint_budget 不存在于 ExploreHeuristics 中",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "absence_support"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("negative exact-absence probe should be marked non-repairable, got: %q", got[0].GroundingNote)
	}
	if res.Repair != nil && len(res.Repair.Targets) > 0 {
		t.Fatalf("negative exact-absence probe must not queue repair targets, got %+v", res.Repair.Targets)
	}
}

func TestEmitEvidence_DowngradesIllustrativeExactTargetMentionWithoutPendingFinding(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "appears_in",
				"source": "internal/skill/analysis_contract.go",
				"line_start": 367,
				"summary": "analysis_contract.go uses explore_mid_loop_hint_budget only as a docstring example",
				"anchor_kind": "definition",
				"anchor_symbol": "explore_mid_loop_hint_budget",
				"context_role_hint": "absence_support"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleIllustrativeOnly {
		t.Fatalf("context role = %q, want illustrative_only", got[0].ContextRole)
	}
	if got[0].Kind != types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("illustrative exact-target mention should be downgraded to unresolved/ungrounded, got kind=%q grounding=%q", got[0].Kind, got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "illustrative mention of the exact") {
		t.Fatalf("grounding note should explain the downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesNegativeExactProbeDefiningHintToAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "RuntimeSettings",
				"predicate": "does not bind",
				"object": "explore_mid_loop_hint_budget",
				"source": "internal/config/runtime.go",
				"line_start": 231,
				"summary": "RuntimeSettings does not bind the missing exact key",
				"anchor_kind": "definition",
				"anchor_symbol": "RuntimeSettings",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative exact-probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesSubjectOnlyTargetDefiningHintToAbsenceSupport(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "explore_mid_loop_hint_budget",
				"predicate": "is absent from YAML struct",
				"object": "explore_mid_loop_hint_budget",
				"source": "internal/config/runtime.go",
				"line_start": 334,
				"summary": "RuntimeSettings does not expose the missing exact key",
				"anchor_kind": "definition",
				"anchor_symbol": "RuntimeSettings",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleAbsenceSupport {
		t.Fatalf("context role = %q, want absence_support", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "absence support") && !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain subject-only exact-target downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesSameFamilyNegativeProbeDefiningHintToRelatedContext(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "ExploreHeuristics",
				"predicate": "does not define",
				"object": "mid_loop_hint_budget",
				"source": "internal/types/config.go",
				"line_start": 627,
				"summary": "ExploreHeuristics does not define a mid_loop_hint_budget field",
				"anchor_kind": "definition",
				"anchor_symbol": "ExploreHeuristics",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "negative probe") {
		t.Fatalf("grounding note should explain negative same-family probe downgrade, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_DowngradesCrossFamilyDefiningHintDuringExactAbsence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token: "explore_mid_loop_hint_budget",
		Kind:  "symbol",
	})
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "different nearby config family") {
		t.Fatalf("evidence summary should stay semantic; cross-family guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
}

func TestEmitEvidence_KeepsExactTargetPendingWithoutAnalyzerUnverifiedWhenNoDefiningProofExists(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 鐨勬渶缁堟湁鏁堝€兼槸鎬庝箞璁＄畻鍑烘潵鐨勶紵",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "config_mapping",
				ExactTargets: []string{"explore_mid_loop_hint_budget"},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		},
		AnswerContract: types.AnswerContract{
			ExactResolution: &types.ExactResolutionContract{
				TargetKind:           types.SubjectConfigKey,
				TargetLabel:          "config key",
				Targets:              []string{"explore_mid_loop_hint_budget"},
				AllowAbsence:         true,
				RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				RelatedContextTerms:  []string{"explore"},
			},
		},
	}
	params := json.RawMessage(`{
		"items": [
			{
				"kind": "direct",
				"subject": "AgentLoopMaxMidLoopInjects",
				"predicate": "defines",
				"source": "internal/config/runtime.go",
				"line_start": 246,
				"summary": "yaml key agent_loop_max_midloop_injects",
				"anchor_kind": "definition",
				"anchor_symbol": "AgentLoopMaxMidLoopInjects",
				"context_role_hint": "defining",
				"diagram_role_hint": "config"
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("context role = %q, want related_context", got[0].ContextRole)
	}
	if got[0].DiagramRole != types.EvidenceDiagramRoleUnknown {
		t.Fatalf("diagram role = %q, want cleared while the exact target remains unresolved", got[0].DiagramRole)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "different nearby config family") {
		t.Fatalf("grounding note should call out cross-family context, got: %q", got[0].GroundingNote)
	}
}

func TestEmitEvidence_RejectsUnknownTopLevelField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"evidence":[{"kind":"direct","source":"x.go"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown top-level field")
	}
	if !strings.Contains(res.Summary, "invalid params") {
		t.Errorf("got: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsUnknownItemField(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","note":"hi"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on unknown item field")
	}
	if !strings.Contains(res.Summary, "unknown field") {
		t.Errorf("error should mention unknown field: %s", res.Summary)
	}
}

func TestEmitEvidence_RejectsMissingSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","subject":"Foo"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on missing source")
	}
}

func TestEmitEvidence_RejectsNonPathSource(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"FooBar"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: 'FooBar' has no slash or dot, not path-shaped")
	}
}

func TestEmitEvidence_RejectsStringLineStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":"twelve"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on string line_start")
	}
}

func TestEmitEvidence_RejectsRelationshipWithoutObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"A","source":"x.go","line_start":1}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure: relationship needs object")
	}
}

func TestEmitEvidence_NormalizesCallDirectionToCallerCallee(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind:       "call",
						File:       "internal/tool/repomap/tool.go",
						Line:       149,
						ToEP:       repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
						Confidence: repomap.ConfidenceAST,
						Provenance: repomap.ProvenanceTreeSitter,
						ResolvedBy: "test_fixture",
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"fullScan","predicate":"calls","object":"buildOrLoadGraph","source":"internal/tool/repomap/tool.go","line_start":149,"summary":"buildOrLoadGraph calls fullScan.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Subject != "buildOrLoadGraph" {
		t.Fatalf("subject = %q, want buildOrLoadGraph", got[0].Subject)
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].Predicate != "calls" {
		t.Fatalf("predicate = %q, want calls", got[0].Predicate)
	}
}

func TestEmitEvidence_CallOwnerSymbolKeepsParserQualifiedPackageIdentity(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/analysis/gate/gate.go", 135,
		"return RunWith(ir, th, ModeRead, Options{})",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"internal/analysis/gate/gate.go": {
			RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate",
			Symbols:   []repomap.Symbol{{Name: "Run", Kind: "function", Line: 132, EndLine: 136}},
			Relations: []repomap.Relation{{Kind: "call", Line: 135, ToEP: repomap.RelationEndpoint{Name: "RunWith"}}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"Run","predicate":"calls","object":"RunWith","source":"internal/analysis/gate/gate.go","line_start":135,"summary":"Run calls RunWith","anchor_kind":"call","anchor_symbol":"RunWith","snippet":"return RunWith(ir, th, ModeRead, Options{})"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("call evidence should ground, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Subject != "Run" || got[0].OwnerSymbol != "gate.Run" || got[0].AnchorSymbol != "RunWith" {
		t.Fatalf("call identity = %+v, want short presentation plus parser-qualified owner", got)
	}
}

func TestEmitEvidence_CppVirtualCallKeepsStaticReceiverMethodIdentity(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/logger.cpp", 36, "sink_->write(line);")
	loggerFile := &repomap.FileInfo{
		RelPath: "src/logger.cpp", Language: repomap.LangCpp, Package: "logx",
		Symbols: []repomap.Symbol{
			{Name: "Logger", Kind: "class", File: "src/logger.cpp", Line: 20, EndLine: 52},
			{Name: "log", Kind: "method", Parent: "Logger", File: "src/logger.cpp", Line: 29, EndLine: 40},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: "src/logger.cpp", Line: 36,
			ToEP:       repomap.RelationEndpoint{Name: "write", Receiver: "Sink"},
			Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "cpp_ast_field_call",
		}},
	}
	sinkFile := &repomap.FileInfo{
		RelPath: "include/logx/sink.hpp", Language: repomap.LangCpp, Package: "logx",
		Symbols: []repomap.Symbol{
			{Name: "Sink", Kind: "class", File: "include/logx/sink.hpp", Line: 7, EndLine: 14},
			{Name: "write", Kind: "method", Parent: "Sink", File: "include/logx/sink.hpp", Line: 10, EndLine: 10, Arity: 1},
		},
	}
	sinkFile.Symbols[1].ID = repomap.MakeSymbolID(repomap.LangCpp, "logx", "Sink", "write", 1)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			loggerFile.RelPath: loggerFile,
			sinkFile.RelPath:   sinkFile,
		},
		MethodIndex: map[repomap.MethodKey]*repomap.Symbol{
			{Pkg: "logx", Receiver: "Sink", Name: "write"}: &sinkFile.Symbols[1],
		},
	})
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"Logger::log","predicate":"calls","object":"sink_->write","source":"src/logger.cpp","line_start":36,"summary":"virtual call through the held sink","anchor_kind":"call","anchor_symbol":"write","snippet":"sink_->write(line);"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("C++ virtual call should ground, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Subject != "Logger.log" || got[0].OwnerSymbol != "Logger.log" ||
		got[0].Object != "Sink.write" || got[0].AnchorSymbol != "Sink.write" {
		t.Fatalf("virtual call must preserve parser-owned static receiver identity: %+v", got)
	}
}

func TestEmitEvidence_SparseGroundedCallStampsQualifiedCallerAndPredicate(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "bindings-py/fastlex/tokenizer.py", 21,
		"return _fastlex.tokenize_bytes(list(data), self._merges)",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"bindings-py/fastlex/tokenizer.py": {
			RelPath: "bindings-py/fastlex/tokenizer.py", Language: "python", Package: "fastlex",
			Symbols: []repomap.Symbol{
				{Name: "FastTokenizer", Kind: "class", Line: 9, EndLine: 37},
				{Name: "tokenize", Kind: "method", Parent: "FastTokenizer", Line: 18, EndLine: 22},
			},
			Relations: []repomap.Relation{{
				Kind: "call", Line: 21,
				ToEP: repomap.RelationEndpoint{Name: "tokenize_bytes", Receiver: "_fastlex"},
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"relationship","object":"tokenize_bytes","source":"bindings-py/fastlex/tokenizer.py","line_start":21,"summary":"native call","anchor_kind":"call","anchor_symbol":"tokenize_bytes","snippet":"return _fastlex.tokenize_bytes(list(data), self._merges)"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("sparse exact call should normalize, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d want 1: %+v", len(got), got)
	}
	if got[0].Subject != "FastTokenizer.tokenize" || got[0].Predicate != "calls" ||
		got[0].Object != "_fastlex.tokenize_bytes" || got[0].OwnerSymbol != "FastTokenizer.tokenize" {
		t.Fatalf("sparse call lost qualified typed direction: %+v", got[0])
	}
}

func TestEmitEvidence_LineRangeNameMentionCannotMintCallEdgeAuthority(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/evaluator.go", 42,
		"return s.Name == emitPatch",
		"})",
		"}",
	)
	params := json.RawMessage(`{"items":[{"kind":"mechanism","source":"internal/agent/evaluator.go","line_start":42,"line_end":44,"summary":"the branch filters the patch tool schema","anchor_kind":"call","anchor_symbol":"emitPatch","snippet":"return s.Name == emitPatch"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("source mention should remain citable, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d, want 1: %+v", len(got), got)
	}
	if got[0].AnchorKind != types.AnchorTextReference ||
		types.ClaimFormOf(got[0]) != types.ClaimTextReferenceFact {
		t.Fatalf("name mention minted call authority: %+v", got[0])
	}
	if got[0].Subject != "" || got[0].Predicate != "" || got[0].Object != "" || got[0].OwnerSymbol != "" {
		t.Fatalf("downgraded text reference retained directed relation fields: %+v", got[0])
	}
	if !strings.Contains(got[0].GroundingNote, "lacked one exact-line caller -> callee invocation") {
		t.Fatalf("typed downgrade caveat missing: %+v", got[0])
	}
}

func TestEmitEvidence_SemanticCallEndpointsPublishExactParserRepairTuple(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/orchestrator/orchestrator.go", 8472,
		"appendStageOutputEvidenceToMutable(o.busCtx.Mutable, output.EvidenceItems)",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"internal/orchestrator/orchestrator.go": {
			RelPath: "internal/orchestrator/orchestrator.go", Language: repomap.LangGo,
			Symbols: []repomap.Symbol{{Name: "applyStageOutput", Kind: "method", Line: 8435, EndLine: 8500}},
			Relations: []repomap.Relation{{
				Kind: "call", Line: 8472,
				ToEP: repomap.RelationEndpoint{Name: "appendStageOutputEvidenceToMutable"},
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"appendStageOutputEvidenceToMutable","predicate":"writes","object":"Mutable","source":"internal/orchestrator/orchestrator.go","line_start":8472,"summary":"writes evidence to Mutable","anchor_kind":"call","anchor_symbol":"appendStageOutputEvidenceToMutable"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("semantic call row should return typed repair feedback, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorKind != types.AnchorTextReference ||
		types.ClaimFormOf(got[0]) != types.ClaimTextReferenceFact {
		t.Fatalf("semantic endpoints must not gain call authority: %+v", got)
	}
	for _, want := range []string{
		`exact source/parser call tuple`,
		`subject="applyStageOutput" predicate=calls object="appendStageOutputEvidenceToMutable"`,
		`anchor_symbol="appendStageOutputEvidenceToMutable"`,
		`keep any broader data-flow/write claim separate`,
	} {
		if !strings.Contains(got[0].GroundingNote, want) || !strings.Contains(res.Summary, want) {
			t.Fatalf("exact call repair feedback missing %q: item=%+v summary=%s", want, got[0], res.Summary)
		}
	}
}

func TestEmitEvidence_RustQualifiedCallKeepsExactLineAndTypedEdge(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/main.rs", 20,
		`let files = walker::collect_files(".");`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"src/main.rs": {
			RelPath: "src/main.rs", Language: "rust",
			Symbols: []repomap.Symbol{
				{Name: "run", Kind: "function", Line: 14, EndLine: 25},
			},
			Relations: []repomap.Relation{{
				Kind: "call", Line: 20,
				ToEP: repomap.RelationEndpoint{Name: "collect_files"},
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"run","predicate":"calls","object":"walker::collect_files","source":"src/main.rs","line_start":20,"summary":"run calls walker::collect_files","anchor_kind":"call","anchor_symbol":"collect_files","snippet":"let files = walker::collect_files(\".\");"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("qualified Rust call should ground, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d want 1: %+v", len(got), got)
	}
	if got[0].LineStart != 20 || got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
		t.Fatalf("exact call line was weakened or relocated: %+v", got[0])
	}
	if got[0].Subject != "run" || got[0].Predicate != "calls" || got[0].Object != "walker::collect_files" || got[0].AnchorSymbol != "walker::collect_files" {
		t.Fatalf("qualified typed edge was not preserved: %+v", got[0])
	}
	if strings.Contains(res.Summary, "adjusted to 10") || strings.Contains(res.Summary, "nearest_call") {
		t.Fatalf("exact line must not enter nearest-call recovery: %s", res.Summary)
	}
}

func TestEmitEvidence_RealignsExplicitDirectedCallBeforeLineNormalization(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/walker.rs", 1,
		"use std::fs;",
		"",
		"/// Collect files.",
		"pub fn collect_files(root: &str) -> Vec<String> {",
		"    let mut out = Vec::new();",
		"    walk(root, &mut out);",
		"    out",
		"}",
		"",
		"fn walk(dir: &str, out: &mut Vec<String>) {",
		"    for entry in entries.flatten() {",
		"        let path = entry.path();",
		"        if path.is_dir() {",
		"            // recurse",
		"            let child = path.to_string_lossy();",
		"            if child.is_empty() { continue; }",
		"            let child = child.as_ref();",
		"            // exact recursive call follows",
		"            walk(child, out);",
		"        }",
		"    }",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"src/walker.rs": {
			RelPath: "src/walker.rs", Language: "rust",
			Symbols: []repomap.Symbol{
				{Name: "collect_files", Kind: "function", Line: 4, EndLine: 8},
				{Name: "walk", Kind: "function", Line: 10, EndLine: 22},
			},
			Relations: []repomap.Relation{
				{Kind: "call", Line: 6, ToEP: repomap.RelationEndpoint{Name: "walk"}},
				{Kind: "call", Line: 19, ToEP: repomap.RelationEndpoint{Name: "walk"}},
			},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"collect_files","predicate":"calls","object":"walk","source":"src/walker.rs","line_start":19,"summary":"collect_files calls walk","anchor_kind":"call","anchor_symbol":"walk"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("directed call should realign, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d want 1: %+v", len(got), got)
	}
	if got[0].LineStart != 6 || got[0].Subject != "collect_files" || got[0].Object != "walk" || got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("explicit directed edge was silently replaced instead of realigned: %+v", got[0])
	}
	for _, want := range []string{"realigned from line 19", "callsite line 6", `"collect_files" -> "walk"`} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("same-turn feedback missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "walk calls walk") {
		t.Fatalf("recursive sibling call must not steal the intended wrapper edge: %s", res.Summary)
	}
}

func TestEmitEvidence_PreservesTwoExactSameEndpointCallSites(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "cmd/root.go", 650,
		`f.StringVar(&a, "a", "", "a")`,
		`f.StringVar(&b, "b", "", "b")`,
		`f.StringVar(&c, "c", "", "c")`,
		`f.IntVar(&first, "first", 0, "first")`,
		``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``, ``,
		`f.IntVar(&second, "second", 0, "second")`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"cmd/root.go": {
			RelPath: "cmd/root.go", Language: "go",
			Symbols: []repomap.Symbol{{Name: "init", Kind: "function", Line: 647, EndLine: 722}},
			Relations: []repomap.Relation{
				{Kind: "call", Line: 653, ToEP: repomap.RelationEndpoint{Name: "IntVar"}},
				{Kind: "call", Line: 678, ToEP: repomap.RelationEndpoint{Name: "IntVar"}},
			},
		},
	}})
	params := json.RawMessage(`{"items":[
		{"kind":"relationship","subject":"init","predicate":"calls","object":"IntVar","source":"cmd/root.go","line_start":653,"summary":"first binding","anchor_kind":"call","anchor_symbol":"IntVar"},
		{"kind":"relationship","subject":"init","predicate":"calls","object":"IntVar","source":"cmd/root.go","line_start":678,"summary":"second binding","anchor_kind":"call","anchor_symbol":"IntVar"}
	]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("two exact callsites rejected, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || got[0].LineStart != 653 || got[1].LineStart != 678 {
		t.Fatalf("same endpoint calls at distinct exact lines were collapsed or moved: %+v\n%s", got, res.Summary)
	}
	for _, item := range got {
		if item.GroundingStatus != types.GroundingGrounded || types.ClaimFormOf(item) != types.ClaimCallEdge {
			t.Fatalf("exact repeated callsite lost authority: %+v", item)
		}
	}
}

func TestEmitEvidence_DoesNotRealignUnreadRepeatedCallsiteToVisibleSibling(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Only the first callsite is visible. The graph also knows about a second
	// same-endpoint call, but that is not authority to move an observation from
	// the unread second line onto the visible first sibling.
	seedReadFileHistory(ctx, "cmd/root.go", 653,
		`f.IntVar(&first, "first", 0, "first")`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"cmd/root.go": {
			RelPath: "cmd/root.go", Language: "go",
			Symbols: []repomap.Symbol{{Name: "init", Kind: "function", Line: 647, EndLine: 722}},
			Relations: []repomap.Relation{
				{Kind: "call", Line: 653, ToEP: repomap.RelationEndpoint{Name: "IntVar"}},
				{Kind: "call", Line: 678, ToEP: repomap.RelationEndpoint{Name: "IntVar"}},
			},
		},
	}})
	params := json.RawMessage(`{"items":[
		{"kind":"relationship","subject":"init","predicate":"calls","object":"IntVar","source":"cmd/root.go","line_start":653,"summary":"first binding","anchor_kind":"call","anchor_symbol":"IntVar"},
		{"kind":"relationship","subject":"init","predicate":"calls","object":"IntVar","source":"cmd/root.go","line_start":678,"summary":"second binding","anchor_kind":"call","anchor_symbol":"IntVar"}
	]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("mixed visible/unread callsites rejected, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("unread sibling was collapsed into the visible callsite: %+v\n%s", got, res.Summary)
	}
	if got[0].LineStart != 653 || got[1].LineStart != 678 {
		t.Fatalf("exact submitted source identities were not preserved: %+v\n%s", got, res.Summary)
	}
	if got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
		t.Fatalf("the visible callsite must retain exact line authority: %+v", got)
	}
	if got[1].GroundingTier == types.TierLineText {
		t.Fatalf("the unread callsite must not inherit the sibling's line-text authority: %+v", got)
	}
	if strings.Contains(got[1].GroundingNote, "realigned from line 678") || strings.Contains(res.Summary, "realigned from line 678") {
		t.Fatalf("unread repeated callsite must request a read, not steal a visible sibling:\n%s", res.Summary)
	}
}

func TestQualifiedCallEndpointEqualDoesNotCollapseDistinctQualifiedOwners(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{a: "SinkRegistry::create", b: "SinkRegistry.create", want: true},
		{a: "walker::collect_files", b: "collect_files", want: true},
		{a: "service->run", b: "run", want: true},
		{a: "First::run", b: "Second::run", want: false},
		{a: "Logger.log", b: "Audit.log", want: false},
	} {
		if got := qualifiedCallEndpointEqual(tc.a, tc.b); got != tc.want {
			t.Fatalf("qualifiedCallEndpointEqual(%q, %q)=%v want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestEmitEvidenceNoopDuplicate_DirectedCarrierCorrectionIsAmendment(t *testing.T) {
	existing := types.EvidenceItem{
		ID: "old", Kind: types.EvidenceRelationship, Scope: types.ScopeLine,
		Source: "tokenizer.py", LineStart: 21, LineEnd: 21,
		AnchorKind: types.AnchorCall, AnchorSymbol: "tokenize_bytes",
		Object: "tokenize_bytes", GroundingStatus: types.GroundingGrounded,
	}
	incoming := existing
	incoming.ID = "new"
	incoming.Subject = "FastTokenizer.tokenize"
	incoming.Predicate = "calls"
	if emitEvidenceNoopDuplicate(existing, incoming) {
		t.Fatal("adding the typed caller/predicate must be an amendment, not an exact duplicate")
	}
}

func TestEmitEvidence_CallCallerAnchorCanonicalizesBeforeGrounding(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/analysis/gate/gate.go", 135,
		"return RunWith(ir, th, ModeRead, Options{})",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"internal/analysis/gate/gate.go": {
			RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate",
			Symbols:   []repomap.Symbol{{Name: "Run", Kind: "function", Line: 132, EndLine: 136}},
			Relations: []repomap.Relation{{Kind: "call", Line: 135, ToEP: repomap.RelationEndpoint{Name: "RunWith"}}},
		},
	}})
	// This is the production near-miss: the relationship object names the
	// callee, but anchor_symbol incorrectly repeats the enclosing caller.
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"Run","predicate":"calls","object":"RunWith","source":"internal/analysis/gate/gate.go","line_start":135,"summary":"Run calls RunWith","anchor_kind":"call","anchor_symbol":"Run","snippet":"return RunWith(ir, th, ModeRead, Options{})"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("caller-shaped call anchor should canonicalize, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d, want 1: %+v", len(got), got)
	}
	if got[0].Subject != "Run" || got[0].Object != "RunWith" ||
		got[0].OwnerSymbol != "gate.Run" || got[0].AnchorSymbol != "RunWith" {
		t.Fatalf("canonical call identity=%+v", got[0])
	}
	if got[0].GroundingStatus != types.GroundingGrounded || got[0].GroundingTier != types.TierLineText {
		t.Fatalf("canonical call must be grounded on the already-read line, got status=%q tier=%q note=%q",
			got[0].GroundingStatus, got[0].GroundingTier, got[0].GroundingNote)
	}
	if strings.Contains(res.Summary, "→ recovered") {
		t.Fatalf("pre-ground canonicalization must not leak a recovered verdict: %s", res.Summary)
	}
}

func TestQualifiedEvidenceSymbolNameInFile_CoversExecutablePackageQualifierFamilies(t *testing.T) {
	symbol := &repomap.Symbol{Name: "run", Kind: "function"}
	tests := []struct {
		language, pkg, want string
	}{
		{"go", "gate", "gate.run"},
		{"python", "gate", "gate.run"},
		{"javascript", "gate", "gate.run"},
		{"typescript", "gate", "gate.run"},
		{"java", "gate", "gate.run"},
		{"kotlin", "gate", "gate.run"},
		{"rust", "gate", "gate::run"},
		{"c", "gate", "gate.run"},
		{"cpp", "gate", "gate::run"},
		{"ruby", "gate", "gate.run"},
		{"swift", "gate", "gate.run"},
		{"lua", "gate", "gate.run"},
		{"arkts", "gate", "gate.run"},
		{"cangjie", "clinic", "clinic::run"},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			fi := &repomap.FileInfo{Language: tc.language, Package: tc.pkg}
			if got := qualifiedEvidenceSymbolNameInFile(fi, symbol); got != tc.want {
				t.Fatalf("qualified owner = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEmitEvidence_NormalizeCallDirection_DoesNotFallbackToWrongSameLineCall(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/tool/repomap/tool.go": {
				RelPath: "internal/tool/repomap/tool.go",
				Symbols: []repomap.Symbol{
					{Name: "buildOrLoadGraph", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 121, EndLine: 170},
					{Name: "fullScan", Kind: "function", File: "internal/tool/repomap/tool.go", Line: 172, EndLine: 220},
				},
				Relations: []repomap.Relation{
					{
						Kind:       "call",
						File:       "internal/tool/repomap/tool.go",
						Line:       147,
						ToEP:       repomap.RelationEndpoint{Name: "NeedsFullRescan", Receiver: "index", File: "internal/tool/repomap/tool.go", Line: 147},
						Confidence: repomap.ConfidenceAST,
						Provenance: repomap.ProvenanceTreeSitter,
						ResolvedBy: "test_fixture",
					},
					{
						Kind:       "call",
						File:       "internal/tool/repomap/tool.go",
						Line:       149,
						ToEP:       repomap.RelationEndpoint{Name: "fullScan", File: "internal/tool/repomap/tool.go", Line: 149},
						Confidence: repomap.ConfidenceAST,
						Provenance: repomap.ProvenanceTreeSitter,
						ResolvedBy: "test_fixture",
					},
				},
			},
		},
	}
	ctx.Mutable.SetSearchGraph(graph)
	params := json.RawMessage(`{"items":[{"kind":"relationship","subject":"buildOrLoadGraph","predicate":"calls","object":"fullScan","source":"internal/tool/repomap/tool.go","line_start":147,"summary":"buildOrLoadGraph calls fullScan.","anchor_kind":"call","anchor_symbol":"fullScan","snippet":"if index.NeedsFullRescan(cacheDir) {\n    logging.Info(\"repo_map: full scan (%d files, no cache)\", len(entries))\n    return fullScan(repoRoot, cacheDir, entries, query)"}]}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Object != "fullScan" {
		t.Fatalf("object = %q, want fullScan", got[0].Object)
	}
	if got[0].LineStart != 149 {
		t.Fatalf("line_start = %d, want recovery to 149", got[0].LineStart)
	}
}

// TestEmitEvidence_AutoSwapsLineEndBeforeStart pins the 2026-04-17
// change: line_end < line_start is treated as an obvious
// transposition typo and auto-swapped. The summary includes a
// "double-check the range" warning so the LLM can notice. Earlier
// behaviour rejected the whole batch over this single keystroke
// mistake; trace 1776439797257469553 iter=2 lost four otherwise-
// valid evidence items to a single typo on item[3].
func TestEmitEvidence_AutoSwapsLineEndBeforeStart(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[{
		"kind":"direct","source":"x.go","line_start":50,"line_end":10,
		"anchor_kind":"definition","anchor_symbol":"Foo",
		"subject":"Foo","summary":"test"
	}]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("auto-swap path must succeed, got rejection: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "AUTO-SWAPPED") {
		t.Errorf("summary must warn about auto-swap: %q", res.Summary)
	}
	// Item should be in the accepted buffer with line_start=10, line_end=50.
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 accepted item, got %d", len(emitted))
	}
	if emitted[0].LineStart != 10 || emitted[0].LineEnd != 50 {
		t.Errorf("swapped range wrong: got [%d, %d], want [10, 50]",
			emitted[0].LineStart, emitted[0].LineEnd)
	}
}

// TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch locks in
// the graceful-degrade behaviour: a single invalid item in a mostly-
// healthy batch skips that item and keeps the rest. Fires ONLY when
// the reject ratio is below 50%; above that the batch is rejected
// wholesale.
func TestEmitEvidence_PerItemRejectSkipsInsteadOfFailingBatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 4 valid + 1 invalid (missing anchor_kind). 1/5 = 20% → sparse
	// reject path. Batch succeeds with 4 accepted; 1 skipped in the
	// Summary. (The healthy items all need required fields filled.)
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"B","subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"},
		{"kind":"direct","source":"d.go","line_start":4,"anchor_kind":"definition","anchor_symbol":"D","subject":"D","summary":"x"},
		{"kind":"direct","source":"e.go","line_start":5,"anchor_kind":"definition","anchor_symbol":"E","subject":"E","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("sparse-reject batch must succeed, got rejection: %s", res.Summary)
	}
	if len(ctx.Mutable.EmittedEvidence()) != 4 {
		t.Errorf("expected 4 accepted, got %d", len(ctx.Mutable.EmittedEvidence()))
	}
	if !strings.Contains(res.Summary, "SKIPPED") {
		t.Errorf("summary must mention skipped item: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "items[2]") {
		t.Errorf("summary must identify the bad item index: %q", res.Summary)
	}
}

// TestEmitEvidence_PartialBatchSurvives pins the commit-61 Batch
// F.1 contract (red line "no system hard-cap"): pre-fix, ≥ 50%
// rejected items hard-failed the WHOLE call — even the valid 1
// item was discarded. Now the 1 valid item survives + per-item
// reject reasons surface in Summary so the LLM can re-emit just
// the failed ones. Only zero-survivor batches still fail.
func TestEmitEvidence_PartialBatchSurvives(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// 1 valid + 2 invalid (missing anchor_kind on two items).
	// Pre-commit-61: hard-failed 2/3. Post-fix: 1 valid item kept,
	// 2 rejections surfaced in Summary.
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"A","subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"},
		{"kind":"direct","source":"c.go","line_start":3,"subject":"C","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if !res.Success {
		t.Fatalf("partial batch should succeed (1 valid item kept); got failure: %s", res.Summary)
	}
	// Summary must still surface per-item reject reasons so the LLM
	// knows to re-emit the failed items.
	if !strings.Contains(strings.ToLower(res.Summary), "skipped") &&
		!strings.Contains(strings.ToLower(res.Summary), "reject") &&
		!strings.Contains(res.Summary, "items[id=") {
		t.Errorf("partial batch Summary must surface per-item reject reasons: %q", res.Summary)
	}
}

// TestEmitEvidence_AllInvalidStillFails pins the empty-survival
// gate: when ZERO items survived per-item validation, the call
// can't proceed (no closure entry to write).
func TestEmitEvidence_AllInvalidStillFails(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[
		{"kind":"direct","source":"a.go","line_start":1,"subject":"A","summary":"x"},
		{"kind":"direct","source":"b.go","line_start":2,"subject":"B","summary":"x"}
	]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatalf("zero-survivor batch must fail; got success: %s", res.Summary)
	}
}

func TestEmitEvidence_ConfigAbsentPrimaryMarksSubstituteEvidenceContextOnly(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: missingKey + " 在哪里定义？",
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{missingKey},
			Entities:        []string{missingKey},
			ExactTargets:    []string{missingKey},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	}}
	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token:  missingKey,
		Kind:   "symbol",
		Reason: "exact target has no current production-defining prescan hit",
	})
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "AgentLoopMaxMidLoopInjects", "source": "internal/config/runtime.go", "line_start": 184, "summary": "related YAML key controls midloop injection budget", "anchor_kind": "definition", "anchor_symbol": "AgentLoopMaxMidLoopInjects"}
        ]
    }`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].ContextRole != types.EvidenceContextRoleRelatedContext {
		t.Fatalf("related substitute evidence context role=%q, want related_context", got[0].ContextRole)
	}
	if strings.Contains(strings.ToLower(got[0].Summary), "context only") {
		t.Fatalf("evidence summary should stay semantic; context-only guidance belongs in grounding_note, got: %q", got[0].Summary)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "do not repair this item") {
		t.Fatalf("grounding note should suppress substitute repair loops: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not spend read_file budget") {
		t.Fatalf("tool summary should still steer away from repairing substitute exact hits, got: %q", res.Summary)
	}
}

func TestEmitEvidence_AnnotatedStageReportDoesNotOverrideExactDefiningProof(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: missingKey + " 在哪里定义？",
		Scenario:   types.ScenarioConfigTrace,
		AnalyzerHints: types.AnalyzerHints{
			Kind:            "config_mapping",
			PrimaryEntities: []string{missingKey},
			Entities:        []string{missingKey},
			ExactTargets:    []string{missingKey},
		},
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
	}}
	ctx.StageReports = []types.StageReport{{
		Stage:    types.StageAnalyze,
		Agent:    types.AgentAnalyzer,
		Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
	}}
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "codrax.yaml", LineStart: 12,
		Subject: missingKey, AnchorKind: types.AnchorAssignment, AnchorSymbol: missingKey,
		Snippet:         missingKey + ": 2",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})

	contract := types.BuildExactResolutionContract(ctx.AnalysisIR.RequestModel)
	if contract == nil {
		t.Fatal("test setup must produce exact-resolution contract")
	}
	if pending := pendingExactResolutionTargets(ctx, contract); len(pending) != 0 {
		t.Fatalf("StageReport prose must not override exact defining proof, pending=%v", pending)
	}

	ctx.Mutable.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token:  missingKey,
		Kind:   "symbol",
		Reason: "typed verifier still marks target unresolved",
	})
	if pending := pendingExactResolutionTargets(ctx, contract); len(pending) != 1 || pending[0] != missingKey {
		t.Fatalf("typed closure unverified finding should still drive exact target blocker, pending=%v", pending)
	}
}

func TestEmitEvidence_DemotesCrossCallableLineLocalOwnerMismatch(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 883,
		"func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {",
		"if ctx == nil || ctx.Mutable == nil {",
		`	return nil, errors.New("analyzer: missing AgentContext.Mutable")`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{
						Name:     "ParseOutput",
						Receiver: "(*analyzerEvaluator)",
						Kind:     "method",
						Line:     629,
						EndLine:  879,
					},
					{
						Name:    "buildAnalysisIR",
						Kind:    "function",
						Line:    883,
						EndLine: 1100,
					},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "conditional",
				"subject": "ParseOutput",
				"predicate": "checks",
				"source": "internal/agent/analyzer.go",
				"line_start": 884,
				"condition": "ctx == nil || ctx.Mutable == nil",
				"summary": "ParseOutput checks ctx before the panic path continues.",
				"anchor_kind": "condition",
				"anchor_symbol": "buildAnalysisIR"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with owner-mismatch demotion, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].OwnerSymbol != "buildAnalysisIR" {
		t.Fatalf("owner symbol = %q, want buildAnalysisIR", got[0].OwnerSymbol)
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "line-local statement") {
		t.Fatalf("grounding note should explain the owner mismatch, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "drop the item") {
		t.Fatalf("summary should steer away from repairing the cross-owner claim, got: %q", res.Summary)
	}
}

func TestEmitEvidence_DemotesConditionClaimThatDoesNotMatchGroundedLine(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 884,
		"if ctx == nil || ctx.Mutable == nil {",
		`	return nil, errors.New("analyzer: missing AgentContext.Mutable")`,
		"}",
		"raw := ctx.Mutable.RequestModel()",
		"if raw == nil {",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{
						Name:    "buildAnalysisIR",
						Kind:    "function",
						Line:    883,
						EndLine: 930,
					},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "conditional",
				"source": "internal/agent/analyzer.go",
				"line_start": 887,
				"condition": "ctx != nil && ctx.Mutable == nil",
				"summary": "guard only checks ctx != nil and misses nil Mutable",
				"anchor_kind": "condition",
				"anchor_symbol": "RequestModel"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success with unsupported-condition demotion, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].Kind != types.EvidenceUnresolved {
		t.Fatalf("kind = %q, want unresolved", got[0].Kind)
	}
	if got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grounding status = %q, want ungrounded", got[0].GroundingStatus)
	}
	if !strings.Contains(strings.ToLower(got[0].GroundingNote), "condition anchor is not supported") {
		t.Fatalf("grounding note should explain unsupported condition claim, got: %q", got[0].GroundingNote)
	}
	if !strings.Contains(strings.ToLower(res.Summary), "do not repair") {
		t.Fatalf("summary should steer away from repairing the speculative condition claim, got: %q", res.Summary)
	}
}

func TestEmitEvidence_NormalizesEquivalentBooleanConditionAndVisibleAnchor(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "cmd/root.go", 2664,
		`if !cmd.Flags().Changed("pipeline-max-steps") {`,
		"\tflagMaxSteps = mergedMaxSteps",
		"}",
	)
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "conditional",
				"source": "cmd/root.go",
				"line_start": 2664,
				"condition": "cmd.Flags().Changed(\"pipeline-max-steps\") == false",
				"summary": "the merged value is used when the flag was not explicitly changed",
				"anchor_kind": "condition",
				"anchor_symbol": "flagMaxSteps"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected equivalent condition to ground, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer, got %d", len(got))
	}
	if got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status = %q, want grounded; note=%q", got[0].GroundingStatus, got[0].GroundingNote)
	}
	if got[0].AnchorSymbol != "Changed" {
		t.Fatalf("anchor_symbol = %q, want visible condition token Changed", got[0].AnchorSymbol)
	}
	if !strings.Contains(got[0].GroundingNote, "normalized it to visible typed-condition token") {
		t.Fatalf("normalization audit note missing: %q", got[0].GroundingNote)
	}
}

func TestConditionClaimsStructurallyEquivalentBooleanForms(t *testing.T) {
	cases := []struct {
		name       string
		left       string
		right      string
		equivalent bool
	}{
		{"go negation", `cmd.Flags().Changed("pipeline-max-steps") == false`, `if !cmd.Flags().Changed("pipeline-max-steps") {`, true},
		{"python not", "ready == false", "if not ready:", true},
		{"ruby unless", "ready != true", "unless ready then", true},
		{"positive literal", "ready == true", "if ready {", true},
		{"opposite polarity", "ready == true", "if !ready {", false},
		{"quoted false is data", `value == "false"`, "if !value {", false},
		{"compound prefix negation is not whole-expression negation", "(a && b) == false", "if !a && b {", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionClaimsStructurallyEquivalent(tc.left, tc.right); got != tc.equivalent {
				t.Fatalf("equivalent(%q, %q) = %v, want %v", tc.left, tc.right, got, tc.equivalent)
			}
		})
	}
}

func TestEmitEvidence_ConditionSnippetCorroboratesAbbreviatedClaimAcrossLanguages(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		line   string
		cond   string
		anchor string
	}{
		{
			name:   "go selector chain with elided threshold",
			file:   "internal/analysis/criterion/eval.go",
			line:   "if env.IR.AnswerContract.CitationReq.Required && env.DraftCitations < env.IR.AnswerContract.CitationReq.MinCitations {",
			cond:   "if env.IR.AnswerContract.CitationReq.Required && ...",
			anchor: "Required",
		},
		{
			name:   "arkts property chain with elided rhs",
			file:   "entry/src/main/ets/pages/Index.ets",
			line:   "if (this.config.citation.required && this.state.ready) {",
			cond:   "if this.config.citation.required && ...",
			anchor: "required",
		},
		{
			name:   "cpp member chain with elided rhs",
			file:   "src/citation.cpp",
			line:   "if (ctx.config.citation.required && budget.remaining() > 0) {",
			cond:   "if ctx.config.citation.required && ...",
			anchor: "required",
		},
		{
			name:   "cangjie member chain with elided rhs",
			file:   "src/citation.cj",
			line:   "if (ctx.config.citation.required && ctx.ready) {",
			cond:   "if ctx.config.citation.required && ...",
			anchor: "required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &EmitEvidence{}
			ctx := newEmitCtx()
			seedReadFileHistory(ctx, tc.file, 42, tc.line)
			body, err := json.Marshal(map[string]any{
				"items": []map[string]any{{
					"scope":         "line",
					"evidence_kind": "direct",
					"subject":       "owner",
					"source":        tc.file,
					"line_start":    42,
					"condition":     tc.cond,
					"summary":       "guard reads the target member before deciding the path",
					"anchor_kind":   "condition",
					"anchor_symbol": tc.anchor,
					"snippet":       tc.line,
				}},
			})
			if err != nil {
				t.Fatalf("marshal params: %v", err)
			}

			res, err := tool.Execute(ctx, json.RawMessage(body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Success {
				t.Fatalf("expected success, got: %s", res.Summary)
			}
			got := ctx.Mutable.EmittedEvidence()
			if len(got) != 1 {
				t.Fatalf("want 1 item, got %d", len(got))
			}
			if got[0].Kind == types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingGrounded {
				t.Fatalf("condition with exact snippet should remain grounded, kind=%s status=%s note=%q",
					got[0].Kind, got[0].GroundingStatus, got[0].GroundingNote)
			}
		})
	}
}

func TestEmitEvidence_RejectsEmptyItems(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{"items":[]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected failure on empty items")
	}
}

func TestEmitEvidence_RejectsMissingMutable(t *testing.T) {
	tool := &EmitEvidence{}
	res, _ := tool.Execute(&types.BusContext{}, json.RawMessage(`{"items":[]}`))
	if res.Success {
		t.Fatal("expected failure when Mutable is nil")
	}
}

func TestEmitEvidence_StableIDDedups(t *testing.T) {
	// Same content emitted twice produces identical IDs. The tool now
	// drops the second call before it can inflate the mutable evidence
	// buffer; downstream mergers still see the same stable ID.
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	body := `{"items":[{"kind":"direct","subject":"Foo","source":"a/b.go","line_start":7,"summary":"x","anchor_kind":"definition","anchor_symbol":"Foo"}]}`
	first, _ := tool.Execute(ctx, json.RawMessage(body))
	second, _ := tool.Execute(ctx, json.RawMessage(body))
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item in buffer after duplicate no-op, got %d", len(got))
	}
	if got[0].ID == "" {
		t.Fatal("stable ID should be assigned")
	}
	if first.Repair != nil && first.Repair.Code == EmitEvidenceDuplicateNoopCode {
		t.Fatalf("first emit must not be marked duplicate: %+v", first.Repair)
	}
	if second.Repair == nil || second.Repair.Code != EmitEvidenceDuplicateNoopCode {
		t.Fatalf("second emit should be duplicate no-op, got %+v", second.Repair)
	}
}

func TestEmitEvidence_ResetEmittedEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	_, _ = tool.Execute(ctx, json.RawMessage(`{"items":[{"kind":"direct","source":"x.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"X"}]}`))
	if len(ctx.Mutable.EmittedEvidence()) != 1 {
		t.Fatalf("expected 1 item before reset")
	}
	ctx.Mutable.ResetEmittedEvidence()
	if len(ctx.Mutable.EmittedEvidence()) != 0 {
		t.Errorf("expected empty buffer after reset")
	}
}

func TestEmitEvidence_ExplicitSupersessionReplacesWrongModelFact(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	wrong := types.EvidenceItem{
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Subject:         "SummaryExtractor",
		Predicate:       "direct",
		Source:          "internal/demo.go",
		LineStart:       1,
		LineEnd:         1,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "SummaryExtractor",
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierSnippetFuzzy,
		Producer:        types.EvidenceProducerExplorerEmitEvidence,
	}
	wrong.ID = types.StableEvidenceID(wrong)
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{wrong})
	acceptedBefore := ctx.Mutable.EmittedEvidence()
	if len(acceptedBefore) != 1 {
		t.Fatalf("seed evidence=%+v", acceptedBefore)
	}
	wrongID := acceptedBefore[0].ID
	seedReadFileHistory(ctx, "internal/demo.go", 1,
		"type SummaryExtractor struct{}",
		"func Extractor() {}",
	)

	params := json.RawMessage(fmt.Sprintf(`{
        "items": [
          {"scope":"line","evidence_kind":"direct","subject":"Extractor","source":"internal/demo.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"Extractor","summary":"actual extractor stage"}
        ],
        "supersessions": [
          {"evidence_id":%q,"replacement_item_index":0}
        ]
    }`, wrongID))
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("supersession failed: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].AnchorSymbol != "Extractor" || got[0].ID == wrongID {
		t.Fatalf("wrong fact must be removed and grounded replacement retained: %+v", got)
	}
	if !strings.Contains(res.Summary, "Explicit evidence supersession committed") ||
		!strings.Contains(res.Summary, wrongID) || !strings.Contains(res.Summary, "id="+got[0].ID) {
		t.Fatalf("summary must expose stable IDs and committed replacement:\n%s", res.Summary)
	}
	refs := ctx.Mutable.EvidenceClosure().AcceptedEvidenceRefs()
	if len(refs) != 1 || refs[0].ID != got[0].ID {
		t.Fatalf("closure must drop stale accepted ref and retain replacement: %+v", refs)
	}
}

func TestEmitEvidence_ExplicitSupersessionCannotRemoveSystemEvidence(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	systemFact := types.EvidenceItem{
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Subject:         "A",
		Predicate:       "calls",
		Object:          "B",
		Source:          "internal/demo.go",
		LineStart:       1,
		LineEnd:         1,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "B",
		GroundingStatus: types.GroundingGrounded,
		Producer:        types.EvidenceProducerRepoMapSelectedCallableBodyCall,
	}
	systemFact.ID = types.StableEvidenceID(systemFact)
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{systemFact})
	seedReadFileHistory(ctx, "internal/demo.go", 1, "func Replacement() {}")

	params := json.RawMessage(fmt.Sprintf(`{
        "items": [
          {"scope":"line","evidence_kind":"direct","subject":"Replacement","source":"internal/demo.go","line_start":1,"anchor_kind":"definition","anchor_symbol":"Replacement"}
        ],
        "supersessions": [
          {"evidence_id":%q,"replacement_item_index":0}
        ]
    }`, ctx.Mutable.EmittedEvidence()[0].ID))
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "only model-emitted evidence can be superseded") {
		t.Fatalf("system evidence removal must fail closed: %+v", res)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Producer != types.EvidenceProducerRepoMapSelectedCallableBodyCall {
		t.Fatalf("failed supersession must not append replacement or remove target: %+v", got)
	}
}

func TestEmitEvidence_ExplicitSupersessionRequiresExactlyGroundedReplacement(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	wrong := types.EvidenceItem{
		Kind: types.EvidenceDirect, Scope: types.ScopeLine, Subject: "Wrong", Predicate: "direct",
		Source: "internal/demo.go", LineStart: 1, LineEnd: 1,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Wrong",
		GroundingStatus: types.GroundingRecovered, Producer: types.EvidenceProducerExplorerEmitEvidence,
	}
	wrong.ID = types.StableEvidenceID(wrong)
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{wrong})
	wrongID := ctx.Mutable.EmittedEvidence()[0].ID
	seedReadFileHistory(ctx, "internal/demo.go", 1, "func Wrong() {}")

	params := json.RawMessage(fmt.Sprintf(`{
        "items": [
          {"scope":"line","evidence_kind":"direct","subject":"NeverRead","source":"internal/missing.go","line_start":99,"anchor_kind":"definition","anchor_symbol":"NeverRead"}
        ],
        "supersessions": [
          {"evidence_id":%q,"replacement_item_index":0}
        ]
    }`, wrongID))
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success || !strings.Contains(res.Summary, "requires exact grounded evidence") {
		t.Fatalf("ungrounded replacement must fail closed: %+v", res)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].ID != wrongID {
		t.Fatalf("failed replacement must preserve original buffer: %+v", got)
	}
}

func TestResolveEmitEvidenceSupersessions_AmbiguousStableIDFailsClosed(t *testing.T) {
	base := types.EvidenceItem{
		ID: "ev-shared", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		Subject: "Thing", Predicate: "direct", Source: "x.go", LineStart: 1, LineEnd: 1,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Thing",
		GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence,
	}
	sibling := base
	sibling.AnchorSymbol = "OtherThing"
	replacement := base
	replacement.ID = "ev-new"
	replacement.Source = "y.go"
	_, _, err := resolveEmitEvidenceSupersessions(
		[]emitEvidenceSupersession{{EvidenceID: "ev-shared", ReplacementItemIndex: 0}},
		[]types.EvidenceItem{base, sibling},
		[]types.EvidenceItem{replacement},
		[]int{0},
	)
	if err == nil || !strings.Contains(err.Error(), "ambiguous IDs cannot be removed") {
		t.Fatalf("ambiguous ID must fail closed, got %v", err)
	}
}

// TestEmitEvidence_AcceptsExplicitLineScope pins the 2026-05+ scope
// migration: when the LLM explicitly sets scope=line on a line-shaped
// emit, the tool accepts it without redirect. The transitional
// default-to-ScopeLine fallback covers tests that omit the field;
// this test confirms the explicit path works the same.
func TestEmitEvidence_AcceptsExplicitLineScope(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "line", "kind": "direct", "subject": "isOK", "source": "internal/agent/foo.go", "line_start": 30, "summary": "isOK returns true", "anchor_kind": "definition", "anchor_symbol": "isOK"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 {
		t.Fatalf("expected 1 emitted item, got %d", len(emitted))
	}
	if emitted[0].Scope != types.ScopeLine {
		t.Errorf("Scope = %q, want %q", emitted[0].Scope, types.ScopeLine)
	}
}

// TestEmitEvidence_RejectsInvalidScopeValue confirms the schema
// validator rejects unknown scope values with a useful error.
func TestEmitEvidence_RejectsInvalidScopeValue(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "wrong_scope", "kind": "direct", "source": "a.go", "line_start": 1, "anchor_kind": "definition", "anchor_symbol": "X"}
        ]
    }`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected rejection on invalid scope value")
	}
	if !strings.Contains(res.Summary, "scope") {
		t.Errorf("rejection should mention scope; got: %s", res.Summary)
	}
}

// TestEmitEvidence_AcceptsScopeFile pins File-scope emission. No
// line_start required; FileRoleLabel mandatory.
func TestEmitEvidence_AcceptsScopeFile(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "file", "kind": "direct", "source": "codrax.yaml", "file_role_label": "config_canonical", "summary": "codrax.yaml is the canonical config layer"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeFile {
		t.Fatalf("expected 1 ScopeFile item; got %+v", emitted)
	}
	if emitted[0].FileRoleLabel != types.FileRoleConfigCanonical {
		t.Errorf("FileRoleLabel = %q, want %q", emitted[0].FileRoleLabel, types.FileRoleConfigCanonical)
	}
	if emitted[0].LineStart != 0 {
		t.Errorf("ScopeFile must keep LineStart=0; got %d", emitted[0].LineStart)
	}
}

// TestEmitEvidence_AcceptsScopeNegative pins Negative-scope emission.
// Requires kind=absent + NegativeQuery + NegativeScope.
func TestEmitEvidence_AcceptsScopeNegative(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "negative", "kind": "absent", "source": "codrax.yaml", "negative_query": {"file": "codrax.yaml", "pattern": "explore_mid_loop_hint_budget"}, "negative_scope": "file", "summary": "key not in codrax.yaml"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeNegative {
		t.Fatalf("expected 1 ScopeNegative item; got %+v", emitted)
	}
	if emitted[0].Kind != types.EvidenceAbsent {
		t.Errorf("Kind = %q, want %q", emitted[0].Kind, types.EvidenceAbsent)
	}
	if emitted[0].NegativeQuery == nil || emitted[0].NegativeQuery.Pattern != "explore_mid_loop_hint_budget" {
		t.Errorf("NegativeQuery missing or wrong pattern: %+v", emitted[0].NegativeQuery)
	}
}

func TestEmitEvidence_AbsentLineEvidencePublishesTypedCompletionRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "line", "kind": "absent", "source": "codrax.yaml", "line_start": 12, "anchor_kind": "definition", "anchor_symbol": "missing_key", "summary": "missing_key is absent"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected absent line evidence to reject, got success: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != types.ToolRepairCodeEvidenceAbsenceToCompletion {
		t.Fatalf("expected typed absence completion repair, got %+v summary=%s", res.Repair, res.Summary)
	}
	hasAbsenceField := false
	for _, field := range res.Repair.Fields {
		if field == "emit_investigation_complete.absence_justification" {
			hasAbsenceField = true
			break
		}
	}
	if !hasAbsenceField {
		t.Fatalf("repair should name completion absence field, got %+v", res.Repair.Fields)
	}
}

// TestEmitEvidence_AcceptsScopeCrossfile pins Crossfile-scope.
// Requires CrossfileQuery (files+pattern) and CrossfileAssertion.
func TestEmitEvidence_AcceptsScopeCrossfile(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "crossfile", "kind": "direct",
           "crossfile_query": {"files": ["cmd/root.go"], "pattern": "flag\\..*Explore"},
           "crossfile_assertion": {"kind": "forbidden"},
           "summary": "no CLI flag for explore_*"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeCrossfile {
		t.Fatalf("expected 1 ScopeCrossfile item; got %+v", emitted)
	}
	if emitted[0].CrossfileQuery == nil || emitted[0].CrossfileAssertion == nil {
		t.Errorf("CrossfileQuery / Assertion missing on emitted item")
	}
}

// TestEmitEvidence_AcceptsScopeSection pins Section-scope. Requires
// SectionPath; LineStart/End optional.
func TestEmitEvidence_AcceptsScopeSection(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "section", "kind": "direct", "source": "codrax.yaml", "section_path": "explore_*", "summary": "explore_* group"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeSection {
		t.Fatalf("expected 1 ScopeSection item; got %+v", emitted)
	}
	if emitted[0].SectionPath != "explore_*" {
		t.Errorf("SectionPath = %q, want %q", emitted[0].SectionPath, "explore_*")
	}
}

// TestEmitEvidence_AcceptsScopeLineRange pins LineRange. Requires
// line_end > line_start.
func TestEmitEvidence_AcceptsScopeLineRange(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	params := json.RawMessage(`{
        "items": [
          {"scope": "line_range", "kind": "direct", "source": "a.go", "line_start": 10, "line_end": 50, "summary": "struct definition block"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	emitted := ctx.Mutable.EmittedEvidence()
	if len(emitted) != 1 || emitted[0].Scope != types.ScopeLineRange {
		t.Fatalf("expected 1 ScopeLineRange item; got %+v", emitted)
	}
	if emitted[0].LineEnd != 50 {
		t.Errorf("LineEnd = %d, want 50", emitted[0].LineEnd)
	}
}

// TestEmitEvidence_RejectsScopeMissingRequiredField confirms per-scope
// required fields are enforced.
func TestEmitEvidence_RejectsScopeMissingRequiredField(t *testing.T) {
	tool := &EmitEvidence{}
	cases := []struct {
		name string
		json string
		hint string
	}{
		{"file scope missing role label", `{"items": [{"scope": "file", "kind": "direct", "source": "a.yaml"}]}`, "file_role_label"},
		{"section scope missing path", `{"items": [{"scope": "section", "kind": "direct", "source": "a.go"}]}`, "section_path"},
		{"crossfile missing query", `{"items": [{"scope": "crossfile", "kind": "direct"}]}`, "crossfile_query"},
		{"negative missing query", `{"items": [{"scope": "negative", "kind": "absent", "source": "a.yaml", "negative_scope": "file"}]}`, "negative_query"},
		// Degenerate line_range values are compatibility-repaired to
		// scope=line elsewhere; a missing starting coordinate is still
		// structurally ungroundable and must reject.
		{"line_range needs line_start", `{"items": [{"scope": "line_range", "kind": "direct", "source": "a.go", "line_end": 10, "anchor_kind": "definition", "anchor_symbol": "X"}]}`, "line_start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := newEmitCtx()
			res, _ := tool.Execute(ctx, json.RawMessage(c.json))
			if res.Success {
				t.Errorf("expected rejection for case %q; res.Summary=%q", c.name, res.Summary)
				return
			}
			if !strings.Contains(strings.ToLower(res.Summary), c.hint) {
				t.Errorf("rejection should mention %q; got: %s", c.hint, res.Summary)
			}
		})
	}
}

// TestEmitEvidence_AutoPairsRoleDescriptionFromLeadingDocComment is the
// Plan 2 v2 (2026-05-05) integration test for the deterministic
// role-description pairing hook. Setup: a Go source whose definition
// at line 5 has a 2-line `//` doc comment immediately above. The LLM
// emits one definition-anchor evidence item; the system MUST auto-pair
// a parallel evidence_kind=mechanism item carrying the doc comment
// text as Summary.
func TestEmitEvidence_AutoPairsRoleDescriptionFromLeadingDocComment(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// Seed read_file gutter with a Go file whose def line is 5 and
	// the two lines above are //-comments.
	seedReadFileHistory(ctx, "internal/agent/foo.go", 1,
		"package foo",
		"",
		"// Foo coordinates the storm pipeline by deciding when each",
		"// stage emits and persisting the rolling baseline.",
		"func Foo() {",
		"    return",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 5, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("want 2 items (1 manual + 1 auto-paired), got %d", len(got))
	}
	var paired *types.EvidenceItem
	for i := range got {
		if got[i].Producer == "auto_pair_role_description" {
			paired = &got[i]
			break
		}
	}
	if paired == nil {
		t.Fatalf("auto-paired mechanism item missing; producers seen: %s", producersOf(got))
	}
	if paired.Kind != types.EvidenceMechanism {
		t.Errorf("auto-paired item kind = %q, want mechanism", paired.Kind)
	}
	if !strings.Contains(paired.Summary, "Foo coordinates the storm pipeline") {
		t.Errorf("auto-paired Summary missing doc-comment text: %q", paired.Summary)
	}
	if paired.LineStart != 3 {
		t.Errorf("auto-paired LineStart = %d, want 3 (first comment line)", paired.LineStart)
	}
	if len(paired.DerivedFrom) != 1 || paired.DerivedFrom[0] != got[0].ID {
		t.Errorf("auto-paired DerivedFrom = %v, want [%q]", paired.DerivedFrom, got[0].ID)
	}
	stats := ctx.Mutable.EvidenceClosure().Stats()
	if stats.AutoPairedRoleDescriptions != 1 {
		t.Errorf("AutoPairedRoleDescriptions counter = %d, want 1", stats.AutoPairedRoleDescriptions)
	}
}

func TestEmitEvidence_AutoPairsRoleDescriptionAcrossDecoratorStack(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", 1,
		"// Source: developer.huawei.com / openharmony Index.ets minimal sample",
		"// Surface: @Entry + @Component + build()",
		"",
		"@Entry",
		"@Component",
		"struct Index {",
		"  build() {}",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Index", "source": "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", "line_start": 6, "summary": "Index is defined here", "anchor_kind": "definition", "anchor_symbol": "Index"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	var paired *types.EvidenceItem
	for i := range got {
		if got[i].Producer == "auto_pair_role_description" {
			paired = &got[i]
			break
		}
	}
	if paired == nil {
		t.Fatalf("auto-paired mechanism item missing; producers seen: %s", producersOf(got))
	}
	if !strings.Contains(paired.Summary, "Index.ets minimal sample") {
		t.Fatalf("auto-paired Summary missing decorated-file header: %q", paired.Summary)
	}
	if paired.LineStart != 1 {
		t.Fatalf("auto-paired LineStart = %d, want 1", paired.LineStart)
	}
}

// TestEmitEvidence_DoesNotAutoPairWhenManualMechanismExists guards the
// dedup rule: if the LLM already supplied a mechanism evidence at the
// same (source, line_start), the auto-pair MUST stand down so the
// authored explanation remains canonical.
func TestEmitEvidence_DoesNotAutoPairWhenManualMechanismExists(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/foo.go", 1,
		"package foo",
		"",
		"// Foo runs the storm pipeline and persists the baseline.",
		"func Foo() {",
		"    return",
		"}",
	)
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 4, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"},
          {"kind": "mechanism", "subject": "Foo", "predicate": "documents", "summary": "Foo handles the storm pipeline (LLM authored).", "source": "internal/agent/foo.go", "line_start": 4, "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	for _, it := range got {
		if it.Producer == "auto_pair_role_description" {
			t.Errorf("auto-pair should stand down when manual mechanism present at same anchor; got auto-paired item %q", it.Summary)
		}
	}
}

// TestEmitEvidence_AutoPairSkipsWhenSourceUnread exercises the safety
// rail: when the source's lines around lineStart are NOT in the
// read_file gutter, the extractor must decline rather than risk a
// false-positive comment from an unread region.
func TestEmitEvidence_AutoPairSkipsWhenSourceUnread(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	// No seedReadFileHistory call → gc.LineIndex is empty.
	params := json.RawMessage(`{
        "items": [
          {"kind": "direct", "subject": "Foo", "source": "internal/agent/foo.go", "line_start": 5, "summary": "Foo is defined here", "anchor_kind": "definition", "anchor_symbol": "Foo"}
        ]
    }`)
	_, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := ctx.Mutable.EmittedEvidence()
	for _, it := range got {
		if it.Producer == "auto_pair_role_description" {
			t.Errorf("auto-pair must not fire without read_file coverage; got %+v", it)
		}
	}
}

func producersOf(items []types.EvidenceItem) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = it.Producer
	}
	return strings.Join(parts, ",")
}

func TestEmitEvidence_ToolSurface(t *testing.T) {
	tool := &EmitEvidence{}
	if tool.Name() != "emit_evidence" {
		t.Errorf("name = %q", tool.Name())
	}
	if tool.IsWrite() {
		t.Errorf("emit_evidence must not be write-classified")
	}
	if tool.Confidence() != 0 {
		t.Errorf("confidence = %f, want 0 (NonEvidenceTool — the tool itself isn't a fact source)", tool.Confidence())
	}
	if !strings.Contains(tool.Description(), "emit_evidence") &&
		!strings.Contains(tool.Description(), "structured evidence") {
		// loose check; the description must guide the LLM
		t.Errorf("description should explain the tool's purpose")
	}
	if !json.Valid(tool.Parameters()) {
		t.Errorf("parameters JSON schema is invalid")
	}
}

// TestFindCallRelationAtLineForCandidates_SinglePass confirms the
// post-2026-05-07 refactor: the helper now walks fi.Relations once
// (set-based candidate matching) instead of K passes (one per
// candidate). Behaviour-equivalent to the old per-candidate loop.
func TestFindCallRelationAtLineForCandidates_SinglePass(t *testing.T) {
	fi := &repomap.FileInfo{
		Relations: []repomap.Relation{
			{Kind: "call", Line: 42, ToEP: repomap.RelationEndpoint{Name: "Bar"}},
			{Kind: "call", Line: 42, ToEP: repomap.RelationEndpoint{Name: "Baz"}},
			{Kind: "call", Line: 50, ToEP: repomap.RelationEndpoint{Name: "Qux"}},
			{Kind: "import", Line: 42, ToEP: repomap.RelationEndpoint{Name: "fmt"}},
		},
	}
	cases := []struct {
		name       string
		candidates []string
		line       int
		wantOK     bool
		wantTarget string
	}{
		{name: "first candidate hits", candidates: []string{"Bar", "X"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "second candidate hits", candidates: []string{"X", "Baz"}, line: 42, wantOK: true, wantTarget: "Baz"},
		{name: "qualified candidate matches short", candidates: []string{"pkg.Bar"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "rust qualified candidate matches short", candidates: []string{"walker::Bar"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "c pointer candidate matches short", candidates: []string{"svc->Bar"}, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "no candidate match does not substitute another call at line", candidates: []string{"Nope"}, line: 42, wantOK: false},
		{name: "wrong line returns false", candidates: []string{"Bar"}, line: 999, wantOK: false},
		{name: "empty candidates falls through to anchor=\"\" path", candidates: nil, line: 42, wantOK: true, wantTarget: "Bar"},
		{name: "all-whitespace candidates → fallback", candidates: []string{"", "  "}, line: 42, wantOK: true, wantTarget: "Bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, ok := findCallRelationAtLineForCandidates(fi, tc.line, tc.candidates)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if rel.ToEP.Name != tc.wantTarget {
				t.Errorf("target=%q want %q", rel.ToEP.Name, tc.wantTarget)
			}
		})
	}
}

func TestNormalizeCallEvidenceDirection_UsesSourceLineCallTargetBeforeRelationFallback(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "internal/agent/analyzer.go", 1818,
		"out := compiler.Compile(rm, sig)",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Symbols: []repomap.Symbol{
					{Name: "buildAnalysisIR", Kind: "function", Line: 1800, EndLine: 1900},
				},
				Relations: []repomap.Relation{
					{Kind: "call", Line: 1818, ToEP: repomap.RelationEndpoint{Name: "RecomputeBudget"}},
				},
			},
		},
	})
	params := json.RawMessage(`{
		"items": [
			{
				"evidence_kind": "direct",
				"subject": "buildAnalysisIR",
				"predicate": "calls",
				"object": "Compile",
				"source": "internal/agent/analyzer.go",
				"line_start": 1818,
				"summary": "buildAnalysisIR calls compiler.Compile",
				"anchor_kind": "call",
				"anchor_symbol": "Compile"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got: %s", res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("want 1 item, got %d", len(got))
	}
	if got[0].Object != "compiler.Compile" {
		t.Fatalf("call target = %q, want compiler.Compile", got[0].Object)
	}
}

func TestSourceLineCallTargetForCandidates_MatchesCrossLanguageSelectors(t *testing.T) {
	cases := []struct {
		name       string
		line       string
		candidate  string
		wantTarget string
	}{
		{name: "go selector", line: "out := compiler.Compile(rm, sig)", candidate: "Compile", wantTarget: "compiler.Compile"},
		{name: "arkts member", line: "this.router.pushUrl({ url: 'pages/Index' })", candidate: "pushUrl", wantTarget: "this.router.pushUrl"},
		{name: "cpp namespace", line: "auto ok = analysis::compiler::Compile(ir);", candidate: "Compile", wantTarget: "analysis::compiler::Compile"},
		{name: "cangjie-style module selector", line: "let graph = normalizer.Normalize(request)", candidate: "Normalize", wantTarget: "normalizer.Normalize"},
		{name: "ts generic", line: "const view = factory.create<ViewModel>(state)", candidate: "create", wantTarget: "factory.create"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			for _, target := range sourceLineCallTargets(tc.line) {
				if callExpressionTargetMatchesCandidate(target, tc.candidate) {
					got = target
					break
				}
			}
			if got != tc.wantTarget {
				t.Fatalf("target = %q, want %q", got, tc.wantTarget)
			}
		})
	}
}

func TestEmitEvidence_CallAnchorCannotAuthorizeAbsentRegistrationObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/registry.cpp", 32,
		"return SinkRegistry::create(kind, path);",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"src/registry.cpp": {
			RelPath: "src/registry.cpp", Language: repomap.LangCpp,
			Symbols: []repomap.Symbol{{Name: "make_sink", Kind: "function", Line: 30, EndLine: 33}},
			Relations: []repomap.Relation{{
				Kind: "call", Line: 32, ToEP: repomap.RelationEndpoint{Name: "SinkRegistry::create"},
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"registration","subject":"make_sink","predicate":"binds","object":"ConsoleSink","source":"src/registry.cpp","line_start":32,"summary":"wrapper binds ConsoleSink","anchor_kind":"call","anchor_symbol":"SinkRegistry::create"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("evidence audit should return typed feedback, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 {
		t.Fatalf("evidence count=%d want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("absent relation endpoint gained authority: %+v", got[0])
	}
	if !strings.Contains(got[0].GroundingNote, `relation object "ConsoleSink" is not present`) ||
		!strings.Contains(res.Summary, "Do NOT repair this item by moving it to a wrapper call") {
		t.Fatalf("same-turn repair feedback missing endpoint diagnosis: %s", res.Summary)
	}
	rows := (types.EvidenceRelationCandidateSource{Items: got}).TypedRelationCandidates(types.TypedRelationQuery{
		Kinds: []types.TypedRelationKind{types.TypedRelationRegisters}, Sources: []string{"make_sink"},
		Purpose: types.TypedRelationPurposePromptHint,
	})
	if len(rows) != 0 {
		t.Fatalf("downgraded row leaked into typed relation provider: %+v", rows)
	}
}

func TestEmitEvidence_RejectsCollapsedGuardedCallAndSparseRegistrationRows(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "guarded call must use separate condition row",
			payload: `{"items":[{"scope":"line","evidence_kind":"conditional","subject":"Logger::log","predicate":"calls","object":"Sink::flush","source":"src/logger.cpp","line_start":38,"condition":"level >= Level::kError","summary":"conditional flush","anchor_kind":"call","anchor_symbol":"flush"}]}`,
			want:    "anchor_kind=condition",
		},
		{
			name:    "registration requires both typed endpoints",
			payload: `{"items":[{"scope":"line","evidence_kind":"registration","object":"ConsoleSink","source":"src/registry.cpp","line_start":15,"summary":"factory mapping","anchor_kind":"definition","anchor_symbol":"create"}]}`,
			want:    "require both subject and object",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (&EmitEvidence{}).Execute(newEmitCtx(), json.RawMessage(tc.payload))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Success || !strings.Contains(res.Summary, tc.want) {
				t.Fatalf("collapsed row was not rejected with precise repair; success=%v summary=%s", res.Success, res.Summary)
			}
		})
	}
}

func TestEmitEvidence_DefinitionCannotAuthorizeFactorySelectionObject(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/registry.cpp", 15,
		"static std::unique_ptr<Sink> create(const std::string& kind) {",
	)
	params := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"SinkRegistry::create","predicate":"maps","object":"ConsoleSink","source":"src/registry.cpp","line_start":15,"summary":"factory maps console to ConsoleSink","anchor_kind":"definition","anchor_symbol":"create"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("endpoint audit should return typed feedback, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Kind != types.EvidenceUnresolved || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("definition laundered an absent factory target: %+v", got)
	}
	for _, want := range []string{"registration object \"ConsoleSink\" is absent", "conditional/condition", "direct/return"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("same-turn split-form guidance missing %q: %s", want, res.Summary)
		}
	}
}

func TestRegistrationBindingCallOnLine_CrossLanguageStructuralShapes(t *testing.T) {
	tests := []struct {
		name, line, endpoint, anchor, registry, object string
	}{
		{
			name: "rust macro wrapper", line: "m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
			endpoint: "tokenize_bytes", anchor: "add_function", registry: "m", object: "wrap_pyfunction!(tokenize_bytes, m)",
		},
		{
			name: "arkts direct handler", line: "router.register(routeName, PageHandler)",
			endpoint: "PageHandler", anchor: "register", registry: "router", object: "PageHandler",
		},
		{
			name: "cangjie wrapper", line: "registry.bind(HandlerFactory.create(RequestHandler))",
			endpoint: "RequestHandler", anchor: "bind", registry: "registry", object: "HandlerFactory.create(RequestHandler)",
		},
		{
			name: "cpp receiver", line: "registry->add(make_handler<RequestHandler>());",
			endpoint: "RequestHandler", anchor: "add", registry: "registry", object: "make_handler<RequestHandler>()",
		},
		{
			name: "go receiver", line: "registry.Add(NewHandler())",
			endpoint: "NewHandler", anchor: "Add", registry: "registry", object: "NewHandler()",
		},
		{
			name: "python receiver", line: "registry.add(make_handler())",
			endpoint: "make_handler", anchor: "add", registry: "registry", object: "make_handler()",
		},
		{
			name: "lua method receiver", line: "registry:add(make_handler())",
			endpoint: "make_handler", anchor: "add", registry: "registry", object: "make_handler()",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			anchor, registry, object, _, ok := registrationBindingCallOnLine(tc.line, tc.endpoint)
			if !ok || anchor != tc.anchor || registry != tc.registry || object != tc.object {
				t.Fatalf("binding=(%q,%q,%q,%t), want (%q,%q,%q,true)", anchor, registry, object, ok, tc.anchor, tc.registry, tc.object)
			}
		})
	}
}

func TestReceiverRegistrationBindingCallsOnLine_PolyglotReceiverSyntax(t *testing.T) {
	tests := []struct {
		name, line, anchor, registry, object string
	}{
		{"rust", "m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;", "add_function", "m", "wrap_pyfunction!(tokenize_bytes, m)"},
		{"go", "registry.Add(NewHandler())", "Add", "registry", "NewHandler()"},
		{"python", "registry.add(make_handler())", "add", "registry", "make_handler()"},
		{"java", "registry.add(HandlerFactory.create())", "add", "registry", "HandlerFactory.create()"},
		{"kotlin", "registry.add(makeHandler())", "add", "registry", "makeHandler()"},
		{"swift", "registry.add(makeHandler())", "add", "registry", "makeHandler()"},
		{"javascript", "registry.add(makeHandler())", "add", "registry", "makeHandler()"},
		{"arkts", "registry.add(makeHandler())", "add", "registry", "makeHandler()"},
		{"cangjie", "registry.bind(HandlerFactory.create(RequestHandler))", "bind", "registry", "HandlerFactory.create(RequestHandler)"},
		{"cpp", "registry->add(make_handler<RequestHandler>());", "add", "registry", "make_handler<RequestHandler>()"},
		{"lua", "registry:add(make_handler())", "add", "registry", "make_handler()"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := receiverRegistrationBindingCallsOnLine(tc.line)
			if len(got) != 1 || got[0].anchor != tc.anchor || got[0].registry != tc.registry || got[0].object != tc.object {
				t.Fatalf("binding calls=%+v, want one (%q,%q,%q)", got, tc.anchor, tc.registry, tc.object)
			}
		})
	}
}

func TestAutoPairSelectedDefinitionBodyCallEvidence_PolyglotParserMatrix(t *testing.T) {
	extensions := map[string]string{
		repomap.LangGo: ".go", repomap.LangPython: ".py", repomap.LangJavaScript: ".js",
		repomap.LangTypeScript: ".ts", repomap.LangJava: ".java", repomap.LangKotlin: ".kt",
		repomap.LangRust: ".rs", repomap.LangC: ".c", repomap.LangCpp: ".cpp",
		repomap.LangRuby: ".rb", repomap.LangSwift: ".swift", repomap.LangLua: ".lua",
		repomap.LangProto: ".proto", repomap.LangArkTS: ".ets", repomap.LangCangjie: ".cj",
	}
	for _, language := range repomap.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			ext := extensions[language]
			if ext == "" {
				t.Fatalf("supported language %q lacks matrix extension", language)
			}
			source := "src/chain" + ext
			provenance := repomap.ProvenanceTreeSitter
			if language == repomap.LangCangjie {
				provenance = repomap.ProvenanceCangjieParser
			}
			fi := &repomap.FileInfo{
				RelPath: source, Language: language,
				Symbols: []repomap.Symbol{{Name: "Caller", Kind: "function", File: source, Line: 5, EndLine: 10}},
				Relations: []repomap.Relation{{
					Kind: "call", File: source, Line: 7,
					FromEP:     repomap.RelationEndpoint{Name: "Caller"},
					ToEP:       repomap.RelationEndpoint{Name: "Helper"},
					Confidence: repomap.ConfidenceAST, Provenance: provenance,
					ResolvedBy: language + "_parser_call",
				}},
			}
			graph := &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: fi}}
			gc := &ground.Context{
				Graph: graph,
				LineIndex: map[string]map[int]string{source: {
					5: "callable Caller definition", 7: "Helper()",
				}},
			}
			ctx := newEmitCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentExplain, PredicateAxis: types.AxisCall,
				AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
				CallChainEndpointProfile: &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath},
			}}
			selected := types.EvidenceItem{
				ID: "selected", Kind: types.EvidenceMechanism, Subject: "Caller",
				Source: source, LineStart: 5, Scope: types.ScopeLine,
				AnchorKind: types.AnchorDefinition, AnchorSymbol: "Caller",
				GroundingStatus: types.GroundingGrounded,
			}
			got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc)
			if len(got) != 1 || got[0].Subject != "Caller" || got[0].Object != "Helper" ||
				got[0].LineStart != 7 || got[0].Producer != types.EvidenceProducerRepoMapSelectedCallableBodyCall ||
				types.ClaimFormOf(got[0]) != types.ClaimCallEdge || !got[0].IsCitable() {
				t.Fatalf("language-neutral selected-body call missing for %s: %+v", language, got)
			}
		})
	}
}

func TestAutoPairSelectedDefinitionBodyCallEvidence_MechanismFunctionDimension(t *testing.T) {
	const source = "src/clock.c"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangC,
		Symbols: []repomap.Symbol{
			{Name: "monotonic_now_ns", Kind: "function", File: source, Line: 11, EndLine: 19},
			{Name: "monotonic_now_ns", Kind: "function", File: source, Line: 25, EndLine: 32},
			{Name: "monotonic_now_ns", Kind: "function", File: source, Line: 37, EndLine: 41},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 17,
			FromEP:     repomap.RelationEndpoint{Name: "monotonic_now_ns"},
			ToEP:       repomap.RelationEndpoint{Name: "QueryPerformanceCounter"},
			Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter,
			ResolvedBy: "c_ast_call",
		}},
	}
	gc := &ground.Context{
		Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: fi}},
		LineIndex: map[string]map[int]string{
			source: {11: "uint64_t monotonic_now_ns(void) {", 17: "QueryPerformanceCounter(&counter);"},
		},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Label: "平台实现方式", Role: types.RequestedAnswerDimensionFunctionOrPurpose,
				Required: true, Index: 1,
			}},
		},
	}}
	selected := types.EvidenceItem{
		ID: "selected", Kind: types.EvidenceMechanism, Subject: "monotonic_now_ns",
		Source: source, LineStart: 11, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "monotonic_now_ns",
		ContextRole:     types.EvidenceContextRoleDefining,
		GroundingStatus: types.GroundingGrounded,
	}

	got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc)
	if len(got) != 1 || got[0].Subject != "monotonic_now_ns" || got[0].Object != "QueryPerformanceCounter" ||
		got[0].LineStart != 17 || got[0].Producer != types.EvidenceProducerRepoMapSelectedCallableBodyCall ||
		types.ClaimFormOf(got[0]) != types.ClaimCallEdge || !got[0].IsCitable() {
		t.Fatalf("typed mechanism function dimension should expose exact selected-body call: %+v", got)
	}

	selected.ContextRole = types.EvidenceContextRoleRelatedContext
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("related mechanism definition must not mint body-call facts: %+v", got)
	}

	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"monotonic_now_ns"}
	selected.ContextRole = types.EvidenceContextRoleDefining
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 1 {
		t.Fatalf("typed exact mechanism target should survive member-set dimension drift: %+v", got)
	}

	ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = []string{"monotonic_now_ns", "cmd_sleep"}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 1 {
		t.Fatalf("typed primary mechanism entity should survive omitted exact-target lane: %+v", got)
	}

	ctx.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = []string{"another_clock"}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("another typed exact target must not admit this definition body: %+v", got)
	}

	ctx.AnalysisIR.RequestModel.AnalyzerHints.ExactTargets = nil
	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = []string{"monotonic_now_ns"}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("breadth-expanded entities alone must not admit mechanism body facts: %+v", got)
	}

	ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities = nil
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("plain mechanism kind without typed dimension or exact target must remain unchanged: %+v", got)
	}
}

func TestEmitEvidence_SelectedDefinitionAutoPairsExactParserBodyCall(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisCall,
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath},
	}}
	const source = "src/chain.go"
	seedReadFileHistory(ctx, source, 5,
		"func Caller() {",
		"prepare()",
		"Helper()",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangGo,
			Symbols: []repomap.Symbol{{Name: "Caller", Kind: "function", File: source, Line: 5, EndLine: 8}},
			Relations: []repomap.Relation{{
				Kind: "call", File: source, Line: 7,
				FromEP: repomap.RelationEndpoint{Name: "Caller"}, ToEP: repomap.RelationEndpoint{Name: "Helper"},
				Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"Caller","predicate":"implements","object":"call chain step","source":"src/chain.go","line_start":5,"summary":"selected callable body","anchor_kind":"definition","anchor_symbol":"Caller"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("selected definition should accept and pair parser call, err=%v result=%+v", err, res)
	}
	var paired *types.EvidenceItem
	for _, item := range ctx.Mutable.EmittedEvidence() {
		if item.Producer == types.EvidenceProducerRepoMapSelectedCallableBodyCall {
			copy := item
			paired = &copy
			break
		}
	}
	if paired == nil || paired.Subject != "Caller" || paired.Object != "Helper" || paired.LineStart != 7 ||
		!paired.IsCitable() || types.ClaimFormOf(*paired) != types.ClaimCallEdge {
		t.Fatalf("exact parser body call did not reach typed evidence pool: %+v", ctx.Mutable.EmittedEvidence())
	}
}

func TestEmitEvidence_SelectedDefinitionAutoPairedCallPublishesCarrierArgumentRepair(t *testing.T) {
	const (
		callSource = "internal/orchestrator/extract_work.go"
		declSource = "internal/orchestrator/orchestrator.go"
		callLine   = "ac := ctxbuilder.BuildAgentContext(o.busCtx, types.AgentExtractor, types.StageExtract)"
	)
	callFile := &repomap.FileInfo{
		RelPath: callSource, Language: repomap.LangGo, Package: "orchestrator",
		Symbols: []repomap.Symbol{{
			Name: "extractStageHasRequiredWork", Kind: "method", Receiver: "Orchestrator",
			File: callSource, Line: 11, EndLine: 17,
		}},
		Relations: []repomap.Relation{{
			Kind: "call", File: callSource, Line: 15,
			FromEP: repomap.RelationEndpoint{
				Name: "extractStageHasRequiredWork", Receiver: "Orchestrator", File: callSource, Line: 15,
			},
			ToEP: repomap.RelationEndpoint{
				Name: "BuildAgentContext", Receiver: "ctxbuilder", File: callSource, Line: 15,
			},
			Confidence: repomap.ConfidenceAST, Provenance: repomap.ProvenanceTreeSitter,
			ResolvedBy: "go_ast_call",
		}},
	}
	declarationFile := &repomap.FileInfo{
		RelPath: declSource, Language: repomap.LangGo, Package: "orchestrator",
		Symbols: []repomap.Symbol{{
			Name: "busCtx", Kind: "field", Parent: "Orchestrator", DeclaredType: "*types.BusContext",
			File: declSource, Line: 55, EndLine: 55,
		}},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqMechanism), ExactTargets: []string{"extractStageHasRequiredWork"},
		},
		DiagramHint: &types.DiagramHint{
			Kind: types.DiagramArchitecture, Required: true,
			Participants: []types.DiagramParticipantHint{{
				Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired,
			}},
		},
	}}
	ctx.Mutable.SetSearchGraph(callTargetTestGraph(callFile, declarationFile))
	seedReadFileHistory(ctx, callSource, 11,
		"func (o *Orchestrator) extractStageHasRequiredWork() bool {",
		"if o == nil || o.busCtx == nil {",
		"return true",
		"}",
		callLine,
		"return agent.ExtractStageHasRequiredWork(ac)",
		"}",
	)

	definitionPayload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"mechanism","subject":"extractStageHasRequiredWork","predicate":"implements","object":"stage readiness","source":%q,"line_start":11,"summary":"selected stage readiness callable","anchor_kind":"definition","anchor_symbol":"extractStageHasRequiredWork"}]}`, callSource))
	res, err := (&EmitEvidence{}).Execute(ctx, definitionPayload)
	if err != nil || !res.Success {
		t.Fatalf("selected definition failed, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "call_argument_flow_pair" ||
		res.Repair.Metadata["completion_blocking"] != "true" ||
		!strings.Contains(res.Repair.Hint, "parser-owned direct call projected from the accepted model-selected callable") ||
		!strings.Contains(res.Repair.Hint, `subject="o.busCtx"`) ||
		!strings.Contains(res.Repair.Hint, `object="ctxbuilder.BuildAgentContext"`) {
		t.Fatalf("auto-paired call must expose its exact carrier argument sibling: %+v", res.Repair)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) < 2 {
		t.Fatalf("definition and parser-owned call should be accepted before repair: %+v", got)
	}
	for _, item := range got {
		if item.AnchorKind == types.AnchorArgument {
			t.Fatalf("system must not mint the argument relation: %+v", got)
		}
	}

	argumentPayload := json.RawMessage(fmt.Sprintf(`{"items":[{"scope":"line","evidence_kind":"relationship","subject":"o.busCtx","predicate":"passes argument","object":"ctxbuilder.BuildAgentContext","source":%q,"line_start":15,"summary":"the bus context enters stage context construction","anchor_kind":"argument","anchor_symbol":"o.busCtx","snippet":%q}]}`, callSource, callLine))
	res, err = (&EmitEvidence{}).Execute(ctx, argumentPayload)
	if err != nil || !res.Success {
		t.Fatalf("exact argument sibling failed, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_status"] == types.ToolRepairStatusActionRequired {
		t.Fatalf("model-authored argument sibling must close the durable repair: %+v", res.Repair)
	}
}

func TestAutoPairSelectedDefinitionBodyCallEvidence_DoesNotDuplicateExactModelCall(t *testing.T) {
	const source = "src/chain.go"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangGo,
		Symbols: []repomap.Symbol{{Name: "Caller", Kind: "function", File: source, Line: 5, EndLine: 8}},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 7,
			ToEP: repomap.RelationEndpoint{Name: "Helper"}, Confidence: repomap.ConfidenceAST,
			Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "go_ast_call",
		}},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisCall,
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath},
	}}
	selected := types.EvidenceItem{
		ID: "selected", Kind: types.EvidenceMechanism, Subject: "Caller", Source: source, LineStart: 5,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Caller",
		GroundingStatus: types.GroundingGrounded,
	}
	exact := types.EvidenceItem{
		ID: "exact", Kind: types.EvidenceRelationship, Subject: "Caller", Predicate: "calls", Object: "Helper",
		Source: source, LineStart: 7, Scope: types.ScopeLine, AnchorKind: types.AnchorCall, AnchorSymbol: "Helper",
		GroundingStatus: types.GroundingGrounded,
	}
	gc := &ground.Context{
		Graph:     &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: fi}},
		LineIndex: map[string]map[int]string{source: {5: "func Caller()", 7: "Helper()"}},
	}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected, exact}, gc); len(got) != 0 {
		t.Fatalf("exact model-owned call must suppress deterministic duplicate: %+v", got)
	}
}

func TestAutoPairSelectedDefinitionBodyCallEvidence_RejectsUnreadAndAmbiguousDefinitions(t *testing.T) {
	const source = "src/chain.rs"
	fi := &repomap.FileInfo{
		RelPath: source, Language: repomap.LangRust,
		Symbols: []repomap.Symbol{
			{Name: "Caller", Kind: "function", File: source, Line: 5, EndLine: 10},
			{Name: "Caller", Kind: "function", File: source, Line: 6, EndLine: 11},
		},
		Relations: []repomap.Relation{{
			Kind: "call", File: source, Line: 8,
			ToEP: repomap.RelationEndpoint{Name: "Helper"}, Confidence: repomap.ConfidenceAST,
			Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "rust_ast_identifier_call",
		}},
	}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentExplain, PredicateAxis: types.AxisCall,
		AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{SinkMode: types.CallChainSinkResolutionDiscoverPath},
	}}
	selected := types.EvidenceItem{
		ID: "selected", Kind: types.EvidenceMechanism, Subject: "Caller", Source: source, LineStart: 5,
		Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Caller",
		GroundingStatus: types.GroundingGrounded,
	}
	gc := &ground.Context{Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: fi}}, LineIndex: map[string]map[int]string{source: {8: "Helper()"}}}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("ambiguous definition identity must fail open: %+v", got)
	}
	fi.Symbols = fi.Symbols[:1]
	gc.LineIndex[source] = map[int]string{5: "fn Caller()"}
	if got := autoPairSelectedDefinitionBodyCallEvidence(ctx, []types.EvidenceItem{selected}, gc); len(got) != 0 {
		t.Fatalf("unread relation line must not become evidence: %+v", got)
	}
}

func TestUniqueNearbyRegistrationBindingCallFailsOpenWhenMoreThanOneLineBindsEndpoint(t *testing.T) {
	const source = "src/registry.ets"
	gc := &ground.Context{LineIndex: map[string]map[int]string{
		source: {
			10: "primary.register(PageHandler)",
			11: "fallback.register(PageHandler)",
		},
	}}
	item := types.EvidenceItem{Source: source, LineStart: 10, Object: "PageHandler"}
	if line, anchor, registry, object, ok := uniqueNearbyRegistrationBindingCall(gc, item); ok {
		t.Fatalf("ambiguous binding lines must fail open, got line=%d anchor=%q registry=%q object=%q", line, anchor, registry, object)
	}
}

func TestEmitEvidence_CrossComponentRegistrationDefinitionPublishesDurableBindingRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}
	const source = "core-rs/src/lib.rs"
	seedReadFileHistory(ctx, source, 45,
		"#[pymodule]",
		"fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {",
		"m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
		"Ok(())",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangRust,
			Symbols: []repomap.Symbol{{Name: "_fastlex", Kind: "function", Line: 46, EndLine: 49}},
		},
	}})

	bad := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"_fastlex","predicate":"registers","object":"tokenize_bytes","source":"core-rs/src/lib.rs","line_start":46,"summary":"module registration","anchor_kind":"definition","anchor_symbol":"_fastlex"}]}`)
	res, err := tool.Execute(ctx, bad)
	if err != nil || !res.Success {
		t.Fatalf("nearby definition should return typed repair, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "registration_binding_expression" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("missing durable binding repair: %+v", res.Repair)
	}
	for _, want := range []string{
		`source="core-rs/src/lib.rs"`, `line_start=47`, `anchor_symbol="add_function"`,
		`subject="m"`, `object="wrap_pyfunction!(tokenize_bytes, m)"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("binding repair missing %q: %+v", want, res.Repair)
		}
	}
	if strings.Contains(res.Summary, "Current actionable repair targets: none") ||
		strings.Contains(res.Summary, "drop the item; do NOT spend read_file budget") {
		t.Fatalf("summary contradicted durable binding repair:\n%s", res.Summary)
	}
	obligations, ok := decodeEmitEvidenceRelationRepairObligations(
		res.Repair.Metadata[emitEvidenceRelationRepairObligationsMetadataKey])
	if !ok || len(obligations) != 1 || obligations[0].EvidenceKind != types.EvidenceRegistration ||
		obligations[0].Line != 47 || obligations[0].Object != "wrap_pyfunction!(tokenize_bytes, m)" {
		t.Fatalf("binding obligation is not exact: %+v", obligations)
	}
	ctx.Mutable.AppendDispatchToolResult(res)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "emit_evidence", Success: true, Summary: "unrelated later success"})
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(ctx); pending == nil ||
		pending.Metadata["repair_scope"] != "registration_binding_expression" {
		t.Fatalf("later unrelated emit cleared durable binding debt: %+v", pending)
	}

	corrected := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"m","predicate":"registers","object":"wrap_pyfunction!(tokenize_bytes, m)","source":"core-rs/src/lib.rs","line_start":47,"summary":"module binds the exported wrapper","anchor_kind":"call","anchor_symbol":"add_function"}]}`)
	res, err = tool.Execute(ctx, corrected)
	if err != nil || !res.Success {
		t.Fatalf("exact binding expression rejected, err=%v result=%+v", err, res)
	}
	ctx.Mutable.AppendDispatchToolResult(res)
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(ctx); pending != nil {
		t.Fatalf("model-owned exact binding emit did not clear debt: %+v", pending)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || !got[1].IsCitable() || types.ClaimFormOf(got[1]) != types.ClaimRegistrationEdge {
		t.Fatalf("corrected model emit did not publish registration authority: %+v", got)
	}
}

func TestEmitEvidence_CitableRegistrationContainerStillRequiresExactBindingExpression(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}
	const source = "core-rs/src/lib.rs"
	seedReadFileHistory(ctx, source, 45,
		"#[pymodule]",
		"fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {",
		"m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
		"Ok(())",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangRust,
			Symbols: []repomap.Symbol{{Name: "_fastlex", Kind: "function", Line: 46, EndLine: 49}},
		},
	}})

	// Production witness r579: this declaration row is itself citable because
	// it names the module container.  It still does not prove which wrapper is
	// bound into that container; the unique parser-owned body call must remain
	// an exact model re-emit obligation.
	coarse := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"pyo3 #[pymodule]","predicate":"exports","object":"_fastlex","source":"core-rs/src/lib.rs","line_start":45,"summary":"module declaration and registration container","anchor_kind":"definition","anchor_symbol":"_fastlex"}]}`)
	res, err := tool.Execute(ctx, coarse)
	if err != nil || !res.Success {
		t.Fatalf("citable container should return typed repair, err=%v result=%+v", err, res)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || !got[0].IsCitable() || got[0].Kind != types.EvidenceRegistration {
		t.Fatalf("coarse container observation should remain citable and unchanged: %+v", got)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "registration_binding_expression" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("citable container escaped exact binding debt: %+v", res.Repair)
	}
	for _, want := range []string{
		`source="core-rs/src/lib.rs"`, `line_start=47`, `anchor_kind="call"`,
		`anchor_symbol="add_function"`, `subject="m"`,
		`object="wrap_pyfunction!(tokenize_bytes, m)"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("binding repair missing %q: %+v", want, res.Repair)
		}
	}
}

func TestEmitEvidence_CitableRegistrationContainerAndExactBindingSameBatchNeedNoRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentExplain,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}
	const source = "core-rs/src/lib.rs"
	seedReadFileHistory(ctx, source, 45,
		"#[pymodule]",
		"fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {",
		"m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
		"Ok(())",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangRust,
			Symbols: []repomap.Symbol{{Name: "_fastlex", Kind: "function", Line: 46, EndLine: 49}},
		},
	}})

	params := json.RawMessage(`{"items":[
		{"scope":"line","evidence_kind":"registration","subject":"pyo3 #[pymodule]","predicate":"exports","object":"_fastlex","source":"core-rs/src/lib.rs","line_start":45,"summary":"module declaration and registration container","anchor_kind":"definition","anchor_symbol":"_fastlex"},
		{"scope":"line","evidence_kind":"registration","subject":"m","predicate":"registers","object":"wrap_pyfunction!(tokenize_bytes, m)","source":"core-rs/src/lib.rs","line_start":47,"summary":"module binds the exported wrapper","anchor_kind":"call","anchor_symbol":"add_function"}
	]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("coarse plus exact registration should pass, err=%v result=%+v", err, res)
	}
	if res.Repair != nil && res.Repair.Metadata["repair_scope"] == "registration_binding_expression" {
		t.Fatalf("same-batch exact model row did not satisfy binding debt: %+v", res.Repair)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 || !got[0].IsCitable() || !got[1].IsCitable() {
		t.Fatalf("model-owned registration rows were not preserved: %+v", got)
	}
}

func TestEmitEvidence_CrossComponentRegistrationWrongCallbackAnchorPublishesDurableBindingRepair(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
	}}
	const source = "core-rs/src/lib.rs"
	seedReadFileHistory(ctx, source, 45,
		"#[pymodule]",
		"fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {",
		"m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?;",
		"Ok(())",
		"}",
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		source: {
			RelPath: source, Language: repomap.LangRust,
			Symbols: []repomap.Symbol{{Name: "_fastlex", Kind: "function", Line: 46, EndLine: 49}},
		},
	}})

	// Production witness r577: the model recognized a registration but used
	// callback semantics and compressed the syntax-owned endpoints.  Initial
	// grounding rejects that shape; the common post-grounding audit must still
	// return the unique exact registration tuple instead of allowing completion
	// with two disconnected call components.
	wrong := json.RawMessage(`{"items":[{"scope":"line","evidence_kind":"registration","subject":"_fastlex","predicate":"binds","object":"tokenize_bytes","source":"core-rs/src/lib.rs","line_start":47,"summary":"exports the wrapper","anchor_kind":"callback","anchor_symbol":"wrap_pyfunction!(tokenize_bytes, m)"}]}`)
	res, err := tool.Execute(ctx, wrong)
	if err != nil || !res.Success {
		t.Fatalf("wrong source-shape row should return typed repair, err=%v result=%+v", err, res)
	}
	if res.Repair == nil || res.Repair.Metadata["repair_scope"] != "registration_binding_expression" ||
		res.Repair.Metadata["completion_blocking"] != "true" {
		t.Fatalf("wrong callback anchor escaped durable binding repair: %+v", res.Repair)
	}
	for _, want := range []string{
		`source="core-rs/src/lib.rs"`, `line_start=47`, `anchor_kind="call"`,
		`anchor_symbol="add_function"`, `subject="m"`,
		`object="wrap_pyfunction!(tokenize_bytes, m)"`,
	} {
		if !strings.Contains(res.Repair.Hint, want) {
			t.Fatalf("binding repair missing %q: %+v", want, res.Repair)
		}
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].IsCitable() || got[0].GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("system must preserve the rejected model row without minting authority: %+v", got)
	}

	ctx.Mutable.AppendDispatchToolResult(res)
	if pending := pendingBlockingEmitEvidenceItemValidationRepair(ctx); pending == nil ||
		pending.Metadata["repair_scope"] != "registration_binding_expression" {
		t.Fatalf("wrong anchor repair did not remain completion-blocking: %+v", pending)
	}
}

func TestEmitEvidence_FactoryGuardAndReturnRemainSeparateAuthoritativeRows(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "src/registry.cpp", 17,
		`if (kind == "console") {`,
		"return std::make_unique<ConsoleSink>();",
	)
	params := json.RawMessage(`{"items":[
		{"scope":"line","evidence_kind":"conditional","subject":"SinkRegistry::create","predicate":"guards","source":"src/registry.cpp","line_start":17,"condition":"kind == \"console\"","summary":"console branch","anchor_kind":"condition","anchor_symbol":"kind"},
		{"scope":"line","evidence_kind":"direct","subject":"SinkRegistry::create","predicate":"returns","object":"ConsoleSink","source":"src/registry.cpp","line_start":18,"summary":"returns ConsoleSink","anchor_kind":"return","anchor_symbol":"ConsoleSink"}
	]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("separate factory facts should ground, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("evidence count=%d want 2: %+v", len(got), got)
	}
	guard := types.EvidenceDeterministicSurfaceText(got[0], false)
	selection := types.EvidenceDeterministicSurfaceText(got[1], false)
	if !strings.Contains(guard, `kind == "console"`) || !strings.Contains(selection, "ConsoleSink") {
		t.Fatalf("typed finalizer surfaces lost guard/selection: guard=%q selection=%q", guard, selection)
	}
}

func TestEmitEvidence_CallRegistrationKeepsVisibleObjectEndpoint(t *testing.T) {
	tool := &EmitEvidence{}
	ctx := newEmitCtx()
	seedReadFileHistory(ctx, "registry.go", 42,
		`registry.Register("json", JsonPlugin)`,
	)
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"registry.go": {
			RelPath: "registry.go", Language: repomap.LangGo,
			Symbols: []repomap.Symbol{{Name: "configure", Kind: "function", Line: 40, EndLine: 44}},
			Relations: []repomap.Relation{{
				Kind: "call", Line: 42, ToEP: repomap.RelationEndpoint{Name: "Register"},
			}},
		},
	}})
	params := json.RawMessage(`{"items":[{"kind":"registration","subject":"registry","predicate":"binds","object":"JsonPlugin","source":"registry.go","line_start":42,"summary":"registry binds JsonPlugin","anchor_kind":"call","anchor_symbol":"Register"}]}`)
	res, err := tool.Execute(ctx, params)
	if err != nil || !res.Success {
		t.Fatalf("visible registration should ground, err=%v summary=%s", err, res.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].Kind != types.EvidenceRegistration || got[0].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("valid registration was weakened: %+v", got)
	}
	rows := (types.EvidenceRelationCandidateSource{Items: got}).TypedRelationCandidates(types.TypedRelationQuery{
		Kinds: []types.TypedRelationKind{types.TypedRelationRegisters}, Sources: []string{"registry"},
		Purpose: types.TypedRelationPurposePromptHint,
	})
	if len(rows) != 1 || rows[0].Member.Name != "JsonPlugin" {
		t.Fatalf("valid registration disappeared from typed relation provider: %+v", rows)
	}
}
