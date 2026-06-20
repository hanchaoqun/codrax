package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Degraded-termination disclosure: when the read run ships an answer
// after the grounding floor failed without a remediation lane (budget
// exhausted, forced finalize on a blocked DAG, or a hard evidence
// stall), the user must see that the answer's evidence base is below
// the configured floor. The signal is the typed TerminationProfile —
// no prose is inspected.

func degradedTerminationSystemCaveat(o *Orchestrator) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	tp := o.busCtx.Mutable.TerminationProfile()
	if tp == nil || !tp.FloorDegraded {
		return ""
	}
	if suppressDegradedTerminationDisclosure(o.busCtx.Mutable.AnswerDocumentV2()) {
		return ""
	}
	if isChineseLang(o.busCtx.Language) {
		return "本回答生成时,已核实证据的比例低于配置的最低标准,且无法继续补充验证。结论可能不完整,请谨慎采信;可补充更具体的文件或方向后重试。"
	}
	return "This answer was produced with a verified-evidence ratio below the configured floor, and no further verification lane was available. Treat the conclusions with caution; re-running with more specific files or directions may improve grounding."
}

func suppressDegradedTerminationDisclosure(doc *types.AnswerDocumentV2) bool {
	view := types.AnswerDocumentPrincipalEvidenceView(doc)
	if !view.HasGroundedPrincipalEvidence() {
		return false
	}
	if view.HasGroundedPrincipalEnumerationEvidence() {
		return true
	}
	if types.SourceLocalizationReviewHasSignal(doc.ReadSourceLocalization) {
		review := types.NormalizeSourceLocalizationReview(*doc.ReadSourceLocalization)
		return (review.Status == types.SourceLocalizationSupported ||
			len(review.OwnerSupportedPaths) > 0) &&
			len(review.OwnerMissingPaths) == 0 &&
			len(review.MissingPaths) == 0
	}
	anchors := types.NormalizeOwnerAnchorView(types.OwnerAnchorView{Items: doc.ReadOwnerAnchors}, 0)
	return anchors.HasStrong
}

// preStageDegradationSystemCaveat tells the user their attached
// artifact could not be processed, so the answer was produced without
// its anchors. Reads the typed degradation records only.
func preStageDegradationSystemCaveat(o *Orchestrator) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return ""
	}
	degraded := o.busCtx.Mutable.PreStageDegradations()
	if len(degraded) == 0 {
		return ""
	}
	var kinds []string
	for _, d := range degraded {
		switch d.Stage {
		case types.StageLogTriage:
			kinds = append(kinds, map[bool]string{true: "日志", false: "log"}[isChineseLang(o.busCtx.Language)])
		case types.StagePerfTriage:
			kinds = append(kinds, map[bool]string{true: "trace", false: "trace"}[isChineseLang(o.busCtx.Language)])
		default:
			kinds = append(kinds, string(d.Stage))
		}
	}
	joined := strings.Join(kinds, "、")
	if isChineseLang(o.busCtx.Language) {
		return "你附加的 " + joined + " 内容未能完成结构化解析,本回答未使用其中的运行时锚点;可检查附件格式或重试后再问。"
	}
	joined = strings.Join(kinds, ", ")
	return "The attached " + joined + " content could not be structurally parsed; this answer was produced without its runtime anchors. Check the attachment format or retry."
}

// appendSystemCaveatsToAnswer is the single chokepoint chaining every
// system-side answer caveat: inactive-scope disclosure plus the
// degraded-termination disclosure. All final-answer assembly paths in
// the read scheduler call this instead of the individual appenders so
// a newly added system caveat is visible on every path at once.
func (o *Orchestrator) appendSystemCaveatsToAnswer(answer string) string {
	answer = o.appendInactiveScopeSystemCaveatToAnswer(answer)
	if caveat := degradedTerminationSystemCaveat(o); caveat != "" {
		answer = AppendSystemCaveatString(answer, caveat, o.busCtx.Language)
	}
	if caveat := preStageDegradationSystemCaveat(o); caveat != "" {
		answer = AppendSystemCaveatString(answer, caveat, o.busCtx.Language)
	}
	return answer
}
