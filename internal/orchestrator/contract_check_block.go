package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

var snippetSelectorTokenRE = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+\b`)

func blockClusterKey(blockID string, rootField string) string {
	return types.BlockClusterKey(blockID, rootField)
}

func blockKindClusterKey(kind types.AnswerBlockKind, rootField string) string {
	return types.BlockKindClusterKey(kind, rootField)
}

func facetClusterKey(kind string, rootField string) string {
	return types.FacetClusterKey(kind, rootField)
}

func relationClusterKey(kind types.DiagramRelationKind, rootField string) string {
	return types.RelationClusterKey(kind, rootField)
}

func familyClusterKey(kind types.QuestionFamily, rootField string) string {
	return types.FamilyClusterKey(kind, rootField)
}

func symbolClusterKey(symbol string, rootField string) string {
	return types.SymbolClusterKey(symbol, rootField)
}

func identityClusterKey(identity string, rootField string) string {
	return types.IdentityClusterKey(identity, rootField)
}

func topicClusterKey(topic string, rootField string) string {
	return types.TopicClusterKey(topic, rootField)
}

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
				ClusterKey: blockKindClusterKey(req.Kind, "answer_block_coverage"),
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
				ClusterKey: blockKindClusterKey(req.Kind, "answer_block_coverage"),
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
		// Single-choice contract relaxation: when the contract
		// declares exactly one AcceptableClaimForm AND the block
		// already carries the structural grounding the contract is
		// trying to verify (facet_ids non-empty + at least one item
		// with citation_ref >= 0), the LLM had no real choice — the
		// "missing" claim_uses[] is a redundant explicit field the
		// LLM forgot to write while still emitting the grounded
		// content. Treat as implicit-default; the answer's coverage
		// is unambiguous. qf_arch run-1 forensic showed iter 3
		// burning two retries on this exact pattern (section blocks
		// pre_stages / main_stages each had facet_ids + cited items
		// but no claim_uses).
		if len(req.AcceptableClaimForms) == 1 && hasGroundedCoverage(b) {
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
			ClusterKey: blockClusterKey(b.ID, "block_claim_use"),
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
			ClusterKey: blockKindClusterKey(types.BlockDiagram, "diagram_block"),
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
	// Kind-mismatch HARD reject only when the plan is REQUIRED. When
	// the explore phase couldn't find enough typed-edge evidence to
	// mandate a specific diagram kind (DiagramPlan.Required=false —
	// diagramPlanFor still produces a non-nil plan with the family's
	// preferred Kind, but flags it advisory), the LLM is free to pick
	// the diagram form that best serves the answer prose. Forcing the
	// preferred kind in that case wastes a retry on what is, by
	// contract, just a preference. Eval forensic (qf_arch x4 round on
	// f399cf4): LLM emitted kind=flow under Required=false → HARD
	// reject + 1 retry, every cycle. With this gate, the same emit
	// passes (flow / sequence / call_dag are all reasonable readings
	// of an architecture answer) and only mandatory family kinds
	// continue to fire mismatches.
	if plan.Required &&
		plan.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != types.DiagramNone &&
		diagramBlock.Diagram.Kind != plan.Kind {
		return []types.Violation{{
			Kind: types.ViolDiagramEdgeUnsupported,
			Detail: fmt.Sprintf(
				"diagram block id=%q declared kind=%s but family contract requires %s",
				diagramBlock.ID, diagramBlock.Diagram.Kind, plan.Kind),
			Repair: fmt.Sprintf(
				"set diagram.kind=%s OR drop the diagram if the family contract should be relaxed",
				plan.Kind),
			ClusterKey: blockClusterKey(diagramBlock.ID, "diagram_kind"),
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
			Repair:     "for each listed edge, either (a) declare its endpoints as labelled nodes in the same Mermaid body (e.g. A[\"Label A\"] --> B[\"Label B\"]), (b) name the endpoints in an item label or block title in this answer, or (c) drop the edge if it represents an inference not backed by any grounded claim.",
			ClusterKey: blockClusterKey(diagramBlock.ID, "diagram_edges"),
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
	// G3 (post_v2_runtime_gap_remediation, 2026-05-04): build
	// (from,to) → typed RelationKind index from EdgeAnchors that
	// declare RelationKind directly. Resolution is typed-first;
	// label inference is the fallback.
	typedRelIndex := buildTypedRelationIndex(doc)

	type labelMismatch struct {
		from, to, label string
		typed           types.DiagramRelationKind
		labelInferred   types.DiagramRelationKind
	}
	type labelOnlyEdge struct {
		from, to, label string
		labelInferred   types.DiagramRelationKind
	}
	var mismatches []labelMismatch
	// B3 v3 (2026-05-04): split typed and label-only counts so the
	// EdgeRelations.Min check can distinguish "typed satisfies the
	// minimum" from "label-only inference saved the minimum". Typed
	// counts are authoritative; label-only counts also fill the
	// contract but trigger ViolDiagramRelationLabelOnly SOFT advisory.
	typedRelCounts := make(map[types.DiagramRelationKind]int)
	labelRelCounts := make(map[types.DiagramRelationKind]int)
	labelOnlyEdges := make(map[types.DiagramRelationKind][]labelOnlyEdge)
	for _, e := range edges {
		// Resolution priority:
		//   1. Typed RelationKind on a matching EdgeAnchor (from,to) →
		//      authoritative.
		//   2. Otherwise InferRelationFromLabel(e.label).
		// Unknown (no typed AND no recognised label keyword) keeps the
		// pre-G3 "label-free edge" semantic — legitimate, no violation.
		typedRel, hasTyped := typedRelIndex[edgeKey{
			from: strings.ToLower(strings.TrimSpace(e.from)),
			to:   strings.ToLower(strings.TrimSpace(e.to)),
		}]
		labelRel := types.InferRelationFromLabel(e.label)
		var rel types.DiagramRelationKind
		switch {
		case hasTyped:
			rel = typedRel
			typedRelCounts[typedRel]++
			// Advisory drift signal: typed says X, label parses to Y, X≠Y.
			// SOFT-only — labels are noisy; never promote.
			if labelRel != types.DiagramRelUnknown && labelRel != typedRel {
				mismatches = append(mismatches, labelMismatch{
					from: e.from, to: e.to, label: e.label,
					typed: typedRel, labelInferred: labelRel,
				})
			}
		default:
			rel = labelRel
			if rel == types.DiagramRelUnknown {
				rel = implicitDiagramRelationKind(diagramBlock, e)
			}
			if rel != types.DiagramRelUnknown {
				labelRelCounts[rel]++
				labelOnlyEdges[rel] = append(labelOnlyEdges[rel],
					labelOnlyEdge{from: e.from, to: e.to, label: e.label, labelInferred: rel})
			}
		}
		// Per-edge claim_form match check is fully retired:
		//
		//   - !hasTyped + labelRel known: the label vocabulary types the
		//     relation; no edge_anchors entry needed (covers the prior
		//     label-only relaxation).
		//   - hasTyped: the LLM declared relation_kind on edge_anchors,
		//     and ClaimFormForRelation(relation_kind) gives a 1-to-1
		//     mapping for every relation except DiagramRelContain. The
		//     claim_form field on edge_anchors is therefore redundant
		//     with relation_kind — requiring the LLM to write both
		//     correctly (and rejecting on mismatch) was the same kind of
		//     "info repetition" the earlier label-only relaxation removed,
		//     just one layer down. Eval run-1 of the post-label-only
		//     relaxation showed an LLM declaring relation_kind=precedence
		//     with claim_form=call_edge across 5 edges and burning 5
		//     finalizer iters on the resulting reject loop — that is the
		//     failure mode this drop cures.
		//
		// The EdgeRelations.Min contract still fires
		// ViolDiagramRelationLabelOnly (SOFT) when label inference fills
		// the count, and diagram_edge_label_mismatch (SOFT) still
		// surfaces typed-vs-label drift for operator visibility.
		_ = rel
	}

	var violations []types.Violation
	for _, contract := range plan.EdgeRelations {
		if contract.Min <= 0 {
			continue
		}
		typedSatisfied := typedRelCounts[contract.Kind] >= contract.Min
		totalSatisfied := typedRelCounts[contract.Kind]+labelRelCounts[contract.Kind] >= contract.Min
		switch {
		case typedSatisfied:
			// All Min met by typed declarations — clean pass.
		case totalSatisfied:
			// Label-only inference filled the gap. Emit SOFT advisory
			// encouraging typed declaration on the label-only edges.
			edges := labelOnlyEdges[contract.Kind]
			edgeStrs := make([]string, 0, len(edges))
			for _, e := range edges {
				edgeStrs = append(edgeStrs, fmt.Sprintf("%s --|%s|--> %s", e.from, e.label, e.to))
			}
			violations = append(violations, types.Violation{
				Kind: types.ViolDiagramRelationLabelOnly,
				Detail: fmt.Sprintf(
					"diagram block id=%q satisfies relation kind=%s minimum=%d via %d label-only edge(s) and %d typed edge(s); typed declaration is the authoritative surface — consider declaring relation_kind on edge_anchors[] for: [%s]",
					diagramBlock.ID, contract.Kind, contract.Min,
					labelRelCounts[contract.Kind], typedRelCounts[contract.Kind],
					strings.Join(edgeStrs, "; ")),
				Repair: fmt.Sprintf(
					"for each label-only edge listed, add an edge_anchors[] entry with relation_kind=%s, from_node, to_node, and claim_form=%s. The contract is currently met but typed declarations let future readers see the typed authority directly.",
					contract.Kind, contract.ClaimForm),
				ClusterKey: relationClusterKey(contract.Kind, "diagram_edges"),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "diagram_edges",
					Reason:     "relation contract met via label-only inference",
					Confidence: 0.5,
				},
				Stage: string(types.StageFinalize),
			})
		default:
			// Even total counts (typed + label) below minimum — HARD
			// reject. Detail describes the typed surface as primary.
			// When typed has occupied an edge with a different
			// RelationKind than this contract's Kind, the surfaced
			// detail explains why label inference can't save the
			// minimum (the edge is already typed-occupied).
			short := contract.Min - typedRelCounts[contract.Kind] - labelRelCounts[contract.Kind]
			detailExtra := ""
			if typedRelCounts[contract.Kind] == 0 && len(labelOnlyEdges[contract.Kind]) == 0 {
				detailExtra = fmt.Sprintf(" — typed declarations exist for other relation kinds; if you intended an edge to carry kind=%s, set its edge_anchors[] entry's relation_kind accordingly", contract.Kind)
			}
			violations = append(violations, types.Violation{
				Kind: types.ViolDiagramEdgeUnsupported,
				Detail: fmt.Sprintf(
					"diagram block id=%q expected at least %d edge(s) of relation kind=%s but found %d typed + %d label-only (need %d more)%s",
					diagramBlock.ID, contract.Min, contract.Kind,
					typedRelCounts[contract.Kind], labelRelCounts[contract.Kind], short, detailExtra),
				Repair: fmt.Sprintf(
					"add at least %d edge(s) of relation kind=%s. PREFERRED: declare relation_kind=%s on a block-level edge_anchors entry along with from_node / to_node / claim_form=%s. ALTERNATIVE: label the Mermaid edge with vocabulary that resolves to this relation (see the %q section above for the recognised label vocabulary).",
					short, contract.Kind, contract.Kind, contract.ClaimForm, types.SectionDiagramEdgeLabelVocabulary),
				ClusterKey: relationClusterKey(contract.Kind, "diagram_edges"),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "diagram_edges",
					Reason:     "minimum-count contract for typed relation not satisfied",
					Confidence: 0.55,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	if len(mismatches) > 0 {
		// G3 SOFT advisory — readers benefit when the rendered label
		// matches the typed declaration. Never promotable to STRICT
		// (label inference is noisy; defaultSoftKinds permanently
		// classifies this kind SOFT).
		details := make([]string, 0, len(mismatches))
		for _, m := range mismatches {
			details = append(details, fmt.Sprintf(
				"%s --|%s|--> %s (typed declares %s; the label reads as %s)",
				m.from, m.label, m.to, m.typed, m.labelInferred))
		}
		violations = append(violations, types.Violation{
			Kind: types.ViolDiagramEdgeLabelMismatch,
			Detail: fmt.Sprintf(
				"diagram block id=%q has %d edge(s) whose typed relation_kind disagrees with the rendered label vocabulary: [%s]",
				diagramBlock.ID, len(mismatches), strings.Join(details, "; ")),
			Repair: fmt.Sprintf(
				"the typed relation_kind on edge_anchors is the authority; align the rendered label vocabulary with each typed declaration so readers see the same relation the validator does (see the %q section above for the recognised label vocabulary). The answer ships with this drift left in place; this signal is advisory.",
				types.SectionDiagramEdgeLabelVocabulary),
			ClusterKey: blockClusterKey(diagramBlock.ID, "diagram_edges"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_edges",
				Reason:     "rendered label disagrees with typed relation declaration",
				Confidence: 0.5,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return violations
}

func implicitDiagramRelationKind(diagramBlock *types.AnswerBlock, edge mermaidEdge) types.DiagramRelationKind {
	if diagramBlock == nil || diagramBlock.Diagram == nil {
		return types.DiagramRelUnknown
	}
	if diagramBlock.Diagram.Kind == types.DiagramSequence && strings.TrimSpace(edge.label) == "" {
		// In Mermaid sequence diagrams an unlabelled directed message
		// arrow still semantically represents one participant sending
		// control to another. Treat this as the typed default `call`
		// relation so call-chain/root-cause sequence spines do not burn
		// retries purely for omitting a redundant edge label.
		return types.DiagramRelCall
	}
	return types.DiagramRelUnknown
}

// edgeKey is the (lowercase from, lowercase to) lookup key for the
// typed-relation index. Keep separate from claimUseEdgeKey so the
// two indexes can diverge (typed-relation lookup does not key on
// ClaimForm — multiple ClaimForm entries on the same edge collapse
// onto the same RelationKind).
type edgeKey struct {
	from, to string
}

// buildTypedRelationIndex collects every EdgeAnchor whose RelationKind
// is set, keyed by case-folded (FromNode, ToNode). When two anchors
// for the same edge declare different RelationKinds, the FIRST entry
// wins (defensive — schema doesn't currently allow duplicates, but
// the validator should not panic on them).
//
// G3 (post_v2_runtime_gap_remediation, 2026-05-04). Building the
// index once per diagram block keeps the per-edge resolution O(1).
func buildTypedRelationIndex(doc *types.AnswerDocumentV2) map[edgeKey]types.DiagramRelationKind {
	idx := make(map[edgeKey]types.DiagramRelationKind)
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		for j := range b.EdgeAnchors {
			a := &b.EdgeAnchors[j]
			if !a.HasEdgeAnchor() {
				continue
			}
			rel := a.RelationKind
			if rel == types.DiagramRelUnknown {
				rel = types.RelationForClaimForm(a.ClaimForm)
			}
			if rel == types.DiagramRelUnknown {
				continue
			}
			k := edgeKey{
				from: strings.ToLower(strings.TrimSpace(a.FromNode)),
				to:   strings.ToLower(strings.TrimSpace(a.ToNode)),
			}
			if _, exists := idx[k]; exists {
				continue
			}
			idx[k] = rel
		}
	}
	return idx
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
//     A -->|label| B  /  A -- text --> B
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
			for _, decl := range mermaidSequenceParticipantDeclarations(line) {
				if decl.ident != "" {
					add(decl.ident)
				}
				if decl.label != "" {
					add(decl.label)
				}
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
	}
	// 2) Block titles + 3) item labels + block-level claim_uses
	if doc != nil {
		for _, b := range doc.Blocks {
			add(b.Title)
			for _, cu := range b.ClaimUses {
				add(cu.FacetID)
				add(cu.EvidenceID)
			}
			for _, it := range b.Items {
				add(it.Label)
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

func mermaidSequenceParticipantDeclarations(line string) []mermaidNodeDecl {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	rest := ""
	switch {
	case strings.HasPrefix(trimmed, "participant "):
		rest = strings.TrimSpace(strings.TrimPrefix(trimmed, "participant "))
	case strings.HasPrefix(trimmed, "actor "):
		rest = strings.TrimSpace(strings.TrimPrefix(trimmed, "actor "))
	default:
		return nil
	}
	if rest == "" {
		return nil
	}
	if idx := strings.Index(rest, " as "); idx > 0 {
		ident := strings.TrimSpace(rest[:idx])
		label := strings.TrimSpace(rest[idx+4:])
		if ident == "" && label == "" {
			return nil
		}
		return []mermaidNodeDecl{{ident: ident, label: label}}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil
	}
	ident := strings.TrimSpace(fields[0])
	if ident == "" {
		return nil
	}
	return []mermaidNodeDecl{{ident: ident, label: ident}}
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
			Repair:     rule.MissingMessage,
			ClusterKey: blockKindClusterKey(rule.ExpectedBlockKind, "uncertainty_block"),
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
// annotation. Claim annotations live ONLY at block level
// (b.ClaimUses); items and diagrams no longer carry claim_use.
func blockHasClaimUse(b types.AnswerBlock) bool {
	return len(b.ClaimUses) > 0
}

// hasGroundedCoverage reports whether a block carries the structural
// evidence that a single-choice claim contract is trying to verify:
// non-empty facet_ids (block declares which facets it covers) AND at
// least one item with citation_ref >= 0 (block is anchored to the
// citations[] pool). When the contract's AcceptableClaimForms has
// exactly one entry, the LLM had no real choice — an implicit
// claim_uses=[{claim_form=<the-only-option>}] is unambiguously
// correct and the missing explicit field is redundant.
func hasGroundedCoverage(b types.AnswerBlock) bool {
	if len(b.FacetIDs) == 0 {
		return false
	}
	for _, item := range b.Items {
		if item.CitationRef >= 0 {
			return true
		}
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
// Coverage = "any block declared this FacetID via block.FacetIDs[]
// or block.ClaimUses[i].FacetID" — a precise typed signal (R3 red
// line).
//
// Promotion gate (G6 post_v2_runtime_gap_remediation, 2026-05-04):
// the (Tier, len(SourceCandidate)) joint logic is centralised on
// FacetRequirement.IsPromoted(). The validator now reads a single
// typed predicate:
//
//   - IsPromoted()=true  → demand coverage (uncovered fires a
//     violation). Always-hard facets and evidence-sufficient
//     promoted facets both flow through this branch.
//   - IsPromoted()=false → skip the gate. Either the policy is
//     AdvisoryOnly (Optional / Enrichment) — handled by
//     validateRichnessRegression — or the policy is
//     WhenEvidenceSufficient with no typed evidence supporting
//     this facet, in which case demanding coverage would force the
//     LLM to invent unsupported claims (R3 invariant — typed
//     evidence absence is a precise skip signal).
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
		// Block-level ClaimUses MAY also carry FacetID via
		// RenderedClaimUse.FacetID — fold those in too.
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
	}
	var out []types.Violation
	for _, req := range view.FacetCoverage.Required {
		// Enrichment facets are AdvisoryOnly — handled by
		// validateRichnessRegression. Defensive guard preserved for
		// callers that hand-construct contracts with Enrichment in
		// the Required list.
		if req.EffectivePromotionPolicy() == types.PromotionAdvisoryOnly {
			continue
		}
		// G6: single typed gate — promote only when the typed
		// (PromotionPolicy, MinEvidenceForPromotion, len(SourceCandidate))
		// triple says so.
		if !req.IsPromoted() {
			continue
		}
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		if covered[kind] {
			continue
		}
		// Detail surfaces the typed evidence count so the LLM /
		// operator can tell WHY this facet was promoted (always-hard
		// vs evidence-sufficient).
		var promotionLabel string
		switch req.EffectivePromotionPolicy() {
		case types.PromotionAlwaysHard:
			promotionLabel = "essential"
		case types.PromotionWhenEvidenceSufficient:
			promotionLabel = fmt.Sprintf("evidence-sufficient (%d typed evidence rows support this facet)",
				len(req.SourceCandidate))
		default:
			promotionLabel = string(req.EffectivePromotionPolicy())
		}
		out = append(out, types.Violation{
			Kind: types.ViolFacetUncovered,
			Detail: fmt.Sprintf(
				"required facet %q (%s) is not covered: no block declared it via block.facet_ids[] or via item.claim_use.facet_id",
				kind, promotionLabel),
			Repair: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet, OR re-investigate to gather evidence whose claim_form matches the facet's acceptable forms (when no current evidence supports the facet).",
				kind),
			ClusterKey: facetClusterKey(kind, "answer_facet_coverage"),
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
// v3 B2 (2026-05-04): when validateRichnessGlaringGap fires on the
// SAME facet, this validator suppresses its telemetry entry to
// avoid duplicate notice. Reviewer / retry hint render reads only
// one violation per facet.
//
// Reads:
//   - view.FacetCoverage.Optional[i].SourceCandidate (non-empty =
//     evidence is available, the answer COULD have surfaced it)
//   - block.FacetIDs / block.ClaimUses[i].FacetID (coverage signal,
//     same as facet_uncovered above)
func validateRichnessRegression(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	if len(view.FacetCoverage.Optional) == 0 {
		return nil
	}
	covered := buildFacetCoverageSet(doc)
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
		// v3 B2 suppression: skip the SOFT telemetry entry when this
		// facet is also a glaring-gap candidate — the new validator
		// emits a more actionable Medium-severity violation that
		// supersedes this one.
		if req.IsGlaringGap(view.Family, false) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolRichnessRegression,
			Detail: fmt.Sprintf(
				"optional richness facet %q has %d evidence candidate(s) but no block surfaced it (telemetry only — answer ships unchanged)",
				kind, len(req.SourceCandidate)),
			Repair: fmt.Sprintf(
				"if the question would benefit from this facet, declare facet_id=%q on a block; otherwise leave as-is (richness regression is informational).",
				kind),
			ClusterKey: facetClusterKey(kind, "answer_richness_facet_coverage"),
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

// buildFacetCoverageSet projects the rendered V2 doc's facet
// coverage signal into a string set: a facet kind appears when ANY
// block declares it via block.FacetIDs OR
// block.ClaimUses[i].FacetID. Shared by the facet / richness /
// glaring-gap validators so coverage logic stays consistent.
func buildFacetCoverageSet(doc *types.AnswerDocumentV2) map[string]bool {
	covered := make(map[string]bool, 8)
	if doc == nil {
		return covered
	}
	for _, b := range doc.Blocks {
		for _, fid := range b.FacetIDs {
			covered[strings.TrimSpace(fid)] = true
		}
		for _, cu := range b.ClaimUses {
			if cu.FacetID != "" {
				covered[strings.TrimSpace(cu.FacetID)] = true
			}
		}
	}
	return covered
}

// validateRichnessGlaringGap (B2 v3, 2026-05-04) records
// ViolRichnessGlaringGap for each Optional facet marked
// EnrichmentGlaring whose SourceCandidate count meets the family
// threshold AND whose FacetID is not surfaced anywhere in the
// rendered V2 doc.
//
// Severity = Medium (retry-eligible). Driving the LLM to re-emit
// with the missing facet declared keeps the answer rich when typed
// evidence supports it. Operators may promote to STRICT via
// pipeline_contract_strict_kinds.
//
// Trigger predicates (all runtime typed):
//   - req.Strength == EnrichmentGlaring
//   - len(req.SourceCandidate) >= EffectiveMinEvidenceForGlaring(family)
//   - covered == false
//
// All three are typed-enum / typed-integer / typed-boolean
// comparisons — R3 invariant satisfied.
func validateRichnessGlaringGap(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || view.FacetCoverage == nil {
		return nil
	}
	if len(view.FacetCoverage.Optional) == 0 {
		return nil
	}
	covered := buildFacetCoverageSet(doc)
	var out []types.Violation
	for _, req := range view.FacetCoverage.Optional {
		kind := strings.TrimSpace(string(req.Kind))
		if kind == "" {
			continue
		}
		isCovered := covered[kind]
		if !req.IsGlaringGap(view.Family, isCovered) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolRichnessGlaringGap,
			Detail: fmt.Sprintf(
				"optional facet %q has %d typed evidence candidate(s) (>= threshold %d) but no block surfaced it; the answer ships without enrichment available evidence supports",
				kind, len(req.SourceCandidate), req.EffectiveMinEvidenceForGlaring(view.Family)),
			Repair: fmt.Sprintf(
				"declare facet_id=%q on at least one block whose payload covers this facet, AND reference at least one of the typed evidence anchors inline so the surrounding prose is grounded.",
				kind),
			ClusterKey: facetClusterKey(kind, "answer_richness_facet_coverage"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_richness_facet_coverage",
				Reason:     "glaring richness facet uncovered with sufficient typed evidence",
				Confidence: 0.7,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// validatePrincipalProseUnderfilled (B2 v3, 2026-05-04) detects the
// "covered but prose thin" pattern using a typed structural signal:
// a principal block declares ≥ 3 typed claim_use annotations but its
// rendered prose contains zero Markdown inline-code references
// (`...` segments). The signal says "you have 3+ grounded evidence
// rows but didn't anchor any identifier inline" — common when the
// LLM generalises the prose away from the typed anchors it cites.
//
// Per-block-kind dispatch:
//   - Summary / Section / Caveat → check block.Text
//   - OrderedList / BulletList    → aggregate items[].Text
//   - Scalar / Decision / Diagram / Table → skip (claim_use density
//     and prose surface differ; a 1-claim_use scalar is not "thin")
//
// Severity = Medium. Operators may promote via
// pipeline_contract_strict_kinds when their pipeline guarantees rich
// prose. R3 invariant: claim_use count is typed integer, inline-code
// match is verbatim Markdown substring; both runtime typed.
func validatePrincipalProseUnderfilled(doc *types.AnswerDocumentV2) []types.Violation {
	if doc == nil {
		return nil
	}
	const claimUseFloor = 3
	var out []types.Violation
	for _, b := range doc.Blocks {
		if b.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		var inlineCode int
		var claimCount int
		var allItemsLabelAnchored bool
		switch b.Kind {
		case types.BlockSummary, types.BlockSection, types.BlockCaveat:
			inlineCode = countInlineCodeSegments(b.Text)
			claimCount = len(b.ClaimUses)
		case types.BlockOrderedList, types.BlockBulletList:
			// Lists declare claim_uses[] once at block level; count
			// cited items as the evidence-density signal (a list with
			// many items pointing at citations[] but no inline code in
			// any item.text is the "abstracted away from evidence" smell).
			//
			// Eval audit (qf_arch run-1 forensic, 2026-05-06): the
			// validator was over-firing on enumeration lists where each
			// item.label IS an identifier (StageLogTriage / StageAnalyze
			// / etc.). Those labels render as bold row headers in the
			// final answer — the user sees the identifier even though
			// item.text doesn't quote it back with backticks. Treat a
			// list as label-anchored when EVERY item has a non-empty
			// identifier-shaped label; the prose-density signal still
			// fires on lists whose items have free-prose labels (which
			// genuinely need backtick anchors in text).
			allItemsLabelAnchored = len(b.Items) > 0
			for _, item := range b.Items {
				inlineCode += countInlineCodeSegments(item.Text)
				if item.CitationRef >= 0 {
					claimCount++
				}
				label := strings.TrimSpace(item.Label)
				if label == "" || !looksLikeIdentifierShape(label) {
					allItemsLabelAnchored = false
				}
			}
		default:
			continue // scalar / decision / diagram / table — skip
		}
		if claimCount < claimUseFloor {
			continue
		}
		if inlineCode > 0 {
			continue
		}
		if allItemsLabelAnchored {
			continue
		}
		if principalListLabelsAnchoredByClaimSurface(b) {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolPrincipalProseUnderfilled,
			Detail: fmt.Sprintf(
				"principal block id=%q (kind=%s) carries %d typed claim_use annotation(s) but its prose contains zero inline `code` references — the surrounding text was abstracted away from the cited evidence",
				b.ID, b.Kind, claimCount),
			Repair: fmt.Sprintf(
				"reference at least one grounded identifier inline (e.g. `funcName` / `package.Type`) on block id=%q so the prose anchors to the typed evidence the answer cited.",
				b.ID),
			ClusterKey: blockClusterKey(b.ID, "answer_prose_density"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "answer_prose_density",
				Reason:     "principal block has multiple cited claims but no inline-code anchors in prose",
				Confidence: 0.6,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

func principalListLabelsAnchoredByClaimSurface(b types.AnswerBlock) bool {
	if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList {
		return false
	}
	if len(b.Items) == 0 || len(b.ClaimUses) == 0 {
		return false
	}
	claimFormsUseDisplayLabels := true
	for _, cu := range b.ClaimUses {
		if !cu.ClaimForm.UsesNonSymbolLabelSurface() {
			claimFormsUseDisplayLabels = false
			break
		}
	}
	allSourceLocationLabels := true
	for _, item := range b.Items {
		label := strings.TrimSpace(item.Label)
		if label == "" || item.CitationRef < 0 {
			return false
		}
		if _, ok := types.ParseAnswerSourceLocationSurface(label); !ok {
			allSourceLocationLabels = false
		}
	}
	return claimFormsUseDisplayLabels || allSourceLocationLabels
}

// countInlineCodeSegments counts Markdown inline-code segments
// (backtick-delimited substrings) in s. Verbatim structural signal
// — universal across languages, no keyword tables. Empty backtick
// runs (“) count as one zero-content segment; the validator only
// cares whether the count is > 0, so this is conservative.
func countInlineCodeSegments(s string) int {
	count := 0
	open := false
	for _, r := range s {
		if r == '`' {
			if open {
				count++
			}
			open = !open
		}
	}
	return count
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
			ClusterKey: blockClusterKey(blockID, "answer_claim_form_support"),
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
	}
	return out
}

func validateCallChainItemCitationRoleAlignment(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, mut *types.MutableState) []types.Violation {
	if doc == nil || mut == nil {
		return nil
	}
	allEvidence := answerItemLabelSupportEvidenceItems(mut)
	if len(allEvidence) == 0 {
		return nil
	}
	var out []types.Violation
	for _, b := range doc.Blocks {
		forms := answerBlockCitationRoleForms(b, view)
		if len(forms) == 0 {
			continue
		}
		for _, item := range b.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			expected, ok := answerClaimRoleMentionedByItemSurface(item, forms, allEvidence)
			if !ok {
				continue
			}
			cit := doc.Citations[item.CitationRef]
			cited := answerCitedEvidenceItems(mut, cit)
			if types.EvidenceSetContainsSameClaimRole(cited, expected) {
				continue
			}
			out = append(out, types.Violation{
				Kind: types.ViolClaimFormUnsupported,
				Detail: fmt.Sprintf(
					"answer item block=%q item=%q visibly names typed evidence role %s but citation_ref points at %s, whose evidence does not project to that same role",
					b.ID, item.ID, types.EvidenceClaimRoleName(expected), answerCitationLocation(cit)),
				Repair: fmt.Sprintf(
					"change item citation_ref to the citation for the matching typed evidence role at %s, or rewrite the item so it no longer asserts that role.",
					types.EvidenceClaimRoleLocation(expected)),
				ClusterKey: blockClusterKey(b.ID, "answer_item_citation_role"),
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_item_citation_role",
					Reason:     "answer item citation supports a different typed role than the visible item surface",
					Confidence: 0.86,
				},
				Stage: string(types.StageFinalize),
			})
		}
	}
	return out
}

func answerBlockCitationRoleForms(b types.AnswerBlock, view *types.AnswerSemanticView) []types.ClaimForm {
	switch b.Kind {
	case types.BlockOrderedList, types.BlockBulletList:
	default:
		return nil
	}
	var forms []types.ClaimForm
	for _, cu := range b.ClaimUses {
		forms = append(forms, cu.ClaimForm)
	}
	if view != nil {
		for _, req := range append(append([]types.BlockRequirement(nil), view.RequiredBlocks...), view.OptionalBlocks...) {
			if req.Kind != b.Kind || len(req.AcceptableClaimForms) == 0 {
				continue
			}
			if len(req.FacetIDs) > 0 && !answerBlockSharesFacet(b, req.FacetIDs) {
				continue
			}
			forms = append(forms, req.AcceptableClaimForms...)
		}
	}
	if answerBlockHasFacet(b, types.FacetPrincipalPathEdge) {
		forms = append(forms, types.ClaimCallEdge)
	}
	if view != nil && (view.Family == types.QFCallChain || view.Family == types.QFRootCauseTrace) &&
		answerBlockHasFacet(b, types.FacetCurrentCodePath) {
		forms = append(forms, types.ClaimCallEdge)
	}
	return types.ClaimFormsSupportingCitationRoleAlignment(forms)
}

func answerBlockHasFacet(b types.AnswerBlock, facet types.AnswerFacetKind) bool {
	want := string(facet)
	for _, id := range b.FacetIDs {
		if strings.TrimSpace(id) == want {
			return true
		}
	}
	for _, cu := range b.ClaimUses {
		if strings.TrimSpace(cu.FacetID) == want {
			return true
		}
	}
	return false
}

func answerBlockSharesFacet(b types.AnswerBlock, facets []string) bool {
	if len(facets) == 0 {
		return false
	}
	for _, facet := range facets {
		facet = strings.TrimSpace(facet)
		if facet == "" {
			continue
		}
		for _, id := range b.FacetIDs {
			if strings.TrimSpace(id) == facet {
				return true
			}
		}
		for _, cu := range b.ClaimUses {
			if strings.TrimSpace(cu.FacetID) == facet {
				return true
			}
		}
	}
	return false
}

func answerClaimRoleMentionedByItemSurface(item types.AnswerBlockItem, forms []types.ClaimForm, evidence []types.EvidenceItem) (types.EvidenceItem, bool) {
	surface := strings.TrimSpace(item.Label + "\n" + item.Text)
	if surface == "" {
		return types.EvidenceItem{}, false
	}
	for _, ev := range evidence {
		if types.EvidenceClaimRoleMentionedBySurface(ev, forms, surface) {
			return ev, true
		}
	}
	return types.EvidenceItem{}, false
}

func answerCitedEvidenceItems(mut *types.MutableState, cit types.Citation) []types.EvidenceItem {
	file := strings.TrimSpace(cit.File)
	if mut == nil || file == "" {
		return nil
	}
	var out []types.EvidenceItem
	for _, ev := range answerItemLabelSupportEvidenceItems(mut) {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if !answerCitationSourceMatches(file, ev.Source) {
			continue
		}
		if !citationLineMatchesEvidence(cit.Line, ev) {
			continue
		}
		out = append(out, ev)
	}
	return out
}

func answerCitationSourceMatches(citationFile, evidenceSource string) bool {
	citationFile = strings.TrimSpace(strings.ReplaceAll(citationFile, `\`, `/`))
	evidenceSource = strings.TrimSpace(strings.ReplaceAll(evidenceSource, `\`, `/`))
	return citationFile != "" && citationFile == evidenceSource
}

func answerCitationLocation(cit types.Citation) string {
	source := strings.TrimSpace(cit.File)
	if source == "" || cit.Line <= 0 {
		return "an unresolved citation"
	}
	return fmt.Sprintf("%s:%d", source, cit.Line)
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
		Kind:       types.ViolAbsenceScopeExceeded,
		Detail:     "exact_resolution.status=absent declared but no citation carries scope=negative + a non-empty negative_pattern; the absence claim is unbounded",
		Repair:     "Re-emit emit_answer_document with citations[] including at least one entry with scope='negative' AND a non-empty negative_pattern naming the exact search query (grep / repomap / file glob) that confirmed the absence. If the bounded evidence is already in the pool, attach it directly as a negative-scope citation; otherwise the next investigation pass must run the bounded search and surface its query.",
		ClusterKey: "root:exact_resolution.absence_scope",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "exact_resolution.absence_scope",
			Reason:     "absence claim without bounded negative-scope citation",
			Confidence: 0.65,
		},
		Stage: string(types.StageFinalize),
	}}
}

// validateMissingRequestedRoleDisclosure enforces the typed
// config-precedence exact-absence disclosure contract:
// AnswerSemanticView.MissingRequestedRoles is the authoritative list
// of user-requested precedence layers that remained unbound in the
// grounded evidence surface, and AnswerDocumentV2 must copy that list
// into document-level missing_requested_roles[] so the renderer can
// materialise explicit missing-layer prose deterministically.
func validateMissingRequestedRoleDisclosure(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}

	expected := make(map[types.EvidenceDiagramRole]string, len(view.MissingRequestedRoles))
	for _, item := range view.MissingRequestedRoles {
		if item.Role == types.EvidenceDiagramRoleUnknown {
			continue
		}
		expected[item.Role] = strings.TrimSpace(item.Label)
	}
	actual := make(map[types.EvidenceDiagramRole]string, len(doc.MissingRequestedRoles))
	for _, item := range doc.MissingRequestedRoles {
		if item.Role == types.EvidenceDiagramRoleUnknown {
			continue
		}
		actual[item.Role] = strings.TrimSpace(item.Label)
	}

	allowedSurface := view.Family == types.QFConfigPrecedence &&
		doc.ExactResolution != nil &&
		doc.ExactResolution.Status == types.AnswerExactResolutionAbsent
	if !allowedSurface {
		if len(actual) == 0 {
			return nil
		}
		return []types.Violation{{
			Kind:       types.ViolMissingRequestedRoleUndisclosed,
			Detail:     "document-level missing_requested_roles[] is only valid for config-precedence exact-absence answers, but this document emitted it outside that surface",
			Repair:     "Re-emit the answer without document-level missing_requested_roles[] unless this dispatch is a config-precedence exact-absence answer whose semantic view explicitly names missing requested layers.",
			ClusterKey: "root:missing_requested_roles",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "missing_requested_roles",
				Reason:     "typed missing-layer disclosure emitted on an incompatible answer surface",
				Confidence: 0.8,
			},
			Stage: string(types.StageFinalize),
		}}
	}
	if len(expected) == 0 && len(actual) == 0 {
		return nil
	}

	var missing []string
	var extra []string
	var labelMismatch []string
	for role, expectedLabel := range expected {
		actualLabel, ok := actual[role]
		if !ok {
			missing = append(missing, string(role))
			continue
		}
		if expectedLabel != "" && !strings.EqualFold(expectedLabel, actualLabel) {
			labelMismatch = append(labelMismatch, fmt.Sprintf("%s(label=%q, want %q)", role, actualLabel, expectedLabel))
		}
	}
	for role := range actual {
		if _, ok := expected[role]; !ok {
			extra = append(extra, string(role))
		}
	}
	if len(missing) == 0 && len(extra) == 0 && len(labelMismatch) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(labelMismatch)
	return []types.Violation{{
		Kind: types.ViolMissingRequestedRoleUndisclosed,
		Detail: fmt.Sprintf(
			"document-level missing_requested_roles[] does not match the semantic-view obligation (missing=%v extra=%v label_mismatch=%v)",
			missing, extra, labelMismatch),
		Repair: fmt.Sprintf(
			"Re-emit the answer with document-level missing_requested_roles[] exactly set to %s. Copy the role set from the semantic-view contract, preserve any user-facing labels (for example `CLI`), and let the renderer materialise the explicit missing-layer prose from this typed field.",
			formatMissingRequestedRolesForRepair(view.MissingRequestedRoles)),
		ClusterKey: "root:missing_requested_roles",
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "missing_requested_roles",
			Reason:     "required typed disclosure for missing requested precedence layers is absent or mismatched",
			Confidence: 0.82,
		},
		Stage: string(types.StageFinalize),
	}}
}

func formatMissingRequestedRolesForRepair(items []types.AnswerMissingRequestedRole) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Label) != "" {
			parts = append(parts, fmt.Sprintf(`{"role":"%s","label":"%s"}`, item.Role, strings.TrimSpace(item.Label)))
			continue
		}
		parts = append(parts, fmt.Sprintf(`{"role":"%s"}`, item.Role))
	}
	return "[" + strings.Join(parts, ", ") + "]"
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
	return runV2BlockOraclesWithOracle(doc, view, mut, nil)
}

// runV2BlockOraclesWithOracle is the oracle-aware variant. nil
// oracle preserves pre-fix-B behaviour (validators that opt into
// the oracle gate fall back to their pre-existing match-only
// semantics). Production caller in contract_check.go::runContractCheck
// threads the same SymbolOracle that the contract.CheckWithOracle
// path uses, so V2 block validators see the same Tier 1-2 truth
// table as the contract.must_include / must_exclude oracles.
func runV2BlockOraclesWithOracle(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, mut *types.MutableState, oracle types.SymbolOracle) []types.Violation {
	if doc == nil || view == nil {
		return nil
	}
	var out []types.Violation
	out = append(out, validateRequiredBlockCoverage(doc, view)...)
	out = append(out, validateCurrentStatusVerdict(doc, view)...)
	out = append(out, validatePrincipalClaimUse(doc, view)...)
	out = append(out, validateDiagramEdgeSupport(doc, view)...)
	out = append(out, validateUncertaintyBlockPresence(doc, view)...)
	out = append(out, validateFacetCoverage(doc, view)...)
	// v3 B2 (2026-05-04) — three-layer quality contract:
	//   layer 3a: ViolRichnessGlaringGap (Medium, retry-eligible)
	//   layer 3b: ViolPrincipalProseUnderfilled (Medium, retry-eligible)
	// Both wired BEFORE validateRichnessRegression so the suppression
	// rule inside validateRichnessRegression sees the glaring fire and
	// drops duplicate telemetry on the same facet.
	out = append(out, validateRichnessGlaringGap(doc, view)...)
	out = append(out, validatePrincipalProseUnderfilled(doc)...)
	out = append(out, validateRichnessRegression(doc, view)...)
	out = append(out, validateClaimFormSupport(doc, mut)...)
	out = append(out, validateCallChainItemCitationRoleAlignment(doc, view, mut)...)
	out = append(out, validateMissingRequestedRoleDisclosure(doc, view)...)
	out = append(out, validateAbsenceScopeBound(doc)...)
	out = append(out, validateEnumerationItemLabelGrounding(doc, mut)...)
	out = append(out, validateEnumerationItemLabelExtractorMatch(doc, view, mut, oracle)...)
	out = append(out, validateEnumerationItemLabelHallucination(doc, oracle, mut)...)
	out = append(out, validateDiagramEdgeEndpointHallucination(doc, oracle)...)
	out = append(out, validateInlineIdentifierHallucination(doc, oracle, mut)...)
	return out
}

func validateCurrentStatusVerdict(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []types.Violation {
	if doc == nil || view == nil || !requiresCurrentStatusDecision(view) {
		return nil
	}
	allowed := currentStatusAllowedVerdicts(view.CurrentStatusDiagnostic)
	var decision *types.AnswerBlock
	for i := range doc.Blocks {
		if doc.Blocks[i].Kind == types.BlockDecision {
			decision = &doc.Blocks[i]
			break
		}
	}
	if decision == nil {
		return []types.Violation{currentStatusVerdictViolation(
			"",
			"current-status diagnostic requires a principal decision block starting with still_present, fixed, or not_enough_evidence",
		)}
	}
	text := strings.TrimSpace(decision.Text)
	if currentStatusVerdictPrefix(text, allowed) == "" {
		return []types.Violation{currentStatusVerdictViolation(
			decision.ID,
			fmt.Sprintf("decision block %q must start with still_present, fixed, or not_enough_evidence", decision.ID),
		)}
	}
	return nil
}

func requiresCurrentStatusDecision(view *types.AnswerSemanticView) bool {
	return view != nil &&
		view.CurrentStatusDiagnostic != nil &&
		view.CurrentStatusDiagnostic.Required
}

func currentStatusAllowedVerdicts(contract *types.CurrentStatusDiagnosticContract) []types.CurrentStatusVerdict {
	if contract != nil && len(contract.AllowedVerdicts) > 0 {
		out := make([]types.CurrentStatusVerdict, 0, len(contract.AllowedVerdicts))
		seen := map[types.CurrentStatusVerdict]bool{}
		for _, verdict := range contract.AllowedVerdicts {
			if verdict == "" || seen[verdict] {
				continue
			}
			seen[verdict] = true
			out = append(out, verdict)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []types.CurrentStatusVerdict{
		types.CurrentStatusStillPresent,
		types.CurrentStatusFixed,
		types.CurrentStatusNotEnoughEvidence,
	}
}

func currentStatusVerdictPrefix(text string, allowed []types.CurrentStatusVerdict) types.CurrentStatusVerdict {
	head := strings.TrimSpace(strings.ToLower(text))
	for _, verdict := range allowed {
		s := string(verdict)
		if s == "" {
			continue
		}
		if head == s || strings.HasPrefix(head, s+" ") ||
			strings.HasPrefix(head, s+":") ||
			strings.HasPrefix(head, s+".") ||
			strings.HasPrefix(head, s+"\n") ||
			strings.HasPrefix(head, s+"\t") {
			return verdict
		}
	}
	return ""
}

func currentStatusVerdictViolation(blockID, detail string) types.Violation {
	if strings.TrimSpace(detail) == "" {
		detail = "current-status diagnostic verdict missing"
	}
	key := "current_status_verdict"
	if strings.TrimSpace(blockID) != "" {
		key = blockClusterKey(blockID, "current_status_verdict")
	}
	return types.Violation{
		Kind:       types.ViolCurrentStatusVerdictMissing,
		Detail:     detail,
		Repair:     "emit a principal decision block whose text starts with exactly one of still_present, fixed, or not_enough_evidence, then explain the current-code evidence boundary",
		ClusterKey: key,
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "answer_contract.current_status_diagnostic",
			Reason:     "current-status diagnostic verdict missing or outside allowed enum",
			Confidence: 1.0,
		},
		Stage: string(types.StageFinalize),
	}
}

// labelLeadingSymbolIdentifier returns the leading identifier-shape
// token of a list-item label IFF the label is shaped like
// "<identifier>" or "<identifier><separator><prose>" — i.e., starts
// with an identifier-shape token followed by EITHER end-of-string OR
// a punctuation/non-alphanumeric character that signals the
// identifier ended.
//
// Returns "" if the label continues into more letters / digits
// after the leading identifier (which means the label is prose like
// "Step 1: read evidence", not a symbol reference, and should not
// be subject to the SymbolOracle existence gate).
//
// Examples:
//
//	"checkCoverage"                  → "checkCoverage"     (bare ident)
//	"checkCoverage — 资源检查"        → "checkCoverage"     (ident + em-dash separator)
//	"checkCoverage (note)"           → "checkCoverage"     (ident + paren)
//	"checkCoverage: desc"            → "checkCoverage"     (ident + colon)
//	"Step 1: read evidence"          → ""                  (ident continues into digit prose)
//	"the checkCoverage function"     → "the"               (3-char leading; caller filters by length)
//	""                               → ""
func labelLeadingSymbolIdentifier(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	end := 0
	for end < len(label) {
		c := label[end]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return ""
	}
	rest := strings.TrimLeftFunc(label[end:], unicode.IsSpace)
	if rest == "" {
		return label[:end]
	}
	r, _ := utf8.DecodeRuneInString(rest)
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		// Leading identifier is followed by more identifier-shape
		// content (after just whitespace) — this is prose, not a
		// symbol reference.
		return ""
	}
	return label[:end]
}

// labelStartsWithQualifiedCodeIdentity reports whether a rendered
// list label begins with a cross-language qualified identity surface
// rather than a standalone declaration name. Examples include Go
// package selectors (`normalizer.Normalize`), C++ namespaces
// (`std::vector`), JS/ArkTS packages (`@scope/pkg.Symbol`), and
// path/module-qualified names (`entry/src/main/ets/pages/Index.ets`).
//
// These surfaces are still code identities, but the qualifier is not
// necessarily a declaration indexed by SymbolOracle. Hard hallucination
// checks for them must therefore come from typed evidence / citation
// alignment, not from asking the declaration oracle whether the leading
// qualifier exists as a function/type/constant.
func labelStartsWithQualifiedCodeIdentity(label string) bool {
	token := leadingCodeIdentityToken(label)
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "@") {
		return true
	}
	return strings.Contains(token, ".") ||
		strings.Contains(token, "::") ||
		strings.Contains(token, "/") ||
		strings.Contains(token, `\`)
}

func leadingCodeIdentityToken(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	end := 0
	for end < len(label) {
		c := label[end]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '.', c == ':', c == '/', c == '\\', c == '-', c == '@':
			end++
			continue
		default:
			goto done
		}
	}
done:
	if end == 0 {
		return ""
	}
	token := strings.Trim(label[:end], ":-")
	if token == "" || !types.IsCodeIdentitySurface(token) {
		return ""
	}
	return token
}

// labelHallucinationGateLengthFloor is the minimum identifier
// length subject to the SymbolOracle existence gate. Below this
// floor we trust the evidence-pool substring match and skip the
// oracle check, because short identifiers (≤9 chars) are
// disproportionately stdlib helpers (`Sprintf`, `Println`, `New`,
// `Get`, …) that the project's repomap doesn't index but which
// LLMs legitimately reference. Hallucinations observed in
// production (s1a r1: `checkNamingConvention` 20 chars,
// `checkSignalSufficiency` 22 chars) are well above this floor.
const labelHallucinationGateLengthFloor = 10

// validateEnumerationItemLabelGrounding (post-shape s1a-20260504-064754
// hallucination forensic, 2026-05-04) checks that every
// ordered_list / bullet_list block's symbol/runtime-shaped
// items[i].label is supported by at least one EvidenceItem in the
// dispatch's evidence pool — by substring match against AnchorSymbol
// / Subject / Object. Display / role / prose labels are not treated
// as symbol claims; when their item carries a valid citation, the
// citation grounds the item and this label-specific oracle skips. The
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
//
// Match semantics: case-folded substring match in BOTH directions
// (label-contains-anchor OR anchor-contains-label) so a label like
// `checkCoverage` matches an evidence anchor like `checkCoverage(ir,
// th)` and a fragment-style anchor like `Coverage` matches the
// label `checkCoverage`. This mirrors the diagram edge oracle's
// `diagramTokenSupported` semantics (R4.3) so the two oracles
// behave consistently across enumeration vs diagram surfaces.
//
// Note (Fix C, 2026-05-07): bidirectional substring vouching has a
// known noise mode — short prose tokens (`signal`, `naming`,
// `coverage`) appearing in any evidence summary / anchor label can
// vouch for HALLUCINATED symbol-shape labels. The hallucination case
// is handled separately by validateEnumerationItemLabelHallucination
// (graph-existence oracle); this validator keeps its original
// "evidence-pool grounding" semantics so the two failure modes
// remain distinct typed signals (different repair paths, severity,
// telemetry).
func validateEnumerationItemLabelGrounding(doc *types.AnswerDocumentV2, mut *types.MutableState) []types.Violation {
	if doc == nil || mut == nil {
		return nil
	}
	// Use the memoised support pool — Turn A artifacts are frozen for
	// the lifetime of an extract / finalize cycle, so re-running the
	// regex + suffix expansion on every contract check (including
	// every retry) was wasted work. The cache is keyed off the
	// internal turnAArtifacts pointer and invalidated on Set / Reset.
	support := mut.CachedLabelSupportTokens(buildEvidenceLabelSupportTokens)
	if emitted := mut.EmittedEvidence(); len(emitted) > 0 {
		if support == nil {
			support = map[string]struct{}{}
		}
		for token := range buildEvidenceLabelSupportTokens(emitted) {
			support[token] = struct{}{}
		}
	}
	if len(support) == 0 {
		return nil
	}
	type ungroundedItem struct {
		itemID string
		label  string
	}
	perBlock := make(map[string][]ungroundedItem)
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList {
			continue
		}
		var blockUngrounded []ungroundedItem
		for _, it := range b.Items {
			label := strings.TrimSpace(it.Label)
			if label == "" {
				continue
			}
			if it.CitationRef >= 0 && it.CitationRef < len(doc.Citations) &&
				types.AnswerSourceLocationLabelMatchesCitation(label, doc.Citations[it.CitationRef]) {
				continue
			}
			if !answerItemLabelNeedsEvidenceToken(label) && answerItemHasResolvedCitation(doc, it) {
				continue
			}
			if !answerItemLabelNeedsEvidenceToken(label) {
				continue
			}
			if answerItemLabelSupportedByQuestionBucket(label, mut) {
				continue
			}
			if answerItemLabelSupportedByRuntimeArtifact(label, mut) {
				continue
			}
			if diagramTokenSupported(label, support) {
				continue
			}
			if answerItemLabelSupportedByAnswerSymbol(label, mut) {
				continue
			}
			blockUngrounded = append(blockUngrounded, ungroundedItem{
				itemID: it.ID,
				label:  label,
			})
		}
		if len(blockUngrounded) > 0 {
			perBlock[b.ID] = blockUngrounded
		}
	}
	if len(perBlock) == 0 {
		return nil
	}
	blockIDs := make([]string, 0, len(perBlock))
	for blockID := range perBlock {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	out := make([]types.Violation, 0, len(blockIDs))
	for _, blockID := range blockIDs {
		ungrounded := perBlock[blockID]
		pairs := make([]string, 0, len(ungrounded))
		for _, u := range ungrounded {
			pairs = append(pairs, fmt.Sprintf("block=%q item=%q label=%q", blockID, u.itemID, u.label))
		}
		out = append(out, types.Violation{
			Kind: types.ViolEnumerationLabelUngrounded,
			Detail: fmt.Sprintf(
				"block %q has %d symbol/runtime-shaped enumeration item label(s) that do not match any grounded support token (evidence anchor / grounded snippet selector): [%s]",
				blockID, len(ungrounded), strings.Join(pairs, "; ")),
			Repair: "for each listed symbol/runtime label, copy the label verbatim from a grounded support token — anchor_symbol, subject/object, or a selector/type identifier visibly present on a grounded snippet line — or replace the item label with display prose and keep the citation on the item. Fabricating identifiers that the evidence does not name is silently misleading; if the answer truly requires an item that no evidence supports, reopen the investigation rather than inventing a label.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_items_label",
				Reason:     "symbol/runtime-shaped items[].label not supported by any evidence pool anchor",
				Confidence: 0.85,
			},
			ClusterKey: blockClusterKey(blockID, "block_items_label"),
			Stage:      string(types.StageFinalize),
		})
	}
	return out
}

type answerItemLabelIntent string

const (
	answerItemLabelIntentDisplay answerItemLabelIntent = "display"
	answerItemLabelIntentSymbol  answerItemLabelIntent = "symbol"
	answerItemLabelIntentRuntime answerItemLabelIntent = "runtime"
)

var labelFileLineTokenRE = regexp.MustCompile(`(?:^|[\s([])(?:[A-Za-z0-9_.@-]+/)+[A-Za-z0-9_.@-]+:\d+\b`)

func classifyAnswerItemLabelIntent(label string) answerItemLabelIntent {
	label = strings.TrimSpace(label)
	if label == "" {
		return answerItemLabelIntentDisplay
	}
	if labelFileLineTokenRE.MatchString(label) {
		return answerItemLabelIntentRuntime
	}
	if snippetSelectorTokenRE.MatchString(label) {
		return answerItemLabelIntentSymbol
	}
	if contract.IsIdentifierShaped(label) {
		return answerItemLabelIntentSymbol
	}
	if ident := labelLeadingSymbolIdentifier(label); ident != "" && len(ident) >= 4 {
		return answerItemLabelIntentSymbol
	}
	return answerItemLabelIntentDisplay
}

func answerItemLabelNeedsEvidenceToken(label string) bool {
	switch classifyAnswerItemLabelIntent(label) {
	case answerItemLabelIntentSymbol, answerItemLabelIntentRuntime:
		return true
	default:
		return false
	}
}

func answerItemHasResolvedCitation(doc *types.AnswerDocumentV2, item types.AnswerBlockItem) bool {
	if doc == nil || item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
		return false
	}
	cit := doc.Citations[item.CitationRef]
	return strings.TrimSpace(cit.File) != "" || strings.TrimSpace(cit.Quote) != ""
}

func answerItemLabelSupportedByCitedEvidenceSubject(doc *types.AnswerDocumentV2, item types.AnswerBlockItem, label string, mut *types.MutableState) bool {
	label = strings.TrimSpace(label)
	if doc == nil {
		return false
	}
	if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
		return false
	}
	cit := doc.Citations[item.CitationRef]
	if types.AnswerSourceLocationLabelMatchesCitation(label, cit) {
		return true
	}
	if mut == nil || !types.IsCodeIdentitySurface(label) {
		return false
	}
	citFile := strings.TrimSpace(cit.File)
	if citFile == "" {
		return false
	}
	evidence := answerItemLabelSupportEvidenceItems(mut)
	if len(evidence) == 0 {
		return false
	}
	for _, ev := range evidence {
		if strings.TrimSpace(ev.Source) != citFile {
			continue
		}
		if !citationLineMatchesEvidence(cit.Line, ev) {
			continue
		}
		if answerCodeSurfaceMatchesEvidenceEndpoint(label, ev) {
			return true
		}
	}
	if answerItemHasResolvedCitation(doc, item) {
		return false
	}
	return answerItemLabelSupportedByEvidenceEndpoint(label, mut)
}

func answerItemLabelSupportedByEvidenceEndpoint(label string, mut *types.MutableState) bool {
	label = strings.TrimSpace(label)
	if mut == nil || !types.IsCodeIdentitySurface(label) {
		return false
	}
	evidence := answerItemLabelSupportEvidenceItems(mut)
	if len(evidence) == 0 {
		return false
	}
	for _, ev := range evidence {
		if ev.GroundingStatus == types.GroundingUngrounded {
			continue
		}
		if answerCodeSurfaceMatchesEvidenceEndpoint(label, ev) {
			return true
		}
	}
	return false
}

func answerItemLabelSupportEvidenceItems(mut *types.MutableState) []types.EvidenceItem {
	if mut == nil {
		return nil
	}
	var out []types.EvidenceItem
	if artifacts := mut.TurnAArtifacts(); artifacts != nil && len(artifacts.EvidenceItems) > 0 {
		out = append(out, artifacts.EvidenceItems...)
	}
	if emitted := mut.EmittedEvidence(); len(emitted) > 0 {
		out = append(out, emitted...)
	}
	return out
}

func answerCodeSurfaceMatchesEvidenceEndpoint(label string, ev types.EvidenceItem) bool {
	for _, endpoint := range []string{ev.Subject, ev.Object, ev.AnchorSymbol, ev.OwnerSymbol} {
		if answerCodeSurfaceMatches(label, endpoint) {
			return true
		}
	}
	for _, term := range ev.SurfaceTerms {
		if answerCodeSurfaceAppearsVerbatim(label, term) {
			return true
		}
	}
	if answerCodeSurfaceAppearsVerbatim(label, ev.Snippet) {
		return true
	}
	return false
}

func answerCodeSurfaceAppearsVerbatim(label, text string) bool {
	label = strings.TrimSpace(label)
	text = strings.TrimSpace(text)
	if label == "" || text == "" || !types.IsCodeIdentitySurface(label) {
		return false
	}
	return strings.Contains(text, label)
}

func answerCodeSurfaceMatches(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if strings.EqualFold(a, b) {
		return true
	}
	if types.IsCodeIdentitySurface(a) && types.IsCodeIdentitySurface(b) {
		aFold := strings.ToLower(strings.TrimSpace(a))
		bFold := strings.ToLower(strings.TrimSpace(b))
		if strings.Contains(aFold, bFold) || strings.Contains(bFold, aFold) {
			return true
		}
	}
	aTail := types.NormalizedSurfaceSymbolTail(a)
	bTail := types.NormalizedSurfaceSymbolTail(b)
	return aTail != "" && bTail != "" && strings.EqualFold(aTail, bTail)
}

func answerItemLabelSupportedByAnswerSymbol(label string, mut *types.MutableState) bool {
	label = strings.TrimSpace(label)
	if mut == nil || label == "" {
		return false
	}
	symbols, _ := mut.EmittedAnswerSymbols()
	for _, sym := range symbols {
		name := strings.TrimSpace(sym.Name)
		if name == "" || sym.File == "" || sym.Line <= 0 {
			continue
		}
		if label == name {
			return true
		}
	}
	return false
}

func answerItemLabelSupportedByQuestionBucket(label string, mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	rm := mut.RequestModel()
	if rm == nil {
		return false
	}
	for _, bucket := range rm.QuestionStructure().Buckets {
		if typedLabelTokenSupportsLabel(bucket.Label, label) {
			return true
		}
	}
	return false
}

func answerItemLabelSupportedByRuntimeArtifact(label string, mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	if bundle := mut.LogTriage(); bundle != nil {
		if runtimeLabelSupportedByLogBundle(label, bundle) {
			return true
		}
	}
	if perf := mut.PerfTrace(); perf != nil {
		for _, frame := range perf.LogFrames() {
			if runtimeLabelSupportedByFrame(label, frame) {
				return true
			}
		}
	}
	return false
}

func runtimeLabelSupportedByLogBundle(label string, bundle *types.LogBundle) bool {
	if bundle == nil {
		return false
	}
	for _, signal := range bundle.Meta.Signals {
		if typedLabelTokenSupportsLabel(string(signal), label) {
			return true
		}
	}
	matched := false
	types.WalkLogErrors(bundle, func(err *types.LogError) {
		if matched || err == nil {
			return
		}
		if typedLabelTokenSupportsLabel(err.Type, label) ||
			typedLabelTokenSupportsLabel(err.Message, label) {
			matched = true
		}
	})
	if matched {
		return true
	}
	types.WalkLogFrames(bundle, func(frame types.LogFrame) {
		if matched {
			return
		}
		if runtimeLabelSupportedByFrame(label, frame) {
			matched = true
		}
	})
	return matched
}

func runtimeLabelSupportedByFrame(label string, frame types.LogFrame) bool {
	return typedLabelTokenSupportsLabel(frame.Lang, label) ||
		typedLabelTokenSupportsLabel(frame.Func, label) ||
		typedLabelTokenSupportsLabel(frame.Pkg, label) ||
		typedLabelTokenSupportsLabel(frame.File, label) ||
		typedLabelTokenSupportsLabel(frame.Raw, label)
}

func typedLabelTokenSupportsLabel(token, label string) bool {
	token = strings.TrimSpace(token)
	label = strings.TrimSpace(label)
	if token == "" || label == "" {
		return false
	}
	t := strings.ToLower(token)
	l := strings.ToLower(label)
	if l == t {
		return true
	}
	if strings.HasPrefix(l, t) && labelBoundaryAfter(l, len(t)) {
		return true
	}
	if strings.HasPrefix(t, l) && labelBoundaryAfter(t, len(l)) {
		return true
	}
	return false
}

func labelBoundaryAfter(s string, n int) bool {
	if n >= len(s) {
		return true
	}
	if s[n] >= 0x80 {
		return true
	}
	switch s[n] {
	case ' ', '\t', '\r', '\n', '.', ':', '/', '\\', '-', '_', '(', ')', '[', ']', '{', '}', ',', ';':
		return true
	}
	return false
}

func citationLineMatchesEvidence(line int, ev types.EvidenceItem) bool {
	start := ev.LineStart
	end := ev.LineEnd
	if start <= 0 {
		return true
	}
	if line <= 0 {
		return true
	}
	if end <= 0 {
		end = start
	}
	return line >= start && line <= end
}

// validateEnumerationItemLabelExtractorMatch (s1a-20260504-130143
// abstraction-drift forensic, 2026-05-04) checks that finalizer's
// items[i].label preserves the verbatim names from the extractor's
// emit_answer_symbol output. Distinct from
// validateEnumerationItemLabelGrounding which only verifies the
// label is a SUBSTRING of any evidence pool anchor — that lenient
// check passes "check 1 (gate.go:148)" because "check" appears in
// evidence prose, but the user reading the answer cannot recover
// the real method names checkCoverage / checkDAGClosure / etc.
//
// Trigger conditions (all required, R3 typed):
//   - mut != nil and the view carries an extractor-backed
//     identifier slate obligation:
//   - QFEnumeration always qualifies
//   - non-enumeration families qualify only when the typed
//     FacetCoverage includes FacetEnumerationItem (today this is
//     the bounded-principal-set path for call-chain /
//     root-cause-trace questions)
//   - extractor emitted at least 3 AnswerSymbols (small slates
//     are noise-prone; below 3 the LLM may legitimately choose
//     framing labels)
//   - principal ordered_list / bullet_list block with at least
//     3 items
//
// Match rule (four-stage, in order):
//
//  1. Exact case-folded equality between items[i].label (trimmed)
//     and one of AnswerSymbols[j].Name.
//  2. Relaxed substring: items[].label wraps the verbatim name
//     with surrounding text ("checkCoverage — 资源检查").
//  3. Token-form-aware (Fix B, 2026-05-07 s5a r2 forensic):
//     flatten both label and slate names by stripping `_`/`-` and
//     case-folding, then check substring in either direction.
//     Catches snake↔camel↔Pascal↔kebab↔SCREAMING_SNAKE↔flatcase
//     equivalences that the extractor and finalizer LLMs spell
//     differently. Reuses contract.FlattenIdentifier so the
//     equivalence rule is identical to the contract.containsSymbol
//     fallback applied at must_include / must_exclude / acceptance
//     check sites — single source of truth across the contract
//     layer for token-form normalisation.
//  4. SymbolOracle gate (Fix B, 2026-05-07 s5a r2 forensic): when
//     `oracle` is non-nil and the label is identifier-shaped, the
//     label counts as matched if the oracle confirms it as a real
//     Tier 1-2 codebase symbol — even when the extractor's slate
//     missed the identifier or spelled it with a typo. This is
//     the same precision/noise red-line the contract.must_include
//     oracle gate applies: extractor LLM emit is a noisy signal
//     (we observed real cases where it missed 3 of 8 implementers
//     and typo'd 2 more); typed_graph is the precise signal.
//
// Language coverage (multi-language generalisation): both the
// flat-form match (stage 3) and the SymbolOracle gate (stage 4)
// are language-agnostic. Stage 3's flatten handles every casing
// convention used by the languages repomap supports — Go (camel /
// Pascal), Python (snake / Pascal), Rust (snake / Pascal /
// SCREAMING_SNAKE), Swift (camel / Pascal), Java (camel / Pascal /
// SCREAMING_SNAKE), Ruby (snake / Pascal), JS/TS (camel / Pascal),
// ArkTS (camel / Pascal), Cangjie (camel / Pascal / snake), Kotlin
// (camel / Pascal). Stage 4's oracle is repomap-backed, which
// already indexes every supported language uniformly, so the
// validator behaves identically regardless of the active
// codebase's source language.
//
// Threshold: at least 80% of items must match an AnswerSymbol.
// Below 80% the violation fires with the list of drifted labels
// + the verbatim names the LLM should have used.
//
// Default classification: STRICT — verbatim identifier
// preservation is the contract between extractor (selects) and
// finalizer (renders). Operators can soften via
// pipeline_contract_strict_kinds yaml.
func validateEnumerationItemLabelExtractorMatch(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, mut *types.MutableState, oracle types.SymbolOracle) []types.Violation {
	if doc == nil || view == nil || mut == nil {
		return nil
	}
	if !viewNeedsExtractorBackedEnumerationSlate(view) {
		return nil
	}
	symbols, _ := mut.EmittedAnswerSymbols()
	if len(symbols) < 3 {
		return nil
	}
	// Build a case-folded lookup of verbatim extractor names.
	nameSet := make(map[string]struct{}, len(symbols))
	flatNameSet := make(map[string]struct{}, len(symbols))
	verbatimNames := make([]string, 0, len(symbols))
	for _, s := range symbols {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		nameSet[strings.ToLower(n)] = struct{}{}
		if flat := contract.FlattenIdentifier(n); len(flat) >= 4 {
			flatNameSet[flat] = struct{}{}
		}
		verbatimNames = append(verbatimNames, n)
	}
	if len(nameSet) == 0 {
		return nil
	}
	type drifted struct {
		itemID string
		label  string
	}
	perBlock := make(map[string][]drifted)
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList {
			continue
		}
		if len(b.Items) < 3 {
			continue
		}
		matched := 0
		blockDrifts := make([]drifted, 0, len(b.Items))
		for _, it := range b.Items {
			label := strings.TrimSpace(it.Label)
			if label == "" {
				continue
			}
			if answerItemLabelSupportedByCitedEvidenceSubject(doc, it, label, mut) {
				matched++
				continue
			}
			ll := strings.ToLower(label)
			if _, exact := nameSet[ll]; exact {
				matched++
				continue
			}
			// Stage 2 — relaxed: allow label to contain the verbatim
			// name as substring ("checkCoverage — 资源检查"). The
			// verbatim name appearing somewhere in the label still
			// preserves the identifier the user needs.
			containsName := false
			for vname := range nameSet {
				if strings.Contains(ll, vname) {
					containsName = true
					break
				}
			}
			if containsName {
				matched++
				continue
			}
			// Stage 3 — token-form-aware (Fix B): flatten both sides
			// to strip _/-, lowercase, then check substring either
			// way. snake↔camel↔Pascal↔kebab all collapse to the same
			// flat form, so spelling-form drift between extractor
			// and finalizer no longer fires.
			flatLabel := contract.FlattenIdentifier(label)
			if len(flatLabel) >= 4 {
				flatHit := false
				for fname := range flatNameSet {
					if strings.Contains(flatLabel, fname) || strings.Contains(fname, flatLabel) {
						flatHit = true
						break
					}
				}
				if flatHit {
					matched++
					continue
				}
			}
			// Stage 4 — SymbolOracle gate (Fix B): when the oracle
			// confirms the label is a real Tier 1-2 codebase symbol,
			// trust the finalizer over the (noisy) extractor slate.
			// shouldOracleGateInclude restricts this to identifier-
			// shaped tokens ≥4 chars so prose like "the dispatcher"
			// can never satisfy via this stage.
			if oracle != nil && contract.ShouldOracleGateInclude(label) {
				if found, tier := oracle.SymbolExists(label); found && tier < 3 {
					matched++
					continue
				}
			}
			blockDrifts = append(blockDrifts, drifted{
				itemID: it.ID,
				label:  label,
			})
		}
		// 80% threshold per block: < 80% match → fire for this block.
		if len(b.Items) > 0 && float64(matched)/float64(len(b.Items)) < 0.80 {
			perBlock[b.ID] = append([]drifted(nil), blockDrifts...)
		}
	}
	if len(perBlock) == 0 {
		return nil
	}
	blockIDs := make([]string, 0, len(perBlock))
	for blockID := range perBlock {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	out := make([]types.Violation, 0, len(blockIDs))
	for _, blockID := range blockIDs {
		driftedItems := perBlock[blockID]
		pairs := make([]string, 0, len(driftedItems))
		for _, d := range driftedItems {
			pairs = append(pairs, fmt.Sprintf("block=%q item=%q label=%q", blockID, d.itemID, d.label))
		}
		out = append(out, types.Violation{
			Kind: types.ViolEnumerationItemLabelExtractorDrift,
			Detail: fmt.Sprintf(
				"block %q has %d enumeration item label(s) drifted from the verbatim identifiers selected upstream; the answer should preserve these names verbatim: [%s]. Drifted items: [%s]",
				blockID, len(driftedItems), strings.Join(verbatimNames, ", "), strings.Join(pairs, "; ")),
			Repair: "for each listed item, copy the verbatim identifier from the typed symbol slate that earlier stages produced (the names are the typed identifiers selected upstream for the user to read). Abstract placeholders like 'check 1 (line N)' lose the real names the user needs to navigate the codebase. The upstream signal is intact; only the rendering needs to copy the names.",
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_items_label",
				Reason:     "answer rendering used placeholder labels instead of preserving the verbatim identifiers selected upstream",
				Confidence: 0.85,
			},
			ClusterKey: blockClusterKey(blockID, "block_items_label"),
			Stage:      string(types.StageFinalize),
		})
	}
	return out
}

func viewNeedsExtractorBackedEnumerationSlate(view *types.AnswerSemanticView) bool {
	if view == nil {
		return false
	}
	if view.Family == types.QFEnumeration {
		return true
	}
	if view.FacetCoverage == nil {
		return false
	}
	for _, req := range view.FacetCoverage.Required {
		if req.Kind == types.FacetEnumerationItem {
			return true
		}
	}
	return false
}

// validateEnumerationItemLabelHallucination (Fix C, s1a-20260507
// hallucination forensic) verifies that every list-item's leading
// identifier resolves to a Tier 1-2 codebase symbol via the typed
// graph SymbolOracle. The validator is the precise typed-graph
// counterpart to validateEnumerationItemLabelGrounding's evidence-
// pool substring vouch — short prose tokens ("signal", "naming")
// in evidence summaries can vouch for a hallucinated name like
// `checkSignalSufficiency` even though no source file declares such
// a function. The graph oracle catches BOTH the prose-vouched
// hallucinations (which Ungrounded misses) and the prose-not-vouched
// hallucinations (where Ungrounded would also fire — this rule
// pre-empts the duplicate via cooccurrence rule C6.2).
//
// Trigger conditions (all required, defensively narrow):
//
//   - oracle != nil — unit-test path / no-graph runs disable the
//     validator.
//   - block kind ∈ { ordered_list, bullet_list, table } — only
//     surfaces that carry "this is a list of named symbols"
//     semantics; scalar / decision / diagram / section / summary
//     have their own oracle paths.
//   - label is shaped <identifier>[<separator><prose>] — prose
//     labels like "Step 1: read evidence" skip via
//     labelLeadingSymbolIdentifier returning "" when the leading
//     identifier continues into more letters/digits (i.e., the
//     label is not a symbol reference).
//   - leading identifier length ≥ labelHallucinationGateLengthFloor
//     (10 chars) — short identifiers are disproportionately stdlib
//     helpers (Sprintf / Println / New / Get) that the project's
//     repomap doesn't index but which LLMs legitimately reference.
//     Production hallucinations observed (s1a r1:
//     `checkNamingConvention` 20 chars, `checkSignalSufficiency`
//     22 chars) are well above this floor.
//   - oracle returns Tier=0 (not found) OR Tier ≥ 3 (low-confidence
//     parse) — same precision contract as contract.must_include /
//     extractor_match oracle gates: Tier 1-2 is "real symbol",
//     anything else is "not vouched".
//
// Default classification: Medium severity, retry-eligible
// finalizer-only (the extractor's slate is fine; only the rendered
// label is wrong). Operators promote to STRICT via
// pipeline_contract_strict_kinds when answer-quality regressions
// are intolerable.
func validateEnumerationItemLabelHallucination(doc *types.AnswerDocumentV2, oracle types.SymbolOracle, mutOpt ...*types.MutableState) []types.Violation {
	if doc == nil || oracle == nil {
		return nil
	}
	var mut *types.MutableState
	if len(mutOpt) > 0 {
		mut = mutOpt[0]
	}
	type hallucinated struct {
		itemID string
		label  string
		ident  string
	}
	perBlock := make(map[string][]hallucinated)
	for _, b := range doc.Blocks {
		if b.Kind != types.BlockOrderedList && b.Kind != types.BlockBulletList && b.Kind != types.BlockTable {
			continue
		}
		var blockHits []hallucinated
		for _, it := range b.Items {
			label := strings.TrimSpace(it.Label)
			if label == "" {
				continue
			}
			if answerItemLabelSupportedByCitedEvidenceSubject(doc, it, label, mut) {
				continue
			}
			if answerItemLabelSupportedByEvidenceEndpoint(label, mut) {
				continue
			}
			if answerItemLabelSupportedByQuestionBucket(label, mut) {
				continue
			}
			if answerItemLabelSupportedByRuntimeArtifact(label, mut) {
				continue
			}
			if answerItemLabelSupportedByAnswerSymbol(label, mut) {
				continue
			}
			if labelStartsWithQualifiedCodeIdentity(label) {
				continue
			}
			ident := labelLeadingSymbolIdentifier(label)
			if len(ident) < labelHallucinationGateLengthFloor {
				continue
			}
			if !contract.IsIdentifierShaped(ident) {
				continue
			}
			// Fix E: cross-form existence — yaml-tag / CLI-flag /
			// snake-case / Pascal-case renderings of the same logical
			// identifier all collapse to the same flat-form key in the
			// graph oracle's lazy flat index, so `pipeline_max_steps`
			// (yaml form) resolves the same Go field as
			// `PipelineMaxSteps` (Pascal form).
			found, tier := oracle.SymbolExistsFlat(ident)
			if found && tier < 3 {
				continue
			}
			blockHits = append(blockHits, hallucinated{
				itemID: it.ID,
				label:  label,
				ident:  ident,
			})
		}
		if len(blockHits) > 0 {
			perBlock[b.ID] = blockHits
		}
	}
	if len(perBlock) == 0 {
		return nil
	}
	blockIDs := make([]string, 0, len(perBlock))
	for blockID := range perBlock {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	out := make([]types.Violation, 0, len(blockIDs))
	for _, blockID := range blockIDs {
		hits := perBlock[blockID]
		pairs := make([]string, 0, len(hits))
		for _, h := range hits {
			pairs = append(pairs, fmt.Sprintf("block=%q item=%q ident=%q label=%q", blockID, h.itemID, h.ident, h.label))
		}
		out = append(out, types.Violation{
			Kind: types.ViolEnumerationLabelHallucinated,
			Detail: fmt.Sprintf(
				"block %q has %d enumeration item label(s) whose leading identifier does not match any function / type / constant declared in the codebase (likely fabricated identifier): [%s]",
				blockID, len(hits), strings.Join(pairs, "; ")),
			Repair:     "the leading identifier in each listed label is a fabricated name that no source file declares. Replace with a real identifier the answer should reference — copy verbatim from a grounded evidence anchor or symbol slate. Fabricated names mislead the reader: the file:line citation will point at a different declaration than the rendered name claims.",
			ClusterKey: blockClusterKey(blockID, "block_items_label"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_items_label",
				Reason:     "answer rendering used identifiers the codebase does not declare (fabricated names)",
				Confidence: 0.9,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// isCodeContextDiagramKind reports whether the diagram's kind
// declaration ITSELF asserts the diagram's nodes must be codebase
// symbols. Only DiagramCallDAG qualifies — call DAGs by definition
// connect code-callable entities (functions / methods). Other
// kinds (flow, sequence, architecture) routinely use abstract role
// labels (`Analyzer`, `HTTP Handler`, `Cache Layer`) without code
// grounding, per the explicit "conceptual role labels are fine
// without a citation" prompt contract in skill/defaults.go.
func isCodeContextDiagramKind(k types.DiagramKind) bool {
	return k == types.DiagramCallDAG
}

// isCodeContextRelationKind reports whether a typed relation
// declares the SPECIFIC edge as connecting code-callable entities.
// Only DiagramRelCall qualifies — call edges are by definition
// caller→callee where both ends are function/method names. Other
// relations connect packages (Import), conceptual roles (Contain
// in architecture), abstract states (Guard / Precedence /
// Observe) where endpoints aren't required to be Go-source
// symbols.
func isCodeContextRelationKind(r types.DiagramRelationKind) bool {
	return r == types.DiagramRelCall
}

// validateDiagramEdgeEndpointHallucination (Fix D, 2026-05-07
// post-Fix-C diagram audit; refined by Fix F, 2026-05-07
// post-m1a-r1 false-positive forensic) verifies that every mermaid
// edge endpoint (the `from` and `to` node identifiers) in a
// code-context diagram resolves to a Tier 1-2 codebase symbol via
// the typed graph SymbolOracle.
//
// Fix F refinement — code-context typed gate (m1a r1 forensic):
// Fix D's initial scope (any BlockDiagram with identifier-shape
// endpoints ≥ 10 chars) over-fired on architecture / flow /
// sequence diagrams whose endpoints are abstract role labels
// (`ExplorerAgent`, `EmitEvidence_TA`) — not codebase symbols.
// The skill prompt explicitly permits role labels in such
// diagrams without code grounding ("conceptual / role labels are
// fine without a citation"). Fix F restricts the oracle gate to
// edges in a code-context, determined by TWO typed signals:
//
//   - Diagram-level: diagram.Kind == DiagramCallDAG. Call DAGs by
//     declaration require nodes to be code-callable entities.
//   - Edge-level: the LLM-declared EdgeAnchor.RelationKind ==
//     DiagramRelCall. A typed `call` relation by definition
//     connects function/method endpoints regardless of the
//     containing diagram's kind.
//
// An edge passes through the oracle gate iff EITHER signal holds.
// Edges that satisfy neither — architecture role nodes, flow
// state nodes, sequence actors, import edges between packages —
// skip the validator entirely. Both signals are typed enums LLM
// authors deliberately, mirroring R3's "precise typed signals
// drive hard gates, noisy signals drive soft guidance".
//
// Why this gate is needed (independent of Fix F):
// validateDiagramEdgeSupport's diagramTokenSupported uses
// bidirectional case-folded substring matching against a support
// pool that includes prose tokens (block titles, item labels,
// claim_use annotations). Short prose tokens like "coherence" or
// "validate" appearing anywhere in the answer's grounding pool
// can vouch a fabricated compound mermaid node id like
// `validateFakeCoherenceCheck`. The graph oracle rejects the same
// name because no source file declares it, preventing the user
// from receiving a diagram that references identifiers that don't
// exist — but only when the typed signals say the diagram is
// about code, not abstract roles.
//
// Trigger conditions (all required, defensively narrow):
//
//   - oracle != nil — unit-test path / no-graph runs disable the
//     validator.
//   - block.Kind == BlockDiagram with a non-empty Diagram.Body —
//     other shapes don't carry mermaid edges.
//   - The edge is in a CODE CONTEXT — diagram.Kind ==
//     DiagramCallDAG OR EdgeAnchor.RelationKind == DiagramRelCall.
//   - endpoint is shaped <identifier>[<separator><prose>] —
//     prose-id endpoints (e.g. "Step 1") skip via
//     labelLeadingSymbolIdentifier returning "" when the leading
//     identifier continues into more letters/digits.
//   - leading identifier length ≥ labelHallucinationGateLengthFloor
//     (10 chars) — short ids (`A`, `Foo`, `node1`) are typically
//     mermaid stub identifiers, not source-file symbol references,
//     and would false-positive.
//   - oracle (Fix E SymbolExistsFlat) returns Tier=0 (not found)
//     OR Tier ≥ 3 (low-confidence parse). Tier 1-2 → pass.
//
// Default classification: Medium severity, retry-eligible
// finalizer-only. Operators promote via
// pipeline_contract_strict_kinds.
func validateDiagramEdgeEndpointHallucination(doc *types.AnswerDocumentV2, oracle types.SymbolOracle) []types.Violation {
	if doc == nil || oracle == nil {
		return nil
	}
	type hallucinated struct {
		endpoint string
		ident    string
	}
	// Build the typed (from,to) → RelationKind index once for the
	// whole document; reused per block to decide whether each edge
	// is in a code context. Keys are case-folded trimmed strings to
	// match the parseMermaidEdges normalisation.
	typedRel := buildTypedRelationIndex(doc)
	perBlock := make(map[string][]hallucinated)
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		if b.Kind != types.BlockDiagram || b.Diagram == nil {
			continue
		}
		body := strings.TrimSpace(b.Diagram.Body)
		if body == "" {
			continue
		}
		edges := parseMermaidEdges(body)
		if len(edges) == 0 {
			continue
		}
		// Fix F: a CallDAG-kind diagram puts every edge in a code
		// context regardless of edge-level relation declarations.
		kindIsCodeContext := isCodeContextDiagramKind(b.Diagram.Kind)
		// Track the hallucinated endpoints encountered in this block;
		// dedupe so the same endpoint named in 5 edges only reports
		// once per block.
		seen := make(map[string]struct{})
		var blockHits []hallucinated
		check := func(endpoint string) {
			if endpoint == "" {
				return
			}
			ident := labelLeadingSymbolIdentifier(endpoint)
			if len(ident) < labelHallucinationGateLengthFloor {
				return
			}
			if !contract.IsIdentifierShaped(ident) {
				return
			}
			// Fix E: cross-form existence — see Fix C for rationale.
			found, tier := oracle.SymbolExistsFlat(ident)
			if found && tier < 3 {
				return
			}
			if _, dup := seen[ident]; dup {
				return
			}
			seen[ident] = struct{}{}
			blockHits = append(blockHits, hallucinated{
				endpoint: endpoint,
				ident:    ident,
			})
		}
		for _, e := range edges {
			// Fix F: per-edge code-context decision. Skip the gate
			// entirely when neither the diagram kind nor the edge's
			// own typed relation says "this is a code-call edge".
			edgeIsCodeContext := kindIsCodeContext
			if !edgeIsCodeContext {
				key := edgeKey{
					from: strings.ToLower(strings.TrimSpace(e.from)),
					to:   strings.ToLower(strings.TrimSpace(e.to)),
				}
				if rel, ok := typedRel[key]; ok && isCodeContextRelationKind(rel) {
					edgeIsCodeContext = true
				}
			}
			if !edgeIsCodeContext {
				continue
			}
			check(e.from)
			check(e.to)
		}
		if len(blockHits) > 0 {
			perBlock[b.ID] = blockHits
		}
	}
	if len(perBlock) == 0 {
		return nil
	}
	blockIDs := make([]string, 0, len(perBlock))
	for blockID := range perBlock {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	out := make([]types.Violation, 0, len(blockIDs))
	for _, blockID := range blockIDs {
		hits := perBlock[blockID]
		pairs := make([]string, 0, len(hits))
		for _, h := range hits {
			pairs = append(pairs, fmt.Sprintf("block=%q ident=%q endpoint=%q", blockID, h.ident, h.endpoint))
		}
		out = append(out, types.Violation{
			Kind: types.ViolDiagramEdgeEndpointHallucinated,
			Detail: fmt.Sprintf(
				"diagram block %q has %d edge endpoint identifier(s) that do not match any function / type / constant declared in the codebase (likely fabricated identifier): [%s]",
				blockID, len(hits), strings.Join(pairs, "; ")),
			Repair:     "the listed mermaid endpoint identifiers are fabricated names that no source file declares. Replace each with a real identifier from the existing evidence anchors / symbol slate, OR rename the node to a non-symbol-shape label so the diagram clearly conveys an abstract role rather than a fictional code location.",
			ClusterKey: blockClusterKey(blockID, "diagram_edges"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "diagram_edges",
				Reason:     "mermaid endpoint identifier not declared in the codebase (fabricated names)",
				Confidence: 0.9,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

// inlineBacktickIdentRE captures markdown inline-code spans whose
// content starts with an identifier-shape token. The capture group
// returns the token UP TO the first non-identifier character — so
// `fmt.Sprintf` captures `fmt` (and the validator's leading-ident
// + length floor then drops it as a stdlib reference). Identifier
// chars: ASCII letter / digit / `_` only — `-` excluded so kebab-
// case CLI flags don't accidentally match.
var inlineBacktickIdentRE = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)`")

// validateInlineIdentifierHallucination (Fix I, 2026-05-07
// post-batch eval forensic) extends the typed-graph SymbolOracle
// existence check (Fix C / Fix D) to PROSE TEXT surfaces — Title /
// Text fields on Summary / Section / Scalar / Decision / Caveat
// blocks plus Items[].Text. Catches the failure mode where the
// finalizer renders an enumeration as markdown numbered prose
// inside a Section block (instead of structured items[].label),
// quoting fabricated identifiers in inline backticks that the
// list-block-only Fix C cannot see.
//
// Production-shape (s1a r1, batch eval 2026-05-07): finalizer
// emitted the gate.Run check enumeration as
// `1. \`checkAnswerContract\`(...) — desc / 2. \`checkSubtopicConsistency\`...
// inside BlockSection.Text. Five names were partial hallucinations
// (`checkAnswerContract` for `checkContractComplete`,
// `checkSubtopicConsistency` for `checkSubtopicCoherence`, etc.);
// the answer shipped to the user with mis-attributed file:line
// citations because no validator scanned section text.
//
// Trigger conditions (mirrors Fix C / Fix D defensively-narrow gate):
//
//   - oracle != nil — unit-test path / no-graph runs disable.
//   - block.Kind ∈ { Summary, Section, Scalar, Decision, Caveat,
//     OrderedList, BulletList, Table } — every prose-bearing
//     block kind. (List blocks scan Items[].Text not Items[].Label
//     so we don't double-count Fix C surface.)
//   - regex extracts a backtick-delimited token starting with
//     an identifier-shape character.
//   - leading identifier (extracted via labelLeadingSymbolIdentifier)
//     length ≥ labelHallucinationGateLengthFloor (10 chars) —
//     stdlib helpers (`Sprintf`, `Println`, `Get`) skip.
//   - oracle (Fix E SymbolExistsFlat) returns Tier=0 OR Tier ≥ 3.
//
// Default classification: Medium severity, retry-eligible
// finalizer-only. Operators promote via
// pipeline_contract_strict_kinds.
func validateInlineIdentifierHallucination(doc *types.AnswerDocumentV2, oracle types.SymbolOracle, mutOpt ...*types.MutableState) []types.Violation {
	if doc == nil || oracle == nil {
		return nil
	}
	var mut *types.MutableState
	if len(mutOpt) > 0 {
		mut = mutOpt[0]
	}
	type hallucinated struct {
		ident   string
		surface string
	}
	perBlock := make(map[string][]hallucinated)
	checkText := func(blockID, text, surface string, seen map[string]struct{}, hits *[]hallucinated) {
		if text == "" {
			return
		}
		matches := inlineBacktickIdentRE.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			token := m[1]
			ident := labelLeadingSymbolIdentifier(token)
			if len(ident) < labelHallucinationGateLengthFloor {
				continue
			}
			if !contract.IsIdentifierShaped(ident) {
				continue
			}
			if inlineIdentifierSupportedByRuntimeArtifact(ident, mut) {
				continue
			}
			if answerItemLabelSupportedByEvidenceEndpoint(token, mut) {
				continue
			}
			found, tier := oracle.SymbolExistsFlat(ident)
			if found && tier < 3 {
				continue
			}
			if _, dup := seen[ident]; dup {
				continue
			}
			seen[ident] = struct{}{}
			*hits = append(*hits, hallucinated{ident: ident, surface: surface})
		}
	}
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		switch b.Kind {
		case types.BlockSummary, types.BlockSection, types.BlockScalar,
			types.BlockDecision, types.BlockCaveat,
			types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
			// supported — every block kind that carries prose text
		default:
			continue
		}
		seen := make(map[string]struct{})
		var hits []hallucinated
		checkText(b.ID, b.Title, "title", seen, &hits)
		checkText(b.ID, b.Text, "text", seen, &hits)
		for j := range b.Items {
			checkText(b.ID, b.Items[j].Text, fmt.Sprintf("items[%d].text", j), seen, &hits)
		}
		if len(hits) > 0 {
			perBlock[b.ID] = hits
		}
	}
	if len(perBlock) == 0 {
		return nil
	}
	blockIDs := make([]string, 0, len(perBlock))
	for blockID := range perBlock {
		blockIDs = append(blockIDs, blockID)
	}
	sort.Strings(blockIDs)
	out := make([]types.Violation, 0, len(blockIDs))
	for _, blockID := range blockIDs {
		hits := perBlock[blockID]
		pairs := make([]string, 0, len(hits))
		for _, h := range hits {
			pairs = append(pairs, fmt.Sprintf("block=%q ident=%q surface=%q", blockID, h.ident, h.surface))
		}
		out = append(out, types.Violation{
			Kind: types.ViolInlineIdentifierHallucinated,
			Detail: fmt.Sprintf(
				"block %q has %d inline-backtick identifier(s) in prose text that are not validated by the symbol oracle or by an answer-grade evidence surface (likely fabricated or background-only code reference): [%s]",
				blockID, len(hits), strings.Join(pairs, "; ")),
			Repair:     "for each listed inline-backtick identifier, either cite and use the exact surface from a typed support lane / symbol slate / answer-grade evidence anchor, or remove the inline backticks and keep the wording as ordinary context prose. Do not promote helper names from retry diagnostics, Evidence notes, raw tool output, search hints, or nearby context into authoritative code references.",
			ClusterKey: blockClusterKey(blockID, "inline_identifier"),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "inline_identifier",
				Reason:     "answer prose used inline-backtick identifiers outside the validated principal evidence surface",
				Confidence: 0.9,
			},
			Stage: string(types.StageFinalize),
		})
	}
	return out
}

func inlineIdentifierSupportedByRuntimeArtifact(ident string, mut *types.MutableState) bool {
	if mut == nil {
		return false
	}
	if bundle := mut.LogTriage(); bundle != nil && inlineIdentifierSupportedByLogBundle(ident, bundle) {
		return true
	}
	if perf := mut.PerfTrace(); perf != nil {
		for _, frame := range perf.LogFrames() {
			if inlineIdentifierSupportedByRuntimeFrame(ident, frame) {
				return true
			}
		}
	}
	return false
}

func inlineIdentifierSupportedByLogBundle(ident string, bundle *types.LogBundle) bool {
	matched := false
	types.WalkLogFrames(bundle, func(frame types.LogFrame) {
		if matched {
			return
		}
		if inlineIdentifierSupportedByRuntimeFrame(ident, frame) {
			matched = true
		}
	})
	return matched
}

func inlineIdentifierSupportedByRuntimeFrame(ident string, frame types.LogFrame) bool {
	return runtimeIdentifierTokenMatches(ident, frame.Func) ||
		runtimeIdentifierTokenMatches(ident, frame.Pkg) ||
		runtimeIdentifierTokenMatches(ident, frame.Raw)
}

func runtimeIdentifierTokenMatches(ident, token string) bool {
	ident = strings.TrimSpace(ident)
	token = strings.TrimSpace(token)
	if ident == "" || token == "" {
		return false
	}
	i := strings.ToLower(ident)
	t := strings.ToLower(token)
	if i == t {
		return true
	}
	return strings.HasPrefix(t, i) && labelBoundaryAfter(t, len(i))
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
//   - EvidenceItem.SurfaceTerms — model-authored labels already
//     validated against the cited source/log/trace line. These cover
//     package paths, file paths, runtime labels, aliases, and other
//     code-identity surfaces that are not declaration symbols.
//   - Grounded snippet selector chains — qualified identifiers that
//     visibly appear on the grounded line itself (e.g.
//     normalizer.Normalize, ctx.Mutable.RequestModel,
//     types.AnalysisIR). Suffix variants are also admitted so
//     readable labels can drop leading qualifiers without becoming
//     ungrounded.
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
		for _, term := range it.SurfaceTerms {
			add(term)
		}
		addSnippetDerivedLabelSupportTokens(out, it)
	}
	return out
}

func addSnippetDerivedLabelSupportTokens(out map[string]struct{}, it types.EvidenceItem) {
	snippet := strings.TrimSpace(it.Snippet)
	if snippet == "" || it.GroundingStatus == types.GroundingUngrounded {
		return
	}
	for _, match := range snippetSelectorTokenRE.FindAllString(snippet, -1) {
		addSelectorTokenAndSuffixes(out, match)
	}
}

// addSelectorTokenAndSuffixes admits the FULL dot-qualified selector
// chain (e.g. `normalizer.Normalize`) and every MULTI-segment suffix
// of it (e.g. `pkg.Normalize`) into the support pool. The FINAL bare
// segment (`Normalize`, `Sprintf`, `Get`, …) is intentionally NOT
// admitted as a standalone token, even when it looks load-bearing.
//
// Why no bare-segment admission:
//
//   - The label oracle (and diagramTokenSupported) uses bidirectional
//     case-folded substring matching: a label like `Normalize` will
//     match a pooled token `normalizer.normalize` because `normalize`
//     is a substring of the chain. So load-bearing project tails are
//     already reachable without polluting the pool.
//   - Admitting bare segments would let common helpers from any
//     grounded snippet (e.g. `Sprintf` from `fmt.Sprintf`, `Println`
//     from `log.Println`, `Get` / `Set` / `New` from countless
//     callsites) silently legitimise fabricated enumeration item
//     labels merely because the LLM happened to read code that
//     invokes a popular helper. The risk is asymmetric: false
//     positives (fabricated labels passing) are silent quality
//     regressions; false negatives (legitimate labels rejected) get
//     caught at retry by the bidirectional substring fallback.
//
// Net effect: snippets contribute the dot-qualified selector chains
// they expose, the label oracle resolves bare tails through the
// substring path, and stdlib short names never enter the pool as
// independent tokens.
func addSelectorTokenAndSuffixes(out map[string]struct{}, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return
	}
	for i := 0; i < len(parts)-1; i++ {
		token := strings.Join(parts[i:], ".")
		if token == "" || !strings.Contains(token, ".") {
			continue
		}
		out[strings.ToLower(token)] = struct{}{}
	}
}

// laneAttrib carries the minimum lane-identity needed by the
// validator: a human-readable title for the violation message and
// the AllowedBlocks set the lane declares.
type laneAttrib struct {
	title         string
	allowedBlocks []string
}

// validateLaneBlockKindCompliance enforces the typed
// AnswerSupportLane.AllowedBlocks contract as a hard validator. Each
// active support lane (observed_artifact / principal_evidence /
// current_code_path / nearest_mechanism / uncertainty_boundary /
// current_status_verdict) declares the block kinds that may
// legitimately render its content. This oracle locates the lane each
// principal block is sourced from by matching its citation pool
// against AnswerSupportEntry.Location, then rejects the block when
// its kind is absent from that lane's AllowedBlocks set.
//
// Skip conditions (no false positives):
//
//   - supportPlan == nil OR no lanes carry an AllowedBlocks restriction:
//     the family doesn't model a lane → block kind contract.
//   - Block has SurfaceRole != principal: only main-line answer
//     blocks carry the lane discipline; supporting context, framing
//     prose, and standalone diagram nodes are unconstrained.
//   - Block has no resolved citations referencing lane locations:
//     the block isn't sourced from any lane the validator can
//     attribute, so the validator declines to claim a violation.
//
// The validator is the typed-signal upgrade for the previously
// advisory `Allowed block kinds: ...` prompt line; the LLM still
// sees the prompt, but emit-time non-compliance is now hard-rejected
// instead of relying on prose-level adherence.
func validateLaneBlockKindCompliance(
	doc *types.AnswerDocumentV2,
	supportPlan *types.AnswerSupportPlan,
) []types.Violation {
	if doc == nil || supportPlan == nil || len(supportPlan.Lanes) == 0 {
		return nil
	}
	// Build location → lanes index, restricted to lanes that actually
	// declare an AllowedBlocks set. Lanes without the field are
	// treated as unconstrained and contribute no entry to the index.
	//
	// A single grounded location may intentionally belong to more
	// than one lane. Current-status verdict synthesis is the load-
	// bearing case: the same current-code citation can support both a
	// path/mechanism block and a bounded `decision` block. Therefore a
	// block passes when ANY matching lane allows its kind.
	locIndex := make(map[string][]laneAttrib)
	for _, lane := range supportPlan.Lanes {
		if len(lane.AllowedBlocks) == 0 {
			continue
		}
		attrib := laneAttrib{title: strings.TrimSpace(lane.Title), allowedBlocks: lane.AllowedBlocks}
		for _, entry := range lane.Entries {
			key := normalizeLaneLocationKey(entry.Location)
			if key == "" {
				continue
			}
			locIndex[key] = appendLaneAttrib(locIndex[key], attrib)
		}
	}
	if len(locIndex) == 0 {
		return nil
	}
	var out []types.Violation
	for blockIdx := range doc.Blocks {
		block := &doc.Blocks[blockIdx]
		if block.SurfaceRole != types.SurfacePrincipal {
			continue
		}
		blockID := strings.TrimSpace(block.ID)
		if blockID == "" {
			blockID = fmt.Sprintf("#%d", blockIdx)
		}
		matchedLane, anyAllowed, hasMatch := blockLaneAlignment(doc, block, locIndex)
		if !hasMatch || anyAllowed {
			continue
		}
		out = append(out, types.Violation{
			Kind: types.ViolLaneBlockKindMismatch,
			Detail: fmt.Sprintf(
				"principal block %q has kind=%q but every cited location is sourced from the %q support lane, which only allows the following block kinds: %v",
				blockID, block.Kind, matchedLane.title, matchedLane.allowedBlocks),
			Repair: fmt.Sprintf(
				"either change block %q's kind to one of %v (the block kinds the supporting lane allows — typically `summary` or `caveat` for observation lanes, `ordered_list` or `diagram` for current-code-path lanes), or move this content under a different supporting lane that allows %q blocks. The block kind must match the lane it draws from so observation facts do not get silently presented as call-chain steps.",
				blockID, matchedLane.allowedBlocks, block.Kind),
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "block_kind_vs_lane_allowed",
				Reason:     "principal block kind not in any matching support lane's allowed-kind list",
				Confidence: 0.9,
			},
			ClusterKey: blockClusterKey(blockID, "lane_block_kind"),
		})
	}
	return out
}

// normalizeLaneLocationKey maps an AnswerSupportEntry.Location to the
// canonical "file:line" form the citation index uses. Empty input
// returns "". Path separators are normalised so Windows-style paths
// match POSIX ones.
func normalizeLaneLocationKey(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	location = strings.ReplaceAll(location, `\`, `/`)
	return strings.ToLower(location)
}

// blockLaneAlignment scans a block's items[].citation_ref for
// lane-attributable citations and reports whether ANY cited location
// resolves to a lane whose AllowedBlocks contains the block's kind.
//
// Returns (matchedLane, anyAllowed, hasMatch):
//   - hasMatch=false → the block has no citation that resolves to
//     any lane in the index; validator declines to fire.
//   - hasMatch=true / anyAllowed=true → at least one cited location
//     is sourced from a lane that allows this block kind; validator
//     declines to fire.
//   - hasMatch=true / anyAllowed=false → every cited location is
//     sourced from lanes that do NOT allow this block kind;
//     validator returns the FIRST matched lane as the suspect.
func blockLaneAlignment(
	doc *types.AnswerDocumentV2,
	block *types.AnswerBlock,
	locIndex map[string][]laneAttrib,
) (laneAttrib, bool, bool) {
	var firstMatch laneAttrib
	matched := false
	allowed := false
	for _, item := range block.Items {
		ref := item.CitationRef
		if ref < 0 || ref >= len(doc.Citations) {
			continue
		}
		cit := doc.Citations[ref]
		key := citationLocationKey(cit)
		if key == "" {
			continue
		}
		attrs, ok := locIndex[key]
		if !ok {
			continue
		}
		for _, attrib := range attrs {
			if !matched {
				firstMatch = attrib
				matched = true
			}
			if blockKindAllowedByLane(string(block.Kind), attrib.allowedBlocks) {
				allowed = true
				break
			}
		}
		if allowed {
			break
		}
	}
	return firstMatch, allowed, matched
}

func appendLaneAttrib(in []laneAttrib, attrib laneAttrib) []laneAttrib {
	for _, existing := range in {
		if existing.title == attrib.title &&
			strings.Join(existing.allowedBlocks, "\x00") == strings.Join(attrib.allowedBlocks, "\x00") {
			return in
		}
	}
	return append(in, attrib)
}

// citationLocationKey renders a Citation as the same canonical
// "file:line" key normalizeLaneLocationKey produces for lane entries.
// Empty file or zero / negative line returns "" so file-only lane
// entries (Location="") don't accidentally match a citation with
// no line.
func citationLocationKey(cit types.Citation) string {
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return ""
	}
	file = strings.ReplaceAll(file, `\`, `/`)
	return strings.ToLower(fmt.Sprintf("%s:%d", file, cit.Line))
}

func blockKindAllowedByLane(kind string, allowed []string) bool {
	if kind == "" || len(allowed) == 0 {
		return false
	}
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), kind) {
			return true
		}
	}
	return false
}
