package tool

// answer_document_projection_p2a_rider_test.go — P2a 显示 rider 五件套 pins
// (ledger docs/design/real_trace_campaign_20260705.md §29.55.3 处置更新 /
// §29.58.1 / §29.58.2, user rulings 2026-07-13):
//
//	件1  fold rows: 边词管车道 + 行名管折叠 + 记号位留形态族 (pinned in the
//	     PTS/PTV5/disp2/custom1g/b6 fold tests; this file adds the negative
//	     old-form tripwire).
//	件2a "· " side-note block one level deeper than the host self row
//	     (ptv6d indent pin) + the G11 stanza-level note (here).
//	件2b component rows reseated under their owning seat with the ↳
//	     connector (here).
//	件2c ∿ cadence-idle verified DISJOINT from the sleep seat's account
//	     (engine ENG-2 same-fact fold consumes the segment's sleep record)
//	     → stays a sibling, never ↳ (negative arm here).
//	件3  binder ⋈ glyph split + Mark split (F1 图例修真 — a binder-only
//	     report no longer lights the ◦ 数据行 entry) (here).
//	件4  optimization-table member/fold cells wear ↳ (rcm2 C4 pin).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// p2aSelfCarveProjection is the production self-stanza shape (witness
// 20260713-062104 family): sleep seat + binder carve rows + a cadence-idle
// row + a drill hop so the tree renders.
func p2aSelfCarveProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-sleep",
				Subject: ".ugc.aweme.lite-17267", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 35.351, CumulativeImpactMS: 35.351,
				Confidence: 0.9, LineStart: 100, LineEnd: 200},
			// Cadence idle: rides the s_sleep StateKind but is semantically its
			// own lane (件2c: account DISJOINT from the sleep seat — sibling).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-idle",
				Subject: ".ugc.aweme.lite-17267", Object: "pacing_idle", TypeToken: "pacing_idle",
				StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 15.758, CumulativeImpactMS: 15.758,
				Confidence: 0.85, LineStart: 210, LineEnd: 240},
			// The binder carve (WO-A1 carrier a).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-binder-1",
				Subject: ".ugc.aweme.lite-17267", Object: "binder:496_9-10961", TypeToken: "binder_wait",
				ChainRelevance: "on_chain", ImpactMS: 1.409, CumulativeImpactMS: 1.409,
				Confidence: 0.8, LineStart: 110, LineEnd: 150},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop-1",
				Subject: "app-9511", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 15.565, CumulativeImpactMS: 15.565,
				Confidence: 0.8, LineStart: 300, LineEnd: 320},
		},
	}
}

// 件2b: the binder component row reseats DIRECTLY under the sleep seat (it
// used to sort by magnitude behind the ∿ row) and wears the ↳ connector; the
// ∿ cadence-idle row stays a sibling (件2c verdict) and never wears ↳.
func TestP2aSelfComponentReseatWearsSubordinateConnector(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) < 3 {
		t.Fatalf("fixture drifted: want sleep+idle+binder self rows, got %d", len(model.SelfRows))
	}
	// Typed order: sleep seat first, its ↳ component directly after, the ∿
	// sibling behind them (pre-reseat magnitude order was sleep, idle, binder).
	if !model.SelfRows[0].Node.IsSleepState() || runtimeTraceProjIdleRowKind(model.SelfRows[0].Node) != "" {
		t.Fatalf("row 0 must be the sleep seat: %+v", model.SelfRows[0].Node)
	}
	if got := runtimeTraceCausalProjectionCanonicalNode(model.SelfRows[1].Node.TypeToken); got != "binder_wait" {
		t.Fatalf("row 1 must be the reseated binder component, got %q", got)
	}
	if !model.SelfRows[1].SubordinateComponentSeat {
		t.Fatalf("the reseated component row must carry the ↳ seat stamp: %+v", model.SelfRows[1])
	}
	if runtimeTraceProjIdleRowKind(model.SelfRows[2].Node) == "" {
		t.Fatalf("row 2 must be the ∿ sibling: %+v", model.SelfRows[2].Node)
	}
	if model.SelfRows[2].SubordinateComponentSeat {
		t.Fatalf("件2c: the cadence-idle row is a DISJOINT sibling — it must never take the ↳ treatment")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	lines := strings.Split(fence, "\n")
	sleepAt, binderAt, idleAt := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "│     ☾ 自身·sleep"):
			sleepAt = i
		case strings.Contains(line, "自身·binder"):
			binderAt = i
		case strings.Contains(line, "帧间空闲"):
			idleAt = i
		}
	}
	if sleepAt < 0 || binderAt != sleepAt+1 || idleAt <= binderAt {
		t.Fatalf("component row must render directly under its owning seat (sleep=%d binder=%d idle=%d):\n%s",
			sleepAt, binderAt, idleAt, fence)
	}
	// ↳ + ⋈ on the component row; the ∿ sibling keeps the bare lead.
	if !strings.HasPrefix(lines[binderAt], "│     "+tracefence.GlyphSubordinate+" "+tracefence.GlyphBinderWait+" 自身·binder") {
		t.Fatalf("component row must lead ↳ ⋈: %q", lines[binderAt])
	}
	if strings.Contains(lines[idleAt], tracefence.GlyphSubordinate) {
		t.Fatalf("∿ sibling must not wear ↳: %q", lines[idleAt])
	}
	// The containment word face stays (词面管语义).
	if !strings.Contains(fence, "不可相加·为[") {
		t.Fatalf("the component pointer word must survive the reseat:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkSubordinateComponent) {
		t.Fatalf("the ↳ legend mark must record at the emission site")
	}
}

// 件3 F1 图例修真: a binder-carve report's rows wear ⋈/IconBinderWait — the
// ◦ 数据行 entry (IconNoDominant) no longer lights on a binder-only shape
// (062916 witness form: the legend claimed 「有形态词的行戴各自形态族记号」
// while the binder row wore ◦ and lit the 数据行 entry).
func TestP2aBinderGlyphSplitStopsLightingNoDominantEntry(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, tracefence.GlyphBinderWait+" 自身·binder") {
		t.Fatalf("binder rows must wear the dedicated ⋈ glyph:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkIconBinderWait) {
		t.Fatalf("the ⋈ legend mark must record at the icon emission site")
	}
	if model.Marks.has(runtimeTraceProjMarkIconNoDominant) {
		t.Fatalf("F1 修真: this shape carries no genuinely word-less data row — the ◦ 数据行 entry must not light")
	}
	// The generated ⋈ legend entry exists in the catalog (form-table single
	// source) and speaks the IPC-wait semantics.
	found := false
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if entry.Mark != runtimeTraceProjMarkIconBinderWait {
			continue
		}
		found = true
		if !strings.Contains(entry.ZH, tracefence.GlyphBinderWait) || !strings.Contains(entry.ZH, "binder IPC 等待") {
			t.Fatalf("⋈ legend entry drifted: %q", entry.ZH)
		}
	}
	if !found {
		t.Fatalf("the ⋈ mark must own a generated legend entry")
	}
}

// 件2a F3: the G11 stanza-level side-note rides the one-level-deeper note
// geometry (its mint point is independent of the per-row demoted stream).
func TestP2aG11StanzaNoteIndentsOneLevelDeeper(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	model.SelfWaitOverflowCount = 2
	model.SelfWaitOverflowMaxMS = 0.4
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "\n│       · 另有 2 条自身等待症状行(单条最大 0.400ms)") {
		t.Fatalf("G11 note must sit one level deeper than the self rows:\n%s", fence)
	}
	enFence := runtimeTraceProjTreeFence(model, false)
	if !strings.Contains(enFence, "\n│       · 2 more self wait-symptom rows") {
		t.Fatalf("EN G11 note must ride the same geometry:\n%s", enFence)
	}
}

// 件1 negative tripwire: the retired lane-in-name forms never resurface on
// any face (fence, detail table, lossless block).
func TestP2aRetiredFoldWordFormsStayRetired(t *testing.T) {
	fold := types.TraceCausalProjectionNode{
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		MergedCount: 3, MergedSubjects: []string{"of-1", "of-2"},
		CumulativeImpactMS: 12.0, Confidence: 0.6,
	}
	for name, got := range map[string]string{
		"tree row zh":  runtimeTraceProjRowName(runtimeTraceProjTreeRow{Node: fold, Kind: runtimeTraceProjTreeRowDepthless, HasData: true}, true),
		"tree row en":  runtimeTraceProjRowName(runtimeTraceProjTreeRow{Node: fold, Kind: runtimeTraceProjTreeRowDepthless, HasData: true}, false),
		"block zh":     runtimeTraceProjDetailFullName(fold, true),
		"block en":     runtimeTraceProjDetailFullName(fold, false),
		"table cell":   runtimeTraceCausalProjectionNodeSubjectCell(fold, true),
		"table cellEN": runtimeTraceCausalProjectionNodeSubjectCell(fold, false),
	} {
		for _, banned := range []string{"链上折叠", "on-chain fold"} {
			if strings.Contains(got, banned) {
				t.Fatalf("%s: retired fold word %q resurfaced: %q", name, banned, got)
			}
		}
		if !strings.Contains(got, "(折叠)") && !strings.Contains(got, "(folded)") {
			t.Fatalf("%s: fold face must carry the dedup stem: %q", name, got)
		}
	}
}
