package orchestrator

// CAVSTR register pins (FRCAP+PSG-2H adversarial review P2 root fix,
// 2026-07-10). Covers:
//
//	survival — a string-channel disclosure seeded before the flagship
//	  overwrite point (attachFirstDraftReference → re-render from doc)
//	  is still in the shipped answer; MUTATION KILL: disabling the
//	  replay in renderFinalAnswerWithLastMileSupplements turns this red;
//	replay idempotency — N re-renders leave exactly ONE instance
//	  (register keyed by verbatim text; fresh renders carry no
//	  string-channel content);
//	register semantics — duplicate registration appends once; ordering
//	  is registration order; raw sections replay in their historical
//	  free-standing form; per-task reset clears the register;
//	PSG-2H interplay — the ship-exit PSG caveat also survives a
//	  re-render ("any overwrite point before the ship exit can no
//	  longer kill the disclosure").

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

func cavstrOrchestrator(t *testing.T) (*Orchestrator, *types.MutableState) {
	t.Helper()
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Ctx: context.Background(), Mutable: mut, Language: "zh"}
	return &Orchestrator{busCtx: bus}, mut
}

func cavstrDoc(text string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: text,
	}}}
}

// TestCAVSTRRegisterPrimitives — dedup by verbatim text, order
// preserved, raw sections keep the free-standing form, reset clears.
func TestCAVSTRRegisterPrimitives(t *testing.T) {
	o, _ := cavstrOrchestrator(t)
	answer := "正文。"
	answer = o.appendRegisteredAnswerCaveatBullet(answer, "披露甲")
	answer = o.appendRegisteredAnswerCaveatBullet(answer, "披露甲") // duplicate → no-op
	answer = o.appendRegisteredAnswerCaveatBullet(answer, "披露乙")
	answer = o.appendRegisteredAnswerCaveatRawSection(answer, "> 独立补充块")
	if strings.Count(answer, "披露甲") != 1 || strings.Count(answer, "披露乙") != 1 {
		t.Fatalf("duplicate registration must append once:\n%s", answer)
	}
	if strings.Count(answer, "**补充说明：**") != 1 {
		t.Fatalf("bullets share one heading:\n%s", answer)
	}
	if !strings.Contains(answer, "> 独立补充块") {
		t.Fatalf("raw section must append:\n%s", answer)
	}
	// Replay onto a fresh surface reproduces all entries, in order.
	replayed := o.replayRegisteredAnswerCaveats("重渲染后的正文。")
	ia, ib, ir := strings.Index(replayed, "披露甲"), strings.Index(replayed, "披露乙"), strings.Index(replayed, "> 独立补充块")
	if ia < 0 || ib < 0 || ir < 0 || !(ia < ib && ib < ir) {
		t.Fatalf("replay must preserve registration order:\n%s", replayed)
	}
	// Per-task reset clears everything.
	o.resetAnswerCaveatReplayRegister()
	if got := o.replayRegisteredAnswerCaveats("下一任务正文。"); got != "下一任务正文。" {
		t.Fatalf("reset register must replay nothing, got:\n%s", got)
	}
}

// TestCAVSTRDisclosureSurvivesFirstDraftAttachment — the review REPRO
// shape: disclosures seeded, then attachFirstDraftReference re-renders
// FinalAnswer from the structured doc (the exact overwrite that used to
// vaporize every string-channel caveat). The disclosure must survive,
// exactly once. MUTATION KILL: a replay-disabled chokepoint turns this
// red.
func TestCAVSTRDisclosureSurvivesFirstDraftAttachment(t *testing.T) {
	o, mut := cavstrOrchestrator(t)
	doc := cavstrDoc("最终稿正文。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	out := &agent.StageOutput{FinalAnswer: "最终稿正文。"}
	out.FinalAnswer = o.appendRegisteredAnswerCaveatBullet(out.FinalAnswer, "质量审阅仍有 1 项未完全解决,以下列出可见边界。")
	out.FinalAnswer = o.appendRegisteredAnswerCaveatBullet(out.FinalAnswer, "以下数值未能定位于证据面：46.821ms。")
	if !strings.Contains(out.FinalAnswer, "46.821ms") {
		t.Fatalf("seed failed:\n%s", out.FinalAnswer)
	}

	// Flagship overwrite: the rejected first draft is filtered behind an
	// accepted structured doc, while FinalAnswer is still re-rendered from
	// that doc and must replay registered disclosures.
	o.attachFirstDraftReference(out, "第一稿正文（内容不同）。", []types.Violation{{
		Kind: types.ViolBlockCoverageMissing, Detail: "x",
	}}, true, nil)

	if strings.Contains(out.FinalAnswer, "第一稿答案") || strings.Contains(out.FinalAnswer, "第一稿正文") {
		t.Fatalf("accepted structured doc must suppress rejected first-draft telemetry:\n%s", out.FinalAnswer)
	}
	for _, disclosure := range []string{"质量审阅仍有 1 项未完全解决", "以下数值未能定位于证据面：46.821ms。"} {
		if got := strings.Count(out.FinalAnswer, disclosure); got != 1 {
			t.Fatalf("disclosure %q must survive the re-render exactly once, got %d:\n%s",
				disclosure, got, out.FinalAnswer)
		}
	}

	// Idempotency across a SECOND overwrite: still exactly once.
	o.appendAnswerDisplayAttachment(out, types.AnswerDisplayAttachment{
		Kind: types.AnswerDisplayAttachmentMarkdown, Title: "附加参考", Body: "参考体",
		Source: types.AnswerDisplayAttachmentSourceSystemCrossCheck,
	})
	for _, disclosure := range []string{"质量审阅仍有 1 项未完全解决", "以下数值未能定位于证据面：46.821ms。"} {
		if got := strings.Count(out.FinalAnswer, disclosure); got != 1 {
			t.Fatalf("second re-render must not duplicate %q, got %d:\n%s",
				disclosure, got, out.FinalAnswer)
		}
	}
}

// TestCAVSTRPSGShipExitCaveatSurvivesRerender — PSG-2H interplay: the
// ship-exit disclosure (latch delivered + shipped doc still violating)
// lands in the register, so a subsequent overwrite replays it — no
// append-order footgun between the ship exit and later overwrites.
func TestCAVSTRPSGShipExitCaveatSurvivesRerender(t *testing.T) {
	mut := psgTraceMutable(psg2hThreadRecord("r1", "dh-irq-bind-0-89", "38.996"))
	bus := psgBus(mut)
	o := &Orchestrator{busCtx: bus}
	doc := psgProseDoc("聚合影响达 46.821ms，另见线程 dh-irq-bind-4-93。")
	if got := runProseScalarGroundingCheck(doc, bus, mut); len(got) != 1 {
		t.Fatalf("first pass must raise, got %+v", got)
	}
	psg2hDeliverHint(t, mut, bus, doc)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	out := &agent.StageOutput{FinalAnswer: "正文结论。"}
	out.FinalAnswer = o.appendProseScalarResidualCaveatToAnswer(out.FinalAnswer)
	if !strings.Contains(out.FinalAnswer, "以下数值/实体未能定位于本报告的证据面") ||
		!strings.Contains(out.FinalAnswer, "dh-irq-bind-4-93") {
		t.Fatalf("ship-exit caveat must have been appended:\n%s", out.FinalAnswer)
	}
	rerendered := o.renderFinalAnswerWithLastMileSupplements(mut.AnswerDocumentV2(), nil)
	if got := strings.Count(rerendered, "以下数值/实体未能定位于本报告的证据面"); got != 1 {
		t.Fatalf("PSG disclosure must survive the re-render exactly once, got %d:\n%s", got, rerendered)
	}
}
