package tool

// answer_document_projection_run2fixa_test.go — RUN2FIX-A 批 pins (§29.174
// 处置②, customer runnable_2.txt P1 显示六件, 2026-07-20). One pin family per
// 件; the witness geometry references are runnable_2.txt line numbers.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// --- 件1 UX-3: the ◎ section-order promise speaks the tail-last rule ---------

// TestRun2FixASectionPromiseSpeaksTailRule — the head promise no longer claims
// a pure 节内最大可消降序 over a list whose unresolved tail is pinned last by
// design (runnable_2:139 vs :148/:150 承诺自毁): both language faces name the
// tail rule, and the behavior arm proves the tail still parks last even when
// its value TOPS every named section (the promise now states the truth).
func TestRun2FixASectionPromiseSpeaksTailRule(t *testing.T) {
	// OMGCLEAN-1 件1+件3 (§29.175 裁定②/.1, 2026-07-20). EVOLUTION RECORD:
	// the tail word co-moves to 其他方向/"other directions" and the promise
	// compresses (zh one line; EN keeps the structure-pinned two-line split).
	model := runtimeTraceProjTreeModel{Target: "worker-T-77"}
	zhLines := runtimeTraceProjElimHead(model, true, true, true, false)
	if len(zhLines) != 2 || !strings.Contains(zhLines[1], "节=修复方向(其他方向恒末,余按节内最大可消降序)") {
		t.Fatalf("zh promise must state the tail-last rule:\n%s", strings.Join(zhLines, "\n"))
	}
	enLines := runtimeTraceProjElimHead(model, false, true, true, false)
	if len(enLines) != 3 || !strings.Contains(enLines[1], "(other directions tail last)") ||
		!strings.Contains(enLines[2], "rest by max-eliminable desc") {
		t.Fatalf("en promise must state the tail-last rule + desc rule:\n%s", strings.Join(enLines, "\n"))
	}
	// Behavior arm (承诺句与实际节序一致性): an unresolved-direction section
	// whose max eliminable EXCEEDS every named section still renders LAST —
	// exactly what the promise now says.
	entry := func(id, direction string, eff float64) runtimeTraceProjElimEntry {
		node := elimChainNode(id, "worker-T-77", "runnable_wait", "runnable", 1, eff, 100)
		node.FixDirection = direction
		return runtimeTraceProjElimEntry{row: runtimeTraceProjTreeRow{HasData: true,
			Kind: runtimeTraceProjTreeRowChain, Node: node}}
	}
	sections := runtimeTraceProjElimSectionsFor([]runtimeTraceProjElimEntry{
		entry("E-a", "scheduling_supply", 7.394),
		entry("E-b", "", 16.684), // unresolved tail, LARGER than every named section
		entry("E-c", "io_dependency", 2.000),
	})
	if len(sections) != 3 || sections[len(sections)-1].direction != "" {
		t.Fatalf("the unresolved tail must park last regardless of value: %+v", sections)
	}
	if sections[0].maxEff != 7.394 || sections[len(sections)-1].maxEff != 16.684 {
		t.Fatalf("named sections keep max-eliminable desc while the tail holds its value: %+v", sections)
	}
}

// --- 件2 UX-4: the fold row names its MAX member (线程·状态·值) ---------------

// TestRun2FixAFoldMaxMemberDisclosure — runnable_2:361-363: the tree's largest
// single value (47.282ms) hid inside 「其余 6 项(折叠)」 with only a range
// disclosed while micro-value rows held named seats. The fold row-2 now names
// the MAX member with thread·state·value (CAPFIX 件2 带值披露同构); carriers
// absent → the legacy line byte-identically (宁漏勿假).
func TestRun2FixAFoldMaxMemberDisclosure(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		OnChainOverflowFold: true, MergedCount: 6,
		MergedSubjects:     []string{"head-thread-11", "CookieMonsterCl-59843"},
		MergedMaxSubject:   "CookieMonsterCl-59843",
		MergedMaxStateKind: "runnable",
		MergedMinMS:        4.558, MergedMaxMS: 47.282,
	}
	line := runtimeTraceProjFoldMemberSinkLine(node, true)
	want := "成员 head-thread-11 · 成员最大 CookieMonsterCl-59843 · runnable 47.282ms · 其余 5 项见明细"
	if line != want {
		t.Fatalf("fold row-2 must name the max member with 线程·状态·值:\n got %q\nwant %q", line, want)
	}
	// Head == max member: the head mention upgrades in place (no double name).
	inPlace := node
	inPlace.MergedMaxSubject = "head-thread-11"
	inPlace.MergedMaxStateKind = ""
	if got := runtimeTraceProjFoldMemberSinkLine(inPlace, true); got != "成员最大 head-thread-11 47.282ms · 其余 5 项见明细" {
		t.Fatalf("head-is-max upgrades in place (state absent → thread·value): %q", got)
	}
	// Negative arm: carriers absent → legacy bytes (老形 pin, 刻意保持).
	legacy := node
	legacy.MergedMaxSubject, legacy.MergedMaxStateKind = "", ""
	if got := runtimeTraceProjFoldMemberSinkLine(legacy, true); got != "成员 head-thread-11 · 其余 5 项见明细" {
		t.Fatalf("carrier-less folds keep the legacy line byte-identically: %q", got)
	}
	// EN face.
	if got := runtimeTraceProjFoldMemberSinkLine(node, false); got != "member head-thread-11 · member max CookieMonsterCl-59843 · runnable 47.282ms · 5 more in the detail blocks" {
		t.Fatalf("en fold row-2 form: %q", got)
	}
	// 复核 P2-1 碰撞负臂 (对抗 F3+冷读 CR-1, app-951/app-9511 形): prefix-
	// colliding roster names must not adopt each other. max=app-951 rides the
	// roster with its B6 pointer suffix beside head app-9511 — the head is
	// NOT the max (no in-place upgrade) and the clause names the
	// pointer-suffixed app-951 roster string, never app-9511.
	collide := types.TraceCausalProjectionNode{
		OnChainOverflowFold: true, MergedCount: 3,
		MergedSubjects:     []string{"app-9511", "app-951(见榜位#3)"},
		MergedMaxSubject:   "app-951",
		MergedMaxStateKind: "runnable",
		MergedMinMS:        1.0, MergedMaxMS: 47.282,
	}
	want = "成员 app-9511 · 成员最大 app-951(见榜位#3) · runnable 47.282ms · 其余 2 项见明细"
	if got := runtimeTraceProjFoldMemberSinkLine(collide, true); got != want {
		t.Fatalf("prefix collision must not misattribute the max member:\n got %q\nwant %q", got, want)
	}
	// The strip is boundary-exact: both pointer mint forms strip; arbitrary
	// parentheses and non-digit ordinals pass through whole.
	if got := runtimeTraceProjFoldRosterBareSubject("app-951 (see root-cause rank #3)"); got != "app-951" {
		t.Fatalf("en pointer suffix must strip: %q", got)
	}
	if got := runtimeTraceProjFoldRosterBareSubject("Binder:951_A(3)"); got != "Binder:951_A(3)" {
		t.Fatalf("non-pointer parentheses must pass through whole: %q", got)
	}
}

// --- 件3 UX-5: no blank name column ever ships -------------------------------

// TestRun2FixASemanticSpanRowNameKeepsHost — runnable_2:418 E48: a background
// semantic-span-kind row whose span name/object never reached the display
// model rendered an ALL-SPACE name column while the detail table named
// SensorService-9388. Root fix: the host subject IS the row identity
// (三面一致); defense in depth: an all-empty name never ships a blank column.
func TestRun2FixASemanticSpanRowNameKeepsHost(t *testing.T) {
	row := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowBackground,
		Node: types.TraceCausalProjectionNode{
			Subject:   "SensorService-9388",
			Predicate: "trace_semantic_span", // span identity, name lanes empty
		}}
	if got := runtimeTraceProjRowName(row, true); got != "SensorService-9388" {
		t.Fatalf("the host subject must survive a name-less semantic span row: %q", got)
	}
	if got := runtimeTraceProjRowName(row, false); got != "SensorService-9388" {
		t.Fatalf("en face keeps the host too: %q", got)
	}
	// 防御纵深负臂 (空名行禁出厂): every lane empty → placeholder, never "".
	bare := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowBackground,
		Node: types.TraceCausalProjectionNode{Predicate: "trace_semantic_span"}}
	if got := runtimeTraceProjRowName(bare, true); strings.TrimSpace(got) == "" {
		t.Fatalf("an all-empty name row must ship a placeholder, got %q", got)
	}
}

// --- 件4 UX-6: truncation priority reversal ----------------------------------

// TestRun2FixANameTruncationPriorityReversal — runnable_2:286 「c…-59566」 /
// :319 「T…-60555」: the state-phrase keep reservation squeezed thread names
// to 1-2 runes. Under the reversal the tid stays whole, the name keeps a
// legible head prefix (≥ floor), and the STATE phrase boundary-truncates
// first (既有 … 机制); family TYPE words stay whole per RCM-2 D2 (scope pin).
func TestRun2FixANameTruncationPriorityReversal(t *testing.T) {
	node := elimChainNode("E-t", "ThreadPoolForeg-60555", "d_state_or_io_wait", "", 3, 10.433, 100)
	row := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain, Node: node}
	name := runtimeTraceProjRowName(row, true)
	if !strings.HasSuffix(name, " · D-state/iowait") {
		t.Fatalf("fixture must carry the state-phrase keep suffix, got %q", name)
	}
	// fixedW 20 reproduces the witness squeeze (budget after keep < floor):
	// pre-batch this fit rendered the 1-rune 「T…-60555」 head form.
	fitted := runtimeTraceProjRowNameFitted(20, row, name, true)
	if fitted != "Thread…-60555 · D-state…" {
		t.Fatalf("截断策略反转 exact form (floor head + tid whole + boundary-cut state word): %q", fitted)
	}
	head := fitted[:strings.Index(fitted, "…")]
	if w := runewidth.StringWidth(head); w < runtimeTraceProjNameHeadPrefixFloorCells {
		t.Fatalf("name head prefix below the legible floor (%d < %d): %q", w, runtimeTraceProjNameHeadPrefixFloorCells, fitted)
	}
	// 「c…-」形红臂: the 1-rune head form is banned outright.
	if strings.Contains(fitted, "T…-60555") {
		t.Fatalf("the 1-rune head form must not ship: %q", fitted)
	}
	// 中腰双侧省略怪形红臂 (:327 「ThreadPoo…oreg-60555」): a pure-letter head
	// tail distinguishes nothing — the cut falls to the prefix-only form.
	if got := runtimeTraceProjMidTruncateKeepPid("ThreadPoolForeg-60555", 20); got != "ThreadPoolFor…-60555" {
		t.Fatalf("pure-letter head tails keep the prefix-only form: %q", got)
	}
	// B5 marker tails survive byte-identically (existing pin doubled here).
	if got := runtimeTraceProjMidTruncateKeepPid("CompThreadPool_0-2955", 16); got != "CompTh…ol_0-2955" {
		t.Fatalf("marker-bearing head tails keep the B5 form: %q", got)
	}
	// RCM-2 D2 scope pin: a FAMILY row's TYPE word never shrinks — the name
	// yields instead (the pre-batch geometry, byte-stable).
	family := elimChainNode("E-f", "RxComputationT-16816", "block_io_by_inode", "", 3, 1.598, 200)
	family.FamilyMemberCount = 2
	familyRow := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain, Node: family}
	familyName := runtimeTraceProjRowName(familyRow, true)
	if !strings.Contains(familyName, "块设备IO(inode)") {
		t.Fatalf("family fixture must carry the type word, got %q", familyName)
	}
	if got := runtimeTraceProjRowNameFitted(30, familyRow, familyName, true); !strings.Contains(got, "块设备IO(inode)") {
		t.Fatalf("D2: the family type word must survive whole (name yields): %q", got)
	}
}

// --- 件5 UX-7: the wait-denominator two-ruler note ---------------------------

// TestRun2FixAWaitDenomRulerNote — runnable_2:130/:132: 四态合计 144.503 and
// 等待 149.263 sat on adjacent lines with no reconciliation path. Beside a
// provable four-state account, a wait denominator diverging beyond the shared
// jitter wears the same-line ruler note (§29.158 RULER2 同构); agreement or an
// absent account keeps every legacy byte.
func TestRun2FixAWaitDenomRulerNote(t *testing.T) {
	projection := types.TraceCausalProjection{
		WindowStartTs: 100.000, WindowEndTs: 100.144503,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "CookieMonsterCl-59843", WindowStartTs: 100.000, WindowEndTs: 100.144503,
			RunningMS: 8.494, RunnableMS: 26.762, SleepMS: 109.247, DStateMS: 0, IOWaitMS: 0,
		},
	}
	model := runtimeTraceProjTreeModel{Target: "CookieMonsterCl-59843", WindowMS: 144.503}
	note := runtimeTraceProjWaitDenomRulerNote(projection, model, 149.263, true)
	if note != "(按自身状态视图行合计尺,与上方四态行不同尺,不可直接对账)" {
		t.Fatalf("diverging rulers must disclose on the wait line: %q", note)
	}
	if en := runtimeTraceProjWaitDenomRulerNote(projection, model, 149.263, false); !strings.Contains(en, "different ruler") {
		t.Fatalf("en ruler note: %q", en)
	}
	// Agreement within the shared jitter → silent (legacy bytes).
	if note := runtimeTraceProjWaitDenomRulerNote(projection, model, 136.009+0.0004, true); note != "" {
		t.Fatalf("agreeing rulers must not note: %q", note)
	}
	// Account absent → silent on every shape.
	bare := projection
	bare.TargetStateAccount = nil
	if note := runtimeTraceProjWaitDenomRulerNote(bare, model, 149.263, true); note != "" {
		t.Fatalf("account-less shapes stay byte-identical: %q", note)
	}
}

// --- 件6 UX-8①: badge symmetry + the headline caliber note -------------------

// run2fixaSelfInversionProjection — the runnable_2 E1/E2 geometry scaled to a
// fixture: TWO self seats of one thread, each rank#1 on its OWN board — the
// inversion seat (板锚 other-board) and the runnable seat (锚点板). Pre-batch
// the inversion seat sat badge-bare while its 行2 printed 根因排序#1.
func run2fixaSelfInversionProjection() types.TraceCausalProjection {
	inv := elimv2DirectionNode("R2f-inv", "worker-T-77", "priority_inversion_candidate", "runnable",
		1, 8.0, 100, "lock_priority", 1000.010, 1000.020)
	inv.PriorityInversionCandidate = true
	inv.RankBoardTarget = "peer-board-88"
	run := elimv2DirectionNode("R2f-run", "worker-T-77", "runnable_wait", "runnable",
		1, 6.0, 200, "scheduling_supply", 1000.030, 1000.040)
	run.RankBoardTarget = "worker-T-77"
	dep := elimv2DirectionNode("R2f-dep", "worker-B-88", "runnable_wait", "runnable",
		2, 3.0, 300, "scheduling_supply", 1000.050, 1000.055)
	dep.RankBoardTarget = "worker-T-77"
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-1", "worker-T-77"},
		WindowStartTs:           1000.000, WindowEndTs: 1000.200,
		OnChainCauses:     []types.TraceCausalProjectionNode{inv, run, dep},
		PrimaryRootCauses: []types.TraceCausalProjectionNode{run, dep},
	}
}

// TestRun2FixASelfInversionSeatBadge — 每板 rank#N(N≤5)持值席佩章完备:
// runnable_2:179 E1 (self inversion, engine 根因排序#1) sat bare while E2
// (#1, another board) wore ❶. The badge lane now admits the engine-published
// self inversion seat; the ELECTION population stays untouched (crown
// zero-move — the deliberate asymmetry, rationale on the helper).
func TestRun2FixASelfInversionSeatBadge(t *testing.T) {
	projection := run2fixaSelfInversionProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var invRow, runRow *runtimeTraceProjTreeRow
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for i := range rows {
			switch rows[i].Node.EvidenceID {
			case "R2f-inv":
				invRow = &rows[i]
			case "R2f-run":
				runRow = &rows[i]
			}
		}
	}
	if invRow == nil || runRow == nil {
		t.Fatalf("fixture rows missing: inv=%v run=%v", invRow != nil, runRow != nil)
	}
	if invRow.Badge != 1 {
		t.Fatalf("the engine-published self inversion seat must wear ❶ (badge=%d)", invRow.Badge)
	}
	if runRow.Badge != 1 {
		t.Fatalf("the runnable board #1 keeps its ❶ (badge=%d)", runRow.Badge)
	}
	// 佩章完备性 (复核 F8, predicate narrowed): the population is exactly the
	// badge authority's own two lanes — the §29.30.1 valid-seat gate ∪ the
	// 件6 self-inversion badge arm. The former HAS-rank sweep would false-red
	// on lane-barred stale-rank rows (a shape this fixture happens not to
	// contain — fixture-dependent truth, now removed). The pin's teeth are
	// the WIRING: runtimeTraceProjAssignTopBadges must have visited every
	// row group and stamped Badge — a skipped group ships bare glyphs.
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for i := range rows {
			row := rows[i]
			_, valid := runtimeTraceProjRowValidSeat(row)
			_, invArm := runtimeTraceProjSelfInversionSeatBadge(row)
			if !valid && !invArm {
				continue
			}
			if row.Badge == 0 {
				t.Fatalf("badge completeness: seat %s (rank#%d) is bare", row.Node.EvidenceID, row.Node.Rank)
			}
		}
	}
	// Crown lane untouched: the inversion seat holds NO valid election seat.
	if _, ok := runtimeTraceProjRowValidSeat(*invRow); ok {
		t.Fatalf("the election gate must stay closed to the self inversion seat (crown zero-move)")
	}
	// The crown goes to the runnable seat and the headline wears the caliber
	// note (锚点板#1 vs ◎ 具名节最大 — the ◎ first section tops with 8.0;
	// 复核 P2-2: the wording names the trigger's actual comparison object).
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "关注线程自身 runnable") {
		t.Fatalf("crown must stay on the runnable seat:\n%s", line)
	}
	if !strings.Contains(line, "(锚点板#1;◎ 按具名节最大)") {
		t.Fatalf("headline must wear the caliber-divergence note:\n%s", line)
	}
	// Fence face: the ❶ renders beside the inversion self row.
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "❶ ⇅ 自身·") {
		t.Fatalf("the self inversion row must wear ❶ on the fence:\n%s", fence)
	}
}

// TestRun2FixAHeadlineNoteSilentWhenCrownTopsOverview — negative arm: when the
// crowned seat IS the ◎ first-section top (the healthy shape), the headline
// keeps its legacy bytes — no note.
func TestRun2FixAHeadlineNoteSilentWhenCrownTopsOverview(t *testing.T) {
	projection := elimv2DirectionBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if strings.Contains(line, "◎ 按具名节最大") {
		t.Fatalf("crown==overview-top must not note:\n%s", line)
	}
}

// TestRun2FixAFirstSectionTopMatchesOverviewHead — 复核 CR-7 一致性 pin: on a
// fixture where the caliber parenthetical is IN PLAY (crown ≠ ◎ first-section
// top), the note's comparison authority (runtimeTraceProjElimChainFirstSectionTop)
// and the RENDERED ◎ fence's first ▸ section head print byte-identical value
// text — the reduced view provably answers "first section head" as the fence
// ships it (the helper's 单值源 claim, now mechanically judged).
func TestRun2FixAFirstSectionTopMatchesOverviewHead(t *testing.T) {
	projection := run2fixaSelfInversionProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	top, ok := runtimeTraceProjElimChainFirstSectionTop(model)
	if !ok {
		t.Fatalf("fixture must resolve a first-section top")
	}
	if !strings.Contains(runtimeTraceProjConclusionLine(projection, model, true), "◎ 按具名节最大") {
		t.Fatalf("fixture must wear the caliber note (the pin guards its comparison)")
	}
	fence := runtimeTraceProjElimOverviewFence(projection, model, true)
	var head string
	for _, l := range strings.Split(fence, "\n") {
		if strings.Contains(l, "最大可消 ") {
			head = l
			break
		}
	}
	if head == "" {
		t.Fatalf("no ▸ section head rendered:\n%s", fence)
	}
	if want := fmt.Sprintf("最大可消 %.3fms", top.row.Node.EffectiveImpactMS); !strings.Contains(head, want) {
		t.Fatalf("◎ first section head must print the authority's value byte-identically:\n head %q\n want %q", head, want)
	}
}
