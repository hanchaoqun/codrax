package tool

// answer_document_projection_ptv4_test.go — PTV4 (#65) pins: the projection
// tree/detail/legend re-layout ruled 2026-07-05.
//
//	T1 — main-row essentials (edge + state glyph + FULL name + bar + ms + % +
//	     the ⚠/⊘/[E#] Keep marks) never truncate and never move down; every
//	     other tag demotes WHOLE to "· " subordinate detail lines (the FitTags
//	     DropOrder elision lane is retired — no information is "…"-ed away).
//	T2 — name truncation keeps the identity-bearing -pid tail (middle cut);
//	     names without a pid tail keep the legacy tail cut.
//	T3 — subordinate/wrap lines break at word boundaries only; tokens like
//	     14.597ms never split (pinned in the NEW-10 file's wrap pins).
//	T6 — ❶❷❸ TOP-N badges come from the typed engine rank + effective
//	     attribution ONLY (mutation: clearing Rank kills the badge).
//	T7 — 防回潮: a legend-cataloged mark may never regrow a parenthesized
//	     in-row explanation inside the fence.
//	T10 — detail surface is two-part: (a) key-metric table capped at 6
//	     columns; (b) per-node vertical blocks with NO name caps.
//	T11 — the evidence-index grouped intro shows the artifact BASENAME, never
//	     a machine-local blob absolute path (RTC f1 leak shape).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// --- T1: main-row essentials never truncate / never move down ------------------

func TestPTV4MainRowEssentialsSurviveOversizedTags(t *testing.T) {
	// Mutation shape: stuff the row with an oversized demotable tag (occupier
	// roster) — the main row must NOT truncate the name or drop the ⚠/⊘/[E#]
	// marks; the oversized tag must land WHOLE on subordinate lines.
	roster := strings.Repeat("VeryBusyOccupierThread-1234:99.000ms、", 6) + "tail-1:1.000ms"
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "CookieMonsterCl-59843", Object: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain",
			ImpactMS: 25.847, CumulativeImpactMS: 25.847,
			ActualImpactMS: 90.000, UndrillableReason: "missing_wakeup",
			OccupierSummary: roster,
		},
		Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
		Depth: 1, HasData: true, EvidenceTag: "E4",
		// CR-2 组③ P7 (2026-07-12): ⚠ now requires the typed interval verdict —
		// stamped as the model build would for a proven crossing (this pin
		// guards MainRow layout mechanics, not the verdict).
		ActualScope: runtimeTraceProjActualScopeAnalysisWindow,
	}
	line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 114.940, true, true)
	lines := strings.Split(line, "\n")
	if len(lines) < 2 {
		t.Fatalf("oversized tags must force the subordinate split:\n%s", line)
	}
	main := lines[0]
	// The full name stays on the main line, untruncated, with bar+ms+%.
	for _, want := range []string{"CookieMonsterCl-59843", "25.847ms", "%", "█"} {
		if !strings.Contains(main, want) {
			t.Fatalf("main row lost essential %q:\n%s", want, line)
		}
	}
	// Keep marks (⚠实际Xms / ⊘链止 / [E#]) stay on the MAIN line.
	for _, want := range []string{"⚠实际90.000ms", "⊘链止", "[E4]"} {
		if !strings.Contains(main, want) {
			t.Fatalf("Keep mark %q moved off the main row:\n%s", want, line)
		}
	}
	// The oversized tag survives WHOLE across subordinate lines (no elision).
	squash := strings.NewReplacer("\n", "", "│", "", " ", "", "·", "").Replace(line)
	if !strings.Contains(squash, strings.NewReplacer(" ", "", "·", "").Replace(roster)) {
		t.Fatalf("demoted tag lost content:\n%s", line)
	}
	if strings.Contains(line, " · … · ") {
		t.Fatalf("the retired elision marker reappeared:\n%s", line)
	}
	// Every physical line holds the row cap.
	for _, l := range lines {
		if w := runewidth.StringWidth(l); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("line over the %d-cell cap (%d):\n%s", runtimeTraceProjTreeRowMaxWidth, w, l)
		}
	}
}

// --- T2: pid-tail-preserving middle truncation ----------------------------------

func TestPTV4NameMidTruncationKeepsPidTail(t *testing.T) {
	// A pid-tailed name truncates in the MIDDLE — the tail survives whole.
	got := runtimeTraceProjTruncateName("CookieMonsterClAAAAAAAA-59843", 16)
	if !strings.Contains(got, "…") || !strings.HasSuffix(got, "-59843") {
		t.Fatalf("pid tail must survive the middle cut: %q", got)
	}
	if w := runewidth.StringWidth(got); w > 16 {
		t.Fatalf("truncated name over budget (%d): %q", w, got)
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 词中截断残词
	// (s_s… 形态) → 词边界截断+整词让位 (composed-name suffix 词边界族).
	// A composed name keeps the whole subject when it fits and tail-truncates
	// the cause suffix at a WORD boundary instead.
	composed := runtimeTraceProjTruncateName("dep-200 · 持锁阻塞(等待方 WebViewLoaderThr-1234)", 24)
	if !strings.HasPrefix(composed, "dep-200 · 持锁阻塞") || !strings.HasSuffix(composed, "…") {
		t.Fatalf("composed name must keep the subject and boundary-cut the cause: %q", composed)
	}
	// A boundary-less cause suffix yields its seat whole (整词让位) — the
	// subject stays, no mid-word residue survives.
	if yielded := runtimeTraceProjTruncateName("dep-200 · priority_inversion_candidate", 32); yielded != "dep-200" {
		t.Fatalf("a boundary-less cause suffix must yield whole and keep the subject: %q", yielded)
	}
	// When the suffix leaves no room, the pid-tailed subject wins whole-row.
	squeezed := runtimeTraceProjTruncateName("ThreadPoolForeg-60555 · IO等待", 14)
	if !strings.HasSuffix(squeezed, "-60555") {
		t.Fatalf("identity tail must win under a squeezed budget: %q", squeezed)
	}
	// No pid tail → legacy tail truncation.
	plain := runtimeTraceProjTruncateName("priority_inversion_candidate", 12)
	if !strings.HasSuffix(plain, "…") || runewidth.StringWidth(plain) > 12 {
		t.Fatalf("tail-less names keep the legacy tail cut: %q", plain)
	}
	// 复核 off-by-one 边界: budget == width("…"+tail) is the exact-fit form —
	// it must stay in the pid-keeping lane ("…-59843"), not fall to the tail
	// cut; one cell less legitimately falls through.
	if got := runtimeTraceProjTruncateName("CookieMonsterCl-59843", 7); got != "…-59843" {
		t.Fatalf("exact-fit …+pid budget must keep the tail: %q", got)
	}
	if got := runtimeTraceProjTruncateName("CookieMonsterCl-59843", 6); strings.HasSuffix(got, "-59843") {
		t.Fatalf("a budget below …+tail cannot fake the pid lane: %q", got)
	}
}

// --- Med-1: reserve-aware budget on EVERY main-line form -------------------------

// TestPTV4RecursRowHoldsCapAcrossBoundary (复核 Med-1 突变面): a RecursOnChain
// row近贴界 must hold the 100-cell cap AFTER the ↺ suffix appends, on BOTH the
// single-line form and the demote (subordinate-split) form — the review found
// 88/440 demote-form rows at 101-102 cells because the demote lane compared
// against the bare cap. Sweeps name widths across the boundary so the exact
// trigger offset cannot dodge the pin.
func TestPTV4RecursRowHoldsCapAcrossBoundary(t *testing.T) {
	for pad := 0; pad <= 30; pad++ {
		name := "W" + strings.Repeat("x", pad) + "-59843"
		for _, oversized := range []bool{false, true} {
			row := runtimeTraceProjTreeRow{
				Node: types.TraceCausalProjectionNode{
					Subject: name, Object: "runnable_wait",
					StateKind: "runnable", ChainRelevance: "on_chain",
					ImpactMS: 25.847, CumulativeImpactMS: 25.847,
				},
				Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeWake,
				Depth: 1, HasData: true, EvidenceTag: "E4", RecursOnChain: true,
			}
			if oversized {
				// Forces the demote lane (the Med-1 escape path).
				row.Node.OccupierSummary = strings.Repeat("BusyThread-1234:99.000ms、", 5) + "t-1:1.000ms"
			}
			for _, zh := range []bool{true, false} {
				line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 114.940, true, zh)
				main := strings.Split(line, "\n")[0]
				if !strings.Contains(main, "↺") {
					t.Fatalf("pad=%d oversized=%v zh=%v: recurs row lost its ↺:\n%s", pad, oversized, zh, line)
				}
				if w := runewidth.StringWidth(main); w > runtimeTraceProjTreeRowMaxWidth {
					t.Fatalf("pad=%d oversized=%v zh=%v: recurs main line over the cap (%d):\n%s",
						pad, oversized, zh, w, line)
				}
			}
		}
	}
}

// --- T6: TOP-N badges from typed rank + effective attribution only --------------

func ptv4BadgeModel(rank int) runtimeTraceProjTreeModel {
	node := func(id, subject string, r int, effective float64) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Node: types.TraceCausalProjectionNode{
				EvidenceID: id, Subject: subject, Object: "runnable_wait",
				StateKind: "runnable", ChainRelevance: "on_chain",
				Rank: r, ImpactMS: effective, CumulativeImpactMS: effective,
				EffectiveImpactMS: effective,
			},
			Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
		}
	}
	return runtimeTraceProjTreeModel{
		Target:   "app-1",
		WindowMS: 100,
		TreeRows: []runtimeTraceProjTreeRow{
			node("a", "worker-1", rank, 30.0),
			node("b", "worker-2", 2, 20.0),
			node("c", "worker-3", 3, 40.0),
			node("d", "worker-4", 4, 10.0),
		},
	}
}

func TestPTV4TopBadgesFollowTypedRankAndAttribution(t *testing.T) {
	model := ptv4BadgeModel(1)
	runtimeTraceProjAssignTopBadges(&model)
	// EVOLUTION RECORD (§29.27.1 徽章跟随席位, 2026-07-11): the badge is the
	// pictograph of each row's PUBLISHED seat ordinal (TOP-5), no longer a
	// display re-sort by effective attribution — the engine already orders
	// seats by published eff (§29.22.1), and the display re-sort let ❷ land
	// on seat #4 (vc_710 witness). worker-4 (seat #4) now wears ❹.
	got := map[string]int{}
	for _, row := range model.TreeRows {
		got[row.Node.Subject] = row.Badge
	}
	want := map[string]int{"worker-1": 1, "worker-2": 2, "worker-3": 3, "worker-4": 4}
	for subject, badge := range want {
		if got[subject] != badge {
			t.Fatalf("badge for %s: got %d want %d (%v)", subject, got[subject], badge, got)
		}
	}
	// TOP-5 bound: a seat past #5 never wears a badge.
	past := ptv4BadgeModel(6)
	runtimeTraceProjAssignTopBadges(&past)
	for _, row := range past.TreeRows {
		if row.Node.Subject == "worker-1" && row.Badge != 0 {
			t.Fatalf("seat #6 must not wear a badge (TOP-5 bound): %+v", row.Badge)
		}
	}
	// MUTATION (rank 断连必红): clearing the typed Rank kills the badge — no
	// prose/impact fallback may re-mint it.
	unranked := ptv4BadgeModel(1)
	for i := range unranked.TreeRows {
		unranked.TreeRows[i].Node.Rank = 0
	}
	runtimeTraceProjAssignTopBadges(&unranked)
	for _, row := range unranked.TreeRows {
		if row.Badge != 0 {
			t.Fatalf("rankless rows must never carry a badge: %+v", row.Node.Subject)
		}
	}
	// Strict effective matrix: a stale Rank paired with zero authoritative
	// attribution is not a valid seat and must stay bare on every surface.
	zeroEff := ptv4BadgeModel(1)
	for i := range zeroEff.TreeRows {
		zeroEff.TreeRows[i].Node.EffectiveImpactMS = 0
	}
	runtimeTraceProjAssignTopBadges(&zeroEff)
	for _, row := range zeroEff.TreeRows {
		if row.Badge != 0 {
			t.Fatalf("zero-effective stale ranks must never carry a badge: %+v", row.Node.Subject)
		}
	}
	// The badge is an independent token, never a state glyph: the label parts
	// keep exactly one state icon beside it.
	badged := ptv4BadgeModel(1)
	runtimeTraceProjAssignTopBadges(&badged)
	for i := range badged.TreeRows {
		fixed, _ := runtimeTraceProjTreeLabelParts(badged.TreeRows[i], true)
		if strings.Count(fixed, "⧖") != 1 {
			t.Fatalf("badge must not disturb the one-state-glyph slot: %q", fixed)
		}
	}
}

// --- T7: 防括号解释回潮 ---------------------------------------------------------

// TestPTV4FenceNeverRegrowsInlineExplanations scans representative fences for
// the retired in-row parenthesized explanations of legend-cataloged marks —
// any regrowth is a red (the legend owns the semantics, the row owns the
// data).
func TestPTV4FenceNeverRegrowsInlineExplanations(t *testing.T) {
	banned := []string{
		"↺(",                     // T4: bare ↺, explanation in the legend
		"⚠跨窗",                    // T4: ⚠实际Xms carries the data, 跨窗 semantics in the legend
		"(链路中转",                  // T4: transit rows carry the 2-word 中转 token
		"链路中转,本轮无独立影响行",          // full retired phrase
		"同值合并(重复发布)",             // T4 ×N 三式: bare N次同值
		"(取最大值,不求和)",             // T4 ×N 三式: bare N线程取最大(单项a~b)
		"整窗等待(疑似空闲)",             // T4: bare 整窗等待 in the fence (full text on (b))
		"(承自等待区间,非本行实测)",         // T4: bare 承自归因Xms
		"↺ (recurs on chain)",    // en twin
		"完整链路见原始 trace_query 记录", // T4: moved to the legend's 省略行 entry
	}
	for _, fx := range new10Fixtures() {
		for _, zh := range []bool{true, false} {
			model := buildRuntimeTraceProjTreeModel(fx.proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			for _, pattern := range banned {
				if strings.Contains(fence, pattern) {
					t.Fatalf("%s zh=%v: retired in-row explanation regrew (%q):\n%s", fx.name, zh, pattern, fence)
				}
			}
		}
	}
}

// --- T10: (a) column cap + (b) uncapped full names -------------------------------

func TestPTV4DetailTableColumnCapAndUncappedFullNames(t *testing.T) {
	longName := "com.example.extremely.long.package.name.subsystem.MainThreadRenderWorkerImpl-4259178"
	model := runtimeTraceProjTreeModel{
		Target:   "app-1",
		WindowMS: 100,
		TreeRows: []runtimeTraceProjTreeRow{{
			Node: types.TraceCausalProjectionNode{
				EvidenceID: "E1", Subject: longName, Object: "runnable_wait",
				StateKind: "runnable", ChainRelevance: "on_chain",
				ImpactMS: 10, CumulativeImpactMS: 10,
			},
			Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true, EvidenceTag: "E1",
		}},
	}
	for _, zh := range []bool{true, false} {
		columns, rows := runtimeTraceProjDetailTable(model, zh)
		if len(columns) > 6 {
			t.Fatalf("T10 (a): the key-metric table is capped at 6 columns, got %d: %v", len(columns), columns)
		}
		if len(rows) != 1 || len(rows[0].Cells) != len(columns) {
			t.Fatalf("row/column arity mismatch: %v vs %v", rows, columns)
		}
		// (b): the FULL name renders verbatim — the 28/22/36-rune caps are
		// withdrawn on this surface.
		full := runtimeTraceProjDetailFullText(model, zh)
		if !strings.Contains(full, longName) {
			t.Fatalf("T10 (b): full name must render uncapped:\n%s", full)
		}
	}
}

// --- 复核: (a) dual-seat dedupe -----------------------------------------------

// TestPTV4DetailTableDualSeatDedupe (复核): one node classified into two
// display lanes (semantic ✦ + adjacent ◇, same typed EvidenceID) renders ONE
// (a) key-metric row with an explicit seat note — never two byte-identical
// rows a reader would sum. Evidence-less rows with colliding composite keys
// stay SEPARATE (fail open — the typed id is the only dedupe identity).
func TestPTV4DetailTableDualSeatDedupe(t *testing.T) {
	span := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "E-span",
			Subject: "T7@ZeusThreadPo-61839", Object: "class_verification",
			SpanName: "VerifyClass com.example.Foo", ImpactMS: 0.285, CumulativeImpactMS: 0.285,
		},
		Kind: runtimeTraceProjTreeRowSemantic, HasData: true, EvidenceTag: "E5",
	}
	adj := span
	adj.Kind = runtimeTraceProjTreeRowAdjacent
	model := runtimeTraceProjTreeModel{
		WindowMS: 100,
		TreeRows: []runtimeTraceProjTreeRow{span},
		Adjacent: []runtimeTraceProjTreeRow{adj},
	}
	_, rows := runtimeTraceProjDetailTable(model, true)
	if len(rows) != 1 {
		t.Fatalf("dual-seat node must render one (a) row: %+v", rows)
	}
	if !strings.Contains(rows[0].Cells[0], "✦/◇双席") {
		t.Fatalf("the surviving row must carry the seat note: %q", rows[0].Cells[0])
	}
	// (b) keeps BOTH stanzas — the lossless surface never folds.
	if full := runtimeTraceProjDetailFullText(model, true); strings.Count(full, "**[E5] ") != 2 {
		t.Fatalf("(b) blocks must keep both seats' stanzas:\n%s", full)
	}
	// Evidence-less rows never dedupe on the composite fallback key.
	bare := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "hog-9", Object: "unknown-thread", StateKind: "running",
			ChainRelevance: "background", ImpactMS: 101.0,
		},
		Kind: runtimeTraceProjTreeRowBackground, HasData: true,
	}
	bareSleep := bare
	bareSleep.Node.StateKind = "s_sleep"
	bareModel := runtimeTraceProjTreeModel{
		WindowMS:   101.0,
		Background: []runtimeTraceProjTreeRow{bare, bareSleep},
	}
	if _, bareRows := runtimeTraceProjDetailTable(bareModel, true); len(bareRows) != 2 {
		t.Fatalf("evidence-less composite-key collisions must stay two rows: %+v", bareRows)
	}
}

// --- T11: evidence grouped intro shows the basename, never a blob path ----------

func TestPTV4EvidenceGroupedIntroBasenamesBlobPath(t *testing.T) {
	const blob = "/Users/han/opt/claude/codrax/.codrax/blob/20260705-182011-000-83744/attached_trace.txt"
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	for i, id := range []string{"n1", "n2"} {
		node := types.TraceCausalProjectionNode{
			EvidenceID: id, Subject: "worker-1", Object: "runnable_wait",
			ImpactMS:  10 + float64(i),
			LineStart: 100 + i, LineEnd: 200 + i,
			SupportRefs: []string{blob + ":" + strings.Repeat("1", 3)},
		}
		evidence.add(node, true)
	}
	intro, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
	if len(items) != 2 {
		t.Fatalf("expected both entries: %+v", items)
	}
	// f1 specimen shape: the machine-local blob absolute path must never leak
	// into the grouped intro — the basename replaces it.
	if strings.Contains(intro, ".codrax/blob") || strings.Contains(intro, "/Users/") {
		t.Fatalf("grouped intro leaks the blob absolute path:\n%s", intro)
	}
	if !strings.Contains(intro, "全部证据位于 `attached_trace.txt`") {
		t.Fatalf("grouped intro must show the artifact basename:\n%s", intro)
	}
}
