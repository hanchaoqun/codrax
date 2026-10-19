package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These captures exercise the public agent adapter boundary without a model
// request. The sentinel is returned before any network or generated answer.
func hmc266NegativePrompt(t *testing.T, ctx *types.AgentContext, stage string) string {
	t.Helper()
	name, targetStage := types.AgentFinalizer, types.StageFinalize
	if stage == "extract" {
		name, targetStage = types.AgentExtractor, types.StageExtract
	}
	ctx = ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentExplorer), name, targetStage)
	registry := tool.NewRegistry()
	tool.RegisterDefaults(registry)
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured HMC266 independent negative handoff")}
	deps := &Dependencies{LLM: capture, Tools: registry, MaxIterations: 1}
	var err error
	if stage == "extract" {
		ctx.Stage, ctx.AgentName = types.StageExtract, types.AgentExtractor
		_, err = NewExtractorAgent(deps).Execute(ctx, traceTeachingSkill(t, "extract-skill"))
	} else {
		ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
		_, err = NewFinalizerAgent(deps).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	}
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("expected one public adapter request: stage=%s calls=%d err=%v", stage, capture.calls, err)
	}
	var prompt strings.Builder
	for _, message := range capture.messages {
		prompt.WriteString(message.Content)
		prompt.WriteString("\n")
	}
	return prompt.String()
}

func hmc266NegativeContext(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx := readerFieldScopePublicContext("en", types.RuntimeQuestionScopeBoundedFactSet, nil)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "artifact", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
	ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}
	return ctx
}

func hmc266AcceptNegative(t *testing.T, ctx *types.AgentContext, origin string) *types.AgentContext {
	t.Helper()
	args, err := json.Marshal(map[string]any{
		"reason": "Query result remains unverified.", "confidence": "medium", "result_kind": "absence",
		"absence_justification": "No successful operation covers this requested predicate.",
		"aggregate_facts": []types.AnswerAggregateFact{{
			Kind: types.AnswerAggregateNegativeObservation, Label: "HMC266 pending predicate", Value: "0",
			Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "trace_query admission check",
			Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: origin},
				{Name: "target", Value: "pending-predicate"}, {Name: "scope", Value: "requested-surface-only"},
				{Name: "result_count", Value: "0"}, {Name: "searched_at", Value: "current_investigation"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("public accepted negative fixture: err=%v result=%+v", err, result)
	}
	// The stage dispatcher freezes accepted completion aggregates before
	// handing the same durable state to Extractor/Finalizer.
	ctx.Mutable.RetainInvestigationAggregateFacts()
	return ctxbuilder.BuildAgentContext(types.ToolBusContext(ctx, types.AgentExplorer), types.AgentFinalizer, types.StageFinalize)
}

func hmc266NegativeSnapshot(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ledger := answerDocObservationLedger(ctx)
	raw, err := json.Marshal([]any{ctx.Mutable.TurnAArtifacts(), ctx.Mutable.StableInvestigationAggregateFacts(),
		ledger, types.CompileTraceCausalProjectionSet(ledger),
		types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger), ctx.Mutable.AnswerDocumentV2()})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestHMC266NegativePublicUnexecutedClaimStaysAdvisory(t *testing.T) {
	for _, origin := range []string{"runtime_artifact", "mcp_resource", "command_measurement"} {
		for _, stage := range []string{"extract", "finalize"} {
			t.Run(origin+"/"+stage, func(t *testing.T) {
				ctx := hmc266NegativeContext(t)
				ctx = hmc266AcceptNegative(t, ctx, origin)
				ledger := answerDocObservationLedger(ctx)
				found := false
				for _, record := range ledger.Records {
					if !strings.HasPrefix(record.ID, "aggregate:") {
						continue
					}
					found = true
					if record.ClaimAuthority != types.ObservationClaimAuthorityModelInference || !record.Negative || record.ResultCount == nil || *record.ResultCount != 0 {
						t.Fatalf("fixture needs retained model-authored zero, not a native result: %+v", record)
					}
				}
				if !found {
					t.Fatalf("accepted model aggregate was not retained: stable=%+v ledger=%+v", ctx.Mutable.StableInvestigationAggregateFacts(), ledger)
				}
				before := hmc266NegativeSnapshot(t, ctx)
				prompt := hmc266NegativePrompt(t, ctx, stage)
				found = false
				for _, line := range strings.Split(prompt, "\n") {
					if !strings.HasPrefix(line, "- kind=`negative_observation`") || !strings.Contains(line, "pending-predicate") {
						continue
					}
					found = true
					if !strings.Contains(line, "fact_authority=`advisory_model_inference`") {
						t.Errorf("public %s projected an unexecuted absence without per-fact authority: %s", stage, line)
					}
				}
				if !found {
					t.Error("advisory receipt disappeared instead of being qualified")
				}
				if strings.Contains(prompt, "`kind=negative_observation` is the typed lane for a verified zero-result") {
					t.Error("generic kind teaching still upgrades every accepted negative claim into verified zero")
				}
				if hmc266NegativeSnapshot(t, ctx) != before {
					t.Fatal("prompt display mutated durable model receipt, measured facts, causal or source authority")
				}
			})
		}
	}
}

func TestHMC266NegativePublicClaimBindingDoesNotUpgradeModelZero(t *testing.T) {
	for _, origin := range []string{"runtime_artifact", "mcp_resource", "command_measurement"} {
		t.Run(origin, func(t *testing.T) {
			ctx := hmc266NegativeContext(t)
			ctx = hmc266AcceptNegative(t, ctx, origin)
			before := hmc266NegativeSnapshot(t, ctx)
			prompt := hmc266NegativePrompt(t, ctx, "finalize")
			found := false
			for _, line := range strings.Split(prompt, "\n") {
				if !strings.HasPrefix(line, "- `aggregate_facts[") || !strings.Contains(line, "kind=`negative_observation`") {
					continue
				}
				found = true
				if !strings.Contains(line, "advisory_model_inference") || strings.Contains(line, "authority_ceiling=`factual`") || strings.Contains(line, "authority_ceiling=`historical`") {
					t.Errorf("binding display promoted model-only negative to factual/historical evidence: %s", line)
				}
			}
			if !found {
				t.Fatal("model binding audit receipt disappeared")
			}
			if hmc266NegativeSnapshot(t, ctx) != before {
				t.Fatal("binding display altered original claim or hard authority")
			}
		})
	}
}

func TestHMC266NegativePublicNativeLogAndMCPDoNotNeedTraceQuery(t *testing.T) {
	for _, lane := range []string{"log", "mcp"} {
		for _, stage := range []string{"extract", "finalize"} {
			t.Run(lane+"/"+stage, func(t *testing.T) {
				ctx := hmc266NegativeContext(t)
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
				ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
				var want []string
				if lane == "mcp" {
					ctx.MCPResponses = []types.MCPResponse{{ServerName: "fixture", Method: "tools/call:status", Success: true,
						ResourceURI: "mcp://fixture/status", MIMEType: "application/vnd.codrax.observation+json",
						Observations: []types.MCPTypedObservation{{Summary: "HMC266_remote_status_zero_jobs", ResourceURI: "mcp://fixture/status", LineStart: 12, LineEnd: 12, Selector: "jobs=zero"}}}}
					ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{MCPResponses: ctx.MCPResponses})
					want = []string{"HMC266_remote_status_zero_jobs", "mcp://fixture/status"}
				} else {
					ctx.LogTriage = &types.LogBundle{Observations: []types.LogObservation{{Kind: types.LogObservationRetryCycle,
						Subject: "HMC266_log_record", Summary: "HMC266_logged_no_pending_jobs", Evidence: "pending_jobs=0", LineStart: 12, LineEnd: 12}}}
					ctx.Mutable.SetLogTriage(ctx.LogTriage)
					want = []string{"HMC266_logged_no_pending_jobs", "pending_jobs=0"}
				}
				ctx = hmc266AcceptNegative(t, ctx, "runtime_artifact")
				ledger := answerDocObservationLedger(ctx)
				native := 0
				for _, record := range ledger.Records {
					if strings.HasPrefix(record.ID, "aggregate:") {
						continue
					}
					native++
					if record.ClaimAuthority == types.ObservationClaimAuthorityModelInference {
						t.Fatalf("direct external source misclassified as a model absence: %+v", record)
					}
				}
				if native == 0 {
					t.Fatal("fixture needs direct log/MCP originals independent from model negative")
				}
				before := hmc266NegativeSnapshot(t, ctx)
				prompt := hmc266NegativePrompt(t, ctx, stage)
				for _, value := range want {
					// Existing Extractor has no MCP body presentation surface;
					// it only carries the original response forward. Protect that
					// durable path here; Finalizer already exposes the native row.
					surface := prompt
					if lane == "mcp" && stage == "extract" {
						surface = hmc266NegativeSnapshot(t, ctx)
					}
					if !strings.Contains(surface, value) {
						t.Errorf("public %s lost direct %s data %q", stage, lane, value)
					}
				}
				if strings.Contains(prompt, "requires trace_query") {
					t.Fatal("legitimate log/MCP source became dependent on a trace query")
				}
				if before != hmc266NegativeSnapshot(t, ctx) {
					t.Fatal("display changed original log/MCP fields, model record, source or causal authority")
				}
			})
		}
	}
}

func TestHMC266NegativePublicRealZeroQueryKeepsExactScope(t *testing.T) {
	ctx := hmc266NegativeContext(t)
	path := filepath.Join(ctx.RepoRoot, "empty-query.ftrace")
	if err := os.WriteFile(path, []byte("writer-101 (101) [001] .... 6.000000: tracing_mark_write: I|101|observed-present-event\n"), 0600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search",
		"pattern": "genuinely-absent-event", "time_start": 6.0, "time_end": 6.1, "limit": 20})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("actual query must succeed before claiming zero: %v %+v", err, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx = hmc266AcceptNegative(t, ctx, "runtime_artifact") // unrelated model claim cannot erase the real query
	before := hmc266NegativeSnapshot(t, ctx)
	prompt := hmc266NegativePrompt(t, ctx, "finalize")
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || views[0].Inventory.Coverage.MatchedTotal != 0 || !views[0].Inventory.RowsComplete || len(views[0].Inventory.Rows) != 0 {
		t.Fatalf("real zero query lost its complete inventory: %+v", views)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if views[0].Source.Path != canonicalPath || views[0].Source.QueryWindowStartTs != 6 || views[0].Source.QueryWindowEndTs != 6.1 || !strings.Contains(prompt, "genuinely-absent-event") {
		t.Fatalf("verified zero escaped its exact source/predicate/window: %+v", views[0])
	}
	if hmc266NegativeSnapshot(t, ctx) != before {
		t.Fatal("zero display changed originals or causal authority")
	}
}
