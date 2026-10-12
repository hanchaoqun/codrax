package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A point event is not a new relation. This receipt can compare two already
// proved messages, but never connect them, infer causality, or order static
// code. Different sources/clocks/windows remain incomparable.
type runtimeDiagramEventOrder struct {
	scope     string
	point     string
	timestamp float64
}

type runtimeDiagramSequenceEvent struct {
	row              RuntimeDiagramRelation
	anchor           types.DiagramEdgeAnchor
	line, occurrence int
}

func runtimeWakeupDiagramEventOrder(r types.ObservationRecord, f traceWakeupEvent) *runtimeDiagramEventOrder {
	point, physical, ok := runtimeWakeupPhysicalPoint(r)
	if !ok {
		return nil
	}
	ref := r.SourceRef
	var window any = []any{ref.QueryWindowKnown, ref.QueryWindowStartTs, ref.QueryWindowKnown, ref.QueryWindowEndTs, ref.QueryLineStart, ref.QueryLineEnd}
	if f.ScanScope != nil {
		s := f.ScanScope
		window = []any{s.TimeStartApplied, s.TimeStart, s.TimeEndApplied, s.TimeEnd, s.LineStart, s.LineEnd}
	}
	key, err := json.Marshal([]any{ref.Path, physical, ref.TimeDomain, ref.CanonicalTimeDomain,
		ref.ClockAlignment, ref.ClockCalibrated, ref.ClockOffsetSec, ref.ClockSlope, window})
	if err != nil {
		return nil
	}
	return &runtimeDiagramEventOrder{scope: string(key), point: point, timestamp: f.Timestamp}
}

// Only exact anchor occurrence → provider tuple matches carry time. Message
// labels (including apparent timestamps), participant names and surrounding
// prose cannot select an event. Control-region boundaries and static messages
// break comparisons: this is not a total ordering of an arbitrary diagram.
func diagramNativeSequenceEventOrderMismatches(doc *types.AnswerDocumentV2, rows []RuntimeDiagramRelation, views ...*types.AnswerSemanticView) []DiagramCallEdgeEvidenceMismatch {
	if doc == nil {
		return nil
	}
	edgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	var out []DiagramCallEdgeEvidenceMismatch
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if block.Diagram == nil || block.Diagram.Kind != types.DiagramSequence {
			continue
		}
		var view *types.AnswerSemanticView
		if len(views) > 0 {
			view = views[0]
		}
		notePoints, noteErr := nativeDiagramNotePoints(block, view, rows, false)
		if noteErr != nil {
			out = append(out, DiagramCallEdgeEvidenceMismatch{BlockID: block.ID, Issue: diagramNativeNoteBindingUnavailable, nativeSequenceOrderDetail: noteErr.Error()})
			continue
		}
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, bi, edgeBlockCounts)
		byPair := map[string][]types.DiagramEdgeAnchor{}
		counts := map[string]int{}
		for _, a := range anchors {
			k := diagramEvidenceEdgeKey(a.FromNode, a.ToNode)
			byPair[k] = append(byPair[k], a)
		}
		for _, e := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			counts[diagramEvidenceEdgeKey(e.From, e.To)]++
		}
		occurrences := map[string]int{}
		latest := map[string]runtimeDiagramSequenceEvent{}
		for lineIndex, line := range strings.Split(block.Diagram.Body, "\n") {
			if current := notePoints[lineIndex+1]; current != nil {
				if prior, ok := latest[current.scope]; ok && prior.row.eventOrder.timestamp > current.timestamp {
					out = append(out, DiagramCallEdgeEvidenceMismatch{BlockID: block.ID, Issue: diagramNativeSequenceEventOrderConflict, nativeSequenceOrderDetail: fmt.Sprintf("block=%q: native note body line %d (%.9fs; %s) must precede native point body line %d (%.9fs; %s); move the selected Note statement and update runtime_notes_v1.body_line in replace_blocks", block.ID, lineIndex+1, current.timestamp, current.point, prior.line, prior.row.eventOrder.timestamp, prior.row.eventOrder.point)})
				} else {
					latest[current.scope] = runtimeDiagramSequenceEvent{row: RuntimeDiagramRelation{eventOrder: current}, line: lineIndex + 1}
				}
				continue
			}
			edges := mermaidcompat.ParseEdges("sequenceDiagram\n" + line)
			if len(edges) != 1 {
				if !nativeSequenceOrderTransparentLine(line) {
					latest = map[string]runtimeDiagramSequenceEvent{}
				}
				continue
			}
			e := edges[0]
			key := diagramEvidenceEdgeKey(e.From, e.To)
			occurrence := occurrences[key]
			occurrences[key]++
			if len(byPair[key]) != counts[key] || occurrence >= len(byPair[key]) {
				latest = map[string]runtimeDiagramSequenceEvent{}
				continue
			}
			a := byPair[key][occurrence]
			var row *RuntimeDiagramRelation
			for ri := range rows {
				r := &rows[ri]
				if r.eventOrder != nil && a.FromIdentity == r.FromIdentity && a.ToIdentity == r.ToIdentity && diagramAnchorRelation(a) == r.Kind {
					row = r
					break
				}
			}
			if row == nil {
				latest = map[string]runtimeDiagramSequenceEvent{}
				continue
			}
			current := row.eventOrder
			if prior, ok := latest[current.scope]; ok && prior.row.eventOrder.timestamp > current.timestamp {
				if prior.anchor.FromNode == "" {
					out = append(out, DiagramCallEdgeEvidenceMismatch{BlockID: block.ID, Issue: diagramNativeSequenceEventOrderConflict, FromNode: a.FromNode, ToNode: a.ToNode, FromSymbol: a.FromIdentity, ToSymbol: a.ToIdentity, Relation: row.Kind, BodyOccurrence: occurrence + 1, nativeSequenceOrderDetail: fmt.Sprintf("block=%q: native event body line %d (%.9fs; %s) must precede native note body line %d (%.9fs; %s); move statements and update runtime_notes_v1.body_line in replace_blocks", block.ID, lineIndex+1, current.timestamp, current.point, prior.line, prior.row.eventOrder.timestamp, prior.row.eventOrder.point)})
					continue
				}
				out = append(out, DiagramCallEdgeEvidenceMismatch{BlockID: block.ID, Issue: diagramNativeSequenceEventOrderConflict,
					FromNode: a.FromNode, ToNode: a.ToNode, FromSymbol: a.FromIdentity, ToSymbol: a.ToIdentity, Relation: row.Kind, BodyOccurrence: occurrence + 1,
					nativeSequenceOrderDetail: fmt.Sprintf("block=%q: move body line %d native event %s -> %s occurrence %d (identities %s -> %s; %.9fs; %s) before body line %d native event %s -> %s occurrence %d (identities %s -> %s; %.9fs; %s) in this same sequential region", block.ID, lineIndex+1, a.FromNode, a.ToNode, occurrence+1, a.FromIdentity, a.ToIdentity, current.timestamp, current.point, prior.line, prior.anchor.FromNode, prior.anchor.ToNode, prior.occurrence, prior.anchor.FromIdentity, prior.anchor.ToIdentity, prior.row.eventOrder.timestamp, prior.row.eventOrder.point)})
			} else {
				latest[current.scope] = runtimeDiagramSequenceEvent{row: *row, anchor: a, line: lineIndex + 1, occurrence: occurrence + 1}
			}
		}
	}
	return out
}

func nativeSequenceOrderTransparentLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "%%") || strings.EqualFold(line, "sequenceDiagram") || len(mermaidcompat.SequenceParticipantDeclarations(line)) > 0 {
		return true
	}
	fields := strings.Fields(strings.ToLower(line))
	if len(fields) == 0 {
		return true
	}
	switch fields[0] {
	case "note", "activate", "deactivate", "autonumber", "title":
		return true
	default:
		return false
	}
}
