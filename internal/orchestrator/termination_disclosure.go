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
	// 件1 (2026-07-13): the disclosure speaks the firing arm's own truth.
	// The follow-up arm never measures a ratio — telling the user "the
	// verified-evidence ratio is below the floor" there was a false answer
	// statement (承诺面 discipline). Empty/legacy arm keeps the ratio
	// wording byte-stable.
	if tp.FloorArm == types.TerminationFloorArmFollowupCoverage {
		if narrowRuntimeFactCaveatScope(o) {
			return ""
		}
		if isChineseLang(o.busCtx.Language) {
			return "本回答生成前,系统建议的部分补充定位/钻取步骤未执行;结论以已收集的证据为准,未覆盖的部分请按未验证对待。"
		}
		return "Some suggested follow-up localization or drill-down steps were not executed before this answer was produced. The conclusions stand on the evidence actually gathered; treat uncovered parts as unverified."
	}
	if isChineseLang(o.busCtx.Language) {
		return "本回答生成时,已核实证据的比例低于配置的最低标准,且无法继续补充验证。结论可能不完整,请谨慎采信;可补充更具体的文件或方向后重试。"
	}
	return "This answer was produced with a verified-evidence ratio below the configured floor, and no further verification lane was available. Treat the conclusions with caution; re-running with more specific files or directions may improve grounding."
}

// completionCaveatLaneSystemCaveats renders the user-facing disclosures for
// the §29.60 softened completion-gate detections (件2, 2026-07-13): the
// detect→disclose contract is only real when the typed CompletionCaveats
// actually reach the answer surface. Exactly the three softened lanes render
// here — other lanes (form/convergence) keep their existing surfaces. Text
// is user-vocabulary only; the typed lane, not prose, is the routing signal.
func completionCaveatLaneSystemCaveats(o *Orchestrator) []string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	if closure == nil {
		return nil
	}
	zh := isChineseLang(o.busCtx.Language)
	var out []string
	for _, caveat := range closure.CompletionCaveats() {
		switch caveat.Lane {
		case types.DowngradeLaneWakeupChainDrilldown:
			if narrowRuntimeFactCaveatScope(o) {
				continue
			}
			if zh {
				out = append(out, "trace 中部分线程的睡眠等待未定位到上游唤醒者;相关结论基于已收集的证据,未定位的唤醒来源请按未验证对待。")
			} else {
				out = append(out, "Some threads' sleep waits in the trace were not traced back to an upstream waker; related conclusions stand on the collected evidence — treat unresolved wakeup sources as unverified.")
			}
		case types.DowngradeLaneExactResolvedDefiningProof:
			if zh {
				out = append(out, "未找到直接命名目标定义位置的已核实证据;涉及精确定义位置的结论请按未验证对待。")
			} else {
				out = append(out, "No verified evidence directly names the target's defining location; treat conclusions about the exact definition site as unverified.")
			}
		case types.DowngradeLaneForcedReadCoverage:
			if zh {
				out = append(out, "部分建议阅读的文件在回答完成前未读取;关于这些文件的说法请按未验证对待。")
			} else {
				out = append(out, "Some files suggested for reading were not read before this answer was completed; treat claims about those files as unverified.")
			}
		}
	}
	return out
}

// narrowRuntimeFactCaveatScope keeps causal-investigation debt out of a
// bounded runtime fact answer. The decision reuses the same validated typed
// report-shape authority as answer materialization; raw request and model
// prose are never inspected. Other caveat lanes remain unaffected.
func narrowRuntimeFactCaveatScope(o *Orchestrator) bool {
	if o == nil || o.busCtx == nil {
		return false
	}
	var rm *types.RequestModel
	if o.busCtx.AnalysisIR != nil {
		rm = &o.busCtx.AnalysisIR.RequestModel
	} else if o.busCtx.Mutable != nil {
		rm = o.busCtx.Mutable.RequestModel()
	}
	if rm == nil {
		return false
	}
	decided, allowed := types.RuntimeTraceReportShapeAuthority(rm)
	return decided && !allowed
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
//
// CAVSTR (2026-07-10): every note is routed through the caveat replay
// register (idempotent by verbatim text) so a later FinalAnswer
// re-render — first-draft attachment, auto-repair, recovery, FRCAP
// best-draft restore — replays the disclosures instead of silently
// dropping them.
func (o *Orchestrator) appendSystemCaveatsToAnswer(answer string) string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return answer
	}
	if note := inactiveScopeSystemCaveat(o.busCtx.Mutable.AnswerDocumentV2(), o.busCtx); note != "" {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, note)
	}
	if caveat := degradedTerminationSystemCaveat(o); caveat != "" {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, caveat)
	}
	for _, caveat := range completionCaveatLaneSystemCaveats(o) {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, caveat)
	}
	if caveat := preStageDegradationSystemCaveat(o); caveat != "" {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, caveat)
	}
	return answer
}
