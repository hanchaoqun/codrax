package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocUnifiedIntentContract_HistoryNarrativeDoesNotAskForScalar(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioGeneric,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"Evidence Origin / Requested Output Boundary",
		"`vcs_metadata`",
		"`summary`, `mechanism`",
		"not a request to emit a `scalar` block",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("history narrative intent contract prompt missing %q:\n%s", want, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "requested outputs:") && strings.Contains(line, "`scalar`") {
			t.Fatalf("history narrative should not list scalar as requested output:\n%s", got)
		}
	}
}

func TestRenderAnswerDocUnifiedIntentContract_HistoryDiagramShowsMixedOrigins(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"`current_source`, `vcs_metadata`, `vcs_diff`",
		"`summary`, `mechanism`, `diagram`",
		"Keep historical diff facts and current-checkout source facts in separate lanes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("history diagram intent contract prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocUnifiedIntentContract_RuntimeArtifactAvoidsRepoCitationPressure(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"`runtime_artifact`",
		"`summary`, `diagnostic`",
		"without current-repo citations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime artifact intent contract prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`current_source`") {
		t.Fatalf("external-only runtime artifact should not list current_source:\n%s", got)
	}
}

func TestRenderAnswerDocUnifiedIntentContract_PerfTraceAvoidsRepoCitationPressure(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
				PerfTrace: &types.PerfBundle{
					Frames: []types.PerfFrame{{FrameNo: 1, DurationMs: 33.3, Janky: true}},
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析 trace"},
					Confidence:        0.9,
				},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"`runtime_artifact`",
		"`summary`, `diagnostic`",
		"without current-repo citations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("perf trace intent contract prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`current_source`") {
		t.Fatalf("external-only perf trace should not list current_source:\n%s", got)
	}
}

func TestRenderAnswerDocClaimBindings_ShowsOriginSpecificPolicies(t *testing.T) {
	mut := types.NewMutableState("history count")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "history count",
		Value: "3",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
			{Name: "measurement_origin", Value: "command_measurement"},
		},
	}})
	mut.SetInvestigationComplete("done")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
					IsCountQuestion: true,
					IsScalarAnswer:  true,
				},
			},
		},
	}
	got := renderAnswerDocClaimBindings(ctx)
	for _, want := range []string{
		"Claim Binding / Gate Policy Handoff",
		"origin=`vcs_metadata`; policy=`repairable`",
		"origin=`command_measurement`; policy=`hard`",
		"authority_ceiling=`historical`",
		"outputs=`summary`, `scalar`, `count`",
		"ledger_records=aggregate:0#vcs_metadata",
		"ledger_records=aggregate:0#command_measurement",
		"Do not translate non-`current_source` bindings into current-source file:line requirements",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("claim binding prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocClaimBindings_RuntimeArtifactWithoutAggregateFacts(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{
						Type: "panic",
						Frames: []types.LogFrame{{
							Raw:  "panic at src/main.go:9",
							File: "src/main.go",
							Line: 9,
						}},
					}},
				},
			},
		},
	}

	got := renderAnswerDocClaimBindings(ctx)
	for _, want := range []string{
		"Claim Binding / Gate Policy Handoff",
		"origin=`runtime_artifact`; policy=`repairable`",
		"source=`log_triage`",
		"support_refs=1",
		"authority_ceiling=`historical`",
		"ledger_records=log:error:0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime artifact claim binding prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "aggregate_facts[-1]") {
		t.Fatalf("runtime artifact binding should not render a fake aggregate_facts index:\n%s", got)
	}
}

func TestRenderAnswerDocClaimBindings_SameCaptureQueryShadowsLegacyPreTriageObservation(t *testing.T) {
	const materializedCapture = "/repo/.codrax/blob/run/attached_trace.txt"
	perf := &types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{
			{
				// Production emit_perf_trace cannot set Authority; zero must be
				// treated as model-extracted navigation, not causal material.
				Subject:    "model dependency candidate",
				Summary:    "target waited for peer completion",
				LineStart:  5,
				LineEnd:    7,
				DurationMs: 8.5,
			},
			{
				Authority: types.PerfObservationAuthorityDeterministicValidator,
				Subject:   "typed priority semantics",
				Summary:   "larger numeric priority is higher",
				LineStart: 1,
			},
		},
		Stalls: []types.PerfStall{
			{
				// Legacy emit_perf_trace rows also carry zero authority.
				// Their model-selected symbol must not survive through the
				// parallel claim-binding surface after an exact same-capture query.
				Symbol:     "model stall candidate",
				Kind:       "io",
				StartTsMs:  2,
				DurationMs: 4,
			},
			{
				Authority:  types.PerfObservationAuthorityDeterministicValidator,
				Symbol:     "typed validator stall",
				Kind:       "io",
				StartTsMs:  3,
				DurationMs: 5,
			},
		},
	}
	validatorObservation := perf.Observations[len(perf.Observations)-1]
	perf.Observations = perf.Observations[:len(perf.Observations)-1]
	for i := 0; i < 12; i++ {
		perf.Observations = append(perf.Observations, types.PerfObservation{
			Subject: fmt.Sprintf("shadowed navigation %02d", i),
		})
	}
	// Keep the validator row after more than the prompt cap's worth of
	// shadowed candidates. Filtering must happen before the cap is applied.
	perf.Observations = append(perf.Observations, validatorObservation)
	mut := types.NewMutableState("analyze trace")
	mut.SetPerfTrace(perf)
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:       "trace-query:window#root_cause_primary:1",
			Origin:   types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query",
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         materializedCapture,
				ArtifactID:   "attached_trace",
				ArtifactKind: "trace",
			},
			Subject:   "worker-200",
			Predicate: "root_cause_primary",
			Summary:   "typed on-chain candidate",
		}},
	})
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Mutable:   mut,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind: "trace", Source: "(inline)", Carrier: "attachment",
		}}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			PerfTrace: perf,
		}},
	}

	got := renderAnswerDocClaimBindings(ctx)
	if strings.Contains(got, "model dependency candidate") ||
		strings.Contains(got, "target waited for peer completion") ||
		strings.Contains(got, "model stall candidate") {
		t.Fatalf("same-capture deterministic query must shadow legacy pre-triage claim material:\n%s", got)
	}
	if !strings.Contains(got, "typed priority semantics") ||
		!strings.Contains(got, "typed validator stall") ||
		!strings.Contains(got, "policy=`repairable`") {
		t.Fatalf("validator-owned perf observation should remain available:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_RendersLegacyVCSNarrativeAsAuditOnly(t *testing.T) {
	mut := types.NewMutableState("latest feature")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary:  "[git_log: evidence_origin=vcs_metadata count=1]\nabc123 Add observation ledger feature and preserve downstream impact notes",
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"## Observation Ledger",
		"`tool:0#vcs_metadata`",
		"origin=`vcs_metadata`",
		"producer=`git_log`",
		"role=`audit_ledger`",
		"policy=`display_only`",
		"legacy_summary_fallback=true; not_answer_grade=true",
		"abc123 Add observation ledger feature",
		"Do not turn non-`current_source` observations into source `file:line` citation requirements",
		"Do not expand abbreviated display strings such as git `--stat` paths containing `...`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "value=\"abc123\"") {
		t.Fatalf("history narrative ledger must not collapse the answer to a scalar commit id:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_RendersTypedVCSHistoryAsOriginSpecificSupport(t *testing.T) {
	mut := types.NewMutableState("latest feature")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		Summary:  "[git_log: evidence_origin=runtime_artifact count=99]\nrendered text is not the ledger source",
		VCSHistory: &types.ToolVCSHistory{
			Kind:     types.ToolVCSHistoryKindGitLog,
			Commits:  []string{"abc123abc123abc123abc123abc123abc123abcd"},
			Ref:      "HEAD",
			Pathspec: "internal/agent",
			ChangedPathSets: []types.ToolVCSChangedPathSet{{
				Commit: "abc123abc123abc123abc123abc123abc123abcd",
				Paths: []string{
					"internal/agent/agent.go",
					"internal/agent/agent_test.go",
					"internal/tool/test_surface_test.go",
				},
				Total:    3,
				Complete: true,
			}},
		},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`tool:0#vcs_metadata`",
		"origin=`vcs_metadata`",
		"producer=`git_log`",
		"role=`supporting_coverage`",
		"policy=`soft`",
		"range=ref=HEAD count=1",
		"pathspec=internal/agent",
		"vcs_history kind=git_log ref=HEAD commits=1 pathspec=internal/agent",
		"Typed VCS Changed-Path Authority",
		"emitted=3; total=3; complete=true",
		"`internal/agent/agent.go`",
		"`internal/agent/agent_test.go`",
		"`internal/tool/test_surface_test.go`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed vcs history prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "rendered text is not the ledger source") ||
		strings.Contains(got, "legacy_summary_fallback") {
		t.Fatalf("typed vcs history prompt should not consume rendered summary:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_BindsPrincipalToTypedLatestMerge(t *testing.T) {
	mut := types.NewMutableState("最近一次合入是什么")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "git_log",
		Success:  true,
		VCSHistory: &types.ToolVCSHistory{
			Kind:        types.ToolVCSHistoryKindGitLog,
			Commits:     []string{"newest-merge", "older-feature"},
			Ref:         "HEAD",
			QueryOrder:  "recent",
			QueryLimit:  5,
			MergesOnly:  true,
			FirstParent: true,
		},
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsHistoryLookup: true},
			HistorySelectionProfile: &types.HistorySelectionProfile{
				Mode:     types.HistorySelectionLatestOne,
				ItemKind: types.HistorySelectionItemMerge,
				Count:    1,
			},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"Typed VCS Principal Selection",
		"selection=`latest_one`",
		"item_kind=`merge`",
		"principal_ordinal=1; commit=`newest-merge`",
		"Later rows from a wider query are context only",
		"Do not replace the selected row based on commit subject, patch size, changed-file role",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed VCS principal selection prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "principal_ordinal=2") {
		t.Fatalf("latest_one must not promote the older contextual row:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_SeparatesHistoricalTransitionAndCurrentDefinitionBindings(t *testing.T) {
	mut := types.NewMutableState("explain historical change in current source")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "git_show", Success: true,
			VCSHistory: &types.ToolVCSHistory{
				Kind:    types.ToolVCSHistoryKindGitShow,
				Commits: []string{"abc123"},
				ChangedPathSets: []types.ToolVCSChangedPathSet{{
					Commit: "abc123", Paths: []string{"internal/agent/agent_test.go"}, Total: 1, Complete: true,
				}},
			},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "helper", Source: "internal/agent/agent_test.go", LineStart: 1808, LineEnd: 1820,
		Scope: types.ScopeLineRange, AnchorKind: types.AnchorDefinition,
		AnchorSymbol: "explicitRuntimeArtifactLog", Subject: "explicitRuntimeArtifactLog",
		GroundingStatus: types.GroundingGrounded,
	},
	})
	ctx := &types.AgentContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:     types.IntentExplain,
		Predicates: types.SemanticPredicates{IsHistoryLookup: true},
	}}}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"Historical Observation / Current Checkout Boundary",
		"historical_transition=`unproven`",
		"no typed revision mapping or behavioural transition witness",
		"symbol=`explicitRuntimeArtifactLog`; current_path=`internal/agent/agent_test.go`",
		"historical_path_match=true",
		"If a historical symbol has no exact current binding below",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("historical/current boundary missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_PreservesAggregateMemberNotes(t *testing.T) {
	mut := types.NewMutableState("Kind 常量说明")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind 常量",
		Value:   "4",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindSymbolPresent", "KindNoCallSites", "KindAnswerSetBounded", "KindExternalArtifactDecoded"},
		MemberNotes: []string{
			"KindSymbolPresent 用于符号存在性判定，检查目标符号是否能在当前证据中解析。",
			"KindNoCallSites 用于调用点缺失判定，表达没有发现调用关系的负向条件。",
			"KindAnswerSetBounded 用于答案集合边界检查，防止枚举类答案无限扩张。",
			"KindExternalArtifactDecoded 用于外部附件解码状态，失败时应走边界说明。",
		},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
			"KindAnswerSetBounded @ internal/analysis/criterion/grammar.go:31",
			"KindExternalArtifactDecoded @ internal/analysis/criterion/grammar.go:64",
		},
	}})
	mut.SetInvestigationComplete("done")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		// Model-authored file:line syntax remains visible, but without an
		// independently Grounded/Recovered witness it is advisory rather than a
		// current-source proof-lane record (CSP-RM A2).
		"`aggregate:0#system_inference`",
		"KindSymbolPresent 用于符号存在性判定",
		"KindNoCallSites 用于调用点缺失判定",
		"KindAnswerSetBounded 用于答案集合边界检查",
		"KindExternalArtifactDecoded 用于外部附件解码状态",
		"support_refs=4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing rich aggregate note %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `notes="KindSymbolPresent"`) {
		t.Fatalf("dry member fallback should not crowd out rich member notes in prompt budget:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_DoesNotRepeatSummaryAsNote(t *testing.T) {
	mut := types.NewMutableState("Kind 常量说明")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "KindSymbolPresent 用于符号存在性判定",
		Value: "1",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"KindSymbolPresent",
		},
		MemberNotes: []string{
			"KindSymbolPresent 用于符号存在性判定",
		},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
		},
	}})
	mut.SetInvestigationComplete("done")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	if strings.Count(got, "KindSymbolPresent 用于符号存在性判定") != 1 {
		t.Fatalf("summary should not be duplicated as a rich note:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_KeepsDiffAndCurrentSourceSeparate(t *testing.T) {
	mut := types.NewMutableState("diff plus current source")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ToolResults: []types.ToolResult{{
			ToolName: "git_diff",
			Success:  true,
			Summary:  "[git_diff: diff_origin=vcs_diff]\ndiff shows scheduler hook was added",
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "current",
			Source:          "internal/scheduler.go",
			LineStart:       42,
			Summary:         "current source still routes scheduler hooks through Run",
			Salience:        types.SalienceLoadBearing,
			AnchorSymbol:    "Run",
			AnchorKind:      types.AnchorDefinition,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
		}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`evidence:current`: origin=`current_source`",
		"internal/scheduler.go",
		"span=line 42",
		"`tool:0#vcs_diff`: origin=`vcs_diff`",
		"diff shows scheduler hook was added",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mixed source/diff ledger prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "`evidence:current`") > strings.Index(got, "`tool:0#vcs_diff`") {
		t.Fatalf("mixed diff+current prompt should keep exact current source before broad diff support:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_RendersMCPResourceWithoutRepoCitationPressure(t *testing.T) {
	ctx := &types.AgentContext{
		MCPResponses: []types.MCPResponse{{
			ServerName: "issues",
			Method:     "read_resource",
			Success:    true,
			Summary:    "release note says the regression was fixed in build 17",
			RawRef:     "mcp://issues/17",
		}},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`mcp:0`",
		"origin=`mcp_resource`",
		"source=`kind=mcp_resource | raw_ref=mcp://issues/17 | server=issues`",
		"release note says the regression",
		"non-`current_source` observations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("MCP observation ledger prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_NamesAllExternalObservationFamilies(t *testing.T) {
	mut := types.NewMutableState("external observations")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "cross repo hit",
			Value: "tools/worker",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginCrossRepoIndex)},
				{Name: "repo", Value: "tools"},
				{Name: "path", Value: "tools/worker.py"},
			},
		},
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "web contract",
			Value: "documented",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginWebPage)},
				{Name: "url", Value: "https://example.test/spec"},
				{Name: "selector", Value: "#contract"},
			},
		},
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "external design paragraph",
			Value: "requires gateway fallback",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginExternalDocument)},
				{Name: "resource_uri", Value: "drive://doc/123"},
				{Name: "page_ref", Value: "page=2"},
				{Name: "paragraph", Value: "7"},
			},
		},
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "MCP schema row",
			Value: "field present",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginMCPResource)},
				{Name: "server", Value: "docs"},
				{Name: "resource_uri", Value: "mcp://docs/spec"},
				{Name: "json_pointer", Value: "/items/0/title"},
			},
		},
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "connector issue",
			Value: "JIRA-7",
			Role:  types.AnswerAggregateRoleSupportingCoverage,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginConnectorResource)},
				{Name: "connector", Value: "jira"},
				{Name: "resource_uri", Value: "jira://JIRA-7"},
			},
		},
	})
	mut.SetInvestigationComplete("done")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"cross-repo index rows",
		"external documents",
		"web pages",
		"MCP resources",
		"connector resources",
		"`aggregate:0#cross_repo_index`",
		"`aggregate:1#web_page`",
		"`aggregate:2#external_document`",
		"`aggregate:3#mcp_resource`",
		"`aggregate:4#connector_resource`",
		"resource_uri=drive://doc/123",
		"json \"/items/0/title\"",
		"non-`current_source` observations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_RendersTypedPayloadRefs(t *testing.T) {
	mut := types.NewMutableState("基于命令输出分析")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "最近 10 次提交的结构化摘要",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginCommandMeasurement)},
			{Name: "command", Value: "git log --oneline -n 10"},
			{Name: "payload_ref", Value: "blob://payload/git-log-full.txt"},
			{Name: "row_set_ref", Value: "blob://rows/git-log-rows.jsonl"},
			{Name: "page_ref", Value: "blob://payload/git-log-full.txt?page=1"},
		},
	}})
	mut.SetInvestigationComplete("done")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`aggregate:0#command_measurement`",
		"source=`kind=command | command=git log --oneline -n 10 | payload_ref=blob://payload/git-log-full.txt | row_set_ref=blob://rows/git-log-rows.jsonl | page_ref=blob://payload/git-log-full.txt?page=1`",
		"最近 10 次提交的结构化摘要",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing typed payload ref %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_RendersExternalRawExcerpt(t *testing.T) {
	mut := types.NewMutableState("基于 git diff 和当前代码分析")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "exec_command",
		Success:  true,
		Summary: "[git_show: evidence_origin=vcs_diff]\n" +
			"commit abc123\n" +
			"详细说明：新增调度入口并调整兼容保护。\n" +
			"影响：当前代码需要结合 scheduler.go 的新入口理解。",
		RawRef: "blob://payload/git-show-abc123.txt",
	}}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true},
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`tool:0#vcs_diff`",
		"source=`kind=vcs_diff | tool_call_id=exec_command[0] | payload_ref=blob://payload/git-show-abc123.txt`",
		"summary=\"commit abc123\"",
		"excerpt=\"[git_show: evidence_origin=vcs_diff] commit abc123 详细说明：新增调度入口并调整兼容保护。 影响：当前代码需要结合 scheduler.go 的新入口理解。\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing external raw excerpt %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_RendersRuntimeProvenanceLane(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "TypeError",
			Message: "Cannot read property name of undefined",
			Frames: []types.LogFrame{{
				Raw: "at UserCard.build (entry/src/main/ets/UserCard.ets:42)",
			}},
		}},
	}
	mut := types.NewMutableState("分析日志崩溃原因")
	mut.SetLogTriage(logBundle)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
			},
		},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`log:error:0`",
		"producer=`log_triage`",
		"lane=`observed_error_occurrence`",
		"Stack frames are artifact-local runtime support",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing runtime provenance lane %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocObservationLedger_PeerErrorsDoNotContradictRelationFence(t *testing.T) {
	logBundle := &types.LogBundle{Errors: []types.LogError{
		{Type: "panic", Message: "index out of bounds"},
		{Type: "runtime_error", Message: "native call failed"},
	}}
	mut := types.NewMutableState("定位混合语言日志中的两个错误帧")
	mut.SetLogTriage(logBundle)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			LogTriage: logBundle,
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{
		"`log:error:0`",
		"`log:error:1`",
		"lane=`observed_error_occurrence`",
		"`log:cross_error_relation`",
		`value="unproven"`,
		"observed_scope=peer_error_occurrences_only",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("peer-error observation handoff missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "lane=`observed_direct_cause`") {
		t.Fatalf("peer errors must not regain direct-cause authority in finalizer handoff:\n%s", got)
	}
}

func TestRenderAnswerDocObservationLedger_CreatesLargeAggregateRowSetArtifact(t *testing.T) {
	members := make([]string, 0, 24)
	notes := make([]string, 0, 24)
	refs := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		member := fmt.Sprintf("Kind%02d", i)
		members = append(members, member)
		notes = append(notes, fmt.Sprintf("%s 用于保持第 %d 类条件的中文说明", member, i+1))
		refs = append(refs, fmt.Sprintf("%s: internal/types/kind.go:%d", member, i+1))
	}
	mut := types.NewMutableState("列出 Kind 成员")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "Kind members",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     members,
		MemberNotes: notes,
		SupportRefs: refs,
	}})
	mut.RetainInvestigationAggregateFacts()
	workDir := t.TempDir()
	ctx := &types.AgentContext{
		WorkDir: workDir,
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	got := renderAnswerDocObservationLedger(ctx)
	if !strings.Contains(got, "row_set_ref=") {
		t.Fatalf("large aggregate ledger should expose row_set_ref:\n%s", got)
	}
	matches, err := filepath.Glob(filepath.Join(workDir, "aggregate-000-system_inference-row-set-*.jsonl"))
	if err != nil {
		t.Fatalf("glob row-set artifact: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one row-set artifact, got %v", matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read row-set artifact: %v", err)
	}
	if !strings.Contains(string(body), `"member":"Kind00"`) ||
		!strings.Contains(string(body), `"note":"Kind00 用于保持第 1 类条件的中文说明"`) {
		t.Fatalf("row-set artifact missing member details:\n%s", string(body))
	}
}
