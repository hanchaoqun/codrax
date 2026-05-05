package types

import (
	"fmt"
)

// answer_document_v2_mutation.go — G2 (post_v2_runtime_gap_remediation,
// 2026-05-04). Unified document-change contract that lifts both
// "full re-emit" (ReplaceAll) and "delta against prev" (Partial)
// onto the same typed surface.
//
// Pre-G2 the orchestrator + tools had two distinct write paths:
//   - SetAnswerDocumentV2(doc)            full re-emit
//   - SetAnswerDocumentV2FromPatch(merged) patch emit (after
//     ApplyAnswerDocumentV2Patch returns the merged doc)
//
// Both paths converge at runContractCheck which reads
// mut.AnswerDocumentV2() — i.e. the validator chokepoint already
// sees a unified merged doc. G2's role is to make the WRITE side
// equally unified so consumers (telemetry, future operator audit
// tooling, retry summary) can read a single Mutation surface to
// describe "what change just happened" without inferring from
// LastEmitFromPatch + counting field deltas.
//
// Red lines:
//   - Both kinds (ReplaceAll, Partial) MUST produce byte-identical
//     merged docs to the legacy SetAnswerDocumentV2 / patch paths
//     when fed equivalent inputs. Tests pin this.
//   - Apply must NEVER mutate prev — it returns a new
//     AnswerDocumentV2.
//   - Summary output is INTERNAL telemetry only (not LLM-facing) —
//     R4 is not a concern here, but the format MUST be parseable
//     so future operator dashboards can structure-mine without
//     regex fragility.

// MutationKind enumerates the two document-change shapes the V2
// carrier supports.
type MutationKind string

const (
	// MutationReplaceAll names a full doc rewrite. The merged
	// result equals the supplied Replace doc verbatim — prev
	// (when present) is discarded.
	MutationReplaceAll MutationKind = "replace_all"

	// MutationPartial names a delta against prev. The merged
	// result equals ApplyAnswerDocumentV2Patch(prev, Patch).
	MutationPartial MutationKind = "partial"
)

// IsValid reports whether m is a known mutation kind.
func (m MutationKind) IsValid() bool {
	switch m {
	case MutationReplaceAll, MutationPartial:
		return true
	}
	return false
}

// AnswerDocumentMutation is the unified document-change model.
// Exactly ONE of (Replace, Patch) is non-nil; the Kind tag
// disambiguates.
type AnswerDocumentMutation struct {
	Kind MutationKind

	// Replace is the new doc payload when Kind=MutationReplaceAll.
	// MUST be nil when Kind=MutationPartial.
	Replace *AnswerDocumentV2

	// Patch is the delta payload when Kind=MutationPartial. MUST be
	// nil when Kind=MutationReplaceAll.
	Patch *AnswerDocumentV2Patch
}

// IsValid reports whether the mutation has the expected shape for
// its declared Kind.
func (m AnswerDocumentMutation) IsValid() bool {
	switch m.Kind {
	case MutationReplaceAll:
		return m.Replace != nil && m.Patch == nil
	case MutationPartial:
		return m.Patch != nil && m.Replace == nil
	}
	return false
}

// Apply lowers the mutation onto prev (may be nil for ReplaceAll).
// Returns the merged doc the chokepoint validator inspects.
//
// Behaviour:
//   - Kind=ReplaceAll: returns the Replace doc verbatim (a shallow
//     copy is NOT made — the caller's invariant is "I produced this
//     doc; the runtime now owns it"). prev is discarded.
//   - Kind=Partial: routes through ApplyAnswerDocumentV2Patch(prev,
//     Patch). prev MUST be non-nil; Patch's invariants apply.
//
// Returns (nil, error) when the mutation shape is invalid OR when
// the underlying ApplyAnswerDocumentV2Patch rejects the patch.
func (m AnswerDocumentMutation) Apply(prev *AnswerDocumentV2) (*AnswerDocumentV2, error) {
	if !m.IsValid() {
		return nil, fmt.Errorf("AnswerDocumentMutation.Apply: invalid mutation shape — kind=%q replace=%t patch=%t",
			m.Kind, m.Replace != nil, m.Patch != nil)
	}
	switch m.Kind {
	case MutationReplaceAll:
		return m.Replace, nil
	case MutationPartial:
		return ApplyAnswerDocumentV2Patch(prev, m.Patch)
	}
	return nil, fmt.Errorf("AnswerDocumentMutation.Apply: unhandled kind %q", m.Kind)
}

// Summary returns a one-line internal telemetry string describing
// the mutation's shape. INTERNAL surface only — never rendered into
// LLM-facing prompts.
//
// Format examples:
//
//	"replace_all blocks=4 citations=2"
//	"partial unchanged=2 replace=1 add=0 remove=1 citations_inherited"
//	"partial unchanged=2 replace=1 add=0 remove=1 citations_replaced=3"
func (m AnswerDocumentMutation) Summary() string {
	switch m.Kind {
	case MutationReplaceAll:
		if m.Replace == nil {
			return "replace_all blocks=0 citations=0"
		}
		return fmt.Sprintf("replace_all blocks=%d citations=%d",
			len(m.Replace.Blocks), len(m.Replace.Citations))
	case MutationPartial:
		if m.Patch == nil {
			return "partial empty"
		}
		base := fmt.Sprintf("partial unchanged=%d replace=%d add=%d remove=%d",
			len(m.Patch.UnchangedBlockIDs), len(m.Patch.ReplaceBlocks),
			len(m.Patch.AddBlocks), len(m.Patch.RemoveBlockIDs))
		switch {
		case m.Patch.ReplaceCitations != nil:
			base += fmt.Sprintf(" citations_replaced=%d", len(m.Patch.ReplaceCitations))
		case len(m.Patch.AppendCitations) > 0:
			base += fmt.Sprintf(" citations_appended=%d", len(m.Patch.AppendCitations))
		default:
			base += " citations_inherited"
		}
		return base
	}
	return fmt.Sprintf("invalid kind=%q", m.Kind)
}

// NewReplaceAllMutation is a convenience constructor for the full-
// emit path. Caller is responsible for ensuring doc is non-nil and
// already represents a persisted/internal V2 document (the runtime
// stamps DocumentModel="v2" before constructing this mutation).
func NewReplaceAllMutation(doc *AnswerDocumentV2) AnswerDocumentMutation {
	return AnswerDocumentMutation{Kind: MutationReplaceAll, Replace: doc}
}

// NewPartialMutation is the convenience constructor for the patch-
// emit path.
func NewPartialMutation(patch *AnswerDocumentV2Patch) AnswerDocumentMutation {
	return AnswerDocumentMutation{Kind: MutationPartial, Patch: patch}
}
