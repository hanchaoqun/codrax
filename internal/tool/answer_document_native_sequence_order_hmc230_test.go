package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeSequenceHMC230Doc(rows []RuntimeDiagramRelation) *types.AnswerDocumentV2 {
	b := types.AnswerBlock{ID: "events", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n"}}
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		b.Diagram.Body += fmt.Sprintf(" %s->>%s: event\n", r.FromNode, r.ToNode)
		b.EdgeAnchors = append(b.EdgeAnchors, types.DiagramEdgeAnchor{FromNode: r.FromNode, ToNode: r.ToNode, FromIdentity: r.FromIdentity, ToIdentity: r.ToIdentity, RelationKind: r.Kind})
	}
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{b}}
}

func TestNativeSequenceOrderHMC230ScopeComparability(t *testing.T) {
	ctx, result, rows := runtimeWakeFixture(t)
	doc := nativeSequenceHMC230Doc(rows)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if got := DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(ctx, doc, view, nil); len(got) != 1 || got[0].Issue != diagramNativeSequenceEventOrderConflict {
		t.Fatalf("need exact order failure: %+v", got)
	}
	for _, change := range []string{"clock", "physical_source", "window", "equal_time", "no_point_receipt"} {
		t.Run(change, func(t *testing.T) {
			copyRows := append([]RuntimeDiagramRelation(nil), rows...)
			order := *copyRows[0].eventOrder
			copyRows[0].eventOrder = &order
			switch change {
			case "equal_time":
				order.timestamp = rows[1].eventOrder.timestamp
			case "no_point_receipt":
				copyRows[0].eventOrder = nil
			default:
				var r types.ObservationRecord
				var f traceWakeupEvent
				for _, candidate := range result.Observations {
					if fact, ok := decodeRuntimeWakeupEvent(candidate); ok {
						r, f = candidate, fact
						break
					}
				}
				before := runtimeWakeupDiagramEventOrder(r, f)
				switch change {
				case "clock":
					r.SourceRef.TimeDomain += "other"
				case "physical_source":
					r.SupportRefs = append([]string(nil), r.SupportRefs...)
					r.SupportRefs[0] = "/another/source.systrace:1"
				case "window":
					r.SourceRef.QueryWindowStartTs -= 1
				}
				after := runtimeWakeupDiagramEventOrder(r, f)
				if before == nil || after == nil || before.scope == after.scope {
					t.Fatal("scope coordinate did not isolate comparison")
				}
				order.scope = after.scope
			}
			if got := diagramCallEdgeEvidenceMismatchesWithRequestModel(doc, view, nil, nil, &ctx.AnalysisIR.RequestModel, copyRows); len(got) > 0 {
				t.Fatalf("incomparable/tied events gained forced total order: %+v", got)
			}
		})
	}
}

func TestNativeSequenceOrderHMC230NativeLaneTeachingAndProjection(t *testing.T) {
	ctx, result, _ := runtimeWakeFixture(t)
	ledger := types.ObservationLedger{Records: result.Observations}
	before, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	text := RenderRuntimeDiagramRelationRecipes(ledger, &ctx.AnalysisIR.RequestModel)
	if !strings.Contains(text, "reuse the same supplied native thread participant") || !strings.Contains(text, "unproven note") {
		t.Fatalf("missing precise reuse boundary: %s", text)
	}
	after, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
	if string(before) != string(after) {
		t.Fatal("presentation authority changed private Trace projection")
	}
	for _, l := range []types.ObservationLedger{{}, {Records: []types.ObservationRecord{{Predicate: "not_a_native_event"}}}} {
		if text := RenderRuntimeDiagramRelationRecipes(l, nil); strings.Contains(text, "reuse the same supplied native thread participant") {
			t.Fatalf("native identity teaching invented without identities: %s", text)
		}
	}
}
