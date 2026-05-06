package types

// ClaimOrigin records WHERE an evidence item's anchor came from. The
// distinction is load-bearing for the AuthorityCeiling axis: an anchor
// extracted from an attached log carries different epistemic weight
// than an anchor read directly from the current repository, even when
// both pass grounding. Drift between log observations and current code
// is the dominant case where the two diverge.
//
// Origin is computed deterministically by the emit_* tool layer at
// evidence write time, never emitted by the LLM. The four values form
// a strict ordering of epistemic strength under drift conditions:
//
//	cross_source > current_repo > log/perf > (degenerate cases)
//
// where cross_source means the same anchor appears in BOTH log and
// current repo at the same line, and the two confirmations stack.
type ClaimOrigin string

const (
	// ClaimOriginUnknown is the zero value. Surfaces only via
	// nil-bus paths (tests, single-shot CLI flows). Treated as
	// equivalent to ClaimOriginCurrentRepo so items that bypass the
	// projection don't get downgraded by accident.
	ClaimOriginUnknown ClaimOrigin = ""

	// ClaimOriginCurrentRepo means the anchor was extracted from the
	// current repository contents. This is the dominant case in
	// read-mode runs without an attached log.
	ClaimOriginCurrentRepo ClaimOrigin = "current_repo"

	// ClaimOriginLog means the anchor was extracted from an attached
	// runtime log via the log_triage stage. The (file, line) pair
	// reflects what the log observed, possibly against a build that
	// has since drifted relative to the current repository.
	ClaimOriginLog ClaimOrigin = "log"

	// ClaimOriginPerf means the anchor was extracted from an attached
	// performance trace (HiTrace / atrace / systrace / perfetto) via
	// the perf_triage stage. Same drift semantics as ClaimOriginLog.
	ClaimOriginPerf ClaimOrigin = "perf"

	// ClaimOriginCrossSource means the anchor is corroborated by BOTH
	// attached artifact observations and current repository contents.
	// The corroboration may be exact same-line agreement OR a typed
	// drift-mapped current-code counterpart whose uncertainty is then
	// expressed by AuthorityCeiling + DriftReason. This origin is for
	// repo-backed evidence that is semantically linked to the artifact;
	// pure artifact-only evidence keeps ClaimOriginLog / Perf.
	ClaimOriginCrossSource ClaimOrigin = "cross_source"
)

var allClaimOrigins = []ClaimOrigin{
	ClaimOriginUnknown,
	ClaimOriginCurrentRepo,
	ClaimOriginLog,
	ClaimOriginPerf,
	ClaimOriginCrossSource,
}

// AllClaimOrigins returns the canonical list of every ClaimOrigin
// value in stable declaration order. Callers must not mutate the
// returned slice.
func AllClaimOrigins() []ClaimOrigin {
	out := make([]ClaimOrigin, len(allClaimOrigins))
	copy(out, allClaimOrigins)
	return out
}

// IsValid reports whether o is one of the canonical 5 values
// (including the empty zero value).
func (o ClaimOrigin) IsValid() bool {
	for _, declared := range allClaimOrigins {
		if o == declared {
			return true
		}
	}
	return false
}

// IsLogDerived reports whether the origin came from an attached
// runtime artifact (log or perf trace) rather than current repo.
// True for ClaimOriginLog and ClaimOriginPerf only — cross_source
// returns false because the anchor is also independently confirmed
// by current code.
func (o ClaimOrigin) IsLogDerived() bool {
	return o == ClaimOriginLog || o == ClaimOriginPerf
}

// AuthorityCeiling caps how strongly downstream stages may state a
// claim that rests on this evidence. The four values form a strict
// authority ladder:
//
//   - factual      — strong claims allowed ("X calls Y", "config key Z
//     is set to V"). Anchor came from current repo and grounded at
//     Tier 1/2, OR is cross-source confirmed.
//   - conditional  — claims must be hedged ("according to the log at
//     line N, current code at line M now reads ...; behaviour may
//     have changed"). Anchor came from log/perf but mapped to current
//     code via line_drift or tail_rename.
//   - historical   — claims must be framed as past observations ("the
//     older build observed X; current code has since refactored or
//     removed the corresponding implementation"). Anchor came from
//     log/perf with file_moved, OR mapping to current code failed.
//   - illustrative — must NOT enter causal chains. Anchor is
//     ungrounded or marked illustrative_only by ContextRole.
//
// AuthorityCeiling is the system-computed projection of GroundingTier,
// EvidenceScope, ClaimOrigin, ContextRole, and (for log-derived
// anchors) DriftReason. The LLM never emits this field directly —
// allowing it would let the LLM launder weak claims through declared
// authority. emit_* tools compute it deterministically and pin it on
// the EvidenceItem before persisting to MutableState.
//
// Downstream consumers (extractor, finalizer, contract checker,
// renderer) read this field as a read-only authority cap. The renderer
// injects hedge prefixes/suffixes when ceiling != factual; the
// contract checker raises ViolAuthorityOverreach when prose makes
// strong claims that conflict with the underlying ceiling.
type AuthorityCeiling string

const (
	// AuthorityUnknown is the zero value. Surfaces only via nil-bus
	// paths (tests, single-shot CLI flows that didn't build a bus
	// context); the renderer treats it as factual-equivalent so
	// such items render without hedge prefixes.
	AuthorityUnknown AuthorityCeiling = ""

	// AuthorityFactual permits strong direct claims. Computed when
	// the anchor came from current repo and the grounding tier is
	// among the strong tiers (line_text, symbol_table) at line/range
	// scope, OR when origin is cross_source.
	AuthorityFactual AuthorityCeiling = "factual"

	// AuthorityConditional caps claims at hedged language. Computed
	// when origin is log/perf and drift was detected as line_drift
	// or tail_rename — the underlying observation still maps to
	// current code, but the mapping is approximate.
	AuthorityConditional AuthorityCeiling = "conditional"

	// AuthorityHistorical caps claims at past-tense framing. Computed
	// when origin is log/perf and drift was detected as file_moved
	// (the function lives elsewhere now) OR no current-code mapping
	// could be established at all (the symbol/file no longer exists).
	AuthorityHistorical AuthorityCeiling = "historical"

	// AuthorityIllustrative forbids the item from entering causal
	// chains. Computed when grounding is ungrounded or context role
	// is illustrative_only. The item may still appear as a related
	// example in prose, but never as a chain link.
	AuthorityIllustrative AuthorityCeiling = "illustrative"
)

var allAuthorityCeilings = []AuthorityCeiling{
	AuthorityUnknown,
	AuthorityFactual,
	AuthorityConditional,
	AuthorityHistorical,
	AuthorityIllustrative,
}

// AllAuthorityCeilings returns the canonical list of every
// AuthorityCeiling value in stable declaration order. Callers must
// not mutate the returned slice.
func AllAuthorityCeilings() []AuthorityCeiling {
	out := make([]AuthorityCeiling, len(allAuthorityCeilings))
	copy(out, allAuthorityCeilings)
	return out
}

// IsValid reports whether a is one of the canonical 5 values
// (including the empty zero value).
func (a AuthorityCeiling) IsValid() bool {
	for _, declared := range allAuthorityCeilings {
		if a == declared {
			return true
		}
	}
	return false
}

// authorityRank assigns a strict total order to ceilings for the
// "weakest wins" merge used when an answer symbol is supported by
// multiple evidence items of varying authority. Higher rank = stronger.
//
// Ordering: illustrative < historical < conditional < factual.
// Unknown ranks at the same level as factual to preserve legacy
// behaviour (an un-set field MUST NOT down-rank a strong sibling).
func (a AuthorityCeiling) authorityRank() int {
	switch a {
	case AuthorityIllustrative:
		return 1
	case AuthorityHistorical:
		return 2
	case AuthorityConditional:
		return 3
	case AuthorityFactual:
		return 4
	}
	return 4 // AuthorityUnknown — fail safe: do not pull down stronger siblings
}

// WeakerOf returns the weaker of two ceilings under the strict ordering
// (illustrative < historical < conditional < factual). Used by the
// extractor to pin AnswerSymbol.Authority = min(underlying evidence
// authorities) so an answer symbol's claim strength is bounded by its
// weakest support.
//
// AuthorityUnknown participates as factual-equivalent (zero-value
// safe).
func WeakerOf(a, b AuthorityCeiling) AuthorityCeiling {
	if a.authorityRank() <= b.authorityRank() {
		// Preserve the original zero-value when both are Unknown.
		if a == AuthorityUnknown && b == AuthorityUnknown {
			return AuthorityUnknown
		}
		// If a is Unknown and b is set, return b's strength (Unknown
		// must not pull down).
		if a == AuthorityUnknown {
			return b
		}
		return a
	}
	if b == AuthorityUnknown {
		return a
	}
	return b
}

// AllowsStrongClaim reports whether prose grounded by an item of this
// ceiling may use direct/causal/assertive language. Renderer hedge
// injection inverts this predicate: ceiling whose AllowsStrongClaim
// returns false MUST get a hedge prefix.
func (a AuthorityCeiling) AllowsStrongClaim() bool {
	switch a {
	case AuthorityFactual, AuthorityUnknown:
		return true
	}
	return false
}

// IsCausalChainEligible reports whether an evidence item of this
// ceiling may participate in finalizer causal chains (as opposed to
// appearing only as a related example). Used by the extractor when
// it walks chains: chains containing any AuthorityIllustrative link
// are dropped from the answer surface entirely.
func (a AuthorityCeiling) IsCausalChainEligible() bool {
	return a != AuthorityIllustrative
}
