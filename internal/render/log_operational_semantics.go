package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const maxDetectedLogOperationalSemantics = 16

var (
	logOperationalProgressRE = regexp.MustCompile(`(?:^|\s)([1-9][0-9]*)/([1-9][0-9]*)(?:\s|$)`)
	logOperationalRenderRE   = regexp.MustCompile(`^(?:[0-9]+\s*│\s*)?(?:\S+\s+)?(?:TRACE|DEBUG|INFO|WARN|WARNING|ERROR)\s+\[render\]\s+`)
	logOperationalAttemptRE  = regexp.MustCompile(`^(?:[0-9]+\s*│\s*)?(?:\S+\s+)?(?:TRACE|DEBUG|INFO|WARN|WARNING|ERROR)\s+\[orchestrator\]\s+([A-Za-z][A-Za-z0-9_-]*)\s+attempt\s+([1-9][0-9]*)/([1-9][0-9]*)\s+failed(?:\s*:|\s|$)`)
)

type logOperationalStagePhrase struct {
	stage     string
	lifecycle string
	phrase    string
}

// DetectLogOperationalSemantics decodes only exact Codrax producer grammars.
// It does not search user prose or model output. In particular, renderer K/N
// is accepted only when the localized lifecycle phrase identifies one stage
// and progressForStageKey confirms the same K/N in the producer's own table.
func DetectLogOperationalSemantics(rawLog string) []types.LogOperationalSemantic {
	if strings.TrimSpace(rawLog) == "" {
		return nil
	}
	phrases := logOperationalStagePhrases()
	var out []types.LogOperationalSemantic
	for lineIndex, rawLine := range strings.Split(rawLog, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if logOperationalRenderRE.MatchString(line) {
			if semantic, ok := decodeRendererOperationalSemantic(line, lineIndex+1, phrases); ok {
				out = append(out, semantic)
			}
		}
		if semantic, ok := decodeDispatchAttemptOperationalSemantic(line, lineIndex+1); ok {
			out = append(out, semantic)
		}
		if len(out) >= maxDetectedLogOperationalSemantics {
			break
		}
	}
	return out
}

func decodeRendererOperationalSemantic(
	line string,
	lineNumber int,
	phrases []logOperationalStagePhrase,
) (types.LogOperationalSemantic, bool) {
	for _, candidate := range phrases {
		phraseAt := strings.Index(line, candidate.phrase)
		if phraseAt < 0 {
			continue
		}
		prefix := line[:phraseAt]
		matches := logOperationalProgressRE.FindAllStringSubmatch(prefix, -1)
		if len(matches) == 0 {
			continue
		}
		match := matches[len(matches)-1]
		numerator, errN := strconv.Atoi(match[1])
		denominator, errD := strconv.Atoi(match[2])
		if errN != nil || errD != nil || denominator <= 0 || numerator > denominator {
			continue
		}
		progress := fmt.Sprintf("%d/%d", numerator, denominator)
		if progressForStageKey(candidate.stage, denominator) != progress {
			continue
		}
		return types.LogOperationalSemantic{
			Protocol:            "codrax_status_v1",
			Producer:            "render",
			EventKind:           types.LogOperationalEventPipelineStageLifecycle,
			Subject:             candidate.stage,
			StageKey:            candidate.stage,
			Lifecycle:           candidate.lifecycle,
			CounterDomain:       types.LogOperationalCounterPipelineStageProgress,
			ValueKind:           types.LogOperationalValueStageOrdinal,
			Numerator:           numerator,
			Denominator:         denominator,
			NumeratorMeaning:    "one_based_pipeline_stage_position",
			DenominatorMeaning:  "total_configured_pipeline_stages",
			TransitionAuthority: types.LogOperationalTransitionEventLocalOnly,
			ExcludedMeanings: []string{
				"model_count",
				"llm_attempt_count",
				"stage_retry_count",
				"failure_count",
				"retry_budget",
				"exhaustion",
				"fallback_count",
				"repair_round_count",
			},
			LineStart:  lineNumber,
			LineEnd:    lineNumber,
			RawExcerpt: line,
		}, true
	}
	return types.LogOperationalSemantic{}, false
}

func decodeDispatchAttemptOperationalSemantic(line string, lineNumber int) (types.LogOperationalSemantic, bool) {
	match := logOperationalAttemptRE.FindStringSubmatch(line)
	if len(match) != 4 {
		return types.LogOperationalSemantic{}, false
	}
	numerator, errN := strconv.Atoi(match[2])
	denominator, errD := strconv.Atoi(match[3])
	if errN != nil || errD != nil || denominator <= 0 || numerator > denominator {
		return types.LogOperationalSemantic{}, false
	}
	return types.LogOperationalSemantic{
		Protocol:            "codrax_orchestrator_attempt_v1",
		Producer:            "orchestrator",
		EventKind:           types.LogOperationalEventDispatchAttemptFailed,
		Subject:             match[1],
		CounterDomain:       types.LogOperationalCounterDispatchAttempt,
		ValueKind:           types.LogOperationalValueAttemptOrdinal,
		Numerator:           numerator,
		Denominator:         denominator,
		NumeratorMeaning:    "current_dispatch_attempt_ordinal",
		DenominatorMeaning:  "maximum_dispatch_attempts",
		TransitionAuthority: types.LogOperationalTransitionEventLocalOnly,
		ExcludedMeanings: []string{
			"pipeline_stage_progress",
			"retry_count",
			"model_count",
			"identical_error_streak",
			"local_fallback_budget",
			"repair_round_cap",
		},
		LineStart:  lineNumber,
		LineEnd:    lineNumber,
		RawExcerpt: line,
	}, true
}

func logOperationalStagePhrases() []logOperationalStagePhrase {
	stageKeys := []string{
		"analyze", "explore", "evidence", "validate", "reconcile", "extract", "finalize",
		"plan", "apply", "verify", "operation", "data",
	}
	lifecycles := []struct {
		name  string
		state stagePhraseState
	}{
		{name: "running", state: stagePhraseRunning},
		{name: "done", state: stagePhraseDone},
		{name: "pending", state: stagePhrasePending},
		{name: "failed", state: stagePhraseFailed},
		{name: "retry", state: stagePhraseRetry},
	}
	var out []logOperationalStagePhrase
	for _, stage := range stageKeys {
		for _, lifecycle := range lifecycles {
			for _, lang := range []string{"zh", "en"} {
				phrase := strings.TrimSpace(stagePhrase(stage, lang, lifecycle.state))
				if phrase == "" {
					continue
				}
				out = append(out, logOperationalStagePhrase{
					stage: stage, lifecycle: lifecycle.name, phrase: phrase,
				})
			}
		}
	}
	return out
}

// RenderLogOperationalSemanticsForPrompt gives model agents the exact counter
// ownership before they interpret adjacent free-form log text. It is evidence
// context, not an answer or a conclusion.
func RenderLogOperationalSemanticsForPrompt(rows []types.LogOperationalSemantic) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("System-decoded operational protocol fields (authoritative for these exact fields):\n")
	b.WriteString("Different counter_domain values are separate namespaces. Do not add, compare, or connect them without an explicit typed transition witness. A current-source constant whose domain is absent below is mechanism context only; this attached log does not prove that gate was traversed.\n")
	b.WriteString("Lifecycle and value_kind are independent fields: lifecycle=retry describes this event's recoverable state; it does not turn value_kind=stage_ordinal into a retry, attempt, failure, budget, or exhaustion count.\n")
	if types.ResolveLogOperationalRelationAuthority(rows) == types.LogOperationalRelationObservedLineOrderOnly {
		b.WriteString("relation_authority=observed_log_line_order_only cross_event_transition=unproven typed_transition_witness=absent. The rows prove that each event was observed in this order; adjacency does not prove that one event drove another, reset or advanced another counter, re-entered a stage, or returned the pipeline to its start. A causal transition claim requires a typed transition witness, or a current-source call path joined to the exact runtime event identity.\n")
	}
	for i, row := range rows {
		fmt.Fprintf(&b, "- row=%d log_line=%d producer=%s event_kind=%s", i+1, row.LineStart, row.Producer, row.EventKind)
		if row.StageKey != "" {
			fmt.Fprintf(&b, " stage=%s lifecycle=%s", row.StageKey, row.Lifecycle)
		}
		if row.Subject != "" && row.StageKey == "" {
			fmt.Fprintf(&b, " subject=%s", row.Subject)
		}
		if row.CounterDomain != "" {
			fmt.Fprintf(&b, " counter_domain=%s value=%d/%d", row.CounterDomain, row.Numerator, row.Denominator)
		}
		if row.ValueKind != "" {
			fmt.Fprintf(&b, " value_kind=%s numerator_meaning=%s denominator_meaning=%s",
				row.ValueKind, row.NumeratorMeaning, row.DenominatorMeaning)
		}
		fmt.Fprintf(&b, " transition_authority=%s\n", row.TransitionAuthority)
		if len(row.ExcludedMeanings) > 0 {
			fmt.Fprintf(&b, "  does_not_mean=%s\n", strings.Join(row.ExcludedMeanings, ","))
		}
		fmt.Fprintf(&b, "  observed_protocol_line=%s\n", row.RawExcerpt)
	}
	return strings.TrimRight(b.String(), "\n")
}
