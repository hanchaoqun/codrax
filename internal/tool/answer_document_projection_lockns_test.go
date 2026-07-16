package tool

// LOCKNS-FIX display pins (real_trace_campaign_20260705.md §29.104.12 +
// §29.104.12.1, 2026-07-16) — the three detail-face disclosure lines minted
// by this batch:
//
//	件6  ②×③ identity-unification consumption (OM-10 关账): the
//	     ns_span_derivation 持有者来历 line appends the 两道互证 parenthetical
//	     exactly when Node.BlockingHolderNsUnification is present.
//	件4  持有者未解析 line: a typed contention row whose holder no lane could
//	     name (no peer ∧ no source ∧ no withdrawal witness) discloses WHY —
//	     sentinel/ownerless form vs unresolvable-payload-tid form — instead of
//	     rendering silence. Resolved and withdrawn rows mint nothing here.
//	件3  持有者核查 line: the unknown-morphology fail-open marker words
//	     「owner 未解析(形态未注册)」 zh/EN.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func locknsDetailText(t *testing.T, node types.TraceCausalProjectionNode, zh bool) string {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(types.TraceCausalProjection{
		OnChainCauses: []types.TraceCausalProjectionNode{node},
	}, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	return runtimeTraceProjDetailFullText(model, zh)
}

// ── 件6 · unification parenthetical on the ns-span origin line ─────────────

func TestLockHolderNsUnificationDetailLineLOCKNS(t *testing.T) {
	node := lockInferredHolderNode()
	node.BlockingHolderSource = "ns_span_derivation"
	node.BlockingHolderNsUnification = "owner_ns_tid=62020 host=nsworker-42500 lanes=ns_span_derivation+wakeup_edge"

	detail := locknsDetailText(t, node, true)
	if !strings.Contains(detail, "持有者来历") || !strings.Contains(detail, "发射对×收尾唤醒两道互证") {
		t.Fatalf("unified ns-span origin must carry the ②×③ cross-corroboration parenthetical:\n%s", detail)
	}
	detailEN := locknsDetailText(t, node, false)
	if !strings.Contains(detailEN, "holder origin") || !strings.Contains(detailEN, "cross-corroborated") {
		t.Fatalf("EN unified origin line missing:\n%s", detailEN)
	}

	// Negative: a single-lane derivation renders the origin WITHOUT the
	// parenthetical (absence never fabricates corroboration).
	single := lockInferredHolderNode()
	single.BlockingHolderSource = "ns_span_derivation"
	singleDetail := locknsDetailText(t, single, true)
	if !strings.Contains(singleDetail, "持有者来历") {
		t.Fatalf("single-lane ns-span origin line must still render:\n%s", singleDetail)
	}
	if strings.Contains(singleDetail, "两道互证") {
		t.Fatalf("single-lane derivation must not claim corroboration:\n%s", singleDetail)
	}
}

// ── 件4 · 持有者未解析 line ────────────────────────────────────────────────

func locknsUnresolvedHolderNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
		Subject: "T7@ZeusThreadPo-61839", Object: "blocking_span",
		BlockingKind:   "lock_contention",
		ChainRelevance: "on_chain", Rank: 1,
		ImpactMS: 0.006, CumulativeImpactMS: 0.006, Confidence: 0.72,
	}
}

func TestLockHolderUnresolvedDetailLineLOCKNS(t *testing.T) {
	// Sentinel/ownerless form (no raw tid): the 件4 letter wording.
	// 修补 件C (冷读 P3-F3, 2026-07-16): the label carries 持有者未解析 and
	// the value carries only the WHY — the doubled 「持有者未解析:
	// 持有者未解析(…)」 render is pinned dead on both lanes.
	sentinel := locknsUnresolvedHolderNode()
	detail := locknsDetailText(t, sentinel, true)
	if !strings.Contains(detail, "持有者未解析: 哨兵值/无主 payload,且无收尾唤醒边") {
		t.Fatalf("sentinel unresolved-holder line missing:\n%s", detail)
	}
	if strings.Contains(detail, "持有者未解析: 持有者未解析") {
		t.Fatalf("件C: the label must not repeat at the sentence head:\n%s", detail)
	}
	detailEN := locknsDetailText(t, sentinel, false)
	if !strings.Contains(detailEN, "holder unresolved: ownerless sentinel payload and no closing wakeup edge") {
		t.Fatalf("EN sentinel unresolved-holder line missing:\n%s", detailEN)
	}
	if strings.Contains(detailEN, "holder unresolved: holder unresolved") {
		t.Fatalf("件C: the EN label must not repeat at the sentence head:\n%s", detailEN)
	}

	// Phantom form (raw tid preserved): the tid-naming variant.
	phantom := locknsUnresolvedHolderNode()
	phantom.BlockingOwnerTidRaw = 51000
	phantomDetail := locknsDetailText(t, phantom, true)
	if !strings.Contains(phantomDetail, "持有者未解析: payload owner tid 51000 无法定位,亦无收尾唤醒边") {
		t.Fatalf("phantom unresolved-holder line missing:\n%s", phantomDetail)
	}
	phantomEN := locknsDetailText(t, phantom, false)
	if !strings.Contains(phantomEN, "holder unresolved: payload owner tid 51000 could not be located and no closing wakeup edge exists") {
		t.Fatalf("EN phantom unresolved-holder line missing:\n%s", phantomEN)
	}

	// 有主形零披露: a resolved row must not mint the line.
	resolved := lockInferredHolderNode()
	if d := locknsDetailText(t, resolved, true); strings.Contains(d, "持有者未解析") {
		t.Fatalf("resolved rows must mint no unresolved-holder line:\n%s", d)
	}

	// Withdrawn rows keep their own 归因撤回 line — never doubled by 件4.
	withdrawn := locknsUnresolvedHolderNode()
	withdrawn.BlockingOwnerTidRaw = 42067
	withdrawn.BlockingHolderContradiction = "推断持有者 X 自身同锁排队 112.2ms/115.9ms(行 45696-79136)"
	wd := locknsDetailText(t, withdrawn, true)
	if strings.Contains(wd, "持有者未解析") {
		t.Fatalf("withdrawn rows carry the 归因撤回 line only, never the 件4 line:\n%s", wd)
	}
	if !strings.Contains(wd, "持有者归因撤回") {
		t.Fatalf("withdrawal line must survive untouched:\n%s", wd)
	}

	// Non-lock rows (no BlockingKind) never read as unresolved holders.
	plain := locknsUnresolvedHolderNode()
	plain.BlockingKind = ""
	if d := locknsDetailText(t, plain, true); strings.Contains(d, "持有者未解析") {
		t.Fatalf("non-lock rows must not mint the unresolved-holder line:\n%s", d)
	}
}

// ── 件3 · 持有者核查 (owner 未解析·形态未注册) line ───────────────────────

func TestOwnerKeyUnregisteredDetailLineLOCKNS(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
		Subject: "aweme-41999", Object: "blocking_span",
		BlockingOwnerKeyUnregistered: true,
		ChainRelevance:               "on_chain", Rank: 1,
		ImpactMS: 12.0, CumulativeImpactMS: 12.0, Confidence: 0.72,
	}
	detail := locknsDetailText(t, node, true)
	if !strings.Contains(detail, "持有者核查") || !strings.Contains(detail, "owner 未解析(形态未注册)") {
		t.Fatalf("unknown-morphology disclosure line missing:\n%s", detail)
	}
	detailEN := locknsDetailText(t, node, false)
	if !strings.Contains(detailEN, "holder check") || !strings.Contains(detailEN, "morphology unregistered") {
		t.Fatalf("EN unknown-morphology disclosure line missing:\n%s", detailEN)
	}

	// Negative: without the typed marker the line never renders.
	node.BlockingOwnerKeyUnregistered = false
	if d := locknsDetailText(t, node, true); strings.Contains(d, "持有者核查") {
		t.Fatalf("marker-less rows must not mint the holder-check line:\n%s", d)
	}
}

// ── 修补轮 件A · 持有者来历 presence-clause fork (冷读 P2-F1+P3-F7, 2026-07-16) ─
//
// The wakeup-edge origin line's presence clause forks on the typed
// owner_tid_presence verdict: the legacy 「不在本 trace」/"absent from this
// trace" claim was FALSE on the collision / comm-mismatch shapes (and
// contradicted the same board's engine collision Summary — numeric collision
// = tid IS present). absent / missing / unknown values keep the legacy
// sentence byte-identically (fail-open).
func TestLockHolderOriginPresenceClauseForkLOCKNSRepair(t *testing.T) {
	legacyZH := "唤醒边推断(payload owner 42067 不在本 trace;由等待方的收尾唤醒边推得,非 payload 证实)"
	legacyEN := "inferred from the waiter's closing wakeup edge (payload owner 42067 absent from this trace; not payload-confirmed)"

	// ① collision shape (P2-F1): the new sentence lands and the old false
	// claim is dead on both lanes.
	collision := lockInferredHolderNode()
	collision.BlockingOwnerTidPresence = "present_collision"
	detail := locknsDetailText(t, collision, true)
	if !strings.Contains(detail, "唤醒边推断(payload owner tid 42067 在本 trace 中存在,但为容器命名空间撞号,非持有者归因依据;由等待方的收尾唤醒边推得,非 payload 证实)") {
		t.Fatalf("collision presence clause missing:\n%s", detail)
	}
	if strings.Contains(detail, "不在本 trace") {
		t.Fatalf("the false absence claim must die on the collision shape:\n%s", detail)
	}
	detailEN := locknsDetailText(t, collision, false)
	if !strings.Contains(detailEN, "inferred from the waiter's closing wakeup edge (payload owner tid 42067 is present in this trace only as a container-namespace numeric collision, not a holder-attribution basis; not payload-confirmed)") {
		t.Fatalf("EN collision presence clause missing:\n%s", detailEN)
	}
	if strings.Contains(detailEN, "absent from this trace") {
		t.Fatalf("the EN false absence claim must die on the collision shape:\n%s", detailEN)
	}

	// ② comm-mismatch shape (P3-F7 同族).
	mismatch := lockInferredHolderNode()
	mismatch.BlockingOwnerTidPresence = "present_comm_mismatch"
	mmDetail := locknsDetailText(t, mismatch, true)
	if !strings.Contains(mmDetail, "唤醒边推断(payload owner tid 42067 在本 trace 中在场但线程名与 payload 所记不符,非持有者归因依据;由等待方的收尾唤醒边推得,非 payload 证实)") {
		t.Fatalf("comm-mismatch presence clause missing:\n%s", mmDetail)
	}
	if strings.Contains(mmDetail, "不在本 trace") {
		t.Fatalf("the false absence claim must die on the comm-mismatch shape:\n%s", mmDetail)
	}
	mmDetailEN := locknsDetailText(t, mismatch, false)
	if !strings.Contains(mmDetailEN, "inferred from the waiter's closing wakeup edge (payload owner tid 42067 is present in this trace but its thread name never matches the payload's owner comm, not a holder-attribution basis; not payload-confirmed)") {
		t.Fatalf("EN comm-mismatch presence clause missing:\n%s", mmDetailEN)
	}

	// ③ absent shape: the legacy sentence is TRUE and stays byte-identically.
	absent := lockInferredHolderNode()
	absent.BlockingOwnerTidPresence = "absent"
	if d := locknsDetailText(t, absent, true); !strings.Contains(d, legacyZH) {
		t.Fatalf("absent shape must keep the legacy sentence byte-identically:\n%s", d)
	}
	if d := locknsDetailText(t, absent, false); !strings.Contains(d, legacyEN) {
		t.Fatalf("EN absent shape must keep the legacy sentence byte-identically:\n%s", d)
	}

	// ④ missing note (legacy wire artifacts) fail-opens to the legacy
	// sentence byte-identically — and so does an unknown future value.
	for _, presence := range []string{"", "some_future_value"} {
		legacy := lockInferredHolderNode()
		legacy.BlockingOwnerTidPresence = presence
		if d := locknsDetailText(t, legacy, true); !strings.Contains(d, legacyZH) {
			t.Fatalf("presence=%q must fail open to the legacy sentence:\n%s", presence, d)
		}
		if d := locknsDetailText(t, legacy, false); !strings.Contains(d, legacyEN) {
			t.Fatalf("EN presence=%q must fail open to the legacy sentence:\n%s", presence, d)
		}
	}
}
