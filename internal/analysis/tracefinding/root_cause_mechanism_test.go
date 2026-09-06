package tracefinding

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRootCausePriorityCandidateKeepsMechanismAndCompositeCaliber(t *testing.T) {
	for _, frame := range []bool{false, true} {
		node := types.TraceCausalProjectionNode{EvidenceID: "E1", Subject: "worker-9", Rank: 1,
			TypeToken: "priority_inversion_candidate", ChainRelevance: "on_chain", StateKind: "runnable",
			ImpactMS: 15, EffectiveImpactMS: 9, EffectiveImpactPublished: true,
			GatedRunnableMS: 5, GatedRunningDeficitMS: 4, GatedCapabilitySource: "default_table"}
		contract, err := CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
			Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{node}}}},
			SeatFrameCausalityAuthority{Applicable: frame, Index: SeatFrameCausalityIndex{"E1": true}})
		if err != nil || len(contract.Candidates) != 1 {
			t.Fatalf("compile: %v %+v", err, contract)
		}
		contract.RootCauseReportEnabled = true
		description := "依赖线程等待调度并在低算力核执行，建议进一步核实供给。"
		report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
			RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: contract.Candidates[0].Decision.CandidateID, Description: description}}}, contract)
		if err != nil {
			t.Fatal(err)
		}
		wire, _ := json.Marshal(report.RootCauses[0])
		for _, want := range []string{`"mechanism_qualifier":"lower_priority_dependency_candidate"`,
			`"runnable_seconds":0.005`, `"running_deficit_seconds":0.004`, `"capability_source":"default_table"`,
			"优先级反转候选", "就绪等待全额 5.000 ms", "运行供给折算缺口 4.000 ms", "未证明反转已发生"} {
			if !strings.Contains(string(wire), want) {
				t.Fatalf("mechanism/composition lost %q: %s", want, wire)
			}
		}
		if strings.Contains(string(wire), "修向=锁") || report.RootCauses[0].Description != description || *report.RootCauses[0].ImpactSeconds != .009 {
			t.Fatalf("qualifying a candidate must preserve value and model prose and cannot invent a lock: %s", wire)
		}
	}
}

func TestRootCausePresentFrameWithUnprovenFlowDoesNotClaimMissingFrames(t *testing.T) {
	input := types.ObservationLedgerInput{RequestModel: &types.RequestModel{
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{FrameCausalityRequested: true}},
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{TypedCausalRowCount: 1, FrameEvidenceStatus: "present",
				FrameFlowCausalConclusion: tracequery.FrameFlowCausalityUnproven},
			Observations: []types.ObservationRecord{{ID: "E-A", Origin: types.AnswerEvidenceOriginRuntimeArtifact}}}}}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), BuildSeatFrameCausalityAuthority(input))
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: contract.Candidates[0].Decision.CandidateID}}}, contract)
	if err != nil {
		t.Fatal(err)
	}
	item := report.RootCauses[0]
	if item.CausalQualifier != types.TraceCausalQualifierFrameUnproven ||
		strings.Contains(strings.Join(item.Evidence, " "), "没有帧证据") {
		t.Fatalf("unproven flow with present frame evidence is not missing frames: %+v", item)
	}
}

func TestRootCauseMechanismBoundarySurvivesEvidenceFitWithoutRepricing(t *testing.T) {
	for _, effective := range []bool{false, true} {
		node := types.TraceCausalProjectionNode{EvidenceID: "E1", Subject: strings.Repeat("依赖线程", 90), Rank: 1,
			TypeToken: "priority_inversion_candidate", ChainRelevance: "on_chain", StateKind: strings.Repeat("状态", 150),
			ImpactMS: 20, EffectiveImpactMS: 9, EffectiveImpactPublished: effective, GatedRunnableMS: 5, GatedRunningDeficitMS: 4}
		contract, err := CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
			Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{node}}}}, SeatFrameCausalityAuthority{})
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
		if item.MechanismQualifier != types.TraceMechanismLowerPriorityDependencyCandidate ||
			!strings.Contains(strings.Join(item.Evidence, " "), "未证明反转已发生或存在锁阻塞") || (item.ImpactBreakdown != nil) != effective {
			t.Fatalf("evidence fit erased the qualifier or raw projection acquired another ruler's split: %+v", item)
		}
		want := .020
		if effective {
			want = .009
		}
		if *item.ImpactSeconds != want {
			t.Fatalf("qualifying evidence must never reprice the selected value: %+v", item)
		}
	}
}
