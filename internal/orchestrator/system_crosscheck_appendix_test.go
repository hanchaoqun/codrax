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

// TestSystemCrossCheckAppendixRendersTypedFacts — the shipping appendix
// lists true typed facts for a prose-present entity. It does not interpret
// unlocatable numerals or vocabulary into a system verdict.
func TestSystemCrossCheckAppendixRendersFindings(t *testing.T) {
	mut := psgTraceMutable(p6TiebaAccount())
	o := &Orchestrator{busCtx: psgBus(mut)}
	doc := psgProseDoc("com.baidu.tieba-59566 耗时 45.123ms,类型为 made_up_snake_token。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	atts := o.busCtx.Mutable.AnswerDisplayAttachments()
	if len(atts) != 1 || atts[0].Title != "系统校验附注" {
		t.Fatalf("expected one appendix attachment, got %+v", atts)
	}
	body := atts[0].Body
	for _, want := range []string{
		"以下为系统对正文中出现的实体/数值的 typed 事实对照，供交叉核验；系统不判定正文正误。",
		"typed 事实:com.baidu.tieba-59566",
		"窗内五态",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("appendix body missing %q:\n%s", want, body)
		}
	}
	for _, banned := range []string{"45.123ms", "made_up_snake_token", "未能在本报告证据面复算", "正文中线程"} {
		if strings.Contains(body, banned) {
			t.Fatalf("free prose must not receive a system verdict for %q:\n%s", banned, body)
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

// TestSystemCrossCheckAppendix76278WitnessShape — EVAL-B30-OWN2 reverses
// the old ruling for this witness. A near-quoted token in model prose is a
// noisy language observation, not typed authority, so the system neither
// repairs it nor publishes a user-visible correctness verdict.
func TestSystemCrossCheckAppendix76278WitnessShape(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "15.758"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	o.busCtx.AttachedHitrace = "  kworker/0:1-42 (42) [000] .... 1.0: sched_blocked_reason: pid=7 iowait=1 caller=fscache_page_wait_o\n"
	doc := psgProseDoc("等待原因为 fscache_page_wait，属于页缓存等待。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	for _, att := range o.busCtx.Mutable.AnswerDisplayAttachments() {
		if strings.Contains(att.Body, "fscache_page_wait") || strings.Contains(att.Body, "未在本报告证据面出现") {
			t.Fatalf("the retired free-prose verdict must not surface, got %+v", att)
		}
	}
}

// TestModelConclusionProseScannerDisconnectedFromProduction is the exact
// architectural guard for EVAL-B30-OWN2. It pins ownership at the two
// production choke points, not any desired answer wording or case-specific
// root cause: contract evaluation cannot inspect prose board conclusions,
// and ship-time cross-check cannot publish such an interpretation.
func TestModelConclusionProseScannerDisconnectedFromProduction(t *testing.T) {
	for _, file := range []string{"contract_check.go", "system_crosscheck_appendix.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(raw)
		for _, banned := range []string{
			"runProseLexiconBoardCheck(" + "docV2",
			"proseLexiconBoardResidualFindings(" + "doc",
			"proseScalarResidualAppendixInputs(" + "doc",
			"proseWallClockConservationFindings(" + "doc",
			"proseHeadlineElimFindings(" + "doc",
			"proseFactJuxtapositionFindings(" + "doc",
		} {
			if strings.Contains(text, banned) {
				t.Fatalf("%s reconnects retired model-conclusion prose scan %q", file, banned)
			}
		}
	}
}

// EVAL-B54-ARITHSUBJ1 production witness: free prose mentions two threads
// and several state-shaped durations. The old nearest-token scanner bound
// worker values to app-100 and published a false 28.300ms contradiction.
// Shipping may list app-100's true typed account, but it may not publish any
// prose-derived subject/value verdict.
func TestSystemCrossCheckDoesNotBindFreeProseDurationsToTypedSubject(t *testing.T) {
	mut := psgTraceMutable(p6AccountRecord("app-100", 0, 0, 10, 0, 0, 10, 10))
	o := &Orchestrator{busCtx: psgBus(mut)}
	doc := psgProseDoc("app-100 等待 10.000ms；worker-200 runnable 8.300ms 后运行 10.000ms。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	lines := o.collectSystemCrossCheckFindings()
	joined := strings.Join(lines, "\n")
	for _, banned := range []string{"正文中线程", "状态时长之和", "28.300ms", "超过该线程"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("shipping appendix minted a prose-derived verdict %q:\n%s", banned, joined)
		}
	}
	if !strings.Contains(joined, "typed 事实:app-100") ||
		!strings.Contains(joined, "running 0.000/runnable 0.000/sleep 10.000") {
		t.Fatalf("the exact typed account should remain available without a prose verdict:\n%s", joined)
	}
}

func TestSystemCrossCheckDoesNotCharacterizeModelPrimaryCause(t *testing.T) {
	primary := lexiconBoardRankRecord(1, "app-20", "state_churn",
		"rank=1", "tier=primary", "effective_impact_ms=5.000")
	context := lexiconBoardRankRecord(2, "rival-30", "running_context",
		"rank=2", "tier=context", "effective_impact_ms=2.000")
	mut := psgTraceMutable(primary, context)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	doc := psgProseDoc("app-20 是当前窗口首要根因候选；rival-30 是同 CPU 背景，需与关键路径分开。")
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	for _, line := range o.collectSystemCrossCheckFindings() {
		for _, banned := range []string{"正文首因", "the body's primary cause", "正文在不同位置"} {
			if strings.Contains(line, banned) {
				t.Fatalf("system must not characterize the model conclusion via prose scan: %s", line)
			}
		}
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

// TestSystemCrossCheckAppendixDoesNotPublishBindingMismatchVerdict —
// ARITHSUBJ1 retires model-prose binding interpretations from production.
// A noisy window/thread association must not create a system attachment.
func TestSystemCrossCheckAppendixDoesNotPublishBindingMismatchVerdict(t *testing.T) {
	mut := psgTraceMutable(psgTraceRecord("r1", "state_drilldown:x", "38.996"))
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	doc := psgBindingDoc(
		"在 bindApplication 窗口（3680.819~3682.619s，1800ms）内，线程 com.xs.fm.lite-6565 的状态分布：sleep 占 63.6%（1430ms），是绝对主导状态。",
		psgSnapshot70())
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	if atts := o.busCtx.Mutable.AnswerDisplayAttachments(); len(atts) != 0 {
		t.Fatalf("free-prose binding mismatch must not become system authority, got %+v", atts)
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
