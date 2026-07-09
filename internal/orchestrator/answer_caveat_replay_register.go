package orchestrator

import (
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_caveat_replay_register.go — CAVSTR root fix (FRCAP+PSG-2H
// adversarial review P2, 2026-07-10; supersedes the CAVSTR docket).
//
// Problem shape (review REPRO): every string-channel caveat appended to
// StageOutput.FinalAnswer — the P6/FRCAP residual-concern disclosure,
// the three appendSystemCaveatsToAnswer notes, the user/soft contract
// caveat bullets, the PSG-2H residual disclosure — vanished whenever a
// later overwrite point re-rendered the answer from the structured
// document (flagship path: attachFirstDraftReference →
// appendAnswerDisplayAttachment → renderFinalAnswerWithLastMileSupplements,
// which is ON the P6-cap main line: strict review default-on +
// rejectedForRewrite true + first draft ≠ final). §29.14 TRUNC rescued
// only the doc-side 系统补充 blocks; the string channel had no keeper.
//
// Fix: an orchestrator-side pending-caveat register. Every string-
// channel append the read scheduler performs on a finalize answer ALSO
// registers its payload (ordered, key-deduplicated); the TRUNC last-mile
// re-render chokepoint replays the register after every re-render, so
// no FinalAnswer overwrite can kill a disclosure again. P7 channel
// separation is preserved absolutely: the register never writes into
// AnswerDocumentV2.Caveats — replay re-appends through the same
// system-channel string helpers the original append used.
//
// Idempotency: entries are keyed by kind+verbatim text. Registering the
// same payload twice appends once; replaying after N re-renders yields
// exactly one instance because each re-render starts from a fresh
// doc-only render that carries no string-channel content.
//
// Reset: per task at runReadSchedulerLoop entry (same boundary as the
// other per-task Mutable resets) so a multi-task run cannot drag task
// N's disclosures into task N+1.

// answerCaveatReplayEntryKind selects the replay form.
type answerCaveatReplayEntryKind int

const (
	// answerCaveatBullet replays via AppendSystemCaveatString — one
	// bullet under the shared "**补充说明：**" / "**Additional notes:**"
	// heading (created once, then appended under).
	answerCaveatBullet answerCaveatReplayEntryKind = iota
	// answerCaveatRawSection replays as a free-standing block appended
	// after a blank line (the enumeration-label verification
	// supplement's historical form).
	answerCaveatRawSection
)

type answerCaveatReplayEntry struct {
	kind answerCaveatReplayEntryKind
	text string
}

type answerCaveatReplayRegister struct {
	entries []answerCaveatReplayEntry
	seen    map[string]bool
}

func answerCaveatReplayKey(kind answerCaveatReplayEntryKind, text string) string {
	if kind == answerCaveatRawSection {
		return "raw\x00" + text
	}
	return "bullet\x00" + text
}

// resetAnswerCaveatReplayRegister clears the per-task register.
func (o *Orchestrator) resetAnswerCaveatReplayRegister() {
	if o == nil {
		return
	}
	o.answerCaveatReplay = nil
}

// registerAnswerCaveatEntry records one entry; reports whether it was
// newly registered (false = duplicate key, already appended once).
func (o *Orchestrator) registerAnswerCaveatEntry(kind answerCaveatReplayEntryKind, text string) bool {
	if o == nil {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if o.answerCaveatReplay == nil {
		o.answerCaveatReplay = &answerCaveatReplayRegister{seen: map[string]bool{}}
	}
	key := answerCaveatReplayKey(kind, text)
	if o.answerCaveatReplay.seen[key] {
		return false
	}
	o.answerCaveatReplay.seen[key] = true
	o.answerCaveatReplay.entries = append(o.answerCaveatReplay.entries, answerCaveatReplayEntry{kind: kind, text: text})
	return true
}

// appendRegisteredAnswerCaveatBullet is the register-aware form of
// AppendSystemCaveatString: first registration appends the bullet;
// a duplicate key returns the answer unchanged (the bullet is already
// in the string, or will be re-added by replay after a re-render).
func (o *Orchestrator) appendRegisteredAnswerCaveatBullet(answer, bullet string) string {
	if o == nil || strings.TrimSpace(bullet) == "" {
		return answer
	}
	if !o.registerAnswerCaveatEntry(answerCaveatBullet, bullet) {
		return answer
	}
	return AppendSystemCaveatString(answer, bullet, o.answerCaveatLanguage())
}

// appendRegisteredAnswerCaveatRawSection is the register-aware form of
// the historical raw-supplement append (TrimRight + blank line + block).
func (o *Orchestrator) appendRegisteredAnswerCaveatRawSection(answer, section string) string {
	if o == nil || strings.TrimSpace(section) == "" {
		return answer
	}
	if !o.registerAnswerCaveatEntry(answerCaveatRawSection, section) {
		return answer
	}
	return strings.TrimRight(answer, "\n") + "\n\n" + strings.TrimSpace(section) + "\n"
}

// replayRegisteredAnswerCaveats re-appends every registered entry in
// registration order. Called by the last-mile re-render chokepoint on a
// FRESH doc-only render (which carries no string-channel content), so
// the result carries each disclosure exactly once, in the order the
// scheduler originally appended it.
func (o *Orchestrator) replayRegisteredAnswerCaveats(answer string) string {
	if o == nil || o.answerCaveatReplay == nil || len(o.answerCaveatReplay.entries) == 0 {
		return answer
	}
	lang := o.answerCaveatLanguage()
	for _, e := range o.answerCaveatReplay.entries {
		switch e.kind {
		case answerCaveatRawSection:
			answer = strings.TrimRight(answer, "\n") + "\n\n" + e.text + "\n"
		default:
			answer = AppendSystemCaveatString(answer, e.text, lang)
		}
	}
	return answer
}

func (o *Orchestrator) answerCaveatLanguage() string {
	if o == nil || o.busCtx == nil {
		return ""
	}
	return o.busCtx.Language
}

// appendUserCaveatsTracked is the register-aware scheduler form of
// AppendUserCaveatsToAnswerForBus: byte-equivalent output on the direct
// path (per-bullet sequential appends produce the same heading+bullet
// bytes as the historical single call), plus registration so the
// bullets survive later re-renders.
func (o *Orchestrator) appendUserCaveatsTracked(answer string, violations []types.Violation) string {
	if o == nil || o.busCtx == nil {
		return AppendUserCaveatsToAnswerForBus(answer, violations, "", nil)
	}
	bullets := materializedUserCaveatBulletsForBus(violations, o.busCtx.Language, o.busCtx)
	for _, b := range bullets {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, b)
	}
	return answer
}

// appendSoftContractCaveatsTracked is the register-aware scheduler form
// of AppendSoftContractCaveatsToAnswerForBus (bullets first, then the
// raw enumeration-verification supplement — the historical order).
func (o *Orchestrator) appendSoftContractCaveatsTracked(answer string, violations []types.Violation) string {
	if o == nil || o.busCtx == nil {
		return AppendSoftContractCaveatsToAnswerForBus(answer, violations, "", nil)
	}
	bullets, raws := softContractCaveatPayloadForBus(violations, o.busCtx.Language, o.busCtx)
	for _, b := range bullets {
		answer = o.appendRegisteredAnswerCaveatBullet(answer, b)
	}
	for _, r := range raws {
		answer = o.appendRegisteredAnswerCaveatRawSection(answer, r)
	}
	return answer
}

// injectResidualConcernsCaveat is the P6 chokepoint helper that
// surfaces the unresolved typed concern list when the finalize
// repair hard cap is reached AND emits a dock notice so the user-
// facing UI knows the repair-loop terminated by design.
//
// P7 (2026-05-10) channel correction: the caveat now flows through
// the SYSTEM channel ("**补充说明：**" /
// "**Additional notes:**" heading), NOT the LLM-authored
// AnswerDocumentV2.Caveats[] slot. The prior P6 implementation
// wrote into doc.Caveats[] which violates
// feedback_no_system_backfill_to_user_panel — that slot is for
// content gaps the LLM itself identified, not orchestrator
// decisions about repair-loop termination.
//
// Returns the answer string with the caveat appended via the
// system channel; caller assigns back to out.FinalAnswer. Empty
// (nothing to inject) input returns the answer unchanged.
//
// Caller has already decided the cap was exceeded (see the
// finalizeRepairHardCapValue / state.retryUsed comparison in the
// contract-failure loop). The dock notice is rendered through
// softFinalizeRepairCapMessage (CN/EN aware, red-line audited);
// the answer body receives detailed concern lines derived only from
// typed Violation data.
func (o *Orchestrator) injectResidualConcernsCaveat(out *agent.StageOutput, violations []types.Violation) {
	if o == nil || o.busCtx == nil || out == nil {
		return
	}
	concernCount := len(violations)
	// Multi-repo posture detection: only suggest /repos sub-repo
	// adjustment when the workspace actually has ≥2 sub-repos.
	// IsSingle() short-circuits cleanly for legacy single-repo
	// runs so the caveat doesn't render an inapplicable hint.
	multiRepo := false
	if mg, ok := o.busCtx.MultiGraph.(*multigraph.MultiGraph); ok && mg != nil && !mg.IsSingle() {
		multiRepo = true
	}
	caveat := softFinalizeRepairCapMessage(o.busCtx.Language, concernCount, multiRepo)
	// System-channel append: writes typed concern detail to the
	// trailing "**补充说明：**" / "**Additional notes:**" section,
	// distinct from the LLM-authored "**说明**：" / "**Caveats:**"
	// channel. The dock summary below intentionally does NOT get
	// appended to the answer; it is a progress/status message.
	// CAVSTR (2026-07-10): routed through the replay register so the
	// disclosure survives the first-draft-attachment re-render.
	for _, bullet := range MaterializeResidualConcernDetails(violations, o.busCtx.Language) {
		out.FinalAnswer = o.appendRegisteredAnswerCaveatBullet(out.FinalAnswer, bullet)
	}
	// Surface the cap event to the dock so the user knows the
	// loop terminated by design rather than silently shipping with
	// hidden caveats. P7: dedicated NoticeFinalizeRepairCap kind
	// (info-class gray) instead of reusing NoticeFallbackFailLoud
	// (warning yellow) — the answer DID ship, this is informational
	// not a fail-loud.
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeFinalizeRepairCap,
		Reasoning:  caveat,
	})
}
