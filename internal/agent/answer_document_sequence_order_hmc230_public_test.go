package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func sequenceOrderHMC230Fixture(t *testing.T) (*types.AgentContext, []tool.RuntimeDiagramRelation) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Observed wakeups"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain, PredicateAxis: types.AxisFlow}}}
	raw, _ := json.Marshal(map[string]any{"path": path, "view": "wakeup_chain", "pid": 100, "time_start": 1.0, "time_end": 1.051})
	query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !query.Success {
		t.Fatalf("query: %v %+v", err, query)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) != 2 {
		t.Fatalf("want two exact native wakeups: %+v", rows)
	}
	return ctx, rows
}

func sequenceOrderHMC230Doc(rows []tool.RuntimeDiagramRelation, reverse bool) *types.AnswerDocumentV2 {
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	seen := map[string]bool{}
	for _, r := range rows {
		for _, n := range [][2]string{{r.FromNode, r.FromLabel}, {r.ToNode, r.ToLabel}} {
			if !seen[n[0]] {
				fmt.Fprintf(&b, " participant %s as %s\n", n[0], n[1])
				seen[n[0]] = true
			}
		}
	}
	ordered := append([]tool.RuntimeDiagramRelation(nil), rows...)
	if reverse {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	var anchors []types.DiagramEdgeAnchor
	for _, r := range ordered {
		fmt.Fprintf(&b, " %s->>%s: observed event\n", r.FromNode, r.ToNode)
		anchors = append(anchors, types.DiagramEdgeAnchor{FromNode: r.FromNode, ToNode: r.ToNode, FromIdentity: r.FromIdentity, ToIdentity: r.ToIdentity, RelationKind: r.Kind, VisibleLabel: "observed event"})
	}
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, TraceCausalClaimCaliber: types.TraceCausalClaimNoConclusion, Text: "Only recorded wakeups; no cause or wait attribution."},
		{ID: "events", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: b.String()}, EdgeAnchors: anchors},
	}}
}

func TestNativeSequenceOrderHMC230PublicRejectAndPatch(t *testing.T) {
	ctx, rows := sequenceOrderHMC230Fixture(t)
	doc := sequenceOrderHMC230Doc(rows, true)
	raw, _ := json.Marshal(doc)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || res.Success {
		t.Fatalf("reversed exact native events must reject, not accept real but backwards arrows: %v %+v", err, res)
	}
	if res.Repair == nil || !strings.Contains(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], "native_sequence_event_order_conflict") {
		t.Fatalf("need precise event-order issue: %+v", res)
	}
	if !strings.Contains(res.Summary, "1.040010") || !strings.Contains(res.Summary, "1.045000") || !strings.Contains(res.Summary, "replace_blocks") {
		t.Fatalf("repair must publish exact source order and executable replacement: %s", res.Summary)
	}
	installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false)
	fixed := sequenceOrderHMC230Doc(rows, false)
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []types.AnswerBlock{fixed.Blocks[1]}})
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("native order replacement rejected: %v %+v", err, res)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != doc.Blocks[0].Text {
		t.Fatal("sibling summary changed")
	}
	if bad := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, got, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(bad) > 0 {
		t.Fatalf("post validation disagrees: %+v", bad)
	}
}

func TestNativeSequenceOrderHMC230PublicPositiveShapes(t *testing.T) {
	for _, shape := range []string{"chronological", "two_independent_wakers", "alt", "par", "loop", "flow", "optional_absent", "unproven_role_notes", "mixed_static", "label_times_are_not_authority", "custom_business_aliases"} {
		t.Run(shape, func(t *testing.T) {
			ctx, rows := sequenceOrderHMC230Fixture(t)
			if shape == "two_independent_wakers" {
				path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
				ctx.Mutable = types.NewMutableState("Independent wakers")
				raw, _ := json.Marshal(map[string]any{"path": path, "view": "wakeup_chain", "pid": 200, "time_start": 5.0, "time_end": 5.03})
				query, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
				if err != nil || !query.Success {
					t.Fatalf("two-waker query: %v %+v", err, query)
				}
				ctx.Mutable.AppendDispatchToolResult(query)
				ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{query}})
				rows = tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
				if len(rows) < 2 || rows[0].FromNode == rows[1].FromNode || rows[0].ToNode != rows[1].ToNode {
					t.Fatalf("need distinct wakers, one observed target: %+v", rows)
				}
				rows = rows[:2]
			}
			doc := sequenceOrderHMC230Doc(rows, shape == "alt" || shape == "par" || shape == "loop" || shape == "flow" || shape == "mixed_static")
			b := &doc.Blocks[1]
			lines := strings.Split(strings.TrimSuffix(b.Diagram.Body, "\n"), "\n")
			n := len(lines)
			switch shape {
			case "alt", "par":
				separator := "else other branch"
				if shape == "par" {
					separator = "and concurrent branch"
				}
				b.Diagram.Body = strings.Join(lines[:n-2], "\n") + "\n" + shape + " selected branch\n" + lines[n-2] + "\n" + separator + "\n" + lines[n-1] + "\nend\n"
			case "loop":
				b.Diagram.Body = strings.Join(lines[:n-2], "\n") + "\nloop selected iteration\n" + lines[n-2] + "\nend\n" + lines[n-1] + "\n"
			case "flow":
				b.Diagram.Kind = types.DiagramFlow
				b.Diagram.Body = "flowchart TD\n"
				for _, a := range b.EdgeAnchors {
					b.Diagram.Body += a.FromNode + " -->|observed event| " + a.ToNode + "\n"
				}
			case "optional_absent":
				doc.Blocks = doc.Blocks[:1]
			case "unproven_role_notes":
				b.Diagram.Body += " participant M as unresolved business role\n Note over M: Role-to-thread mapping is unproven\n"
			case "mixed_static":
				b.Diagram.Body = strings.Join(lines[:n-1], "\n") + "\n participant A as Alpha.Run\n participant B as Beta.Run\n A->>B: static call\n" + lines[n-1] + "\n"
				b.EdgeAnchors = append(b.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", FromIdentity: "Alpha.Run", ToIdentity: "Beta.Run", RelationKind: types.DiagramRelCall, VisibleLabel: "static call"})
				ctx.EvidenceItems = []types.EvidenceItem{{ID: "static-call", Kind: types.EvidenceRelationship, Subject: "Alpha.Run", Predicate: "calls", Object: "Beta.Run", Source: "example.go", LineStart: 10, AnchorKind: types.AnchorCall, Scope: types.ScopeLine, GroundingStatus: types.GroundingGrounded, Producer: types.EvidenceProducerExplorerEmitEvidence}}
			case "label_times_are_not_authority":
				b.EdgeAnchors[0].VisibleLabel = "label 9.9"
				b.EdgeAnchors[1].VisibleLabel = "label 0.0"
				b.Diagram.Body = strings.Replace(b.Diagram.Body, "observed event", "label 9.9", 1)
				b.Diagram.Body = strings.Replace(b.Diagram.Body, "observed event", "label 0.0", 1)
			case "custom_business_aliases":
				aliases := map[string]string{}
				for _, a := range b.EdgeAnchors {
					for _, id := range []string{a.FromNode, a.ToNode} {
						if aliases[id] == "" {
							aliases[id] = fmt.Sprintf("business%d", len(aliases)+1)
						}
					}
				}
				if len(aliases) != 3 {
					t.Fatalf("native source identity should supply three lanes, not one per event: %v", aliases)
				}
				for id, alias := range aliases {
					b.Diagram.Body = strings.ReplaceAll(b.Diagram.Body, id, alias)
				}
				for i := range b.EdgeAnchors {
					a := &b.EdgeAnchors[i]
					a.FromNode = aliases[a.FromNode]
					a.ToNode = aliases[a.ToNode]
				}
			}
			raw, _ := json.Marshal(doc)
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || !res.Success {
				t.Fatalf("valid %s rejected: %v %+v", shape, err, res)
			}
			if bad := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(bus), ctx.EvidenceItems); len(bad) > 0 {
				t.Fatalf("post rejects positive: %+v", bad)
			}
		})
	}
}

func TestNativeSequenceOrderHMC230OldLeaseRetired(t *testing.T) {
	ctx, rows := sequenceOrderHMC230Fixture(t)
	doc := sequenceOrderHMC230Doc(rows, true)
	doc.Blocks[1].EdgeAnchors = nil
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	raw, _ := json.Marshal(doc)
	first, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || first.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &first, false) {
		t.Fatalf("missing exact anchor lease: %v %+v", err, first)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	var edits []map[string]any
	for _, f := range lease.Failures {
		for _, c := range lease.AllowedAdditions {
			if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(f, c) {
				edits = append(edits, map[string]any{"action": "attach", "failure_ref": f.FailureRef, "addition_ref": c.AdditionRef, "edge": map[string]string{"from_node": f.FromNode, "to_node": f.ToNode, "visible_label": "observed event"}})
				break
			}
		}
	}
	if len(edits) != 2 {
		t.Fatalf("need two exact attach actions: %+v", lease)
	}
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
	res, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || res.Success || res.Repair == nil || !strings.Contains(res.Repair.Metadata[types.ToolRepairMetaDiagramRelationFailureIssues], "native_sequence_event_order_conflict") {
		t.Fatalf("attachment must reveal reversed time without publishing it: %v %+v", err, res)
	}
	if installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false) || ctx.Mutable.AnswerDiagramRelationRepairLease() != nil {
		t.Fatal("order-only repair retained stale edge lease")
	}
	if hint, ok := answerDocDiagramRelationDeltaPatchHint(&res, true, false); !ok || !strings.Contains(hint, "replace_blocks") || strings.Contains(hint, "use `diagram_edge_edits`") {
		t.Fatalf("wrong order-repair teaching: %s", hint)
	}
	// Earlier opaque refs cannot act on this new generation.
	stale, err := (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || stale.Success {
		t.Fatalf("old attachment refs replayed: %v %+v", err, stale)
	}
	fixed := sequenceOrderHMC230Doc(rows, false)
	raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []types.AnswerBlock{fixed.Blocks[1]}})
	res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
	if err != nil || !res.Success {
		t.Fatalf("order replacement after old lease failed: %v %+v", err, res)
	}
}

func TestNativeSequenceOrderHMC230PublicStaleTupleRejects(t *testing.T) {
	for _, change := range []string{"cross_event_tuple", "stale_event", "stale_source"} {
		t.Run(change, func(t *testing.T) {
			ctx, rows := sequenceOrderHMC230Fixture(t)
			doc := sequenceOrderHMC230Doc(rows, false)
			if change == "cross_event_tuple" {
				doc.Blocks[1].EdgeAnchors[0].ToIdentity = rows[1].ToIdentity
			} else {
				results := ctx.Mutable.DispatchToolResults()
				for ri := range results {
					for i := range results[ri].Observations {
						r := &results[ri].Observations[i]
						if r.Predicate != "wakeup_chain_edge" {
							continue
						}
						if change == "stale_event" {
							r.Span.StartTs += .000001
						} else {
							r.SourceRef.Path += ".other"
						}
					}
				}
				ctx.Mutable = types.NewMutableState("stale facts")
				for _, r := range results {
					ctx.Mutable.AppendDispatchToolResult(r)
				}
			}
			raw, _ := json.Marshal(doc)
			res, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), raw)
			if err != nil || res.Success {
				t.Fatalf("stale native tuple accepted: %v %+v", err, res)
			}
		})
	}
}
