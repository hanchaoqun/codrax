package orchestrator

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

const finalizerNoVisibleOutputDegradeReason = "finalizer_no_visible_output_timeout"

func (o *Orchestrator) finalizerNoVisibleOutputFallback(err error) *agent.StageOutput {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	var nv *llm.StreamNoVisibleOutputTimeoutError
	if !errors.As(err, &nv) {
		return nil
	}
	if !o.hasReusableTurnBSlateForFinalize() && !o.hasReconcileEvidenceContext() {
		return nil
	}
	answer := o.renderFinalizerNoVisibleOutputFallbackAnswer(nv)
	if strings.TrimSpace(answer) == "" {
		return nil
	}
	o.busCtx.TaskState.LastError = ""
	return &agent.StageOutput{
		MissingPiece:     types.MissingNone,
		FinalAnswer:      answer,
		AnswerDegraded:   true,
		SkipAnswerChecks: true,
		DegradeReason:    finalizerNoVisibleOutputDegradeReason,
		Error:            err.Error(),
	}
}

func (o *Orchestrator) renderFinalizerNoVisibleOutputFallbackAnswer(err *llm.StreamNoVisibleOutputTimeoutError) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	lang := o.busCtx.Language
	zh := isChineseLang(lang)
	var b strings.Builder
	if zh {
		b.WriteString("## 降级答案\n\n")
		b.WriteString("最终成文模型的流式连接仍在活动，但在 ")
		b.WriteString(err.IdleFor.String())
		b.WriteString(" 内没有产生可用于用户答案或工具调用的输出。系统已停止继续等待，下面仅基于前序阶段已经收集和提炼的结构化信息给出保守摘要。\n\n")
	} else {
		b.WriteString("## Degraded Answer\n\n")
		b.WriteString("The final-answer model stream stayed active but produced no user-visible answer or tool-call output for ")
		b.WriteString(err.IdleFor.String())
		b.WriteString(". The system stopped waiting and is using only the structured evidence already collected by earlier stages.\n\n")
	}

	sections := 0
	if s := renderFallbackAnswerSymbols(o, zh); s != "" {
		b.WriteString(s)
		sections++
	}
	if s := renderFallbackAggregateFacts(o, zh); s != "" {
		b.WriteString(s)
		sections++
	}
	if s := renderFallbackEvidenceItems(o, zh); s != "" {
		b.WriteString(s)
		sections++
	}
	if s := renderFallbackHypothesisVerdicts(o, zh); s != "" {
		b.WriteString(s)
		sections++
	}
	if sections == 0 {
		if zh {
			b.WriteString("本轮没有足够的结构化证据可安全转写成结论。建议重试一次，或切换 provider/model 后再问。\n\n")
		} else {
			b.WriteString("There was not enough structured evidence to safely restate as a conclusion. Retry once, or switch provider/model and ask again.\n\n")
		}
	}
	if zh {
		b.WriteString("## 说明\n\n")
		b.WriteString("- 这是降级输出，未经过最终成文模型的完整结构化润色。\n")
		b.WriteString("- 上面的内容只来自已收集证据、聚合事实、答案候选或假设判定；没有补写新的源码或 trace 结论。\n")
		b.WriteString("- 如果需要更完整的叙述、图表或最终措辞，请重试一次，或切换 provider/model。\n")
	} else {
		b.WriteString("## Notes\n\n")
		b.WriteString("- This is a degraded output and did not complete the normal structured final-answer pass.\n")
		b.WriteString("- The bullets above only restate collected evidence, aggregate facts, answer candidates, or hypothesis verdicts; no new source or trace conclusions were invented.\n")
		b.WriteString("- Retry once, or switch provider/model, if you need a fuller narrative, diagram, or polished final wording.\n")
	}
	return strings.TrimSpace(b.String())
}

func renderFallbackAnswerSymbols(o *Orchestrator, zh bool) string {
	symbols := fallbackAnswerSymbols(o)
	if len(symbols) == 0 {
		return ""
	}
	var b strings.Builder
	if zh {
		b.WriteString("## 已提炼的答案候选\n\n")
	} else {
		b.WriteString("## Extracted Answer Candidates\n\n")
	}
	for i, sym := range symbols {
		if i >= 8 {
			break
		}
		name := strings.TrimSpace(sym.Name)
		if name == "" {
			continue
		}
		b.WriteString("- `")
		b.WriteString(name)
		b.WriteString("`")
		if loc := answerSymbolLocation(sym); loc != "" {
			b.WriteString(" (")
			b.WriteString(loc)
			b.WriteString(")")
		}
		if rationale := strings.TrimSpace(sym.Rationale); rationale != "" {
			b.WriteString(": ")
			b.WriteString(oneLine(rationale, 220))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func fallbackAnswerSymbols(o *Orchestrator) []types.AnswerSymbol {
	if o == nil || o.busCtx == nil {
		return nil
	}
	out := make([]types.AnswerSymbol, 0, len(o.busCtx.AnswerSymbols))
	seen := map[string]bool{}
	appendSymbol := func(sym types.AnswerSymbol) {
		key := strings.TrimSpace(sym.Name) + "\x00" + answerSymbolLocation(sym)
		if strings.TrimSpace(sym.Name) == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, sym)
	}
	for _, sym := range o.busCtx.AnswerSymbols {
		appendSymbol(sym)
	}
	if o.busCtx.Mutable != nil {
		if symbols, _ := o.busCtx.Mutable.EmittedAnswerSymbols(); len(symbols) > 0 {
			for _, sym := range symbols {
				appendSymbol(sym)
			}
		}
	}
	return out
}

func answerSymbolLocation(sym types.AnswerSymbol) string {
	file := strings.TrimSpace(sym.File)
	if file == "" {
		return ""
	}
	if sym.Line > 0 {
		return fmt.Sprintf("%s:%d", file, sym.Line)
	}
	return file
}

func renderFallbackAggregateFacts(o *Orchestrator, zh bool) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	facts := o.busCtx.Mutable.StableInvestigationAggregateFacts()
	if len(facts) == 0 {
		return ""
	}
	var b strings.Builder
	if zh {
		b.WriteString("## 已确认的聚合事实\n\n")
	} else {
		b.WriteString("## Accepted Aggregate Facts\n\n")
	}
	for i, fact := range facts {
		if i >= 8 {
			break
		}
		label := strings.TrimSpace(fact.Label)
		if label == "" {
			label = string(fact.Kind)
		}
		value := strings.TrimSpace(fact.Value)
		b.WriteString("- ")
		b.WriteString(oneLine(label, 140))
		if value != "" {
			b.WriteString(": ")
			b.WriteString(oneLine(value, 160))
			if unit := strings.TrimSpace(fact.Unit); unit != "" {
				b.WriteByte(' ')
				b.WriteString(unit)
			}
		}
		if len(fact.Members) > 0 {
			b.WriteString(" [")
			b.WriteString(strings.Join(limitStrings(fact.Members, 5, 80), ", "))
			if len(fact.Members) > 5 {
				b.WriteString(", ...")
			}
			b.WriteString("]")
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func renderFallbackEvidenceItems(o *Orchestrator, zh bool) string {
	items := fallbackEvidenceItems(o)
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	if zh {
		b.WriteString("## 关键证据摘录\n\n")
	} else {
		b.WriteString("## Key Evidence Excerpts\n\n")
	}
	for i, ev := range items {
		if i >= 8 {
			break
		}
		summary := strings.TrimSpace(ev.Summary)
		if summary == "" {
			summary = fallbackEvidenceLabel(ev)
		}
		if summary == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(oneLine(summary, 260))
		if loc := ev.DisplayLocation(true); loc != "" {
			b.WriteString(" (")
			b.WriteString(loc)
			b.WriteString(")")
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func fallbackEvidenceItems(o *Orchestrator) []types.EvidenceItem {
	if o == nil || o.busCtx == nil {
		return nil
	}
	var emitted []types.EvidenceItem
	if o.busCtx.Mutable != nil {
		emitted = o.busCtx.Mutable.EmittedEvidence()
	}
	items := types.ExactResolutionSurfaceEvidencePool(emitted, o.busCtx.EvidenceItems, o.busCtx.AnswerChains)
	if o.busCtx.Mutable != nil {
		if ta := o.busCtx.Mutable.TurnAArtifacts(); ta != nil {
			items = types.ExactResolutionSurfaceEvidencePool(items, ta.EvidenceItems, nil)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IsCitable() != items[j].IsCitable() {
			return items[i].IsCitable()
		}
		return items[i].SalienceLockedForScoring() && !items[j].SalienceLockedForScoring()
	})
	return items
}

func fallbackEvidenceLabel(ev types.EvidenceItem) string {
	parts := []string{}
	for _, s := range []string{ev.AnchorSymbol, ev.Subject, ev.Predicate, ev.Object} {
		s = strings.TrimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func renderFallbackHypothesisVerdicts(o *Orchestrator, zh bool) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	verdicts := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(verdicts) == 0 {
		return ""
	}
	var b strings.Builder
	if zh {
		b.WriteString("## 假设判定\n\n")
	} else {
		b.WriteString("## Hypothesis Verdicts\n\n")
	}
	for i, v := range verdicts {
		if i >= 8 {
			break
		}
		label := strings.TrimSpace(v.HypothesisID)
		if label == "" {
			label = "hypothesis"
		}
		b.WriteString("- `")
		b.WriteString(label)
		b.WriteString("`: ")
		b.WriteString(string(v.Status))
		if rationale := strings.TrimSpace(v.Rationale); rationale != "" {
			b.WriteString(" — ")
			b.WriteString(oneLine(rationale, 220))
		}
		if citation := strings.TrimSpace(v.Citation); citation != "" {
			b.WriteString(" (")
			b.WriteString(citation)
			b.WriteString(")")
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return b.String()
}

func limitStrings(in []string, n int, maxLen int) []string {
	if n <= 0 || len(in) == 0 {
		return nil
	}
	out := make([]string, 0, min(n, len(in)))
	for _, raw := range in {
		s := oneLine(raw, maxLen)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= n {
			break
		}
	}
	return out
}

func oneLine(s string, maxLen int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if maxLen > 0 {
		runes := []rune(s)
		if len(runes) > maxLen {
			return strings.TrimSpace(string(runes[:maxLen])) + "..."
		}
	}
	return s
}
