package types

import (
	"sort"
	"strings"
)

// ScopeDisclosureKind is the typed answer-surface enum for a block
// that explicitly declares why a principal answer is bounded by the
// active sub-repo set. Each value is mutually exclusive; validators
// consume the field directly without parsing decision/caveat prose.
type ScopeDisclosureKind string

const (
	ScopeDisclosureUnknown ScopeDisclosureKind = ""

	// ScopeDisclosureInactiveScopeNamed: the answer cites at least
	// one specific inactive sub-repo RootRel by name. The validator
	// counts this as disclosure when the inactive RootRel also
	// appears as a token-bounded match in the block surface.
	ScopeDisclosureInactiveScopeNamed ScopeDisclosureKind = "inactive_scope_named"

	// ScopeDisclosureOutOfActiveScope: the answer asserts the target
	// is outside the active scope. Reserved for absence answers in
	// multi-repo contexts where the user-facing recovery prose
	// frames the boundary rather than naming a specific RootRel.
	ScopeDisclosureOutOfActiveScope ScopeDisclosureKind = "out_of_active_scope"

	// ScopeDisclosureRequiresWorkspaceAdjust: the answer recommends
	// the operator adjust the active sub-repo set (e.g. via
	// `/repos focus`) before retrying.
	ScopeDisclosureRequiresWorkspaceAdjust ScopeDisclosureKind = "requires_workspace_adjust"
)

// AllScopeDisclosureKinds returns the canonical non-empty values for
// schema generation and validator coverage tests.
func AllScopeDisclosureKinds() []ScopeDisclosureKind {
	return []ScopeDisclosureKind{
		ScopeDisclosureInactiveScopeNamed,
		ScopeDisclosureOutOfActiveScope,
		ScopeDisclosureRequiresWorkspaceAdjust,
	}
}

// IsValid reports whether s is a declared ScopeDisclosureKind value.
// The empty value (ScopeDisclosureUnknown) is also valid: it means
// "this block carries no scope disclosure obligation".
func (s ScopeDisclosureKind) IsValid() bool {
	if s == ScopeDisclosureUnknown {
		return true
	}
	for _, declared := range AllScopeDisclosureKinds() {
		if s == declared {
			return true
		}
	}
	return false
}

// NormalizeScopeDisclosureKind validates a raw string against the
// declared enum. Returns Unknown + false for non-empty invalid values.
func NormalizeScopeDisclosureKind(raw string) (ScopeDisclosureKind, bool) {
	v := ScopeDisclosureKind(strings.TrimSpace(raw))
	if !v.IsValid() {
		return ScopeDisclosureUnknown, false
	}
	return v, true
}

// InactiveScopeDisclosureReason names the typed cause that promotes
// the disclosure obligation from advisory to required. Each reason
// is derived from typed RequestModel / AnswerDocumentV2 fields,
// never from raw prose.
type InactiveScopeDisclosureReason string

const (
	InactiveScopeReasonNone                  InactiveScopeDisclosureReason = ""
	InactiveScopeReasonExactAbsentInActive   InactiveScopeDisclosureReason = "exact_target_absent_in_active"
	InactiveScopeReasonLocatePrincipalEmpty  InactiveScopeDisclosureReason = "locate_intent_principal_empty"
	InactiveScopeReasonEnumerateScopeBounded InactiveScopeDisclosureReason = "enumerate_scope_bounded"
)

// InactiveScopeDisclosureObligation is the typed contract emitted by
// the AnswerSemanticView compiler for multi-repo answers whose
// principal surface is bounded by an active sub-repo subset. The
// finalizer must visibly disclose that boundary via one of the
// ScopeDisclosureKind blocks OR by naming at least one inactive
// RootRel token in the visible answer surface.
//
// Required is computed from precise typed signals only:
//   - BusContext.PendingSubRepos is non-empty (multi-repo + at least
//     one inactive sub-repo);
//   - the answer indicates bounded coverage (exact_resolution=absent,
//     or locate intent with empty principal set, or enumerate intent
//     constrained to active scope).
//
// Reason carries which signal triggered the obligation for
// telemetry + prompt rendering. PendingRootRels is the canonical
// list of inactive sub-repo paths to disclose; ActiveRootRels is
// passed through for prompt context only.
type InactiveScopeDisclosureObligation struct {
	Required         bool
	Reason           InactiveScopeDisclosureReason
	PendingRootRels  []string
	ActiveRootRels   []string
	RequestedSubject string // optional: the user-mentioned subject the answer could not resolve
}

// Active reports whether the obligation is required AND has at least
// one pending RootRel to disclose. Validators short-circuit on
// !Active() to keep single-repo / fully-covered runs unchanged.
func (o *InactiveScopeDisclosureObligation) Active() bool {
	if o == nil || !o.Required {
		return false
	}
	for _, r := range o.PendingRootRels {
		if strings.TrimSpace(r) != "" {
			return true
		}
	}
	return false
}

// BuildInactiveScopeDisclosureObligationFromBus assembles the typed
// obligation from BusContext + the rendered AnswerDocumentV2. It is
// pure (no IO, no LLM) and reads only typed fields:
//   - busCtx.PendingSubRepos (canonical inactive RootRel list)
//   - busCtx.AnalysisIR.RequestModel.Intent / Predicates / ExactTargets
//   - doc.ExactResolution.Status
//   - doc.Blocks[].Kind / SurfaceRole (to detect empty principal slate)
//
// nil BusContext / nil AnswerDocumentV2 / single-repo posture all
// return a non-Active obligation (the field is filled but Required
// stays false).
func BuildInactiveScopeDisclosureObligationFromBus(busCtx *BusContext, doc *AnswerDocumentV2) InactiveScopeDisclosureObligation {
	out := InactiveScopeDisclosureObligation{}
	if busCtx == nil || len(busCtx.PendingSubRepos) == 0 {
		return out
	}
	out.PendingRootRels = canonicalRootRels(busCtx.PendingSubRepos)
	if len(out.PendingRootRels) == 0 {
		return out
	}
	// ActiveRootRels is informational for prompt rendering. Compute
	// it from the same multi-repo state when available. We keep this
	// best-effort: BusContext doesn't expose ActiveRootRels directly,
	// so we leave it nil here — the prompt renderer can derive from
	// MultiGraph snapshots if needed. The obligation itself does not
	// depend on this field.

	if !inactiveScopeIntentEligible(busCtx) {
		return out
	}

	reason, requested := inactiveScopeAbsenceReason(busCtx, doc)
	if reason == InactiveScopeReasonNone {
		return out
	}
	out.Required = true
	out.Reason = reason
	out.RequestedSubject = requested
	return out
}

// inactiveScopeIntentEligible reports whether the request's typed
// intent makes a missing-target answer informative for disclosure.
// Locate / explain / enumerate questions all benefit from disclosure;
// reflect / compare questions whose principal slate is intentionally
// open-ended do not.
func inactiveScopeIntentEligible(busCtx *BusContext) bool {
	if busCtx == nil || busCtx.AnalysisIR == nil {
		return false
	}
	rm := busCtx.AnalysisIR.RequestModel
	switch rm.Intent {
	case IntentExplain, IntentEnumerate, IntentReturnValue, IntentTrace:
		return true
	}
	// Predicate-driven eligibility for older callers that omit a
	// crisp Intent value. Scalar / count / role-locate / category
	// enumeration all bound coverage to the active set.
	preds := rm.Predicates
	if preds.IsRoleLocateLookup || preds.IsScalarAnswer || preds.IsCountQuestion || preds.IsCategoryEnumeration {
		return true
	}
	// ExactTargets being non-empty means the user asked for a
	// specific identifier; inactive-scope disclosure helps the
	// user retry against the right active set.
	return len(rm.AnalyzerHints.ExactTargets) > 0
}

// inactiveScopeAbsenceReason returns the typed reason the obligation
// fires, along with the user-mentioned subject (best-effort) for
// prompt rendering. Returns (None, "") when no eligible signal is
// present.
func inactiveScopeAbsenceReason(busCtx *BusContext, doc *AnswerDocumentV2) (InactiveScopeDisclosureReason, string) {
	if busCtx == nil || busCtx.AnalysisIR == nil {
		return InactiveScopeReasonNone, ""
	}
	rm := busCtx.AnalysisIR.RequestModel

	// (1) Exact-target absence is the strongest typed signal: the
	// model already verified the target doesn't exist in the
	// active scope. Disclosing the inactive scope is a coverage
	// statement, not a guess.
	if doc != nil && doc.ExactResolution != nil && doc.ExactResolution.Status == AnswerExactResolutionAbsent {
		return InactiveScopeReasonExactAbsentInActive, firstNonEmpty(rm.AnalyzerHints.ExactTargets...)
	}

	// (2) Locate intent + empty principal slate. The model is trying
	// to point at a single entity (function/file/route/config), but
	// no principal item was produced. The user asked "where is X" —
	// inactive scope is precisely the missing answer.
	if rm.Predicates.IsRoleLocateLookup && answerDocumentPrincipalSlateIsEmpty(doc) {
		return InactiveScopeReasonLocatePrincipalEmpty, firstNonEmpty(rm.AnalyzerHints.ExactTargets...)
	}

	// (3) Enumerate intent with an empty principal slate: same
	// shape as locate-empty above, but only fires when the answer
	// truly has no principal items AND the user asked for a
	// category/relational enumeration. Disclosure is still
	// optional in this branch because enumeration questions may
	// legitimately answer "no items found in active scope".
	if (rm.Intent == IntentEnumerate || rm.Predicates.IsCategoryEnumeration) &&
		answerDocumentPrincipalSlateIsEmpty(doc) {
		return InactiveScopeReasonEnumerateScopeBounded, firstNonEmpty(rm.AnalyzerHints.ExactTargets...)
	}

	return InactiveScopeReasonNone, ""
}

// answerDocumentPrincipalSlateIsEmpty reports whether no principal
// item appears in any list/table block. Used for locate-intent
// disclosure: the model produced no principal answer member, so the
// boundary explanation matters.
func answerDocumentPrincipalSlateIsEmpty(doc *AnswerDocumentV2) bool {
	if doc == nil {
		return true
	}
	for _, block := range doc.Blocks {
		switch block.Kind {
		case BlockOrderedList, BlockBulletList, BlockTable:
		default:
			continue
		}
		if block.SurfaceRole != SurfacePrincipal {
			continue
		}
		for _, item := range block.Items {
			if strings.TrimSpace(item.Label) != "" {
				return false
			}
		}
	}
	return true
}

// AnswerDocumentDisclosesInactiveScope reports whether the rendered
// AnswerDocumentV2 satisfies the typed obligation. Two paths
// satisfy:
//
//  1. Any block carries a non-Unknown ScopeDisclosure value.
//  2. Any visible surface (block.Title / block.Text /
//     item.Label / item.Text) contains a token-bounded match
//     for at least one PendingRootRel.
//
// Path 1 is the strongly-typed channel; path 2 accepts inline prose
// disclosure that names an inactive RootRel as a code-like token
// (using the existing CodeSurfaceAppearsAsToken boundary rule).
func AnswerDocumentDisclosesInactiveScope(doc *AnswerDocumentV2, obligation InactiveScopeDisclosureObligation) bool {
	if !obligation.Active() || doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if block.ScopeDisclosure != ScopeDisclosureUnknown && block.ScopeDisclosure.IsValid() {
			return true
		}
	}
	surface := answerDocumentVisibleSurface(doc)
	if surface == "" {
		return false
	}
	// Normalise sentence-ending punctuation to spaces so a RootRel
	// appearing as the last token before "." / "," / "；" / "。"
	// satisfies disclosure. This is prose-tolerant token matching
	// (the typed RootRel itself is still compared verbatim).
	relaxed := relaxedSurfaceForDisclosure(surface)
	for _, rootRel := range obligation.PendingRootRels {
		if rootRelTokenAppearsInSurface(rootRel, relaxed) {
			return true
		}
		// Also accept the basename of the RootRel (e.g. "tools-py"
		// for RootRel="repo-tools-py" → both should disclose).
		if base := rootRelBasename(rootRel); base != "" && base != rootRel {
			if rootRelTokenAppearsInSurface(base, relaxed) {
				return true
			}
		}
	}
	return false
}

// rootRelTokenAppearsInSurface reports whether the verbatim rootRel
// string appears in surface as a discrete token. A "token" here
// means: bounded on each side by a non-RootRel character. The left
// boundary is sentence/whitespace/punctuation; the right boundary
// is either the same OR a `/` (path continuation, so "repo-tools-py"
// is recognised as the leading segment of "repo-tools-py/tools/x.py").
// All other characters that may legitimately appear INSIDE a RootRel
// (letters, digits, `_`, `-`, `.`) remain token-extension on both
// sides so a basename like "tools-py" does not falsely match inside
// a longer identifier such as "tools-python".
func rootRelTokenAppearsInSurface(rootRel, surface string) bool {
	rootRel = strings.TrimSpace(rootRel)
	surface = strings.TrimSpace(surface)
	if rootRel == "" || surface == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(surface[start:], rootRel)
		if idx < 0 {
			return false
		}
		pos := start + idx
		end := pos + len(rootRel)
		if isRootRelLeftBoundary(surface, pos-1) && isRootRelRightBoundary(surface, end) {
			return true
		}
		start = end
		if start >= len(surface) {
			return false
		}
	}
}

// isRootRelLeftBoundary reports whether the byte at idx (or
// out-of-range before/after the surface) can sit immediately before
// a RootRel token. Anything that's NOT a RootRel-internal character
// counts as boundary.
func isRootRelLeftBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	return !rootRelInternalChar(s[idx])
}

// isRootRelRightBoundary reports whether the byte at idx can sit
// immediately after a RootRel token. The right side additionally
// accepts `/` to recognise path continuation (RootRel as the leading
// path segment of a longer file reference). Sentence-end '.' (a '.'
// immediately followed by a non-internal char OR end-of-surface) is
// treated as boundary even though '.' itself is normally
// RootRel-internal: this lets a basename like "tools-py" disclose
// when written at the end of a sentence ("found in tools-py.").
func isRootRelRightBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	if s[idx] == '/' {
		return true
	}
	if s[idx] == '.' {
		// peek ahead: if next byte is missing or non-internal,
		// '.' is sentence punctuation, not RootRel continuation.
		if idx+1 >= len(s) {
			return true
		}
		if !rootRelInternalChar(s[idx+1]) {
			return true
		}
	}
	return !rootRelInternalChar(s[idx])
}

// rootRelInternalChar reports whether byte b is a character that can
// appear INSIDE a RootRel value. Multi-repo fixtures use
// alphanumerics, `_`, `-`, and `.` (the last for e.g. "v0.1"
// versioned sub-repos). `/` is NOT internal because it is the
// path separator the token may be a prefix of.
func rootRelInternalChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_' || b == '-' || b == '.':
		return true
	default:
		return false
	}
}

// relaxedSurfaceForDisclosure is kept as a no-op for now: the
// rootRelTokenAppearsInSurface matcher already handles sentence-end
// punctuation correctly via its left/right boundary rules. This
// helper remains as a hook so future tightening of which characters
// count as "internal" vs "boundary" can be applied to surface text
// without changing the call site.
func relaxedSurfaceForDisclosure(s string) string {
	return s
}

func answerDocumentVisibleSurface(doc *AnswerDocumentV2) string {
	if doc == nil {
		return ""
	}
	var b strings.Builder
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}
	for _, block := range doc.Blocks {
		add(block.Title)
		add(block.Text)
		for _, item := range block.Items {
			add(item.Label)
			add(item.Text)
		}
	}
	for _, caveat := range doc.Caveats {
		add(caveat)
	}
	return b.String()
}

func canonicalRootRels(rels []string) []string {
	seen := make(map[string]bool, len(rels))
	out := make([]string, 0, len(rels))
	for _, r := range rels {
		r = strings.TrimSpace(strings.ReplaceAll(r, "\\", "/"))
		if r == "" || r == "." || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func rootRelBasename(rootRel string) string {
	rootRel = strings.TrimSpace(strings.ReplaceAll(rootRel, "\\", "/"))
	if rootRel == "" {
		return ""
	}
	// Strip a single "repo-" prefix so RootRels like "repo-tools-py"
	// disclose under either spelling. We do NOT strip other
	// prefixes; this is the only convention the multirepo fixtures
	// use and the case regex explicitly accepts both forms.
	trimmed := strings.TrimPrefix(rootRel, "repo-")
	if trimmed == rootRel {
		return ""
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
