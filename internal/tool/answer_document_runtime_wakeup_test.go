package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeWakeFixture(t *testing.T) (*types.BusContext, types.ToolResult, []RuntimeDiagramRelation) {
	t.Helper()
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_summary/events.systrace")
	start, end := 10.001, 10.012
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Mutable: types.NewMutableState("sleep"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Source: "user_explicit"}},
			RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactCountOrDuration}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "pid": 41, "time_start": start, "time_end": end, "min_duration_ms": 0.1})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 2 {
		t.Fatalf("expected both direct events, got %+v", rows)
	}
	return ctx, result, rows
}

func TestRuntimeWakeupPublicDiagramKinds(t *testing.T) {
	for _, kind := range []types.DiagramKind{types.DiagramFlow, types.DiagramSequence, types.DiagramArchitecture, types.DiagramCallDAG} {
		t.Run(string(kind), func(t *testing.T) {
			ctx, _, rows := runtimeWakeFixture(t)
			doc := runtimeNestingDoc(rows[0], true)
			doc.Blocks[1].Diagram.Kind = kind
			if kind == types.DiagramSequence {
				r := rows[0]
				doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant " + r.FromNode + " as " + r.FromLabel + "\n participant " + r.ToNode + " as " + r.ToLabel + "\n " + r.FromNode + "->>" + r.ToNode + ": 唤醒\n"
			} else {
				doc.Blocks[1].Diagram.Body = strings.ReplaceAll(doc.Blocks[1].Diagram.Body, "包含", "唤醒")
			}
			doc.Blocks[1].EdgeAnchors[0].VisibleLabel = "唤醒"
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || !res.Success {
				t.Fatalf("runtime event rejected: %v %+v", err, res)
			}
			if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) > 0 {
				t.Fatalf("post gate disagrees: %+v", got)
			}
			if got := DiagramCallEdgeEvidenceMismatches(doc, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) == 0 {
				t.Fatal("context-free gate trusted a runtime enum")
			}
		})
	}
}

func TestRuntimeWakeupPublicRejectAndLocalRepair(t *testing.T) {
	ctx, _, rows := runtimeWakeFixture(t)
	doc := runtimeNestingDoc(rows[0], false)
	raw, _ := json.Marshal(doc)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || res.Success || res.Repair == nil {
		t.Fatalf("unanchored event accepted: %v %+v", err, res)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatal(err)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(ctx.Mutable.LastRejectedAnswerDocumentV2(), delta.Failures, delta.AllowedAdditions)
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	for _, f := range lease.Failures {
		for _, c := range lease.AllowedAdditions {
			if c.RelationKind != types.DiagramRelWakeup || !types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c) {
				continue
			}
			raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{{Action: "attach", FailureRef: f.FailureRef, AdditionRef: c.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: f.FromNode, ToNode: f.ToNode, VisibleLabel: "唤醒"}}}})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			if err != nil || !res.Success {
				t.Fatalf("local wakeup repair: %v %+v", err, res)
			}
			return
		}
	}
	t.Fatal("real rejection did not expose executable wakeup repair")
}

func TestRuntimeWakeupAuthorityNegativeMatrix(t *testing.T) {
	for _, name := range []string{"reverse", "call", "temporal", "transitive", "same_name", "wrong_instance", "mixed_source_call", "repeated_pair_forged_instance", "other_window", "other_target", "negative", "census_only", "changed_event", "duplicate_carrier", "conflicting_event"} {
		t.Run(name, func(t *testing.T) {
			ctx, result, rows := runtimeWakeFixture(t)
			doc := runtimeNestingDoc(rows[0], true)
			a := &doc.Blocks[1].EdgeAnchors[0]
			switch name {
			case "reverse":
				a.FromIdentity, a.ToIdentity = a.ToIdentity, a.FromIdentity
			case "call":
				a.RelationKind = types.DiagramRelCall
			case "temporal":
				a.RelationKind = types.DiagramRelTemporal
			case "transitive":
				a.ToIdentity = rows[1].ToIdentity
			case "same_name":
				a.FromIdentity, a.ToIdentity = rows[0].FromLabel, rows[0].ToLabel
			case "wrong_instance":
				a.FromIdentity, a.ToIdentity = rows[1].FromIdentity, rows[1].ToIdentity
			case "repeated_pair_forged_instance":
				doc.Blocks[1].Diagram.Body += " A[loader] -->|唤醒| B[target]\n A -->|唤醒| B\n"
				valid := *a
				valid.FromNode, valid.ToNode = "A", "B"
				forged := valid
				forged.ToIdentity += ":not-this-event"
				doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, valid, forged)
			case "mixed_source_call":
				doc.Blocks[1].Diagram.Body += " A[caller] -->|调用| B[callee]\n"
				doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "caller", ToIdentity: "callee", RelationKind: types.DiagramRelCall, VisibleLabel: "调用"})
			default:
				var conflictRows []types.ObservationRecord
				for i := range result.Observations {
					r := &result.Observations[i]
					if r.Predicate != "wakeup_chain_edge" {
						continue
					}
					switch name {
					case "other_window":
						r.SourceRef.QueryWindowStartTs = 9
					case "other_target":
						r.SourceRef.QueryTargetPID = 999
					case "negative":
						r.Negative = true
					case "census_only":
						r.Predicate = "wakeup_edge_census"
					case "changed_event":
						r.Span.StartTs += .0001
					case "duplicate_carrier":
						r.RichNotes = append(r.RichNotes, r.RichNotes[len(r.RichNotes)-1])
					case "conflicting_event":
						copyR := *r
						copyR.ID += ":conflict"
						copyR.RichNotes = append([]string(nil), r.RichNotes...)
						f, _ := decodeRuntimeWakeupEvent(copyR)
						f.Timestamp += .000001
						copyR.Span.StartTs, copyR.Span.EndTs = f.Timestamp, f.Timestamp
						b, _ := json.Marshal(f)
						for j, n := range copyR.RichNotes {
							if strings.HasPrefix(n, traceWakeupEventNote) {
								copyR.RichNotes[j] = traceWakeupEventNote + string(b)
							}
						}
						conflictRows = append(conflictRows, copyR)
					}
				}
				result.Observations = append(result.Observations, conflictRows...)
				ctx.Mutable = types.NewMutableState("negative")
				ctx.Mutable.AppendDispatchToolResult(result)
			}
			raw, _ := json.Marshal(doc)
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success {
				t.Fatalf("%s accepted: %v %+v", name, err, res)
			}
		})
	}
}
