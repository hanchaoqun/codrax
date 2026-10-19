package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc265ReadinessFinalizerPrompt(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured HMC265 readiness handoff")}
	_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("expected one actual finalizer adapter request, calls=%d err=%v", capture.calls, err)
	}
	var prompt strings.Builder
	for _, message := range capture.messages {
		prompt.WriteString(message.Content)
		prompt.WriteString("\n")
	}
	return prompt.String()
}

func hmc265RawReadContext(t *testing.T, sourceProof bool) *types.AgentContext {
	t.Helper()
	ctx := boundedRuntimeReaderHandoffTestContext()
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{
		types.RuntimeQuestionFactTargetWaitOccurrences, types.RuntimeQuestionFactCountOrDuration,
	}
	path := filepath.Join(ctx.RepoRoot, "capture.systrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\nworker-7 (7) [000] .... 2.010000: tracing_mark_write: I|7|raw material only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"path": path})
	result, err := (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || result.RuntimeArtifactRead == nil {
		t.Fatalf("public raw artifact read: %v / %+v", err, result)
	}
	results := []types.ToolResult{result}
	if sourceProof {
		path = filepath.Join(ctx.RepoRoot, "README.md")
		if err := os.WriteFile(path, []byte("# Unrelated source material\n"), 0600); err != nil {
			t.Fatal(err)
		}
		params, _ = json.Marshal(map[string]any{"path": path})
		result, err = (&tool.ReadFile{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success || result.ReadCoverage == nil {
			t.Fatalf("public source read: %v / %+v", err, result)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

func TestHMC265ReadinessActualFinalizerSeparatesRawReadFromFacts(t *testing.T) {
	for _, sourceProof := range []bool{false, true} {
		t.Run(map[bool]string{false: "raw_only", true: "raw_and_source"}[sourceProof], func(t *testing.T) {
			ctx := hmc265RawReadContext(t, sourceProof)
			ledger := answerDocObservationLedger(ctx)
			before := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)
			if before.RuntimeObservationCount != 1 || before.DeterministicRuntimeQueryCount != 0 || before.CurrentSourceSatisfied != sourceProof {
				t.Fatalf("fixture must reproduce read-only runtime observations: %+v", before)
			}
			prompt := hmc265ReadinessFinalizerPrompt(t, ctx)
			for _, forbidden := range []string{"combined_ready=", "combined_proof_ready", "runtime and current-source proof are both present", "## 有限窗口查询的读者事实卡"} {
				if strings.Contains(prompt, forbidden) {
					t.Errorf("raw material gained query/fact readiness %q", forbidden)
				}
			}
			for _, want := range []string{"source-lane availability only", "not tool invocation counts", "artifact_read_records=1", "deterministic_query_records=0", "do not establish requested fact coverage"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finalizer lacks scoped readiness boundary %q", want)
				}
			}
			after := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, answerDocObservationLedger(ctx))
			if !reflect.DeepEqual(before, after) {
				t.Fatal("prompt calibration changed source obligations or completion authority")
			}
		})
	}
}

func TestHMC265ReadinessRequestedFamilyDoesNotBecomeAvailableFact(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := boundedRuntimeReaderHandoffTestContext()
			ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
			artifacts := ctx.Mutable.TurnAArtifacts()
			artifacts.ToolResults[0].Observations = artifacts.ToolResults[0].Observations[:1] // actual state only
			artifacts.ToolResults[0].TraceEvidenceAuthority = nil
			ctx.Mutable.SetTurnAArtifacts(*artifacts)
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{
				types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactTargetWaitOccurrences,
				types.RuntimeQuestionFactFrequencyResidency,
			}
			got := renderAnswerDocBoundedRuntimeFinalReaderHandoff(ctx)
			if !strings.Contains(got, "157.248") {
				t.Fatal("known state facts disappeared")
			}
			for _, absent := range []string{"目标线程的调度器标记等待清单", "已观测频率的持续时间与覆盖分布", "target scheduler-marked wait roster", "observed frequency duration and coverage distribution"} {
				if strings.Contains(got, absent) {
					t.Errorf("requested but unpublished family was presented as a visible fact: %s", absent)
				}
			}
		})
	}
}

func TestHMC265ReadinessActualFinalizerKeepsZeroWaitAndCausalEvidence(t *testing.T) {
	ctx := boundedRuntimeReaderHandoffTestContext()
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	prompt := hmc265ReadinessFinalizerPrompt(t, ctx)
	for _, want := range []string{"调度器标记等待清单：共 0 次", "运行 157.248 毫秒", "35.960", "558000–2100000 kHz"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual known fact lost: %s", want)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("readiness handoff mutated measured facts")
	}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
	if !strings.Contains(renderAnswerDocRuntimeTraceAnswerGuidance(ctx), "Scheduler transition interval hint:") {
		t.Fatal("causal teaching disappeared")
	}
}

func TestHMC265ReadinessLegitimateExternalLanesDoNotNeedTraceQuery(t *testing.T) {
	for _, origin := range []types.AnswerEvidenceOrigin{types.AnswerEvidenceOriginRuntimeArtifact, types.AnswerEvidenceOriginMCPResource} {
		t.Run(string(origin), func(t *testing.T) {
			ctx := hmc265RawReadContext(t, false)
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
			ctx.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}
			ctx.TurnRouteHint = types.TurnRouteHint{Route: "repo", Source: "artifact", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			record := types.ObservationRecord{ID: "external-observed-fact", Origin: origin, Producer: "log_triage", Summary: "recorded failure status", SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/log.txt"}, Span: types.ObservationSpan{LineStart: 12, LineEnd: 12}}
			if origin == types.AnswerEvidenceOriginMCPResource {
				record.Producer = "mcp:fixture"
				record.SourceRef.Kind = types.ObservationSourceMCPResource
				record.SourceRef.ResourceURI = "mcp://fixture/status"
			}
			ledger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
			before := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)
			if before.DeterministicRuntimeQueryCount != 0 || !before.RuntimeOnlySufficient {
				t.Fatalf("fixture must carry legitimate non-trace support: %+v", before)
			}
			got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
			if !strings.Contains(got, "external observation support is available within its own recorded scope") || strings.Contains(got, "requires trace_query") {
				t.Errorf("legal log/MCP support lacks its own bounded evidence scope: %s", got)
			}
			if !reflect.DeepEqual(before, types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)) {
				t.Fatal("display changed external-lane authority")
			}
		})
	}
}

func TestHMC265ReadinessActualFinalizerKeepsMCPFactWithoutTrace(t *testing.T) {
	ctx := &types.AgentContext{
		RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "en",
		Mutable:       types.NewMutableState("inspect the remote status"),
		TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "artifact", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Language: "en", Intent: types.IntentRootCause,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly},
		}},
		MCPResponses: []types.MCPResponse{{
			ServerName: "fixture", Method: "tools/call:status", Success: true,
			ResourceURI: "mcp://fixture/status", MIMEType: "application/vnd.codrax.observation+json",
			Observations: []types.MCPTypedObservation{{Summary: "remote-job-status-failed", ResourceURI: "mcp://fixture/status", LineStart: 12, LineEnd: 12, Selector: "job=seven"}},
		}},
	}
	prompt := hmc265ReadinessFinalizerPrompt(t, ctx)
	for _, want := range []string{"remote-job-status-failed", "mcp_records=1", "deterministic_query_records=0", "external observation support is available within its own recorded scope"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual external fact handoff lost %q", want)
		}
	}
	if strings.Contains(prompt, "combined_ready=") || strings.Contains(prompt, "requires trace_query") {
		t.Fatal("MCP evidence was misrepresented as whole-task or trace-query readiness")
	}
}

func TestHMC265ReadinessCountsRecordsNotQueriesAndUsesSameFallbackLedger(t *testing.T) {
	ctx := boundedRuntimeReaderHandoffTestContext()
	ledger := answerDocObservationLedger(ctx)
	if len(ledger.Records) < 2 {
		t.Fatal("fixture needs multiple facts from one tool result")
	}
	got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
	if !strings.Contains(got, "not distinct queries") || !strings.Contains(got, "not tool invocation counts") {
		t.Fatal("multiple result rows still masquerade as multiple queries")
	}
	if fallback := renderAnswerDocRuntimeSourceAuthority(ctx, types.ObservationLedger{}); fallback != got {
		t.Fatal("authority and stage counts used different ledgers for empty-input compatibility")
	}
}

func TestHMC265ReadinessExternalPresenceIsNotPolicySufficiency(t *testing.T) {
	for _, count := range []int{0, 1, 9} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx := &types.AgentContext{TurnRouteHint: types.TurnRouteHint{Route: "repo", Source: "mixed", NeedsRepoAccess: true},
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause,
					ExternalObservationPolicy: &types.ExternalObservationPolicy{ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly}}}}
			var records []types.ObservationRecord
			for n := 0; n < count; n++ {
				records = append(records, types.ObservationRecord{
					ID: fmt.Sprintf("mcp:%d", n), Origin: types.AnswerEvidenceOriginMCPResource,
					SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceMCPResource, ResourceURI: "mcp://fixture/status"},
					Span:      types.ObservationSpan{LineStart: n + 1}, Summary: "remote observed value",
				})
			}
			ledger := types.ObservationLedger{Records: records}
			authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)
			if authority.RuntimeOnlySufficient != (count == 1) {
				t.Fatalf("fixture must distinguish empty/small/broad external sets: %+v", authority)
			}
			got := renderAnswerDocRuntimeSourceAuthority(ctx, ledger)
			if !strings.Contains(got, fmt.Sprintf("external_observation_present=%t", count > 0)) {
				t.Fatalf("presence derived from policy rather than actual records: %s", got)
			}
			if !reflect.DeepEqual(authority, types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, ledger)) {
				t.Fatal("display changed external sufficiency")
			}
		})
	}
	if answerDocHasExternalObservationRecord(types.ObservationLedger{Records: []types.ObservationRecord{{Origin: types.AnswerEvidenceOriginMCPResource, SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRecoveryNavigation}}}}) {
		t.Fatal("historical navigation became a current external observation")
	}
}
