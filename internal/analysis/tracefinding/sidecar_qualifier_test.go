package tracefinding

import (
	"encoding/json"
	"os"
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
	authority := SeatFrameCausalityAuthority{Applicable: true, Index: SeatFrameCausalityIndex{"E-A": true}}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), authority)
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
	// Gate open, no frame-unproven authority on any seat ⇒ explicit proven.
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), SeatFrameCausalityAuthority{Applicable: true})
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

// QUALGATE-1 (§40.30 V-QUAL-1 plan A): gate closed ⇒ every candidate and the
// contract ceiling say not_applicable — never proven — and the wire stays
// explicit with a bare summary; a not_applicable seat may still carry status
// proven (frame causality is orthogonal to on-chain causality explicitness).
func TestSidecarQualifierGateClosedIsNotApplicable(t *testing.T) {
	contract, err := CompileCandidateContract(types.ObservationLedger{}, sidecarQualifierSet(true), SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range contract.Candidates {
		if c.Decision.CausalQualifier != types.TraceCausalQualifierNotApplicable {
			t.Fatalf("gate closed must yield not_applicable, never proven: %+v", c.Decision)
		}
	}
	if contract.CausalCeiling != types.TraceCausalQualifierNotApplicable {
		t.Fatalf("derived ceiling must be not_applicable: %q", contract.CausalCeiling)
	}
	contract.RootCauseReportEnabled = true
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: contract.Candidates[0].Decision.CandidateID}}}, contract)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	if !strings.Contains(string(raw), `"causal_qualifier":"not_applicable"`) || strings.Contains(string(raw), "帧因果未证") {
		t.Fatalf("not_applicable must be explicit on the wire with a bare summary:\n%s", raw)
	}
}

// The gate reads ONLY the analyzer's typed frame decision on the ledger
// input's RequestModel: absent profile ⇒ closed (fail-closed), false ⇒ closed,
// true ⇒ open with the seat-level index behind it.
func TestBuildSeatFrameCausalityAuthorityGatesOnTypedRequestProfile(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{
		{ToolName: "trace_query", Success: true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{TypedCausalRowCount: 3, FrameEvidenceStatus: "absent"},
			Observations:           []types.ObservationRecord{{ID: "E-A", Origin: types.AnswerEvidenceOriginRuntimeArtifact}}},
	}}
	if a := BuildSeatFrameCausalityAuthority(input); a.Applicable || a.SeatQualifier("E-A") != types.TraceCausalQualifierNotApplicable || a.SeatFrameUnproven("E-A") {
		t.Fatalf("no typed profile ⇒ gate closed: %+v", a)
	}
	input.RequestModel = &types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: false}}
	if a := BuildSeatFrameCausalityAuthority(input); a.Applicable || a.SeatQualifier("E-A") != types.TraceCausalQualifierNotApplicable {
		t.Fatalf("frame_causality_requested=false ⇒ gate closed: %+v", a)
	}
	input.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = true
	a := BuildSeatFrameCausalityAuthority(input)
	if !a.Applicable || a.SeatQualifier("E-A") != types.TraceCausalQualifierFrameUnproven || !a.SeatFrameUnproven("E-A") ||
		a.SeatQualifier("E-Z") != types.TraceCausalQualifierProven {
		t.Fatalf("frame_causality_requested=true ⇒ gate open with the seat-level index: %+v", a)
	}
}

// The gate must never regress onto the keyword lane: the provider reads no
// request text, keyword list, or scenario label.
func TestSeatAuthorityProviderReadsNoKeywordLane(t *testing.T) {
	src, err := os.ReadFile("seat_authority.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"AnalyzerHints", "Keywords", "Entities", "RawRequest", ".Scenario", "strings.Contains(strings.ToLower"} {
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("seat_authority.go must not read the keyword/scenario lane (%q found)", forbidden)
		}
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
