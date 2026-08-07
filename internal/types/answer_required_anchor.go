package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
)

const maxRequiredMechanismAnchors = 6

// AnswerRequiredAnchor is a typed visible-anchor obligation for
// mechanism-style answers. It preserves exact code/tool/file endpoints that
// came through analyzer-authored typed lanes, without deriving control flow
// from user prose or final rendered prose.
type AnswerRequiredAnchor struct {
	Text string           `json:"text"`
	Kind ContractTermKind `json:"kind"`
}

// CompileRequiredMechanismAnchors converts analyzer-visible exact anchors into
// a final-answer structural obligation. It intentionally consumes only typed
// RequestModel / AnswerContract fields:
//   - AnalyzerHints.ExactTargets names exact current-turn identities already
//     validated upstream. MentionedEntities remains exploration context and is
//     deliberately too noisy to create a hard publication obligation.
//   - QFCallChain additionally consumes the precise ordered endpoint profile:
//     its source is authoritative in exact/discover mode, while its sink is
//     authoritative only in exact mode.
//   - AnswerContract.MustIncludeTerms contributes kind information so tool
//     names and file stems are not treated as source symbols.
//
// The result is restricted to explanation/mechanism families. Scalar,
// enumeration, config, and relation lookups already have stronger principal
// lanes and should not gain a second anchor-list obligation. QFCallChain is a
// deliberate exception to the relation/category suppression: source and sink
// names are endpoint identity, not answer-member enumeration, and must remain
// visible even when the analyzer also marks the path as relational.
//
// subRepoNames is the snapshot of multi-repo identifiers (Slug + RootRel +
// RootRel basename) stamped on the AnswerSurfacePlan. Entities matching this
// set are repository / project names, not code symbols, so they are filtered
// out of the candidate pool before InferContractTermKind's identifier-shape
// heuristic mistakes them for ContractTermSymbol. Without this filter, the
// 2026-05-16 architectural-comparison loop (subject names "codrax" / "opencode"
// auto-injected as item labels then rejected as ungrounded enumeration labels)
// reproduces.
func CompileRequiredMechanismAnchors(rm RequestModel, contract AnswerContract, family QuestionFamily, subRepoNames []string) []AnswerRequiredAnchor {
	if !requiredMechanismAnchorsEnabled(rm, family) {
		return nil
	}
	ordered, exact := mechanismAnchorHardCandidateSet(rm, family, subRepoNames)
	if len(ordered) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ordered))
	out := make([]AnswerRequiredAnchor, 0, minInt(maxRequiredMechanismAnchors, len(ordered)))
	add := func(text string, kind ContractTermKind) {
		text = strings.TrimSpace(text)
		if text == "" || !mechanismAnchorTermKindEligible(kind) {
			return
		}
		// A call-chain obligation names executable source/sink symbols. Analyzer
		// context also carries source files in MentionedEntities, but a file is
		// evidence context rather than a graph endpoint. Keep the distinction
		// typed and suffix-based: qualified symbols such as gate.Run still pass,
		// while analyzer.go and internal/agent/analyzer.go do not become visible
		// endpoint obligations.
		if family == QFCallChain && HasCodeOrConfigPathSuffix(text) {
			return
		}
		key := requiredAnchorKey(text)
		if key == "" {
			return
		}
		if _, ok := exact[key]; !ok {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, AnswerRequiredAnchor{Text: text, Kind: kind})
	}

	for _, term := range NormalizedMustIncludeTerms(contract) {
		kind := term.Kind
		if !kind.IsValid() {
			kind = InferContractTermKind(term.Text)
		}
		add(term.Text, kind)
		if len(out) >= maxRequiredMechanismAnchors {
			logging.Debug("required mechanism anchors capped at %d; later candidates not carried", maxRequiredMechanismAnchors)
			return out
		}
	}
	for i, text := range ordered {
		add(text, InferContractTermKind(text))
		if len(out) >= maxRequiredMechanismAnchors {
			if i+1 < len(ordered) {
				logging.Debug("required mechanism anchors capped at %d; %d later candidate(s) not carried", maxRequiredMechanismAnchors, len(ordered)-i-1)
			}
			return out
		}
	}
	return out
}

func requiredMechanismAnchorsEnabled(rm RequestModel, family QuestionFamily) bool {
	if RuntimeSourceRequestSuppressesCurrentSourceAnswerContract(&rm, TurnRouteHint{}) {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Intent == IntentReturnValue ||
		rm.Intent == IntentConfigQuery {
		return false
	}
	if family == QFCallChain {
		return rm.Intent == IntentExplain || rm.Intent == IntentTrace
	}
	if rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Intent == IntentEnumerate {
		return false
	}
	switch family {
	case QFArchitecture, QFGeneric:
		return rm.Intent == IntentExplain ||
			rm.Intent == IntentTrace ||
			rm.Scenario == ScenarioArchitectureExplain
	default:
		return false
	}
}

func mechanismAnchorHardCandidateSet(rm RequestModel, family QuestionFamily, subRepoNames []string) ([]string, map[string]struct{}) {
	// Repository / project identifiers are not eligible mechanism anchors.
	// They look identifier-shaped to InferContractTermKind (which yields
	// ContractTermSymbol) but they have no code definition site; promoting
	// them into RequiredMechanismAnchor causes the pre-emit normaliser to
	// auto-inject them as item labels, which the post-emit
	// enumeration-label oracle then rejects as ungrounded.
	subRepoFilter := make(map[string]struct{}, len(subRepoNames))
	for _, name := range subRepoNames {
		key := requiredAnchorKey(name)
		if key == "" {
			continue
		}
		subRepoFilter[key] = struct{}{}
	}
	seen := map[string]struct{}{}
	var ordered []string
	add := func(text string) {
		text = strings.TrimSpace(text)
		key := requiredAnchorKey(text)
		if key == "" {
			return
		}
		if _, isSubRepo := subRepoFilter[key]; isSubRepo {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		ordered = append(ordered, text)
	}
	for _, text := range rm.AnalyzerHints.ExactTargets {
		add(text)
	}
	if family == QFCallChain && rm.CallChainEndpointProfile.Active() {
		add(rm.CallChainEndpointProfile.Source)
		if rm.CallChainEndpointProfile.ExactActive() {
			add(rm.CallChainEndpointProfile.Sink)
		}
	}
	return ordered, seen
}

func mechanismAnchorTermKindEligible(kind ContractTermKind) bool {
	switch kind {
	case ContractTermSymbol, ContractTermToolName, ContractTermFileStem:
		return true
	default:
		return false
	}
}

// MissingRequiredMechanismAnchors reports typed visible-anchor obligations not
// represented by the structured answer carrier. It compares only exact typed
// fields: block titles, item labels, typed-relation label endpoints, table
// cells, and diagram edge endpoints.
// Summary / section prose is deliberately ignored so free-form text does not
// become a control signal. Cells are scanned by verbatim containment of the
// typed anchor (the must_include oracle's primitive): a table row legitimately
// annotates its carrier ("MutableState（并行 fork 合并）"), and containment
// only SUPPRESSES a would-be violation — false positives quiet the check,
// they never create one.
func MissingRequiredMechanismAnchors(doc *AnswerDocumentV2, required []AnswerRequiredAnchor) []AnswerRequiredAnchor {
	if doc == nil || len(required) == 0 {
		return nil
	}
	present := map[string]struct{}{}
	var cellTexts []string
	for _, block := range doc.Blocks {
		recordAnchorSurface(present, block.Title)
		relationLabelCarrier := answerBlockCarriesTypedIdentityRelation(block)
		for _, item := range block.Items {
			recordAnchorSurface(present, item.Label)
			if relationLabelCarrier {
				recordTypedRelationLabelEndpoints(present, item.Label)
			}
			for _, cell := range item.Cells {
				if trimmed := strings.ToLower(strings.TrimSpace(cell)); trimmed != "" {
					cellTexts = append(cellTexts, trimmed)
				}
			}
		}
		for _, edge := range block.EdgeAnchors {
			recordAnchorSurface(present, edge.FromNode)
			recordAnchorSurface(present, edge.ToNode)
		}
	}
	var missing []AnswerRequiredAnchor
	seenRequired := map[string]struct{}{}
	for _, anchor := range required {
		key := requiredAnchorKey(anchor.Text)
		if key == "" {
			continue
		}
		if _, dup := seenRequired[key]; dup {
			continue
		}
		seenRequired[key] = struct{}{}
		if requiredAnchorAnyKeyPresent(anchor.Text, present) {
			continue
		}
		if requiredAnchorPresentInCells(key, cellTexts) {
			continue
		}
		missing = append(missing, anchor)
	}
	return missing
}

// answerBlockCarriesTypedIdentityRelation reports whether the model explicitly
// declared that a block carries endpoint-bearing relations. This is a precise
// structured signal: it never infers relation meaning from an item label or
// surrounding prose. The supported forms share the citation-role identity
// contract used by the evidence alignment validator.
func answerBlockCarriesTypedIdentityRelation(block AnswerBlock) bool {
	for _, use := range block.ClaimUses {
		if use.ClaimForm.SupportsCitationRoleAlignment() {
			return true
		}
	}
	return false
}

// recordTypedRelationLabelEndpoints records the two exact endpoint identities
// from a typed relation label. ASCII arrows require surrounding spaces so C++
// member access such as receiver->method and operator-> are never mistaken for
// answer-graph relations. Exactly one arrow is accepted; multi-hop labels stay
// out of this carrier because one block-level claim annotation cannot prove
// every hop. Free-form Text and summary prose never reach this function.
func recordTypedRelationLabelEndpoints(dst map[string]struct{}, label string) {
	left, right, ok := splitTypedRelationLabel(label)
	if !ok {
		return
	}
	recordAnchorSurface(dst, left)
	recordAnchorSurface(dst, right)
}

func splitTypedRelationLabel(label string) (string, string, bool) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", "", false
	}
	matched := ""
	matchedAt := -1
	for _, delimiter := range []string{" -> ", " → "} {
		if at := strings.Index(label, delimiter); at >= 0 {
			if matched != "" || strings.Contains(label[at+len(delimiter):], delimiter) {
				return "", "", false
			}
			matched = delimiter
			matchedAt = at
		}
	}
	if matchedAt < 0 {
		return "", "", false
	}
	left := strings.TrimSpace(label[:matchedAt])
	right := strings.TrimSpace(label[matchedAt+len(matched):])
	if left == "" || right == "" ||
		strings.Contains(left, " -> ") || strings.Contains(left, " → ") ||
		strings.Contains(right, " -> ") || strings.Contains(right, " → ") {
		return "", "", false
	}
	return left, right, true
}

// requiredAnchorPresentInCells reports whether the canonical anchor
// key appears verbatim inside any table cell.
func requiredAnchorPresentInCells(key string, cells []string) bool {
	if key == "" {
		return false
	}
	for _, cell := range cells {
		if _, _, qualified := requiredAnchorQualifiedParts(key); qualified {
			if requiredAnchorQualifiedTokenPresent(cell, key) {
				return true
			}
			continue
		}
		if strings.Contains(cell, key) {
			return true
		}
	}
	return false
}

func requiredAnchorQualifiedTokenPresent(cell, key string) bool {
	for from := 0; from <= len(cell)-len(key); {
		rel := strings.Index(cell[from:], key)
		if rel < 0 {
			return false
		}
		start := from + rel
		end := start + len(key)
		leftOK := start == 0 || !requiredAnchorIdentifierByte(cell[start-1])
		rightOK := end == len(cell) || !requiredAnchorIdentifierByte(cell[end])
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
	return false
}

func requiredAnchorIdentifierByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '_' || b == '.' || b == ':'
}

func recordAnchorSurface(dst map[string]struct{}, text string) {
	for _, key := range requiredAnchorSurfaceKeys(text) {
		dst[key] = struct{}{}
	}
}

func requiredAnchorKey(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "()")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.ToLower(text)
}

func requiredAnchorAnyKeyPresent(text string, present map[string]struct{}) bool {
	for _, key := range requiredAnchorRequiredKeys(text) {
		if _, ok := present[key]; ok {
			return true
		}
	}
	return false
}

// requiredAnchorRequiredKeys preserves the identity boundary of a qualified
// required endpoint. A present surface may still expand into owner/member keys
// (so StageOutput.AnalysisIR can carry the two simple anchors StageOutput and
// AnalysisIR), but a required gate.Run must not itself degrade to owner=gate
// or member=run: that would let gate.RunWith satisfy it through the shared
// owner. Matching remains exact after deterministic punctuation compaction.
func requiredAnchorRequiredKeys(text string) []string {
	primary := requiredAnchorKey(text)
	if primary == "" {
		return nil
	}
	out := []string{primary}
	if compact := requiredAnchorCompactKey(primary); compact != "" {
		out = append(out, "compact:"+compact)
	}
	return out
}

func requiredAnchorSurfaceKeys(text string) []string {
	primary := requiredAnchorKey(text)
	if primary == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(key string) {
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			return
		}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	add(primary)
	if compact := requiredAnchorCompactKey(primary); compact != "" {
		add("compact:" + compact)
	}
	if owner, member, ok := requiredAnchorQualifiedParts(primary); ok {
		add(owner)
		add(member)
		if compact := requiredAnchorCompactKey(owner); compact != "" {
			add("compact:" + compact)
		}
		if compact := requiredAnchorCompactKey(member); compact != "" {
			add("compact:" + compact)
		}
	}
	return out
}

func requiredAnchorQualifiedParts(key string) (owner, member string, ok bool) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" || strings.Contains(key, "/") {
		return "", "", false
	}
	var parts []string
	switch {
	case strings.Count(key, ".") == 1:
		parts = strings.Split(key, ".")
	case strings.Count(key, "::") == 1:
		parts = strings.Split(key, "::")
	default:
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	member = strings.TrimSpace(parts[1])
	if owner == "" || member == "" {
		return "", "", false
	}
	return owner, member, true
}

func requiredAnchorCompactKey(key string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(key)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
