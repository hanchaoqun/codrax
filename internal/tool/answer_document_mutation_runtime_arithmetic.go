package tool

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Arithmetic tokens are collected per sentence/answer row. A percentage may
// have more than one preceding duration in the same clause (for example
// "total 0.635ms, over a 144.557ms window, 0.44%"). The materializer elects
// one duration/window pair only when typed arithmetic makes that pair unique;
// nearest-token order is not authority.
var (
	runtimeTraceDurationTokenRE = regexp.MustCompile(`(?i)[0-9]+(?:\.[0-9]+)?\s*(?:ms|毫秒)`)
	runtimeTracePercentTokenRE  = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?\s*%`)
)

const (
	runtimeTraceArithmeticRelationCap       = 4
	runtimeTraceArithmeticWindowDedupeMS    = 0.001
	runtimeTraceArithmeticMinTolerancePctPt = 0.0005
)

type runtimeTraceArithmeticRelation struct {
	durationMS     float64
	claimedPercent float64
	percentToken   string
}

type runtimeTraceArithmeticRelationGroup struct {
	candidates []runtimeTraceArithmeticRelation
}

type runtimeTraceArithmeticNumeratorAuthority struct {
	durationMS   float64
	windowMS     float64
	completeness string
	principal    bool
}

type runtimeTraceArithmeticAuthorityResolver struct {
	numerators           []runtimeTraceArithmeticNumeratorAuthority
	singleResultFallback string
}

// materializeRuntimeTraceArithmeticRelationCaveat checks only model-authored
// visible text against producer-typed trace windows. It never edits a block,
// never rejects the answer, and never promotes a capped sample into a complete
// numerator. A mismatch, non-unique denominator, or non-complete enumeration
// is published as a document caveat.
func materializeRuntimeTraceArithmeticRelationCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil {
		return false
	}
	relationGroups := runtimeTraceModelArithmeticRelationGroups(doc)
	if len(relationGroups) == 0 {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return false
	}
	windows := runtimeTraceTypedWindowsMS(ledger.Records)
	windowMS, windowUnique := 0.0, false
	if len(windows) == 1 {
		windowMS, windowUnique = windows[0], true
	}
	authority := runtimeTraceArithmeticBuildAuthorityResolver(
		append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...),
	)
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var notes []string
	// Second net behind the relation-level dedupe: distinct relations that
	// render to the same note text would still read as a stutter.
	appendNote := func(note string) {
		for _, existing := range notes {
			if existing == note {
				return
			}
		}
		notes = append(notes, note)
	}
	for _, group := range relationGroups {
		if len(notes) >= runtimeTraceArithmeticRelationCap {
			break
		}
		if note, publish := runtimeTraceArithmeticRelationGroupNote(
			group, windows, windowMS, windowUnique, authority, zh,
		); publish {
			appendNote(note)
		}
	}
	if len(notes) == 0 {
		return false
	}
	prefix := "Arithmetic relation check: "
	separator := "; "
	if zh {
		prefix = "数值关系复算："
		separator = "；"
	}
	caveat := prefix + strings.Join(notes, separator)
	for _, existing := range doc.Caveats {
		if strings.TrimSpace(existing) == caveat {
			return false
		}
	}
	doc.Caveats = append(doc.Caveats, caveat)
	return true
}

func runtimeTraceArithmeticRelationGroupNote(
	group runtimeTraceArithmeticRelationGroup,
	windows []float64,
	windowMS float64,
	windowUnique bool,
	authority runtimeTraceArithmeticAuthorityResolver,
	zh bool,
) (string, bool) {
	if len(group.candidates) == 0 {
		return "", false
	}
	type consistentPair struct {
		relation runtimeTraceArithmeticRelation
		windowMS float64
	}
	var consistent []consistentPair
	for _, relation := range group.candidates {
		tolerance := runtimeTraceArithmeticPercentTolerance(relation.percentToken)
		for _, candidateWindowMS := range windows {
			computed := relation.durationMS / candidateWindowMS * 100
			if math.Abs(computed-relation.claimedPercent) <= tolerance {
				consistent = append(consistent, consistentPair{
					relation: relation,
					windowMS: candidateWindowMS,
				})
			}
		}
	}
	if len(consistent) == 1 {
		pair := consistent[0]
		completeness := authority.completenessFor(pair.relation.durationMS, pair.windowMS)
		if completeness == "complete" {
			return "", false
		}
		if windowUnique {
			computed := pair.relation.durationMS / pair.windowMS * 100
			return runtimeTraceArithmeticCheckedText(
				pair.relation,
				pair.windowMS,
				computed,
				runtimeTraceArithmeticPercentTolerance(pair.relation.percentToken),
				completeness,
				false,
				zh,
			), true
		}
		return runtimeTraceArithmeticElectedPairText(
			pair.relation,
			pair.windowMS,
			len(group.candidates),
			len(windows),
			completeness,
			zh,
		), true
	}
	if len(consistent) > 1 {
		return runtimeTraceArithmeticUnverifiedText(group.candidates[0], "unknown", zh), true
	}
	if len(group.candidates) > 1 {
		return runtimeTraceArithmeticAmbiguousPairText(
			group.candidates[0],
			len(group.candidates),
			len(windows),
			"unknown",
			zh,
		), true
	}

	relation := group.candidates[0]
	switch {
	case !windowUnique && len(windows) < 2:
		return runtimeTraceArithmeticUnverifiedText(relation, "unknown", zh), true
	case !windowUnique:
		// ARITH-DENOM (2026-07-24, NW-WIN-TYPED 拆件): with one
		// syntactically possible numerator and a multi-window census, the
		// denominator is disambiguated per relation by arithmetic
		// consistency. Multiple syntactic numerators are handled above and
		// may never be collapsed by token proximity.
		tolerance := runtimeTraceArithmeticPercentTolerance(relation.percentToken)
		closest, closestDiff := 0.0, math.MaxFloat64
		for _, w := range windows {
			computed := relation.durationMS / w * 100
			diff := math.Abs(computed - relation.claimedPercent)
			if diff < closestDiff {
				closest, closestDiff = w, diff
			}
		}
		completeness := authority.completenessFor(relation.durationMS, closest)
		return runtimeTraceArithmeticNoConsistentWindowText(
			relation, closest, len(windows), tolerance, completeness, zh,
		), true
	default:
		computed := relation.durationMS / windowMS * 100
		tolerance := runtimeTraceArithmeticPercentTolerance(relation.percentToken)
		completeness := authority.completenessFor(relation.durationMS, windowMS)
		return runtimeTraceArithmeticCheckedText(
			relation,
			windowMS,
			computed,
			tolerance,
			completeness,
			true,
			zh,
		), true
	}
}

func runtimeTraceModelArithmeticRelationGroups(doc *types.AnswerDocumentV2) []runtimeTraceArithmeticRelationGroup {
	var out []runtimeTraceArithmeticRelationGroup
	for _, block := range doc.Blocks {
		if RuntimeTraceSystemBlock(block) {
			continue
		}
		surface := types.AnswerBlockVisibleSurface(block)
		for _, percentMatch := range runtimeTracePercentTokenRE.FindAllStringIndex(surface, -1) {
			if len(percentMatch) != 2 {
				continue
			}
			percentToken := strings.TrimSpace(strings.TrimSuffix(
				surface[percentMatch[0]:percentMatch[1]],
				"%",
			))
			claimedPercent, percentErr := strconv.ParseFloat(percentToken, 64)
			if percentErr != nil || claimedPercent < 0 {
				continue
			}
			// A duration written immediately after the percentage in brackets is
			// an explicit local value binding, for example "73.4% (84.358ms)".
			// Prefer that one syntactic relation over every duration that happens
			// to precede the percentage in the clause. Otherwise a window total in
			// "114.940ms, accounting for 73.4% (84.358ms)" can be misreported as
			// the numerator. This is syntax-only and remains a soft advisory; it
			// does not inspect metric names, the request, or case-specific values.
			if relation, ok := runtimeTraceArithmeticPostpositiveDuration(
				surface, percentMatch[1], claimedPercent, percentToken,
			); ok {
				out = append(out, runtimeTraceArithmeticRelationGroup{
					candidates: []runtimeTraceArithmeticRelation{relation},
				})
				if len(out) >= runtimeTraceArithmeticRelationCap {
					return out
				}
				continue
			}
			clauseStart := runtimeTraceArithmeticClauseStart(surface, percentMatch[0])
			clause := surface[clauseStart:percentMatch[0]]
			group := runtimeTraceArithmeticRelationGroup{}
			for _, durationMatch := range runtimeTraceDurationTokenRE.FindAllStringIndex(clause, -1) {
				if len(durationMatch) != 2 {
					continue
				}
				durationEnd := clauseStart + durationMatch[1]
				bridge := surface[durationEnd:percentMatch[0]]
				if utf8.RuneCountInString(bridge) > 96 ||
					!runtimeTraceArithmeticBridgeBindsSameMetric(bridge) {
					continue
				}
				durationToken := strings.TrimSpace(clause[durationMatch[0]:durationMatch[1]])
				lowerDurationToken := strings.ToLower(durationToken)
				if strings.HasSuffix(lowerDurationToken, "ms") {
					durationToken = strings.TrimSpace(durationToken[:len(durationToken)-2])
				} else {
					durationToken = strings.TrimSpace(strings.TrimSuffix(durationToken, "毫秒"))
				}
				durationMS, durationErr := strconv.ParseFloat(durationToken, 64)
				if durationErr != nil || durationMS < 0 {
					continue
				}
				duplicate := false
				for _, existing := range group.candidates {
					if existing.durationMS == durationMS {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				group.candidates = append(group.candidates, runtimeTraceArithmeticRelation{
					durationMS:     durationMS,
					claimedPercent: claimedPercent,
					percentToken:   percentToken,
				})
			}
			if len(group.candidates) == 0 {
				continue
			}
			out = append(out, group)
			if len(out) >= runtimeTraceArithmeticRelationCap {
				return out
			}
		}
	}
	return out
}

// runtimeTraceArithmeticPostpositiveDuration recognizes only the narrow,
// language-independent shape "% (<duration>)" (including full-width and
// square opening brackets). Requiring an opening delimiter prevents a loose
// later duration, another metric, or the next sentence from being joined.
func runtimeTraceArithmeticPostpositiveDuration(
	surface string,
	percentEnd int,
	claimedPercent float64,
	percentToken string,
) (runtimeTraceArithmeticRelation, bool) {
	if percentEnd < 0 || percentEnd > len(surface) {
		return runtimeTraceArithmeticRelation{}, false
	}
	suffix := surface[percentEnd:]
	durationMatch := runtimeTraceDurationTokenRE.FindStringIndex(suffix)
	if len(durationMatch) != 2 {
		return runtimeTraceArithmeticRelation{}, false
	}
	bridge := suffix[:durationMatch[0]]
	if utf8.RuneCountInString(bridge) > 16 || !runtimeTraceArithmeticPostpositiveBridge(bridge) {
		return runtimeTraceArithmeticRelation{}, false
	}
	durationToken := strings.TrimSpace(suffix[durationMatch[0]:durationMatch[1]])
	lowerDurationToken := strings.ToLower(durationToken)
	if strings.HasSuffix(lowerDurationToken, "ms") {
		durationToken = strings.TrimSpace(durationToken[:len(durationToken)-2])
	} else {
		durationToken = strings.TrimSpace(strings.TrimSuffix(durationToken, "毫秒"))
	}
	durationMS, err := strconv.ParseFloat(durationToken, 64)
	if err != nil || durationMS < 0 {
		return runtimeTraceArithmeticRelation{}, false
	}
	return runtimeTraceArithmeticRelation{
		durationMS:     durationMS,
		claimedPercent: claimedPercent,
		percentToken:   percentToken,
	}, true
}

func runtimeTraceArithmeticPostpositiveBridge(bridge string) bool {
	hasOpeningDelimiter := false
	for _, r := range bridge {
		switch r {
		case ' ', '\t', '\r', '\n':
		case '(', '（', '[', '【':
			hasOpeningDelimiter = true
		default:
			return false
		}
	}
	return hasOpeningDelimiter
}

func runtimeTraceArithmeticClauseStart(surface string, percentStart int) int {
	if percentStart <= 0 || percentStart > len(surface) {
		return 0
	}
	before := surface[:percentStart]
	index := strings.LastIndexAny(before, "。!?\n！？")
	// An ASCII period is a sentence boundary only when it is not the decimal
	// point between two digits. Treating every '.' as a boundary turns
	// 0.817ms into 817ms and can manufacture a false arithmetic advisory.
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] != '.' {
			continue
		}
		decimalPoint := i > 0 && i+1 < len(before) &&
			before[i-1] >= '0' && before[i-1] <= '9' &&
			before[i+1] >= '0' && before[i+1] <= '9'
		if !decimalPoint && i > index {
			index = i
			break
		}
	}
	if index < 0 {
		return 0
	}
	_, size := utf8.DecodeRuneInString(before[index:])
	if size <= 0 {
		return index + 1
	}
	return index + size
}

// runtimeTraceArithmeticBridgeBindsSameMetric rejects the R20 false-join
// shape:
//
//	sleep=85.915ms，io_wait 占比 <0.5%
//
// Both values occur in one sentence, but the comma introduces a new metric
// subject. Only a suffix that is empty or begins with a closed relation
// connector can cross a comma/semicolon. This is deliberately a precision
// filter for a soft advisory: uncertain prose is skipped rather than turned
// into a user-visible arithmetic warning. It does not inspect the request,
// case identity, PID, or concrete values.
func runtimeTraceArithmeticBridgeBindsSameMetric(bridge string) bool {
	lastBoundaryEnd := -1
	for i, r := range bridge {
		switch r {
		case ',', '，', ';', '；':
			lastBoundaryEnd = i + len(string(r))
		}
	}
	if lastBoundaryEnd < 0 {
		return true
	}
	tail := strings.TrimSpace(bridge[lastBoundaryEnd:])
	tail = strings.TrimLeft(tail, " \t\r\n()（）[]【】<>＜＞=≈~～")
	if tail == "" {
		return true
	}
	lower := strings.ToLower(tail)
	for _, prefix := range []string{
		"占", "约占", "占比", "比例", "为", "约为", "相当于", "对应",
		"about ", "approximately ", "roughly ", "which ", "represent",
		"account", "equal", "or ", "around ",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// runtimeTraceTypedWindowsMS collects the deduplicated typed window-length
// census (ms) from deterministic-query selected_window notes.
func runtimeTraceTypedWindowsMS(records []types.ObservationRecord) []float64 {
	var windows []float64
	for _, record := range records {
		if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
		if !ok || end <= start {
			continue
		}
		durationMS := (end - start) * 1000
		duplicate := false
		for _, existing := range windows {
			if math.Abs(existing-durationMS) <= runtimeTraceArithmeticWindowDedupeMS {
				duplicate = true
				break
			}
		}
		if !duplicate {
			windows = append(windows, durationMS)
		}
	}
	return windows
}

// runtimeTraceArithmeticBuildAuthorityResolver binds an arithmetic numerator
// to the deterministic query row that published that value in the elected
// window. EnumerationAuthority is query/result scoped: it must never be ORed
// across the session and then attributed to every model-visible duration.
//
// target_window_states is finer still. It is a self-balancing wall-clock
// account whose state values remain complete even when another product in the
// same trace_query result (for example a capped event_search preview) is
// compacted. Its own total/window conservation therefore owns completeness.
func runtimeTraceArithmeticBuildAuthorityResolver(results []types.ToolResult) runtimeTraceArithmeticAuthorityResolver {
	resolver := runtimeTraceArithmeticAuthorityResolver{}
	inScopeResults := 0
	for _, result := range results {
		toolName := strings.TrimSpace(result.ToolName)
		inScope := toolName == "trace_query" ||
			(toolName == "read_file" && result.RuntimeArtifactRead != nil)
		if !inScope {
			continue
		}
		inScopeResults++
		resultStatus := runtimeTraceArithmeticNormalizeCompleteness(result.EnumerationAuthority)
		if inScopeResults == 1 {
			resolver.singleResultFallback = resultStatus
		} else {
			resolver.singleResultFallback = ""
		}
		for _, record := range result.Observations {
			if !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
				continue
			}
			start, end, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
			if !ok || end <= start {
				continue
			}
			windowMS := (end - start) * 1000
			if strings.TrimSpace(record.Predicate) == "target_window_states" {
				resolver.numerators = append(resolver.numerators,
					runtimeTraceArithmeticTargetStateAuthorities(record, windowMS)...,
				)
				continue
			}
			if strings.TrimSpace(record.Unit) != "ms" {
				continue
			}
			value, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64)
			if err != nil || value < 0 {
				continue
			}
			resolver.numerators = append(resolver.numerators, runtimeTraceArithmeticNumeratorAuthority{
				durationMS: value, windowMS: windowMS, completeness: resultStatus,
			})
		}
	}
	return resolver
}

func runtimeTraceArithmeticTargetStateAuthorities(
	record types.ObservationRecord,
	windowMS float64,
) []runtimeTraceArithmeticNumeratorAuthority {
	values := make([]float64, 0, 8)
	for _, key := range []string{
		types.TraceNoteKeyRunning,
		types.TraceNoteKeyRunnable,
		types.TraceNoteKeySleep,
		types.TraceNoteKeyDState,
		types.TraceNoteKeyIOWait,
		types.TraceNoteKeySleepIOWait,
		types.TraceNoteKeyTotal,
		types.TraceNoteKeyDeterministicRunning,
	} {
		if value, ok := runtimeTraceArithmeticTypedNoteFloat(record.RichNotes, key); ok {
			values = append(values, value)
		}
	}
	if value, err := strconv.ParseFloat(strings.TrimSpace(record.Value), 64); err == nil && value >= 0 {
		values = append(values, value)
	}
	totalMS, totalOK := runtimeTraceArithmeticTypedNoteFloat(record.RichNotes, types.TraceNoteKeyTotal)
	status := "unknown"
	if totalOK && windowMS > 0 {
		switch {
		case math.Abs(totalMS-windowMS) <= runtimeTraceArithmeticWindowDedupeMS:
			status = "complete"
		case totalMS >= 0 && totalMS < windowMS-runtimeTraceArithmeticWindowDedupeMS:
			status = "incomplete"
		}
	}
	out := make([]runtimeTraceArithmeticNumeratorAuthority, 0, len(values))
	for _, value := range values {
		out = append(out, runtimeTraceArithmeticNumeratorAuthority{
			durationMS: value, windowMS: windowMS, completeness: status, principal: true,
		})
	}
	return out
}

func runtimeTraceArithmeticTypedNoteFloat(notes []string, key string) (float64, bool) {
	prefix := strings.TrimSpace(key) + "="
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if !strings.HasPrefix(note, prefix) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(note, prefix)), 64)
		if err == nil && value >= 0 {
			return value, true
		}
	}
	return 0, false
}

func runtimeTraceArithmeticNormalizeCompleteness(authority *types.ToolEnumerationAuthority) string {
	if authority == nil {
		return "unknown"
	}
	if strings.TrimSpace(authority.Status) == "complete" {
		return "complete"
	}
	return "incomplete"
}

func (resolver runtimeTraceArithmeticAuthorityResolver) completenessFor(durationMS, windowMS float64) string {
	for _, principalOnly := range []bool{true, false} {
		statuses := map[string]bool{}
		for _, candidate := range resolver.numerators {
			if candidate.principal != principalOnly ||
				math.Abs(candidate.durationMS-durationMS) > runtimeTraceArithmeticWindowDedupeMS ||
				math.Abs(candidate.windowMS-windowMS) > runtimeTraceArithmeticWindowDedupeMS {
				continue
			}
			statuses[candidate.completeness] = true
		}
		if len(statuses) == 1 {
			for status := range statuses {
				return status
			}
		}
		if len(statuses) > 1 {
			return "unknown"
		}
	}
	if resolver.singleResultFallback != "" {
		return resolver.singleResultFallback
	}
	return "unknown"
}

// The tolerance is half of the claimed percentage's last displayed decimal
// unit (plus a small floor for floating-point noise). This one rule works for
// integer, one-decimal, and multi-decimal percentages without a hidden
// relation-specific threshold.
func runtimeTraceArithmeticPercentTolerance(percentToken string) float64 {
	decimals := 0
	if dot := strings.IndexByte(percentToken, '.'); dot >= 0 {
		decimals = len(percentToken) - dot - 1
	}
	tolerance := 0.5 * math.Pow10(-decimals)
	if tolerance < runtimeTraceArithmeticMinTolerancePctPt {
		return runtimeTraceArithmeticMinTolerancePctPt
	}
	return tolerance
}

func runtimeTraceArithmeticCheckedText(
	relation runtimeTraceArithmeticRelation,
	windowMS, computed, tolerance float64,
	completeness string,
	mismatch, zh bool,
) string {
	if zh {
		if mismatch {
			return fmt.Sprintf(
				"正文 %.3fms / %.3f%%，按 typed 窗长 %.3fms 重算为 %.3f%%，差值 %.3f 个百分点超过统一容差 %.3f；completeness=%s，正文保留未改写%s",
				relation.durationMS, relation.claimedPercent, windowMS, computed,
				math.Abs(computed-relation.claimedPercent), tolerance, completeness,
				runtimeTraceArithmeticCompletenessQualifier(completeness, true),
			)
		}
		return fmt.Sprintf(
			"正文 %.3fms / %.3f%% 的关系复算为 %.3f%%，但 completeness=%s，无法确认该分子是完整总量；正文保留未改写",
			relation.durationMS, relation.claimedPercent, computed, completeness,
		)
	}
	if mismatch {
		return fmt.Sprintf(
			"model text %.3fms / %.3f%% recomputes to %.3f%% over the typed %.3fms window; the %.3f percentage-point difference exceeds the unified %.3f tolerance; completeness=%s, and model prose was retained unchanged%s",
			relation.durationMS, relation.claimedPercent, computed, windowMS,
			math.Abs(computed-relation.claimedPercent), tolerance, completeness,
			runtimeTraceArithmeticCompletenessQualifier(completeness, false),
		)
	}
	return fmt.Sprintf(
		"model text %.3fms / %.3f%% recomputes to %.3f%%, but completeness=%s cannot establish that the numerator is a complete total; model prose was retained unchanged",
		relation.durationMS, relation.claimedPercent, computed, completeness,
	)
}

func runtimeTraceArithmeticElectedDenominatorText(
	relation runtimeTraceArithmeticRelation,
	windowMS float64,
	windowCount int,
	completeness string,
	zh bool,
) string {
	computed := relation.durationMS / windowMS * 100
	if zh {
		return fmt.Sprintf(
			"正文 %.3fms / %.3f%% 的关系复算为 %.3f%%(分母=%d 个 typed 窗长中唯一算术自洽的 %.3fms)，但 completeness=%s，无法确认该分子是完整总量；正文保留未改写",
			relation.durationMS, relation.claimedPercent, computed, windowCount, windowMS, completeness,
		)
	}
	return fmt.Sprintf(
		"model text %.3fms / %.3f%% recomputes to %.3f%% (denominator = the only arithmetically consistent typed window %.3fms of %d candidates), but completeness=%s cannot establish that the numerator is a complete total; model prose was retained unchanged",
		relation.durationMS, relation.claimedPercent, computed, windowMS, windowCount, completeness,
	)
}

func runtimeTraceArithmeticElectedPairText(
	relation runtimeTraceArithmeticRelation,
	windowMS float64,
	numeratorCount int,
	windowCount int,
	completeness string,
	zh bool,
) string {
	if numeratorCount <= 1 {
		return runtimeTraceArithmeticElectedDenominatorText(
			relation,
			windowMS,
			windowCount,
			completeness,
			zh,
		)
	}
	computed := relation.durationMS / windowMS * 100
	if zh {
		return fmt.Sprintf(
			"正文 %.3fms / %.3f%% 的关系复算为 %.3f%%(分子=%d 个同句 duration、分母=%d 个 typed 窗长中唯一算术自洽的一对；分母 %.3fms)，但 completeness=%s，无法确认该分子是完整总量；正文保留未改写",
			relation.durationMS, relation.claimedPercent, computed,
			numeratorCount, windowCount, windowMS, completeness,
		)
	}
	return fmt.Sprintf(
		"model text %.3fms / %.3f%% recomputes to %.3f%% (the unique arithmetically consistent pair across %d same-clause durations and %d typed windows; denominator %.3fms), but completeness=%s cannot establish that the numerator is a complete total; model prose was retained unchanged",
		relation.durationMS, relation.claimedPercent, computed,
		numeratorCount, windowCount, windowMS, completeness,
	)
}

func runtimeTraceArithmeticAmbiguousPairText(
	relation runtimeTraceArithmeticRelation,
	numeratorCount int,
	windowCount int,
	completeness string,
	zh bool,
) string {
	if zh {
		return fmt.Sprintf(
			"正文 %.3f%% 前有 %d 个可绑定 duration，且 %d 个 typed 窗长未能选出唯一算术自洽的分子/分母对，关系未复算；completeness=%s，正文保留未改写",
			relation.claimedPercent, numeratorCount, windowCount, completeness,
		)
	}
	return fmt.Sprintf(
		"model text %.3f%% has %d bindable same-clause durations, and %d typed windows do not identify one unique arithmetically consistent numerator/denominator pair; the relation was not recomputed; completeness=%s and model prose was retained unchanged",
		relation.claimedPercent, numeratorCount, windowCount, completeness,
	)
}

func runtimeTraceArithmeticNoConsistentWindowText(
	relation runtimeTraceArithmeticRelation,
	closestWindowMS float64,
	windowCount int,
	tolerance float64,
	completeness string,
	zh bool,
) string {
	computed := relation.durationMS / closestWindowMS * 100
	if zh {
		return fmt.Sprintf(
			"正文 %.3fms / %.3f%% 与全部 %d 个 typed 窗长均不自洽；最接近的窗长 %.3fms 重算为 %.3f%%，差值 %.3f 个百分点超过统一容差 %.3f；completeness=%s，正文保留未改写%s",
			relation.durationMS, relation.claimedPercent, windowCount, closestWindowMS, computed,
			math.Abs(computed-relation.claimedPercent), tolerance, completeness,
			runtimeTraceArithmeticCompletenessQualifier(completeness, true),
		)
	}
	return fmt.Sprintf(
		"model text %.3fms / %.3f%% is arithmetically inconsistent with every one of the %d typed windows; the closest window %.3fms recomputes to %.3f%%, a %.3f percentage-point difference beyond the unified %.3f tolerance; completeness=%s, and model prose was retained unchanged%s",
		relation.durationMS, relation.claimedPercent, windowCount, closestWindowMS, computed,
		math.Abs(computed-relation.claimedPercent), tolerance, completeness,
		runtimeTraceArithmeticCompletenessQualifier(completeness, false),
	)
}

func runtimeTraceArithmeticUnverifiedText(relation runtimeTraceArithmeticRelation, completeness string, zh bool) string {
	if zh {
		return fmt.Sprintf(
			"正文 %.3fms / %.3f%% 的 typed 窗长无法唯一定位，关系未复算；completeness=%s，正文保留未改写",
			relation.durationMS, relation.claimedPercent, completeness,
		)
	}
	return fmt.Sprintf(
		"the typed window for model text %.3fms / %.3f%% could not be uniquely located, so the relation was not recomputed; completeness=%s and model prose was retained unchanged",
		relation.durationMS, relation.claimedPercent, completeness,
	)
}

func runtimeTraceArithmeticCompletenessQualifier(completeness string, zh bool) string {
	switch completeness {
	case "incomplete":
		if zh {
			return "；该分子若来自 capped/paged 子集，只能作为样本或下界"
		}
		return "; a numerator from capped/paged rows is only a sample or lower bound"
	case "unknown":
		if zh {
			return "；无法确认该分子是完整总量"
		}
		return "; the numerator cannot be confirmed as a complete total"
	default:
		return ""
	}
}
