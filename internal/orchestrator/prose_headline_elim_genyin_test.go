package orchestrator

// prose_headline_elim_genyin_test.go — ANSWERFACE-1 件3 pins (§29.140 G7,
// 2026-07-19): the copula-bound 根因 anchor members (根因是/根因为/根因在于 —
// never bare 根因, which the report's own 「根因排序」 label saturates) and
// the unpublished-inversion fallback lane. Witness (semantic_span run-2): the
// headline claims 「丢帧的根因是优先级反转」 while the deterministic #1 is a
// class-verification seat and the board seats NO priority-inversion row — the
// closed seat-derived lexicon is structurally blind to an unpublished cause
// word, so the lane matches the CLOSED global inversion family under the full
// §29.110 guard set and juxtaposes it against board #1. Any published
// inversion-family seat silences the lane wholesale (the seat-derived entity
// lanes own that shape — short_runnable 形 pin below).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// genyinSemanticSpanRecords is the semantic_span-isomorphic board: a
// class-verification #1 on the worker thread, the target's own runnable #2,
// zero priority-inversion seats, and the target's four-state account.
func genyinSemanticSpanRecords() []types.ObservationRecord {
	one := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "4.600",
		"rank=1", "tier=deterministic_optimization", "chain_relevance=on_chain", "causality=self_deterministic",
		types.TraceNoteKeyEffectiveImpactMS+"=4.600",
		types.TraceNoteKeySemanticClass+"=class_verification",
		types.TraceNoteKeyRankBoardTarget+"=app-100")
	one.Subject = "worker-200"
	one.Object = "class_verification"

	two := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_secondary", "0.800",
		"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS+"=0.800",
		types.TraceNoteKeyRankBoardTarget+"=app-100")
	two.Subject = "app-100"
	two.Object = "scheduling_pressure_candidate"

	account := p6AccountRecord("app-100", 1.200, 0.800, 5.000, 0, 0, 7.000, 7.000)
	return []types.ObservationRecord{one, two, account}
}

// TestHeadlineElim_UnpublishedInversionCauseJuxtaposed — the semantic run-2
// witness shape: the headline anchors on 根因是, claims the unpublished
// inversion word, AND names the #1 entity in a supporting role (worker-200 as
// the low-priority executor). The entity lane resolves worker-200 == #1 and
// stays silent; the 件3 lane still juxtaposes the inversion claim.
func TestHeadlineElim_UnpublishedInversionCauseJuxtaposed(t *testing.T) {
	mut := psgTraceMutable(genyinSemanticSpanRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("app-100 在 5.000s–5.007s 窗口内丢帧的根因是优先级反转：低优先级线程 worker-200（prio=20/CFS）在同一窗口执行类验证操作 VerifyClass com.example.Foo，占用 CPU 约 5ms，导致高优先级线程 app-100 被唤醒后仅获得 1.2ms 运行时间。")
	findings := proseHeadlineElimFindings(doc, bus, mut)
	if len(findings) != 1 {
		t.Fatalf("the unpublished-inversion shape must yield exactly one finding, got %d: %+v", len(findings), findings)
	}
	zh := findings[0].userReadable("zh")
	for _, want := range []string{
		"正文核心/首要原因=优先级反转(本报告无已发布的该类席位)",
		"本报告根因排序#1=worker-200 · 类校验",
		"有效归因 4.600ms",
		"两者不一致",
		"应显式声明分歧并给出数值依据(如适用)",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh finding must contain %q:\n%s", want, zh)
		}
	}
	en := findings[0].userReadable("en")
	if !strings.Contains(en, "priority inversion (this report publishes no seat of that class)") {
		t.Fatalf("EN finding must carry the unpublished-seat label:\n%s", en)
	}
}

// TestHeadlineElim_InversionSeatPresentNewLaneSilent — short_runnable 形: the
// board DOES seat inversion-family rows and the headline correctly claims
// inversion; the entity lane silences by family identity match, and the 件3
// lane must not resurrect a finding (seat presence silences it wholesale).
func TestHeadlineElim_InversionSeatPresentNewLaneSilent(t *testing.T) {
	one := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "1.661",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=1.661",
		types.TraceNoteKeyRankBoardTarget+"=com.baidu.tieba-59566")
	one.Subject = "CookieMonsterCl-59843"
	one.Object = "priority_inversion_candidate"
	two := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_secondary", "0.014",
		"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS+"=0.014",
		types.TraceNoteKeyRankBoardTarget+"=com.baidu.tieba-59566")
	two.Subject = "com.baidu.tieba-59566"
	two.Object = "priority_inversion_runnable_wait"
	account := p6AccountRecord("com.baidu.tieba-59566", 0, 0.014, 2.978, 0, 0, 2.992, 2.992)
	mut := psgTraceMutable(one, two, account)
	bus := psgBus(mut)
	doc := psgProseDoc("com.baidu.tieba-59566 主线程在这段 2.992ms 窗口内的卡顿根因是优先级反转导致的唤醒延迟。")
	for _, f := range proseHeadlineElimFindings(doc, bus, mut) {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			t.Fatalf("a correctly-claimed seated inversion must stay silent, got %s", f.userReadable("zh"))
		}
	}
}

// TestHeadlineElim_GenyinCopulaAnchorForms — the copula members anchor
// (根因为/根因在于), while interrogative tails (根因是否/根因为何) and the
// report's own 根因排序 label never anchor.
func TestHeadlineElim_GenyinCopulaAnchorForms(t *testing.T) {
	mut := psgTraceMutable(genyinSemanticSpanRecords()...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"根因为 copula":  "本窗丢帧的根因为优先级反转，worker-200 抢占了执行机会。",
		"根因在于 copula": "问题的根因在于优先级反转，而调度器未做优先级继承。",
	} {
		findings := proseHeadlineElimFindings(psgProseDoc(prose), bus, mut)
		found := false
		for _, f := range findings {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=优先级反转(本报告无已发布的该类席位)") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must anchor and juxtapose, got %+v", name, findings)
		}
	}
	for name, prose := range map[string]string{
		"interrogative 根因是否": "根因是否为优先级反转值得进一步分析，需要更多证据。",
		"interrogative 根因为何": "根因为何尚不明确，优先级反转只是候选之一。",
		"report label 根因排序":  "该行参与根因排序的类别、榜位与置信档见图例，优先级反转未上榜。",
	} {
		for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
				t.Fatalf("%s must not anchor, got %s", name, f.userReadable("zh"))
			}
		}
	}
}

// TestHeadlineElim_UnpublishedInversionGuardLanes — the 件3 lane walks the
// full §29.110 guard set: entity negation (而非), subjunctive lead (若),
// inability prefix (不能排除). Each shape stays silent.
func TestHeadlineElim_UnpublishedInversionGuardLanes(t *testing.T) {
	mut := psgTraceMutable(genyinSemanticSpanRecords()...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"negated 而非":  "本窗丢帧的根因是类校验工作，而非优先级反转。",
		"subjunctive": "若根因是优先级反转，则应观测到同 CPU 抢占，但数据并不支持。",
		"inability":   "当前证据不能排除根因是优先级反转的可能，需要补充调度数据。",
	} {
		for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
			if strings.Contains(f.userReadable("zh"), "优先级反转(本报告无已发布的该类席位)") {
				t.Fatalf("%s must silence the unpublished-inversion lane, got %s", name, f.userReadable("zh"))
			}
		}
	}
}
