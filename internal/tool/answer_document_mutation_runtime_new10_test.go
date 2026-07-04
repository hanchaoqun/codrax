package tool

// NEW-10 (§7.6 对比场景客户回访 2026-07-04, docs/design/
// customer_dead_session_audit_20260703.md): markdown/HTML viewer robustness of
// the projection-tree fence. Root cause of the reported drift: web monospace
// fonts render CJK at ≈1.6-1.8× and emoji at unstable widths, and several
// viewers pre-wrap long lines. Adjudication: NO per-surface second content
// render (v3 three-surface byte-identity red line); instead
//   1. row display width hard-capped at 100 cells (单行完整性优先) — enforced
//      by the B4 drop lane + the NEW-10 keep-truncation lane. F-2 (§7.7
//      回访聚焦复核 2026-07-04): over-wide NoTruncate carriers (the NEW-3
//      caliber note / D3 composition split) no longer overflow the row — they
//      wrap byte-identically onto prefix-aligned ↳ continuation lines, so
//      EVERY fence line holds the cap;
//   2. 记号区规整 — every rendered node row carries EXACTLY one state glyph
//      (typed StateKind switch, ◦ default), so same-depth rows keep a constant
//      glyph count and web font drift collapses to a constant offset;
//   3. no tab characters anywhere in the fence;
//   4. F-3 (§7.7 回访聚焦复核 2026-07-04): the shared label column caps at
//      runtimeTraceProjTreeLabelColumnMax — one deep/long-CJK row (or an
//      over-wide 🎯 header) can no longer lift the whole fence past the cap.
// The detail table (native markdown) remains the lossless fallback surface.
//
// The pins below run on the NEW-7 representative fixture set (the same shapes
// the bidirectional legend pin exercises), so the two width faces and the
// legend face can never drift apart on different fixtures.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

type new10Fixture struct {
	name string
	proj types.TraceCausalProjection
}

// new10Fixtures is the NEW-7 representative shape set, reused verbatim so the
// width/glyph/tab pins and the bidirectional legend pin always run on the same
// fixtures.
func new10Fixtures() []new10Fixture {
	return []new10Fixture{
		{"revisit_6_0_io_fold", revisit76IOProjection()},
		{"revisit_6_0_residual_own_edge", revisit76ResidualProjection()},
		{"revisit_7_0_rich_chain", revisit76RichChainProjection()},
		{"flat_undrillable", revisit76FlatUndrillableProjection()},
		{"berlin_cause_adjacent_background", revisit76BerlinLayersProjection()},
	}
}

// TestTraceProjectionNew10WidthConstantsPinned pins the NEW-10 budget numbers
// structurally: re-widening either constant re-opens the customer-reported
// viewer wrap and must be a deliberate ledger adjudication, not a drive-by.
func TestTraceProjectionNew10WidthConstantsPinned(t *testing.T) {
	if runtimeTraceProjTreeRowMaxWidth != 100 {
		t.Fatalf("NEW-10 row cap must stay 100 display cells, got %d", runtimeTraceProjTreeRowMaxWidth)
	}
	if runtimeTraceProjTreeLabelWidth != 44 {
		t.Fatalf("NEW-10 label base must stay 44 display cells, got %d", runtimeTraceProjTreeLabelWidth)
	}
	// F-3: the label-column cap is DERIVED (row cap minus the minimal
	// metric+stub area of a fully shaved data row) — 50 with the pinned
	// constants above. A drive-by change to any input constant moves it and
	// must be a deliberate ledger adjudication.
	if runtimeTraceProjTreeLabelColumnMax != 50 {
		t.Fatalf("F-3 label-column cap must derive to 50 display cells, got %d", runtimeTraceProjTreeLabelColumnMax)
	}
}

// TestTraceProjectionNew10FenceLineWidthCap (as rewritten by F-2, §7.7
// 回访聚焦复核 2026-07-04): EVERY fence line renders within the 100-cell display
// budget (runewidth-measured, so CJK/emoji count their terminal cells) — the
// former carrier-row exemption is gone, because over-wide NoTruncate carriers
// now wrap onto ↳ continuation lines instead of overflowing. Continuation
// content integrity: for every typed carrier row (census from the MODEL, never
// guessed from text) the full caliber-note text must survive in the fence
// modulo the wrap decorations (rails, ↳, inserted newlines/indent). On the en
// surface the flat-fallback header sentence (§7.30 裁定3 pinned wording, no
// CJK-drift exposure) remains the single exemption.
func TestTraceProjectionNew10FenceLineWidthCap(t *testing.T) {
	squash := func(s string) string {
		return strings.NewReplacer("\n", "", "│", "", "↳", "", " ", "").Replace(s)
	}
	fixtures := new10Fixtures()
	for _, fx := range fixtures {
		for _, zh := range []bool{true, false} {
			name := fx.name + "/zh"
			if !zh {
				name = fx.name + "/en"
			}
			model := buildRuntimeTraceProjTreeModel(fx.proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			fence := runtimeTraceProjTreeFence(model, zh)
			for _, line := range strings.Split(fence, "\n") {
				w := runewidth.StringWidth(line)
				if w <= runtimeTraceProjTreeRowMaxWidth {
					continue
				}
				if !zh && strings.TrimSpace(model.Target) == "" && strings.HasPrefix(line, "(") {
					// en flat-fallback header sentence (pinned §7.30 裁定3
					// wording) — the scale note already wraps off it.
					continue
				}
				t.Fatalf("%s: line over the %d-cell cap (%d cells):\n%s\nfence:\n%s",
					name, runtimeTraceProjTreeRowMaxWidth, w, line, fence)
			}
			// F-2 integrity: every carrier's note text survives, wrapped or not.
			squashedFence := squash(fence)
			for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
				for _, row := range rows {
					if len(row.IOFoldPeers) == 0 {
						continue
					}
					note := runtimeTraceProjIOFoldNoteText(row.IOFoldPeers, zh)
					if !strings.Contains(squashedFence, squash(note)) {
						t.Fatalf("%s: carrier note lost content across the continuation wrap:\nnote: %s\nfence:\n%s",
							name, note, fence)
					}
				}
			}
		}
	}
}

// TestTraceProjectionNew10CaliberNoteWrapsToContinuationLine pins the F-2 lane
// on the real revisit shape: the 3-peer caliber note cannot fit its primary
// row, so the fence carries it on a ↳ continuation line and the main row
// (232.428ms primary) holds the cap.
func TestTraceProjectionNew10CaliberNoteWrapsToContinuationLine(t *testing.T) {
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(revisit76IOProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		marker := "↳ 同段IO另有"
		if !zh {
			marker = "↳ same-segment IO also measured"
		}
		if !strings.Contains(fence, marker) {
			t.Fatalf("zh=%v: over-wide caliber note must move to a ↳ continuation line:\n%s", zh, fence)
		}
		for _, line := range strings.Split(fence, "\n") {
			if !strings.Contains(line, "232.428") {
				continue
			}
			if strings.Contains(line, "同段IO另有") || strings.Contains(line, "same-segment IO also measured") {
				t.Fatalf("zh=%v: the main carrier row must no longer hold the over-wide note inline:\n%s", zh, line)
			}
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("zh=%v: main carrier row still over the cap (%d cells):\n%s", zh, w, line)
			}
		}
	}
}

// TestTraceProjectionNoteContinuationLinesIntegrity is the F-2 wrap-helper
// unit pin: every produced line holds the row cap, the first line carries the
// "↳ " marker, later lines align under it, and the chunk concatenation is
// byte-identical to the input (wrap only — never truncation).
func TestTraceProjectionNoteContinuationLinesIntegrity(t *testing.T) {
	note := strings.Repeat("同段IO另有 io_burst_episode 226.153ms、io_wait 112.011/107.672ms 口径;", 3) + "证据 E2、E3、E4"
	indent := "│   │ "
	lines := runtimeTraceProjNoteContinuationLines(indent, note)
	if len(lines) < 2 {
		t.Fatalf("a %d-cell note must wrap across multiple continuation lines: %q", runewidth.StringWidth(note), lines)
	}
	var joined strings.Builder
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("continuation line %d over the row cap (%d cells): %q", i, w, line)
		}
		prefix := indent + "  "
		if i == 0 {
			prefix = indent + "↳ "
		}
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("continuation line %d must carry the aligned prefix %q: %q", i, prefix, line)
		}
		joined.WriteString(strings.TrimPrefix(line, prefix))
	}
	if joined.String() != note {
		t.Fatalf("continuation chunks must concatenate byte-identically to the note:\nwant: %s\ngot:  %s", note, joined.String())
	}
}

// TestTraceProjectionNew10LabelColumnCapDeepChainLongCJKNames is the F-3 pin
// on the review's counter-shape (≥20-cell CJK process names on a depth-8
// trunk + a 🎯 header wider than the cap): pre-cap, the depth-fixed prefix +
// the 20-cell name floor lifted the SHARED column to ~59 and the header to 57,
// putting the whole fence 105-118 cells wide. Now the column caps at
// runtimeTraceProjTreeLabelColumnMax, deep names truncate further (B1
// semantics, detail table lossless), the 🎯 header renders in full (never
// truncated), and every fence line holds the row cap.
func TestTraceProjectionNew10LabelColumnCapDeepChainLongCJKNames(t *testing.T) {
	target := "应用主进程界面渲染合成器工作线程-215380"
	trunk := []string{
		"后台数据同步服务工作线程-1001",
		"多媒体解码服务工作线程-1002",
		"网络请求调度器工作线程-1003",
		"数据库事务提交工作线程-1004",
		"图片缓存清理服务线程-1005",
		"消息队列分发器工作线程-1006",
		"传感器数据采集工作线程-1007",
		"电源管理策略服务线程-1008",
	}
	// WakeupPath is stored root-first with the target LAST; trunk depth 1..8
	// walks it back-to-front.
	path := append([]string(nil), trunk...)
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	path = append(path, target)
	projection := types.TraceCausalProjection{
		WakeupPath:    path,
		WindowStartTs: 100.0,
		WindowEndTs:   100.5,
	}
	for i, subject := range trunk {
		projection.OnChainCauses = append(projection.OnChainCauses, types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "deep-" + subject,
			Subject: subject, Object: "runnable_wait", StateKind: "runnable",
			ChainRelevance: "on_chain", ChainDepth: i + 1,
			ImpactMS: 10.0 + float64(i), CumulativeImpactMS: 10.0 + float64(i),
			LineStart: 100 + i*10, LineEnd: 105 + i*10,
			SupportRefs: []string{"deep.systrace:100-200"},
			Confidence:  0.8,
		})
	}
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		if width := runtimeTraceProjTreeLabelColumn(model, zh); width > runtimeTraceProjTreeLabelColumnMax {
			t.Fatalf("zh=%v: shared label column must cap at %d, got %d", zh, runtimeTraceProjTreeLabelColumnMax, width)
		}
		fence := runtimeTraceProjTreeFence(model, zh)
		// The 🎯 anchor never truncates, even wider than the capped column.
		if !strings.Contains(fence, "🎯 "+target) {
			t.Fatalf("zh=%v: over-wide 🎯 header must render in full:\n%s", zh, fence)
		}
		for _, line := range strings.Split(fence, "\n") {
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("zh=%v: deep-chain fence line over the row cap (%d cells):\n%s\nfence:\n%s",
					zh, w, line, fence)
			}
		}
	}
}

// TestTraceProjectionNew10EveryRowCarriesExactlyOneStateGlyph: 记号区规整 —
// every rendered node row (chain/transit, self, adjacent, background) carries
// EXACTLY one glyph from the typed state-icon enumeration, none blank. The
// scan is typed: rows come from the model, the expected glyph from the same
// runtimeTraceProjStateIcon switch the renderer uses (a new icon return value
// fails the set-membership check and forces extending this pin). Omitted fold
// markers are exempt by design — they are chain summaries, not node rows.
func TestTraceProjectionNew10EveryRowCarriesExactlyOneStateGlyph(t *testing.T) {
	glyphs := []string{"✦", "💤", "⚙", "⏳", "⛓", "◦"}
	countGlyphs := func(line string) int {
		total := 0
		for _, g := range glyphs {
			total += strings.Count(line, g)
		}
		return total
	}
	fixtures := new10Fixtures()
	for _, fx := range fixtures {
		for _, zh := range []bool{true, false} {
			name := fx.name + "/zh"
			if !zh {
				name = fx.name + "/en"
			}
			model := buildRuntimeTraceProjTreeModel(fx.proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			width := runtimeTraceProjTreeLabelColumn(model, zh)
			windowMode := model.WindowMS > 0
			denom := model.WindowMS
			if !windowMode {
				denom = model.BarMaxMS
			}
			check := func(family, line string, expected string) {
				if !strings.Contains(line, expected) {
					t.Fatalf("%s/%s: row lost its typed state glyph %q:\n%s", name, family, expected, line)
				}
				if got := countGlyphs(line); got != 1 {
					t.Fatalf("%s/%s: row must carry exactly 1 state glyph, got %d:\n%s", name, family, got, line)
				}
			}
			expectIcon := func(row runtimeTraceProjTreeRow) string {
				icon := runtimeTraceProjStateIcon(row.Node, row.Kind, nil)
				member := false
				for _, g := range glyphs {
					if icon == g {
						member = true
					}
				}
				if !member {
					t.Fatalf("%s: state icon %q not in the pinned glyph set — extend the NEW-10 pin", name, icon)
				}
				return icon
			}
			for _, row := range model.TreeRows {
				if row.Kind == runtimeTraceProjTreeRowOmitted {
					continue
				}
				check("tree", runtimeTraceProjTreeRowLine(row, width, denom, windowMode, zh), expectIcon(row))
			}
			for _, row := range model.SelfRows {
				check("self", runtimeTraceProjSelfRowText(row, zh), expectIcon(row))
			}
			for _, row := range model.Adjacent {
				check("adjacent", runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh), expectIcon(row))
			}
			for _, row := range model.Background {
				check("background", runtimeTraceProjStanzaRowLine(row, width, denom, windowMode, zh), expectIcon(row))
			}
		}
	}
}

// TestTraceProjectionNew10FenceNeverContainsTabs: tabs render at viewer-defined
// widths and would silently destroy the display-cell alignment budget — the
// fence must be space-only on every surface.
func TestTraceProjectionNew10FenceNeverContainsTabs(t *testing.T) {
	fixtures := new10Fixtures()
	for _, fx := range fixtures {
		for _, zh := range []bool{true, false} {
			model := buildRuntimeTraceProjTreeModel(fx.proj, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
			if fence := runtimeTraceProjTreeFence(model, zh); strings.Contains(fence, "\t") {
				t.Fatalf("%s zh=%v: fence must never contain tab characters:\n%s", fx.name, zh, fence)
			}
		}
	}
}
