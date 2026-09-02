package tracefinding

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRootCauseReportPreservesRankedValueCaliber(t *testing.T) {
	for _, tc := range []struct {
		name     string
		node     types.TraceCausalProjectionNode
		category types.TraceRootCauseCategory
		want     string
	}{
		{"running supply deficit", types.TraceCausalProjectionNode{TypeToken: "running", ImpactMS: 74.915,
			EffectiveImpactMS: 65.912, EffectiveImpactPublished: true, SupplyFoldComputed: true,
			SupplyFoldDeficitMS: 65.912, SupplyFoldIdealMS: 9.003, SupplyFoldKnownMS: 74.915,
			SupplyFoldCapabilitySource: "default_table"}, types.TraceRootCauseComputeSupplyShortage, "供给折算缺口"},
		{"fragmented running supply deficit", types.TraceCausalProjectionNode{TypeToken: "fragmented_running", ImpactMS: 12,
			EffectiveImpactMS: 3, EffectiveImpactPublished: true, SupplyFoldComputed: true,
			SupplyFoldDeficitMS: 3, SupplyFoldIdealMS: 9, SupplyFoldKnownMS: 10, SupplyFoldUnknownMS: 2},
			types.TraceRootCauseComputeSupplyShortage, "未知 2.000 ms"},
		{"raw running does not become a deficit", types.TraceCausalProjectionNode{TypeToken: "running", ImpactMS: 12,
			SpanName: "DrawFrame", SupplyFoldComputed: true, SupplyFoldDeficitMS: 3, SupplyFoldKnownMS: 12},
			types.TraceRootCausePhaseHighLoad, "E1"},
		{"D proven non IO", types.TraceCausalProjectionNode{TypeToken: "d_state_or_io_wait", ImpactMS: 36.757,
			EffectiveImpactMS: 36.757, EffectiveImpactPublished: true, DStateRefinedNonIO: true,
			DStateSplitMS: 36.757}, types.TraceRootCauseSleepBlocking, "非 I/O"},
		{"D unknown IO share", types.TraceCausalProjectionNode{TypeToken: "d_state_or_io_wait", ImpactMS: 7},
			types.TraceRootCauseSleepBlocking, "不能全部视为 I/O"},
		{"mixed D IO", types.TraceCausalProjectionNode{TypeToken: "d_state_or_io_wait", ImpactMS: 10,
			DStateSplitMS: 3, IOWaitSplitMS: 7}, types.TraceRootCauseSleepBlocking, "I/O 等待 7.000 ms"},
		{"fragmented mixed D IO", types.TraceCausalProjectionNode{TypeToken: "fragmented_d_state_or_io_wait", ImpactMS: 10,
			DStateSplitMS: 3, IOWaitSplitMS: 7}, types.TraceRootCauseSleepBlocking, "D 状态 3.000 ms"},
		{"merged pure IO", types.TraceCausalProjectionNode{TypeToken: "d_state_or_io_wait", ImpactMS: 7,
			IOWaitSplitMS: 7}, types.TraceRootCauseIOBlocking, "I/O 等待 7.000 ms"},
		{"explicit IO wait including S", types.TraceCausalProjectionNode{TypeToken: "io_wait", ImpactMS: 7, StateKind: "s_sleep"},
			types.TraceRootCauseIOBlocking, "E1"},
		{"semantic work is not a supply fold", types.TraceCausalProjectionNode{TypeToken: "jit_compile", ImpactMS: 12,
			EffectiveImpactMS: 12, EffectiveImpactPublished: true, SupplyFoldComputed: true, SupplyFoldDeficitMS: 3},
			types.TraceRootCauseJITCompilation, "E1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := tc.node
			node.EvidenceID, node.Subject, node.Rank, node.ChainRelevance = "E1", "worker-9", 1, "on_chain"
			contract, err := CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
				Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{node}}}}, "proven")
			if err != nil || len(contract.Candidates) != 1 {
				t.Fatalf("compile: %v %+v", err, contract)
			}
			contract.RootCauseReportEnabled = true
			report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
				RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: contract.Candidates[0].Decision.CandidateID}}}, contract)
			if err != nil {
				t.Fatal(err)
			}
			item := report.RootCauses[0]
			if item.Category != tc.category || !strings.Contains(strings.Join(item.Evidence, " "), tc.want) {
				t.Fatalf("ranked value lost its precise caliber: %+v; want %s / %s", item, tc.category, tc.want)
			}
			wantValue := node.ImpactMS
			if node.EffectiveImpactPublished {
				wantValue = node.EffectiveImpactMS
			}
			if *item.ImpactSeconds != wantValue/1000 || item.CandidateID != "" {
				t.Fatalf("value or identity drift: %+v", item)
			}
		})
	}
}

func TestRootCauseMagnitudeComponentsAreFrozenWithCandidate(t *testing.T) {
	node := types.TraceCausalProjectionNode{EvidenceID: "E1", Subject: "worker", Rank: 1, TypeToken: "running",
		ChainRelevance: "on_chain", EffectiveImpactPublished: true, EffectiveImpactMS: 3, ImpactMS: 12,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 3, SupplyFoldIdealMS: 9, SupplyFoldKnownMS: 10,
		SupplyFoldUnknownMS: 2, SupplyFoldCapabilitySource: "freq_only"}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{node}}}}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, set, "proven")
	if err != nil {
		t.Fatal(err)
	}
	want := contract.Candidates[0].Decision
	if want.Token.Lane != "cpu_work" || want.Magnitude.Components.SupplyFoldDeficitMS != 3 || want.Magnitude.Components.SupplyFoldIdealMS != 9 {
		t.Fatalf("source registry/value basis was changed or lost: %+v", want)
	}
	raw, _ := json.Marshal(want)
	var got types.TraceCauseDecision
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if err := validateCandidateSnapshot(got, want); err != nil {
		t.Fatal(err)
	}
	got.Magnitude.Components.SupplyFoldUnknownMS = 0
	if err := validateCandidateSnapshot(got, want); err == nil {
		t.Fatal("model rewrite of the frozen coverage was accepted")
	}
	set.Projections[0].RankedSeats[0].SupplyFoldUnknownMS = 0
	changed, err := CompileCandidateContract(types.ObservationLedger{}, set, "proven")
	if err != nil || changed.ContractHash == contract.ContractHash {
		t.Fatalf("value basis omitted from contract identity: %v", err)
	}
	// A zero published deficit is not raw-work credit. Both known-zero and
	// absent-fold running remain in the projection but not the root roster.
	for _, computed := range []bool{true, false} {
		set.Projections[0].RankedSeats[0].EffectiveImpactMS = 0
		set.Projections[0].RankedSeats[0].SupplyFoldComputed = computed
		empty, err := CompileCandidateContract(types.ObservationLedger{}, set, "proven")
		if err != nil || len(empty.Candidates) != 0 {
			t.Fatalf("zero effective value became raw running root: %v %+v", err, empty)
		}
	}
}
