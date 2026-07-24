package tool

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A relation is deliberately narrow: a duration immediately followed by a
// percentage in the same sentence/answer row. This is an advisory cross-check
// of model-authored arithmetic, never an authority for rewriting that prose.
var runtimeTraceDurationPercentRelationRE = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(?:ms|毫秒)[^。.!?\n]{0,96}?([0-9]+(?:\.[0-9]+)?)\s*%`)

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

// materializeRuntimeTraceArithmeticRelationCaveat checks only model-authored
// visible text against producer-typed trace windows. It never edits a block,
// never rejects the answer, and never promotes a capped sample into a complete
// numerator. A mismatch, non-unique denominator, or non-complete enumeration
// is published as a document caveat.
func materializeRuntimeTraceArithmeticRelationCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil {
		return false
	}
	relations := runtimeTraceModelArithmeticRelations(doc)
	if len(relations) == 0 {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	if !ledger.HasDeterministicRuntimeQueryObservation() {
		return false
	}
	windowMS, windowUnique := runtimeTraceUniqueTypedWindowMS(ledger.Records)
	completeness := runtimeTraceArithmeticEnumerationCompleteness(
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
	for _, relation := range relations {
		if len(notes) >= runtimeTraceArithmeticRelationCap {
			break
		}
		switch {
		case !windowUnique:
			appendNote(runtimeTraceArithmeticUnverifiedText(relation, completeness, zh))
		default:
			computed := relation.durationMS / windowMS * 100
			tolerance := runtimeTraceArithmeticPercentTolerance(relation.percentToken)
			mismatch := math.Abs(computed-relation.claimedPercent) > tolerance
			if !mismatch && completeness == "complete" {
				continue
			}
			appendNote(runtimeTraceArithmeticCheckedText(
				relation, windowMS, computed, tolerance, completeness, mismatch, zh,
			))
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

func runtimeTraceModelArithmeticRelations(doc *types.AnswerDocumentV2) []runtimeTraceArithmeticRelation {
	var out []runtimeTraceArithmeticRelation
	for _, block := range doc.Blocks {
		if RuntimeTraceSystemBlock(block) {
			continue
		}
		surface := types.AnswerBlockVisibleSurface(block)
		for _, match := range runtimeTraceDurationPercentRelationRE.FindAllStringSubmatch(surface, -1) {
			if len(match) != 3 {
				continue
			}
			durationMS, durationErr := strconv.ParseFloat(match[1], 64)
			claimedPercent, percentErr := strconv.ParseFloat(match[2], 64)
			if durationErr != nil || percentErr != nil || durationMS < 0 || claimedPercent < 0 {
				continue
			}
			// One claim repeated in prose (possibly at different token
			// precision, e.g. 18.76% vs 18.760%) is one relation: parsed
			// values are the dedupe key, the first occurrence's token keeps
			// tolerance authority. Without this, repeated prose multiplies
			// byte-identical notes (no_window.txt 客户形).
			duplicate := false
			for _, existing := range out {
				if existing.durationMS == durationMS && existing.claimedPercent == claimedPercent {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			out = append(out, runtimeTraceArithmeticRelation{
				durationMS:     durationMS,
				claimedPercent: claimedPercent,
				percentToken:   match[2],
			})
			if len(out) >= runtimeTraceArithmeticRelationCap {
				return out
			}
		}
	}
	return out
}

func runtimeTraceUniqueTypedWindowMS(records []types.ObservationRecord) (float64, bool) {
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
	if len(windows) != 1 {
		return 0, false
	}
	return windows[0], true
}

func runtimeTraceArithmeticEnumerationCompleteness(results []types.ToolResult) string {
	seen := false
	for _, result := range results {
		toolName := strings.TrimSpace(result.ToolName)
		inScope := toolName == "trace_query" ||
			(toolName == "read_file" && result.RuntimeArtifactRead != nil)
		if !inScope || result.EnumerationAuthority == nil {
			continue
		}
		seen = true
		if strings.TrimSpace(result.EnumerationAuthority.Status) != "complete" {
			return "incomplete"
		}
	}
	if !seen {
		return "unknown"
	}
	return "complete"
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
