package tool

// P0-E 锁车道修3 display pins (ledger §24.9-C F5, 2026-07-09): the inferred
// -holder caveat reaches all THREE user faces — the tree row (short 推断
// qualifier), the detail stanza (持有者来历 full-origin line + 移交链 +
// 归因撤回 lines), and the lead (括注 qualifier). Pre-P0-E the engine caveat
// existed on Summary/text faces but no user panel consumed it: 置信"中" was
// the only residue (opendir_78 grep 'wakeup edge|推断' zero hits).
//
// MUTATION self-checks:
//   - deleting runtimeTraceProjBlockingHolderInferred's wakeup_edge arm reds
//     every test below;
//   - dropping the detail 持有者来历 add() reds
//     TestLockHolderInferredDetailStanzaP0E;
//   - dropping the lead qualifier reds TestLockHolderInferredLeadQualifierP0E.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func lockInferredHolderNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
		Subject: "ugc.aweme.lite-16547", Object: "blocking_span",
		BlockingKind: "monitor_contention", BlockingPeer: "LegoHandler-16865",
		BlockingSubjectIsHolder: true,
		BlockingHolderSource:    "wakeup_edge",
		BlockingOwnerTidRaw:     42067,
		ChainRelevance:          "on_chain", Rank: 1,
		ImpactMS: 115.944, CumulativeImpactMS: 115.944, Confidence: 0.62,
	}
}

func TestLockHolderInferredRowQualifierP0E(t *testing.T) {
	node := lockInferredHolderNode()
	if got := runtimeTraceCausalProjectionBlockingName(node, true); got != "持锁阻塞(等待方 LegoHandler-16865;持有者推断)" {
		t.Fatalf("holder-subject inferred row must carry the 推断 qualifier, got %q", got)
	}
	if got := runtimeTraceCausalProjectionBlockingName(node, false); got != "holds lock, blocking LegoHandler-16865 (holder inferred)" {
		t.Fatalf("EN holder-subject inferred row qualifier, got %q", got)
	}
	// Waiter-subject shape (critical_blocking row): the owner name carries
	// the qualifier inside the parens.
	waiter := node
	waiter.BlockingSubjectIsHolder = false
	waiter.BlockingPeer = "holder-42067"
	if got := runtimeTraceCausalProjectionBlockingName(waiter, true); got != "锁竞争等待(持有者 holder-42067·推断)" {
		t.Fatalf("waiter-subject inferred row qualifier, got %q", got)
	}
	// 复核收尾③d: the ns-span derivation lane is ALSO an inference — the
	// qualifier fires on it identically (the two-lane typed enum, §18.E).
	nsSpan := lockInferredHolderNode()
	nsSpan.BlockingHolderSource = "ns_span_derivation"
	if got := runtimeTraceCausalProjectionBlockingName(nsSpan, true); got != "持锁阻塞(等待方 LegoHandler-16865;持有者推断)" {
		t.Fatalf("ns-span-derived holder must carry the 推断 qualifier, got %q", got)
	}
	// Payload-direct rows stay byte-identical (negative arm).
	direct := lockInferredHolderNode()
	direct.BlockingHolderSource = "contention_payload"
	if got := runtimeTraceCausalProjectionBlockingName(direct, true); got != "持锁阻塞(等待方 LegoHandler-16865)" {
		t.Fatalf("payload-direct row must stay unqualified, got %q", got)
	}
	// No BlockingKind → the predicate never fires on non-lock rows.
	plain := types.TraceCausalProjectionNode{BlockingHolderSource: "wakeup_edge"}
	if runtimeTraceProjBlockingHolderInferred(plain) {
		t.Fatal("a non-lock row must never read as an inferred holder")
	}
}

func TestLockHolderInferredDetailStanzaP0E(t *testing.T) {
	node := lockInferredHolderNode()
	node.BlockingHolderHandoff = "NetworkKit_GRS_GrsClient-Init_0 --> NetworkKit_AssetsUtil_Operate_0"
	projection := types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "持有者来历") || !strings.Contains(detail, "唤醒边推断(payload owner 42067 不在本 trace") {
		t.Fatalf("detail stanza must carry the 持有者来历 origin line:\n%s", detail)
	}
	if !strings.Contains(detail, "持有者移交链") || !strings.Contains(detail, "最后持有者,非全段持有") {
		t.Fatalf("detail stanza must disclose the hand-off chain semantics:\n%s", detail)
	}
	// EN face parity.
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "holder origin") || !strings.Contains(detailEN, "closing wakeup edge") {
		t.Fatalf("EN detail stanza must carry the holder origin line:\n%s", detailEN)
	}
	// 复核收尾③d: the ns-span derivation origin renders its own wording.
	nsSpan := lockInferredHolderNode()
	nsSpan.BlockingHolderSource = "ns_span_derivation"
	nsModel := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{nsSpan},
	}, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	nsDetail := runtimeTraceProjDetailFullText(nsModel, true)
	if !strings.Contains(nsDetail, "持有者来历") || !strings.Contains(nsDetail, "ns-span 推导(payload owner 42067 为容器命名空间 id") {
		t.Fatalf("ns-span 推导 origin line must render:\n%s", nsDetail)
	}
	// Payload-direct rows carry NO origin line (negative arm).
	direct := lockInferredHolderNode()
	direct.BlockingHolderSource = "contention_payload"
	directModel := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{direct},
	}, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if strings.Contains(runtimeTraceProjDetailFullText(directModel, true), "持有者来历") {
		t.Fatal("payload-direct rows must not mint a 持有者来历 line")
	}
}

func TestLockHolderContradictionDetailStanzaP0E(t *testing.T) {
	node := lockInferredHolderNode()
	// The withdrawn shape: the engine cleared holder/peer and left the typed
	// witness; the display row is the WAITER-subject unresolved form.
	node.BlockingSubjectIsHolder = false
	node.BlockingPeer = ""
	node.BlockingHolderSource = ""
	node.Subject = "LegoHandler-16865"
	// G10 (§27.4/§28.1, 2026-07-09): the engine witness is minted in Chinese
	// (§22.2.1 词条尺子; payload/tid stay untranslated, number and line
	// formats byte-preserved) — the zh 明细 face no longer carries an EN
	// sentence verbatim.
	node.BlockingHolderContradiction = "推断持有者 ugc.aweme.lite-16547 自身在同一 payload 持有者 tid 42067 上排队 112.223ms(本段共 115.944ms;行 45696-79136)"
	projection := types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "持有者归因撤回") || !strings.Contains(detail, "112.223ms") {
		t.Fatalf("detail stanza must disclose the withdrawn attribution witness:\n%s", detail)
	}
	// 复核收尾③e: the EN face of the withdrawal line.
	modelEN := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	detailEN := runtimeTraceProjDetailFullText(modelEN, false)
	if !strings.Contains(detailEN, "holder attribution withdrawn") ||
		!strings.Contains(detailEN, "queued on the same lock") {
		t.Fatalf("EN detail stanza must disclose the withdrawal:\n%s", detailEN)
	}
}

func TestLockHolderInferredLeadQualifierP0E(t *testing.T) {
	node := lockInferredHolderNode()
	projection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{node},
		OnChainCauses:     []types.TraceCausalProjectionNode{node},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	lead := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(lead, "(持有者推断)") {
		t.Fatalf("the lead must qualify an inferred-holder lock claim, got %q", lead)
	}
	// Payload-direct lead stays unqualified (negative arm).
	direct := lockInferredHolderNode()
	direct.BlockingHolderSource = "contention_payload"
	directProjection := types.TraceCausalProjection{
		PrimaryRootCauses: []types.TraceCausalProjectionNode{direct},
		OnChainCauses:     []types.TraceCausalProjectionNode{direct},
	}
	directModel := buildRuntimeTraceProjTreeModel(directProjection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if lead := runtimeTraceProjConclusionLine(directProjection, directModel, true); strings.Contains(lead, "持有者推断") {
		t.Fatalf("a payload-direct lead must stay unqualified, got %q", lead)
	}
}
