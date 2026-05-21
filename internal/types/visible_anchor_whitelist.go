package types

import (
	"sort"
	"strings"
)

// VisibleAnchorWhitelist is the per-dispatch authoritative slate of
// every symbol the finalizer's answer document MAY cite as an item
// label, an inline backtick reference, a diagram edge endpoint, or
// any other surface where the post-emit hallucination oracles fire.
//
// The 2026-05-17 T1 architectural fix surfaces this list in the
// finalizer prompt BEFORE emit time so the model writes against an
// explicit constraint space instead of inferring valid surfaces from
// scattered evidence-pool fragments and discovering misses only
// through retry rounds. Same data flows through the post-emit
// validators (validateEnumerationItemLabelHallucination /
// validateInlineIdentifierHallucination /
// validateDiagramEdgeEndpointHallucination); aligning the pre-emit
// slate with the post-emit check eliminates the antagonistic-
// constraint pattern where the model fixes one oracle and silently
// trips another.
//
// Structure choice — Required vs Groundable split — keeps the
// per-section signal precise: "you MUST surface these" is a stronger
// directive than "you MAY cite these" and downstream prompt
// rendering can emphasize accordingly. Surfaces never appear in
// both buckets; Required wins on overlap.
type VisibleAnchorWhitelist struct {
	// Required is the must-surface set drawn from the analyzer's
	// AnswerSemanticView.RequiredMechanismAnchors. Each entry's
	// Symbol matches a typed contract obligation; the finalizer's
	// normalizeRequiredMechanismAnchorCarriers will auto-inject the
	// label if the LLM omits it, so pre-flight visibility is the
	// model's chance to place each anchor with prose context rather
	// than letting the auto-injection drop it into a default block.
	Required []VisibleAnchorEntry

	// Groundable is the open set of symbols sourced from typed
	// AnswerSupportEntry projections (AnchorSymbol / Subject /
	// Object) and the answer-symbol slate. The model may cite any
	// of these without tripping a hallucination oracle. Items
	// missing from this set are NOT necessarily wrong — the
	// underlying support plan is per-family — but a citation
	// outside the union risks the oracle when no other typed signal
	// vouches for the surface.
	Groundable []VisibleAnchorEntry
}

// VisibleAnchorEntry is one symbol the LLM may use in the answer
// document. Symbol is the leading identifier the post-emit oracle
// would resolve (the bare form, no decorator); SurfaceForms lists
// alternative spellings the SymbolOracle's flat-form index accepts
// equivalently (e.g. `runContractCheck` ↔ `run_contract_check`).
type VisibleAnchorEntry struct {
	// Symbol is the bare leading identifier (e.g. "Orchestrator",
	// "runContractCheck"). This is the form the post-emit oracles
	// compare against the codebase symbol graph.
	Symbol string

	// QualifiedSymbol is the typed-relation form when available
	// ("Type.Method" / "package.Function"); empty when the
	// underlying evidence carries only a bare identifier.
	QualifiedSymbol string

	// SourceFile + SourceLine point at the grounded citation the
	// model should pair with this symbol. Empty SourceLine means
	// the underlying evidence is symbol-only (no file:line locus).
	SourceFile string
	SourceLine int

	// Kind classifies the symbol's role (function / type /
	// constant / member / unknown). The downstream renderer uses
	// this to group entries and to phrase the constraint hint.
	Kind VisibleAnchorKind

	// SurfaceForms lists alternative spellings the SymbolOracle's
	// flat-form index will accept as equivalent (CamelCase ↔
	// snake_case ↔ kebab-case ↔ yaml-tag forms). Always contains
	// at least Symbol itself when non-empty. Empty when the
	// underlying evidence did not surface alternative forms.
	SurfaceForms []string

	// GroundingTier captures the underlying evidence's confidence
	// tier so the renderer can warn the model when only Recovered
	// / lower-tier entries back a surface — "cite this with a
	// caveat" rather than "cite this freely".
	GroundingTier GroundingTier

	// Origin tags the upstream source of this entry for telemetry:
	//
	//   - "required_mechanism_anchor" — analyzer-emitted obligation
	//   - "support_lane:<lane-kind>"  — typed support plan entry
	//   - "answer_symbol"             — extractor's emit_answer_symbol slate
	//
	// Origin is not user-visible; it's a debugging breadcrumb.
	Origin string
}

// VisibleAnchorKind classifies a whitelist entry's role. Stored as a
// string so the renderer can format directly without a switch on
// every kind, and so JSON traces preserve the value verbatim.
type VisibleAnchorKind string

const (
	VisibleAnchorKindSymbol        VisibleAnchorKind = "symbol"
	VisibleAnchorKindFunction      VisibleAnchorKind = "function"
	VisibleAnchorKindType          VisibleAnchorKind = "type"
	VisibleAnchorKindConstant      VisibleAnchorKind = "constant"
	VisibleAnchorKindMember        VisibleAnchorKind = "member"
	VisibleAnchorKindCallSite      VisibleAnchorKind = "call_site"
	VisibleAnchorKindCondition     VisibleAnchorKind = "condition"
	VisibleAnchorKindReturn        VisibleAnchorKind = "return"
	VisibleAnchorKindAssignment    VisibleAnchorKind = "assignment"
	VisibleAnchorKindInitializer   VisibleAnchorKind = "initializer"
	VisibleAnchorKindImport        VisibleAnchorKind = "import"
	VisibleAnchorKindStringLiteral VisibleAnchorKind = "string_literal"
	VisibleAnchorKindOther         VisibleAnchorKind = "other"
)

// MaxVisibleAnchorWhitelistEntries caps the rendered slate so a
// dispatch with a long lane chain (drift-bounded root-cause +
// observed-anchors + nearest-mechanism + boundary disclosures, all
// populated) cannot blow past a sensible prompt-section budget. 60
// entries cover the median P95 finalize dispatch by a comfortable
// margin per the May-16 sweep; truncation past the cap surfaces a
// "+N more" note so the model knows the slate is bounded.
const MaxVisibleAnchorWhitelistEntries = 60

// BuildVisibleAnchorWhitelist assembles the per-dispatch authoritative
// surface slate from typed inputs already on the AnswerSupportPlan and
// AnswerSemanticView. Pure function: same inputs always produce the
// same output. Caller hands in nil-safe plan/view; either nil yields
// an empty whitelist (no panic).
//
// Order discipline:
//
//   - Required entries follow the AnswerRequiredAnchor slice's order
//     (user-mention sequence preserved upstream by
//     mechanismAnchorMentionedSet).
//   - Groundable entries are sorted by (GroundingTier ascending,
//     Symbol ascending) so the highest-confidence anchors lead. Tier
//     ascending puts Tier 1 / 2 (high confidence) before Recovered /
//     Tier 3 / Ungrounded — the renderer's "safe to cite" vs "cite
//     with caveat" boundary maps to this order.
//
// Dedup discipline: (Symbol, SourceFile, SourceLine) tuple identity.
// A symbol appearing in both Required and Groundable lands in
// Required only — the Groundable bucket is the strict complement.
func BuildVisibleAnchorWhitelist(plan *AnswerSupportPlan, view *AnswerSemanticView) *VisibleAnchorWhitelist {
	out := &VisibleAnchorWhitelist{}
	requiredKeys := make(map[string]struct{})

	// Required pass — preserves the slice's user-mention order.
	if view != nil {
		for _, anchor := range view.RequiredMechanismAnchors {
			entry := visibleAnchorEntryFromRequired(anchor)
			if entry.Symbol == "" {
				continue
			}
			key := visibleAnchorEntryKey(entry)
			if _, dup := requiredKeys[key]; dup {
				continue
			}
			requiredKeys[key] = struct{}{}
			out.Required = append(out.Required, entry)
			if len(out.Required) >= MaxVisibleAnchorWhitelistEntries {
				break
			}
		}
	}

	// Groundable pass — strict complement of Required. Walk every
	// support lane entry; project to a whitelist entry only when the
	// typed evidence carries an AnchorSymbol / Subject / Object the
	// model can cite verbatim.
	groundableKeys := make(map[string]struct{})
	groundable := make([]VisibleAnchorEntry, 0, 32)
	if plan != nil {
		for _, lane := range plan.Lanes {
			origin := visibleAnchorOriginForLane(lane.Kind)
			for _, raw := range lane.Entries {
				for _, entry := range visibleAnchorEntriesFromSupport(raw, origin) {
					if entry.Symbol == "" {
						continue
					}
					key := visibleAnchorEntryKey(entry)
					if _, isRequired := requiredKeys[key]; isRequired {
						continue
					}
					if _, dup := groundableKeys[key]; dup {
						continue
					}
					groundableKeys[key] = struct{}{}
					groundable = append(groundable, entry)
				}
			}
		}
	}

	sort.SliceStable(groundable, func(i, j int) bool {
		ti, tj := groundable[i].GroundingTier, groundable[j].GroundingTier
		if ti != tj {
			return visibleAnchorTierRank(ti) < visibleAnchorTierRank(tj)
		}
		return groundable[i].Symbol < groundable[j].Symbol
	})

	remaining := MaxVisibleAnchorWhitelistEntries - len(out.Required)
	if remaining < 0 {
		remaining = 0
	}
	if len(groundable) > remaining {
		groundable = groundable[:remaining]
	}
	out.Groundable = groundable
	return out
}

// IsEmpty reports whether the whitelist carries no actionable
// constraint. A nil receiver, an empty Required + empty Groundable,
// is functionally equivalent to "no pre-flight slate available" and
// the renderer should short-circuit to keep the prompt clean.
func (w *VisibleAnchorWhitelist) IsEmpty() bool {
	if w == nil {
		return true
	}
	return len(w.Required) == 0 && len(w.Groundable) == 0
}

// visibleAnchorEntryFromRequired projects an AnswerRequiredAnchor to
// a whitelist entry. The required-anchor carries Text + Kind, no
// file:line — the renderer flags this as "the symbol must appear, the
// citation comes from the support lane / evidence pool".
func visibleAnchorEntryFromRequired(a AnswerRequiredAnchor) VisibleAnchorEntry {
	text := strings.TrimSpace(a.Text)
	if text == "" {
		return VisibleAnchorEntry{}
	}
	return VisibleAnchorEntry{
		Symbol: text,
		Kind:   visibleAnchorKindFromContractTerm(a.Kind),
		Origin: "required_mechanism_anchor",
	}
}

// visibleAnchorEntriesFromSupport projects ONE support-lane entry to
// at most three whitelist entries (AnchorSymbol + Subject + Object,
// when distinct). SurfaceTerms are folded into each entry's
// SurfaceForms when they identifier-shaped.
func visibleAnchorEntriesFromSupport(raw AnswerSupportEntry, origin string) []VisibleAnchorEntry {
	source := strings.TrimSpace(raw.Source)
	line := raw.LineStart
	tier := raw.GroundingTier
	kind := visibleAnchorKindFromAnchorKind(raw.AnchorKind)
	candidates := visibleAnchorCandidateSurfaces(raw)
	surfaceForms := visibleAnchorIdentifierSurfaceTerms(raw.SurfaceTerms)

	out := make([]VisibleAnchorEntry, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		if !IsCodeIdentitySurface(c) {
			continue
		}
		entry := VisibleAnchorEntry{
			Symbol:          visibleAnchorBareSymbol(c),
			QualifiedSymbol: visibleAnchorQualifiedSymbol(c),
			SourceFile:      source,
			SourceLine:      line,
			Kind:            kind,
			GroundingTier:   tier,
			Origin:          origin,
			SurfaceForms:    surfaceForms,
		}
		if entry.Symbol == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// visibleAnchorCandidateSurfaces returns the identity surfaces that are safe
// to pair with raw.Source:raw.LineStart in the prompt-side anchor guide.
// Definition evidence can expose the symbol and its owner surfaces. Non-
// definition evidence must stay role-accurate: a call/condition/assignment
// line proves the visible target on that line, not the caller/owner's
// definition. This prevents the guide from advertising "Foo @ call-site line"
// as a definition anchor and sending the model into avoidable line repairs.
func visibleAnchorCandidateSurfaces(raw AnswerSupportEntry) []string {
	push := func(out *[]string, values ...string) {
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v != "" {
				*out = append(*out, v)
			}
		}
	}
	var out []string
	switch raw.AnchorKind {
	case AnchorDefinition, "":
		push(&out, raw.AnchorSymbol, raw.Subject, raw.Object)
	case AnchorCall:
		push(&out, raw.AnchorSymbol, raw.Object)
	case AnchorReturn, AnchorAssignment, AnchorInitializer, AnchorCondition, AnchorImport, AnchorStringLiteral:
		push(&out, raw.AnchorSymbol, raw.Object)
	default:
		push(&out, raw.AnchorSymbol)
	}
	return out
}

// visibleAnchorIdentifierSurfaceTerms filters SurfaceTerms to the
// identifier-shaped subset (ASCII letters/digits + the canonical
// separator set). Non-identifier surfaces (prose phrases, sentence
// fragments) are dropped: the SymbolOracle.SymbolExistsFlat path
// only resolves identifier shapes.
func visibleAnchorIdentifierSurfaceTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	out := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		clean := strings.TrimSpace(t)
		if clean == "" || seen[clean] {
			continue
		}
		if !IsCodeIdentitySurface(clean) {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// visibleAnchorBareSymbol returns the leading identifier of a possibly-
// qualified surface ("Type.Method" → "Method"; "package::Type" → "Type";
// bare "Foo" → "Foo"). The bare form is what the post-emit oracle's
// flat-form index resolves against.
func visibleAnchorBareSymbol(s string) string {
	for _, sep := range []string{"::", "/", "."} {
		if idx := strings.LastIndex(s, sep); idx >= 0 && idx+len(sep) < len(s) {
			return s[idx+len(sep):]
		}
	}
	return s
}

// visibleAnchorQualifiedSymbol returns the full qualified form when
// the surface contains a separator (Type.Method / package::Type),
// empty otherwise. The renderer shows this alongside the bare form so
// the model knows whether the answer should refer to the qualified
// or the bare spelling.
func visibleAnchorQualifiedSymbol(s string) string {
	for _, sep := range []string{"::", "/", "."} {
		if strings.Contains(s, sep) {
			return s
		}
	}
	return ""
}

func visibleAnchorEntryKey(e VisibleAnchorEntry) string {
	return e.Symbol + "|" + e.SourceFile + "|" + intToString(e.SourceLine)
}

func visibleAnchorOriginForLane(kind AnswerSupportLaneKind) string {
	if kind == "" {
		return "support_lane"
	}
	return "support_lane:" + string(kind)
}

func visibleAnchorKindFromContractTerm(k ContractTermKind) VisibleAnchorKind {
	switch k {
	case ContractTermSymbol:
		return VisibleAnchorKindSymbol
	case ContractTermToolName:
		return VisibleAnchorKindOther
	case ContractTermFileStem:
		return VisibleAnchorKindOther
	}
	return VisibleAnchorKindOther
}

func visibleAnchorKindFromAnchorKind(k AnchorKind) VisibleAnchorKind {
	switch k {
	case AnchorDefinition:
		return VisibleAnchorKindFunction
	case AnchorCall:
		return VisibleAnchorKindCallSite
	case AnchorReturn:
		return VisibleAnchorKindReturn
	case AnchorAssignment:
		return VisibleAnchorKindAssignment
	case AnchorInitializer:
		return VisibleAnchorKindInitializer
	case AnchorCondition:
		return VisibleAnchorKindCondition
	case AnchorImport:
		return VisibleAnchorKindImport
	case AnchorStringLiteral:
		return VisibleAnchorKindStringLiteral
	}
	return VisibleAnchorKindOther
}

// visibleAnchorTierRank orders GroundingTier values so line-anchored
// tiers rank before symbol-table tiers, both before fuzzy / nearest
// recovery tiers, all before the unknown / empty tier. Comparable via
// integer ordering; smaller = higher confidence.
//
// Tier classification mirrors the internal/tool/ground/ vocabulary
// without replicating the validation logic — the rank is presentation-
// layer ordering only, not a grounding-status decision.
func visibleAnchorTierRank(t GroundingTier) int {
	switch t {
	case TierLineText, TierFileLayer, TierLineRange:
		return 0 // direct evidence at the cited line / file / range
	case TierSymbolTable, TierFQNameSameFile,
		TierSectionParse, TierCrossfileVerified, TierNegativeConfirmed:
		return 1 // typed graph / parse-tree resolution
	case TierSnippetFuzzy, TierPackageSymbol,
		TierNearestCall, TierNearestCondition:
		return 2 // recovery tiers — cite with caveat
	}
	return 3 // empty / unknown
}

// IsHighConfidenceTier reports whether an entry's GroundingTier sits
// in the "safe to cite freely" band (rank 0-1). The renderer uses
// this to split the Groundable section into two sub-buckets — a
// strong signal to the model that lower-rank entries should carry an
// uncertainty caveat when promoted to principal claims.
func (e VisibleAnchorEntry) IsHighConfidenceTier() bool {
	return visibleAnchorTierRank(e.GroundingTier) <= 1
}

// intToString is a small helper for visibleAnchorEntryKey to avoid
// pulling strconv into a key-building hot path.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
