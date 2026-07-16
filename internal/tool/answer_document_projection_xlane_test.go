package tool

// answer_document_projection_xlane_test.go — XLANE-1 display pins
// (§29.104.1/§29.104.2, 2026-07-15; customer witness runnable2.txt).
//
//   件1 word face — the represented-by-chain-seat ◇ demotion speaks its OWN
//   honest sentence family 锚定份由链席代表(整席降道) (this seat HAS
//   credential), never the R4 无链上凭证 sentence; value untouched on 行1;
//   legend entry bidirectional.
//
//   件3 词面 rider — the 自身·墙钟席 / 自身·确定性优化 qualifier words are
//   target-exclusive: a foreign-subject row (another query step's legitimate
//   self seat fused into this tree, the witness E29/E32 shape) NEVER wears
//   them on either face (tree 行2 + ◎ overview), while the target's own rows
//   keep wearing them byte-identically.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// xlaneRepresentedSatelliteProjection — the runnable2 E11 shape after the
// engine fix: the chain seat keeps its own ⛓ account; the fully-anchored
// satellite rides ◇ with the typed represented marker and its value untouched.
func xlaneRepresentedSatelliteProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs: 17729.471126,
		WindowEndTs:   17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			// The chain-lane runnable seat (the one full representative).
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "xlane-chain-seat",
			Subject: "shadowhook-task-64305", Object: "runnable_wait", TypeToken: "runnable_wait",
			StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 17.635, CumulativeImpactMS: 17.635, EffectiveImpactMS: 17.635,
			Rank: 1, Tier: "primary", Confidence: 0.8, LineStart: 10, LineEnd: 20,
		}},
		AdjacentCauses: []types.TraceCausalProjectionNode{{
			// The represented-demoted satellite (values untouched).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xlane-satellite",
			Subject: "shadowhook-task-64305", Object: "scheduler_latency", TypeToken: "scheduler_latency",
			StateKind: "runnable", ChainRelevance: "adjacent",
			ImpactMS: 23.471, CumulativeImpactMS: 23.471, EffectiveImpactMS: 23.471,
			ChainAnchorRepresentedByChainSeat: true,
			Rank:                              1, Confidence: 0.7, LineStart: 30, LineEnd: 40,
		}},
	}
}

// 件1: the ◇ satellite renders the honest represented sentence (zh+en), keeps
// its value, records the legend mark, and never wears the R4 无凭证 sentence.
func TestXLANERepresentedSatelliteRendersDisclosure(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(xlaneRepresentedSatelliteProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "代表(整席降道):该席账目全额锚定于 typed 唤醒依赖窗内(有凭证),同段物理时间已由链上席全额代表,本席为诊断投影记 ◇ 邻近、不重复参赛,数值不变") ||
		!strings.Contains(fence, "锚定份由") {
		t.Fatalf("件1: the represented satellite must speak the honest sentence:\n%s", fence)
	}
	if !strings.Contains(fence, "23.471ms") {
		t.Fatalf("件1: the satellite value must stay untouched on the board:\n%s", fence)
	}
	if strings.Contains(fence, "无链上凭证(整席降道)") {
		t.Fatalf("件1: the 无凭证 word family is forbidden on a credential-anchored satellite:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkChainAnchorRepresented) {
		t.Fatalf("件1: the represented legend mark must record at the emission site")
	}
	if model.Marks.has(runtimeTraceProjMarkChainCredentialDemoted) {
		t.Fatalf("件1: the R4 mark must NOT fire on the represented form")
	}
	modelEN := buildRuntimeTraceProjTreeModel(xlaneRepresentedSatelliteProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !strings.Contains(fenceEN, "anchored share represented by") ||
		!strings.Contains(fenceEN, "(whole-seat demotion): this seat's whole account is anchored inside typed wakeup-dependency windows (it HAS credential) and the same physical time is already fully represented on the chain tier") {
		t.Fatalf("件1: en mirror of the represented sentence missing:\n%s", fenceEN)
	}
}

// 件1 pointer form: when the cross-channel stamp resolved the chain seat's
// [E#], the sentence names it inline (锚定份由链席[E#]代表 — 互指正席).
func TestXLANERepresentedSatelliteSentenceCarriesChainSeatPointer(t *testing.T) {
	projection := xlaneRepresentedSatelliteProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// Find the satellite row and check the cross-channel machinery + sentence
	// agreement: with a stamped ref the sentence must name it; without one it
	// must keep the generic noun (never a guessed E#).
	found := false
	for _, row := range model.Adjacent {
		if row.Node.EvidenceID != "xlane-satellite" {
			continue
		}
		found = true
		notes := runtimeTraceProjSameSegMirrorTagTexts(row, true)
		joined := strings.Join(notes, "\n")
		if ref := strings.TrimSpace(row.CrossChannelChainRef); ref != "" {
			if !strings.Contains(joined, "锚定份由链席["+ref+"]代表") {
				t.Fatalf("件1 互指: the sentence must name the stamped chain-seat ref %q:\n%s", ref, joined)
			}
		} else if !strings.Contains(joined, "锚定份由本线程链上席代表") {
			t.Fatalf("件1 互指: without a stamped ref the sentence keeps the generic noun:\n%s", joined)
		}
	}
	if !found {
		t.Fatalf("fixture drifted: the satellite row must render in the adjacent stanza")
	}
}

// 修复轮 P3-① (对抗复核, 2026-07-16): the ref-less noun-fallback branch pin —
// a represented row rendered without a stamped cross-channel ref (chain seat
// off-board / tag-less) speaks the generic 本线程链上席 noun and never guesses
// an E#.
func TestXLANERepresentedSentenceNounFallbackWithoutRef(t *testing.T) {
	row := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "shadowhook-task-64305", Object: "scheduler_latency",
		ChainRelevance:                    "adjacent",
		ChainAnchorRepresentedByChainSeat: true,
	}}
	notes := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, true), "\n")
	if !strings.Contains(notes, "锚定份由本线程链上席代表(整席降道)") {
		t.Fatalf("P3-①: the ref-less render must speak the generic noun:\n%s", notes)
	}
	if strings.Contains(notes, "锚定份由链席[") {
		t.Fatalf("P3-①: no E# may be guessed without a stamped ref:\n%s", notes)
	}
	notesEN := strings.Join(runtimeTraceProjSameSegMirrorTagTexts(row, false), "\n")
	if !strings.Contains(notesEN, "anchored share represented by this thread's chain-lane seat (whole-seat demotion)") {
		t.Fatalf("P3-①: en mirror of the noun fallback missing:\n%s", notesEN)
	}
}

// xlaneForeignSelfProjection — the witness E29/E32 shape: the tree target is
// ease.cloudmusic-63993, and a shadowhook-task row minted with a SELF basis
// (legitimate in its own query step) is fused into this tree. A target-self
// control row rides beside it.
func xlaneForeignSelfProjection(foreignBasis string) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"shadowhook-task-64305", "ease.cloudmusic-63993"},
		WindowStartTs: 17729.471126,
		WindowEndTs:   17729.622508,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			// Foreign-subject row carrying a self basis (E29/E32 病形).
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xlane-foreign-self",
			Subject: "shadowhook-task-64305", Object: "running_burst", TypeToken: "compute_supply",
			StateKind: "running", ChainRelevance: "on_chain",
			Causality: "self_wall_clock", OnChainBasis: foreignBasis,
			ImpactMS: 6.604, CumulativeImpactMS: 6.604, EffectiveImpactMS: 3.358,
			Rank: 2, Tier: "secondary", Confidence: 0.8, LineStart: 50, LineEnd: 60,
		}, {
			// Target-self control row: keeps wearing the qualifier.
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "xlane-target-self",
			Subject: "ease.cloudmusic-63993", Object: "io_latency", TypeToken: "io_latency",
			ChainRelevance: "on_chain",
			Causality:      "self_wall_clock", OnChainBasis: "self_wall_clock_interval",
			ImpactMS: 1.767, CumulativeImpactMS: 1.767, EffectiveImpactMS: 1.767,
			Rank: 3, Tier: "tertiary", Confidence: 0.86, LineStart: 70, LineEnd: 80,
		}},
	}
}

// 件3 词面 rider: the 自身· qualifier words are target-exclusive — the
// foreign-subject row never wears them (tree 行2 + ◎ overview), the target's
// own row keeps them (positive control, both faces).
func TestXLANEForeignSubjectNeverWearsSelfQualifier(t *testing.T) {
	for _, tc := range []struct {
		name  string
		basis string
		word  string
	}{
		{"wall clock seat word", "self_wall_clock_interval", "自身·墙钟席"},
		{"deterministic span word", "self_deterministic_span", "自身·确定性优化"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projection := xlaneForeignSelfProjection(tc.basis)
			projection.RootCauseFamilyObserved = true
			model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
			fence := runtimeTraceProjTreeFence(model, true)
			elim := runtimeTraceProjElimOverviewFence(projection, model, true)
			for _, line := range strings.Split(fence+"\n"+elim, "\n") {
				if strings.Contains(line, "shadowhook") && strings.Contains(line, tc.word) {
					t.Fatalf("件3 负向 pin: a foreign-subject row must never wear %s: %q", tc.word, line)
				}
			}
			// 行2/◎ lines are subject-less continuation lines — sweep the row
			// blocks: no 自身· wearing may appear anywhere in this render
			// except on the target's own row block (the control asserts the
			// word survives for the target).
			foreignRows := xlaneCollectRowLines(fence, "shadowhook")
			for _, block := range foreignRows {
				if strings.Contains(block, "自身·墙钟席") || strings.Contains(block, "自身·确定性优化") {
					t.Fatalf("件3 负向 pin: the foreign row block must carry no 自身· word:\n%s", block)
				}
			}
			if tc.basis == "self_wall_clock_interval" {
				// Positive control: the qualifier survives for the TARGET's own
				// self seat (it renders in the self stanza as 自身·IO延迟 without
				// the subject name; the foreign blocks above are pinned clean, so
				// any occurrence is the target's).
				if !strings.Contains(fence, "自身·墙钟席") {
					t.Fatalf("件3 positive control: the target's own self seat must keep wearing 自身·墙钟席:\n%s", fence)
				}
			}
		})
	}
}

// xlaneCollectRowLines groups the fence into per-row blocks: a block starts at
// a line naming the subject and extends over the following continuation lines
// (`· …` rows 2..N) until the next subject-bearing line.
func xlaneCollectRowLines(fence, subject string) []string {
	var blocks []string
	var current []string
	inBlock := false
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimLeft(line, "│ \t")
		isContinuation := strings.HasPrefix(trimmed, "·")
		if strings.Contains(line, subject) && !isContinuation {
			if inBlock {
				blocks = append(blocks, strings.Join(current, "\n"))
			}
			current = []string{line}
			inBlock = true
			continue
		}
		if inBlock {
			if isContinuation {
				current = append(current, line)
				continue
			}
			blocks = append(blocks, strings.Join(current, "\n"))
			current = nil
			inBlock = false
		}
	}
	if inBlock {
		blocks = append(blocks, strings.Join(current, "\n"))
	}
	return blocks
}

// 件3 fail-open pin: a target-less (flat) model proves nothing about subject
// identity — the wearing keeps the legacy bytes (no suppression without a
// proven mismatch).
func TestXLANEFlatModeKeepsLegacySelfWearing(t *testing.T) {
	projection := xlaneForeignSelfProjection("self_wall_clock_interval")
	projection.WakeupPath = nil // flat mode: no resolved target
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.SelfQualifierForeignSubject {
				t.Fatalf("件3 fail-open: a target-less model must stamp no foreign-subject verdict: %+v", row.Node.Subject)
			}
		}
	}
}
