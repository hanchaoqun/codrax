package tracefinding

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// sidecar_qualifier_test.go — SIDECAR-Q1 (user ruling 2026-09-02,
// colleague_merge_audit_20260802.md §40.28 ②): the public v2 sidecar carries
// an ALWAYS-EXPLICIT seat-level causal_qualifier and impact_caliber, both
// bound from the frozen typed contract; the qualifier comes from the same
// evidence-ID authority index the Markdown crown face consults (never the
// session-wide ANY aggregate), and its summary wears the headline's exact
// words 「（帧因果未证）」.

func sidecarQualifierSet(effectiveA bool) types.TraceCausalProjectionSet {
	a := types.TraceCausalProjectionNode{EvidenceID: "E-A", Subject: "RenderThread", Rank: 1, TypeToken: "scheduler_latency",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMS: 12, EffectiveImpactMS: 8.5, EffectiveImpactPublished: effectiveA}
	b := types.TraceCausalProjectionNode{EvidenceID: "E-B", Subject: "GLThread", Rank: 2, TypeToken: "runnable_wait",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ImpactMS: 3}
	return types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		WindowStartTs: 1, WindowEndTs: 1.016,
		RankedSeats: []types.TraceCausalProjectionNode{a, b},
	}}}
}

func TestSidecarQualifierIsSeatLevelAndAlwaysExplicit(t *testing.T) {
	// Only seat A's evidence carries the frame-unproven authority.
	index := SeatFrameCausalityIndex{"E-A": true}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), index)
	if err != nil || len(contract.Candidates) != 2 {
		t.Fatalf("compile: %v %+v", err, contract)
	}
	byID := map[string]types.TraceCauseDecision{}
	for _, c := range contract.Candidates {
		byID[c.Decision.SubjectName] = c.Decision
	}
	if byID["RenderThread"].CausalQualifier != types.TraceCausalQualifierFrameUnproven {
		t.Fatalf("seat A (frame-unproven evidence) must be qualified: %+v", byID["RenderThread"])
	}
	if byID["GLThread"].CausalQualifier != types.TraceCausalQualifierProven {
		t.Fatalf("seat B (clean evidence) must stay proven — the qualifier is seat-level, never session-wide: %+v", byID["GLThread"])
	}
	if contract.CausalCeiling != types.TraceCausalQualifierFrameUnproven {
		t.Fatalf("the contract ceiling is DERIVED from the seats (unproven iff any admitted seat is): %q", contract.CausalCeiling)
	}
	contract.RootCauseReportEnabled = true
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{
			{CandidateID: byID["RenderThread"].CandidateID},
			{CandidateID: byID["GLThread"].CandidateID},
		}}, contract)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	wire := string(raw)
	for _, want := range []string{
		`"impact_caliber":"effective_attribution"`, `"causal_qualifier":"frame_unproven"`,
		`"impact_caliber":"window_projection"`, `"causal_qualifier":"proven"`,
		"RenderThread线程CPU调度延迟（帧因果未证）",
		"链上有效归因为 8.500 ms", "窗内投影占用为 3.000 ms（未发布有效归因）",
	} {
		if !strings.Contains(wire, want) {
			t.Fatalf("wire missing %q:\n%s", want, wire)
		}
	}
	if strings.Contains(wire, "GLThread线程CPU调度延迟（帧因果未证）") || strings.Contains(wire, "有效影响") {
		t.Fatalf("proven seat must not wear the qualifier and no evidence sentence may call a window projection 有效:\n%s", wire)
	}
	if strings.Count(wire, `"schema_version":2`) != 1 {
		t.Fatalf("append-only extension keeps schema_version 2:\n%s", wire)
	}
}

func TestSidecarQualifierEmptyIndexMeansProven(t *testing.T) {
	// No authority ⇒ no qualifier claim; still explicit on the wire.
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range contract.Candidates {
		if c.Decision.CausalQualifier != types.TraceCausalQualifierProven {
			t.Fatalf("empty index must yield an explicit proven qualifier: %+v", c.Decision)
		}
	}
	if contract.CausalCeiling != types.TraceCausalQualifierProven {
		t.Fatalf("derived ceiling must be proven: %q", contract.CausalCeiling)
	}
}

func TestBuildSeatFrameCausalityIndexKeysOnlyFrameUnprovenTraceResults(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{
		{ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{TypedCausalRowCount: 3, FrameEvidenceStatus: "absent"},
			Observations:           []types.ObservationRecord{{ID: "E-A", Origin: types.AnswerEvidenceOriginRuntimeArtifact}}},
		{ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{TypedCausalRowCount: 3, FrameEvidenceStatus: "present"},
			Observations:           []types.ObservationRecord{{ID: "E-B", Origin: types.AnswerEvidenceOriginRuntimeArtifact}}},
		// A zero-row exploratory probe never taints anything (the T3-1 class).
		{ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{TypedCausalRowCount: 0, FrameEvidenceStatus: "absent"},
			Observations:           []types.ObservationRecord{{ID: "E-C", Origin: types.AnswerEvidenceOriginRuntimeArtifact}}},
	}}
	index := BuildSeatFrameCausalityIndex(input)
	if !index.SeatFrameUnproven("E-A") || index.SeatFrameUnproven("E-B") || index.SeatFrameUnproven("E-C") {
		t.Fatalf("index must key exactly the frame-unproven trace results' evidence: %+v", index)
	}
	if index.SeatFrameUnproven("E-B", "E-A") != true || SeatFrameCausalityIndex(nil).SeatFrameUnproven("E-A") {
		t.Fatalf("membership must be any-of over the seat's ids and empty index must be false")
	}
}
