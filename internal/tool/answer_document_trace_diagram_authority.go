package tool

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

type reportLocalRuntimeTemporalEdgeSet struct {
	counts map[string]int
}

// ReportLocalRuntimeTemporalDiagramOwnedBlockIDs exposes the exact block-local
// routing decision to post-finalizer contract validation. Pre-emit and
// post-emit must consume this same decision; otherwise a diagram accepted as
// runtime temporal in the tool can be reclassified as source invocation by the
// orchestrator a few milliseconds later.
func ReportLocalRuntimeTemporalDiagramOwnedBlockIDs(
	ctx *types.BusContext,
	doc *types.AnswerDocumentV2,
) map[string]bool {
	if ctx == nil || doc == nil {
		return nil
	}
	owned, hints := preCheckReportLocalRuntimeTemporalDiagramAuthority(doc, newPreEmitCheckContext(ctx))
	if len(hints) > 0 || len(owned) == 0 {
		return nil
	}
	out := make(map[string]bool, len(owned))
	for index := range owned {
		if index < 0 || index >= len(doc.Blocks) {
			continue
		}
		out[strings.TrimSpace(doc.Blocks[index].ID)] = true
	}
	return out
}

// DiagramCallEdgeEvidenceMismatchesWithRuntimeContext is the post-finalizer
// counterpart of preCheckDiagramCallEdgeEvidenceAlignment. It removes only
// blocks already bound to report-local runtime temporal authority, then runs
// the unchanged source matcher over every remaining block.
func DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(
	ctx *types.BusContext,
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	evidence []types.EvidenceItem,
) []DiagramCallEdgeEvidenceMismatch {
	// B945: pre-emit and post-finalizer validation share the exact same
	// dispatch-scoped registered-export receipt. Only a model-authored,
	// distinct-node standalone registration anchor is removed from the ordinary
	// physical source-edge view. Collapsed nodes, Mermaid arrows, partial pairs,
	// and every ordinary call remain visible to the unchanged evidence matcher.
	doc = documentWithoutStandaloneSemanticHandoffAnchors(doc, ctx)
	stagePrecedence := diagramVerifiedReadModeStagePrecedence(ctx, view)
	owned := ReportLocalRuntimeTemporalDiagramOwnedBlockIDs(ctx, doc)
	if len(owned) == 0 {
		// Keep post-finalizer validation on the same exact relation evidence
		// surface as pre-emit validation. This projection is inert unless the
		// model authored a type_relation anchor, and it admits only coverage-
		// gate-eligible provider rows; it never adds or rewrites a visible edge.
		evidence = preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, evidence)
		mismatches := DiagramCallEdgeEvidenceMismatches(doc, view, evidence, stagePrecedence)
		if len(mismatches) == 0 {
			mismatches = DiagramRequestedStagePrecedenceSpineMismatches(doc, view, stagePrecedence)
		}
		return mismatches
	}
	copyDoc := *doc
	copyDoc.Blocks = make([]types.AnswerBlock, 0, len(doc.Blocks)-len(owned))
	for _, block := range doc.Blocks {
		if !owned[strings.TrimSpace(block.ID)] {
			copyDoc.Blocks = append(copyDoc.Blocks, block)
		}
	}
	evidence = preEmitEvidenceWithExactTypedDiagramRelations(&copyDoc, ctx, evidence)
	mismatches := DiagramCallEdgeEvidenceMismatches(&copyDoc, view, evidence, stagePrecedence)
	if len(mismatches) == 0 {
		mismatches = DiagramRequestedStagePrecedenceSpineMismatches(&copyDoc, view, stagePrecedence)
	}
	return mismatches
}

// preCheckReportLocalRuntimeTemporalDiagramAuthority binds an optional frame
// diagram to one trace_query result's own complete typed temporal rows. It does
// not use the report-wide QF family or ANY-sticky session authority: those are
// too coarse to distinguish a runtime frame diagram from a sibling source
// relation diagram. The returned block indices are safe to exclude from the
// source-call gate; every other block remains subject to that gate unchanged.
func preCheckReportLocalRuntimeTemporalDiagramAuthority(
	doc *types.AnswerDocumentV2,
	pctx *preEmitCheckContext,
) (map[int]bool, []emitFixHint) {
	if doc == nil || pctx == nil || pctx.ctx == nil {
		return nil, nil
	}
	edgeSets := reportLocalRuntimeTemporalEdgeSets(pctx.ctx)
	if len(edgeSets) == 0 {
		return nil, nil
	}

	owned := map[int]bool{}
	edgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		if len(edges) == 0 {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		bodyCounts := make(map[string]int, len(edges))
		for _, edge := range edges {
			from := reportLocalRuntimeTemporalEndpoint(edge.From, labels)
			to := reportLocalRuntimeTemporalEndpoint(edge.To, labels)
			bodyCounts[diagramEvidenceEdgeKey(from, to)]++
		}
		matched := false
		for _, edgeSet := range edgeSets {
			if reportLocalRuntimeTemporalEdgeSubset(bodyCounts, edgeSet.counts) {
				matched = true
				break
			}
		}
		if !matched {
			if reportLocalRuntimeTemporalBlockDeclaresTemporal(block) {
				return nil, []emitFixHint{reportLocalRuntimeTemporalDiagramHint(
					block.ID,
					"visible temporal endpoints are not a subset of any one complete report-local typed frame-temporal result",
				)}
			}
			continue
		}

		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, edgeBlockCounts)
		if issue := reportLocalRuntimeTemporalAnchorIssue(edges, anchors); issue != "" {
			return nil, []emitFixHint{reportLocalRuntimeTemporalDiagramHint(block.ID, issue)}
		}
		owned[i] = true
	}
	return owned, nil
}

func reportLocalRuntimeTemporalEdgeSets(ctx *types.BusContext) []reportLocalRuntimeTemporalEdgeSet {
	if ctx == nil {
		return nil
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, 1)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	if ctx.Mutable != nil {
		results = append(results, ctx.Mutable.DispatchToolResults()...)
		results = append(results, ctx.Mutable.SystemTraceSupplementResults()...)
	}
	out := make([]reportLocalRuntimeTemporalEdgeSet, 0, len(results))
	seenSets := map[string]bool{}
	for _, result := range results {
		authority := result.TraceEvidenceAuthority
		if !result.Success || authority == nil || authority.FrameFlowEdgeCount <= 0 ||
			strings.TrimSpace(authority.FrameFlowCausalConclusion) != "unproven" ||
			strings.TrimSpace(authority.FrameFlowRelationAuthority) != "temporal_sequence" {
			continue
		}
		counts := map[string]int{}
		seenRows := map[string]bool{}
		for _, record := range result.Observations {
			if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
				!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
				record.GroundingPolicy != types.ClaimGroundingHard ||
				strings.TrimSpace(record.Predicate) != "frame_temporal_sequence" ||
				!strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "evidence_fact:") {
				continue
			}
			rowKey := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%.9f\x00%.9f",
				strings.TrimSpace(record.SourceRef.CaptureIdentityPath),
				strings.TrimSpace(record.SourceRef.Path),
				strings.TrimSpace(record.Subject),
				strings.TrimSpace(record.Object),
				record.Span.LineStart,
				record.Span.LineEnd,
				record.Span.StartTs,
				record.Span.EndTs,
			)
			if seenRows[rowKey] {
				continue
			}
			seenRows[rowKey] = true
			from, to := strings.TrimSpace(record.Subject), strings.TrimSpace(record.Object)
			if from == "" || to == "" {
				continue
			}
			counts[diagramEvidenceEdgeKey(from, to)]++
		}
		rowCount := 0
		keys := make([]string, 0, len(counts))
		for key, count := range counts {
			rowCount += count
			keys = append(keys, fmt.Sprintf("%s=%d", key, count))
		}
		if rowCount != authority.FrameFlowEdgeCount {
			continue
		}
		// The identity need only deduplicate repeated publication. Map iteration
		// order is irrelevant to authority, so sort the multiset entries first.
		sort.Strings(keys)
		setKey := strings.Join(keys, "\x01")
		if seenSets[setKey] {
			continue
		}
		seenSets[setKey] = true
		out = append(out, reportLocalRuntimeTemporalEdgeSet{counts: counts})
	}
	return out
}

func reportLocalRuntimeTemporalEndpoint(alias string, labels map[string]string) string {
	alias = strings.TrimSpace(alias)
	if label := strings.TrimSpace(labels[strings.ToLower(alias)]); label != "" {
		return html.UnescapeString(label)
	}
	return html.UnescapeString(alias)
}

func reportLocalRuntimeTemporalEdgeSubset(body, typed map[string]int) bool {
	if len(body) == 0 || len(typed) == 0 {
		return false
	}
	for key, count := range body {
		if count <= 0 || typed[key] < count {
			return false
		}
	}
	return true
}

func reportLocalRuntimeTemporalEdgeSetEqual(left, right map[string]int) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for key, count := range left {
		if count <= 0 || right[key] != count {
			return false
		}
	}
	return true
}

// normalizeExactReportLocalRuntimeTemporalEdgeAnchors restores only omitted
// metadata for a complete report-local temporal diagram. The visible diagram
// is never edited. Recovery is deliberately narrower than the authority
// validator: the full endpoint occurrence multiset must equal one complete
// typed result, every visible edge must carry the capsule's exact canonical
// temporal label, and the block must have zero local or sibling-carried
// anchors. Subsets, mixed/source diagrams, translated or causal labels, and
// partial/conflicting metadata stay fail-closed for the normal validator.
func normalizeExactReportLocalRuntimeTemporalEdgeAnchors(
	doc *types.AnswerDocumentV2,
	pctx *preEmitCheckContext,
) int {
	if doc == nil || pctx == nil || pctx.ctx == nil {
		return 0
	}
	edgeSets := reportLocalRuntimeTemporalEdgeSets(pctx.ctx)
	if len(edgeSets) == 0 {
		return 0
	}
	edgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	repaired := 0
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil ||
			len(diagramEvidenceEffectiveAnchorsForBlock(doc, i, edgeBlockCounts)) != 0 {
			continue
		}
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		if len(edges) == 0 {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		bodyCounts := make(map[string]int, len(edges))
		canonical := true
		for _, edge := range edges {
			if strings.TrimSpace(edge.Label) != types.RuntimeTraceTemporalDiagramEdgeLabel {
				canonical = false
				break
			}
			from := reportLocalRuntimeTemporalEndpoint(edge.From, labels)
			to := reportLocalRuntimeTemporalEndpoint(edge.To, labels)
			bodyCounts[diagramEvidenceEdgeKey(from, to)]++
		}
		if !canonical {
			continue
		}
		matched := false
		for _, edgeSet := range edgeSets {
			if reportLocalRuntimeTemporalEdgeSetEqual(bodyCounts, edgeSet.counts) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		block.EdgeAnchors = make([]types.DiagramEdgeAnchor, 0, len(edges))
		for _, edge := range edges {
			block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{
				FromNode:     strings.TrimSpace(edge.From),
				ToNode:       strings.TrimSpace(edge.To),
				RelationKind: types.DiagramRelTemporal,
				ClaimForm:    types.ClaimFormForRelation(types.DiagramRelTemporal),
			})
			repaired++
		}
	}
	return repaired
}

func reportLocalRuntimeTemporalBlockDeclaresTemporal(block *types.AnswerBlock) bool {
	if block == nil {
		return false
	}
	for _, anchor := range block.EdgeAnchors {
		if diagramAnchorRelation(anchor) == types.DiagramRelTemporal {
			return true
		}
	}
	return false
}

func reportLocalRuntimeTemporalAnchorIssue(edges []mermaidcompat.Edge, anchors []types.DiagramEdgeAnchor) string {
	bodyCounts := map[string]int{}
	for _, edge := range edges {
		bodyCounts[diagramEvidenceEdgeKey(edge.From, edge.To)]++
	}
	anchorCounts := map[string]int{}
	for _, anchor := range anchors {
		key := diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)
		if bodyCounts[key] == 0 {
			return fmt.Sprintf("typed anchor %s->%s has no matching visible arrow", anchor.FromNode, anchor.ToNode)
		}
		if diagramAnchorRelation(anchor) != types.DiagramRelTemporal {
			return fmt.Sprintf("visible runtime-temporal arrow %s->%s has relation_kind=%s instead of temporal",
				anchor.FromNode, anchor.ToNode, firstDiagramRelationName(diagramAnchorRelation(anchor)))
		}
		anchorCounts[key]++
	}
	for key, count := range bodyCounts {
		if anchorCounts[key] != count {
			return fmt.Sprintf("visible runtime-temporal arrow occurrence count=%d but temporal anchor count=%d for %s",
				count, anchorCounts[key], strings.ReplaceAll(key, "\x00", "->"))
		}
	}
	return ""
}

func firstDiagramRelationName(relation types.DiagramRelationKind) string {
	if relation == types.DiagramRelUnknown {
		return "missing"
	}
	return string(relation)
}

func reportLocalRuntimeTemporalDiagramHint(blockID, issue string) emitFixHint {
	return emitFixHint{
		Field:               "blocks[].edge_anchors[] AND blocks[kind=diagram].diagram.body",
		HardSignal:          preEmitHardSignalTypedCallEdgeEvidence,
		OffendingBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
		ExpectedShape: types.RuntimeTraceTemporalDiagramRelationContract +
			" Keep only a faithful subset of one report-local typed temporal edge set, with one relation_kind=temporal anchor per visible arrow occurrence; do not borrow endpoints across trace queries or source-code evidence.",
		Reason: fmt.Sprintf("block=%q: %s. Runtime temporal ordering has its own typed authority and must not be upgraded into or rejected as a source-code invocation.",
			blockID, issue),
	}
}

// preCheckRuntimeTraceTemporalDiagramAuthority closes the structured diagram
// half of an exact runtime authority boundary. It activates only when this
// report has positive unproven frame-temporal edges and no typed causal row in
// any consumed trace result. A mixed report with a proved wakeup/IPC/lock/flow
// row therefore stays out of this report-wide fallback instead of being
// downgraded by an unrelated earlier frame probe.
//
// The check reads only TraceEvidenceAuthority, parsed Mermaid edges, and the
// schema-valid relation_kind enum. It never scans the request, reasoning, edge
// messages, block prose, or rendered answer for causal words.
func preCheckRuntimeTraceTemporalDiagramAuthority(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if doc == nil || view == nil || view.Family != types.QFRootCauseTrace || pctx == nil ||
		!runtimeTraceOnlyUnprovenFrameTemporalAuthority(pctx.ctx) {
		return nil
	}

	bodyEdgeBlockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	var mismatches []DiagramCallEdgeEvidenceMismatch
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		edges := mermaidcompat.ParseEdges(block.Diagram.Body)
		if len(edges) == 0 {
			continue
		}
		relations := diagramTypedAnchorRelationSet(
			diagramEvidenceEffectiveAnchorsForBlock(doc, i, bodyEdgeBlockCounts),
		)
		for _, edge := range edges {
			key := diagramEvidenceEdgeKey(edge.From, edge.To)
			typed := relations[key]
			if len(typed) == 1 && (typed[types.DiagramRelTemporal] || typed[types.DiagramRelObserve]) {
				continue
			}
			mismatches = append(mismatches, DiagramCallEdgeEvidenceMismatch{
				BlockID:  block.ID,
				Issue:    "runtime_temporal_edge_owner_required",
				FromNode: strings.TrimSpace(edge.From),
				ToNode:   strings.TrimSpace(edge.To),
				Relation: firstDiagramRelation(typed),
			})
		}
	}
	if len(mismatches) == 0 {
		return nil
	}

	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		relation := string(mismatch.Relation)
		if relation == "" {
			relation = "missing"
		}
		parts = append(parts, fmt.Sprintf("block=%q edge=%s->%s relation_kind=%s",
			mismatch.BlockID, mismatch.FromNode, mismatch.ToNode, relation))
	}
	return []emitFixHint{{
		Field:                       "blocks[kind=diagram].diagram.body AND blocks[].edge_anchors[]",
		HardSignal:                  preEmitHardSignalTypedCallEdgeEvidence,
		OffendingBlockKinds:         preEmitDiagramMismatchBlockKinds(doc, mismatches),
		ExpectedShape:               types.RuntimeTraceTemporalDiagramRelationContract + " Repair only these visible pairs: " + strings.Join(parts, "; ") + diagramRelationSurgicalRepairInstruction,
		Reason:                      "the report-local typed trace authority contains measured temporal adjacency but zero typed causal rows. A call/callback/other causal relation enum or an unowned arrow would upgrade ordering into causality; relation_kind=temporal preserves frame ordering, while a separately grounded relation_kind=observe remains background evidence rather than a cause.",
		DiagramRelationFailurePairs: diagramRelationFailurePairFingerprints(mismatches),
	}}
}

func runtimeTraceOnlyUnprovenFrameTemporalAuthority(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, 1)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	// Keep the raw per-dispatch authority rows as well as the ledger merge.
	// The ledger intentionally coalesces repeated trace-query results for
	// publication, while this ceiling must notice any proved typed causal row
	// before applying a report-wide temporal-only fallback.
	if ctx.Mutable != nil {
		results = append(results, ctx.Mutable.DispatchToolResults()...)
		results = append(results, ctx.Mutable.SystemTraceSupplementResults()...)
	}
	hasUnprovenTemporalFrameEdge := false
	for _, result := range results {
		authority := result.TraceEvidenceAuthority
		if authority == nil {
			continue
		}
		if authority.TypedCausalRowCount > 0 || strings.TrimSpace(authority.CausalConclusion) == "bounded_by_typed_rows" {
			return false
		}
		frameConclusion := strings.TrimSpace(authority.FrameFlowCausalConclusion)
		if frameConclusion != "" && frameConclusion != "unproven" {
			return false
		}
		if frameConclusion == "unproven" &&
			strings.TrimSpace(authority.FrameFlowRelationAuthority) == "temporal_sequence" &&
			authority.FrameFlowEdgeCount > 0 {
			hasUnprovenTemporalFrameEdge = true
		}
	}
	return hasUnprovenTemporalFrameEdge
}

func firstDiagramRelation(relations map[types.DiagramRelationKind]bool) types.DiagramRelationKind {
	for _, relation := range types.AllDiagramRelationKinds() {
		if relations[relation] {
			return relation
		}
	}
	return types.DiagramRelUnknown
}
