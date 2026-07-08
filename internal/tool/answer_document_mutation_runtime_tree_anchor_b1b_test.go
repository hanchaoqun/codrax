package tool

// B1-b display pins (§22 B1-b, huadong_01 CHAIN-PATH audit 2026-07-07).
//
// F1 display half: the mid-path any-position election (compile-root, pinned in
// internal/types) reaches the 🎯 root — the tree roots on the user thread, the
// header keeps ‹用户关注线程›, and the 免责横幅 lane never fires. Pre-B1-b this
// exact shape rooted on the artifact path end (VSyncGenerator) WITH the banner.
//
// F2: a typed user entity inside the long-trunk folded middle
// (projection.WakeupPathUserEntityHits — AnchorUserEntities 同源, single
// compile-root comparator) is force-expanded out of the "…省略N节点" fold:
//   - with its own data row → normal trunk row (line numbers / values kept);
//   - without one → the named ◦ ⊚中转 transit row;
//   - the fold row(s) cover only the remaining segment(s) with true counts;
//   - no hits → the single fold row stays byte-stable (legacy).
//
// PTV8-LAD EVOLUTION RECORD (§24.11 维度A / §24.8, 2026-07-08): two revisions
// of the F2 display half —
//   - L3: the 18-cell 用户关注线程(中转) long label → the 3-cell ⊚中转 short
//     token (the label out-widened the identity it disclosed on huadong_78);
//   - F2 展开一次: a hit inside a run-length CYCLE segment merges into the
//     "↺ 循环×N: A ⇄ B" row (which names it in full) instead of minting a
//     per-hit ladder row; hits outside cycle runs keep force-expanding exactly
//     as pinned below (these fixtures carry no repetition, so every pin here
//     exercises the unchanged lane; the cycle lane's pins live in
//     answer_document_projection_lad_test.go).
// The types-side F2 carrier (WakeupPathUserEntityHits) is untouched.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// F1 display half: huadong shape end-to-end through the ledger compile — user
// entity mid-path, path end hijacked by zero-impact transit nodes.
func TestRuntimeTraceProjTreeAnchorMidPathElectionRootsUserThreadB1b(t *testing.T) {
	projection := anchorB1ToolProjection(t, []string{"42591"},
		anchorB1ToolPathRecord("path-huadong", "oney.hmn.berlin-42591",
			"VSyncGenerator-2270 -> oney.hmn.berlin-42591 -> tppmgr-idle-7-189 -> gpu-pm-re-327"),
	)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.Target != "oney.hmn.berlin-42591" {
		t.Fatalf("B1-b: 🎯 root must be the truncated elected path's user thread, got %q", model.Target)
	}
	if !model.TargetUserElected {
		t.Fatalf("B1-b: model must carry the typed election marker")
	}
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"42591"}})
	if model.RootFocusAnchorOnly {
		t.Fatalf("B1-b: the mid-path election must keep ‹用户关注线程›, not the 免责 lane")
	}
	if header := runtimeTraceProjTreeHeaderLabel(model, true); !strings.Contains(header, "‹用户关注线程›") {
		t.Fatalf("B1-b: header must carry ‹用户关注线程›, got %q", header)
	}
}

// F1 acceptance surface (huadong E16/E18/E19 身份归位): once the mid-path
// election roots the user thread, its subject rows seat as the 🎯 target's OWN
// rows (SelfRows) instead of anonymous depthless chain rows under a foreign
// root — the identity mechanics key off model.Target, which the truncated
// elected path now supplies.
func TestRuntimeTraceProjTreeUserEntityRowsBecomeSelfRowsB1b(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:            []string{"VSyncGenerator-2270", "oney.hmn.berlin-42591"},
		WakeupPathUserElected: true,
		WindowStartTs:         100.0,
		WindowEndTs:           100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			b1bUserFocusFoldNode("oney.hmn.berlin-42591", 21),
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if len(model.SelfRows) != 1 || model.SelfRows[0].Node.Subject != "oney.hmn.berlin-42591" {
		t.Fatalf("B1-b: the elected user entity's rows must seat as 🎯 self rows, got %+v", model.SelfRows)
	}
}

// b1bUserFocusFoldPath is the shared long-trunk shape: 11 path elements →
// trunk len 10 > 8 → fold range trunk[5:7] = {p6-6 (path idx 4), p7-7 (path
// idx 3)}.
func b1bUserFocusFoldPath() []string {
	return []string{"p10-10", "p9-9", "p8-8", "p7-7", "p6-6", "p5-5",
		"p4-4", "p3-3", "p2-2", "p1-1", "target-7"}
}

func b1bUserFocusFoldNode(subject string, impact float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: subject,
		Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
		ImpactMS: impact, Confidence: 0.8,
	}
}

// b1bUserFocusFoldProjection is the F2 TRANSIT shape (also the legend
// bidirectional fixture): the typed user entity p6-6 (path idx 4 → trunk idx
// 5, inside the fold) carries NO data row, so the force-expand renders the
// named ◦ ⊚中转 row and the fold shrinks to the remaining node.
func b1bUserFocusFoldProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:               b1bUserFocusFoldPath(),
		WakeupPathUserEntityHits: []int{4},
		WindowStartTs:            100.0,
		WindowEndTs:              100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			b1bUserFocusFoldNode("target-7", 50),
			b1bUserFocusFoldNode("p1-1", 40),
		},
	}
}

func b1bFoldFence(t *testing.T, projection types.TraceCausalProjection) string {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return runtimeTraceProjTreeFence(model, true)
}

// F2 pin (transit shape): the folded user entity is force-expanded as the
// named transit row; the fold row covers ONLY the remaining node with a true
// count. Reverting the force-expand (folding over the hits) reds both asserts.
func TestRuntimeTraceProjTreeFoldForceExpandsUserEntityTransitB1b(t *testing.T) {
	fence := b1bFoldFence(t, b1bUserFocusFoldProjection())
	if !strings.Contains(fence, "p6-6 ⊚中转") {
		t.Fatalf("B1-b F2: the folded user entity must render as the ⊚中转 named transit row, fence:\n%s", fence)
	}
	if !strings.Contains(fence, "…省略1节点") || strings.Contains(fence, "…省略2节点") {
		t.Fatalf("B1-b F2: the fold row must cover only the remaining node (省略1, not 省略2), fence:\n%s", fence)
	}

	// Negative (legacy byte-stability): the same shape WITHOUT hits keeps the
	// single 2-node fold and never mints the user-focus wording.
	legacy := b1bUserFocusFoldProjection()
	legacy.WakeupPathUserEntityHits = nil
	legacyFence := b1bFoldFence(t, legacy)
	if !strings.Contains(legacyFence, "…省略2节点") {
		t.Fatalf("B1-b F2: the no-hit fold must stay the legacy single 2-node fold, fence:\n%s", legacyFence)
	}
	if strings.Contains(legacyFence, "⊚中转") {
		t.Fatalf("B1-b F2: the no-hit lane must never mint the user-focus wording, fence:\n%s", legacyFence)
	}
}

// F2 pin (data shape): a folded user entity WITH its own on-chain row renders
// as a normal trunk data row — value and evidence lane kept, no anonymous
// fold, no transit wording (the row has data).
func TestRuntimeTraceProjTreeFoldForceExpandsUserEntityWithDataB1b(t *testing.T) {
	projection := b1bUserFocusFoldProjection()
	projection.WakeupPathUserEntityHits = []int{3} // p7-7 → trunk idx 6, inside the fold
	projection.OnChainCauses = append(projection.OnChainCauses, b1bUserFocusFoldNode("p7-7", 33))
	fence := b1bFoldFence(t, projection)
	if !strings.Contains(fence, "p7-7") || !strings.Contains(fence, "33.000ms") {
		t.Fatalf("B1-b F2: the folded user entity's DATA row must render with its value, fence:\n%s", fence)
	}
	if !strings.Contains(fence, "…省略1节点") || strings.Contains(fence, "…省略2节点") {
		t.Fatalf("B1-b F2: the fold row must exclude the force-expanded data row from its count, fence:\n%s", fence)
	}
	if strings.Contains(fence, "⊚中转") {
		t.Fatalf("B1-b F2: a data row is not a transit — no transit wording, fence:\n%s", fence)
	}
}

// F2 pin (both folded nodes are user entities): every hit expands; no fold row
// remains (zero-count fold rows must not render).
func TestRuntimeTraceProjTreeFoldForceExpandsEveryUserEntityB1b(t *testing.T) {
	projection := b1bUserFocusFoldProjection()
	projection.WakeupPathUserEntityHits = []int{3, 4} // p7-7 + p6-6: the whole folded middle
	fence := b1bFoldFence(t, projection)
	if strings.Contains(fence, "省略") {
		t.Fatalf("B1-b F2: expanding every folded node must leave no fold row, fence:\n%s", fence)
	}
	for _, name := range []string{"p6-6", "p7-7"} {
		if !strings.Contains(fence, name+" ⊚中转") {
			t.Fatalf("B1-b F2: %s must render as a named transit row, fence:\n%s", name, fence)
		}
	}
}

// F1(b) 复核建议② regression pin (对抗复核 2026-07-07, SHIP-WITH-FIXES): a DATA
// node on the producer-truncated (dropped) suffix keeps its rendered seat —
// zero silent drops. Real compile chain end-to-end: ChainResult → producer
// observations (traceQueryTypedObservations: the path record publishes the
// TRUNCATED walk, the wakeup_causal_impact record publishes the suffix node's
// data) → projection compile → tree model → fence. The suffix node's subject
// is no longer a trunk member and its engine ChainDepth exceeds the truncated
// trunk, so depthAttach cannot seat it — it must take the honest depthless
// 链上·深度未解析 lane (Kind=Depthless) and appear in the fence; the demote
// gate eats only aggregate-metric / unknown-thread rows, never a labeled data
// node. Negative assert: the node's row NEVER disappears from the fence.
func TestRuntimeTraceProjTreeDroppedSuffixDataNodeKeepsDepthlessSeatB1b(t *testing.T) {
	chain := b1bOvershootChain()
	// Extend the overshoot: the nil-impact transit also wakes a DATA node —
	// the un-truncated walk ends [… target → transit → vsync → transit → gpu],
	// so gpu-pm-re-327 sits on the dropped post-target suffix.
	chain.Nodes = append(chain.Nodes, tracequery.ChainNode{
		ID: "n-gpu", Thread: tracequery.ThreadRef{Comm: "gpu-pm-re", PID: 327},
		Impact: &tracequery.WakeupCausalImpact{ChainDepth: 3, TotalMs: 5.0},
	})
	chain.Edges = append(chain.Edges, tracequery.WakeupEdge{
		From: "n-transit", To: "n-gpu",
		Waker:    tracequery.ThreadRef{Comm: "tppmgr-idle-7", PID: 189},
		Wakee:    tracequery.ThreadRef{Comm: "gpu-pm-re", PID: 327},
		WakeupTs: 1.06, WakeupLine: 95,
	})
	chain.CausalImpacts = []tracequery.WakeupCausalImpact{{
		Thread:           tracequery.ThreadRef{Comm: "gpu-pm-re", PID: 327},
		Window:           tracequery.TimeWindow{StartTs: 1.05, EndTs: 1.07},
		ChainDepth:       3,
		OnChain:          true,
		DominantState:    "runnable",
		DominantImpactMs: 5.0,
		TotalMs:          5.0,
		LineStart:        95, LineEnd: 96,
	}}
	records := traceQueryTypedObservations(tracequery.Result{WakeupChain: &chain},
		"trace", "/blobs/b1b-suffix.json", "", "", time.Now())
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{
		Records:            records,
		AnchorUserEntities: []types.AnchorUserEntity{{Value: "42591", TypedLane: true}},
	})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	p := set.Projections[0]
	// Producer half sanity: the published path is target-terminated and the
	// suffix data node is NOT a trunk member.
	if len(p.WakeupPath) == 0 || p.WakeupPath[len(p.WakeupPath)-1] != "oney.hmn.berlin-42591" {
		t.Fatalf("B1-b F1(b): the compiled path must end at the target, got %v", p.WakeupPath)
	}
	for _, item := range p.WakeupPath {
		if item == "gpu-pm-re-327" {
			t.Fatalf("B1-b F1(b): the dropped-suffix node must not be a trunk member, got %v", p.WakeupPath)
		}
	}
	model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var seat *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Node.Subject == "gpu-pm-re-327" {
			seat = &model.TreeRows[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("零静默丢: the dropped-suffix DATA node must keep a rendered seat, rows: %+v", model.TreeRows)
	}
	if seat.Kind != runtimeTraceProjTreeRowDepthless {
		t.Fatalf("B1-b F1(b): the seat must be the honest depthless lane, got Kind=%q Edge=%q", seat.Kind, seat.Edge)
	}
	if seat.Edge != runtimeTraceProjTreeEdgeChainUnresolved {
		t.Fatalf("B1-b F1(b): the depthless seat must carry the 链上·深度未解析 edge, got %q", seat.Edge)
	}
	if fence := runtimeTraceProjTreeFence(model, true); !strings.Contains(fence, "gpu-pm-re-327") {
		t.Fatalf("零静默丢: the dropped-suffix node's row must appear in the fence:\n%s", fence)
	}
}
