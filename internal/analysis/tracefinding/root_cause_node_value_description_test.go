package tracefinding

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func nodeValueDescriptionFixture() (types.TraceCausalProjection, types.TraceCausalProjectionNode) {
	node := types.TraceCausalProjectionNode{
		EvidenceID: "rank-1", Subject: "worker-1", Rank: 1, TypeToken: "priority_inversion_candidate",
		StateKind: "s_sleep", SleepMS: 17, RunningMS: 1, RunnableMS: 1,
		ImpactMS: 1, EffectiveImpactMS: 1, EffectiveImpactPublished: true, GatedRunnableMS: 1,
		ChainRelevance: "on_chain", RankQueryWindowStartTs: 2, RankQueryWindowEndTs: 2.02,
	}
	return types.TraceCausalProjection{ArtifactLabel: "capture.systrace", WindowStartTs: 2, WindowEndTs: 2.02,
		RankedSeats: []types.TraceCausalProjectionNode{node}}, node
}

func TestRootCauseNodeValueDescriptionUsesExactCandidateConversion(t *testing.T) {
	projection, node := nodeValueDescriptionFixture()
	before, _ := json.Marshal(projection)
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, set, SeatFrameCausalityAuthority{})
	if err != nil || len(contract.Candidates) != 1 {
		t.Fatalf("candidate fixture: %v %+v", err, contract)
	}
	frozen, _ := json.Marshal(contract)
	decision := contract.Candidates[0].Decision
	for _, lang := range []string{"zh", "zh-CN", "en", "en-US"} {
		got := RootCauseNodeValueDescription(projection, node, lang)
		want := RootCauseValueDescriptionForLanguage(decision, lang)
		if got == "" || got != want {
			t.Errorf("%s node adapter diverged from the exact compiled candidate: %q != %q", lang, got, want)
		}
	}
	after, _ := json.Marshal(projection)
	if string(before) != string(after) {
		t.Fatal("description changed the node/projection")
	}
	again, err := CompileCandidateContract(types.ObservationLedger{}, set, SeatFrameCausalityAuthority{})
	afterContract, _ := json.Marshal(again)
	if err != nil || string(frozen) != string(afterContract) {
		t.Fatal("description changed registry, candidate identity, value, or selection contract")
	}
}

func TestRootCauseValueDescriptionLanguageKeepsLegacyChineseBytes(t *testing.T) {
	projection, base := nodeValueDescriptionFixture()
	const qualifier = "低优先级依赖方的调度/算力供给候选，未证明反转已发生或存在锁阻塞"
	for _, tc := range []struct {
		name   string
		change func(*types.TraceCausalProjectionNode)
		zh     string
		en     []string
	}{
		{"full runnable and zero deficit", func(*types.TraceCausalProjectionNode) {},
			qualifier + "；组成：就绪等待全额 1.000 ms + 运行供给折算缺口 0.000 ms",
			[]string{"an actual inversion or lock blocking is not proved", "runnable wait counted in full 1.000 ms + folded running-supply deficit 0.000 ms"}},
		{"default capability", func(n *types.TraceCausalProjectionNode) { n.GatedCapabilitySource = "default_table" },
			qualifier + "；组成：就绪等待全额 1.000 ms + 运行供给折算缺口 0.000 ms（运行缺口按默认算力比估算）", []string{"default capability ratios"}},
		{"frequency capability", func(n *types.TraceCausalProjectionNode) { n.GatedCapabilitySource = "freq_only" },
			qualifier + "；组成：就绪等待全额 1.000 ms + 运行供给折算缺口 0.000 ms（运行缺口仅按频率比折算）", []string{"frequency ratios only"}},
		{"evidence capability", func(n *types.TraceCausalProjectionNode) { n.GatedCapabilitySource = "evidence_table" },
			qualifier + "；组成：就绪等待全额 1.000 ms + 运行供给折算缺口 0.000 ms（运行缺口采用证据支持的算力比）", []string{"evidence-supported capability ratios"}},
		{"missing composition", func(n *types.TraceCausalProjectionNode) { n.GatedRunnableMS = 0 }, qualifier, []string{"lower-priority dependency", "not proved"}},
		{"running supply", func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.StateKind = "running", "running"
			n.EffectiveImpactMS, n.ImpactMS = 3, 12
			n.SupplyFoldComputed, n.SupplyFoldDeficitMS, n.SupplyFoldIdealMS = true, 3, 9
			n.SupplyFoldKnownMS, n.SupplyFoldUnknownMS, n.SupplyFoldCapabilitySource = 10, 2, "default_table"
		}, "供给折算缺口（估算下界，非全部运行耗时）；频率已知 10.000 ms，未知 2.000 ms；采用默认算力比",
			[]string{"folded supply deficit (estimated lower bound, not total running time)", "frequency known for 10.000 ms, unknown for 2.000 ms", "default capability ratios"}},
		{"D non IO", func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.DStateRefinedNonIO, n.DStateSplitMS = "d_state_or_io_wait", true, 1
		}, "D 状态等待，已有非 I/O 证据", []string{"D-state wait with non-I/O evidence"}},
		{"D IO split", func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.EffectiveImpactMS, n.DStateSplitMS, n.IOWaitSplitMS = "fragmented_d_state_or_io_wait", 10, 3, 7
		}, "等待组成：非 IO D-state 3.000 ms，I/O 等待 7.000 ms；不是可直接消除的承诺",
			[]string{"non-IO D-state 3.000 ms, I/O wait 7.000 ms", "not a promise of directly eliminable time"}},
		{"D unsplit", func(n *types.TraceCausalProjectionNode) { n.TypeToken = "d_state_or_io_wait" },
			"D 状态与 I/O 等待的合并口径，不能全部视为 I/O", []string{"combined D-state and I/O-wait caliber", "not all of it can be treated as I/O"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := base
			tc.change(&node)
			candidate, ok := compileCandidate(projection, node, "registry-fixture", SeatFrameCausalityAuthority{})
			if !ok {
				t.Fatal("fixture is not a candidate")
			}
			if got := RootCauseValueDescription(candidate.Decision); got != tc.zh {
				t.Errorf("legacy Chinese bytes changed:\ngot  %q\nwant %q", got, tc.zh)
			}
			if got := RootCauseNodeValueDescription(projection, node, "zh-CN"); got != tc.zh {
				t.Errorf("Chinese reader and legacy sidecar differ: %q != %q", got, tc.zh)
			}
			english := RootCauseNodeValueDescription(projection, node, "en-US")
			for _, want := range tc.en {
				if !strings.Contains(english, want) {
					t.Errorf("English value ruler missing %q: %s", want, english)
				}
			}
			if strings.Contains(english, "组成") || strings.Contains(english, "缺口") {
				t.Errorf("English card borrowed Chinese wording: %s", english)
			}
		})
	}
}

func TestRootCauseNodeValueDescriptionDoesNotInventGatedComposition(t *testing.T) {
	projection, base := nodeValueDescriptionFixture()
	for name, change := range map[string]func(*types.TraceCausalProjectionNode){
		"missing":        func(n *types.TraceCausalProjectionNode) { n.GatedRunnableMS = 0 },
		"sum mismatch":   func(n *types.TraceCausalProjectionNode) { n.GatedRunningDeficitMS = 3 },
		"negative":       func(n *types.TraceCausalProjectionNode) { n.GatedRunningDeficitMS = -0.5 },
		"NaN":            func(n *types.TraceCausalProjectionNode) { n.GatedRunningDeficitMS = math.NaN() },
		"infinite":       func(n *types.TraceCausalProjectionNode) { n.GatedRunnableMS = math.Inf(1) },
		"unknown source": func(n *types.TraceCausalProjectionNode) { n.GatedCapabilitySource = "guessed" },
		"raw caliber":    func(n *types.TraceCausalProjectionNode) { n.EffectiveImpactPublished = false },
		"raw sleep is not composition": func(n *types.TraceCausalProjectionNode) {
			n.GatedRunnableMS = 0
			n.SleepMS, n.RunningMS, n.RunnableMS = 1, 0, 0
		},
	} {
		t.Run(name, func(t *testing.T) {
			node := base
			change(&node)
			for _, lang := range []string{"zh", "en"} {
				got := RootCauseNodeValueDescription(projection, node, lang)
				if strings.Contains(got, "组成：") || strings.Contains(got, "composition:") || strings.Contains(got, " ms") {
					t.Errorf("invalid/missing gated facts became numeric composition: %s", got)
				}
			}
		})
	}
}

func TestRootCauseNodeValueDescriptionRetainsCompilerAndFamilyBoundaries(t *testing.T) {
	projection, base := nodeValueDescriptionFixture()
	outside := false
	for name, change := range map[string]func(*types.TraceCausalProjectionNode){
		"no published value": func(n *types.TraceCausalProjectionNode) { n.EffectiveImpactMS = 0 },
		"no raw value":       func(n *types.TraceCausalProjectionNode) { n.ImpactMS, n.EffectiveImpactPublished = 0, false },
		"outside window":     func(n *types.TraceCausalProjectionNode) { n.WithinRequestedWindow = &outside },
		"no rank":            func(n *types.TraceCausalProjectionNode) { n.Rank = 0 },
		"no evidence":        func(n *types.TraceCausalProjectionNode) { n.EvidenceID = "" },
		"context row":        func(n *types.TraceCausalProjectionNode) { n.Tier = types.TraceCausalTierContextOnly },
		"overflow":           func(n *types.TraceCausalProjectionNode) { n.OnChainOverflowFold = true },
		"background":         func(n *types.TraceCausalProjectionNode) { n.ChainRelevance = "background" },
		"unknown family":     func(n *types.TraceCausalProjectionNode) { n.TypeToken = "future_family" },
		"composite score":    func(n *types.TraceCausalProjectionNode) { n.Unit = types.TraceObservationUnitCompositeScore },
		"IO does not borrow gated components": func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.ImpactMS, n.EffectiveImpactMS = "io_latency", 31, 31
		},
		"semantic work does not borrow supply": func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.SupplyFoldComputed, n.SupplyFoldDeficitMS = "jit_compile", true, 1
		},
		"raw running does not become supply": func(n *types.TraceCausalProjectionNode) {
			n.TypeToken, n.SupplyFoldComputed, n.SupplyFoldDeficitMS, n.EffectiveImpactPublished = "running", true, 1, false
		},
	} {
		t.Run(name, func(t *testing.T) {
			node := base
			change(&node)
			for _, lang := range []string{"zh", "en"} {
				want := ""
				if name == "IO does not borrow gated components" {
					// IO now describes its own unknown ruler; it still must not
					// borrow any numeric composition from the gated fixture.
					want = "IO观测时长（口径未明确）"
					if lang == "en" {
						want = "I/O duration (measurement unspecified)"
					}
				}
				if got := RootCauseNodeValueDescription(projection, node, lang); got != want {
					t.Errorf("family/compiler boundary changed: got %q want %q", got, want)
				}
			}
		})
	}
}

func TestRootCauseNodeValueDescriptionNeverJoinsPeerOrWindow(t *testing.T) {
	projection, first := nodeValueDescriptionFixture()
	second := first
	// Same subject and rank are deliberately insufficient identity. Even a
	// caller's projection roster containing the other row cannot supply parts.
	second.EvidenceID = "other-window"
	second.RankQueryWindowStartTs, second.RankQueryWindowEndTs = 4, 4.02
	second.GatedRunnableMS, second.GatedRunningDeficitMS, second.EffectiveImpactMS = 2, 3, 5
	projection.RankedSeats = []types.TraceCausalProjectionNode{second}
	if got := RootCauseNodeValueDescription(projection, first, "en"); !strings.Contains(got, "full 1.000 ms + folded running-supply deficit 0.000 ms") || strings.Contains(got, "3.000 ms") {
		t.Errorf("descriptor borrowed the same-subject/rank peer: %s", got)
	}
	if got := RootCauseNodeValueDescription(projection, second, "en"); !strings.Contains(got, "full 2.000 ms + folded running-supply deficit 3.000 ms") {
		t.Errorf("second row lost its own window's composition: %s", got)
	}
	first.GatedRunnableMS = 0
	if got := RootCauseNodeValueDescription(projection, first, "en"); strings.Contains(got, "composition:") {
		t.Errorf("missing composition filled from another window: %s", got)
	}
}
