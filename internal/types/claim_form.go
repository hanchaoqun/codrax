package types

// claim_form.go — Phase 0 of the Semantic Surface Contract rollout
// (docs/design/semantic_surface_contract_phases.md §0).
//
// ClaimForm classifies the BASIC sentence shape that an EvidenceItem
// can validly support in the rendered answer. It is a deterministic
// projection of EvidenceItem's typed fields:
//
//     (Origin × Scope × DiagramRole × AnchorKind) → ClaimForm
//
// The projection is a PURE function — never read from LLM input,
// never round-tripped through prompts. Phase 0 only computes and
// surfaces the projection in trace logs (no behaviour change). Later
// phases (1: facet plan compilation; 4: validator hard gates) will
// consume the projection.
//
// ── Why a separate enum (vs just AnchorKind) ──
//
// AnchorKind alone (call / condition / definition / etc.) is a
// repo-anchor classification. It cannot distinguish:
//
//   - log/perf frame observation (Origin=Log, AnchorKind irrelevant)
//   - absence claim (Scope=Negative, AnchorKind=N/A)
//   - precedence-role assertion in config trace (DiagramRole=Config)
//
// ClaimForm captures the broader sentence-shape category by taking
// the dominant typed signal across all four input fields. Multiple
// EvidenceItem fields naturally collapse to one ClaimForm: a log
// frame that happened to anchor at a Call site is still
// ClaimExternalObservation (we cite the log fact, not the call edge),
// because the log origin determines the strongest sentence the
// evidence supports.
//
// ── Why the projection is necessary ──
//
// Without ClaimForm, validator gates trying to enforce "this
// evidence cannot be promoted to that claim shape" must inspect
// 4-5 fields each call. ClaimForm collapses the cross-product into
// a 9-state enum that downstream gates can pattern-match cheaply.
//
// Red lines held:
//   - LLM-emitted ClaimForm flows through RenderedClaimUse only
//     (block.claim_uses[].claim_form on V2 emit_answer_document); the
//     deterministic ClaimFormOf projection is the system-side
//     validator that re-derives the form from typed evidence fields
//     and rejects mismatches. The two-channel split keeps the LLM
//     answering "which sentence shape am I asserting" while the
//     validator owns the "is that supportable by the evidence"
//     question.
//   - Zero keyword tables (red line)
//   - Zero new EvidenceItem write surface — ClaimForm projection is
//     read-only over fields the LLM already emits via emit_evidence;
//     RenderedClaimUse is a separate carrier on AnswerBlock, not a
//     write back into EvidenceItem.

// ClaimForm is the deterministic projection of an EvidenceItem onto
// the basic sentence shape it can validly support. See ClaimFormOf
// for the projection function and the priority order.
type ClaimForm string

const (
	// ClaimUnknown is the safe fallback when no typed signal in the
	// EvidenceItem is strong enough to project. Phase-1+ planners
	// MUST treat ClaimUnknown as "cannot be principal";
	// validators MUST not enforce form-specific rules on it.
	ClaimUnknown ClaimForm = ""

	// ClaimDefinitionFact: evidence cites the definition site of a
	// symbol (function decl, type decl, const decl). Supports
	// "what X is" claims; cannot independently support "what X does
	// in production" without a separate flow-direction anchor.
	ClaimDefinitionFact ClaimForm = "definition_fact"

	// ClaimCallEdge: evidence cites a call/dispatch/invocation site.
	// Supports "X calls Y" / "Y is invoked from X" claims and is the
	// only ClaimForm that validly supports a directed edge in a
	// call-flow diagram.
	ClaimCallEdge ClaimForm = "call_edge"

	// ClaimGuardCondition: evidence cites an `if` / `switch` /
	// `select case` / equivalent branching guard. Supports "Y runs
	// only when condition C" claims. Distinct from
	// ClaimAssignmentFact: an assignment whose RHS happens to be a
	// boolean is NOT a guard — only the branch line itself is.
	ClaimGuardCondition ClaimForm = "guard_condition"

	// ClaimAssignmentFact: evidence cites a variable / field
	// assignment. Supports "X is set to Y" claims. Cannot be
	// promoted to ClaimGuardCondition (the assignment may feed a
	// later branch but the assignment site itself is not a guard).
	ClaimAssignmentFact ClaimForm = "assignment_fact"

	// ClaimReturnFact: evidence cites a return statement. Supports
	// "function F returns V" claims. Cannot independently support
	// "function F's caller observes V" without a separate
	// ClaimCallEdge linking the call site.
	ClaimReturnFact ClaimForm = "return_fact"

	// ClaimAbsenceFact: evidence is a negative observation
	// (Scope=ScopeNegative). Supports "X was not found" claims
	// SCOPED to the search the evidence describes (the
	// NegativePattern field is required). Cannot be widened to
	// repo-wide absence without explicit search-coverage
	// declaration — see Phase 4's validateAbsenceScopeBound.
	ClaimAbsenceFact ClaimForm = "absence_fact"

	// ClaimPrecedenceRole: evidence is tagged as a layer in a
	// config / runtime / override precedence stack
	// (DiagramRole=Config|Runtime|Override). Supports "this is the
	// X-th layer in the Y precedence chain" claims. The claim's
	// authority is bounded by the layer label, not by the
	// underlying source code's literal value.
	ClaimPrecedenceRole ClaimForm = "precedence_role"

	// ClaimExternalObservation: evidence originated from an attached
	// log / perf trace (Origin=Log | Origin=Perf). Supports "the
	// runtime observed X at time T" claims; CANNOT be widened to
	// "the current code does X" without a corroborating
	// repo-side anchor. This is the temporal-escalation guard
	// formalised as a ClaimForm.
	ClaimExternalObservation ClaimForm = "external_observation"

	// ClaimImportEdge: evidence cites an `import` / `use` / `require`
	// statement. Supports "package P depends on Q" but NOT
	// "package P calls Q at runtime" — that's ClaimCallEdge.
	ClaimImportEdge ClaimForm = "import_edge"
)

// allClaimForms is the canonical iteration order for tests and
// schema generation. Update this slice when adding a new constant.
var allClaimForms = []ClaimForm{
	ClaimDefinitionFact,
	ClaimCallEdge,
	ClaimGuardCondition,
	ClaimAssignmentFact,
	ClaimReturnFact,
	ClaimAbsenceFact,
	ClaimPrecedenceRole,
	ClaimExternalObservation,
	ClaimImportEdge,
}

// AllClaimForms returns the canonical iteration order. Returned
// slice is a defensive copy so callers can sort / mutate freely.
func AllClaimForms() []ClaimForm {
	return append([]ClaimForm(nil), allClaimForms...)
}

// IsValid reports whether c is a known non-empty ClaimForm. The
// empty value (ClaimUnknown) is intentionally not "valid" — it is
// the explicit fallback for un-projectable evidence.
func (c ClaimForm) IsValid() bool {
	for _, v := range allClaimForms {
		if c == v {
			return true
		}
	}
	return false
}

type ClaimLabelSurfaceKind string

const (
	ClaimLabelSurfaceUnknown ClaimLabelSurfaceKind = ""

	// ClaimLabelSurfaceSymbolLike means an answer item label, when it
	// is code-shaped, should still be validated as a source symbol or
	// declaration-like endpoint.
	ClaimLabelSurfaceSymbolLike ClaimLabelSurfaceKind = "symbol_like"

	// ClaimLabelSurfaceDisplayLabel means the label itself is a typed
	// evidence surface (for example an import path or precedence role),
	// not a source declaration symbol. It still needs citation and
	// evidence grounding; it just must not be forced through symbol
	// existence gates.
	ClaimLabelSurfaceDisplayLabel ClaimLabelSurfaceKind = "display_label"
)

func (k ClaimLabelSurfaceKind) IsValid() bool {
	switch k {
	case ClaimLabelSurfaceSymbolLike, ClaimLabelSurfaceDisplayLabel:
		return true
	default:
		return false
	}
}

// LabelSurfaceKind classifies the answer-visible item/table label
// contract for every known ClaimForm. Keep this switch exhaustive:
// tests iterate AllClaimForms and fail when a registered ClaimForm
// falls through to ClaimLabelSurfaceUnknown.
func (c ClaimForm) LabelSurfaceKind() ClaimLabelSurfaceKind {
	switch c {
	case ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition,
		ClaimAssignmentFact, ClaimReturnFact, ClaimAbsenceFact:
		return ClaimLabelSurfaceSymbolLike
	case ClaimImportEdge, ClaimPrecedenceRole, ClaimExternalObservation:
		return ClaimLabelSurfaceDisplayLabel
	default:
		return ClaimLabelSurfaceUnknown
	}
}

// UsesNonSymbolLabelSurface reports whether answer item/table labels
// for this claim form are typed display surfaces rather than source
// declaration symbols. Validators use this to avoid routing labels
// such as import paths or config precedence roles through symbol-only
// gates while still requiring citations / evidence grounding.
func (c ClaimForm) UsesNonSymbolLabelSurface() bool {
	return c.LabelSurfaceKind() == ClaimLabelSurfaceDisplayLabel
}

// ClaimFormOf is the deterministic projection. Priority order
// (highest first; first match wins so subsequent rules never
// over-ride a stronger signal):
//
//  1. Origin ∈ {Log, Perf} → ClaimExternalObservation
//     Reason: the log frame IS the evidence; whatever AnchorKind
//     happens to coincide with is incidental. The strongest claim
//     is "the runtime observed X", not "the source code does X".
//     Origin=CrossSource (log + repo confirmed) falls through to
//     the AnchorKind path because the cross-source confirmation
//     promotes the evidence to a current-mechanism class.
//
//  2. Scope == ScopeNegative → ClaimAbsenceFact
//     Reason: a negative anchor CANNOT support any positive sentence
//     shape regardless of AnchorKind hint, and the answer-side gate
//     must enforce scope bounds (Phase 4 validateAbsenceScopeBound).
//
//  3. DiagramRole != Unknown && != Default → ClaimPrecedenceRole
//     Reason: when the diagram-role tag is set to a non-default
//     layer (Config / Runtime / Override), the evidence is being
//     used as a layer-in-precedence assertion. The literal value
//     at file:line is secondary; the layer position is primary.
//     DiagramRoleDefault means "no special layer role"; falls
//     through to AnchorKind.
//
//  4. AnchorKind switch — repo-side current-mechanism classification:
//     AnchorCall       → ClaimCallEdge
//     AnchorCondition  → ClaimGuardCondition
//     AnchorReturn     → ClaimReturnFact
//     AnchorAssignment → ClaimAssignmentFact
//     AnchorInitializer → ClaimAssignmentFact
//     AnchorImport     → ClaimImportEdge
//     AnchorDefinition → ClaimDefinitionFact
//
//  5. fall-through → ClaimUnknown
//     Reason: with no Origin, no Negative scope, no DiagramRole,
//     and no AnchorKind, there is no typed signal to project from.
//     Phase 4 gates treat ClaimUnknown as "cannot be principal".
//
// The function is intentionally trivial — every branch is a typed
// enum equality, no string parsing, no LLM input, no heuristic.
// This is the precise-signals-for-hard-gates red line in code.
func ClaimFormOf(item EvidenceItem) ClaimForm {
	// Priority 1: external-observation origin dominates everything.
	switch item.Origin {
	case ClaimOriginLog, ClaimOriginPerf:
		return ClaimExternalObservation
	}

	// Priority 2: negative scope.
	if item.Scope == ScopeNegative {
		return ClaimAbsenceFact
	}

	// Priority 3: precedence-role tag (config-trace layer marker).
	if item.DiagramRole != EvidenceDiagramRoleUnknown &&
		item.DiagramRole != EvidenceDiagramRoleDefault {
		return ClaimPrecedenceRole
	}

	// Priority 4: AnchorKind dispatch.
	switch item.AnchorKind {
	case AnchorCall:
		return ClaimCallEdge
	case AnchorCondition:
		return ClaimGuardCondition
	case AnchorReturn:
		return ClaimReturnFact
	case AnchorAssignment, AnchorInitializer:
		return ClaimAssignmentFact
	case AnchorImport:
		return ClaimImportEdge
	case AnchorDefinition:
		return ClaimDefinitionFact
	}

	// Priority 5: no typed signal → unknown.
	return ClaimUnknown
}
