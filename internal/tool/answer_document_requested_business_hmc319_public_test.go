package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const hmc319BusinessQuestion = "只分析附加 trace 的 5.000000 秒到 5.012000 秒，不查源码。请列出 business-700 的同步业务层级，分别给每层包含子层的总耗时、自身耗时，以及自身区间内的运行、等待调度和睡眠时间。用清楚的层级图或树说明 OpenDocument、LoadIndex、DecodeRecord 的关系；自身耗时可能分成几段，应说明如何计量避免重复。另一个线程上同名 OpenDocument 和 AsyncPrefetch 应如何理解？如果某个片段没有调度状态记录，能否说它没有占用 CPU？请保留证据范围，不把业务嵌套直接当作响应根因或唤醒依赖。"

func hmc319BusinessFixture(t *testing.T) (*types.BusContext, types.ToolResult, []RuntimeDiagramRelation) {
	return hmc319BusinessFixtureTrace(t, nil)
}

func hmc319BusinessFixtureTrace(t *testing.T, change func(string) string) (*types.BusContext, types.ToolResult, []RuntimeDiagramRelation) {
	t.Helper()
	trace, err := os.ReadFile("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		trace = []byte(change(string(trace)))
	}
	root := t.TempDir()
	path := filepath.Join(root, "events.systrace")
	if err := os.WriteFile(path, trace, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: root, Language: "zh", AttachedHitrace: string(trace), Mutable: types.NewMutableState(hmc319BusinessQuestion)}
	payload := b1793TargetRosterPayload(t, true)
	payload["intent"], payload["question_kind"], payload["predicate_axis"] = "explain", "mechanism", "flow"
	payload["predicates"].(map[string]any)["is_relational_lookup"] = false
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .95, "dimensions": []any{map[string]any{"index": 1, "label": "层级图或树", "required": true, "role": "diagram", "source_quote": "用清楚的层级图或树说明 OpenDocument、LoadIndex、DecodeRecord 的关系"}}}
	payload["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "fact_families": []string{"count_or_duration", "target_scheduler_state", "target_wait_occurrences"}, "frame_causality_requested": false, "runtime_work_relation_requested": false, "confidence": .95}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 5.0, "time_end": 5.012, "source_quote": "只分析附加 trace 的 5.000000 秒到 5.012000 秒", "confidence": .95}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "列出 business-700 的同步业务层级", "confidence": .95}
	payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 700, "business-700")}
	participants := []any{}
	for _, name := range []string{"OpenDocument", "LoadIndex", "DecodeRecord"} {
		participants = append(participants, map[string]any{"identity": name, "source_quote": name, "role": "incident_required"})
	}
	payload["diagram_hint"] = map[string]any{"required": true, "kind": "flow", "relation_scope_quote": "用清楚的层级图或树说明 OpenDocument、LoadIndex、DecodeRecord 的关系", "participants": participants}
	raw, _ := json.Marshal(payload)
	analysis, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !analysis.Success {
		t.Fatalf("public analysis: %v %s", err, analysis.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || rm.DiagramHint == nil || len(rm.DiagramHint.Participants) != 3 || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 700 {
		t.Fatalf("accepted analysis lost the original typed request: %+v", rm)
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, RequiredKind: rm.DiagramHint.Kind, Participants: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5.0, "time_end": 5.012})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 {
		t.Fatalf("real query did not retain the two synchronous parent relations: %+v", rows)
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || len(diagramIncidentParticipantObligations(view)) != 3 {
		t.Fatalf("fixture must exercise all three accepted diagram obligations: plan=%+v obligations=%+v", view.DiagramPlan, view.DiagramParticipantObligations)
	}
	return ctx, result, rows
}

func hmc319BusinessDocument(rows []RuntimeDiagramRelation) *types.AnswerDocumentV2 {
	doc := runtimeNestingDoc(rows[0], false)
	doc.Blocks[0].Text = "所选线程的同步打点包含关系来自精确实例；同名的另一线程独立，未知调度状态不是零，不证明调用、唤醒或响应根因。"
	doc.Blocks[0].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimNoConclusion
	var body strings.Builder
	body.WriteString("flowchart TD\n")
	seen := map[string]bool{}
	for _, row := range rows {
		for _, endpoint := range [][2]string{{row.FromNode, row.FromLabel}, {row.ToNode, row.ToLabel}} {
			if !seen[endpoint[0]] {
				fmt.Fprintf(&body, " %s[%q]\n", endpoint[0], endpoint[1])
				seen[endpoint[0]] = true
			}
		}
		fmt.Fprintf(&body, " %s -->|包含| %s\n", row.FromNode, row.ToNode)
		doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "包含"})
	}
	doc.Blocks[1].Diagram.Body = body.String()
	return doc
}

func TestHMC319RequestedBusinessPublicExactContainment(t *testing.T) {
	ctx, _, rows := hmc319BusinessFixture(t)
	doc := hmc319BusinessDocument(rows)
	raw, _ := json.Marshal(doc)
	result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("current exact business instances and both verified containment edges must fulfill the requested tree without a contradictory unproven boundary: %v %s", err, result.Summary)
	}
	if mismatches := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(mismatches) != 0 {
		t.Fatalf("post gate disagrees with the accepted exact containment: %+v", mismatches)
	}
}

func TestHMC319RequestedBusinessPublicLocalAttach(t *testing.T) {
	ctx, _, rows := hmc319BusinessFixture(t)
	doc := hmc319BusinessDocument(rows)
	doc.Blocks[1].EdgeAnchors = doc.Blocks[1].EdgeAnchors[1:]
	lease := requestedThreadRejectLeaseHMC242(t, ctx, doc)
	var edit emitAnswerDiagramEdgeEdit
	for _, failure := range lease.Failures {
		for _, candidate := range lease.AllowedAdditions {
			if candidate.FromIdentity == rows[0].FromIdentity && candidate.ToIdentity == rows[0].ToIdentity && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				edit = emitAnswerDiagramEdgeEdit{Action: "attach", FailureRef: failure.FailureRef, AdditionRef: candidate.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: failure.FromNode, ToNode: failure.ToNode, VisibleLabel: "包含"}}
			}
		}
	}
	if edit.AdditionRef == "" {
		t.Fatalf("current public rejection did not offer the exact omitted containment: %+v", lease)
	}
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{edit}})
	result, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("same current business binding must survive local attach: %v %s", err, result.Summary)
	}
	after := ctx.Mutable.AnswerDocumentV2()
	if len(after.Blocks[1].EdgeAnchors) != 2 || !reflect.DeepEqual(after.Blocks[0], doc.Blocks[0]) || !reflect.DeepEqual(after.Blocks[2], doc.Blocks[2]) || after.Blocks[1].Diagram.Body != doc.Blocks[1].Diagram.Body {
		t.Fatal("atomic attach changed non-target content or lost exact native edges")
	}
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, after, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
		t.Fatalf("post gate: %+v", got)
	}
}

func hmc319HasBusinessBinding(rows []RuntimeDiagramRelation, name string) bool {
	for _, row := range rows {
		for _, bound := range append(append([]string(nil), row.requestedFrom...), row.requestedTo...) {
			if name == "" || name == bound {
				return true
			}
		}
	}
	return false
}

func TestHMC319RequestedBusinessCurrentQueryGuard(t *testing.T) {
	for _, mode := range []string{"ledger_json", "result_json", "changed_row", "cropped_isolated_root", "source_generation", "no_target", "cursor_target", "multiple_targets", "wrong_thread", "outside_window", "partial_topology", "omitted_nodes"} {
		t.Run(mode, func(t *testing.T) {
			ctx, result, _ := hmc319BusinessFixture(t)
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			rm := ctx.AnalysisIR.RequestModel
			if !hmc319HasBusinessBinding(RuntimeDiagramRelations(ledger, &rm), "OpenDocument") {
				t.Fatal("positive current complete inventory prerequisite")
			}
			switch mode {
			case "ledger_json":
				raw, _ := json.Marshal(ledger)
				ledger = types.ObservationLedger{}
				if err := json.Unmarshal(raw, &ledger); err != nil {
					t.Fatal(err)
				}
			case "result_json":
				raw, _ := json.Marshal(result)
				result = types.ToolResult{}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				ctx.Mutable = types.NewMutableState(hmc319BusinessQuestion)
				ctx.Mutable.AppendDispatchToolResult(result)
				ledger = types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
			case "changed_row":
				for i := range ledger.Records {
					if ledger.Records[i].Predicate == types.TraceBusinessTreePredicate {
						ledger.Records[i].Summary += " altered"
						break
					}
				}
			case "cropped_isolated_root":
				var kept []types.ObservationRecord
				for _, row := range ledger.Records {
					fact, ok := DecodeTraceBusinessTreeFact(row)
					if !ok || fact.Node.Thread.PID != 900 {
						kept = append(kept, row)
					}
				}
				ledger.Records = kept
			case "source_generation":
				path := filepath.Join(ctx.RepoRoot, "events.systrace")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, []byte("# next capture\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "no_target":
				rm.RuntimeTargets = nil
			case "cursor_target":
				rm.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
			case "multiple_targets":
				rm.RuntimeTargets = append(rm.RuntimeTargets, types.RuntimeTarget{Kind: types.RuntimeTargetKindThread, PID: 900, Source: "user_explicit"})
			case "wrong_thread":
				rm.RuntimeTargets[0].PID = 900
				rm.RuntimeTargetProfile.SourceQuote = "background-900"
			case "outside_window":
				end := 5.007
				rm.RuntimeArtifactScopeProfile.TimeEnd = &end
			case "partial_topology", "omitted_nodes":
				for i, row := range ledger.Records {
					fact, ok := DecodeTraceBusinessTreeFact(row)
					if !ok {
						continue
					}
					if mode == "partial_topology" {
						fact.Coverage = "partial_topology"
					} else {
						fact.NodeCount++
						fact.OmittedNodes++
					}
					raw, _ := json.Marshal(fact)
					ledger.Records[i].RichNotes = []string{types.TraceNoteKeyBusinessTreeNode + "=" + string(raw)}
				}
			}
			if rows := RuntimeDiagramRelations(ledger, &rm); hmc319HasBusinessBinding(rows, "") {
				t.Fatalf("%s borrowed requested participant identity: %+v", mode, rows)
			}
		})
	}
}

func TestHMC319RequestedBusinessIsolatedDuplicateIsAmbiguous(t *testing.T) {
	ctx, _, _ := hmc319BusinessFixtureTrace(t, func(trace string) string {
		return strings.Replace(trace, "business-700 (600) [003] .... 5.012000:", "business-700 (600) [003] .... 5.011100: tracing_mark_write: B|600|OpenDocument\nbusiness-700 (600) [003] .... 5.011500: tracing_mark_write: E|600\nbusiness-700 (600) [003] .... 5.012000:", 1)
	})
	rows := runtimeDiagramRelationsForContext(ctx)
	if hmc319HasBusinessBinding(rows, "OpenDocument") {
		t.Fatal("isolated same-name root must prevent choosing one of two distinct physical instances")
	}
	if !hmc319HasBusinessBinding(rows, "LoadIndex") {
		t.Fatal("unrelated exact singleton instance lost its own independent identity")
	}
}

func TestHMC319RequestedBusinessSeparateSourcesNeverAlias(t *testing.T) {
	ctx, first, _ := hmc319BusinessFixture(t)
	other, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ctx.RepoRoot, "independent.systrace")
	if err := os.WriteFile(path, other, 0600); err != nil {
		t.Fatal(err)
	}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5.0, "time_end": 5.012})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, result}})
	if rows := runtimeDiagramRelationsForContext(ctx); hmc319HasBusinessBinding(rows, "") {
		t.Fatal("same bytes/name/thread in another original source cannot own the same requested participant")
	}
}

func TestHMC319RequestedBusinessProducerSealCannotBeResigned(t *testing.T) {
	for _, mode := range []string{"changed_row", "changed_query", "changed_target", "changed_window", "foreign_run", "memo", "same_seal"} {
		t.Run(mode, func(t *testing.T) {
			ctx, result, _ := hmc319BusinessFixture(t)
			switch mode {
			case "changed_row":
				for i := range result.Observations {
					if result.Observations[i].Predicate == types.TraceBusinessTreePredicate {
						result.Observations[i].Summary += " altered before compiler"
					}
				}
			case "changed_query":
				for i := range result.Observations {
					result.Observations[i].SourceRef.QueryScopeID += ":invented"
				}
			case "changed_target":
				for i := range result.Observations {
					result.Observations[i].SourceRef.QueryTargetPID = 900
				}
			case "changed_window":
				for i := range result.Observations {
					result.Observations[i].SourceRef.QueryWindowEndTs = 6
				}
			case "memo":
				result.ReusedFromRunMemo = true
			case "foreign_run":
				ctx.Mutable = types.NewMutableState(hmc319BusinessQuestion)
			}
			// Calling the producer hook again cannot repair public-row mutation.
			ctx.Mutable.StampRuntimeCurrentQueryPopulation(&result)
			ctx.ToolResults = []types.ToolResult{result}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{})
			input := types.ObservationLedgerInputFromBusContext(ctx, 0)
			input.ToolResults = []types.ToolResult{result}
			input.SystemTraceSupplementResults = nil
			ledger := types.CompileObservationLedger(input)
			got := hmc319HasBusinessBinding(RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel), "")
			if got != (mode == "same_seal") {
				t.Fatalf("%s producer census current=%v", mode, got)
			}
		})
	}
}

func TestHMC319RequestedBusinessDistinctQueryPublicationsRemainIndependent(t *testing.T) {
	ctx, first, _ := hmc319BusinessFixture(t)
	// Different actual window publications have distinct raw payload identities;
	// changing only an unused display limit can reuse identical payload bytes.
	second := businessRefTestQuery(t, ctx, map[string]any{"path": filepath.Join(ctx.RepoRoot, "events.systrace"), "view": "window_stats", "pid": 700, "time_start": 5.0, "time_end": 5.011})
	ctx.Mutable.AppendDispatchToolResult(second)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{first, second}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if !hmc319HasBusinessBinding(rows, "OpenDocument") {
		t.Fatal("another complete query of the same physical instance must not revoke a legitimate exact binding")
	}
	for _, row := range rows {
		if row.Kind != types.DiagramRelContain {
			t.Fatal("business binding promoted relation kind")
		}
	}
	if first.Observations[0].SourceRef.QueryScopeID == second.Observations[0].SourceRef.QueryScopeID {
		t.Fatal("test must exercise independent query publications")
	}
	if len(rows) != 4 {
		for i, result := range []types.ToolResult{first, second} {
			for _, record := range result.Observations {
				if fact, ok := DecodeTraceBusinessTreeFact(record); ok {
					t.Logf("publication %d: scope=%s instance=%s name=%s nodes=%d omitted=%d", i, record.SourceRef.QueryScopeID, fact.Node.ID, fact.Node.Name, fact.NodeCount, fact.OmittedNodes)
				}
			}
		}
		t.Fatalf("independent query edge identities were merged: %d", len(rows))
	}
}

func TestHMC319RequestedBusinessRealPartialTopologyRetainsEdgesWithoutBinding(t *testing.T) {
	ctx, _, rows := hmc319BusinessFixtureTrace(t, func(trace string) string {
		return strings.Replace(trace, "business-700 (600) [003] .... 5.011000: tracing_mark_write: E|600\n", "", 1)
	})
	if len(rows) != 2 {
		t.Fatal("observed parent edges must remain factual despite an open parent")
	}
	if hmc319HasBusinessBinding(runtimeDiagramRelationsForContext(ctx), "") {
		t.Fatal("partial topology cannot prove unique requested business instances")
	}
}

func TestHMC319RequestedBusinessSharedAcceptedQuote(t *testing.T) {
	ctx, _, _ := hmc319BusinessFixture(t)
	for i := range ctx.AnalysisIR.RequestModel.DiagramHint.Participants {
		ctx.AnalysisIR.RequestModel.DiagramHint.Participants[i].SourceQuote = "OpenDocument、LoadIndex、DecodeRecord"
	}
	rows := runtimeDiagramRelationsForContext(ctx)
	for _, name := range []string{"OpenDocument", "LoadIndex", "DecodeRecord"} {
		if !hmc319HasBusinessBinding(rows, name) {
			t.Fatalf("co-listed exact business instance %s lost its independently selected identity", name)
		}
	}
}
