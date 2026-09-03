package orchestrator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// proseSelectedTypedReconciliationFindings implements the S4-3 ruling:
// model prose is read only as a noisy DISPLAY selector. It never supplies a
// subject, value, relation or verdict. Every emitted byte of fact content is
// rendered from tool.RuntimeTraceReconciliationRows, whose E# comes from the
// visible causal-projection evidence index.
func proseSelectedTypedReconciliationFindings(doc *types.AnswerDocumentV2, bus *types.BusContext) []proseScalarBindingFinding {
	if doc == nil || bus == nil {
		return nil
	}
	prose := collectModelProseUnits(doc)
	if len(prose) == 0 {
		return nil
	}
	rows := tool.RuntimeTraceReconciliationRows(bus)
	if len(rows) == 0 {
		return nil
	}
	var out []proseScalarBindingFinding
	seen := map[string]bool{}
	add := func(f proseScalarBindingFinding) {
		if strings.TrimSpace(f.entryZH) == "" || seen[f.entryZH] {
			return
		}
		seen[f.entryZH] = true
		out = append(out, f)
	}
	for _, row := range rows {
		switch row.Kind {
		case tool.RuntimeTraceReconciliationTargetState:
			if proseSelectsTargetStateReconciliation(prose, row) {
				add(renderTargetStateReconciliation(row))
			}
		case tool.RuntimeTraceReconciliationRankOne:
			if proseSelectsHeadlineReconciliation(prose) {
				add(renderRankOneReconciliation(row))
			}
			if proseSelectsDirectionReconciliation(prose, row) {
				add(renderDirectionReconciliation(row))
			}
		}
	}
	return out
}

func proseSelectsTargetStateReconciliation(prose []proseTextUnit, row tool.RuntimeTraceReconciliationRow) bool {
	// Exact three-decimal tokens are a narrow presence selector. Zero values
	// stay out because 0.000 is ubiquitous and would select unrelated rows.
	values := []float64{row.WindowMS, row.RunningMS, row.RunnableMS, row.SleepMS, row.DStateMS, row.IOWaitMS, row.TotalMS}
	for _, unit := range prose {
		for _, value := range values {
			if value > 0 && proseContainsExactThreeDecimal(unit.text, value) {
				return true
			}
		}
		// Equation shape is intentionally only a noisy selector. It does not
		// bind operands to a thread or decide whether the equation is right.
		if proseFactEquationRE.MatchString(unit.text) {
			return true
		}
	}
	return false
}

func proseContainsExactThreeDecimal(text string, value float64) bool {
	face := regexp.QuoteMeta(fmt.Sprintf("%.3f", value))
	re := regexp.MustCompile(`(?:^|[^0-9.])` + face + `(?:$|[^0-9.])`)
	return re.MatchString(text)
}

func proseSelectsHeadlineReconciliation(prose []proseTextUnit) bool {
	for _, unit := range prose {
		for _, span := range proseSentenceSpans(unit.text) {
			if proseHeadlineSentenceAnchored(unit.text[span[0]:span[1]]) {
				return true
			}
		}
	}
	return false
}

func proseSelectsDirectionReconciliation(prose []proseTextUnit, row tool.RuntimeTraceReconciliationRow) bool {
	token := strings.TrimSpace(row.FixDirection)
	wordZH, okZH := tracefence.FixDirectionWord(token, true)
	wordEN, okEN := tracefence.FixDirectionWord(token, false)
	if !okZH || !okEN {
		return false
	}
	// A direction already named anywhere needs no duplicate reference row.
	for _, unit := range prose {
		lower := strings.ToLower(unit.text)
		if strings.Contains(unit.text, wordZH) || strings.Contains(lower, strings.ToLower(wordEN)) ||
			strings.Contains(lower, strings.ToLower(token)) ||
			strings.Contains(lower, strings.ReplaceAll(strings.ToLower(token), "_", " ")) {
			return false
		}
	}
	// An enumeration-shaped repair section may select the typed direction
	// fact. This shape never enters a retry, verdict, or answer-body mutation.
	for _, unit := range prose {
		headed := false
		for _, head := range proseHeadlineDirectionHeadWords {
			if strings.Contains(unit.text, head) {
				headed = true
				break
			}
		}
		if headed && proseHeadlineEnumerationLineCount(unit.text) >= 2 {
			return true
		}
	}
	return false
}

func renderTargetStateReconciliation(row tool.RuntimeTraceReconciliationRow) proseScalarBindingFinding {
	artifactZH, artifactEN := reconciliationArtifactPrefix(row.ArtifactLabel)
	// §40.49 合流复核收编 (G-target-state #1): the row prints the five
	// DISJOINT lanes, so its D term is the exclusive non-IO lane and is
	// labelled with the single-source customer-face word — byte-equal with
	// the body wall-clock partition / wait-coverage / fact-juxtaposition
	// faces. The bare word "D-state" is reserved for the published fold,
	// which the body four-state line prints as "D-state …(其中 IO等待 …)";
	// spelling this lane bare put two calibers under one word on one
	// customer page (D-state 4.039 here vs D-state 5.379 in the body).
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("对账参考: %s%s 全窗状态分区 running %.3fms + runnable %.3fms + sleep %.3fms + %s %.3fms + io_wait %.3fms = %.3fms(分析窗 %.3fms) [%s]",
			artifactZH, row.Subject, row.RunningMS, row.RunnableMS, row.SleepMS, tool.TraceStateNonIODStateWord(true), row.DStateMS, row.IOWaitMS, row.TotalMS, row.WindowMS, row.EvidenceTag),
		entry: fmt.Sprintf("Reconciliation reference: %s%s full-window state partition: running %.3fms + runnable %.3fms + sleep %.3fms + %s %.3fms + io_wait %.3fms = %.3fms (analysis window %.3fms) [%s]",
			artifactEN, row.Subject, row.RunningMS, row.RunnableMS, row.SleepMS, tool.TraceStateNonIODStateWord(false), row.DStateMS, row.IOWaitMS, row.TotalMS, row.WindowMS, row.EvidenceTag),
	}
}

func renderRankOneReconciliation(row tool.RuntimeTraceReconciliationRow) proseScalarBindingFinding {
	artifactZH, artifactEN := reconciliationArtifactPrefix(row.ArtifactLabel)
	causeZH := tool.TraceRootCauseTypeZHLabel(row.CauseToken)
	if strings.TrimSpace(causeZH) == "" {
		causeZH = strings.TrimSpace(row.CauseToken)
	}
	causeEN := strings.ReplaceAll(strings.TrimSpace(row.CauseToken), "_", " ")
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("同尺并置: %s根因排序#1 %s / %s,已发布有效归因 %.3fms [%s]",
			artifactZH, row.Subject, causeZH, row.EffectiveMS, row.EvidenceTag),
		entry: fmt.Sprintf("Like-for-like reference: %sroot-cause rank #1 %s / %s, published attribution %.3fms [%s]",
			artifactEN, row.Subject, causeEN, row.EffectiveMS, row.EvidenceTag),
	}
}

func renderDirectionReconciliation(row tool.RuntimeTraceReconciliationRow) proseScalarBindingFinding {
	wordZH, _ := tracefence.FixDirectionWord(row.FixDirection, true)
	wordEN, _ := tracefence.FixDirectionWord(row.FixDirection, false)
	artifactZH, artifactEN := reconciliationArtifactPrefix(row.ArtifactLabel)
	return proseScalarBindingFinding{
		entryZH: fmt.Sprintf("同尺并置: %s根因排序#1 的修向=%s,该席有效归因 %.3fms [%s]",
			artifactZH, wordZH, row.EffectiveMS, row.EvidenceTag),
		entry: fmt.Sprintf("Like-for-like reference: %sroot-cause rank #1 fix direction=%s, seat attribution %.3fms [%s]",
			artifactEN, wordEN, row.EffectiveMS, row.EvidenceTag),
	}
}

func reconciliationArtifactPrefix(raw string) (zh, en string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	// A label may be a Windows or POSIX path. Only the final component is
	// needed to disambiguate per-artifact E# namespaces.
	if i := strings.LastIndexAny(raw, `/\\`); i >= 0 && i+1 < len(raw) {
		raw = raw[i+1:]
	}
	return "工件 " + raw + " · ", "artifact " + raw + " · "
}
