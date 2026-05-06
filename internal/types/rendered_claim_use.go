package types

import "strings"

// rendered_claim_use.go — Phase 3 of the Semantic Surface Contract
// (docs/design/semantic_surface_contract_phases.md §3).
//
// RenderedClaimUse is an OPTIONAL per-payload annotation the LLM
// may attach to AnswerStep / AnswerSymbol / AnswerValue /
// AnswerBoolean. It identifies which FacetCoverageContract slot
// the payload fills + which ClaimForm shape its underlying evidence
// carries + how the payload USES the claim (principal main-line
// content vs supporting context vs prose-only narration vs diagram
// node).
//
// Phase 3 is back-compat-only: when LLM omits the field, emit /
// render are byte-identical to pre-Phase-3. When LLM fills it,
// emit_answer_document validates self-consistency (EvidenceID points
// at a real Citation in the document; when ClaimForm is set, it
// matches ClaimFormOf(referenced evidence)). Phase 4 validators
// will enforce facet coverage by reading these annotations across
// the AnswerDocument tree.
//
// Why per-payload (not per-document):
//   - One AnswerStep can carry one specific facet contribution; a
//     batch-level annotation would force the LLM to redistribute
//     facets after the fact, losing the proximity to the rendered
//     content.
//   - Phase 4 validators want fine-grained "this step covers
//     FacetEnumerationItem-3" rather than "this answer covers
//     enumeration items 1,3,5,7"; the per-payload form is the
//     direct schema for that semantic.

// SurfaceRole flags whether a block is the principal answer payload.
// Empty (the default) means "not principal" — the block is supporting
// context, framing prose, or a diagram-only contribution. Validators
// that gate principal-only obligations (claim_use coverage, slate
// counts) only fire on SurfacePrincipal blocks; everything else is
// treated uniformly as not-principal, so a single principal flag is
// sufficient.
type SurfaceRole string

const (
	// SurfacePrincipal: this payload is main-line answer content.
	// Counts toward enumeration boundaries, role-lookup answers,
	// and any other "the answer IS this" surface.
	SurfacePrincipal SurfaceRole = "principal"
)

// IsValid reports whether r is one of the known SurfaceRole values.
// Empty input is INVALID — the validator may either reject or
// silently drop the annotation, but back-compat for the field
// itself (omitted => annotation absent) is enforced one level up
// in NormalizeSurfaceRole.
func (r SurfaceRole) IsValid() bool {
	return r == SurfacePrincipal
}

// NormalizeSurfaceRole canonicalizes a raw string. Empty input is
// passed through as "" (caller treats as "not annotated"); a
// non-empty unknown value falls back to "" with ok=false so the
// schema validator can decide whether to reject.
func NormalizeSurfaceRole(raw string) (SurfaceRole, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return "", true
	}
	r := SurfaceRole(trimmed)
	if r.IsValid() {
		return r, true
	}
	return "", false
}

// AllSurfaceRoles returns a defensive copy of every valid
// SurfaceRole — useful for schema enum generation and exhaustive
// switch coverage tests.
func AllSurfaceRoles() []SurfaceRole {
	return []SurfaceRole{
		SurfacePrincipal,
	}
}

// RenderedClaimUse is the optional per-payload annotation. All
// fields are independently optional — the LLM may fill any subset.
// Validator semantics (Phase 3):
//
//   - FacetID: when set, must be the string form of a known
//     AnswerFacetKind value. Empty = "unannotated facet"; LLM may
//     leave this for Phase 4 to infer from context.
//
//   - EvidenceID: when set, must point at a Citation in the
//     document-level pool (matched by File:Line). Phase 3 does
//     simple existence check; Phase 4 will cross-reference against
//     the FacetCoverageContract.SourceCandidate list.
//
//   - ClaimForm: when set AND EvidenceID is also set, must equal
//     ClaimFormOf(referenced citation's underlying evidence).
//     Mismatch is a soft validator failure (Phase 3) / hard reject
//     (Phase 4 promotion).
//
// SurfaceRole moved to AnswerBlock-level only — was dead on
// RenderedClaimUse (no semantic readers; only IsEmpty() consulted it
// to detect empty-annotation cases).
//
// Phase 1-B source-fix (V2 runtime eval followup, 2026-05-04):
// FromNode/ToNode were REMOVED from this struct. Their u3a-1 real-
// eval forensic showed that swelling claim_use's inner field count
// from 4 → 6 invited LLMs to mis-fill sibling fields like
// citation_ref into the same nested object (7-iter strict-decode
// retry loop). Edge anchors now live on the typed
// AnswerBlock.EdgeAnchors []DiagramEdgeAnchor — see
// answer_document_v2.go. The LLM sees TWO distinct shapes:
// claim_use (4 fields, facet/claim annotation) vs edge_anchors
// (3 fields, diagram-edge typed triples), and the two cannot be
// confused.
//
// JSON tags use "omitempty" so a zero-valued struct serialises to
// the empty object {}. Pointer-typed embedding on the parent payload
// means an absent annotation serialises to absence (no claim_use
// key in the JSON output) — back-compat is guaranteed.
type RenderedClaimUse struct {
	FacetID    string    `json:"facet_id,omitempty"`
	EvidenceID string    `json:"evidence_id,omitempty"`
	ClaimForm  ClaimForm `json:"claim_form,omitempty"`
}

// IsEmpty reports whether c carries no annotation data. Useful for
// the validator to distinguish "explicitly empty" (treat as no-op)
// from "unset" (the parent's pointer is nil).
func (c *RenderedClaimUse) IsEmpty() bool {
	if c == nil {
		return true
	}
	return c.FacetID == "" && c.EvidenceID == "" && c.ClaimForm == ""
}
