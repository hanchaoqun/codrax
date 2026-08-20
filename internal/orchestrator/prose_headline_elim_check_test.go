package orchestrator

// prose_headline_elim_check_test.go — HEADLINE-ELIM 件2+件3 pins
// (§29.104.14.1 witness 定谳, docs/design/real_trace_campaign_20260705.md,
// 2026-07-16; customer witness /Users/han/opt/customlogs/cust_span_runnable
// .txt, donghu 970481 帧). Positive pins replay the witness shape (prose
// headline = the #2 inversion seat while the board's #1 is the 9.586ms
// deterministic 类校验 row; prose 「已运行在…最高频点…排除算力供给不足」 while
// the report published a typed 4.843ms supply-fold deficit seat); negative
// pins hold every silence lane (宁漏勿假指): headline=board, unresolvable
// entity, conflicting entities, negated anchor, incoherent thread+class
// pair, no rank board, multi-board without a resolvable target, and both
// one-sided arm-B forms.
//
// 复核修补轮 pins (2026-07-16, 件1–件6): claim-site inability/subjunctive
// look-behind, entity-site negation (而非/not redirect), membership (之一 /
// "one of") and attribution (用户认为…) anchor kills, arm-B sentence-level
// thread-subject binding, composite-seat deficit-value quoting, and the
// 核心…原因 infix particle guard — each with its FIRED counterpart so the
// noisy side only ever loses hits it would have distorted.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// headlineElimWitnessRecords mirrors the cust_span_runnable board shape:
// chain #1 类校验 (self-deterministic, 9.586), #2/#3 inversion seats
// (8.608/7.727), #4 compute-supply fold seat (eff 4.843, typed deficit
// 4.843), one ADJACENT-channel rank=1 row (its own #N space must never
// hijack the chain #1), and the target's full-window state account.
func headlineElimWitnessRecords() []types.ObservationRecord {
	one := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "9.586",
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=self_deterministic",
		types.TraceNoteKeyEffectiveImpactMS+"=9.586",
		types.TraceNoteKeySemanticClass+"=class_verification",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993",
		types.TraceNoteKeyMemberCount+"=8",
		types.TraceNoteKeyTGID+"=63993")
	one.Subject = "ease.cloudmusic-63993"
	one.Object = "class_verification"

	two := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_secondary", "8.608",
		"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=8.608",
		types.TraceNoteKeyCumulativeImpactMS+"=8.870",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	two.Subject = "shadowhook-task-64305"
	two.Object = "priority_inversion_candidate"

	three := psgTraceRecord("trace_query:t#root_cause_rank:3", "root_cause_tertiary", "7.727",
		"rank=3", "tier=tertiary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=7.727",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	three.Subject = "WifiHandlerThre-12073"
	three.Object = "priority_inversion_candidate"

	four := psgTraceRecord("trace_query:t#root_cause_rank:4", "root_cause_tertiary", "107.084",
		"rank=4", "tier=tertiary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS+"=4.843",
		types.TraceNoteKeySupplyFoldDeficitMS+"=4.843",
		types.TraceNoteKeySupplyFoldIdealMS+"=102.241",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	four.Subject = "ease.cloudmusic-63993"
	four.Object = "compute_supply"

	adjacentOne := psgTraceRecord("trace_query:t#root_cause_rank:5", "root_cause_tertiary", "10.643",
		"rank=1", "tier=tertiary", "chain_relevance=adjacent", "causality=adjacent_to_wakeup_chain",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	adjacentOne.Subject = "workSharkThread-64796"
	adjacentOne.Object = "d_state_or_io_wait"

	account := p6AccountRecord("ease.cloudmusic-63993", 107.084, 2.136, 40.710, 1.452, 0, 151.382, 151.382)
	return []types.ObservationRecord{one, two, three, four, adjacentOne, account}
}

// headlineElimWitnessProse is the witness headline + supply-claim prose
// (production shape: the thread is spelled WITHOUT its tid in the headline
// sentence — the entity resolves through the published inversion word
// family; the supply claim carries the 排除…不足 form).
const headlineElimWitnessProse = "Choreographer#doFrame 970481帧（约151ms分析窗口）的核心丢帧原因为shadowhook-task线程（优先级CFS 20）对UI线程（RT 53）施加的优先级反转阻塞。UI线程CPU频率已运行在超大核最高频点2750000kHz，排除算力供给不足。"

// TestHeadlineElim_WitnessBothArms — the cust_span_runnable replay: arm A
// juxtaposes the prose inversion headline against the board's 类校验 #1 with
// BOTH published values; arm B juxtaposes the supply-adequacy claim against
// the typed supply-fold deficit seat. No banned accusatory wording.
func TestHeadlineElim_WitnessBothArms(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc(headlineElimWitnessProse)

	findings := proseHeadlineElimFindings(doc, bus, mut)
	if len(findings) != 2 {
		t.Fatalf("witness shape must yield exactly the two arm findings, got %d: %+v", len(findings), findings)
	}
	armA := findings[0].userReadable("zh")
	armB := findings[1].userReadable("zh")
	for _, want := range []string{
		"正文核心/首要原因=",
		"shadowhook-task-64305 · 优先级反转候选",
		"根因排序#2",
		"有效归因 8.608ms",
		"本报告根因排序#1=ease.cloudmusic-63993 · 类校验",
		"有效归因 9.586ms",
		"两者不一致",
		"应显式声明分歧并给出数值依据(如适用)",
	} {
		if !strings.Contains(armA, want) {
			t.Fatalf("arm A finding missing %q:\n%s", want, armA)
		}
	}
	armAEN := findings[0].userReadable("en")
	for _, want := range []string{
		"priority_inversion_candidate", "class_verification",
		"root-cause rank #1", "root-cause rank #2", "8.608", "9.586",
		"(if applicable)",
	} {
		if !strings.Contains(armAEN, want) {
			t.Fatalf("arm A EN finding missing %q:\n%s", want, armAEN)
		}
	}
	for _, want := range []string{
		"正文含「排除算力供给不足」",
		"本报告已发布供给折算缺口席:ease.cloudmusic-63993 供给折算缺口 4.843ms(运行频点非最高)",
		"根因排序#4",
	} {
		if !strings.Contains(armB, want) {
			t.Fatalf("arm B finding missing %q:\n%s", want, armB)
		}
	}
	armBEN := findings[1].userReadable("en")
	for _, want := range []string{"supply-fold deficit seat", "supply-fold deficit 4.843", "root-cause rank #4"} {
		if !strings.Contains(armBEN, want) {
			t.Fatalf("arm B EN finding missing %q:\n%s", want, armBEN)
		}
	}
	// CR-4 retired accusatory wordings stay banned on this lane too.
	cr4BannedWordingCheck(t, []string{armA, armB, armAEN, armBEN})
}

// TestHeadlineElim_RetiredFromProduction — the offline diagnostic may
// still compare prose with the board, but model-conclusion interpretation
// cannot ride a system-authored shipping attachment (ARITHSUBJ1/OWN2).
func TestHeadlineElim_RetiredFromProduction(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, Language: "zh"}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, psgProseDoc(headlineElimWitnessProse))
	out := &agent.StageOutput{FinalAnswer: "正文"}
	o.attachSystemCrossCheckAppendix(out, "", nil)
	for _, att := range o.busCtx.Mutable.AnswerDisplayAttachments() {
		for _, banned := range []string{"正文核心/首要原因=", "供给折算缺口席", "正文未出现"} {
			if strings.Contains(att.Body, banned) {
				t.Fatalf("model-conclusion verdict %q leaked into shipping appendix:\n%s", banned, att.Body)
			}
		}
	}
}

// TestHeadlineElim_HeadlineMatchesBoardSilent — 标题因=榜首形: the prose
// headline names the #1 class → zero findings (identity match, either lane).
func TestHeadlineElim_HeadlineMatchesBoardSilent(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	for _, prose := range []string{
		"核心丢帧原因为UI线程自身的类校验工作，属于确定性可优化项。",
		"the root cause of the dropped frame is the thread's own class_verification work.",
		// Thread-identity lane: the headline names the #1 subject verbatim.
		"核心原因为 ease.cloudmusic-63993 自身的确定性工作。",
	} {
		findings := proseHeadlineElimFindings(psgProseDoc(prose), bus, mut)
		for _, f := range findings {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
				t.Fatalf("headline==board must stay silent for %q, got %s", prose, f.userReadable("zh"))
			}
		}
	}
}

// TestHeadlineElim_SilenceLanes — every arm-A ambiguity lane stays silent:
// no extractable entity, conflicting class families, negated anchor,
// thread+class pair no published seat carries, and no anchor at all.
func TestHeadlineElim_SilenceLanes(t *testing.T) {
	records := headlineElimWitnessRecords()
	// An io_latency seat so a two-family conflict is constructible.
	ioSeat := psgTraceRecord("trace_query:t#root_cause_rank:6", "root_cause_tertiary", "1.767",
		"rank=7", "tier=tertiary", "chain_relevance=on_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=1.767",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	ioSeat.Subject = "ease.cloudmusic-63993"
	ioSeat.Object = "io_latency"
	records = append(records, ioSeat)
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"unresolvable entity":     "核心丢帧原因为系统整体负载过高，多个子系统相互作用。",
		"conflicting families":    "核心原因是优先级反转与IO延迟共同作用的结果。",
		"negated anchor":          "shadowhook-task-64305 并非核心原因，真正的问题在别处。",
		"incoherent thread+class": "核心原因为 WifiHandlerThre-12073 的类校验工作。",
		"no anchor":               "shadowhook-task-64305 的优先级反转贡献了 8.608ms 影响。",
	} {
		findings := proseHeadlineElimFindings(psgProseDoc(prose), bus, mut)
		for _, f := range findings {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
				t.Fatalf("%s must stay silent, got %s", name, f.userReadable("zh"))
			}
		}
	}
}

// TestHeadlineElim_NoBoardSilent — 无 rank 板报告: a run without any typed
// rank rows yields zero findings from both arms.
func TestHeadlineElim_NoBoardSilent(t *testing.T) {
	mut := psgTraceMutable(p6AccountRecord("ease.cloudmusic-63993", 107.084, 2.136, 40.710, 1.452, 0, 151.382, 151.382))
	bus := psgBus(mut)
	if findings := proseHeadlineElimFindings(psgProseDoc(headlineElimWitnessProse), bus, mut); len(findings) != 0 {
		t.Fatalf("boardless run must stay silent, got %+v", findings)
	}
}

// TestHeadlineElim_MultiBoardTargetSelection — 多板形取 target 板 #1: two
// boards publish conflicting #1 identities; the sole target_window_states
// account resolves the analysis-target board (XLANE-3 typed board target),
// and the finding compares against THAT board's #1. Without the account the
// arm stays silent.
func TestHeadlineElim_MultiBoardTargetSelection(t *testing.T) {
	records := headlineElimWitnessRecords()
	otherOne := psgTraceRecord("trace_query:u#root_cause_rank:1", "root_cause_primary", "12.000",
		"rank=1", "tier=primary", "chain_relevance=on_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=12.000",
		types.TraceNoteKeyRankBoardTarget+"=other.proc-111")
	otherOne.Subject = "other.proc-111"
	otherOne.Object = "io_latency"
	records = append(records, otherOne)
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	doc := psgProseDoc(headlineElimWitnessProse)

	findings := proseHeadlineElimFindings(doc, bus, mut)
	var armA string
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			armA = f.userReadable("zh")
		}
	}
	if armA == "" {
		t.Fatalf("target-board #1 must be resolvable through the account, got %+v", findings)
	}
	if !strings.Contains(armA, "类校验") || !strings.Contains(armA, "9.586") {
		t.Fatalf("the finding must compare against the TARGET board's #1 (类校验 9.586), got %s", armA)
	}
	if strings.Contains(armA, "12.000") {
		t.Fatalf("the other board's #1 must not leak into the finding: %s", armA)
	}

	// Same two boards WITHOUT a target account → unresolvable → silent.
	var noAccount []types.ObservationRecord
	for _, r := range records {
		if r.Predicate == "target_window_states" {
			continue
		}
		noAccount = append(noAccount, r)
	}
	mut2 := psgTraceMutable(noAccount...)
	findings2 := proseHeadlineElimFindings(doc, psgBus(mut2), mut2)
	for _, f := range findings2 {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			t.Fatalf("conflicting boards without a resolvable target must stay silent, got %s", f.userReadable("zh"))
		}
	}
}

// TestHeadlineElim_SupplyClaimNegationSubjunctiveSilent — 件1 (P1-1) + P3-2:
// inability-prefixed claim hits die (「不能/难以/无法完全排除算力供给不足」
// asserts the deficit MAY exist — quoting the affirmative substring would
// invert the body) and subjunctive clauses (「若排除…」) never claim; the
// bare affirmative form still fires with the deficit-value wording.
func TestHeadlineElim_SupplyClaimNegationSubjunctiveSilent(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"不能排除":   "UI线程频率数据不完整，不能排除算力供给不足。",
		"难以排除":   "从现有采样看，难以排除算力供给不足的可能。",
		"无法完全排除": "本窗口内无法完全排除算力供给不足。",
		"若字句":    "若排除算力供给不足，则瓶颈应在锁竞争。",
	} {
		for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
			if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
				t.Fatalf("%s must stay silent, got %s", name, f.userReadable("zh"))
			}
		}
	}
	findings := proseHeadlineElimFindings(psgProseDoc("综合频点数据，排除算力供给不足。"), bus, mut)
	var armB string
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			armB = f.userReadable("zh")
		}
	}
	if armB == "" {
		t.Fatalf("bare affirmative claim must keep firing, got %+v", findings)
	}
	if !strings.Contains(armB, "供给折算缺口 4.843ms(运行频点非最高)") {
		t.Fatalf("arm B must quote the typed deficit value, got %s", armB)
	}
}

// TestHeadlineElim_EntityNegationRedirect — 件2 (P1-2): a body that
// CORRECTLY denies the inversion (witness counter-example shape, zh + EN)
// never yields an arm-A finding — the negated entity hit dies and nothing
// else resolves; in the 「X 而非 Y」 shape the non-negated X is the body's
// true claimed cause and still juxtaposes when X ≠ #1.
func TestHeadlineElim_EntityNegationRedirect(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"zh 而非 反例":  "核心丢帧原因是UI线程自身的确定性计算工作，而非优先级反转阻塞。",
		"EN not 反例": "The core cause of the dropped frame is the UI thread's own deterministic computation work, not priority inversion blocking.",
	} {
		for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
				t.Fatalf("%s must stay silent, got %s", name, f.userReadable("zh"))
			}
		}
	}
	findings := proseHeadlineElimFindings(psgProseDoc("核心丢帧原因是优先级反转阻塞，而非类校验。"), bus, mut)
	var armA string
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			armA = f.userReadable("zh")
		}
	}
	if armA == "" {
		t.Fatalf("X-而非-Y with X≠#1 must fire with X as the claimed cause, got %+v", findings)
	}
	for _, want := range []string{"shadowhook-task-64305", "8.608", "类校验", "9.586"} {
		if !strings.Contains(armA, want) {
			t.Fatalf("redirect finding missing %q:\n%s", want, armA)
		}
	}
}

// TestHeadlineElim_MembershipAttributionLanes — 件3 (P2-1/P2-3): membership
// forms (zh 「…之一」 suffix, EN "one of" prefix — one rule, two language
// faces) and attributed third-party claims (用户认为/前置分析认为…) never
// anchor; the bare sole-cause form still fires.
func TestHeadlineElim_MembershipAttributionLanes(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	for name, prose := range map[string]string{
		"zh 之一":     "优先级反转阻塞是本帧的主要原因之一。",
		"EN one of": "Priority inversion blocking is one of the primary causes of the dropped frame.",
		"用户认为 归属":   "用户认为核心丢帧原因是优先级反转阻塞。",
		"前置分析认为 归属": "前置分析认为根本原因是优先级反转阻塞。",
	} {
		for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
			if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
				t.Fatalf("%s must stay silent, got %s", name, f.userReadable("zh"))
			}
		}
	}
	// The EN membership kill is semantic, not a plural-regex accident: a
	// singular anchor behind "one of" dies too.
	if proseHeadlineSentenceAnchored("this is one of the primary cause candidates") {
		t.Fatalf("EN one-of prefix must kill the anchor")
	}
	findings := proseHeadlineElimFindings(psgProseDoc("优先级反转阻塞是本帧的主要原因。"), bus, mut)
	fired := false
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			fired = true
		}
	}
	if !fired {
		t.Fatalf("bare sole-cause form must keep firing, got %+v", findings)
	}
}

// TestHeadlineElim_SupplySubjectBinding — 件4 (P2-4): a claim sentence
// naming a FOREIGN thread identity never juxtaposes with the deficit seat
// (「RenderThread-64334 已运行在接近最高频点」 is a true statement about that
// thread); a claim sentence naming the seat's own tid still fires. The
// ref-less default (claim is about the analysis subject) is pinned by the
// witness test.
func TestHeadlineElim_SupplySubjectBinding(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	for _, f := range proseHeadlineElimFindings(psgProseDoc("RenderThread-64334 已运行在接近最高频点。"), bus, mut) {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			t.Fatalf("foreign-thread claim must stay silent, got %s", f.userReadable("zh"))
		}
	}
	findings := proseHeadlineElimFindings(psgProseDoc("ease.cloudmusic-63993 已运行在超大核最高频点。"), bus, mut)
	fired := false
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			fired = true
		}
	}
	if !fired {
		t.Fatalf("seat-tid claim must keep firing, got %+v", findings)
	}
}

// TestHeadlineElim_SupplyCompositeSeatDeficitValue — 件5 (P2-5): a composite
// seat (E11 shape: eff 2.004 = runnable full 0.093 + running folded 1.911)
// quotes the typed supply_fold_deficit_ms — the value that GATES the arm —
// never the eff, whose full-amount components must not wear the folded
// label. The pure fold seat (witness E5, eff == deficit == 4.843) keeps its
// value verbatim via the witness test.
func TestHeadlineElim_SupplyCompositeSeatDeficitValue(t *testing.T) {
	var records []types.ObservationRecord
	for _, r := range headlineElimWitnessRecords() {
		if strings.Contains(r.ID, "#root_cause_rank:4") {
			continue // the composite seat replaces the pure fold seat
		}
		records = append(records, r)
	}
	comp := psgTraceRecord("trace_query:t#root_cause_rank:9", "root_cause_tertiary", "2.004",
		"rank=6", "tier=tertiary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS+"=2.004",
		types.TraceNoteKeySupplyFoldDeficitMS+"=1.911",
		types.TraceNoteKeyRankBoardTarget+"=ease.cloudmusic-63993")
	comp.Subject = "ease.cloudmusic-63993"
	comp.Object = "compute_supply"
	records = append(records, comp)
	mut := psgTraceMutable(records...)
	findings := proseHeadlineElimFindings(psgProseDoc("UI线程已运行在超大核最高频点，排除算力供给不足。"), psgBus(mut), mut)
	var armB, armBEN string
	for _, f := range findings {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			armB, armBEN = f.userReadable("zh"), f.userReadable("en")
		}
	}
	if armB == "" {
		t.Fatalf("composite seat must fire, got %+v", findings)
	}
	if !strings.Contains(armB, "供给折算缺口 1.911ms(运行频点非最高)") {
		t.Fatalf("composite seat must quote the typed deficit 1.911, got %s", armB)
	}
	if strings.Contains(armB, "2.004") || strings.Contains(armBEN, "2.004") {
		t.Fatalf("the composite eff 2.004 must never wear the folded label:\nzh %s\nen %s", armB, armBEN)
	}
	if !strings.Contains(armBEN, "supply-fold deficit 1.911") {
		t.Fatalf("EN composite finding must quote the deficit, got %s", armBEN)
	}
}

// TestHeadlineElim_AnchorInfixParticleGuard — 件6① (P2-2): the 核心…原因
// infix refuses structural particles, so the cross-phrase CPU-cores shape
// (「多个核心被抢占的原因」) never anchors even with a resolvable entity in
// the sentence; the witness 「核心丢帧原因」 form keeps anchoring.
func TestHeadlineElim_AnchorInfixParticleGuard(t *testing.T) {
	mut := psgTraceMutable(headlineElimWitnessRecords()...)
	bus := psgBus(mut)
	prose := "多个核心被抢占的原因是shadowhook-task-64305的优先级反转。"
	for _, f := range proseHeadlineElimFindings(psgProseDoc(prose), bus, mut) {
		if strings.Contains(f.userReadable("zh"), "正文核心/首要原因=") {
			t.Fatalf("cross-phrase 核心…原因 shape must stay silent, got %s", f.userReadable("zh"))
		}
	}
	if proseHeadlineSentenceAnchored("多个核心被抢占的原因是后台任务风暴。") {
		t.Fatalf("particle infix must not anchor")
	}
	if !proseHeadlineSentenceAnchored("核心丢帧原因为优先级反转阻塞。") {
		t.Fatalf("the witness 核心丢帧原因 form must keep anchoring")
	}
}

// TestHeadlineElim_SupplyRiderOneSidedSilence — 件3 negative pins: the
// claim without a typed deficit seat is silent; the deficit seat without a
// claim is silent; the typed field decides, never the word face.
func TestHeadlineElim_SupplyRiderOneSidedSilence(t *testing.T) {
	// (a) claim present, no supply-fold deficit note anywhere.
	var noDeficit []types.ObservationRecord
	for _, r := range headlineElimWitnessRecords() {
		keep := r
		if strings.Contains(r.ID, "#root_cause_rank:4") {
			continue // drop the fold seat entirely
		}
		noDeficit = append(noDeficit, keep)
	}
	mutA := psgTraceMutable(noDeficit...)
	docA := psgProseDoc("UI线程已运行在超大核最高频点，排除算力供给不足。")
	for _, f := range proseHeadlineElimFindings(docA, psgBus(mutA), mutA) {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			t.Fatalf("claim without a typed deficit seat must stay silent, got %s", f.userReadable("zh"))
		}
	}
	// (b) deficit seat present, no claim in the prose.
	mutB := psgTraceMutable(headlineElimWitnessRecords()...)
	docB := psgProseDoc("该帧的等待主要来自跨线程唤醒链路，详见根因排序。")
	for _, f := range proseHeadlineElimFindings(docB, psgBus(mutB), mutB) {
		if strings.Contains(f.userReadable("zh"), "供给折算缺口席") {
			t.Fatalf("deficit seat without a prose claim must stay silent, got %s", f.userReadable("zh"))
		}
	}
}

// --- FREQDIR-1 件4 (§29.149 修向④, 2026-07-19) --------------------------------

// freqdirOmissionRecords builds the 95946 witness board head: the chain #1
// non-inversion running seat wearing the engine-stamped fix_direction token
// (frequency_thermal, eff 58.320) plus a #2 inversion seat the prose DOES
// enumerate.
func freqdirOmissionRecords(withDirection bool) []types.ObservationRecord {
	notes := []string{
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS + "=58.320",
		types.TraceNoteKeyTGID + "=17267",
	}
	if withDirection {
		notes = append(notes, types.TraceNoteKeyFixDirection+"=frequency_thermal")
	}
	one := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_primary", "157.248", notes...)
	one.Subject = ".ugc.aweme.lite-17267"
	one.Object = "running"
	one.Confidence = 0.86

	two := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_secondary", "7.405",
		"rank=2", "tier=secondary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		types.TraceNoteKeyEffectiveImpactMS+"=7.405",
		types.TraceNoteKeyFixDirection+"=lock_priority")
	two.Subject = "CompThread_0-2955"
	two.Object = "priority_inversion_candidate"

	// 返工 P3-1: a SECOND same-direction chain seat with a smaller eff — the
	// 最大可消 face must be the direction's MAX (58.320), so a max→min
	// mutation turns the positive pin red.
	three := psgTraceRecord("trace_query:t#root_cause_rank:3", "root_cause_tertiary", "12.100",
		"rank=3", "tier=tertiary", "chain_relevance=on_chain", "causality=self_wall_clock",
		types.TraceNoteKeyEffectiveImpactMS+"=12.100",
		types.TraceNoteKeyFixDirection+"=frequency_thermal")
	three.Subject = "keva-1-17437"
	three.Object = "running"
	return []types.ObservationRecord{one, two, three}
}

// freqdirOmissionProse — the 95946 witness prose shape: a 修复方向 head with
// a numbered enumeration that never names the #1 direction word (no headline
// anchor word and no supply-adequacy claim, so arms A/B stay silent).
const freqdirOmissionProse = "修复方向及各自提升空间如下。\n1. 优先级反转:理论上可消除约 30.060ms 的等待。\n2. fscache 等待优化:约 16.358ms。\n3. 块设备 IO:约 4.4ms。"

// TestHeadlineElim_DirectionOmissionFinding — arm C positive pin (漏报→恰一
// 附注): the enumeration omits the board #1 direction → exactly ONE
// information-only juxtaposition line quoting the direction word and the
// direction's largest seat value; no accusatory wording; nothing else fires.
func TestHeadlineElim_DirectionOmissionFinding(t *testing.T) {
	mut := psgTraceMutable(freqdirOmissionRecords(true)...)
	bus := psgBus(mut)
	doc := psgProseDoc(freqdirOmissionProse)
	findings := proseHeadlineElimFindings(doc, bus, mut)
	if len(findings) != 1 {
		t.Fatalf("the omission shape must yield exactly one arm-C finding, got %d: %+v", len(findings), findings)
	}
	zh := findings[0].userReadable("zh")
	t.Logf("FREQDIR-1 件4 witness appendix line (zh): %s", zh)
	// 返工 P2-2(a): the sentence states the ABSENCE fact only (正文未出现该
	// 方向词) — never a presupposed 清单 (the retired 「正文修复方向清单未
	// 提及」 form assumed a direction list exists). 返工 P3-1: 最大可消 is
	// the direction's MAX across its two seats (58.320 > 12.100).
	for _, want := range []string{
		"事实对照：修向 频率与热治理",
		"最大可消 58.320ms",
		"该方向最大排序项",
		"正文未出现该方向词",
	} {
		if !strings.Contains(zh, want) {
			t.Fatalf("arm C zh finding missing %q:\n%s", want, zh)
		}
	}
	if strings.Contains(zh, "清单") {
		t.Fatalf("arm C must not presuppose a direction list (清单):\n%s", zh)
	}
	en := findings[0].userReadable("en")
	for _, want := range []string{
		"Evidence reference: fix direction frequency & thermal",
		"max recoverable 58.320ms",
		"the direction's word does not appear in the body",
	} {
		if !strings.Contains(en, want) {
			t.Fatalf("arm C EN finding missing %q:\n%s", want, en)
		}
	}
	cr4BannedWordingCheck(t, []string{zh, en})
}

// TestHeadlineElim_DirectionOmissionSilenceLanes — arm C 宁漏勿假指 pins:
// the direction word present in ANY model block (either face or the raw
// token) silences; prose without an enumeration head silences; a head word
// without the enumeration shape silences; a #1 seat without the stamped
// direction token silences.
func TestHeadlineElim_DirectionOmissionSilenceLanes(t *testing.T) {
	silent := func(name string, records []types.ObservationRecord, doc *types.AnswerDocumentV2) {
		t.Helper()
		mut := psgTraceMutable(records...)
		findings := proseHeadlineElimFindings(doc, psgBus(mut), mut)
		if len(findings) != 0 {
			t.Fatalf("%s must stay silent, got %d: %+v", name, len(findings), findings)
		}
	}
	// 词在场任意块 → 静默 (zh face, in a DIFFERENT block from the list).
	docZH := psgProseDoc(freqdirOmissionProse)
	docZH.Blocks = append(docZH.Blocks, types.AnswerBlock{
		ID: "detail", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text: "另见 频率与热治理 方向的折算说明。",
	})
	silent("zh word present in any block", freqdirOmissionRecords(true), docZH)
	// EN face presence silences too.
	docEN := psgProseDoc(freqdirOmissionProse)
	docEN.Blocks = append(docEN.Blocks, types.AnswerBlock{
		ID: "detail", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text: "See the frequency & thermal lane for the folded caliber.",
	})
	silent("EN word present in any block", freqdirOmissionRecords(true), docEN)
	// Raw registry token presence silences.
	docToken := psgProseDoc(freqdirOmissionProse)
	docToken.Blocks = append(docToken.Blocks, types.AnswerBlock{
		ID: "detail", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text: "the seat is stamped frequency_thermal in the rank rows",
	})
	silent("raw token present in any block", freqdirOmissionRecords(true), docToken)
	// 无枚举头 → 静默.
	silent("no enumeration head", freqdirOmissionRecords(true),
		psgProseDoc("主要等待集中在优先级反转与 fscache,合计约 46ms。\n1. 优先级反转\n2. fscache"))
	// Head word without the enumeration shape → 静默 (one bullet is no list).
	silent("head without enumeration shape", freqdirOmissionRecords(true),
		psgProseDoc("提升空间最大的方向是优先级反转,详见上文分析。"))
	// 返工 P3-1: exactly ONE bullet beside the head → 静默 (a single bullet
	// is not an enumeration; the ≥2 threshold is load-bearing).
	silent("single-bullet head", freqdirOmissionRecords(true),
		psgProseDoc("修复方向如下。\n- 优先级反转:约 30.060ms。"))
	// 返工 P2-2(b): horizontal rules never count as enumeration lines — a
	// head unit with hr separators and one real bullet stays below the
	// threshold.
	silent("horizontal rules are not enumerators", freqdirOmissionRecords(true),
		psgProseDoc("修复方向如下。\n---\n- 优先级反转:约 30.060ms。\n----"))
	// 返工 P2-2(b): a bullet symbol needs following whitespace — glued
	// prose dashes never count.
	silent("glued dashes are not bullets", freqdirOmissionRecords(true),
		psgProseDoc("修复方向如下。\n-优先级反转约 30.060ms\n-fscache 约 16.358ms"))
	// 返工 P2-2(c): the enumeration quotes a same-direction seat's published
	// eff face (58.320) — the direction is covered IN SUBSTANCE under other
	// words, so the word-absence juxtaposition stays silent.
	silent("direction eff face inside the enumeration", freqdirOmissionRecords(true),
		psgProseDoc("修复方向及各自提升空间如下。\n1. 供给折算相关优化:约 58.320ms。\n2. fscache 等待优化:约 16.358ms。"))
	// #1 seat without a stamped direction → 静默 (absence stays absent).
	silent("unstamped #1 seat", freqdirOmissionRecords(false), psgProseDoc(freqdirOmissionProse))
}
