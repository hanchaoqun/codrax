package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func projectNativeDiagramNotes(blockProps map[string]any, view *types.AnswerSemanticView) {
	diagram, _ := blockProps["diagram"].(map[string]any)
	props, _ := diagram["properties"].(map[string]any)
	if props == nil {
		return
	}
	delete(props, "runtime_notes_v1")
	if view == nil || view.RuntimeMeasurementContract == nil || view.DiagramPlan == nil || view.DiagramPlan.Kind != types.DiagramSequence {
		return
	}
	var choices []any
	for _, a := range view.RuntimeMeasurementContract.NativeBusinessDiagramAnnotations() {
		r := a.Record
		r.SupportRefs = []string{a.StartRef}
		lane := runtimeWakeupDisplayNode(r, tracequery.ThreadRef(a.Owner), "")
		choices = append(choices, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"selector", "point", "node_id", "body_line"}, "description": fmt.Sprintf("Business %q; observed owner %s; native participant %s; start %.9fs (%s), end %.9fs (%s); query window [%.9f,%.9f), clock %q. Display only; no role mapping or causal permission.", a.Name, types.RuntimeWakeupThreadLabel(a.Owner), lane, a.StartTs, a.StartRef, a.EndTs, a.EndRef, r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs, r.SourceRef.TimeDomain), "properties": map[string]any{
			"selector": map[string]any{"type": "string", "enum": []string{a.Selector}}, "point": map[string]any{"type": "string", "enum": []string{"start", "end"}}, "node_id": map[string]any{"type": "string", "minLength": 1, "description": "Use the exact native participant above or one existing declared alias whose event anchors bind to that same native lane."}, "body_line": map[string]any{"type": "integer", "minimum": 1},
		}})
	}
	if len(choices) == 0 {
		return
	}
	props["runtime_notes_v1"] = map[string]any{"type": "array", "maxItems": 64, "description": "Optional exact native business point annotations for sequence diagrams. Select a current business selector and start/end point, its exact existing native participant (or explicitly bound alias), and one existing Note statement's 1-based body_line. The provider replaces that line with verified owner/name/time. All other notes remain model-owned; this proves no business-role mapping or causal arrow. Keep selected points in source order within each linear region; correct placement with a full replace_blocks entry.", "items": map[string]any{"oneOf": choices}}
}

func nativeDiagramNoteLine(line string) bool {
	// Syntax only. The note's payload after ':' is never an identity selector.
	line = strings.TrimSpace(line)
	if !strings.Contains(line, ":") || len(mermaidcompat.ParseEdges("sequenceDiagram\n"+line)) != 0 {
		return false
	}
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "note over ") || strings.HasPrefix(lower, "note left of ") || strings.HasPrefix(lower, "note right of ")
}

func nativeDiagramNotePoints(block *types.AnswerBlock, view *types.AnswerSemanticView, rows []RuntimeDiagramRelation, materialize bool) (map[int]*runtimeDiagramEventOrder, error) {
	points := map[int]*runtimeDiagramEventOrder{}
	if block == nil || block.Diagram == nil || len(block.Diagram.RuntimeNotesV1) == 0 {
		return points, nil
	}
	d := block.Diagram
	if d.Kind != types.DiagramSequence || len(d.RuntimeNotesV1) > 64 || view == nil || view.RuntimeMeasurementContract == nil {
		return nil, fmt.Errorf("runtime_notes_v1 requires a sequence diagram and current native business choices")
	}
	available := map[string]types.NativeBusinessDiagramAnnotation{}
	for _, a := range view.RuntimeMeasurementContract.NativeBusinessDiagramAnnotations() {
		available[a.Selector] = a
	}
	lines := strings.Split(d.Body, "\n")
	for i, s := range d.RuntimeNotesV1 {
		a, ok := available[s.Selector]
		if !ok || (s.Point != "start" && s.Point != "end") {
			return nil, fmt.Errorf("runtime_notes_v1[%d] selector/point is unavailable or stale", i)
		}
		if s.BodyLine < 1 || s.BodyLine > len(lines) || points[s.BodyLine] != nil || !nativeDiagramNoteLine(lines[s.BodyLine-1]) {
			return nil, fmt.Errorf("runtime_notes_v1[%d] body_line must select one unique existing Note statement", i)
		}
		at, ref := a.StartTs, a.StartRef
		if s.Point == "end" {
			at, ref = a.EndTs, a.EndRef
		}
		record := a.Record
		record.SupportRefs = []string{ref}
		lane := runtimeWakeupDisplayNode(record, tracequery.ThreadRef(a.Owner), "")
		declared := 0
		for _, n := range diagramExplicitSourceNodeDeclarations(d.Body, d.Kind) {
			if n.ID == s.NodeID {
				declared++
			}
		}
		binds := s.NodeID == lane
		for _, anchor := range block.EdgeAnchors {
			for _, r := range rows {
				if anchor.RelationKind != r.Kind || anchor.FromIdentity != r.FromIdentity || anchor.ToIdentity != r.ToIdentity {
					continue
				}
				binds = binds || anchor.FromNode == s.NodeID && r.FromNode == lane || anchor.ToNode == s.NodeID && r.ToNode == lane
			}
		}
		if !strings.HasPrefix(lane, "rt_thread_") || declared != 1 || !binds || runtimeDiagramAliasBindingsConflict(rows, block.EdgeAnchors, s.NodeID) {
			return nil, fmt.Errorf("runtime_notes_v1[%d] node_id=%q is not bound to the exact native owner lane %q; use that declared lane or a proved event-side alias, never a name match", i, s.NodeID, lane)
		}
		order := runtimeWakeupDiagramEventOrder(record, traceWakeupEvent{Timestamp: at})
		if order == nil {
			return nil, fmt.Errorf("runtime_notes_v1[%d] has no precise source/clock/window point", i)
		}
		name := nativeDiagramNoteText(a.Name)
		rendered := fmt.Sprintf("Note over %s: %s %s @ %.9fs", s.NodeID, name, s.Point, at)
		if materialize {
			lines[s.BodyLine-1] = rendered
		} else if strings.TrimSpace(lines[s.BodyLine-1]) != rendered {
			return nil, fmt.Errorf("runtime_notes_v1[%d] selected note no longer matches its native point; resubmit its exact selector/line", i)
		}
		points[s.BodyLine] = order
	}
	if materialize {
		d.Body = strings.Join(lines, "\n")
	}
	return points, nil
}

func nativeDiagramNoteText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.NewReplacer("#", "#35;", "&", "#38;", "<", "#60;", ">", "#62;", ";", "#59;", "\"", "#quot;", "\r", "<br/>", "\n", "<br/>").Replace(text)
}

// Atomic edge/declaration edits cannot silently retarget an existing selector
// when they insert or delete earlier source lines. Relocate only the exact
// unchanged selected statement; text equality is source-coordinate lineage,
// never evidence or an inference about the note's contents.
func preserveAtomicNativeNoteCoordinates(base types.AnswerBlock, changed *types.AnswerBlock) error {
	if base.Diagram == nil || changed == nil || changed.Diagram == nil || len(base.Diagram.RuntimeNotesV1) == 0 {
		return nil
	}
	before, after := strings.Split(base.Diagram.Body, "\n"), strings.Split(changed.Diagram.Body, "\n")
	notes := append([]types.RuntimeDiagramNoteSelection(nil), base.Diagram.RuntimeNotesV1...)
	for i, n := range notes {
		if n.BodyLine < 1 || n.BodyLine > len(before) {
			return fmt.Errorf("native note source coordinate is unavailable; repair with a complete diagram emit/replace_blocks")
		}
		count, line := 0, 0
		for j, s := range after {
			if s == before[n.BodyLine-1] {
				count++
				line = j + 1
			}
		}
		if count != 1 {
			return fmt.Errorf("native note source coordinate is no longer unique; repair with a complete diagram emit/replace_blocks")
		}
		notes[i].BodyLine = line
	}
	changed.Diagram.RuntimeNotesV1 = notes
	return nil
}

func materializeNativeDiagramNotes(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, rows []RuntimeDiagramRelation) error {
	if doc == nil {
		return nil
	}
	// Validate and materialize on private copies; a later invalid selection
	// must not partially change the submitted document.
	changed := map[int]types.AnswerBlock{}
	var failures []string
	for i, b := range doc.Blocks {
		if b.Diagram == nil || len(b.Diagram.RuntimeNotesV1) == 0 {
			continue
		}
		b = cloneAtomicDiagramPatchBlock(b)
		if _, err := nativeDiagramNotePoints(&b, view, rows, true); err != nil {
			failures = append(failures, fmt.Sprintf("block %q: %v", b.ID, err))
			continue
		}
		changed[i] = b
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	for i, b := range changed {
		doc.Blocks[i] = b
	}
	return nil
}

func preCheckNativeDiagramNotes(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) []emitFixHint {
	if err := materializeNativeDiagramNotes(doc, view, runtimeDiagramRelationsForContext(ctx)); err != nil {
		return []emitFixHint{{Field: "diagram.runtime_notes_v1", ExpectedShape: "Select a current native business selector, start/end, exact owner node and existing Note body_line. Use replace_blocks to correct owner/placement; preserve other content. Omit an optional selector when its owner or span is unavailable, leaving an honest unproven model note.", Reason: err.Error(), ForceHard: true, HardSignal: preEmitHardSignalExactReceiptBinding}}
	}
	return nil
}
