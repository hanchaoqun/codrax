package agent

import (
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

func TestRenderAnswerDocObservationLedger_RendersVCSNarrativeAsOriginSpecificSupport(t *testing.T) {
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
		"policy=`soft`",
		"abc123 Add observation ledger feature",
		"Do not turn non-`current_source` observations into source `file:line` citation requirements",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "value=\"abc123\"") {
		t.Fatalf("history narrative ledger must not collapse the answer to a scalar commit id:\n%s", got)
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
		"`aggregate:0#current_source`",
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
		"source=`kind=vcs_diff | payload_ref=blob://payload/git-show-abc123.txt`",
		"summary=\"commit abc123\"",
		"excerpt=\"[git_show: evidence_origin=vcs_diff] commit abc123 详细说明：新增调度入口并调整兼容保护。 影响：当前代码需要结合 scheduler.go 的新入口理解。\"",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("observation ledger prompt missing external raw excerpt %q:\n%s", want, got)
		}
	}
}
