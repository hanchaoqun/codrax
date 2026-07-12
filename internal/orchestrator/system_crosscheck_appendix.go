package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// system_crosscheck_appendix.go — S3' ② (§29.47.1 user ruling, 2026-07-12,
// docs/design/real_trace_campaign_20260705.md; witnesses 2779 + 76278: two
// soft mechanical findings — three real kernel event names, and a
// near-correct truncated-token quote — each drew a whole-answer rewrite
// whose second draft came out WORSE than the first).
//
// 检测与执行分离 (§29.47.1 principle): mechanical checks may always LOOK,
// but only fatal criteria that hold no matter who is right may BLOCK; every
// other finding becomes information. The soft prose lanes therefore trigger
// ZERO repair rounds — the first strict-passing draft ships as written —
// and the system's mechanical findings render HERE instead: one independent
// deterministic block at the answer tail, in the system's own voice (the
// causal tree / detail table precedent: the system never writes the model's
// prose, but it may speak on its own surface).
//
// Wording duty: the lead-in is humble and truthful (mechanical cross-check,
// may contain false positives, the body and its evidence take precedence);
// items are user-readable and never name internal machinery (no check
// names, no pipeline vocabulary). Zero findings → the block does not render.
//
// P3-1 known edge (recorded 2026-07-12, no fix by ruling): when an operator
// PROMOTES the prose kinds via pipeline_contract_strict_kinds, the legacy
// latch/cap caveat lanes revive alongside this appendix, so the same
// finding may disclose twice in that opt-in configuration — accepted as an
// operator-chosen edge, not a default-path behavior.

// systemCrossCheckFindingCap bounds the rendered item list.
const systemCrossCheckFindingCap = 8

func systemCrossCheckAppendixTitle(lang string) string {
	if isChineseLang(lang) {
		return "系统校验附注"
	}
	return "System Cross-check Notes"
}

func systemCrossCheckLeadIn(lang string) string {
	if isChineseLang(lang) {
		return "以下为系统机械交叉核对结果，供参考，可能存在误报；以正文与证据为准。"
	}
	return "The following are the system's mechanical cross-check results, for reference only; false positives are possible — the answer body and its evidence take precedence."
}

// systemCrossCheckScalarFinding renders the numeric/identity re-derivation
// finding (the values the deterministic re-scan could not locate on any
// evidence surface of the shipped report).
func systemCrossCheckScalarFinding(lang string, labels []string) string {
	listed := labels
	extra := 0
	if len(listed) > systemCrossCheckFindingCap {
		extra = len(listed) - systemCrossCheckFindingCap
		listed = listed[:systemCrossCheckFindingCap]
	}
	if isChineseLang(lang) {
		msg := "正文中 " + strings.Join(listed, "、") + " 未能在本报告证据面复算或定位"
		if extra > 0 {
			msg += fmt.Sprintf("（另有 %d 项）", extra)
		}
		return msg
	}
	msg := "the body's " + strings.Join(listed, ", ") + " could not be re-derived or located on this report's evidence surfaces"
	if extra > 0 {
		msg += fmt.Sprintf(" (+%d more)", extra)
	}
	return msg
}

// collectSystemCrossCheckFindings re-runs the deterministic prose scans
// against the SHIPPED document (the one in MutableState — every accept,
// restore and best-draft path writes the shipping doc back there) and
// renders each finding user-readably.
func (o *Orchestrator) collectSystemCrossCheckFindings() []string {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return nil
	}
	mut := o.busCtx.Mutable
	doc := mut.AnswerDocumentV2()
	lang := o.busCtx.Language
	var out []string
	tokens, misbound := proseScalarResidualAppendixInputs(doc, o.busCtx, mut)
	if len(tokens) > 0 {
		out = append(out, systemCrossCheckScalarFinding(lang, tokens))
	}
	for i, f := range misbound {
		if i >= systemCrossCheckFindingCap {
			out = append(out, systemCrossCheckMoreLine(lang, len(misbound)-i))
			break
		}
		out = append(out, f.userReadable(lang))
	}
	findings := proseLexiconBoardResidualFindings(doc, o.busCtx, mut)
	for i, f := range findings {
		if i >= systemCrossCheckFindingCap {
			out = append(out, systemCrossCheckMoreLine(lang, len(findings)-i))
			break
		}
		out = append(out, f.userReadable(lang))
	}
	// CR-3 件① P6 (2026-07-12): the wall-clock conservation arms — prose
	// state durations vs the window and vs the published full-window state
	// partition. Information lane only (§29.42.4 全批软纪律): disclosure
	// here, never a violation, never a rewrite.
	wall := proseWallClockConservationFindings(doc, o.busCtx, mut)
	for i, f := range wall {
		if i >= systemCrossCheckFindingCap {
			out = append(out, systemCrossCheckMoreLine(lang, len(wall)-i))
			break
		}
		out = append(out, f.userReadable(lang))
	}
	return out
}

func systemCrossCheckMoreLine(lang string, n int) string {
	if isChineseLang(lang) {
		return fmt.Sprintf("另有 %d 项同类核对提示", n)
	}
	return fmt.Sprintf("%d more cross-check note(s) of the same family", n)
}

// attachSystemCrossCheckAppendix renders the S3' appendix: the system's
// mechanical findings against the shipped draft plus, when the strict-lane
// repair arbitration kept the first draft, its one-line note (③: the
// first-draft retention region never duplicates — the note replaces it)
// and the restored draft's residual STRICT concerns (P2-2, the P6
// injectResidualConcernsCaveat family rendered on this surface). Zero
// content → no attachment, byte-identical answer.
func (o *Orchestrator) attachSystemCrossCheckAppendix(out *agent.StageOutput, arbitrationNote string, residualStrict []types.Violation) {
	if o == nil || out == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	findings := o.collectSystemCrossCheckFindings()
	note := strings.TrimSpace(arbitrationNote)
	if len(findings) == 0 && note == "" && len(residualStrict) == 0 {
		return
	}
	lang := o.busCtx.Language
	var b strings.Builder
	if note != "" {
		b.WriteString(note)
		b.WriteString("\n\n")
	}
	if len(residualStrict) > 0 {
		for _, bullet := range MaterializeResidualConcernDetails(residualStrict, lang) {
			b.WriteString(strings.TrimSpace(bullet))
			b.WriteString("\n\n")
		}
	}
	if len(findings) > 0 {
		b.WriteString(systemCrossCheckLeadIn(lang))
		b.WriteString("\n")
		for _, f := range findings {
			b.WriteString("\n- ")
			b.WriteString(f)
		}
		b.WriteString("\n")
	}
	logging.Info("[orchestrator] system cross-check appendix: %d finding(s) rendered (advisory information lane — never a retry, never a body edit)", len(findings))
	o.appendAnswerDisplayAttachment(out, types.AnswerDisplayAttachment{
		Kind:   types.AnswerDisplayAttachmentMarkdown,
		Title:  systemCrossCheckAppendixTitle(lang),
		Body:   b.String(),
		Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck,
	})
}
