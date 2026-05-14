package types

import "strings"

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
//   - AnalyzerHints.MentionedEntities and ExactTargets name exact current-turn
//     endpoints already validated upstream.
//   - AnswerContract.MustIncludeTerms contributes kind information so tool
//     names and file stems are not treated as source symbols.
//
// The result is restricted to explanation/mechanism families. Scalar,
// enumeration, config, and relation lookups already have stronger principal
// lanes and should not gain a second anchor-list obligation.
func CompileRequiredMechanismAnchors(rm RequestModel, contract AnswerContract, family QuestionFamily) []AnswerRequiredAnchor {
	if !requiredMechanismAnchorsEnabled(rm, family) {
		return nil
	}
	ordered, mentioned := mechanismAnchorMentionedSet(rm)
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
		key := requiredAnchorKey(text)
		if key == "" {
			return
		}
		if _, ok := mentioned[key]; !ok {
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
			return out
		}
	}
	for _, text := range ordered {
		add(text, InferContractTermKind(text))
		if len(out) >= maxRequiredMechanismAnchors {
			return out
		}
	}
	return out
}

func requiredMechanismAnchorsEnabled(rm RequestModel, family QuestionFamily) bool {
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsRelationalLookup ||
		rm.Intent == IntentReturnValue ||
		rm.Intent == IntentEnumerate ||
		rm.Intent == IntentConfigQuery {
		return false
	}
	switch family {
	case QFArchitecture, QFCallChain, QFGeneric:
		return rm.Intent == IntentExplain ||
			rm.Intent == IntentTrace ||
			rm.Scenario == ScenarioArchitectureExplain
	default:
		return false
	}
}

func mechanismAnchorMentionedSet(rm RequestModel) ([]string, map[string]struct{}) {
	seen := map[string]struct{}{}
	var ordered []string
	add := func(text string) {
		text = strings.TrimSpace(text)
		key := requiredAnchorKey(text)
		if key == "" {
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
	for _, text := range rm.AnalyzerHints.MentionedEntities {
		add(text)
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
// fields: block titles, item labels, and diagram edge endpoints. Summary /
// section prose is deliberately ignored so free-form text does not become a
// control signal.
func MissingRequiredMechanismAnchors(doc *AnswerDocumentV2, required []AnswerRequiredAnchor) []AnswerRequiredAnchor {
	if doc == nil || len(required) == 0 {
		return nil
	}
	present := map[string]struct{}{}
	for _, block := range doc.Blocks {
		recordAnchorSurface(present, block.Title)
		for _, item := range block.Items {
			recordAnchorSurface(present, item.Label)
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
		if _, ok := present[key]; ok {
			continue
		}
		missing = append(missing, anchor)
	}
	return missing
}

func recordAnchorSurface(dst map[string]struct{}, text string) {
	key := requiredAnchorKey(text)
	if key == "" {
		return
	}
	dst[key] = struct{}{}
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
