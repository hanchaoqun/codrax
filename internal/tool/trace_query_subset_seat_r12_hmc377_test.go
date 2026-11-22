package tool

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 round 12 (batch-owner ruling R-377-12 ③, 2026-11-22): the
// restriction subset's words.
//
// ③ The CPU-restriction row states its own restricted account, never the
// thread's whole-window account; the ⛓ row's rest names each of its carriers
// by [E#]; the pair class names only the real ◇ partner; every number the
// face prints is the engine's. Real windows at fa2f01465 (review-population
// logs/final/{pairword,fullword}_head_*.txt): w47 HeapTaskDaemon-26594 ⛓ row
// 「其余282.170ms(◇余段席)」 while ◇ [E41] published 167.008 and the
// restriction row [E43] 115.162; w54w1 nputmethod.anco-38135 restriction row
// 「全窗36.968ms=锚定7.413ms+本行其余29.555ms」 beside the thread's 105.717;
// n21w daily_control-26026 ⛓ row 「同源二分对席:◇席=[E14]…合计还原全窗账
// 182.107ms」 naming the restriction row as its ◇ partner on a 196.790ms
// account.

func hmc377R12Fence(t *testing.T, projection types.TraceCausalProjection, zh bool) (runtimeTraceProjTreeModel, string) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	fence := runtimeTraceProjTreeFence(model, zh)
	return model, fence
}

// hmc377R12Joined drops the tree rails and the wrap breaks so a sentence can
// be matched whole.
func hmc377R12Joined(fence string) string {
	joined := strings.Join(strings.Fields(strings.ReplaceAll(fence, "│", " ")), " ")
	return strings.ReplaceAll(joined, "cpuset 限制", "cpuset限制")
}

// The ⛓ row's rest is carried by its ◇ remainder seat and the same thread's
// CPU-restriction row (the engine resolved its remainder claim to both): the
// rest names each carrier by [E#]; and the ◇ row prints the engine's numbers.
func TestHMC377R12ChainRowNamesEveryCarrierOfItsRest(t *testing.T) {
	const subject = "HeapTaskDaemon-26594"
	chain := elimChainNode("r12-chain", subject, "runnable_wait", "runnable", 1, 19.476, 41)
	chain.ChainAnchorFullMS, chain.ChainAnchoredMS = 301.646, 19.476
	chain.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimRemainder), Role: string(tracequery.RootCauseShareHolderRemainderSeat), ShareMS: 282.170,
			Resolved: true, HolderEvidenceIDs: []string{"r12-rem", "r12-cpu"}},
	}
	adjacent := func(id, typ string, value float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: subject, Predicate: "root_cause_adjacent", Object: typ, TypeToken: typ, StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 2, Tier: "tertiary",
			ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value,
			LineStart: line, LineEnd: line + 9, Confidence: 0.76,
		}
	}
	rem := adjacent("r12-rem", "runnable_wait", 167.008, 50)
	rem.ChainAnchorRemainderSeat, rem.ChainAnchoredMS, rem.ChainAnchorFullMS = true, 19.476, 301.646
	rem.ShareClaims = []types.TraceCausalProjectionShareClaim{
		{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat), ShareMS: 19.476,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r12-chain"}},
		{Part: string(tracequery.RootCauseShareClaimSubset), Role: string(tracequery.RootCauseShareHolderSubsetSeat), ShareMS: 115.162,
			Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r12-cpu"}},
	}
	// The engine's counted-here number (167.008) is what the face prints;
	// the display never derives it from the rest.
	hmc377R12SetRestricted(&rem, 0, 115.162, 167.008)
	cpu := adjacent("r12-cpu", "cpu_affinity_or_cpuset", 115.162, 60)
	cpu.ChainCredentialLaneDemoted = true
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true, WakeupPath: []string{"app-100"}, WindowStartTs: 10.0, WindowEndTs: 11.0,
		OnChainCauses:  []types.TraceCausalProjectionNode{chain},
		AdjacentCauses: []types.TraceCausalProjectionNode{rem, cpu},
	}
	for _, zh := range []bool{true, false} {
		model, fence := hmc377R12Fence(t, projection, zh)
		remRow, cpuRow := xlane2FindRowByEvidenceID(&model, "r12-rem"), xlane2FindRowByEvidenceID(&model, "r12-cpu")
		if remRow == nil || cpuRow == nil || remRow.EvidenceTag == "" || cpuRow.EvidenceTag == "" {
			t.Fatal("the ◇ and restriction rows must be drawn")
		}
		joined := hmc377R12Joined(fence)
		name := map[bool]string{true: "zh", false: "en"}[zh]
		t.Run(name+"/the rest names every carrier", func(t *testing.T) {
			want := "其余282.170ms(◇余段席[" + remRow.EvidenceTag + "]、CPU亲和/cpuset限制[" + cpuRow.EvidenceTag + "]分计)"
			if !zh {
				want = "remainder 282.170ms (counted by the ◇ remainder seat [" + remRow.EvidenceTag + "] and the CPU affinity/cpuset restriction [" + cpuRow.EvidenceTag + "])"
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("%q missing:\n%s", want, fence)
			}
		})
		t.Run(name+"/the remainder keeps the plain rest word (T22)", func(t *testing.T) {
			want := "+其余282.170ms(无链上凭证;其中CPU亲和/cpuset限制下115.162ms另行计入[" + cpuRow.EvidenceTag + "],本行计167.008ms)"
			if !zh {
				want = "+ remainder 282.170ms (no chain credential; of which 115.162ms under the CPU affinity/cpuset restriction is counted separately on [" + cpuRow.EvidenceTag + "], this row 167.008ms)"
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("%q missing:\n%s", want, fence)
			}
			// Review-11 mutant T22: the remainder's rest word 「本行其余」
			// would call the whole rest this row's while part of it is
			// counted on the restriction row.
			for _, banned := range []string{"本行其余282.170ms", "this remainder 282.170ms"} {
				if strings.Contains(joined, banned) {
					t.Fatalf("the rest word must not claim the whole rest for this row (%q):\n%s", banned, fence)
				}
			}
		})
	}
}

// The restriction row states its own restricted account — every number the
// engine's — and is never paired or worded as the thread's whole-window
// account; the ⛓ row's pair class names only the real ◇ partner. Public
// trace_query entry, in-repo donghu board of HiPlayer_79_DEM-17267
// [13762.791708, 13763.024898]: JankManager-9655's restriction row (27.507ms
// restricted: 1.759 inside the ⛓ seat's walked window, 25.748 its own).
func TestHMC377R12RestrictionRowStatesItsOwnAccount(t *testing.T) {
	const fixture = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture not present: %v", err)
	}
	result, _ := hmc376QueryTarget(t, fixture, "root_cause_rank", map[string]any{"pid": 17267, "target_scope": "thread"}, 13762.791708, 13763.024898)
	projection := hmc376Projection(result)
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			var text strings.Builder
			for _, block := range runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{}) {
				text.WriteString(block.Text)
				text.WriteString("\n")
			}
			joined := hmc377R12Joined(text.String())
			want := regexp.MustCompile(`CPU亲和/cpuset限制下合计27\.507ms:其中1\.759ms另行计入\[E\d+[^\]]*\],本行计25\.748ms\(无链上凭证\)`)
			if lang == "en" {
				want = regexp.MustCompile(`27\.507ms under the CPU affinity/cpuset restriction in all: of which 1\.759ms is counted separately on \[E\d+[^\]]*\]; this row 25\.748ms \(no chain credential\)`)
			}
			if !want.MatchString(joined) {
				t.Fatalf("the restriction row's own account %q missing:\n%s", want, text.String())
			}
			for _, banned := range []string{"全窗27.507ms", "full-window 27.507ms", "合计还原全窗账 27.507ms", "restores the full-window account 27.507ms"} {
				if strings.Contains(joined, banned) {
					t.Fatalf("the restriction row's subset must never read as a whole-window account (%q):\n%s", banned, text.String())
				}
			}
		})
	}
}

// A share the engine did not resolve is not counted elsewhere: a ◇ remainder
// whose subset claim lost its holder (the cap cut the restriction row) prints
// no 「另行计入」 clause — the engine left the share in its value and wrote no
// counted-elsewhere number (review-11 mutant T02 printed it from the claim).
// And a rest one of whose carriers is drawn only inside another row takes the
// see-detail form, never naming that row as a carrier nor crediting the whole
// rest to ◇.
func TestHMC377R12RestWordsFollowTheEngineResolution(t *testing.T) {
	const subject = "daily_control-26026"
	adjacent := func(id, typ string, value float64, line int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
			Subject: subject, Predicate: "root_cause_adjacent", Object: typ, TypeToken: typ, StateKind: "runnable",
			ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 2, Tier: "tertiary",
			ImpactMS: value, CumulativeImpactMS: value, EffectiveImpactMS: value, RunnableMS: value,
			LineStart: line, LineEnd: line + 9, Confidence: 0.76,
		}
	}
	t.Run("unresolved subset share", func(t *testing.T) {
		rem := adjacent("r12-rem", "runnable_wait", 196.506, 50)
		rem.ChainAnchorRemainderSeat, rem.ChainAnchoredMS, rem.ChainAnchorFullMS = true, 0.284, 196.790
		rem.ShareClaims = []types.TraceCausalProjectionShareClaim{
			{Part: string(tracequery.RootCauseShareClaimSubset), Role: string(tracequery.RootCauseShareHolderSubsetSeat), ShareMS: 181.823},
		}
		projection := types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WakeupPath: []string{"app-100"}, WindowStartTs: 10.0, WindowEndTs: 11.0,
			AdjacentCauses: []types.TraceCausalProjectionNode{rem},
		}
		for _, zh := range []bool{true, false} {
			_, fence := hmc377R12Fence(t, projection, zh)
			for _, banned := range []string{"另行计入", "counted separately on", "本行计", "this row 14"} {
				if strings.Contains(fence, banned) {
					t.Fatalf("zh=%t: an unresolved share is counted here, never elsewhere (%q):\n%s", zh, banned, fence)
				}
			}
		}
	})
	t.Run("a carrier drawn inside another row", func(t *testing.T) {
		chain := elimChainNode("r12-chain", subject, "runnable_wait", "runnable", 1, 0.284, 41)
		chain.ChainAnchorFullMS, chain.ChainAnchoredMS = 196.790, 0.284
		chain.ShareClaims = []types.TraceCausalProjectionShareClaim{
			{Part: string(tracequery.RootCauseShareClaimRemainder), Role: string(tracequery.RootCauseShareHolderRemainderSeat), ShareMS: 196.506,
				Resolved: true, HolderEvidenceIDs: []string{"r12-rem", "r12-cpu"}},
		}
		// The remainder record is drawn only inside a folded overflow row
		// (several threads' rows, the cross-thread max fold).
		fold := adjacent("r12-fold", "runnable_wait", 16.183, 70)
		fold.Subject = "udk-irq-6-75"
		fold.MergedEvidenceIDs = []string{"r12-rem"}
		fold.MergedCount = 2
		cpu := adjacent("r12-cpu", "cpu_affinity_or_cpuset", 181.823, 60)
		cpu.ChainCredentialLaneDemoted = true
		projection := types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WakeupPath: []string{"app-100"}, WindowStartTs: 10.0, WindowEndTs: 11.0,
			OnChainCauses:  []types.TraceCausalProjectionNode{chain},
			AdjacentCauses: []types.TraceCausalProjectionNode{cpu, fold},
		}
		for _, zh := range []bool{true, false} {
			model, fence := hmc377R12Fence(t, projection, zh)
			if row := xlane2FindRowByEvidenceID(&model, "r12-chain"); row == nil || row.EvidenceTag == "" {
				t.Fatalf("zh=%t: the ⛓ row must be drawn", zh)
			}
			joined := hmc377R12Joined(fence)
			want := "其余196.506ms(◇余段席见明细)"
			if !zh {
				want = "remainder 196.506ms (◇ remainder seat: see the detail index)"
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("zh=%t: %q missing:\n%s", zh, want, fence)
			}
		}
	})
}

// The restriction row's parts counted on the thread's on-chain seats: by
// [E#] when the holder is drawn here; the see-detail form when it is not; and
// on case A' (the restriction row's chain-account claim: the chain seat keeps
// its own diverging account) the walked part is said to lie inside the chain
// seat's time, which keeps its own account — never "counted there" at the
// ledger value. Real faces at fa2f01465 stated the thread's 全窗 instead
// (review-population out/pop/head/head_w02w2.fence_zh.txt E25:
// 「账目关系(锚定权属失合):全窗31.834ms=锚定30.537ms+唤醒锚定0.946ms(⛓唤醒锚定席)+本行其余0.351ms」).
func TestHMC377R12RestrictionRowPartsByHolder(t *testing.T) {
	const subject = "hwservicemanage-64505"
	chain := elimChainNode("r12-chain", subject, "runnable_wait", "runnable", 1, 20.256, 41)
	unwalked := elimChainNode("r12-unwalked", subject, "runnable_wait", "runnable", 2, 0.946, 81)
	cpu := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "r12-cpu",
		Subject: subject, Predicate: "root_cause_adjacent", Object: "cpu_affinity_or_cpuset", TypeToken: "cpu_affinity_or_cpuset", StateKind: "runnable",
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain", Rank: 9, Tier: "tertiary",
		ImpactMS: 0.351, CumulativeImpactMS: 0.351, EffectiveImpactMS: 0.351, RunnableMS: 0.351,
		LineStart: 60, LineEnd: 69, Confidence: 0.72,
	}
	hmc377R12SetRestricted(&cpu, 31.834, 31.483, 0.351)
	walkedClaim := types.TraceCausalProjectionShareClaim{Part: string(tracequery.RootCauseShareClaimAnchored), Role: string(tracequery.RootCauseShareHolderChainSeat),
		ShareMS: 30.537, Resolved: true, HolderEvidenceIDs: []string{"r12-chain"}}
	unwalkedClaim := types.TraceCausalProjectionShareClaim{Part: string(tracequery.RootCauseShareClaimUnwalked), Role: string(tracequery.RootCauseShareHolderUnwalkedSeat),
		ShareMS: 0.946, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"r12-unwalked"}}
	accountClaim := types.TraceCausalProjectionShareClaim{Part: string(tracequery.RootCauseShareClaimChainAccount), Role: string(tracequery.RootCauseShareHolderChainSeat),
		ShareMS: 20.256, Resolved: true, HolderEvidenceIDs: []string{"r12-chain"}}
	render := func(t *testing.T, onChain []types.TraceCausalProjectionNode, claims []types.TraceCausalProjectionShareClaim, zh bool) (runtimeTraceProjTreeModel, string) {
		node := cpu
		node.ShareClaims = claims
		return hmc377R12Fence(t, types.TraceCausalProjection{
			RootCauseFamilyObserved: true, WakeupPath: []string{"app-100"}, WindowStartTs: 10.0, WindowEndTs: 11.0,
			OnChainCauses: onChain, AdjacentCauses: []types.TraceCausalProjectionNode{node},
		}, zh)
	}
	contains := func(t *testing.T, fence, want string, zh bool) {
		t.Helper()
		joined := hmc377R12Joined(fence)
		if zh {
			joined, want = strings.ReplaceAll(joined, " ", ""), strings.ReplaceAll(want, " ", "")
		}
		if !strings.Contains(joined, want) {
			t.Fatalf("%q missing:\n%s", want, fence)
		}
	}
	for _, zh := range []bool{true, false} {
		name := map[bool]string{true: "zh", false: "en"}[zh]
		t.Run(name+"/each part by its holder", func(t *testing.T) {
			model, fence := render(t, []types.TraceCausalProjectionNode{chain, unwalked}, []types.TraceCausalProjectionShareClaim{walkedClaim, unwalkedClaim}, zh)
			c, u := xlane2FindRowByEvidenceID(&model, "r12-chain"), xlane2FindRowByEvidenceID(&model, "r12-unwalked")
			if c == nil || u == nil {
				t.Fatal("holders must be drawn")
			}
			want := "CPU亲和/cpuset限制下合计31.834ms:其中30.537ms另行计入[" + c.EvidenceTag + "]、0.946ms另行计入[" + u.EvidenceTag + "],本行计0.351ms(无链上凭证)"
			if !zh {
				want = "31.834ms under the CPU affinity/cpuset restriction in all: of which 30.537ms is counted separately on [" + c.EvidenceTag + "], 0.946ms is counted separately on [" + u.EvidenceTag + "]; this row 0.351ms (no chain credential)"
			}
			contains(t, fence, want, zh)
		})
		t.Run(name+"/the chain seat keeps its own account (case A')", func(t *testing.T) {
			model, fence := render(t, []types.TraceCausalProjectionNode{chain, unwalked}, []types.TraceCausalProjectionShareClaim{walkedClaim, unwalkedClaim, accountClaim}, zh)
			c := xlane2FindRowByEvidenceID(&model, "r12-chain")
			want := "其中30.537ms落在链上席[" + c.EvidenceTag + "]时段内(链席另列自账)"
			if !zh {
				want = "of which 30.537ms lies inside the time of the chain seat [" + c.EvidenceTag + "] (the chain seat keeps its own separate account)"
			}
			contains(t, fence, want, zh)
		})
		t.Run(name+"/a holder not drawn here takes the see-detail form", func(t *testing.T) {
			_, fence := render(t, []types.TraceCausalProjectionNode{unwalked}, []types.TraceCausalProjectionShareClaim{walkedClaim, unwalkedClaim}, zh)
			want := "其中30.537ms另行计入本线程链上席(见明细)"
			if !zh {
				want = "of which 30.537ms is counted separately on this thread's on-chain seat (see the detail index)"
			}
			contains(t, fence, want, zh)
			if strings.Contains(fence, "另行计入[]") || strings.Contains(fence, "counted separately on []") {
				t.Fatalf("an empty reference must never be printed:\n%s", fence)
			}
		})
	}
}
