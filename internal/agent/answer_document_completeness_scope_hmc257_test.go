package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func resourcePageContextHMC257(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	ctx.WorkDir = filepath.Join(ctx.RepoRoot, ".codrax", "blob", "query")
	var results []types.ToolResult
	for _, path := range []string{"captures/capture.data", "captures/nested/capture.data"} {
		for _, pid := range []int{101, 202} {
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": pid, "time_start": 10, "time_end": 10.05})
			r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
			if err != nil || !r.Success {
				t.Fatalf("native resource query: %v %s", err, r.Summary)
			}
			results = append(results, r)
		}
	}
	// The public execution loop registers payload navigation when it records
	// the returned tool result; direct Execute alone is not that publication.
	for _, r := range results {
		ctx.Mutable.AppendDispatchToolResult(r)
	}
	payload := ""
	for _, r := range results[0].Observations {
		if r.Predicate == tool.TraceResourceStackPredicate {
			payload = r.SourceRef.PayloadRef
		}
	}
	args, _ := json.Marshal(map[string]any{"path": payload, "line_offset": 2, "limit": 5})
	page, err := (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !page.Success || page.RuntimeArtifactRead == nil || !page.RuntimeArtifactRead.TraceQueryBlob || page.EnumerationAuthority == nil || page.EnumerationAuthority.Status != "incomplete" {
		t.Fatalf("real published result page: %v %+v", err, page)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: append(results, page)})
	ctx.Mutable.ResetDispatchToolResults()
	return ctx
}

func TestHMC257EnumerationActualContextBudget(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	var results []types.ToolResult
	for n := 0; n < 18; n++ {
		results = append(results, types.ToolResult{ToolName: "trace_query", Success: true, RawRef: fmt.Sprintf("query-%d", n), Observations: []types.ObservationRecord{{SourceRef: types.ObservationSourceRef{Path: strings.Repeat("x", 5000), PayloadRef: fmt.Sprintf("payload-%d", n), QueryScopeID: fmt.Sprintf("query-%d", n)}}}, EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete", Boundaries: []types.ToolEnumerationBoundary{{Scope: "event_search", Dimension: "events", Emitted: 1, Total: 2, TotalKnown: true}}}})
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	for _, prompt := range []string{renderAnswerDocRuntimeEnumerationAuthority(ctx), renderTraceFinalRuntimeEnumerationAuthority(ctx)} {
		if !strings.Contains(prompt, "omitted_exact_boundaries=2") || !strings.Contains(prompt, "exact_source_locators_omitted=1") || len(prompt) > 12000 || strings.Contains(prompt, strings.Repeat("x", 100)) {
			t.Fatalf("budget lost bounds or emitted partial source: bytes=%d\n%s", len(prompt), prompt)
		}
	}
}

// A real partial read of one native payload must not demote its independently
// complete event/frame tables, nor invent a causal coverage verdict.
func TestHMC257NativeCompletenessActualFinalizer(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := resourcePageContextHMC257(t)
			before, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
			prompt := dependencyObservationMessages(t, ctx)
			for _, want := range []string{"coverage_kind=`artifact_page`", "does not change independently published native rows or aggregates", "page coordinates", "Native resource fact choices"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finalizer lost scoped completeness %q", want)
				}
			}
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			const interpretation = "独立解释仍保留；来源帧不是执行顺序，未证关系继续未证。"
			interpretationKind := types.BlockSummary
			if kind == types.BlockSummary {
				interpretationKind = types.BlockSection
			}
			blocks := []types.AnswerBlock{{ID: "interpretation", Kind: interpretationKind, Title: "独立解释", Text: interpretation}}
			selectedSummary := false
			for i, table := range view.RuntimeMeasurementContract.Choices() {
				if table.HasNativeResourceFacts() {
					blockKind := kind
					if kind == types.BlockSummary {
						if selectedSummary {
							blockKind = types.BlockTable
						}
						selectedSummary = true
					}
					blocks = append(blocks, types.AnswerBlock{ID: string(rune('a' + i)), Kind: blockKind, RuntimeMeasurement: &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}})
				}
			}
			raw, _ := json.Marshal(map[string]any{"blocks": blocks})
			r, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || !r.Success {
				t.Fatalf("public answer emit: %v %s", err, r.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			events, frames, empty, queries := 0, 0, 0, 0
			seen := map[string]bool{}
			for _, b := range doc.Blocks {
				if b.RuntimeMeasurement == nil || !b.RuntimeMeasurement.IsBound() || !b.RuntimeMeasurement.BoundTable.HasNativeResourceFacts() {
					continue
				}
				table := b.RuntimeMeasurement.BoundTable
				key := table.ObservationID + "/" + string(table.View)
				if seen[key] {
					continue
				}
				seen[key] = true
				if table.View == types.RuntimeMeasurementSummary {
					queries++
					events += len(table.Rows)
					if len(table.Rows) == 0 {
						empty++
					}
				} else if table.View == types.RuntimeMeasurementMembers {
					frames += len(table.Rows)
				}
			}
			if queries != 4 || events != 5 || frames != 14 || empty != 1 || doc.Blocks[0].Text != interpretation {
				t.Fatalf("page affected native collection or interpretation: queries=%d events=%d frames=%d empty=%d", queries, events, frames, empty)
			}
			text := render.RenderAnswerDocument(doc, "zh")
			// Bounded fact-only requests intentionally do not materialize a
			// causal-report footer. Native views retain their own measurements.
			for _, want := range []string{"10005000000", "单位=bytes", "原深度"} {
				if !strings.Contains(text, want) {
					t.Errorf("rendered %s lost scope/native fact %q", kind, want)
				}
			}
			for _, bad := range []string{"当前返回项只能作为样本或下界", "没有获得可支撑根因、唤醒链或确定性工作阶段的观测"} {
				if strings.Contains(text, bad) {
					t.Errorf("a result-page read acquired global authority: %q", bad)
				}
			}
			after, _ := json.Marshal(ctx.Mutable.TurnAArtifacts().ToolResults)
			if string(before) != string(after) {
				t.Fatal("presentation changed producer facts/read coverage")
			}
		})
	}
}

func TestHMC257NativeCompletenessKeepsRealQueryTruncation(t *testing.T) {
	ctx := resourcePageContextHMC257(t)
	args := json.RawMessage(`{"source":"path","path":"captures/capture.data","view":"resource_stack","pid":101,"time_start":10,"time_end":10.05,"limit":1}`)
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !r.Success || r.EnumerationAuthority == nil || r.EnumerationAuthority.Status != "incomplete" {
		t.Fatalf("real bounded query: %v %+v", err, r)
	}
	results := append(ctx.Mutable.TurnAArtifacts().ToolResults, r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ctx.Mutable.ResetDispatchToolResults()
	prompt := dependencyObservationMessages(t, ctx)
	for _, want := range []string{"coverage_kind=`query_result`", "coverage_kind=`artifact_page`", "scope=`resource_stack`; dimension=`events`; emitted=1; total=3", "bounded samples or lower bounds, not an exhaustive census"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("partial page or independent complete query erased real compaction %q", want)
		}
	}
}
