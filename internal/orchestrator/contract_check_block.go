package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// V2 block-only carrier validators (B4 落地 — block_only_carrier.md
// §5.4). 4 validators raise SOFT-by-default ViolationKind values
// when the LLM's emitted AnswerDocumentV2 fails to satisfy the
// AnswerSemanticView contract.
//
// Default classification: SOFT (telemetry only) during B4-B5; B6
// promotes to STRICT once V2 is the default carrier. Operators
// override via pipeline_contract_strict_kinds yaml field.
//
// Per the precise-signals-for-hard-gates red line (R2), all 4
// validators read ONLY:
//   - doc.Blocks[i].Kind / .ID / .FacetIDs / .SurfaceRole / .ClaimUses
//   - view.RequiredBlocks[i].Kind / .MinCount / .MaxCount / .Required /
//     .FacetIDs / .AcceptableClaimForms / .SurfaceRoleHint
//   - view.UncertaintyRules[i].TriggerFacet / .ExpectedBlockKind
//   - view.DiagramPlan.Required / .Kind
//   - view.FacetCoverage.Required[i].Kind
// — i.e. typed enum + verbatim string match only. Zero ranker scores
// or fuzzy heuristics.

// validateRequiredBlockCoverage checks each Required=true entry in
// view.RequiredBlocks against doc.Blocks. Counts blocks that match
// Kind, raises ViolBlockCoverageMissing when:
//   - actual count < req.MinCount, OR
//   - req.MaxCount > 0 AND actual count > req.MaxCount
//
// Failure-mode summary: "LLM emitted V2 doc but skipped a required
// block kind OR over-filled a capped one."
func validateRequiredBlockCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	var out []types.Violation
	counts := make(map[types.AnswerBlockKind]int, len(doc.Blocks))
	for _, b := range doc.Blocks {
		counts[b.Kind]++
	}
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		got := counts[req.Kind]
		if got < req.MinCount {
			out = append(out, types.Violation{
				Kind: types.ViolBlockCoverageMissing,
				Detail: fmt.Sprintf(
					"required block kind=%s appears %d time(s) in answer; the family contract requires at least %d",
					req.Kind, got, req.MinCount),
				Repair: fmt.Sprintf(
					"emit at least %d block(s) of kind=%s. Per the rationale: %s",
					req.MinCount, req.Kind, req.Rationale),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_block_coverage",
					Reason:     "required block kind under-emitted",
					Confidence: 0.85,
				},
				Stage: string(types.StageFinalize),
			})
			continue
		}
		if req.MaxCount > 0 && got > req.MaxCount {
			out = append(out, types.Violation{
				Kind: types.ViolBlockCoverageMissing,
				Detail: fmt.Sprintf(
					"required block kind=%s appears %d time(s); the family contract caps it at %d",
					req.Kind, got, req.MaxCount),
				Repair: fmt.Sprintf(
					"reduce kind=%s blocks to at most %d. Per the rationale: %s",
					req.Kind, req.MaxCount, req.Rationale),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_block_coverage",
					Reason:     "required block kind over-emitted",
					Confidence: 0.7,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	return out
}

// validatePrincipalClaimUse checks that every block whose
// SurfaceRole is "principal" (or whose corresponding BlockRequirement
// hint is SurfacePrincipal) carries at least one RenderedClaimUse —
// at the block level OR on at least one item — when the matching
// BlockRequirement's AcceptableClaimForms is non-empty.
//
// Failure-mode summary: "principal block content has no claim_use
// annotation but its family requires one."
func validatePrincipalClaimUse(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	// Build req map by Kind (first match wins; multiple requirements
	// of same Kind take the strictest by ANDing AcceptableClaimForms,
	// which would only happen if a family declared duplicate rows —
	// not currently the case).
	reqByKind := make(map[types.AnswerBlockKind]types.BlockRequirement, len(view.RequiredBlocks))
	for _, r := range view.RequiredBlocks {
		if _, ok := reqByKind[r.Kind]; !ok {
			reqByKind[r.Kind] = r
		}
	}
	var out []types.Violation
	for _, b := range doc.Blocks {
		req, ok := reqByKind[b.Kind]
		if !ok {
			continue
		}
		if len(req.AcceptableClaimForms) == 0 {
			// no claim form check requested
			continue
		}
		isPrincipal := b.SurfaceRole == types.SurfacePrincipal ||
			(b.SurfaceRole == "" && req.SurfaceRoleHint == types.SurfacePrincipal)
		if !isPrincipal {
			continue
		}
		if blockHasClaimUse(b) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolPrincipalClaimUseMissing,
			Detail: fmt.Sprintf(
				"principal block id=%q kind=%s has no claim_use; family requires one of %v",
				b.ID, b.Kind, formNames(req.AcceptableClaimForms)),
			Repair: fmt.Sprintf(
				"emit claim_use on the block (or on at least one item) declaring claim_form ∈ %v so the validator can match the principal payload to its evidence shape",
				formNames(req.AcceptableClaimForms)),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_claim_use",
				Reason:     "principal block lacks claim_use annotation",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validateDiagramEdgeSupport checks each BlockDiagram in doc.Blocks
// against view.DiagramPlan: when DiagramPlan.Required, the diagram
// must exist and its Kind should match (when both view and block
// declare a Kind). On top of those structural checks (R4.3 deepening
// 2026-05-04), it now performs per-edge semantic grounding:
// every edge parsed from the Mermaid body must have BOTH endpoints
// resolvable to (a) a node declared in the same body, OR (b) a
// substring of an Item.Label / Block.Title in the same answer doc,
// OR (c) a substring of any RenderedClaimUse FacetID/EvidenceID in
// the answer. Edges whose endpoints fail every grounding fork are
// surfaced as ViolDiagramEdgeUnsupported with Detail listing each
// unsupported (from -> to) pair so the LLM can repair them on retry.
//
// Failure-mode summary: "V2 doc emitted a diagram block but (a) its
// declared Kind disagrees with the family's DiagramFacetGraph kind,
// (b) the family required a diagram and none was emitted, OR (c)
// the diagram contains hallucinated edges whose endpoints have no
// grounded basis in the rest of the answer."
func validateDiagramEdgeSupport(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.DiagramPlan == nil {
		return nil
	}
	plan := view.DiagramPlan
	var diagramBlock *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockDiagram {
			diagramBlock = &doc.Blocks[i]
			break
		}
	}
	if plan.Required && diagramBlock == nil {
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"family contract requires a diagram of kind=%s but no BlockDiagram is present in the answer",
				plan.Kind),
			Repair: fmt.Sprintf(
				"emit a BlockDiagram (kind=%s) covering node facets %v and edge facets %v",
				plan.Kind, plan.NodeFacets, plan.EdgeFacets),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_block",
				Reason:     "required diagram absent",
				Confidence: 0.85,
			},
			Stage: string(types.StageFinalize),
		}}
	}
	if diagramBlock == nil || diagramBlock.Diagram == nil {
		return nil
	}
	if plan.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != plan.Kind {
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q declared kind=%s but family contract expects %s",
				diagramBlock.ID, diagramBlock.Diagram.Kind, plan.Kind),
			Repair: fmt.Sprintf(
				"set diagram.kind=%s OR drop the diagram if the family contract should be relaxed",
				plan.Kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_kind",
				Reason:     "diagram kind mismatch",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		}}
	}

	// R4.3 deepening — per-edge endpoint grounding. Skip when the
	// diagram has no body (defensive; the schema validator should
	// reject empty Body, but a downstream consumer must not panic).
	body := diagramBlock.Diagram.Body
	if strings.TrimSpace(body) == "" {
		return nil
	}
	edges := parseMermaidEdges(body)
	if len(edges) == 0 {
		return nil
	}
	support := buildDiagramSupportTokens(doc, diagramBlock)
	var unsupported []mermaidEdge
	for _, e := range edges {
		if !diagramTokenSupported(e.from, support) || !diagramTokenSupported(e.to, support) {
			unsupported = append(unsupported, e)
		}
	}
	if len(unsupported) > 0 {
		// Aggregate as a single violation — the LLM should see the full
		// list of broken edges in one repair pass instead of being
		// re-prompted per edge. When Layer 1 fails, skip Layer 2: ask
		// the LLM to ground endpoints first, then re-check relations on
		// the next attempt.
		pairs := make([]string, 0, len(unsupported))
		for _, e := range unsupported {
			pairs = append(pairs, fmt.Sprintf("%s -> %s", e.from, e.to))
		}
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q has %d edge(s) whose endpoints are not grounded in any item, block title, claim_use annotation, or declared node label: [%s]",
				diagramBlock.ID, len(unsupported), strings.Join(pairs, ", ")),
			Repair: "for each listed edge, either (a) declare its endpoints as labelled nodes in the same Mermaid body (e.g. A[\"Label A\"] --> B[\"Label B\"]), (b) name the endpoints in an item label or block title in this answer, or (c) drop the edge if it represents an inference not backed by any grounded claim.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_edges",
				Reason:     "edge endpoints lack grounding in the rest of the answer",
				Confidence: 0.65,
			},
			Stage: string(types.StageFinalize),
		}}
	}
	// Layer 2 (Phase 3-C5) — relation legality. For each labelled
	// edge whose label resolves to a typed DiagramRelationKind with
	// an edge-level ClaimForm, require a claim_use in this document
	// that (a) matches the ClaimForm and (b) anchors the (from, to)
	// endpoints. Plus each DiagramFacetGraph.EdgeRelations contract
	// with Min>0 must be met by at least Min labelled edges.
	return validateDiagramRelationLegality(doc, view, diagramBlock, edges)
}

// validateDiagramRelationLegality is the Phase 3-C5 Layer-2 check.
// Pre-condition: every edge endpoint already passes Layer 1
// grounding; this function only inspects label-based relation
// legality. Returns one aggregated violation for missing anchored
// claim_uses + one per EdgeRelations.Min shortfall.
func validateDiagramRelationLegality(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	diagramBlock *types.AnswerBlock,
	edges []mermaidEdge,
) []types.Violation {
	plan := view.DiagramPlan
	anchoredIndex := buildClaimUseEdgeAnchorIndex(doc)

	type missingAnchor struct {
		from, to, label string
		want            types.ClaimForm
	}
	var missing []missingAnchor
	relCounts := make(map[types.DiagramRelationKind]int)
	for _, e := range edges {
		rel := types.InferRelationFromLabel(e.label)
		if rel == types.DiagramRelUnknown {
			continue
		}
		relCounts[rel]++
		expected := types.ClaimFormForRelation(rel)
		if expected == types.ClaimUnknown {
			continue
		}
		if !claimUseAnchorsEdge(anchoredIndex, e.from, e.to, expected) {
			missing = append(missing, missingAnchor{
				from: e.from, to: e.to, label: e.label, want: expected,
			})
		}
	}

	var violations []types.Violation
	if len(missing) > 0 {
		details := make([]string, 0, len(missing))
		for _, m := range missing {
			details = append(details, fmt.Sprintf("%s --|%s|--> %s (need claim_use claim_form=%s anchored to from_node=%q to_node=%q)",
				m.from, m.label, m.to, m.want, m.from, m.to))
		}
		violations = append(violations, types.Violation{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q has %d labelled edge(s) lacking a typed claim_use anchored to (from_node, to_node) with the expected claim_form: [%s]",
				diagramBlock.ID, len(missing), strings.Join(details, "; ")),
			Repair: "for each listed edge, attach a claim_use to a block / item with claim_form set to the listed value AND from_node / to_node set to the verbatim node identifiers. Alternatively, drop the edge label if the relation isn't supported by typed evidence.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_edges",
				Reason:     "labelled edges lack typed claim_use anchor",
				Confidence: 0.6,
			},
			Stage: string(types.StageFinalize),
		})
	}
	for _, contract := range plan.EdgeRelations {
		if contract.Min <= 0 {
			continue
		}
		if relCounts[contract.Kind] >= contract.Min {
			continue
		}
		violations = append(violations, types.Violation{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q expected at least %d edge(s) of relation kind=%s but found %d (label the edges with vocabulary the relation kind recognises)",
				diagramBlock.ID, contract.Min, contract.Kind, relCounts[contract.Kind]),
			Repair: fmt.Sprintf(
				"add at least %d Mermaid edge(s) whose label resolves to relation kind=%s (see the %q section above for the recognised label vocabulary); each such edge should be supported by a claim_use with claim_form=%s anchored to its (from_node, to_node)",
				contract.Min-relCounts[contract.Kind], contract.Kind, types.SectionDiagramEdgeLabelVocabulary, contract.ClaimForm),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_edges",
				Reason:     "minimum-count contract for typed relation not satisfied",
				Confidence: 0.55,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return violations
}

// buildClaimUseEdgeAnchorIndex collects every DiagramEdgeAnchor
// in doc, keyed by (lower(FromNode), lower(ToNode), ClaimForm).
// Multiple anchors sharing the same key collapse — the index
// stores a presence bit, not the entry itself.
//
// Phase 1-B source-fix (V2 runtime eval followup, 2026-05-04):
// Pre-fix, this index walked block-level claim_uses[] and
// item-level claim_use[].FromNode/ToNode (when those fields lived
// inside RenderedClaimUse). The u3a-1 forensic showed that schema
// density caused LLMs to mis-fill sibling fields. Edge anchors
// now live on AnswerBlock.EdgeAnchors[] as a typed array — the
// index walks that array directly.
func buildClaimUseEdgeAnchorIndex(doc *types.AnswerDocumentV2) map[claimUseEdgeKey]struct{} {
	idx := make(map[claimUseEdgeKey]struct{})
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		for j := range b.EdgeAnchors {
			a := &b.EdgeAnchors[j]
			if !a.HasEdgeAnchor() {
				continue
			}
			idx[claimUseEdgeKey{
				from:  strings.ToLower(strings.TrimSpace(a.FromNode)),
				to:    strings.ToLower(strings.TrimSpace(a.ToNode)),
				claim: a.ClaimForm,
			}] = struct{}{}
		}
	}
	return idx
}

type claimUseEdgeKey struct {
	from, to string
	claim    types.ClaimForm
}

// claimUseAnchorsEdge reports whether the index contains a
// DiagramEdgeAnchor whose (FromNode, ToNode, ClaimForm) matches
// the edge. Matching is case-folded on the node identifiers.
func claimUseAnchorsEdge(idx map[claimUseEdgeKey]struct{}, from, to string, want types.ClaimForm) bool {
	_, ok := idx[claimUseEdgeKey{
		from:  strings.ToLower(strings.TrimSpace(from)),
		to:    strings.ToLower(strings.TrimSpace(to)),
		claim: want,
	}]
	return ok
}

// mermaidEdge is the (from, to[, label]) tuple extracted from one
// edge declaration in a Mermaid body. The from / to fields are raw
// identifier / label substrings as they appear in source — matching
// is case-folded downstream.
//
// The label field captures the verbatim relation marker the LLM put
// on the edge:
//   - flowchart `A -->|cond| B`            → label = "cond"
//   - flowchart `A -- text --> B`          → label = "text" (best-effort)
//   - sequenceDiagram `A->>B: handleReq`   → label = "handleReq"
//
// Empty label means "unlabelled edge" (or a label form the parser
// could not extract). Phase 3 §6.2.2's InferRelationFromLabel
// resolves the label to a typed DiagramRelationKind; an empty label
// resolves to DiagramRelUnknown — a legitimate state the validator
// treats as "label-free edge" rather than a violation.
type mermaidEdge struct {
	from  string
	to    string
	label string
}

// parseMermaidEdges scans a Mermaid body and returns every edge
// declaration it can recognise. Supported shapes (covers the bodies
// the LLM emits for flowchart / sequenceDiagram / call_dag /
// architecture):
//   - flowchart:    A --> B  /  A --- B  /  A ==> B  /  A -.-> B
//                   A -->|label| B  /  A -- text --> B
//   - sequence:     A->>B: msg  /  A-->>B: reply  /  A->B: txt
//
// Unrecognised lines (subgraph headers, classDefs, %%comments) are
// silently skipped — the goal is best-effort coverage of the common
// edge syntaxes, not an exhaustive Mermaid parser.
func parseMermaidEdges(body string) []mermaidEdge {
	var edges []mermaidEdge
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "click") {
			continue
		}
		// SequenceDiagram (and noteOver) lines carry a trailing
		// `: message` payload that is prose, not a node identifier.
		// Capture it as the edge's label BEFORE stripping so the
		// relation-aware validator can read the message verbatim.
		var seqLabel string
		if idx := strings.Index(line, ":"); idx > 0 {
			seqLabel = strings.TrimSpace(line[idx+1:])
			line = strings.TrimSpace(line[:idx])
		}
		from, to, label, ok := splitMermaidEdgeLine(line)
		if !ok {
			continue
		}
		if label == "" {
			label = seqLabel
		}
		from = stripMermaidNodeShape(from)
		to = stripMermaidNodeShape(to)
		if from == "" || to == "" {
			continue
		}
		edges = append(edges, mermaidEdge{from: from, to: to, label: label})
	}
	return edges
}

// splitMermaidEdgeLine attempts to split a single Mermaid statement
// into (from, to) on the first arrow operator it finds. Returns
// ok=false when no arrow appears (declaration-only lines).
//
// Arrow operators (longest-first match prevents `--` from clipping
// `-->`):
//
//	-->>  -.->  -->  -->|...|  ==>  ==>|...|
//	-.->|...|  --|text|-->  -- text -->
//	---  ->>  -->>
func splitMermaidEdgeLine(line string) (string, string, string, bool) {
	// Capture the FIRST `|inline label|` block before stripping —
	// Mermaid flowchart's `A -->|cond| B` puts the relation marker
	// between pipes. Subsequent `|...|` groups (rare) are stripped
	// without capture; first wins.
	var pipeLabel string
	for {
		i := strings.Index(line, "|")
		if i < 0 {
			break
		}
		j := strings.Index(line[i+1:], "|")
		if j < 0 {
			break
		}
		if pipeLabel == "" {
			pipeLabel = strings.TrimSpace(line[i+1 : i+1+j])
		}
		line = line[:i] + " " + line[i+1+j+1:]
	}
	// Operator candidates ordered longest-first.
	operators := []string{
		"-->>", "-.->", "-->", "==>", "->>", "---", "==", "->",
	}
	for _, op := range operators {
		idx := strings.Index(line, op)
		if idx < 0 {
			continue
		}
		from := strings.TrimSpace(line[:idx])
		to := strings.TrimSpace(line[idx+len(op):])
		// `---` matches inside identifiers like `a---b`; require
		// the from / to halves to look like identifier / label.
		if from == "" || to == "" {
			continue
		}
		// Drop trailing punctuation that follows the to token (";",
		// ",", trailing class binding `:::cls`).
		to = strings.SplitN(to, ";", 2)[0]
		to = strings.SplitN(to, ":::", 2)[0]
		to = strings.TrimSpace(to)
		// `-- text -->` shape: after stripping the `|...|` block we
		// may be left with `A   B` separated by inner text — the
		// arrow we found could be the inner `--`. Heuristic: if both
		// sides still contain an arrow, pick the rightmost one.
		if strings.Contains(to, "-->") || strings.Contains(to, "==>") || strings.Contains(to, "->>") {
			// recurse on the rightmost portion; preserve any
			// pipe-label captured in the outer call (the recursion
			// only re-parses the trailing `to` segment).
			f2, t2, innerLabel, ok := splitMermaidEdgeLine(to)
			if ok {
				if innerLabel != "" {
					return f2, t2, innerLabel, true
				}
				return f2, t2, pipeLabel, true
			}
		}
		return from, to, pipeLabel, true
	}
	return "", "", "", false
}

// stripMermaidNodeShape collapses a node declaration with a shape
// wrapper to its identifier. Examples:
//
//	A[Label]    -> A
//	A("Label")  -> A
//	A((Round))  -> A
//	A>flag]     -> A
//	A{rhombus}  -> A
//	A&fa:fa-x   -> A (cosmetic prefix, drop the stuff after)
//
// When no shape wrapper is present, the input is returned trimmed.
// The label itself is intentionally discarded — the supported-token
// set carries label substrings from the body separately so matching
// remains label-aware.
func stripMermaidNodeShape(tok string) string {
	t := strings.TrimSpace(tok)
	if t == "" {
		return ""
	}
	// Strip optional class binding suffix `:::clsName`
	if i := strings.Index(t, ":::"); i > 0 {
		t = strings.TrimSpace(t[:i])
	}
	// Find the first shape opener and cut at it: [, (, {, >
	cutAt := -1
	for i, r := range t {
		if r == '[' || r == '(' || r == '{' || r == '>' {
			cutAt = i
			break
		}
	}
	if cutAt > 0 {
		t = strings.TrimSpace(t[:cutAt])
	}
	// Drop leading `&` cosmetic prefix
	t = strings.TrimPrefix(t, "&")
	return t
}

// buildDiagramSupportTokens collects every textual anchor in the
// answer doc that an edge endpoint can match against. The set is
// case-folded so callers can do plain `strings.Contains` lookups.
//
// Sources, in order of intentional generosity:
//  1. Mermaid node declarations from the diagram body itself
//     (identifier + bracketed label, both flavours).
//  2. Every block.Title (case-folded full string) and every
//     block-level / item-level / diagram-level RenderedClaimUse's
//     FacetID + EvidenceID.
//  3. Every Item.Label across all blocks in the doc.
//
// Why doc-level (not Mutable) sources only: the V2 carrier is
// intentionally self-describing — the answer the user reads must
// itself ground every edge. Pulling subjects from the evidence pool
// would let the LLM hide an unsupported edge behind evidence the
// user never sees rendered.
func buildDiagramSupportTokens(doc *types.AnswerDocumentV2, diagramBlock *types.AnswerBlock) map[string]struct{} {
	out := make(map[string]struct{}, 32)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		out[strings.ToLower(s)] = struct{}{}
	}
	// 1) Mermaid node declarations from the body. A single line can
	// declare multiple nodes — `A[Label] --> B[Label2]` is one
	// statement with TWO decls. Walk every shape opener on the line
	// instead of stopping at the first one.
	if diagramBlock != nil && diagramBlock.Diagram != nil {
		for _, line := range strings.Split(diagramBlock.Diagram.Body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "%%") {
				continue
			}
			for _, decl := range mermaidNodeDeclarationsAll(line) {
				if decl.ident != "" {
					add(decl.ident)
				}
				if decl.label != "" {
					add(decl.label)
				}
			}
		}
		// Diagram-level claim uses
		for _, cu := range diagramBlock.Diagram.ClaimUses {
			add(cu.FacetID)
			add(cu.EvidenceID)
		}
	}
	// 2) Block titles + 3) item labels + per-block / per-item claim_use
	if doc != nil {
		for _, b := range doc.Blocks {
			add(b.Title)
			for _, cu := range b.ClaimUses {
				add(cu.FacetID)
				add(cu.EvidenceID)
			}
			for _, it := range b.Items {
				add(it.Label)
				if it.ClaimUse != nil {
					add(it.ClaimUse.FacetID)
					add(it.ClaimUse.EvidenceID)
				}
			}
		}
	}
	return out
}

// mermaidNodeDecl is the (identifier, label) pair extracted from one
// Mermaid node declaration found anywhere in a body line. A single
// statement like `A[Label A] --> B[Label B]` produces two decls.
type mermaidNodeDecl struct {
	ident string
	label string
}

// mermaidNodeDeclarationsAll walks `line` and returns every node
// declaration it can recognise (one statement may declare several
// nodes joined by arrows). Supported shape wrappers, longest-first:
//
//	A["..."]       -> ident=A label=...
//	A[...]         -> ident=A label=...
//	A(("..."))     -> ident=A label=...
//	A((...))       -> ident=A label=...
//	A({...})       -> ident=A label=...
//	A{...}         -> ident=A label=...
//	A("...")       -> ident=A label=...
//	A(...)         -> ident=A label=...
//	A>...]         -> ident=A label=...
//
// Identifier extraction walks backwards from each opener to the
// previous whitespace or arrow-edge boundary; the label is the inner
// text between opener and matching close.
func mermaidNodeDeclarationsAll(line string) []mermaidNodeDecl {
	openers := []struct{ open, close string }{
		{"[\"", "\"]"},
		{"((", "))"},
		{"{{", "}}"},
		{"(\"", "\")"},
		{"[", "]"},
		{"(", ")"},
		{"{", "}"},
		{">", "]"},
	}
	var out []mermaidNodeDecl
	cursor := 0
	for cursor < len(line) {
		// Find the next-earliest opener at or after `cursor`.
		bestPos := -1
		var bestOpener struct{ open, close string }
		for _, op := range openers {
			pos := strings.Index(line[cursor:], op.open)
			if pos < 0 {
				continue
			}
			absPos := cursor + pos
			if bestPos < 0 || absPos < bestPos {
				bestPos = absPos
				bestOpener = op
			}
		}
		if bestPos < 0 {
			break
		}
		// Walk backwards from bestPos to find the identifier.
		identStart := bestPos
		for identStart > cursor {
			r := line[identStart-1]
			if r == ' ' || r == '\t' || r == '>' || r == ']' || r == ')' || r == '}' || r == '|' {
				break
			}
			identStart--
		}
		ident := strings.TrimSpace(line[identStart:bestPos])
		// Identifier sanity check: arrow-operator characters can
		// never start an identifier. Walk past the opener WITHOUT
		// claiming any close — otherwise the `>` opener (which
		// shares its `]` close with `[`) would swallow the rest of
		// the line on `B --> C[Label]` shapes.
		if ident == "" || strings.ContainsAny(ident, "-=>") {
			cursor = bestPos + len(bestOpener.open)
			continue
		}
		// Locate matching close.
		labelStart := bestPos + len(bestOpener.open)
		closeRel := strings.Index(line[labelStart:], bestOpener.close)
		if closeRel < 0 {
			cursor = bestPos + len(bestOpener.open)
			continue
		}
		labelEnd := labelStart + closeRel
		label := strings.TrimSpace(line[labelStart:labelEnd])
		label = strings.Trim(label, "\"'")
		out = append(out, mermaidNodeDecl{ident: ident, label: label})
		cursor = labelEnd + len(bestOpener.close)
	}
	return out
}

// diagramTokenSupported reports whether `token` (an edge endpoint's
// raw text) appears as / inside any anchor in the support set. The
// match is case-folded and uses substring containment in BOTH
// directions — token-contains-anchor AND anchor-contains-token —
// so e.g. a single-letter `B` in the diagram matches a longer
// support token `BlockHandler` and vice versa.
func diagramTokenSupported(token string, support map[string]struct{}) bool {
	t := strings.ToLower(strings.TrimSpace(token))
	if t == "" {
		return false
	}
	if _, ok := support[t]; ok {
		return true
	}
	for s := range support {
		if strings.Contains(s, t) || strings.Contains(t, s) {
			return true
		}
	}
	return false
}

// validateUncertaintyBlockPresence walks view.UncertaintyRules; for
// each rule whose TriggerFacet appears in view.FacetCoverage's
// Required list, the answer must include at least one block whose
// Kind == rule.ExpectedBlockKind. Empty TriggerFacet rules apply
// unconditionally (their MissingMessage tells the LLM why).
//
// Failure-mode summary: "the family contract demands a caveat /
// disclosure block (e.g. log-source drift), but the answer omitted
// it."
func validateUncertaintyBlockPresence(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || len(view.UncertaintyRules) == 0 {
		return nil
	}
	hasKind := make(map[types.AnswerBlockKind]bool, len(doc.Blocks))
	for _, b := range doc.Blocks {
		hasKind[b.Kind] = true
	}
	requiredFacets := make(map[string]bool)
	if view.FacetCoverage != nil {
		for _, r := range view.FacetCoverage.Required {
			requiredFacets[string(r.Kind)] = true
		}
	}
	var out []types.Violation
	for _, rule := range view.UncertaintyRules {
		// Empty TriggerFacet means "always-required disclosure" —
		// e.g. shape=value families' bounded-scope caveat. Otherwise
		// require the trigger facet to be in the family's required
		// facet set.
		if rule.TriggerFacet != "" && !requiredFacets[rule.TriggerFacet] {
			continue
		}
		if hasKind[rule.ExpectedBlockKind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolUncertaintyBlockMissing,
			Detail: fmt.Sprintf(
				"uncertainty rule (trigger=%q) requires a block of kind=%s but none is present",
				rule.TriggerFacet, rule.ExpectedBlockKind),
			Repair: rule.MissingMessage,
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "uncertainty_block",
				Reason:     "required disclosure block absent",
				Confidence: 0.75,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// blockHasClaimUse reports whether a block carries any claim_use
// annotation — either at block level (b.ClaimUses) or on any item
// (b.Items[i].ClaimUse non-nil) or on a diagram (b.Diagram.ClaimUses).
func blockHasClaimUse(b types.AnswerBlock) bool {
	if len(b.ClaimUses) > 0 {
		return true
	}
	for _, it := range b.Items {
		if it.ClaimUse != nil {
			return true
		}
	}
	if b.Diagram != nil && len(b.Diagram.ClaimUses) > 0 {
		return true
	}
	return false
}

// formNames stringifies a ClaimForm slice for error messages.
func formNames(forms []types.ClaimForm) []string {
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		out = append(out, string(f))
	}
	return out
}

// validateFacetCoverage (R2.3 V2 重接, post_shape_residual_audit.md
// 2026-05-04) checks that every Required FacetCoverageContract entry
// (Tier=Hard/Soft) is covered by at least one V2 block whose
// block.FacetIDs[] names the facet's Kind.
//
// Pre-B8-T4 the V1 sibling (runFacetCoverageOracle) walked V1 doc
// payloads and matched the facet's SourceCandidate evidence IDs
// against per-payload citation refs — V2 mirrors that with the
// typed FacetIDs slice the V2 emit_answer_document validator
// already populates on every block. Coverage now = "any block
// declared this FacetID" — a precise, typed signal (R2 red line).
//
// Tier branching (Phase 5-E1, 2026-05-04):
//   - TierEssential (HARD): always demand coverage. Even when
//     SourceCandidate is empty post-binding, the analyzer template
//     pinned this facet as essential — the answer cannot ship
//     without it; uncovered fires a violation.
//   - TierExpected (SOFT): demand coverage IFF SourceCandidate is
//     non-empty. Non-empty SourceCandidate = typed evidence exists
//     to support the facet, so the answer COULD have surfaced it.
//     Empty SourceCandidate = no typed evidence binds to this
//     facet's AcceptableForms — skipping the gate avoids a noisy
//     "uncovered" demand the LLM cannot satisfy honestly. R3
//     justified because SourceCandidate is fully typed (Phase 5-E0
//     audit).
//   - TierEnrichment (Optional): handled by
//     validateRichnessRegression below, not here.
//
// Skip rules (file-level):
//   - view == nil OR view.FacetCoverage == nil: family doesn't carry
//     facet obligations.
//
// Default classification SOFT (covering a facet often requires
// fresh evidence; missed HARD facet = re-explore hint, not finalizer
// self-failure). Operators promote to STRICT via
// pipeline_contract_strict_kinds.
func validateFacetCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		// Items / ClaimUses MAY also carry FacetID via
		// RenderedClaimUse.FacetID — fold those in too.
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
		for _, item := range b.Items {
			if item.ClaimUse != nil && item.ClaimUse.FacetID != "" {
				covered[strings.TrimSpace(item.ClaimUse.FacetID)] = true
			}
		}
	}
	var out []types.Violation
	for _, req := range view.FacetCoverage.Required {
		if req.Tier == types.TierEnrichment {
			continue
		}
		// Phase 5-E1 evidence-sufficient gate: TierExpected facets
		// with no typed evidence supporting them are skipped — the
		// answer has nothing typed to surface, so demanding coverage
		// would force the LLM to invent unsupported claims.
		// TierEssential always demands regardless (analyzer template
		// pinned it as essential).
		if req.Tier == types.TierExpected && len(req.SourceCandidate) == 0 {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		if covered[kind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolFacetUncovered,
			Detail: fmt.Sprintf(
				"required facet %q (tier=%s) is not covered: no V2 block declared it via block.facet_ids[] or via item.claim_use.facet_id",
				kind, req.Tier),
			Repair: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet, OR re-investigate to gather evidence whose ClaimForm matches the facet's AcceptableForms (when no current evidence supports the facet).",
				kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_facet_coverage",
				Reason:     "required answer facet uncovered by emitted blocks",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validateRichnessRegression (R2.3 V2 重接, post_shape_residual_audit
// 2026-05-04) records ViolRichnessRegression for each Optional facet
// (Tier=Enrichment) whose SourceCandidate is non-empty but no block
// declared its FacetID. This is the pure-telemetry tier — Phase 5
// design says the kind is SOFT-by-default and explicitly NOT
// promotable to STRICT (richness regression is observation, not a
// correctness gate).
//
// Reads:
//   - view.FacetCoverage.Optional[i].SourceCandidate (non-empty =
//     evidence is available, the answer COULD have surfaced it)
//   - block.FacetIDs / item.ClaimUse.FacetID (coverage signal,
//     same as facet_uncovered above)
func validateRichnessRegression(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	if len(view.FacetCoverage.Optional) == 0 {
		return nil
	}
	covered := make(map[string]bool, 8)
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
		for _, item := range b.Items {
			if item.ClaimUse != nil && item.ClaimUse.FacetID != "" {
				covered[strings.TrimSpace(item.ClaimUse.FacetID)] = true
			}
		}
	}
	var out []types.Violation
	for _, req := range view.FacetCoverage.Optional {
		if len(req.SourceCandidate) == 0 {
			continue // no evidence available — not a regression
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		if covered[kind] {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolRichnessRegression,
			Detail: fmt.Sprintf(
				"optional richness facet %q has %d evidence candidate(s) but no V2 block surfaced it (telemetry only — answer ships unchanged)",
				kind, len(req.SourceCandidate)),
			Repair: fmt.Sprintf(
				"if the question would benefit from this facet, declare facet_id=%q on a block; otherwise leave as-is (richness regression is informational).",
				kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_richness_facet_coverage",
				Reason:     "optional facet with available evidence not surfaced",
				Confidence: 0.5,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validateClaimFormSupport (R2.3 V2 重接, post_shape_residual_audit.md
// 2026-05-04) — for every RenderedClaimUse on the V2 doc that names
// both a ClaimForm AND an EvidenceID, look up the EvidenceItem in
// the closure pool and verify the LLM-declared ClaimForm is
// COMPATIBLE with the deterministic ClaimFormOf(item) projection.
//
// Two compatibility shapes count:
//
//  1. Exact match — ClaimUse.ClaimForm == ClaimFormOf(item).
//  2. Generalisation match — ClaimFormOf(item) == ClaimUnknown
//     (the projection couldn't lock the form from typed evidence
//     fields). The LLM is allowed to declare a more specific form
//     than the projection produced; only HARD contradictions fire.
//
// Mismatch = LLM declared the wrong form for the cited evidence
// (or cited the wrong evidence for the declared form). Either
// way the finalizer can fix it without new investigation:
// FallbackFinalizerOnly per default policy.
//
// Default classification STRICT (per V1 rationale: explicit
// LLM-emitted self-contradiction; finalizer rewrite without new
// evidence is enough). Operators relax via
// pipeline_contract_strict_kinds.
//
// Skip rules:
//   - mut == nil (test no-op path).
//   - EvidenceID empty / not found in pool (LLM may cite by
//     EvidenceID we never observed; treat as "no signal" rather
//     than a different violation).
//   - ClaimForm empty (LLM didn't declare; nothing to check).
//   - ClaimFormOf(evidence) == ClaimUnknown (generalisation OK).
func validateClaimFormSupport(doc *types.AnswerDocumentV2, mut *types.MutableState) []types.Violation {
	if doc == nil || mut == nil {
		return nil
	}
	pool := mut.EmittedEvidence()
	if len(pool) == 0 {
		return nil
	}
	byID := make(map[string]types.EvidenceItem, len(pool))
	for _, ev := range pool {
		if ev.ID != "" {
			byID[ev.ID] = ev
		}
	}
	if len(byID) == 0 {
		return nil
	}
	var out []types.Violation
	checkClaim := func(cu *types.RenderedClaimUse, blockID, scope string) {
		if cu == nil || cu.ClaimForm == "" || cu.EvidenceID == "" {
			return
		}
		ev, ok := byID[strings.TrimSpace(cu.EvidenceID)]
		if !ok {
			return
		}
		projected := types.ClaimFormOf(ev)
		if projected == types.ClaimUnknown {
			return
		}
		if projected == cu.ClaimForm {
			return
		}
		out = append(out, types.Violation{
			Kind: types.ViolClaimFormUnsupported,
			Detail: fmt.Sprintf(
				"%s in block %q declared claim_form=%s but the cited evidence (id=%s, source=%s:%d) projects to claim_form=%s",
				scope, blockID, cu.ClaimForm, cu.EvidenceID, ev.Source, ev.LineStart, projected),
			Repair: fmt.Sprintf(
				"either change claim_form to %s on this annotation, or cite a different evidence id whose typed fields project to %s. Do NOT invent new evidence — pick from the existing pool.",
				projected, cu.ClaimForm),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_claim_form_support",
				Reason:     "RenderedClaimUse declares form incompatible with cited evidence projection",
				Confidence: 0.85,
			},
			Stage: string(types.StageFinalize),
		})
	}
	for _, b := range doc.Blocks {
		for i := range b.ClaimUses {
			checkClaim(&b.ClaimUses[i], b.ID, "block-level claim_use")
		}
		if b.Diagram != nil {
			for i := range b.Diagram.ClaimUses {
				checkClaim(&b.Diagram.ClaimUses[i], b.ID, "diagram claim_use")
			}
		}
		for j := range b.Items {
			checkClaim(b.Items[j].ClaimUse, b.ID, fmt.Sprintf("item[%d] claim_use", j))
		}
	}
	return out
}

// validateAbsenceScopeBound (R2.3 V2 重接, post_shape_residual_audit.md
// 2026-05-04) fires when the V2 doc claims a NEGATIVE finding
// (ExactResolution.Status == AnswerExactResolutionAbsent) but no
// citation in the pool carries a bounded negative scope to back the
// claim up. Operationally: an answer that says "X is absent from
// the codebase" must cite at least one Citation with
// Scope=ScopeNegative AND non-empty NegativePattern (the search
// query the system ran and confirmed absent), otherwise the
// absence claim is unbounded.
//
// V2 carrier shares Citation + ExactResolution types verbatim with
// V1, so this is a direct port of the V1 oracle (deleted at B8-T4).
//
// Default classification STRICT (V1 rationale: safety-critical for
// config-trace / absence questions where downstream consumers act
// on the negative finding — operator removes a config knob, etc.).
// Operators relax via pipeline_contract_strict_kinds.
//
// Inputs are typed-only (AnswerExactResolutionAbsent +
// EvidenceScope=ScopeNegative + NegativePattern non-empty). Zero
// prose keyword matching.
func validateAbsenceScopeBound(doc *types.AnswerDocumentV2) []types.Violation {
	if doc == nil || doc.ExactResolution == nil {
		return nil
	}
	if doc.ExactResolution.Status != types.AnswerExactResolutionAbsent {
		return nil
	}
	bounded := false
	for _, c := range doc.Citations {
		if c.Scope == types.ScopeNegative && strings.TrimSpace(c.NegativePattern) != "" {
			bounded = true
			break
		}
	}
	if bounded {
		return nil
	}
	return []types.Violation{{
		Kind:   types.ViolAbsenceScopeExceeded,
		Detail: "exact_resolution.status=absent declared but no citation carries scope=negative + a non-empty negative_pattern; the absence claim is unbounded",
		Repair: "Re-emit emit_answer_document with citations[] including at least one entry with scope='negative' AND a non-empty negative_pattern naming the exact search query (grep / repomap / file glob) that confirmed the absence. If the bounded evidence is already in the pool, attach it directly as a negative-scope citation; otherwise the next investigation pass must run the bounded search and surface its query.",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "exact_resolution.absence_scope",
			Reason:     "absence claim without bounded negative-scope citation",
			Confidence: 0.65,
		},
		Stage: string(types.StageFinalize),
	}}
}

// runV2BlockOracles is the single orchestrator-side dispatch entry
// for B4. Returns the union of all V2 validator violations. Caller
// (runContractCheck) appends to the result Violations slice the
// same way Block 2/3 oracles do.
//
// Returns nil when doc or view is nil.
//
// R2.3 (post_shape_residual_audit.md, 2026-05-04): three new V2
// oracles join the dispatch:
//   - validateFacetCoverage (HARD/SOFT facet uncovered)
//   - validateRichnessRegression (Optional facet telemetry)
//   - validateClaimFormSupport (LLM-declared ClaimForm vs typed
//     evidence's projected ClaimForm — needs evidence pool, so
//     dispatch via runV2BlockOraclesWithMut wrapper that takes
//     the MutableState handle)
func runV2BlockOracles(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	return runV2BlockOraclesWithMut(doc, view, nil)
}

// runV2BlockOraclesWithMut is the mut-aware variant. nil mut
// disables the validators that need evidence-pool access (used in
// unit tests that don't wire a Mutable). Production caller in
// contract_check.go::runContractCheck threads mut.
func runV2BlockOraclesWithMut(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, mut *types.MutableState) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	var out []types.Violation
	out = append(out, validateRequiredBlockCoverage(doc, view)...)
	out = append(out, validatePrincipalClaimUse(doc, view)...)
	out = append(out, validateDiagramEdgeSupport(doc, view)...)
	out = append(out, validateUncertaintyBlockPresence(doc, view)...)
	out = append(out, validateFacetCoverage(doc, view)...)
	out = append(out, validateRichnessRegression(doc, view)...)
	out = append(out, validateClaimFormSupport(doc, mut)...)
	out = append(out, validateAbsenceScopeBound(doc)...)
	out = append(out, validateEnumerationItemLabelGrounding(doc, mut)...)
	return out
}

// validateEnumerationItemLabelGrounding (post-shape s1a-20260504-064754
// hallucination forensic, 2026-05-04) checks that every
// ordered_list / bullet_list block's items[i].label is supported by
// at least one EvidenceItem in the dispatch's evidence pool — by
// substring match against AnchorSymbol / Subject / Object. The
// case that motivated the oracle: explorer emitted 28 evidence
// items naming the 9 real gate.Run checks (checkCoverage /
// checkContractComplete / checkSubtopicCoherence / etc.), but the
// finalizer wrote 9 items with fabricated labels (checkCrossSignalCoherence
// / checkAnswerSubjectKindIsValid / etc.) and shipped, because no
// pre-existing oracle compared `items[].label` to the evidence
// pool — `validateClaimFormSupport` checks claim_form vs evidence
// kind but ignores the label string itself, and the
// self_consistency_reviewer compares SUMMARY-vs-BODY only.
//
// Skip conditions (no false positives):
//
//   - mut == nil → unit-test path, no evidence pool to check against.
//   - Empty / nil EvidenceItems → no anchors to ground against; the
//     LLM may legitimately rely on extractor-derived data only.
//   - Blocks with kind ∉ { ordered_list, bullet_list }: scalar /
//     decision / diagram / section / summary blocks have their own
//     grounding lanes (citation_ref + grounder).
//   - Items with empty label: the prose lives in `text`, not the
//     label, so there's nothing structurally to ground.
//   - Item kind == "flow" or "caveat": those are narration / scope
//     notes, not principal enumeration entries — `Kind` discipline
//     (Plan D, 2026-05-02) explicitly excludes them from the count.
//   - Block.SurfaceRole == prose_only / diagram_only: not principal
//     payload.
//
// Match semantics: case-folded substring match in BOTH directions
// (label-contains-anchor OR anchor-contains-label) so a label like
// `checkCoverage` matches an evidence anchor like `checkCoverage(ir,
// th)` and a fragment-style anchor like `Coverage` matches the
// label `checkCoverage`. This mirrors the diagram edge oracle's
// `diagramTokenSupported` semantics (R4.3) so the two oracles
// behave consistently across enumeration vs diagram surfaces.
func validateEnumerationItemLabelGrounding(doc *types.AnswerDocumentV2, mut *types.MutableState) []types.Violation {
	if doc == nil || mut == nil {
		return nil
	}
	turnA := mut.TurnAArtifacts()
	if turnA == nil || len(turnA.EvidenceItems) == 0 {
		return nil
	}
	support := buildEvidenceLabelSupportTokens(turnA.EvidenceItems)
	if len(support) == 0 {
		return nil
	}
	type ungroundedItem struct {
		blockID string
		itemID  string
		label   string
	}
	var ungrounded []ungroundedItem
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList {
			continue
		}
		if b.SurfaceRole == types.SurfaceProseOnly || b.SurfaceRole == types.SurfaceDiagramOnly {
			continue
		}
		for _, it := range b.Items {
			label := strings.TrimSpace(it.Label)
			if label == "" {
				continue
			}
			if diagramTokenSupported(label, support) {
				continue
			}
			ungrounded = append(ungrounded, ungroundedItem{
				blockID: b.ID,
				itemID:  it.ID,
				label:   label,
			})
		}
	}
	if len(ungrounded) == 0 {
		return nil
	}
	pairs := make([]string, 0, len(ungrounded))
	for _, u := range ungrounded {
		pairs = append(pairs, fmt.Sprintf("block=%q item=%q label=%q", u.blockID, u.itemID, u.label))
	}
	return []types.Violation{{
		Kind: types.ViolEnumerationLabelUngrounded,
		Detail: fmt.Sprintf(
			"%d enumeration item label(s) do not match any evidence pool anchor_symbol / subject / object: [%s]",
			len(ungrounded), strings.Join(pairs, "; ")),
		Repair: "for each listed item, copy the label verbatim from one of the evidence pool's anchor_symbol values (or replace the item with one whose label is grounded). Fabricating identifiers that the evidence does not name is silently misleading; if the answer truly requires an item that no evidence supports, reopen the investigation rather than inventing a label.",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "block_items_label",
			Reason:     "items[].label not supported by any evidence pool anchor",
			Confidence: 0.85,
		},
		Stage: string(types.StageFinalize),
	}}
}

// buildEvidenceLabelSupportTokens collects every ground-able token
// from the evidence pool. Tokens are case-folded so callers do
// substring lookup with the same key normaliser. Sources:
//
//   - EvidenceItem.AnchorSymbol — the named identifier the explorer
//     pinned at the citation line.
//   - EvidenceItem.Subject / Object — the relation endpoints
//     (loaded by Predicate-axis evidence, e.g. "checkCoverage CALLS
//     ContractComplete").
//   - EvidenceItem.OwnerSymbol — the enclosing function / type when
//     the anchor is itself nested.
//
// Empty tokens are dropped. Each token also has its leading/trailing
// whitespace trimmed before lower-casing so the diagramTokenSupported
// substring loop never has to re-trim.
func buildEvidenceLabelSupportTokens(items []types.EvidenceItem) map[string]struct{} {
	out := make(map[string]struct{}, 4*len(items))
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		out[strings.ToLower(s)] = struct{}{}
	}
	for _, it := range items {
		add(it.AnchorSymbol)
		add(it.OwnerSymbol)
		add(it.Subject)
		add(it.Object)
	}
	return out
}

// _ keeps strings import used (formNames + Detail strings).
var _ = strings.TrimSpace
