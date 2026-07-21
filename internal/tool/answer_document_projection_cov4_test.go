package tool

// answer_document_projection_cov4_test.go — COV-4 批 pins (ledger
// docs/design/real_trace_campaign_20260705.md §29.27 rulings ①/② + §29.27.1,
// user rulings 2026-07-11):
//
//   A (§29.27②) — the four-state coverage account: the focused thread's
//        full-window wall-clock partition (running+runnable+sleep+D-state,
//        io_wait as the typed IO label inside D-state) renders between the
//        window header and the wait-attribution arms, ONLY when the typed
//        partition provably balances the analysis window (Σ四态==窗口恒等式,
//        不平衡拒渲不造数). Witness-isomorphic shape: cust_trace_vc_710
//        (151.382ms window, running 109.270ms dominant, VerifyClass semantic
//        work + supply-converted pointer).
//   B (§7.30 S1 负面先例) — the converted supply figure NEVER joins wall-clock
//        arithmetic: it publishes as 「见 ➌[E#](折算,不计入四态合计)」 and the
//        partition Σ / running residual never include it.
//   C (§29.27.1 ①②) — badge follows the SEAT, TOP-5, single authority: every
//        rendered surface of a TOP-5 seat wears ➊..➎; Rank=0 symptom-demoted
//        rows and data_gap seatless rows never wear one (误伤面双向 pin).
//   D (§29.27.1 ③) — 三面记号一致: tree row head, detail-face seat line and
//        the coverage account's running attribution line inline the SAME
//        badge glyph + [E#] pair from the single token source.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cov4RunningDominantProjection is the cust_trace_vc_710-isomorphic shape
// (§29.27.1 验收: 数值不必逐等但形状/词形必须): a running-dominant frame —
// window 151.382ms, target running 109.270ms — with an on-chain runnable
// seat #1, the target's own class-verification semantic family (seat #3),
// the target's own running supply-fold row (seat #4), and a balanced typed
// TargetStateAccount.
func cov4RunningDominantProjection() types.TraceCausalProjection {
	const winStart, winEnd = 100.000000, 100.151382
	return types.TraceCausalProjection{
		WakeupPath:    []string{"shadowhook-64305", "ease.app-63993"},
		WindowStartTs: winStart,
		WindowEndTs:   winEnd,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject:                "ease.app-63993",
			RunningMS:              109.270,
			RunnableMS:             8.226,
			SleepMS:                28.507,
			DStateMS:               4.039,
			IOWaitMS:               1.340,
			SleepIOWaitMS:          20.000,
			TotalMS:                151.382,
			DeterministicRunningMS: 11.833,
			WindowStartTs:          winStart,
			WindowEndTs:            winEnd,
			EvidenceID:             "obs-account",
		},
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "e-shadowhook",
			Subject: "shadowhook-64305", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
			ImpactMS: 26.392, CumulativeImpactMS: 26.392, EffectiveImpactMS: 26.392,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			Confidence: 0.8, LineStart: 3, LineEnd: 9,
		}},
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "e-verify",
			Subject:  "ease.app-63993",
			SpanName: "VerifyClass demo", SemanticClass: "class_verification",
			Predicate: "trace_semantic_span", Rank: 3, Tier: "deterministic_optimization",
			ImpactMS: 10.394, CumulativeImpactMS: 10.394, EffectiveImpactMS: 10.394,
			ChainRelevance: "on_chain", Confidence: 0.7,
			LineStart: 11, LineEnd: 19,
		}},
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfrunning",
				Subject: "ease.app-63993", Object: "running", TypeToken: "running",
				StateKind: "running", Predicate: "root_cause_secondary", Rank: 4, Tier: "secondary",
				ImpactMS: 109.270, CumulativeImpactMS: 109.270, EffectiveImpactMS: 16.100,
				SupplyFoldComputed: true, SupplyFoldDeficitMS: 16.100, SupplyFoldIdealMS: 93.170,
				SupplyFoldKnownMS: 109.270,
				ChainRelevance:    "on_chain", Causality: "on_wakeup_chain",
				Confidence: 0.7, LineStart: 21, LineEnd: 29,
			},
		},
	}
}

func cov4LeadText(t *testing.T, projection types.TraceCausalProjection, zh bool) (runtimeTraceProjTreeModel, string) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	lang := "zh"
	if !zh {
		lang = "en"
	}
	return model, runtimeTraceProjLeadText(projection, model, lang, zh)
}

// --- A: the four-state account on the running-dominant shape --------------------

func TestCov4FourStateAccountRendersRunningDominantFrame(t *testing.T) {
	projection := cov4RunningDominantProjection()
	model, lead := cov4LeadText(t, projection, true)
	// The partition line: all four states, window-denominator percentages,
	// the io_wait label INSIDE the D-state term, and the Σ==window identity.
	// 复核 A-1: the sleep term carries its own S+iowait refinement label
	// (the G12 §29.13 Harmony IO-wait form) exactly like the D term.
	if !strings.Contains(lead, "- 关注线程全窗四态: running 109.270ms(72%) + runnable 8.226ms(5%) + sleep 28.507ms(19%,其中 IO等待 20.000ms) + D-state 5.379ms(4%,其中 IO等待 1.340ms) = 151.382ms(四态合计=分析窗)。") {
		t.Fatalf("the four-state partition line must render on the balanced running-dominant shape:\n%s", lead)
	}
	// The running attribution line: deterministic work + converted pointer +
	// the ruling-verbatim residual word (自身执行, never 未归因).
	// 复核 E-P3: the ∩running union renders with the family count and the
	// pointer row's own largest value (X beside a Y-valued row never reads
	// contradictory).
	if !strings.Contains(lead, "- running 109.270ms: 确定性工作 11.833ms(共1类,最大 10.394ms 见 ") {
		t.Fatalf("the running line must open with the deterministic-work wall clock + 共N类/最大 disclosure:\n%s", lead)
	}
	if !strings.Contains(lead, "供给折算影响 16.100ms 见") || !strings.Contains(lead, "(折算,不计入四态合计)") {
		t.Fatalf("the converted supply figure must publish as a pointer with the 折算 caliber:\n%s", lead)
	}
	if !strings.Contains(lead, "自身执行(无确定性可优化工作) 97.437ms") {
		t.Fatalf("the running residual must wear the ruling-verbatim word with running−deterministic:\n%s", lead)
	}
	// The legend entry renders exactly when the account does (direction A).
	if !model.Marks.has(runtimeTraceProjMarkFourStateAccount) || !strings.Contains(lead, "- 全窗四态 = ") {
		t.Fatalf("the account's legend entry must render with the account:\n%s", lead)
	}
	// Additive only: the legacy wait-attribution machinery still speaks below
	// (the account never replaces the wait-denominator sentence family).
	if !strings.Contains(lead, "链上已归因") {
		t.Fatalf("the legacy wait-attribution sentence must stay (additive design):\n%s", lead)
	}
	// EN face mirrors the same shape.
	_, en := cov4LeadText(t, projection, false)
	if !strings.Contains(en, "- Focused-thread full-window states: running 109.270ms (72%)") ||
		!strings.Contains(en, "sleep 28.507ms (19%, incl. IO wait 20.000ms)") ||
		!strings.Contains(en, "deterministic work 11.833ms (1 classes, largest 10.394ms, see ") ||
		!strings.Contains(en, "own execution (no deterministic optimizable work) 97.437ms") ||
		!strings.Contains(en, "(converted; not in the four-state total)") {
		t.Fatalf("the EN account face must mirror the zh shape:\n%s", en)
	}
}

// --- A: Σ四态==窗口恒等式 (不平衡拒渲不造数) -----------------------------------

func TestCov4FourStateAccountRefusesImbalance(t *testing.T) {
	balanced := cov4RunningDominantProjection()
	_, control := cov4LeadText(t, balanced, true)
	if !strings.Contains(control, "全窗四态") {
		t.Fatalf("fixture drifted: the balanced control must render the account")
	}
	// Σ < window (unobserved gap — the textup_792 coverage-span shape).
	imbalanced := cov4RunningDominantProjection()
	imbalanced.TargetStateAccount.SleepMS -= 10
	imbalanced.TargetStateAccount.TotalMS -= 10
	_, lead := cov4LeadText(t, imbalanced, true)
	if strings.Contains(lead, "全窗四态") || strings.Contains(lead, "自身执行") {
		t.Fatalf("an imbalanced partition must refuse the whole account (Σ四态≠窗口):\n%s", lead)
	}
	// 复核 B-1 (修前假过): a 0.1ms gap sat inside the retired 0.5ms jitter
	// tolerance and rendered "Σ(四态合计=分析窗)" beside a visibly different
	// window figure — the gate is now the display-precision claim itself.
	tenth := cov4RunningDominantProjection()
	tenth.TargetStateAccount.SleepMS -= 0.1
	tenth.TargetStateAccount.TotalMS -= 0.1
	if _, l := cov4LeadText(t, tenth, true); strings.Contains(l, "全窗四态") {
		t.Fatalf("a 0.1ms partition gap must refuse at display precision (B-1):\n%s", l)
	}
	// The refusal falls back to the legacy coverage face byte-identically.
	noAccount := cov4RunningDominantProjection()
	noAccount.TargetStateAccount = nil
	_, legacy := cov4LeadText(t, noAccount, true)
	if lead != legacy {
		t.Fatalf("the refused-account lead must be byte-identical to the account-less lead:\n--- refused:\n%s\n--- legacy:\n%s", lead, legacy)
	}
}

func TestCov4FourStateAccountRefusesWindowAndSubjectMismatch(t *testing.T) {
	// Window mismatch (defense in depth over the compile admission).
	wrongWindow := cov4RunningDominantProjection()
	wrongWindow.TargetStateAccount.WindowStartTs += 0.010
	wrongWindow.TargetStateAccount.WindowEndTs += 0.010
	if _, lead := cov4LeadText(t, wrongWindow, true); strings.Contains(lead, "全窗四态") {
		t.Fatalf("an account with a different typed window must refuse:\n%s", lead)
	}
	// Foreign subject: the partition of some other thread never renders as
	// the focused thread's account.
	foreign := cov4RunningDominantProjection()
	foreign.TargetStateAccount.Subject = "shadowhook-64305"
	if _, lead := cov4LeadText(t, foreign, true); strings.Contains(lead, "全窗四态") {
		t.Fatalf("a foreign-subject partition must refuse:\n%s", lead)
	}
}

// --- B: 禁折算值进墙钟 (§7.30 S1 负面先例) --------------------------------------

func TestCov4ConvertedNeverInWallClock(t *testing.T) {
	projection := cov4RunningDominantProjection()
	_, lead := cov4LeadText(t, projection, true)
	// The residual is running − deterministic (97.437), NEVER running −
	// deterministic − converted (81.337) and never any mixed figure.
	if strings.Contains(lead, "81.337") {
		t.Fatalf("the converted supply figure must never subtract from the wall-clock residual:\n%s", lead)
	}
	// The partition Σ speaks 151.382 (wall clock); a Σ that swallowed the
	// converted 16.100 would print 167.482 (the S1 四态和 119% disease).
	if strings.Contains(lead, "167.482") {
		t.Fatalf("the converted supply figure must never join the partition Σ:\n%s", lead)
	}
	// The converted figure appears ONLY on the pointer clause.
	line := ""
	for _, l := range strings.Split(lead, "\n") {
		if strings.Contains(l, "16.100") {
			if line != "" {
				t.Fatalf("the converted figure must appear on exactly one account line:\n%s", lead)
			}
			line = l
		}
	}
	if line == "" || !strings.Contains(line, "见") || !strings.Contains(line, "折算,不计入四态合计") {
		t.Fatalf("the converted figure must ride the 见-pointer clause only: %q", line)
	}
}

// --- C: badge follows the seat, TOP-5, negative gates ---------------------------

func TestCov4BadgeNegativeGates(t *testing.T) {
	projection := cov4RunningDominantProjection()
	// A symptom-demoted row (Rank=0 post-G9) and a data_gap seatless row: both
	// render, neither ever wears a badge.
	projection.OnChainCauses = append(projection.OnChainCauses,
		types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-selfsleep",
			Subject: "ease.app-63993", Object: "sleep_wait", TypeToken: "sleep_wait",
			StateKind: "s_sleep", Predicate: "root_cause_target_self_state", Rank: 0,
			Tier:     types.TraceCausalTierTargetSelfState,
			ImpactMS: 28.507, CumulativeImpactMS: 28.507, EffectiveImpactMS: 28.507,
			ChainRelevance: "on_chain", Confidence: 0.7,
		},
		types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-gap",
			Subject: "blind-77", Object: "trace_gap", TypeToken: "trace_gap",
			Predicate: "root_cause_data_gap", Rank: 0, Tier: "data_gap", TraceGapKind: "no_sched_data",
			ChainRelevance: "on_chain", Confidence: 0.4,
		})
	// An on-chain overflow fold roster carrying a stale member Rank: the
	// counted roster is never a seat holder (the badge doc's fold-arm gate —
	// this row makes the 署名 pin actually cover that arm, 复核 C-3-P4).
	projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-fold",
		Subject: "", Object: "runnable_wait", TypeToken: "runnable_wait",
		Predicate: "root_cause_context", Rank: 5, OnChainOverflowFold: true,
		MergedCount: 3, MergedMaxMS: 2.0, ImpactMS: 2.0, EffectiveImpactMS: 2.0,
		ChainRelevance: "on_chain", Confidence: 0.5,
	})
	// A ◇ stanza seat with its channel's own ordinal #1.
	// EVOLUTION RECORD (CLOSE-1 复核 F1, 2026-07-11): the §29.27.1-era
	// assertion ("the stanza surface wears its seat's glyph like every other
	// surface") is REFINED — 佩戴 = 有效持席 (§29.30.1 shared valid-seat
	// gate) and lane-kind legality is a component of seat validity, so the
	// adjacent/background stanza faces hold no valid seat: no glyph, bare
	// 「#N」 chip retained, never in the election (a ➊-wearing stanza row
	// beside a differently-crowned 主根因 was the split the shared gate
	// kills). Tree-face lanes keep their badges.
	// EVOLUTION RECORD (UXR-1 复核 P2-3, 2026-07-11): the former Rank:5 here
	// was the OLD global-space ordinal form — under §29.36.2 the adjacent
	// channel numbers its own contiguous 1..K, and a #5 on a one-member ◇
	// channel is exactly the stale-artifact shape the fail-close drops. The
	// fixture re-forms to the engine-actual channel ordinal #1 (which also
	// sharpens the badge negative gate: ➊ is the most tempting glyph).
	projection.AdjacentCauses = append(projection.AdjacentCauses, types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-adjacent",
		Subject: "animator-777", Object: "trace_span", TypeToken: "trace_span",
		SpanName: "animator", Predicate: "root_cause_tertiary", Rank: 1, Tier: "tertiary",
		ImpactMS: 115.196, CumulativeImpactMS: 115.196, EffectiveImpactMS: 115.196,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		Confidence: 0.6, LineStart: 31, LineEnd: 39,
	})
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	badges := map[string]int{}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Badge > 0 {
				badges[row.Node.EvidenceID] = row.Badge
			}
			switch row.Node.EvidenceID {
			case "e-selfsleep", "e-gap":
				if row.Badge != 0 {
					t.Fatalf("seatless rows (Rank=0 symptom / data_gap) must never wear a badge: %+v", row)
				}
			case "e-fold":
				if row.Badge != 0 {
					t.Fatalf("an overflow fold roster must never wear a badge (fold-arm gate): %+v", row)
				}
			}
		}
	}
	// Positive: every TREE-FACE TOP-5 seat wears its own ordinal's glyph
	// regardless of row shape (chain #1 / semantic ✦ #3 / self-cause running
	// #4). The ◇ stanza channel-#1 row holds NO valid seat post-F1 (lane-kind
	// arm + P2-2(b) channel belt) — no glyph, channel chip retained.
	want := map[string]int{"e-shadowhook": 1, "e-verify": 3, "e-selfrunning": 4}
	for id, seat := range want {
		if badges[id] != seat {
			t.Fatalf("badge for %s: got %d want %d (徽章跟随席位, %v)", id, badges[id], seat, badges)
		}
	}
	if badges["e-adjacent"] != 0 {
		t.Fatalf("F1: the ◇ stanza row must not wear a glyph (lane-kind arm): %v", badges)
	}
	// Fence face: the ◇ row renders without a glyph (no ➊ despite channel #1).
	// EVOLUTION RECORD (UXR-1 §29.36.2, 2026-07-11, supersedes the F1 "bare
	// 「#N」 chip retained" clause): the ◇ seat is the adjacent channel's own
	// ordinal — the detail face reads 邻近影响 #1, never the on-chain
	// 根因排序 word and never a bare #N (禁裸 #N / 通道词面).
	fence := runtimeTraceProjTreeFence(model, true)
	stanzaLine := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "animator-777") {
			stanzaLine = line
		}
	}
	if stanzaLine == "" || strings.Contains(stanzaLine, "➊") {
		t.Fatalf("the ◇ stanza row must render glyph-less on the fence face (F1): %q\n%s", stanzaLine, fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if strings.Contains(detail, "根因排序: #1·置信中") {
		t.Fatalf("the ◇ seat must not wear the on-chain 根因排序 word (§29.36.2):\n%s", detail)
	}
	if !strings.Contains(detail, "邻近影响: #1") {
		t.Fatalf("the ◇ seat must read 邻近影响 #1 on the detail face (通道词面):\n%s", detail)
	}
	// The SELF-ROW SURFACE wears the glyph too (the textup_792 tree-head
	// seats #2/#3 disease): the target's own #4 running row renders ➍ on its
	// tree-head line.
	selfLine := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "自身·") && strings.Contains(line, "running") {
			selfLine = line
		}
	}
	if selfLine == "" || !strings.Contains(selfLine, "➍") {
		t.Fatalf("the self-cause #4 seat row must wear ➍ on its tree-head line: %q\n%s", selfLine, fence)
	}
}

// --- D: 三面记号一致 (tree head / detail face / coverage account) ---------------

func TestCov4TriFaceSeatTokens(t *testing.T) {
	projection := cov4RunningDominantProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	detail := runtimeTraceProjDetailFullText(model, true)
	// Resolve the semantic row's rendered tag (evidence-index assigned).
	var semantic *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.EvidenceID == "e-verify" {
			semantic = &model.TreeRows[i]
		}
	}
	if semantic == nil || semantic.Badge != 3 || strings.TrimSpace(semantic.EvidenceTag) == "" {
		t.Fatalf("fixture drifted: the semantic family row must hold seat #3 with a rendered E# tag: %+v", semantic)
	}
	pair := "➌[" + semantic.EvidenceTag + "]"
	// Face 1 — the tree row head wears ➌ (fence).
	if !strings.Contains(fence, "➌") {
		t.Fatalf("the tree face must wear the seat badge:\n%s", fence)
	}
	// Face 2 — the detail seat line inlines the same glyph via the single
	// token source (根因排序: ➌ #3; A2 件10(a): one post-badge space).
	if !strings.Contains(detail, "根因排序: ➌ #3") {
		t.Fatalf("the detail face must inline the seat glyph (➌ #3):\n%s", detail)
	}
	// Face 3 — the coverage account's running line points with the SAME
	// glyph+E# pair (inside the 复核 E-P3 共N类/最大 disclosure form).
	if !strings.Contains(lead, "最大 10.394ms 见 "+pair) {
		t.Fatalf("the coverage account must inline the same ➌[E#] pair (三面记号一致), want %q:\n%s", pair, lead)
	}
	// The supply pointer carries its own seat's glyph (➍ + its E#). The
	// self-cause running row renders in the target's self area on the tree
	// shape (SYM-2 §24.17 lane) — the pointer resolver reads it there too.
	var supply *runtimeTraceProjTreeRow
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows} {
		for i := range rows {
			if rows[i].Node.EvidenceID == "e-selfrunning" {
				supply = &rows[i]
			}
		}
	}
	if supply == nil || supply.Badge != 4 || strings.TrimSpace(supply.EvidenceTag) == "" {
		t.Fatalf("fixture drifted: the supply row must hold seat #4 with a rendered E# tag: %+v", supply)
	}
	if !strings.Contains(lead, "见 ➍["+supply.EvidenceTag+"]") {
		t.Fatalf("the supply pointer must inline its own seat glyph+E#:\n%s", lead)
	}
}

// --- regression: account-less wait-dominant shapes stay byte-stable -------------

func TestCov4AccountAbsentLeavesCoverageUntouched(t *testing.T) {
	// The textup_792 regression baseline: a wait-dominant shape with NO
	// balanced account (either no record or an unprovable partition) keeps
	// the existing coverage-sentence family verbatim — no 全窗四态 wording
	// anywhere.
	projection := cov4RunningDominantProjection()
	projection.TargetStateAccount = nil
	_, lead := cov4LeadText(t, projection, true)
	for _, banned := range []string{"全窗四态", "自身执行", "确定性工作", "供给折算影响", "四态合计"} {
		if strings.Contains(lead, banned) {
			t.Fatalf("account-less shapes must not speak any account wording (%q):\n%s", banned, lead)
		}
	}
}

// --- C-1: seat token passes the SAME negative gate (双面一致) -------------------

func TestCov4SeatTokenGatedWithBadge(t *testing.T) {
	// The SYM2 stale-Rank shape: a wait-symptom self row carrying a stale
	// engine Rank=1 (tier=target_self_state). The tree face is bare by the
	// single badge authority — the detail face must be bare too (复核 C-1:
	// the first cut minted 「➊#1」 from the raw rank, splitting the faces).
	projection := sym2FlatMixedProjection()
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].EvidenceID == "e-selfbinder" {
			projection.OnChainCauses[i].Rank = 1 // deliberate stale-rank mutation
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, row := range model.TreeRows {
		if row.Node.EvidenceID == "e-selfbinder" && row.Badge != 0 {
			t.Fatalf("fixture drifted: the stale-Rank symptom row must stay bare on the tree face: %+v", row)
		}
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	binderStart := strings.Index(detail, "**main-6565 / binder等待**")
	if binderStart < 0 {
		t.Fatalf("fixture drifted: missing binder detail block:\n%s", detail)
	}
	binderEnd := strings.Index(detail[binderStart+2:], "\n**")
	if binderEnd < 0 {
		binderEnd = len(detail) - binderStart
	} else {
		binderEnd += 2
	}
	binderBlock := detail[binderStart : binderStart+binderEnd]
	if strings.Contains(binderBlock, "➊ #1") {
		t.Fatalf("the symptom detail block must not mint a ➊ the tree face refused (双面一致):\n%s", binderBlock)
	}
	// The bare pre-batch chip form stays for the gated seat text (词面先例).
	if !strings.Contains(binderBlock, "#1") {
		t.Fatalf("the gated seat keeps its bare ordinal chip:\n%s", binderBlock)
	}
	// Positive control: an ungated seat still wears its glyph on the detail
	// face (the single token source stays live).
	if !strings.Contains(detail, "➋ #2") {
		t.Fatalf("ungated seats keep the glyph form on the detail face:\n%s", detail)
	}
}
