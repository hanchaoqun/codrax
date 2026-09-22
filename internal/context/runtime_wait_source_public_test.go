package context_test

import (
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Legacy census text is a trace_query compatibility format, not a general
// syntax for promoting arbitrary successful tool output into measured waits.
func TestRuntimeWaitSourcePublicLegacyCensusProducer(t *testing.T) {
	for _, tc := range []struct {
		name, producer string
		success, want  bool
	}{
		{"trace_query_legacy", "trace_query", true, true},
		{"failed_trace_query", "trace_query", false, false},
		{"source_read", "read_file", true, false},
		{"command_output", "exec_command", true, false},
		{"model_completion", "emit_investigation_complete", true, false},
		{"unknown_producer", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, results := runtimeWaitSourcePublicBus(t)
			const subject = "legacy-owner-777"
			const caller = "legacy_wait_symbol"
			results = append(results, types.ToolResult{ToolName: tc.producer, Success: tc.success,
				Summary: "- blocked_reason " + subject + " iowait=1 count=7 line=8 caller=" + caller + "+0x10/0x20\n"})
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			before := artifactDenialSnapshot(t, bus.Mutable.TurnAArtifacts())
			for _, stage := range []types.PipelineStage{types.StageExplore, types.StageFinalize} {
				ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
				if !strings.Contains(ac.TraceWaitEvidence, "threadpool-400") {
					t.Fatal("fixture must retain the genuine native wakeup path")
				}
				got := artifactDenialSection(t, ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "wait-origin-test"}), ctxbuilder.SectionTraceWaitEvidence)
				for _, token := range []string{subject, caller + " ×7"} {
					if strings.Contains(got, token) != tc.want {
						t.Errorf("%s: legacy census source=%q success=%t: presence of %q=%t, want %t", stage, tc.producer, tc.success, token, strings.Contains(got, token), tc.want)
					}
				}
			}
			if after := artifactDenialSnapshot(t, bus.Mutable.TurnAArtifacts()); before != after {
				t.Fatal("context filtering must not erase original tool results")
			}
		})
	}
}

// Model aggregate provenance is an attribution string, not a producer badge.
// Exercise its real publication and ledger compilation before prompt assembly.
func TestRuntimeWaitSourcePublicModelAggregateIsNotMeasuredWait(t *testing.T) {
	for _, producer := range []string{"trace_query", "trace_query:run2", "model_emitted"} {
		t.Run(producer, func(t *testing.T) {
			bus, results := runtimeWaitSourcePublicBus(t)
			bus.ToolResults = results
			const subject = "model-only-owner-888"
			const caller = "model_only_wait_symbol"
			params := artifactDenialJSON(t, map[string]any{
				"result_kind": "resolved", "confidence": "high", "reason": "Retain a model-authored audit note separately from measured trace records.",
				"aggregate_facts": []map[string]any{{
					"kind": "scalar_value", "role": "audit_ledger", "label": "model wait hypothesis", "value": "1", "unit": "records",
					"provenance": producer, "support_refs": []string{"trace_query:window_stats"},
					"dimensions": []map[string]string{
						{"name": "origin", "value": "runtime_artifact"},
						{"name": "artifact_id", "value": "attached_trace"},
						{"name": "artifact_kind", "value": "trace"},
						{"name": "target", "value": subject},
						{"name": "predicate", "value": "blocked_reason_census"},
					},
					"members": []string{types.TraceNoteKeyBlockedReasonCensus + "=" + caller + "×1(Σ99.000ms)"},
				}},
			})
			result, err := (&tool.EmitInvestigationComplete{}).Execute(bus, params)
			if err != nil || !result.Success || !bus.Mutable.IsInvestigationComplete() {
				t.Fatalf("fixture aggregate publication did not complete: %v; %s", err, result.Summary)
			}
			facts := bus.Mutable.StableInvestigationAggregateFacts()
			if len(facts) != 1 || facts[0].Provenance != producer {
				t.Fatalf("fixture lost the published aggregate before the context boundary: %+v", facts)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results, AcceptedAggregateFacts: facts})
			ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
			found := false
			for _, r := range ledger.Records {
				if r.Subject == subject {
					found = true
					if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || r.ClaimAuthority != types.ObservationClaimAuthorityModelInference || r.Producer != producer {
						t.Fatalf("fixture model authority must survive ledger compilation: %+v", r)
					}
				}
			}
			if !found {
				t.Fatal("fixture model aggregate never reached the compiled ledger")
			}
			before := artifactDenialSnapshot(t, []any{bus.Mutable.StableInvestigationAggregateFacts(), bus.Mutable.TurnAArtifacts(), ledger})
			for _, stage := range []types.PipelineStage{types.StageExplore, types.StageFinalize} {
				ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, stage)
				if !strings.Contains(ac.TraceWaitEvidence, "threadpool-400") {
					t.Fatal("native measured wakeup facts must remain available")
				}
				got := artifactDenialSection(t, ctxbuilder.BuildPromptContext(ac, &skill.Config{Name: "wait-origin-test"}), ctxbuilder.SectionTraceWaitEvidence)
				if strings.Contains(got, subject) || strings.Contains(got, caller) || strings.Contains(got, "99.000ms") {
					t.Errorf("%s: model aggregate provenance=%q was promoted into native measured wait context", stage, producer)
				}
			}
			afterLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, 0))
			if after := artifactDenialSnapshot(t, []any{bus.Mutable.StableInvestigationAggregateFacts(), bus.Mutable.TurnAArtifacts(), afterLedger}); before != after {
				t.Fatal("runtime-only prompt filtering must preserve model aggregates and the original audit ledger")
			}
		})
	}
}

func runtimeWaitSourcePublicBus(t *testing.T) (*types.BusContext, []types.ToolResult) {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(artifactDenialFixture(t), "\n") {
		if !strings.Contains(line, "sched_blocked_reason:") {
			lines = append(lines, line)
		}
	}
	bus, path, rm := artifactDenialPublicBus(t, "legacy.systrace", strings.Join(lines, "\n"), 2, "threadpool-400")
	// Unspecified legacy scope retains window census at both dispatch stages.
	rm.RuntimeQuestionProfile = nil
	bus.Mutable.SetRequestModel(rm)
	bus.AnalysisIR.RequestModel = rm
	var results []types.ToolResult
	for _, view := range []string{"wakeup_chain", "window_stats"} {
		result, err := (&tool.TraceQuery{}).Execute(bus, artifactDenialJSON(t, map[string]any{
			"source": "path", "path": path, "view": view, "pid": 100,
			"time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace",
		}))
		if err != nil || !result.Success || len(result.Observations) == 0 {
			t.Fatalf("fixture native query failed: %v; %s", err, result.Summary)
		}
		for _, r := range result.Observations {
			for _, note := range r.RichNotes {
				if strings.HasPrefix(note, types.TraceNoteKeyBlockedReasonCensus+"=") {
					t.Fatal("fixture must exercise legacy fallback without typed census notes")
				}
			}
		}
		results = append(results, result)
	}
	return bus, results
}
