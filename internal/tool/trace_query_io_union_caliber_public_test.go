package tool

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Every observation in this test must originate in a native TraceQuery call.
// The issuer's IO interval straddles the target's dependency window; it is
// useful adjacent evidence, not a completion-closed target blocking proof.
func TestTraceQueryIOUnionCaliberPublicNativeCarriage(t *testing.T) {
	ctx, path := businessRefTestContext(t, `# tracer: nop
idle-0 (0) [000] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=99 next_prio=20
idle-0 (0) [001] .... 5.000010: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=issuer next_pid=41 next_prio=120
issuer-41 (41) [001] .... 5.000100: block_rq_issue: 8,0 R 4096 () 123 + 8 [issuer]
issuer-41 (41) [001] .... 5.000120: sched_switch: prev_comm=issuer prev_pid=41 prev_prio=120 prev_state=D ==> next_comm=idle next_pid=0 next_prio=120
issuer-41 (41) [001] .... 5.000121: sched_blocked_reason: pid=41 iowait=1 caller=io_schedule
target-99 (99) [000] .... 5.000150: sched_switch: prev_comm=target prev_pid=99 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
irq-2 (2) [001] .... 5.000170: block_rq_complete: 8,0 R () 123 + 8 [0]
other-3 (3) [001] .... 5.000180: sched_wakeup: comm=issuer pid=41 prio=120 target_cpu=001
idle-0 (0) [001] .... 5.000190: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=issuer next_pid=41 next_prio=120
issuer-41 (41) [001] .... 5.000220: sched_wakeup: comm=target pid=99 prio=20 target_cpu=000
idle-0 (0) [000] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=99 next_prio=20
target-99 (99) [000] .... 5.001000: sched_switch: prev_comm=target prev_pid=99 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "root_cause_rank", "pid": 99,
		"time_start": 5, "time_end": 5.001, "min_duration_ms": .001, "limit": 64})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if payload.RootCauseRank == nil {
		t.Fatal("native query omitted rank")
	}
	if payload.WindowStats == nil || len(payload.WindowStats.IOLatencies) != 1 ||
		payload.WindowStats.IOLatencies[0].CompletionWokeIssuer || math.Abs(payload.WindowStats.IOLatencies[0].DurationMs-.070) > 1e-6 {
		t.Fatal("native request must remain .070ms residence without completion-to-issuer closure")
	}
	found := false
	unionType, unionMS := "", 0.0
	for _, row := range payload.RootCauseRank.Items {
		if row.Source == "window_stats.io_facet_family" {
			found = true
			unionType, unionMS = row.Type, row.CumulativeImpactMs
			if row.IOValueCaliber != types.TraceIOValueCaliberMixed || row.ChainRelevance != "adjacent" || row.ResourceCompletionClosure {
				t.Fatalf("native union lost mixed ruler: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("native fixture did not produce a facet union (not a product RED): %+v", payload.RootCauseRank.Items)
	}
	if unionType != "io_wait" || math.Abs(unionMS-.080) > 1e-6 {
		t.Fatalf("native fixture must publish the iowait-led .080ms union: type=%s value=%.9f", unionType, unionMS)
	}
	unionEvidence := ""
	for _, record := range result.Observations {
		if strings.Contains(strings.Join(record.RichNotes, "\n"), "source=window_stats.io_facet_family") {
			unionEvidence = record.ID
		}
	}
	if unionEvidence == "" {
		t.Fatal("native facet union has no published observation identity")
	}
	projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
	ledger := types.ObservationLedger{Records: result.Observations}
	contract, err := tracefinding.CompileCandidateContract(ledger, types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{projection}}, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, candidate := range contract.Candidates {
		ownsUnion := false
		for _, evidenceID := range candidate.Decision.EvidenceRefs {
			ownsUnion = ownsUnion || evidenceID == unionEvidence
		}
		if !ownsUnion {
			continue
		}
		found = true
		if candidate.PrimaryEligible || candidate.Decision.Magnitude == nil || math.Abs(candidate.Decision.Magnitude.Value-unionMS) > 1e-6 {
			t.Fatalf("native union changed value or became root eligible: %+v", candidate)
		}
		if parts := candidate.Decision.Magnitude.Components; parts == nil || parts.IOValueCaliber != types.TraceIOValueCaliberMixed {
			t.Errorf("candidate lost native union measurement: %+v", parts)
		}
		contract.RootCauseReportEnabled = true
		if _, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
			RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: candidate.Decision.CandidateID}}}, contract); err == nil {
			t.Fatal("adjacent union became a selectable root cause")
		}
	}
	if !found {
		t.Fatal("native union omitted from projection/candidate contract")
	}
	if report, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2}, contract); err != nil || len(report.RootCauses) != 0 {
		t.Fatalf("new measurement label must not fabricate a sidecar selection: %v %+v", err, report)
	}
	for _, language := range []string{"zh", "en"} {
		ctx.Language = language
		ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}, AnswerContract: types.AnswerContract{Language: language}}
		ctx.ToolResults = []types.ToolResult{result}
		params, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "summary", "kind": "summary", "surface_role": "principal", "trace_causal_claim_caliber": "no_causal_conclusion", "text": "The independent IO observations do not prove a new root cause."}}})
		emitted, err := (&EmitAnswerDocument{}).Execute(ctx, params)
		if err != nil || !emitted.Success {
			t.Fatalf("public emit failed: %v %s", err, emitted.Summary)
		}
		want := types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberMixed, language == "zh")
		for _, id := range []string{"runtime_trace_causal_projection", "runtime_trace_causal_projection_detail_full"} {
			block := projectionClusterBlock(ctx.Mutable.AnswerDocumentV2().Blocks, id)
			if block == nil || !strings.Contains(types.AnswerBlockVisibleSurface(*block), want) {
				t.Errorf("%s %s lost native union measurement %q", language, id, want)
			}
		}
	}
}
