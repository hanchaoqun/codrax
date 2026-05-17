package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_code_symbol_reconcile.go keeps repo-grounded analyzer
// corrections that depend on the shared qualified-name resolver. The
// rules here do not scan user prose for intent keywords; they consume
// the LLM-emitted typed classification plus repomap symbol existence.

type qualifiedCodeSymbolResolution struct {
	Original string
	Symbol   string
	File     string
}

// reconcileQualifiedCodeSymbolConfigDrift repairs a specific
// structural contradiction: the analyzer sometimes classifies
// multi-segment code surfaces (`pkg.symbol`, `Namespace::method`,
// `Module.Type.member`) as config keys because they contain separators.
//
// The hard signal is repo-grounded and typed: the analyzer must first
// emit an explicit comparison partition (buckets, or cross-component
// sub-topics), and at least two mentioned surfaces must resolve through
// the existing multi-language qualified-name resolver to code symbols.
// MentionedEntities is used only as exact surface provenance for symbol
// lookup; it is not enough by itself to change intent. Only then do we
// move the request out of config_trace/config_mapping and into a bucketed
// mechanism comparison. Real config-key lookups stay untouched when the
// analyzer did not emit a typed comparison signal, the qualified surfaces
// do not resolve as code symbols, the mentioned lane is absent, or the
// user asked for a scalar / role-locate value. This reconcile path
// deliberately does not re-scan rm.RawRequest; raw-text scope/entity
// detection may guide prompts, but it must not drive a hard intent/family
// rewrite.
func reconcileQualifiedCodeSymbolConfigDrift(rm types.RequestModel, graph *repomap.Graph) (types.RequestModel, string) {
	if !qualifiedCodeSymbolConfigDriftCandidate(rm) {
		return rm, ""
	}
	resolved := qualifiedCodeSymbolResolutions(rm, graph)
	if len(resolved) < 2 {
		return rm, ""
	}

	var changes []string
	if rm.Intent == types.IntentConfigQuery {
		rm.Intent = types.IntentExplain
		changes = append(changes, "intent→explain")
	}
	if rm.Scenario == types.ScenarioConfigTrace {
		rm.Scenario = types.ScenarioArchitectureExplain
		changes = append(changes, "scenario→architecture_explain")
	}
	if types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqConfigMapping {
		rm.AnalyzerHints.Kind = string(types.ReqMechanism)
		changes = append(changes, "question_kind→mechanism")
	}
	if rm.AnswerSubject.Kind == types.SubjectConfigKey {
		rm.AnswerSubject = types.AnswerSubject{Kind: types.SubjectGeneric, Confidence: 0.35}
		changes = append(changes, "answer_subject→generic")
	}
	if rm.PredicateAxis == types.AxisConfigure {
		rm.PredicateAxis = types.AxisUnknown
		changes = append(changes, "predicate_axis→unknown")
	}
	if !rm.Predicates.IsCrossComponent {
		rm.Predicates.IsCrossComponent = true
		changes = append(changes, "predicates.is_cross_component→true")
	}

	if len(rm.Buckets) < 2 {
		rm.Buckets = bucketsFromQualifiedCodeSymbols(resolved)
		changes = append(changes, fmt.Sprintf("buckets→%d qualified code symbols", len(rm.Buckets)))
	}
	if len(rm.SubTopics) == 0 {
		rm.SubTopics = subTopicsFromQualifiedCodeSymbols(resolved)
		changes = append(changes, fmt.Sprintf("sub_topics→%d qualified code symbols", len(rm.SubTopics)))
	}
	for _, r := range resolved {
		rm.AnalyzerHints.Entities = appendUniqueString(rm.AnalyzerHints.Entities, r.Symbol)
		rm.AnalyzerHints.Keywords = appendUniqueString(rm.AnalyzerHints.Keywords, r.Symbol)
	}
	if len(changes) == 0 {
		return rm, ""
	}
	return rm, "qualified code symbols are comparison subjects, not config keys: " + strings.Join(changes, ", ")
}

func qualifiedCodeSymbolConfigDriftCandidate(rm types.RequestModel) bool {
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsRoleLocateLookup ||
		rm.Predicates.IsCountQuestion || rm.Predicates.IsHistoryLookup {
		return false
	}
	if rm.DiagnosticProfile.RequiresDiagnosticRootCause() || rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if !qualifiedCodeSymbolTypedComparisonSignal(rm) {
		return false
	}
	return rm.Intent == types.IntentConfigQuery ||
		rm.Scenario == types.ScenarioConfigTrace ||
		types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) == types.ReqConfigMapping ||
		rm.AnswerSubject.Kind == types.SubjectConfigKey ||
		rm.PredicateAxis == types.AxisConfigure
}

func qualifiedCodeSymbolTypedComparisonSignal(rm types.RequestModel) bool {
	if len(rm.Buckets) >= 2 {
		return true
	}
	return rm.Predicates.IsCrossComponent && len(rm.SubTopics) >= 2
}

func qualifiedCodeSymbolResolutions(rm types.RequestModel, graph *repomap.Graph) []qualifiedCodeSymbolResolution {
	if graph == nil || len(graph.SymbolDefs) == 0 {
		return nil
	}
	candidates := rm.AnalyzerHints.MentionedEntities
	if len(candidates) == 0 {
		return nil
	}

	entities := make(map[string]string, len(candidates))
	var ordered []string
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, prefix := expandEntityNameAliases(candidate); len(prefix) == 0 {
			continue
		}
		key := strings.ToLower(candidate)
		if _, exists := entities[key]; exists {
			continue
		}
		entities[key] = candidate
		ordered = append(ordered, candidate)
	}
	if len(entities) == 0 {
		return nil
	}

	byOriginal := make(map[string]qualifiedCodeSymbolResolution, len(entities))
	forEachMatchingDef(entities, graph, func(_, entOrig, symName string, d *repomap.Symbol) bool {
		if _, exists := byOriginal[entOrig]; exists {
			return true
		}
		byOriginal[entOrig] = qualifiedCodeSymbolResolution{
			Original: entOrig,
			Symbol:   symName,
			File:     strings.TrimSpace(d.File),
		}
		return true
	})
	if len(byOriginal) == 0 {
		return nil
	}

	out := make([]qualifiedCodeSymbolResolution, 0, len(byOriginal))
	for _, original := range ordered {
		if r, ok := byOriginal[original]; ok {
			out = append(out, r)
		}
	}
	return out
}

func bucketsFromQualifiedCodeSymbols(resolved []qualifiedCodeSymbolResolution) []types.QuestionBucket {
	out := make([]types.QuestionBucket, 0, len(resolved))
	for _, r := range resolved {
		anchors := []string{r.Original}
		if r.Symbol != "" && !strings.EqualFold(r.Symbol, r.Original) {
			anchors = append(anchors, r.Symbol)
		}
		out = append(out, types.QuestionBucket{
			Label:   r.Original,
			Anchors: anchors,
			Index:   len(out) + 1,
		})
	}
	return out
}

func subTopicsFromQualifiedCodeSymbols(resolved []qualifiedCodeSymbolResolution) []types.SubTopic {
	out := make([]types.SubTopic, 0, len(resolved))
	for _, r := range resolved {
		entities := []string{r.Original}
		if r.Symbol != "" && !strings.EqualFold(r.Symbol, r.Original) {
			entities = append(entities, r.Symbol)
		}
		out = append(out, types.SubTopic{
			Summary:  "Compare grounded code symbol " + r.Original,
			Entities: entities,
		})
	}
	return out
}

func appendUniqueString(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	for _, existing := range in {
		if strings.EqualFold(strings.TrimSpace(existing), value) {
			return in
		}
	}
	return append(in, value)
}
