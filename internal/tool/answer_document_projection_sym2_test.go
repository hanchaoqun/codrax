package tool

// answer_document_projection_sym2_test.go — SYM-2 批 display pins (ledger
// real_trace_campaign_20260705.md §24.17, user ruling 2026-07-08, revising the
// §24.13 裁定一 scope):
//
//   R2  自因族类别词 — runnable family 行2 word = 调度压力候选 (§7.4
//       demand-side vocabulary, replacing 就绪排队候选); D-state rows split
//       off the ⛓ IO family word into D状态候选 (glyph/legend unchanged);
//       IO rows keep IO阻塞候选. The 「(优先级低于RT)」 行2 tail rides the
//       typed engine flag only.
//   R3  witness forms — cmp_78 修后形: the self binder row stays off the
//       board (等待症状族) while the self RUNNABLE row competes, heads the
//       board and is crowned lead (自因族加冕); the self running deficit row
//       crowns with the §20.2 折算,按大核满频 grammar (与非自身行同文法).
//   R4  零缺口自洽 — a deficit==0 self running row participates at eff 0 and
//       therefore never beats a positive candidate: the 「无供给缺口」 G4
//       verdict and the non-crowning are self-consistent by construction.
//
// 突变自查 (recorded): (M1) widening the engine skip arm back to 全量
// subject==target starves TestSYM2FlatSelfRunnableCrownedLead's fixture shape
// at the source (its engine mirror pins live in tracequery); on the display
// side, deleting the board's IsTargetSelfStateRow arm reds the binder control
// here; (M3) re-merging the D-state token family into IOBlock reds
// TestSYM2DStateCategoryWord and the census membership pin.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sym2FlatMixedProjection is the cmp_78_01 修后形 (FLAT): the target's own
// binder wait (等待症状族, tier=target_self_state, eff-max 3.843) plus the
// target's own runnable row (自因可拆解族 — the engine now ranks it
// tier=primary) plus a smaller non-self row.
func sym2FlatMixedProjection() types.TraceCausalProjection {
	selfRunnable := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selfrunnable",
		Subject: "main-6565", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 2.500, CumulativeImpactMS: 2.500, EffectiveImpactMS: 2.500,
		ChainRelevance: "on_chain", Confidence: 0.8,
	}
	return types.TraceCausalProjection{
		WindowStartTs:     3680.819,
		WindowEndTs:       3682.619,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{selfRunnable},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfbinder",
				Subject: "main-6565", Object: "binder_wait", TypeToken: "binder_wait",
				Predicate: "root_cause_target_self_state", Rank: 0,
				Tier:     types.TraceCausalTierTargetSelfState,
				ImpactMS: 3.843, CumulativeImpactMS: 3.843, EffectiveImpactMS: 3.843,
				ChainRelevance: "on_chain", Confidence: 0.8,
			},
			selfRunnable,
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-peer",
				Subject: "LaunchPoolT1-6712", Object: "runnable_wait", TypeToken: "runnable_wait",
				Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
				ImpactMS: 1.200, CumulativeImpactMS: 1.200, EffectiveImpactMS: 1.200,
				ChainRelevance: "on_chain", Confidence: 0.7,
			},
		},
	}
}

func TestSYM2FlatSelfRunnableCrownedLead(t *testing.T) {
	projection := sym2FlatMixedProjection()
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if strings.TrimSpace(model.Target) != "" {
		t.Fatalf("fixture drifted: this pin needs the FLAT render")
	}
	// binder 等对端仍降道: the self binder row keeps its seat + ordinal but
	// never boards; the competing self runnable row DOES board and heads it.
	board := runtimeTraceProjRankBoard(model.TreeRows)
	if len(board) == 0 {
		t.Fatalf("fixture drifted: the board must have members")
	}
	for _, row := range board {
		if row.Node.IsTargetSelfStateRow() {
			t.Fatalf("the wait-symptom self row must stay off the board: %+v", row.Node)
		}
	}
	if board[0].Node.EvidenceID != "e-selfrunnable" {
		t.Fatalf("the competing self runnable row (eff-max among members) must head the board: %+v", board[0].Node)
	}
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.Subject != "main-6565" || lead.EvidenceID != "e-selfrunnable" {
		t.Fatalf("§24.17 加冕: the lead must crown the target's own runnable row, got lane=%d lead=%+v", lane, lead)
	}
	// EVOLUTION RECORD (§29.27.1 徽章跟随席位, 2026-07-11): the crowned self
	// runnable row publishes the contiguous seat #1 and wears ❶; the wait-
	// symptom self row is Rank=0 and its target_self_state tier is an
	// independent defense arm.
	for _, row := range model.TreeRows {
		switch row.Node.EvidenceID {
		case "e-selfrunnable":
			if row.Badge != 1 {
				t.Fatalf("the crowned #1 seat row must wear ❶, got badge %d", row.Badge)
			}
		case "e-selfbinder":
			if row.Badge != 0 {
				t.Fatalf("the wait-symptom self row must never wear a badge (tier defense): %+v", row)
			}
		}
	}
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "**主根因:** main-6565") {
		t.Fatalf("the conclusion must crown the self runnable row (同文法, no special form):\n%s", line)
	}
	if strings.Contains(line, "未定位到链上主根因") || strings.Contains(line, "关注线程自身等待/持锁") {
		t.Fatalf("a crowned self-cause lead must not carry the honest-fallback disclosure:\n%s", line)
	}
}

func TestSYM2SelfRunningDeficitCrownedWithBigCoreFmaxGrammar(t *testing.T) {
	// 自因 running 行加冕形: the engine publishes eff == supply-fold deficit
	// (§20.2) and the crowned row renders the SAME 折算,按大核满频 grammar as
	// any non-self running cause node (与非自身行同文法).
	running := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-selfrunning",
		Subject: "main-6565", Object: "running", TypeToken: "running", StateKind: "running",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 493.000, CumulativeImpactMS: 493.000, EffectiveImpactMS: 6.100,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 6.100, SupplyFoldIdealMS: 486.900,
		SupplyFoldKnownMS: 493.000,
		ChainRelevance:    "on_chain", Confidence: 0.8,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{running},
		OnChainCauses:     []types.TraceCausalProjectionNode{running},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-selfrunning" {
		t.Fatalf("the self running deficit row must crown through the primary lane, got lane=%d lead=%+v", lane, lead)
	}
	row := runtimeTraceProjTreeRow{Node: running, Kind: runtimeTraceProjTreeRowDepthless,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok {
		t.Fatalf("the crowned running row must build the four-line grammar")
	}
	if !strings.Contains(structured.IdentityRow, "算力供给候选") {
		t.Fatalf("行2 category word must be 算力供给候选: %q", structured.IdentityRow)
	}
	if !strings.Contains(structured.Breakdown, "有效归因 6.100ms = running(折算,按大核满频) 6.100ms") {
		t.Fatalf("行3 must speak the §20.2 deficit caliber: %q", structured.Breakdown)
	}
	if len(structured.SubRows) != 1 || !strings.Contains(structured.SubRows[0], "running 原始 493.000ms → 计入 6.100ms(折算,按大核满频)") {
		t.Fatalf("拆解子行 must carry the raw→counted fold: %+v", structured.SubRows)
	}
}

func TestSYM2ZeroDeficitSelfRunningNeverBeatsPositiveCandidate(t *testing.T) {
	// R4 零缺口自洽: the "已按大核满频…无供给缺口" self running row participates
	// with eff 0 (§20.2 authoritative zero) — it sorts below every positive
	// candidate, so it is never crowned; the verdict and the non-crowning are
	// consistent by construction (pin 之).
	zeroRunning := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-zerorunning",
		Subject: "main-6565", Object: "running", TypeToken: "running", StateKind: "running",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 493.000, CumulativeImpactMS: 493.000, EffectiveImpactMS: 0,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 0, SupplyFoldIdealMS: 493.000,
		SupplyFoldKnownMS: 493.000,
		ChainRelevance:    "on_chain", Confidence: 0.8,
	}
	peer := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-peer",
		Subject: "LaunchPoolT1-6712", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_secondary", Rank: 2, Tier: "secondary",
		ImpactMS: 1.500, CumulativeImpactMS: 1.500, EffectiveImpactMS: 1.500,
		ChainRelevance: "on_chain", Confidence: 0.7,
	}
	projection := types.TraceCausalProjection{
		WindowStartTs:     100.000,
		WindowEndTs:       100.600,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{peer},
		OnChainCauses:     []types.TraceCausalProjectionNode{zeroRunning, peer},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	// The zero-deficit row remains lossless context but never boards. Only the
	// positive typed contender owns a rank seat.
	board := runtimeTraceProjRankBoard(model.TreeRows)
	if len(board) != 1 || board[0].Node.EvidenceID != "e-peer" {
		t.Fatalf("only the positive candidate may board; zero-deficit running is context-only: %+v", board)
	}
	lead, lane := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil || lane != runtimeTraceProjLeadLanePrimary || lead.EvidenceID != "e-peer" {
		t.Fatalf("eff=0 参赛值排尾: the positive candidate must win the board head, got lane=%d lead=%+v", lane, lead)
	}
	// The zero row's own G4 verdict stays the affirmative no-deficit form —
	// consistent with its zero participation value.
	clause, _, ok := runtimeTraceProjSupplyFoldClause(zeroRunning, model.WindowMS, true)
	if !ok || !strings.Contains(clause, "已按大核满频(或接近)运行,无供给缺口") {
		t.Fatalf("the zero-deficit verdict must keep the affirmative form: %q", clause)
	}
}

func TestSYM2RunnableBelowRTTailOnIdentityRow(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "main-6565", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 12.000, CumulativeImpactMS: 12.000, EffectiveImpactMS: 12.000,
		ChainRelevance: "on_chain", Confidence: 0.8,
		RunnableBelowRTPreempted: true,
	}
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowDepthless,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok || !strings.HasSuffix(structured.IdentityRow, "(优先级低于RT)") {
		t.Fatalf("行2 must end with the typed below-RT tail: %+v", structured)
	}
	if !strings.Contains(structured.IdentityRow, "调度压力候选") {
		t.Fatalf("the runnable family 行2 word is 调度压力候选 (§24.17/§7.4): %q", structured.IdentityRow)
	}
	en := row
	if structuredEN, ok := runtimeTraceProjCauseStructuredParts(en, false); !ok ||
		!strings.HasSuffix(structuredEN.IdentityRow, " (priority below RT)") {
		t.Fatalf("EN 行2 tail: %+v", structuredEN)
	}
	// Control: without the typed flag the tail never renders (the display
	// never re-derives priorities from prose).
	control := node
	control.RunnableBelowRTPreempted = false
	controlRow := runtimeTraceProjTreeRow{Node: control, Kind: runtimeTraceProjTreeRowDepthless,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if structured, ok := runtimeTraceProjCauseStructuredParts(controlRow, true); !ok ||
		strings.Contains(structured.IdentityRow, "优先级低于RT") {
		t.Fatalf("the tail must ride the typed flag only: %+v", structured)
	}
}

func TestSYM2DStateCategoryWordSplitsOffIOFamily(t *testing.T) {
	// D-state STATE row → D状态候选, glyph stays ⛓ (既有 D-state 语义).
	dstate := types.TraceCausalProjectionNode{
		Subject: "main-6565", Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
		StateKind: "d_state", Rank: 1, Tier: "primary", ChainRelevance: "on_chain",
		ImpactMS: 5.000, CumulativeImpactMS: 5.000, EffectiveImpactMS: 5.000, Confidence: 0.8,
	}
	word, _ := runtimeTraceProjCauseCategoryWord(dstate, runtimeTraceProjTreeRowDepthless, true)
	if word != "D状态候选" {
		t.Fatalf("D-state 行2 word must be D状态候选 (§24.17): %q", word)
	}
	if glyph := runtimeTraceProjStateIcon(dstate, runtimeTraceProjTreeRowDepthless, true, nil); glyph != "⛓" {
		t.Fatalf("the D-state glyph stays ⛓ (glyph=⛓ 既有 D-state 语义): %q", glyph)
	}
	// Stateless D-token row rides the token family to the same word.
	tokenOnly := types.TraceCausalProjectionNode{TypeToken: "d_state_or_io_wait"}
	if word := runtimeTraceProjImpactFormFamilyWord(tokenOnly, true); word != "D状态候选" {
		t.Fatalf("the d_state_or_io_wait token family word: %q", word)
	}
	// Controls: IO stays on the IO family word — state row and token row.
	iowait := dstate
	iowait.StateKind = "io_wait"
	iowait.Object, iowait.TypeToken = "io_wait", "io_wait"
	if word, _ := runtimeTraceProjCauseCategoryWord(iowait, runtimeTraceProjTreeRowDepthless, true); word != "IO阻塞候选" {
		t.Fatalf("io_wait rows keep IO阻塞候选: %q", word)
	}
	ioToken := types.TraceCausalProjectionNode{TypeToken: "io_latency"}
	if form := runtimeTraceProjImpactFormTokenFamily("io_latency"); form != runtimeTraceProjImpactFormIOBlock {
		t.Fatalf("io_latency stays in the ⛓ IO family: %v (%+v)", form, ioToken)
	}
}

func TestSYM2DStateStaysAWaitViewInTheCensus(t *testing.T) {
	// COV census guard: the D-state family split is a WORD split only — a
	// stateless self D-token row must keep its wait-view membership (§24.11
	// C-3 denominator census would otherwise silently shrink).
	row := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "main-6565", TypeToken: "d_state_or_io_wait",
	}, Kind: runtimeTraceProjTreeRowSelf, HasData: true}
	if !runtimeTraceProjTargetSelfWaitViewRow(row) {
		t.Fatalf("a stateless D-state token row stays a wait view (census membership)")
	}
	occupancy := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "main-6565", TypeToken: "running",
	}, Kind: runtimeTraceProjTreeRowSelf, HasData: true}
	if runtimeTraceProjTargetSelfWaitViewRow(occupancy) {
		t.Fatalf("a running token row is occupancy, never a wait view")
	}
}
