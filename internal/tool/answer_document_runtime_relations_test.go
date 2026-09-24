package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeNestingFixture(t *testing.T) (*types.BusContext, types.ToolResult, []RuntimeDiagramRelation) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh",
		Mutable: types.NewMutableState("business nesting"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 5, "time_end": 5.012})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 {
		t.Fatalf("want two exact direct nested relations, got %+v", rows)
	}
	return ctx, result, rows
}

func runtimeNestingDoc(row RuntimeDiagramRelation, anchored bool) *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "业务层级来自同一线程的同步打点；包含关系不代表调用或根因。"},
		{ID: "diag", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: fmt.Sprintf("flowchart TD\n %s[%q] -->|包含| %s[%q]\n", row.FromNode, row.FromLabel, row.ToNode, row.ToLabel)}},
		{ID: "table", Kind: types.BlockTable, Columns: []string{"业务", "口径"}, Items: []types.AnswerBlockItem{{Cells: []string{"业务层级", "父子不叠加"}, CitationRef: -1}}},
	}}
	if anchored {
		doc.Blocks[1].EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "包含"}}
	}
	return doc
}

func TestRuntimeBusinessContainmentPublicEmitRejectPatchPost(t *testing.T) {
	ctx, _, rows := runtimeNestingFixture(t)
	doc := runtimeNestingDoc(rows[0], false)
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success {
		t.Fatalf("missing anchor must reject: %v %+v", err, res)
	}
	// The agent installs the official lease from the real tool rejection. No
	// handcrafted failure/candidate is substituted for that public response.
	var delta types.AnswerDiagramRelationRepairDelta
	if res.Repair == nil || json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta) != nil {
		t.Fatalf("missing real repair delta: %+v", res)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(ctx.Mutable.LastRejectedAnswerDocumentV2(), delta.Failures, delta.AllowedAdditions)
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	if lease == nil || !types.AnswerDiagramRelationRepairHasExecutableAttachPair(lease.Failures, lease.AllowedAdditions) {
		t.Fatalf("real rejection must publish executable attach pair: %+v / %+v", lease, res)
	}
	var failure types.AnswerDiagramRelationRepairFailure
	var candidate types.AnswerDiagramRelationRepairCandidate
	for _, f := range lease.Failures {
		for _, c := range lease.AllowedAdditions {
			if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c) {
				failure, candidate = f, c
			}
		}
	}
	if candidate.RelationKind != types.DiagramRelContain {
		t.Fatalf("wrong repair kind: %+v", candidate)
	}
	schema := string((&EmitAnswerDocumentPatch{}).parametersForContext(nil, ctx.Mutable, ctx))
	if !strings.Contains(schema, "attach") || !strings.Contains(schema, candidate.AdditionRef) {
		t.Fatal("actual patch schema omits published repair")
	}
	before := ctx.Mutable.LastRejectedAnswerDocumentV2()
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{{Action: "attach", FailureRef: failure.FailureRef, AdditionRef: candidate.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: failure.FromNode, ToNode: failure.ToNode, VisibleLabel: "包含"}}}})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("precise attach rejected: %v %+v", err, res)
	}
	after := ctx.Mutable.AnswerDocumentV2()
	if !reflect.DeepEqual(before.Blocks[0], after.Blocks[0]) || !reflect.DeepEqual(before.Blocks[2], after.Blocks[2]) {
		t.Fatal("repair changed non-diagram blocks")
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, after, view, nil); len(got) > 0 {
		t.Fatalf("post-validator disagrees: %+v", got)
	}
	if got := DiagramCallEdgeEvidenceMismatches(after, view, nil); len(got) == 0 {
		t.Fatal("context-free source gate must not trust runtime enum/IDs")
	}
}

func TestRuntimeBusinessContainmentPublicEmitAuthorityNegativeMatrix(t *testing.T) {
	for _, name := range []string{"reverse", "call", "precedence", "name_only", "forged_identity", "cross_query", "grandchild", "missing_parent", "forged_typed_row", "arbitrary_alias_without_anchor", "stable_alias_wrong_instance", "missing_identity_pair"} {
		t.Run(name, func(t *testing.T) {
			ctx, result, rows := runtimeNestingFixture(t)
			row := rows[0]
			if name == "grandchild" {
				for _, a := range rows {
					for _, b := range rows {
						if a.ToIdentity == b.FromIdentity {
							row = a
							row.ToIdentity = b.ToIdentity
							row.ToNode = b.ToNode
							row.ToLabel = b.ToLabel
						}
					}
				}
			}
			doc := runtimeNestingDoc(row, true)
			a := &doc.Blocks[1].EdgeAnchors[0]
			switch name {
			case "reverse":
				a.FromIdentity, a.ToIdentity = a.ToIdentity, a.FromIdentity
			case "call":
				a.RelationKind = types.DiagramRelCall
			case "precedence":
				a.RelationKind = types.DiagramRelPrecedence
			case "name_only":
				a.FromIdentity, a.ToIdentity = row.FromLabel, row.ToLabel
			case "forged_identity":
				a.ToIdentity += "bad"
			case "stable_alias_wrong_instance":
				for _, other := range rows {
					if other.FromIdentity != row.FromIdentity {
						a.FromIdentity, a.ToIdentity = other.FromIdentity, other.ToIdentity
					}
				}
			case "missing_identity_pair":
				a.FromIdentity, a.ToIdentity = "", ""
			case "cross_query":
				ledger := types.ObservationLedger{Records: append([]types.ObservationRecord(nil), result.Observations...)}
				for i := range ledger.Records {
					ledger.Records[i].SourceRef.QueryScopeID += "other"
				}
				other := RuntimeDiagramRelations(ledger, nil)
				a.ToIdentity = other[0].ToIdentity
			case "missing_parent", "forged_typed_row":
				for i := range result.Observations {
					r := &result.Observations[i]
					if f, ok := DecodeTraceBusinessTreeFact(*r); ok && f.Node.Name == row.FromLabel {
						if name == "missing_parent" {
							r.Predicate = "unrelated"
						} else {
							r.Unit = "ns"
						}
					}
				}
				ctx.Mutable = types.NewMutableState("negative authority")
				ctx.Mutable.AppendDispatchToolResult(result)
			case "arbitrary_alias_without_anchor":
				doc.Blocks[1].EdgeAnchors = nil
				doc.Blocks[1].Diagram.Body = fmt.Sprintf("flowchart TD\n A[%q] --> B[%q]\n", row.FromLabel, row.ToLabel)
			}
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success {
				t.Fatalf("unproved relation accepted: %v %+v", err, res)
			}
			if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) == 0 {
				t.Fatal("post-validator accepted unproved relation")
			}
		})
	}
}

func TestRuntimeBusinessContainmentPublicPatchCannotBorrowCandidateOrStaleReceipt(t *testing.T) {
	for _, name := range []string{"different_candidate", "stale_runtime_receipt"} {
		t.Run(name, func(t *testing.T) {
			ctx, result, rows := runtimeNestingFixture(t)
			raw, _ := json.Marshal(runtimeNestingDoc(rows[0], false))
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("missing first rejection: %v %+v", err, res)
			}
			var delta types.AnswerDiagramRelationRepairDelta
			if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
				t.Fatal(err)
			}
			base := ctx.Mutable.LastRejectedAnswerDocumentV2()
			lease := types.NewAnswerDiagramRelationRepairLease(base, delta.Failures, delta.AllowedAdditions)
			if lease == nil {
				t.Fatal("missing lease")
			}
			f := lease.Failures[0]
			var selected types.AnswerDiagramRelationRepairCandidate
			for _, c := range lease.AllowedAdditions {
				match := types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c)
				if match == (name == "stale_runtime_receipt") {
					selected = c
					break
				}
			}
			if selected.AdditionRef == "" {
				t.Fatalf("candidate setup missing: %+v", lease)
			}
			if name == "stale_runtime_receipt" {
				for i := range result.Observations {
					if result.Observations[i].Predicate == types.TraceBusinessTreePredicate {
						result.Observations[i].Unit = "ns"
					}
				}
				ctx.Mutable = types.NewMutableState("changed evidence")
				ctx.Mutable.AppendDispatchToolResult(result)
				ctx.Mutable.SetLastRejectedAnswerDocumentV2(base)
			}
			ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
			raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{{Action: "attach", FailureRef: f.FailureRef, AdditionRef: selected.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: f.FromNode, ToNode: f.ToNode, VisibleLabel: "包含"}}}})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			if err != nil || res.Success {
				t.Fatalf("unsafe patch accepted: %v %+v", err, res)
			}
			if ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatal("failed repair became accepted answer")
			}
			if got := ctx.Mutable.LastRejectedAnswerDocumentV2(); got != nil && (!reflect.DeepEqual(got.Blocks[0], base.Blocks[0]) || !reflect.DeepEqual(got.Blocks[2], base.Blocks[2])) {
				t.Fatal("failed repair changed unrelated blocks")
			}
		})
	}
}

func TestRuntimeBusinessContainmentScopeWindowAndTarget(t *testing.T) {
	ctx, result, _ := runtimeNestingFixture(t)
	start, end := 5.0, 5.012
	rm := ctx.AnalysisIR.RequestModel
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5..5.012"}
	ledger := types.ObservationLedger{Records: result.Observations}
	if len(RuntimeDiagramRelations(ledger, &rm)) != 2 {
		t.Fatal("exact requested window lost valid relations")
	}
	end = 5.007
	if len(RuntimeDiagramRelations(ledger, &rm)) != 0 {
		t.Fatal("broad query borrowed into narrower explicit window")
	}
	rm.RuntimeArtifactScopeProfile = nil
	rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}, SourceQuote: "target time"}
	rm.RuntimeTargets = []types.RuntimeTarget{{PID: 900, Source: "user"}}
	if len(RuntimeDiagramRelations(ledger, &rm)) != 0 {
		t.Fatal("background query nested business borrowed into requested target")
	}
	rm.RuntimeTargets[0].PID = 700
	if len(RuntimeDiagramRelations(ledger, &rm)) != 2 {
		t.Fatal("target projection lost direct relations")
	}
}

func TestRuntimeBusinessContainmentUnrelatedTreeDoesNotChangeSourceGate(t *testing.T) {
	ctx, _, _ := runtimeNestingFixture(t)
	view := &types.AnswerSemanticView{Family: types.QFCallChain, RelationAxis: types.AxisFlow}
	evidence := []types.EvidenceItem{diagramEvidenceTestCall("Alpha.Run", "Beta.Run")}
	for _, kind := range []types.DiagramRelationKind{types.DiagramRelCall, types.DiagramRelContain} {
		doc := diagramEvidenceTestDoc("A", "B")
		doc.Blocks[0].EdgeAnchors[0].RelationKind = kind
		before := DiagramCallEdgeEvidenceMismatches(doc, view, evidence)
		after := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, evidence)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("unrelated runtime evidence changed source %s: before=%+v after=%+v", kind, before, after)
		}
		if kind == types.DiagramRelCall && len(after) != 0 {
			t.Fatalf("valid source call rejected: %+v", after)
		}
		if kind == types.DiagramRelContain && len(after) == 0 {
			t.Fatal("unproved source containment gained authority")
		}
	}
}

func TestRuntimeBusinessContainmentProviderScopeAndProjection(t *testing.T) {
	ctx, result, rows := runtimeNestingFixture(t)
	before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: result.Observations}))
	ledger := types.ObservationLedger{Records: result.Observations}
	text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	if !strings.Contains(text, rows[0].FromIdentity) || !strings.Contains(text, "另省略 0") {
		t.Fatalf("missing authoring receipt: %s", text)
	}
	for _, row := range rows {
		if row.FromLabel == "AsyncPrefetch" || row.ToLabel == "AsyncPrefetch" {
			t.Fatal("async promoted to synchronous tree")
		}
	}
	after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	if string(before) != string(after) {
		t.Fatal("display provider mutated causal projection")
	}
	// The same marker name on the background thread owns no child relation.
	for _, r := range result.Observations {
		if f, ok := DecodeTraceBusinessTreeFact(r); ok && f.Node.Thread.PID == 900 {
			id := runtimeBusinessInstanceKey(r, f, f.Node.ID)
			for _, row := range rows {
				if row.FromIdentity == id || row.ToIdentity == id {
					t.Fatal("same-name cross-thread join")
				}
			}
		}
	}
	// Different receipts cannot close a missing parent by coincidental ID/name.
	for _, field := range []string{"query", "payload", "source"} {
		t.Run(field, func(t *testing.T) {
			copyRows := append([]types.ObservationRecord(nil), result.Observations...)
			for i := range copyRows {
				f, ok := DecodeTraceBusinessTreeFact(copyRows[i])
				if !ok || f.Node.Name != "LoadIndex" {
					continue
				}
				switch field {
				case "query":
					copyRows[i].SourceRef.QueryScopeID += "x"
				case "payload":
					copyRows[i].SourceRef.PayloadRef += "x"
				case "source":
					copyRows[i].SourceRef.Path += "x"
				}
			}
			if got := RuntimeDiagramRelations(types.ObservationLedger{Records: copyRows}, nil); len(got) != 0 {
				t.Fatalf("cross-%s join: %+v", field, got)
			}
		})
	}
}

func TestRuntimeBusinessContainmentRecipeDistinguishesRealSameNameThreadsAndWindows(t *testing.T) {
	ctx, path := businessRefTestContext(t, strings.Join([]string{
		"worker-700 (700) [003] .... 1.000000: tracing_mark_write: B|700|Work",
		"worker-900 (900) [004] .... 1.001000: tracing_mark_write: B|900|Work",
		"worker-700 (700) [003] .... 1.002000: tracing_mark_write: B|700|Phase",
		"worker-900 (900) [004] .... 1.003000: tracing_mark_write: B|900|Phase",
		"worker-700 (700) [003] .... 1.004000: tracing_mark_write: E|700",
		"worker-900 (900) [004] .... 1.005000: tracing_mark_write: E|900",
		"worker-700 (700) [003] .... 1.008000: tracing_mark_write: E|700",
		"worker-900 (900) [004] .... 1.009000: tracing_mark_write: E|900",
	}, "\n")+"\n")
	physicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	var ledger types.ObservationLedger
	for _, window := range [][2]float64{{1, 1.010}, {1.0025, 1.009}} {
		result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": window[0], "time_end": window[1]})
		if !result.Success {
			t.Fatal(result.Summary)
		}
		ledger.Records = append(ledger.Records, result.Observations...)
	}
	rows := RuntimeDiagramRelations(ledger, nil)
	if len(rows) != 4 {
		t.Fatalf("two same-name threads × two queries must remain four relations: %+v", rows)
	}
	recipe := RenderRuntimeDiagramRelationRecipes(ledger, nil)
	scopes := map[string]bool{}
	for _, row := range rows {
		if row.FromLabel != "Work" || row.ToLabel != "Phase" {
			t.Fatalf("unexpected fixture relationship: %+v", row)
		}
		if !strings.Contains(row.ScopeLabel, "物理源="+fmt.Sprintf("%q", physicalPath)) || !strings.Contains(row.ScopeLabel, "所选窗口=") || !strings.Contains(recipe, row.ScopeLabel) {
			t.Fatalf("recipe cannot select source/window: %+v", row)
		}
		wantParent, wantChild := "原始文件第1行", "原始文件第3行"
		if strings.Contains(row.ScopeLabel, `线程="worker-900"`) {
			wantParent, wantChild = "原始文件第2行", "原始文件第4行"
		} else if !strings.Contains(row.ScopeLabel, `线程="worker-700"`) {
			t.Fatalf("missing exact emitting thread: %+v", row)
		}
		if !strings.Contains(row.ScopeLabel, "父起点="+wantParent) || !strings.Contains(row.ScopeLabel, "子起点="+wantChild) {
			t.Fatalf("parent/child starts confused: %+v", row)
		}
		if scopes[row.ScopeLabel] {
			t.Fatalf("same-name instance recipe is ambiguous: %s", row.ScopeLabel)
		}
		scopes[row.ScopeLabel] = true
		original := row
		row.ScopeLabel = "reader text is not authority"
		if !runtimeDiagramRelationProved([]RuntimeDiagramRelation{row}, original.FromIdentity, original.ToIdentity, original.Kind) {
			t.Fatal("reader guidance became relation authority")
		}
	}
	for _, window := range []string{"所选窗口=1.000000..1.010000 秒", "所选窗口=1.002500..1.009000 秒"} {
		if !strings.Contains(recipe, window) {
			t.Fatalf("query window omitted: %s", recipe)
		}
	}
}

func TestRuntimeBusinessContainmentRecipeStartUsesPhysicalSupportCoordinates(t *testing.T) {
	record := types.ObservationRecord{SupportRefs: []string{"/capture/physical.systrace:2-9"}}
	fact := TraceBusinessTreeFact{IndexPath: "/capture/bundle.index"}
	fact.Node.SourcePath = "/capture/physical.systrace"
	fact.Node.StartLine = 102
	if got := runtimeBusinessStartLabel(record, fact); got != "原始文件第2行" {
		t.Fatalf("virtual start leaked as physical line: %s", got)
	}
	record.SupportRefs = nil
	if got := runtimeBusinessStartLabel(record, fact); !strings.Contains(got, "查询索引") || !strings.Contains(got, "102") || strings.Contains(got, "原始文件") {
		t.Fatalf("unknown mapping invented a physical line: %s", got)
	}
}
