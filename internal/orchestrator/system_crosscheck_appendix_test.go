package orchestrator

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// system_crosscheck_appendix_test.go — S3' pins (§29.47.1 user ruling,
// 2026-07-12; witnesses 2779 + 76278). ① soft findings trigger ZERO repair
// rounds (the kinds are plain-soft for the bus, never retry roots); ② the
// system's mechanical findings render as ONE deterministic appendix block
// at the answer tail, humble lead-in + user-readable items, zero findings →
// no block; the block never names internal machinery.

func crossCheckHarness(t *testing.T, prose string) (*Orchestrator, *agent.StageOutput) {
	t.Helper()
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758", "type=binder_wait"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	doc := psgProseDoc(prose)
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "正文"}
	return o, out
}

// TestSoftProseLanesNeverRetry — S3' ①: the two prose lanes' kinds are
// SOFT for every bus (no strict-arm promotion) and never survive the
// finalizer retry-root filter — a soft-only round ships the first draft
// with ZERO repair dispatches.
func TestSoftProseLanesNeverRetry(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758"))
	bus := psgBus(mut)
	soft := []types.Violation{
		{Kind: types.ViolProseScalarUngrounded, Detail: "45ms"},
		{Kind: types.ViolProseLexiconBoardInconsistent, Detail: "token"},
	}
	for _, v := range soft {
		if isStrictViolationForBus(v, bus) {
			t.Fatalf("S3' ①: %s must never be strict for the bus", v.Kind)
		}
	}
	if got := FilterFinalizerRetryRootViolationsForBus(soft, bus); len(got) != 0 {
		t.Fatalf("S3' ①: soft prose findings must never become retry roots, got %+v", got)
	}
}

// TestSystemCrossCheckAppendixRendersFindings — S3' ②: unlocatable scalar +
// unpublished token on the shipped doc render as ONE appendix attachment
// with the humble lead-in and user-readable items.
func TestSystemCrossCheckAppendixRendersFindings(t *testing.T) {
	o, out := crossCheckHarness(t, "耗时 45.123ms,类型为 made_up_snake_token。")
	o.attachSystemCrossCheckAppendix(out, "", nil)
	atts := o.busCtx.Mutable.AnswerDisplayAttachments()
	if len(atts) != 1 || atts[0].Title != "系统校验附注" {
		t.Fatalf("expected one appendix attachment, got %+v", atts)
	}
	body := atts[0].Body
	for _, want := range []string{
		"以下为系统对正文中出现的实体/数值的 typed 事实对照，供交叉核验；系统不判定正文正误。",
		"45.123ms",
		"made_up_snake_token",
		"未能在本报告证据面复算或定位",
		"未在本报告证据面出现",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("appendix body missing %q:\n%s", want, body)
		}
	}
}

// TestSystemCrossCheckAppendixSilentWhenClean — zero findings → zero block,
// answer untouched.
func TestSystemCrossCheckAppendixSilentWhenClean(t *testing.T) {
	o, out := crossCheckHarness(t, "传导延迟 15.758ms,类型为 binder_wait。")
	before := out.FinalAnswer
	o.attachSystemCrossCheckAppendix(out, "", nil)
	if atts := o.busCtx.Mutable.AnswerDisplayAttachments(); len(atts) != 0 {
		t.Fatalf("a clean doc must render no appendix, got %+v", atts)
	}
	if out.FinalAnswer != before {
		t.Fatalf("a clean doc must leave the answer byte-identical")
	}
}

// TestSystemCrossCheckAppendixNoInternalJargon — the appendix speaks the
// user's language: no internal machinery vocabulary on either language
// face, for every finding family at once.
func TestSystemCrossCheckAppendixNoInternalJargon(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758"))
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: lang}}
		doc := psgProseDoc("耗时 45.123ms,类型为 made_up_snake_token;主根因是 a-1,主根因是 b-2。")
		mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
		out := &agent.StageOutput{FinalAnswer: "body"}
		o.attachSystemCrossCheckAppendix(out, finalizeRepairArbitrationNote(lang), nil)
		atts := o.busCtx.Mutable.AnswerDisplayAttachments()
		if len(atts) != 1 {
			t.Fatalf("[%s] expected one attachment, got %d", lang, len(atts))
		}
		body := atts[0].Body
		// Blocklist entries are matched case-sensitively (acronyms like
		// "ERM" must not trip on English words like "term"); the generic
		// machinery words below are matched case-insensitively.
		for _, term := range skill.InternalTermsBlocklist {
			if strings.Contains(body, term) {
				t.Fatalf("[%s] appendix leaks internal term %q:\n%s", lang, term, body)
			}
		}
		lower := strings.ToLower(body)
		for _, term := range []string{"violation", "finalize", "orchestrator",
			"retry", "contract", "lexicon", "prose_scalar", "kind=", "stage"} {
			if strings.Contains(lower, term) {
				t.Fatalf("[%s] appendix leaks internal term %q:\n%s", lang, term, body)
			}
		}
	}
}

// TestSystemCrossCheckAppendix76278WitnessShape — the §29.47.1 second
// witness: the body quotes fscache_page_wait while the attached trace's
// real token is the truncated fscache_page_wait_o. The near-quote still
// counts as a finding (set membership is exact), but under S3' it ships
// as ONE appendix item — the raise is never strict, so the whole-answer
// rewrite that damaged the 76278 report can no longer happen.
func TestSystemCrossCheckAppendix76278WitnessShape(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	o.busCtx.AttachedHitrace = "  kworker/0:1-42 (42) [000] .... 1.0: sched_blocked_reason: pid=7 iowait=1 caller=fscache_page_wait_o\n"
	doc := psgProseDoc("等待原因为 fscache_page_wait，属于页缓存等待。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	violations := lexiconBoardViolations(runProseLexiconBoardCheck(doc, o.busCtx, mut))
	if len(violations) != 1 || isStrictViolationForBus(violations[0], o.busCtx) {
		t.Fatalf("the near-quote must raise exactly one NON-strict finding, got %+v", violations)
	}
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	atts := o.busCtx.Mutable.AnswerDisplayAttachments()
	if len(atts) != 1 || !strings.Contains(atts[0].Body, "fscache_page_wait") {
		t.Fatalf("the finding must surface on the appendix, got %+v", atts)
	}
	// The trace's REAL truncated token is quotable (S1a corpus) — control.
	mut2 := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758"))
	bus2 := psgBus(mut2)
	bus2.AttachedHitrace = o.busCtx.AttachedHitrace
	honest := psgProseDoc("等待原因为 fscache_page_wait_o（trace 原文截断形）。")
	if got := lexiconBoardViolations(runProseLexiconBoardCheck(honest, bus2, mut2)); len(got) != 0 {
		t.Fatalf("the attachment's own token must never flag, got %+v", got)
	}
}

// TestSystemCrossCheckAppendixWiredAtBothShipExits — 突变 pin (S3' ②
// wiring): the read scheduler's TWO answer-shipping exits (the main exit
// and the forced-finalize early return) must both route through
// attachSystemCrossCheckAppendix — removing either call silently drops the
// information lane for that exit.
func TestSystemCrossCheckAppendixWiredAtBothShipExits(t *testing.T) {
	raw, err := os.ReadFile("orchestrator.go")
	if err != nil {
		t.Fatalf("read orchestrator.go: %v", err)
	}
	if got := strings.Count(string(raw), "o.attachSystemCrossCheckAppendix(lastFinalize, repairArbitrationNote, repairArbitrationResiduals)"); got < 2 {
		t.Fatalf("both ship exits must attach the system cross-check appendix, found %d call site(s)", got)
	}
}

// TestSystemCrossCheckAppendixMisboundZHFace — the binding-mismatch family
// renders its own zh user face on the appendix (per-item), never the
// English detail sentence on the zh surface.
func TestSystemCrossCheckAppendixMisboundZHFace(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	doc := psgBindingDoc(
		"在 bindApplication 窗口（3680.819~3682.619s，1800ms）内，线程 com.xs.fm.lite-6565 的状态分布：sleep 占 63.6%（1430ms），是绝对主导状态。",
		psgSnapshot70())
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	atts := o.busCtx.Mutable.AnswerDisplayAttachments()
	if len(atts) != 1 {
		t.Fatalf("expected one appendix attachment, got %+v", atts)
	}
	body := atts[0].Body
	if !strings.Contains(body, "在证据面发布于窗口") {
		t.Fatalf("the zh face must speak the published-window fact in Chinese:\n%s", body)
	}
	// CR-4 修复轮方向改造: fact form only — no prose characterization.
	for _, banned := range []string{"正文将", "正文称", "表述为"} {
		if strings.Contains(body, banned) {
			t.Fatalf("accusatory wording %q must not render:\n%s", banned, body)
		}
	}
	if strings.Contains(body, "is published under window") {
		t.Fatalf("the English detail sentence must not leak onto the zh face:\n%s", body)
	}
}

// TestSystemCrossCheckAppendixNeverEntersModelBlocks — P3-5 structural pin:
// the appendix is a display attachment on the system surface ONLY — the
// model-authored document's block structure stays byte-identical (the S3'
// red line: the system never writes into the model's answer body).
func TestSystemCrossCheckAppendixNeverEntersModelBlocks(t *testing.T) {
	o, out := crossCheckHarness(t, "耗时 45.123ms,类型为 made_up_snake_token。")
	before, err := json.Marshal(o.busCtx.Mutable.AnswerDocumentV2())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	o.attachSystemCrossCheckAppendix(out, finalizeRepairArbitrationNote("zh"), nil)
	after, err := json.Marshal(o.busCtx.Mutable.AnswerDocumentV2())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("the appendix must never mutate the model document:\nbefore=%s\nafter=%s", before, after)
	}
	if atts := o.busCtx.Mutable.AnswerDisplayAttachments(); len(atts) != 1 {
		t.Fatalf("the appendix must ride the display-attachment lane, got %+v", atts)
	}
}
