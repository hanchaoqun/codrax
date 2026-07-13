package orchestrator

// prose_cr4_arm6_test.go — CR-4 席位面 pins, refactored to the fact-
// juxtaposition ruling (用户裁定 2026-07-12; the accusatory arms retired).
//
// The two-round ghost-seat witnesses (56249 CAL-1 + 91951 SMR-1) stay
// pinned in the ENGINE's real document shape:
//   - round 1 (#N chips): the CR-2 F-2② seat gate — now scanning fused
//     render-shape units (the unit-granularity root fix survives the
//     refactor) — catches the unseated subjects;
//   - round 2 (R-chips): no chip grammar claims anything; the fact lane's
//     「typed 席位=无」 line for the prose-present subject carries the
//     juxtaposition instead;
//   - the transplanted confidence renders as a FACT (published-for line),
//     never as a prose characterization.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cr4Arm6Round1Doc is the 56249 (CAL-1 round) witness board VERBATIM, in the
// engine's ordered-list shape: Title carries the head, each item carries the
// bold label (chip + subject + value) and the text (type + confidence).
func cr4Arm6Round1Doc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "ol-causes", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		Title: "卡顿根因排序（按 effective_attribution 排列）",
		Items: []types.AnswerBlockItem{
			{Label: "#1 CompThread_0-2955 D/IO 等待 36.757 ms", Text: "D_state_or_io_wait，channel=on_chain，confidence=0.82，持 D 状态导致目标线程依赖资源不可用，是最强根因信号。"},
			{Label: "#2 JankManager-9655 runnable_wait 16.687 ms", Text: "runnable_wait，channel=on_chain，confidence=0.76，是目标线程在就绪队列中等待调度的有效归属时间。"},
			{Label: "#3 app-9511 优先级反转聚合 21.153 ms", Text: "priority_inversion_candidate，channel=on_chain，confidence=0.91，waker prio(52) < wakee(53)，两次出现聚合贡献量高于单条 runnable_wait。"},
			{Label: "#4 DetectViewRect-17679 优先级反转 14.302 ms", Text: "priority_inversion_candidate，channel=on_chain，confidence=0.91，waker prio(24/ohos_cfs) < wakee(53/ohos_rt)，构成第三大阻塞因子。"},
		},
	}}}
}

// cr4Arm6Round2Doc is the 91951 (SMR-1 round) witness board VERBATIM — the
// 「R3:」 chip form.
func cr4Arm6Round2Doc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "root-cause-rank", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		Title: "根因排序",
		Items: []types.AnswerBlockItem{
			{Label: "R1: CompThread_0-2955 D-state 背压", Text: "36.757ms on-chain 主根因 (tier=primary, d_state=36.757ms, io_wait=0.000ms)。多核分散传导，沿 TimerDispatch -> app-9511 -> ugc.aweme.lite 链形成累计背压，是窗口内持续时间最长的阻塞来源。"},
			{Label: "R2: JankManager-9655 runnable_wait", Text: "16.687ms on-chain 次根因 (tier=secondary, runnable_wait, cpu=0 freq=1160000kHz)。"},
			{Label: "R3: app-9511 欠优先级唤醒 (priority_inversion_candidate)", Text: "21.153ms 聚合影响，priority_inversion_candidate 已确认 (confidence=0.91)。waker_prio=52/ohos_rt < wakee_prio=53/ohos_rt，形成反向优先级依赖，延迟 ugc.aweme.lite 的就绪通知。"},
		},
	}}}
}

// cr4Arm6SystemIndexBlock is the thread-silent system evidence-index audit
// line (rank=3 · confidence=0.91, no name-tid token) that used to mute the
// confidence arm — the carrier-hygiene fix (typed record.Confidence only)
// keeps it inert.
func cr4Arm6SystemIndexBlock() types.AnswerBlock {
	blk := types.AnswerBlock{
		ID:   "runtime_trace_causal_projection_evidence",
		Kind: types.BlockTable,
		Text: "- **E35** — 定位: 行 22662–24827; 审计: tier=tertiary · causality=on_wakeup_chain · rank=3 · confidence=0.91 · merged",
	}
	blk.SystemGeneratedKind = types.AnswerSystemGeneratedRuntimeTrace
	return blk
}

// cr4BannedWordingCheck asserts the fact-juxtaposition ruling's wording
// grep: accusatory characterizations never render.
func cr4BannedWordingCheck(t *testing.T, lines []string) {
	t.Helper()
	joined := strings.Join(lines, "\n")
	for _, banned := range []string{"正文将", "正文称", "表述为"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("accusatory wording %q must never render:\n%s", banned, joined)
		}
	}
}

func cr4AllAppendixLines(t *testing.T, doc *types.AnswerDocumentV2, bus *types.BusContext, mut *types.MutableState) []string {
	t.Helper()
	var lines []string
	tokens, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	lines = append(lines, tokens...)
	for _, f := range misbound {
		lines = append(lines, f.userReadable("zh"), f.userReadable("en"))
	}
	for _, f := range proseLexiconBoardResidualFindings(doc, bus, mut) {
		lines = append(lines, f.userReadable("zh"), f.userReadable("en"), f.internal)
	}
	for _, f := range proseWallClockConservationFindings(doc, bus, mut) {
		lines = append(lines, f.userReadable("zh"), f.userReadable("en"))
	}
	for _, f := range proseFactJuxtapositionFindings(doc, bus, mut) {
		lines = append(lines, f.userReadable("zh"), f.userReadable("en"))
	}
	return lines
}

// TestCR4Arm6_Round1WitnessSeatAndConfidenceFacts — 56249 形: the #N board
// must trip the CR-2 seat gate on the fused units, and the transplanted
// 0.91 renders as a published-for FACT (no prose characterization), with
// the thread-silent index line present.
func TestCR4Arm6_Round1WitnessSeatAndConfidenceFacts(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := cr4Arm6Round1Doc()
	doc.Blocks = append(doc.Blocks, cr4Arm6SystemIndexBlock())

	findings := proseLexiconBoardResidualFindings(doc, bus, mut)
	var seatJoined []string
	for _, f := range findings {
		if strings.Contains(f.internal, "holds no seat on the measured board") {
			seatJoined = append(seatJoined, f.internal)
		}
	}
	joined := strings.Join(seatJoined, " | ")
	if !strings.Contains(joined, "app-9511") || !strings.Contains(joined, "DetectViewRect-17679") {
		t.Fatalf("round-1 witness must seat-flag app-9511 and DetectViewRect-17679, got: %+v", findings)
	}
	if strings.Contains(joined, "CompThread_0-2955") || strings.Contains(joined, "JankManager-9655") {
		t.Fatalf("seated subjects must not be flagged: %s", joined)
	}

	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	var confHit string
	for _, f := range advisory {
		if strings.Contains(f.entry, "confidence 0.91") {
			confHit = f.entry
		}
	}
	if confHit == "" {
		t.Fatalf("the transplanted 0.91 must render its published-for fact despite the thread-silent index line, got %+v", advisory)
	}
	if !strings.Contains(confHit, "CompThread_0-2955") {
		t.Fatalf("the confidence fact must name the publishing subject: %s", confHit)
	}
	cr4BannedWordingCheck(t, cr4AllAppendixLines(t, doc, bus, mut))
}

// TestCR4Arm6_Round2WitnessFactLane — 91951 形 (R-chips): the chip grammar
// claims nothing; the fact lane must carry app-9511's 「typed 席位=无」 line
// (the ghost-seat juxtaposition) plus the seated rows' seat facts.
func TestCR4Arm6_Round2WitnessFactLane(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := cr4Arm6Round2Doc()
	doc.Blocks = append(doc.Blocks, cr4Arm6SystemIndexBlock())

	facts := proseFactJuxtapositionFindings(doc, bus, mut)
	var appLine, seatedLine string
	for _, f := range facts {
		zh := f.userReadable("zh")
		if strings.Contains(zh, "app-9511") && strings.Contains(zh, "typed 席位=无") {
			appLine = zh
		}
		if strings.Contains(zh, "CompThread_0-2955") && strings.Contains(zh, "typed 席位=#1") {
			seatedLine = zh
		}
	}
	if appLine == "" {
		t.Fatalf("round-2 witness must render app-9511's typed 席位=无 fact, got %+v", facts)
	}
	if seatedLine == "" {
		t.Fatalf("the seated subject's seat fact must render for juxtaposition, got %+v", facts)
	}
	cr4BannedWordingCheck(t, cr4AllAppendixLines(t, doc, bus, mut))
}

// TestCR4Arm6_CircledOptimizationListDrawsNoSeatFinding — 复放误报向量
// (donghu 54110 verbatim): the 优化点 ①..⑤ list is ordinary numbering — no
// chip grammar, no seat finding; any fact lines that render are typed truth.
func TestCR4Arm6_CircledOptimizationListDrawsNoSeatFinding(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "optimization-points", Kind: types.BlockSection, SurfaceRole: types.SurfacePrincipal,
		Title: "优化点",
		Text:  "① 提升 CompThread_0 调度优先级至 RT 级别，消除 D 态阻塞瓶颈（36.757ms）；② JankManager-9655 提升至 RT 级别；③ keva 线程 IO 操作异步化；④ 合并短唤醒链；⑤ 评估 DetectViewRect-17679 触发频率，必要时合并入主线程或提升其优先级。",
	}}}
	for _, f := range proseLexiconBoardResidualFindings(doc, bus, mut) {
		if strings.Contains(f.internal, "holds no seat") {
			t.Fatalf("an optimization list must never read as a board: %s", f.internal)
		}
	}
	cr4BannedWordingCheck(t, cr4AllAppendixLines(t, doc, bus, mut))
}

// TestCR4Arm6_LooseNameProximityMutesBindingClaims — 复放误报向量 (donghu
// 54110 verbatim shape): 1.354ms belongs to 「keva-3」 (single-digit tail,
// invisible to the strict extractor); the published-for fact for the value
// must not render off the farther JankManager binding.
func TestCR4Arm6_LooseNameProximityMutesBindingClaims(t *testing.T) {
	rec := psgTraceRecord("trace_query:t#root_cause_rank:1", "root_cause_rank_4", "1.354", "rank=4")
	rec.Subject = "keva-3-17439"
	rec.Object = "io_wait"
	other := psgTraceRecord("trace_query:t#root_cause_rank:2", "root_cause_secondary", "16.687", "rank=2")
	other.Subject = "JankManager-9655"
	mut := psgTraceMutable(rec, other)
	bus := psgBus(mut)
	doc := psgProseDoc("次要根因为 JankManager-9655 的 runnable_wait 累计 16.687ms,其中 keva-3 同时携带 io_wait 1.354ms。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	for _, f := range misbound {
		if strings.Contains(f.entry, "1.354") {
			t.Fatalf("a nearer loose-only name must mute the binding fact: %s", f.entry)
		}
	}
}

// TestCR4Arm6_P3bBindsLabelSubjectNotTextNeighbor — the fused-unit detection
// fix: the 主根因 claim binds the Label's CompThread (nearest), not the
// Text's app-9511 — the board #1 agreement keeps P3b silent.
func TestCR4Arm6_P3bBindsLabelSubjectNotTextNeighbor(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := cr4Arm6Round2Doc()
	for _, f := range proseLexiconBoardResidualFindings(doc, bus, mut) {
		if strings.Contains(f.internal, "differs from the measured board") ||
			strings.Contains(f.internal, "different entities as THE primary") {
			t.Fatalf("the fused unit must bind 主根因 to the Label's CompThread (board #1 agreement): %s", f.internal)
		}
	}
}

// TestCR4FactLaneAirtightness — 零输出影响 pin (structural): the fact
// provider is consumed ONLY by the system cross-check appendix — never by a
// violation, retry, or caveat path. Source grep over the package.
func TestCR4FactLaneAirtightness(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "proseFactJuxtapositionFindings(") {
			continue
		}
		switch name {
		case "prose_fact_juxtaposition.go", "system_crosscheck_appendix.go":
			continue
		default:
			t.Fatalf("fact lane leaked outside the appendix surface: %s", name)
		}
	}
}
