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

const hmc242ParticipantQuote = "target 线程（tid 42）"

func requestedThreadFixtureHMC242(t *testing.T) (*types.BusContext, []RuntimeDiagramRelation) {
	return requestedThreadFixtureCustomHMC242(t, "", nil)
}

func requestedThreadFixtureCustomHMC242(t *testing.T, question string, customize func(map[string]any)) (*types.BusContext, []RuntimeDiagramRelation) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_event_participants/wakeups.systrace")
	if err != nil {
		t.Fatal(err)
	}
	trace, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if question == "" {
		question = "看看附件中 1.000 到 1.020 秒 target 线程（tid 42）被谁唤醒了。逐次列出时间、双方线程和能确认的进程信息，再画一张唤醒时序图。"
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AttachedHitrace: string(trace), Mutable: types.NewMutableState(question)}
	payload := b1793TargetRosterPayload(t, true)
	payload["intent"], payload["question_kind"], payload["predicate_axis"] = "trace", "call_chain", "call"
	payload["predicates"].(map[string]any)["is_relational_lookup"] = true
	payload["runtime_question_profile"] = map[string]any{"scope": "relation_analysis", "frame_causality_requested": false, "runtime_work_relation_requested": false, "confidence": .95}
	payload["requested_answer_dimensions"] = map[string]any{"is_dimensioned_answer": true, "confidence": .95, "dimensions": []any{map[string]any{"index": 1, "label": "唤醒时序图", "required": true, "role": "diagram", "source_quote": "画一张唤醒时序图"}}}
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 1.0, "time_end": 1.02, "source_quote": "1.000 到 1.020 秒", "confidence": .95}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": hmc242ParticipantQuote, "confidence": .95}
	payload["runtime_targets"] = []any{runtimeTargetProvenanceRowHMC237("thread", 42, "target-42")}
	payload["diagram_hint"] = map[string]any{"required": true, "kind": "sequence", "relation_scope_quote": hmc242ParticipantQuote + "被谁唤醒了", "participants": []any{map[string]any{"identity": hmc242ParticipantQuote, "source_quote": hmc242ParticipantQuote, "role": "incident_required"}}}
	if customize != nil {
		customize(payload)
	}
	wantParticipants := len(payload["diagram_hint"].(map[string]any)["participants"].([]any))
	raw, _ := json.Marshal(payload)
	analysis, err := (&EmitAnalysis{}).Execute(ctx, raw)
	if err != nil || !analysis.Success {
		t.Fatalf("public analysis: %v %s", err, analysis.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil || rm.DiagramHint == nil || len(rm.DiagramHint.Participants) != wantParticipants {
		t.Fatalf("analysis lost participant: %+v", rm)
	}
	// The pipeline derives this presentation contract from the accepted hint.
	// No participant identity/selector is injected outside EmitAnalysis.
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm, AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: rm.DiagramHint.Required, RequiredKind: rm.DiagramHint.Kind, Participants: append([]types.DiagramParticipantHint(nil), rm.DiagramHint.Participants...)}}}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "time_start": 1.0, "time_end": 1.02})
	ctx.Mutable.AppendDispatchToolResult(result)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	rows := runtimeDiagramRelationsForContext(ctx)
	if len(rows) != 3 {
		t.Fatalf("want all three exact target events, got %+v", rows)
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || len(view.DiagramParticipantObligations) == 0 {
		t.Fatalf("fixture did not activate the accepted diagram obligation: hint=%+v contract=%+v diagram=%+v obligations=%+v", rm.DiagramHint, ctx.AnalysisIR.AnswerContract.Diagram, view.DiagramPlan, view.DiagramParticipantObligations)
	}
	return ctx, rows
}

func requestedThreadDocHMC242(rows []RuntimeDiagramRelation, anchored bool) *types.AnswerDocumentV2 {
	doc := runtimeNestingDoc(rows[0], false)
	doc.Blocks[0].Text = "该窗口记录三次唤醒；线程身份来自同一份原生事件，不表示唤醒者造成全部等待。"
	doc.Blocks[0].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimNoConclusion
	doc.Blocks[1].Diagram.Kind = types.DiagramSequence
	doc.Blocks[1].FacetIDs = []string{"diagram_spine", "current_code_path"}
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	seen := map[string]bool{}
	for _, row := range rows {
		for _, endpoint := range [][2]string{{row.FromNode, row.FromLabel}, {row.ToNode, row.ToLabel}} {
			if !seen[endpoint[0]] {
				fmt.Fprintf(&b, " participant %s as %s\n", endpoint[0], endpoint[1])
				seen[endpoint[0]] = true
			}
		}
	}
	for _, row := range rows {
		fmt.Fprintf(&b, " %s->>%s: 唤醒\n", row.FromNode, row.ToNode)
		if anchored {
			doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "唤醒"})
		}
	}
	doc.Blocks[1].Diagram.Body = b.String()
	return doc
}

func TestHMC242RequestedThreadPublicOrderedDiagram(t *testing.T) {
	ctx, rows := requestedThreadFixtureHMC242(t)
	doc := requestedThreadDocHMC242(rows, true)
	raw, _ := json.Marshal(doc)
	out, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !out.Success {
		t.Fatalf("public native diagram cannot fulfill exact selected request participant: %v %s", err, out.Summary)
	}
	if got := DiagramParticipantCoverageMismatchesWithRuntimeContext(ctx, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
		t.Fatalf("post gate disagrees: %+v", got)
	}
}

func TestHMC242RequestedThreadPublicSharedQuoteRemainsUnbound(t *testing.T) {
	for _, mode := range []string{"same_quote", "context_actor", "co_listed_other_quote", "different_quote"} {
		t.Run(mode, func(t *testing.T) {
			quote := hmc242ParticipantQuote + "和 observer"
			question := "看看附件中 1.000 到 1.020 秒 " + quote + "被谁唤醒了，再画一张唤醒时序图。"
			ctx, rows := requestedThreadFixtureCustomHMC242(t, question, func(payload map[string]any) {
				payload["runtime_target_profile"].(map[string]any)["source_quote"] = quote
				diagram := payload["diagram_hint"].(map[string]any)
				diagram["relation_scope_quote"] = quote + "被谁唤醒了"
				otherQuote, otherRole := quote, "incident_required"
				if mode == "context_actor" {
					otherRole = "context_only"
				}
				if mode == "co_listed_other_quote" {
					otherQuote = "observer"
				}
				diagram["participants"] = []any{
					map[string]any{"identity": "target", "source_quote": quote, "role": "incident_required"},
					map[string]any{"identity": "observer", "source_quote": otherQuote, "role": otherRole},
				}
				if mode == "different_quote" {
					diagram["participants"] = []any{map[string]any{"identity": "target", "source_quote": "target", "role": "incident_required"}}
					diagram["relation_scope_quote"] = "被谁唤醒了"
				}
			})
			for _, row := range rows {
				if len(row.requestedFrom)+len(row.requestedTo) != 0 {
					t.Fatalf("ambiguous quote borrowed native identity: %+v", row)
				}
			}
			raw, _ := json.Marshal(requestedThreadDocHMC242(rows, true))
			result, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || result.Success || !strings.Contains(result.Summary, "missing_unproven_boundary") {
				t.Fatalf("unproved participant mapping acquired coverage: %v %s", err, result.Summary)
			}
		})
	}
}

func TestHMC242RequestedThreadShortIdentityWholeSurfaceOnly(t *testing.T) {
	ctx, _ := requestedThreadFixtureCustomHMC242(t, "", func(payload map[string]any) {
		payload["diagram_hint"].(map[string]any)["participants"].([]any)[0].(map[string]any)["identity"] = "target"
	})
	rm := &ctx.AnalysisIR.RequestModel
	p := rm.DiagramHint.Participants[0]
	if id, ok := requestedRuntimeParticipantThread(rm, p); !ok || id != 42 {
		t.Fatalf("accepted short identity lost exact target: %d %t", id, ok)
	}
	for _, identity := range []string{"tar", "retarget", "business_owner", "target-42"} {
		changed := p
		changed.Identity = identity
		if _, ok := requestedRuntimeParticipantThread(rm, changed); ok {
			t.Fatalf("unquoted/substring identity %q was bound", identity)
		}
	}
}

func requestedThreadRejectLeaseHMC242(t *testing.T, ctx *types.BusContext, doc *types.AnswerDocumentV2) *types.AnswerDiagramRelationRepairLease {
	t.Helper()
	raw, _ := json.Marshal(doc)
	out, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || out.Success || out.Repair == nil {
		t.Fatalf("expected public repair: %v %+v", err, out)
	}
	var delta types.AnswerDiagramRelationRepairDelta
	if err := json.Unmarshal([]byte(out.Repair.Metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON]), &delta); err != nil {
		t.Fatalf("repair delta: %v %s", err, out.Summary)
	}
	lease := types.NewAnswerDiagramRelationRepairLease(ctx.Mutable.LastRejectedAnswerDocumentV2(), delta.Failures, delta.AllowedAdditions)
	if lease == nil {
		t.Fatalf("missing lease: %+v", delta)
	}
	ctx.Mutable.SetAnswerDiagramRelationRepairLease(lease)
	return lease
}

func TestHMC242RequestedThreadPublicLocalAttach(t *testing.T) {
	ctx, rows := requestedThreadFixtureHMC242(t)
	doc := requestedThreadDocHMC242(rows, true)
	doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors[:1], doc.Blocks[1].EdgeAnchors[2:]...)
	lease := requestedThreadRejectLeaseHMC242(t, ctx, doc)
	var edit emitAnswerDiagramEdgeEdit
	for _, f := range lease.Failures {
		for _, c := range lease.AllowedAdditions {
			if c.FromIdentity == rows[1].FromIdentity && c.ToIdentity == rows[1].ToIdentity && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c) {
				edit = emitAnswerDiagramEdgeEdit{Action: "attach", FailureRef: f.FailureRef, AdditionRef: c.AdditionRef, Edge: &types.DiagramEdgeAnchor{FromNode: f.FromNode, ToNode: f.ToNode, VisibleLabel: "唤醒"}}
			}
		}
	}
	if edit.FailureRef == "" {
		t.Fatalf("no executable native attach: %+v", lease)
	}
	ac := &types.AgentContext{AnalysisIR: ctx.AnalysisIR, Mutable: ctx.Mutable, RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, AttachedHitrace: ctx.AttachedHitrace}
	schema := string((&EmitAnswerDocumentPatch{}).ParametersFor(ac))
	if !strings.Contains(schema, edit.FailureRef) || !strings.Contains(schema, edit.AdditionRef) {
		t.Fatal("actual current schema lost selected refs")
	}
	raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{edit}})
	out, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !out.Success {
		t.Fatalf("public local repair: %v %s", err, out.Summary)
	}
	after := ctx.Mutable.AnswerDocumentV2()
	if len(after.Blocks[1].EdgeAnchors) != 3 || !reflect.DeepEqual(doc.Blocks[0], after.Blocks[0]) || !reflect.DeepEqual(doc.Blocks[2], after.Blocks[2]) {
		t.Fatal("local repair lost native events or changed unrelated blocks")
	}
}

func TestHMC242RequestedThreadPublicOrderRejectAndRepair(t *testing.T) {
	ctx, rows := requestedThreadFixtureHMC242(t)
	wrong := []RuntimeDiagramRelation{rows[0], rows[2], rows[1]}
	doc := requestedThreadDocHMC242(wrong, true)
	lease := requestedThreadRejectLeaseHMC242(t, ctx, doc)
	var failure types.AnswerDiagramRelationRepairFailure
	var candidate types.AnswerDiagramRelationRepairCandidate
	for _, f := range lease.Failures {
		if f.Issue == diagramNativeSequenceEventOrderConflict {
			failure = f
		}
	}
	if failure.FailureRef == "" {
		t.Fatalf("time order did not remain a hard gate: %+v", lease)
	}
	for _, c := range lease.AllowedAdditions {
		if c.FromIdentity == failure.FromIdentity && c.ToIdentity == failure.ToIdentity {
			candidate = c
			break
		}
	}
	if candidate.AdditionRef == "" {
		t.Fatalf("missing same exact event for local re-placement: %+v", lease)
	}
	// The final (middle-time) event must be reinserted before the later one.
	before := rows[2].FromNode + "->>" + rows[2].ToNode + ": 唤醒"
	placement := sequencePlacementPublicRef(t, ctx, before)
	patch := map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": []emitAnswerDiagramEdgeEdit{
		{Action: "remove", FailureRef: failure.FailureRef},
		{Action: "add", AdditionRef: candidate.AdditionRef, PlacementRef: placement, Edge: &types.DiagramEdgeAnchor{FromNode: candidate.FromNodeIDs[0], ToNode: candidate.ToNodeIDs[0], VisibleLabel: "唤醒"}},
	}}
	raw, _ := json.Marshal(patch)
	out, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
	if err != nil || !out.Success {
		t.Fatalf("same native event local chronological repair: %v %s", err, out.Summary)
	}
	after := ctx.Mutable.AnswerDocumentV2()
	if len(after.Blocks[1].EdgeAnchors) != 3 {
		t.Fatal("chronological repair changed the event population")
	}
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, after, types.BuildAnswerSemanticViewForBusContext(ctx), nil); len(got) != 0 {
		t.Fatalf("post order/authority failure: %+v", got)
	}
}

func TestHMC242RequestedThreadPublicOrderCannotDiscardOrReplay(t *testing.T) {
	for _, mode := range []string{"remove_only", "add_only", "same_ref_twice", "stale_position", "changed_body"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows := requestedThreadFixtureHMC242(t)
			doc := requestedThreadDocHMC242([]RuntimeDiagramRelation{rows[0], rows[2], rows[1]}, true)
			lease := requestedThreadRejectLeaseHMC242(t, ctx, doc)
			f, c := lease.Failures[0], lease.AllowedAdditions[0]
			before := rows[2].FromNode + "->>" + rows[2].ToNode + ": 唤醒"
			edits := []emitAnswerDiagramEdgeEdit{{Action: "remove", FailureRef: f.FailureRef}, {Action: "add", AdditionRef: c.AdditionRef, PlacementRef: sequencePlacementPublicRef(t, ctx, before), Edge: &types.DiagramEdgeAnchor{FromNode: c.FromNodeIDs[0], ToNode: c.ToNodeIDs[0], VisibleLabel: "唤醒"}}}
			switch mode {
			case "remove_only":
				edits = edits[:1]
			case "add_only":
				edits = edits[1:]
			case "same_ref_twice":
				edits = append(edits, edits[1])
			case "stale_position":
				edits[1].PlacementRef = "sp1-old"
			case "changed_body":
				doc.Blocks[1].Diagram.Body += "\nNote over " + rows[0].ToNode + ": changed"
				ctx.Mutable.SetLastRejectedAnswerDocumentV2(doc)
			}
			raw, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary", "table"}, "diagram_edge_edits": edits})
			out, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, raw)
			if err != nil || out.Success {
				t.Fatalf("invalid native movement published: %v %+v", err, out)
			}
		})
	}
}

func TestHMC242RequestedThreadNativeScopeCannotBorrowOldNodes(t *testing.T) {
	for _, mode := range []string{"source", "window"} {
		t.Run(mode, func(t *testing.T) {
			ctx, old := requestedThreadFixtureHMC242(t)
			path, err := filepath.Abs("../../eval/fixtures/hmosperf_event_participants/wakeups.systrace")
			if err != nil {
				t.Fatal(err)
			}
			start := 1.0
			if mode == "source" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(t.TempDir(), "another.systrace")
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				start = 1.005
			}
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "wakeup_chain", "time_start": start, "time_end": 1.02})
			rows := RuntimeDiagramRelations(types.ObservationLedger{Records: result.Observations}, &ctx.AnalysisIR.RequestModel)
			if len(rows) == 0 {
				t.Fatal("scope negative did not reach real native rows")
			}
			if requestedRuntimeParticipantNode(hmc242ParticipantQuote, old[0].ToNode, rows) {
				t.Fatal("different native source/query window borrowed previous node")
			}
			if !requestedRuntimeParticipantNode(hmc242ParticipantQuote, rows[0].ToNode, rows) {
				t.Fatal("new source/query lost its own exact request binding")
			}
		})
	}
}

func TestHMC242RequestedThreadBridgeBoundaries(t *testing.T) {
	for _, name := range []string{"other_tid", "process", "lookup", "artifact", "name_only", "business_role", "other_quote", "context_role", "ambiguous_roster", "no_profile", "unknown_tgid", "different_tgid"} {
		t.Run(name, func(t *testing.T) {
			ctx, rows := requestedThreadFixtureHMC242(t)
			rm := ctx.AnalysisIR.RequestModel
			rm.RuntimeTargets = append([]types.RuntimeTarget(nil), rm.RuntimeTargets...)
			hint := *rm.DiagramHint
			hint.Participants = append([]types.DiagramParticipantHint(nil), hint.Participants...)
			rm.DiagramHint = &hint
			profile := *rm.RuntimeTargetProfile
			rm.RuntimeTargetProfile = &profile
			for i := range rows {
				rows[i].requestedFrom, rows[i].requestedTo = nil, nil
			}
			switch name {
			case "other_tid":
				rm.RuntimeTargets[0].PID = 99
			case "process":
				rm.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
			case "lookup":
				rm.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
			case "artifact":
				rm.RuntimeTargets[0].Source = "artifact_metadata"
			case "name_only":
				rm.RuntimeTargets[0].PID = 0
			case "business_role":
				hint.Participants[0].Identity = "business owner"
			case "other_quote":
				profile.SourceQuote = "different request tid 42"
			case "context_role":
				hint.Participants[0].Role = types.DiagramParticipantContextOnly
			case "ambiguous_roster":
				rm.RuntimeTargets = append(rm.RuntimeTargets, rm.RuntimeTargets[0])
			case "no_profile":
				rm.RuntimeTargetProfile = nil
			case "unknown_tgid", "different_tgid":
				copyEndpoint := *rows[0].threadEndpoints
				rows[0].threadEndpoints = &copyEndpoint
				copyEndpoint.to.TGID = 0
				if name == "different_tgid" {
					copyEndpoint.to.TGID = 777
				}
			}
			got := bindRequestedRuntimeParticipants(rows, &rm)
			for _, row := range got {
				if len(row.requestedFrom)+len(row.requestedTo) > 0 {
					t.Fatalf("unproved request/instance mapping: %+v", row)
				}
			}
		})
	}
}

func TestHMC242RequestedThreadVisibilityUsesCurrentNativeNode(t *testing.T) {
	ctx, rows := requestedThreadFixtureHMC242(t)
	base := requestedThreadDocHMC242(rows, true).Blocks[1]
	failure := types.AnswerDiagramParticipantVisibilityRepairFailure{Participant: hmc242ParticipantQuote, BaseDiagramFingerprint: types.AnswerDiagramParticipantVisibilityFingerprint(base)}
	for _, name := range []string{"same", "wrong_side", "changed_native_node", "old_ref"} {
		t.Run(name, func(t *testing.T) {
			block := cloneAtomicDiagramPatchBlock(base)
			edit := emitAnswerDiagramParticipantEdit{NodeID: rows[0].ToNode, VisibleLabel: "目标线程（tid 42）"}
			current := append([]RuntimeDiagramRelation(nil), rows...)
			if name == "wrong_side" {
				edit.NodeID = rows[0].FromNode
			}
			if name == "changed_native_node" {
				for i := range current {
					current[i].ToNode += "_new_" + name
				}
			}
			if name == "old_ref" {
				block.Diagram.Body += "\nNote over " + rows[0].ToNode + ": changed"
			}
			err := applyOneModelAuthoredDiagramParticipantVisibilityEdit(&block, block, edit, failure, current)
			if (err == nil) != (name == "same") {
				t.Fatalf("unexpected visibility binding: %v", err)
			}
		})
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	if !strings.Contains(text, "requested_participant_binding=") || string(before) != string(after) {
		t.Fatal("bridge lost teaching or changed causal projection")
	}
}
