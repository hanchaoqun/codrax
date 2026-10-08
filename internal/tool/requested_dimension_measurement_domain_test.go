package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise producer -> dispatch -> shared authority -> public completion, not
// just a manually assembled snapshot. The view is not an applicability switch.
func TestEmitInvestigationCompleteMeasurementDomainIgnoresSoftSourceIntent(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	for _, tc := range []struct {
		name, fixture, view string
		start, end          float64
		adjust              func(*types.BusContext)
		wantSource          bool
		omitQuery           bool
		modelOnly           bool
		assertSourceRead    bool
	}{
		{name: "CPU distribution", fixture: "hmosperf_cpu_state_frequency", view: "cpu_state_frequency", start: 1, end: 1.04},
		{name: "IO distribution", fixture: "hmosperf_io_activity", view: "window_stats", start: 2, end: 2.25},
		{name: "attachment and triage only", wantSource: true, omitQuery: true},
		{name: "model aggregate borrows real query path and payload", fixture: "hmosperf_io_activity", view: "window_stats", start: 2, end: 2.25, wantSource: true, modelOnly: true},
		{name: "unrelated README", fixture: "hmosperf_io_activity", view: "window_stats", start: 2, end: 2.25, assertSourceRead: true, adjust: func(ctx *types.BusContext) {
			ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{ID: "source:README", Kind: types.EvidenceDirect,
				Source: "README.md", LineStart: 1, Snippet: "# Project overview", Summary: "unrelated repository overview", GroundingStatus: types.GroundingGrounded})
		}},
		{name: "independent source outcome", fixture: "hmosperf_io_activity", view: "window_stats", start: 2, end: 2.25, wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.TurnRouteHint.RequiredOutcomes = types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation
		}},
		{name: "precise Makefile binding", fixture: "hmosperf_io_activity", view: "window_stats", start: 2, end: 2.25, wantSource: true, adjust: func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "Makefile", Confidence: 1, RequestedDimensionIndices: []int{4}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := runtimeDimensionSourceContext()
			ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = nil
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired}
			if !tc.omitQuery {
				path, err := filepath.Abs("../../eval/fixtures/" + tc.fixture + "/events.systrace")
				if err != nil {
					t.Fatal(err)
				}
				params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": tc.view, "time_start": tc.start, "time_end": tc.end})
				query, err := (&TraceQuery{}).Execute(ctx, params)
				if err != nil || !query.Success || len(query.Observations) == 0 {
					t.Fatalf("query must publish addressable producer facts: %v %+v", err, query)
				}
				if tc.modelOnly {
					// The file and query payload exist, but no current producer
					// receipt is in this turn. Provenance is model text, not proof.
					ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar,
						Label: "model distribution", Value: "8", Provenance: "trace_query", Dimensions: []types.AnswerAggregateDimension{
							{Name: "origin", Value: "runtime_artifact"}, {Name: "path", Value: path}, {Name: "payload_ref", Value: query.RawRef},
						}}})
					ctx.Mutable.RetainInvestigationAggregateFacts()
					ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
					found := false
					for _, record := range ledger.Records {
						if record.Producer == "trace_query" && record.SourceRef.Path == path && record.SourceRef.PayloadRef == query.RawRef {
							found = true
							if record.ClaimAuthority != types.ObservationClaimAuthorityModelInference {
								t.Fatalf("omitting authority in model aggregate must compile as model_inference: %+v", record)
							}
						}
					}
					if !found || query.RawRef == "" {
						t.Fatal("fixture must retain an addressable model aggregate borrowing a real payload")
					}
				} else {
					ctx.Mutable.AppendDispatchToolResult(query)
				}
			}
			ctx.Mutable.SetPerfTrace(ctx.AnalysisIR.RequestModel.PerfTrace)
			if tc.adjust != nil {
				tc.adjust(ctx)
			}
			before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
			if tc.assertSourceRead && !authority.CurrentSourceSatisfied {
				t.Fatalf("README fixture must actually land current-source proof: %+v", authority)
			}
			if (authority.AddressableDeterministicRuntimeQueryCount == 0) != (tc.omitQuery || tc.modelOnly) {
				t.Fatalf("producer address did not reach shared authority: %+v", authority)
			}
			skip := renderEmitEvidenceExternalObservationSoftSkipSummary(ctx, []string{"observed measurements"})
			if strings.Contains(skip, "call emit_investigation_complete directly") == tc.wantSource {
				t.Fatalf("advisory disagrees with operation applicability: wantSource=%v %s", tc.wantSource, skip)
			}
			res, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"The producer-owned measurements and coverage answer the observation surfaces.","confidence":"high","result_kind":"resolved"}`))
			if err != nil || !res.Success {
				t.Fatalf("completion call: %v %+v", err, res)
			}
			if strings.Contains(res.Summary, "DOWNGRADED") != tc.wantSource || (ctx.Mutable.InvestigationCompleteReason() == "") != tc.wantSource {
				t.Fatalf("public completion applicability wantSource=%v: %+v", tc.wantSource, res)
			}
			if ctx.Mutable.EvidenceClosure().HasCompletionCaveat(types.DowngradeLaneContractChain) {
				t.Fatal("operation applicability must not rely on convergence escape")
			}
			after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
			if string(before) != string(after) {
				t.Fatal("domain selection changed dimensions or minted source exclusion")
			}
		})
	}
}
