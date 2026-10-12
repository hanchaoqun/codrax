package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeBindingHMC231VisibilityRelabelKeepsNotesAndUnprovenRole(t *testing.T) {
	base := types.AnswerBlock{ID: "graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n participant W as worker-200\n Note over W: unchanged state\n"}, ParticipantBoundaries: []types.DiagramParticipantBoundary{{Participant: "文档工作线程", Status: types.DiagramParticipantBoundaryUnproven}}}
	lease := types.WithAnswerDiagramParticipantVisibilityRepairFailures(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{base}}, nil, []types.AnswerDiagramParticipantVisibilityRepairFailure{{BlockID: "graph", Participant: "文档工作线程", Issue: "boundary_participant_not_visible"}})
	failure := lease.ParticipantVisibilityFailures[0]
	edit := emitAnswerDiagramParticipantEdit{Action: "ensure_visible", ParticipantRef: failure.ParticipantRef, NodeID: "W", VisibleLabel: "文档工作线程"}
	got := cloneAtomicDiagramPatchBlock(base)
	if err := applyOneModelAuthoredDiagramParticipantVisibilityEdit(&got, base, edit, failure); err != nil {
		t.Fatal(err)
	}
	if strings.Count(got.Diagram.Body, "participant ") != 1 || !strings.Contains(got.Diagram.Body, "Note over W: unchanged state") || !reflect.DeepEqual(got.ParticipantBoundaries, base.ParticipantBoundaries) || len(got.EdgeAnchors) > 0 {
		t.Fatalf("relabel gained authority or lost node/note: %+v", got)
	}
	for _, mode := range []string{"stale", "wrong_label", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			b := cloneAtomicDiagramPatchBlock(base)
			e := edit
			switch mode {
			case "stale":
				b.Diagram.Body += "\nNote over W: other"
			case "wrong_label":
				e.VisibleLabel = "another role"
			case "duplicate":
				b.Diagram.Body += "\nparticipant W as same"
				failure.BaseDiagramFingerprint = types.AnswerDiagramParticipantVisibilityFingerprint(b)
			}
			if err := applyOneModelAuthoredDiagramParticipantVisibilityEdit(&b, b, e, failure); err == nil {
				t.Fatal("invalid rebinding accepted")
			}
		})
	}
}

func TestNativeBindingHMC231OnlySameProducerLane(t *testing.T) {
	rows := []RuntimeDiagramRelation{{Kind: types.DiagramRelWakeup, FromIdentity: "event-a", ToIdentity: "event-b", FromNode: "rt_thread_source", ToNode: "rt_thread_worker"}, {Kind: types.DiagramRelWakeup, FromIdentity: "event-c", ToIdentity: "event-d", FromNode: "rt_thread_worker", ToNode: "rt_thread_main"}}
	anchors := []types.DiagramEdgeAnchor{{FromNode: "S", ToNode: "W", FromIdentity: "event-a", ToIdentity: "event-b", RelationKind: types.DiagramRelWakeup}, {FromNode: "W", ToNode: "M", FromIdentity: "event-c", ToIdentity: "event-d", RelationKind: types.DiagramRelWakeup}}
	if runtimeDiagramAliasBindingsConflict(rows, anchors, "W") {
		t.Fatal("same native thread's different event roles must reuse one lane")
	}
	for _, scope := range []string{"source", "window", "generation", "thread"} {
		copyRows := append([]RuntimeDiagramRelation(nil), rows...)
		copyRows[1].FromNode += "_different_" + scope
		if !runtimeDiagramAliasBindingsConflict(copyRows, anchors, "W") {
			t.Fatal("foreign producer lane merged")
		}
	}
}

func TestNativeBindingHMC231NoteEscapingAndActualRender(t *testing.T) {
	name := "业务:读取;阶段\n第二行<not-html>"
	escaped := nativeDiagramNoteText(name)
	if escaped != "业务:读取#59;阶段<br/>第二行#60;not-html#62;" {
		t.Fatal(escaped)
	}
	body := "sequenceDiagram\n participant W as 工作线程\n Note over W: " + escaped + " end @ 1.044500000s"
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "graph", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: body}}}}
	visible := render.RenderAnswerDocument(doc, "zh")
	if !strings.Contains(visible, "业务:读取#59;阶段") || !strings.Contains(visible, "<br/>") || len(mermaidcompat.ParseEdges(body)) != 0 || len(mermaidcompat.SequenceParticipantReferences(body)) != 1 {
		t.Fatalf("name changed graph syntax or disappeared: %s", visible)
	}
}

func TestNativeBindingHMC231AtomicCoordinateLineage(t *testing.T) {
	base := types.AnswerBlock{Diagram: &types.AnswerDiagramBlock{Body: "sequenceDiagram\nparticipant W\nNote over W: native point", RuntimeNotesV1: []types.RuntimeDiagramNoteSelection{{Selector: "s", Point: "end", NodeID: "W", BodyLine: 3}}}}
	changed := cloneAtomicDiagramPatchBlock(base)
	changed.Diagram.Body = "sequenceDiagram\nparticipant W\nparticipant M\nNote over W: native point"
	if err := preserveAtomicNativeNoteCoordinates(base, &changed); err != nil || changed.Diagram.RuntimeNotesV1[0].BodyLine != 4 || base.Diagram.RuntimeNotesV1[0].BodyLine != 3 {
		t.Fatalf("lineage or copy broken: %v %+v", err, changed)
	}
	changed.Diagram.Body += "\nNote over W: native point"
	if err := preserveAtomicNativeNoteCoordinates(base, &changed); err == nil {
		t.Fatal("ambiguous source line silently picked")
	}
}
