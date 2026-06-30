package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreCheckArtifactObservedFrameCitations_RejectsObservedLineAsSourceCitation(t *testing.T) {
	ctx := artifactFrameDriftBusContext()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 250}},
		Blocks: []types.AnswerBlock{{
			ID:   "observed",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "frame",
				Label:       "buildAnalysisIR",
				Text:        "observed runtime frame",
				CitationRef: 0,
			}},
		}},
	}

	hints := preCheckArtifactObservedFrameCitations(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("expected observed-frame citation rejection, got %d: %+v", len(hints), hints)
	}
	if hints[0].Field != "citations[0]" {
		t.Fatalf("hint field = %q, want citations[0]", hints[0].Field)
	}
	if hints[0].ExpectedShape == "" || hints[0].Reason == "" {
		t.Fatalf("hint should explain the typed artifact/source boundary: %+v", hints[0])
	}
	wantShape := "runtime artifact frame coordinates should stay in observed-artifact rows without current-repo citations; cite the current grounded source anchor instead: internal/agent/analyzer.go:861"
	if hints[0].ExpectedShape != wantShape {
		t.Fatalf("hint shape = %q, want %q", hints[0].ExpectedShape, wantShape)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_ObservationOnlyDropsCitationPool(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			File: "src/main.cj",
			Line: 18,
			Func: "init",
			Raw:  "src/main.cj:18 init",
		}}}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
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
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "src/main.cj", Line: 18}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "日志显示 init 在运行时栈顶失败。",
			Items: []types.AnswerBlockItem{{
				ID:          "runtime",
				Label:       "init",
				CitationRef: 0,
			}},
		}},
	}

	fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx)
	if fixed == 0 {
		t.Fatal("expected runtime artifact citations to normalize")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("observation-only runtime answer should not keep citations: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != -1 {
		t.Fatalf("runtime observation citation_ref = %d, want -1", got)
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("normalized runtime observation should not be rejected, got %+v", hints)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_AcceptedCurrentSourceProofDisablesObservationOnlyCleanup(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			File: "src/main.cj",
			Line: 18,
			Func: "init",
			Raw:  "src/main.cj:18 init",
		}}}},
	})
	evidence := []types.EvidenceItem{{
		ID:              "current-source-init",
		Kind:            types.EvidenceDirect,
		Origin:          types.ClaimOriginCurrentRepo,
		Source:          "internal/runtime/init.go",
		LineStart:       44,
		LineEnd:         52,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "init",
		Subject:         "init",
		Summary:         "current source explains the init guard",
		GroundingStatus: types.GroundingGrounded,
	}}
	mut.AppendEvidence(evidence)
	ctx := &types.BusContext{
		Mutable:       mut,
		EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
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
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active || !authority.CurrentSourceSatisfied || authority.CanHardBlockCompletion {
		t.Fatalf("test setup should expose accepted current-source proof through authority, got %+v", authority)
	}
	if answerDocumentRuntimeObservationOnly(ctx) {
		t.Fatal("accepted current-source proof must disable observation-only cleanup")
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/runtime/init.go", Line: 44}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "当前源码中的 init guard 解释了运行时栈顶失败。",
			Items: []types.AnswerBlockItem{{
				ID:          "current",
				Label:       "init guard",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx); fixed != 0 {
		t.Fatalf("accepted current-source proof should keep current-source citations, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 1 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("current-source citation should be preserved: %+v", doc)
	}
}

func TestAnswerDocumentHasLoadBearingCurrentSourceLane_UsesAuthorityPrecision(t *testing.T) {
	softCtx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes:                        []string{"current parser mechanism"},
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
				Confidence:                          0.9,
			},
		}},
	}
	softAuthority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(softCtx, types.ObservationLedger{})
	if !softAuthority.Active ||
		softAuthority.CurrentSourceRequirement != types.RuntimeSourceRequirementSoft ||
		softAuthority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("test setup should expose soft non-load-bearing authority, got %+v", softAuthority)
	}
	if answerDocumentHasLoadBearingCurrentSourceLane(softCtx) {
		t.Fatal("soft unanchored current-source profile must not become a citation cleanup hard boundary")
	}

	preciseCtx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes:                        []string{"internal/tracequery/parse.go:42"},
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
				Confidence:                          0.9,
			},
		}},
	}
	preciseAuthority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(preciseCtx, types.ObservationLedger{})
	if !preciseAuthority.Active ||
		preciseAuthority.CurrentSourceRequirement != types.RuntimeSourceRequirementPrecise ||
		!preciseAuthority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("test setup should expose precise load-bearing authority, got %+v", preciseAuthority)
	}
	if !answerDocumentHasLoadBearingCurrentSourceLane(preciseCtx) {
		t.Fatal("precise anchored current-source profile must remain load-bearing")
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_MixedRuntimeCurrentSourceKeepsCurrentCitations(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "timeout"}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentExplain,
				Scenario:  types.ScenarioArchitectureExplain,
				LogTriage: mut.LogTriage(),
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{{
						Path:       "internal/llm/openai.go",
						Confidence: 0.9,
						Rationale:  "current-source mechanism requested by the user",
					}},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/llm/openai.go", Line: 608}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "当前源码说明 first byte watchdog 如何取消请求。",
			Items: []types.AnswerBlockItem{{
				ID:          "current",
				Label:       "watchdog",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx); fixed != 0 {
		t.Fatalf("mixed runtime+current-source answer should keep current citations, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 1 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("current-source citation was not preserved: %+v", doc)
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("mixed runtime+current-source citations should not be rejected, got %+v", hints)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_RouteBackedSoftMissingSourceDropsCurrentCitations(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "LLM timeout"}},
	})
	ctx := routeBackedMixedRootCauseBusContext(mut, nil)
	doc := routeBackedMixedRootCauseCitationDoc()

	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) == 0 {
		t.Fatal("soft route-backed mixed runtime/source without source evidence should keep cleanup active")
	}
	if fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx); fixed == 0 {
		t.Fatalf("soft route-backed mixed runtime/source citation should normalize without source evidence: %+v", doc)
	}
	if len(doc.Citations) != 0 || doc.Blocks[0].Items[0].CitationRef != -1 {
		t.Fatalf("soft missing-source citation should be detached: %+v", doc)
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("normalized soft missing-source answer should not be rejected, got %+v", hints)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_LedgerRuntimeObservationUsesAuthority(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "runtime_probe",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "runtime_probe:span:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "runtime_probe",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				ArtifactID:   "hiprofiler_data.htrace.systrace",
				ArtifactKind: "trace",
				RowSetRef:    "span:62380.029137877-62380.59491",
			},
			Subject:   "render span",
			Predicate: "runtime_observed",
			Summary:   "runtime probe observed the expensive render span",
		}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "mixed",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.8,
			},
		}},
	}
	if types.RuntimeArtifactContextActiveFromBus(ctx) {
		t.Fatal("test must not rely on attached artifact or trace_query runtime counter")
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active || authority.RuntimeObservationCount != 1 || !authority.CanDowngradeToCaveat || authority.CanHardBlockCompletion {
		t.Fatalf("test setup must use soft ledger-backed runtime authority, got %+v", authority)
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{
			File:  "hiprofiler_data.htrace.systrace",
			Line:  1087530,
			Quote: "runtime probe row",
		}},
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "runtime probe observed the target span.",
			Items: []types.AnswerBlockItem{{
				ID:          "span",
				Label:       "render span",
				CitationRef: 0,
			}},
		}},
	}

	fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx)
	if fixed == 0 {
		t.Fatalf("ledger-backed runtime observation should activate artifact citation cleanup: %+v", doc)
	}
	if len(doc.Citations) != 0 || doc.Blocks[0].Items[0].CitationRef != -1 {
		t.Fatalf("artifact citation should be detached without current-source proof: %+v", doc)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_RouteBackedCombinedProofKeepsCurrentCitations(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "LLM timeout"}},
	})
	ctx := routeBackedMixedRootCauseBusContext(mut, []types.EvidenceItem{{
		ID:        "source-timeout-retry",
		Source:    "internal/llm/openai.go",
		LineStart: 472,
		Summary:   "current source defines the first-byte timeout retry boundary",
		Origin:    types.ClaimOriginCurrentRepo,
	}})
	doc := routeBackedMixedRootCauseCitationDoc()

	if fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx); fixed != 0 {
		t.Fatalf("route-backed mixed runtime+source answer with source evidence should keep current citations, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/llm/openai.go" || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("current-source citation was not preserved: %+v", doc)
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("combined runtime+current-source citations should not be rejected, got %+v", hints)
	}
}

func routeBackedMixedRootCauseBusContext(mut *types.MutableState, evidence []types.EvidenceItem) *types.BusContext {
	return &types.BusContext{
		Mutable: mut,
		TurnRouteHint: types.TurnRouteHint{
			Route:           "repo",
			Source:          "mixed",
			NeedsRepoAccess: true,
			Confidence:      0.9,
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
					CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
					SourceQuotes:         []string{"LLM timeout"},
					Confidence:           0.9,
				},
			},
		},
		EvidenceItems: evidence,
	}
}

func routeBackedMixedRootCauseCitationDoc() *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/llm/openai.go", Line: 472}},
		Blocks: []types.AnswerBlock{{
			ID:   "source",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "retry",
				Label:       "ErrStreamFirstByteTimeout",
				Text:        "当前源码定义了 first-byte timeout 的重试边界。",
				CitationRef: 0,
			}},
		}},
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_DriftedObservedFrameRemapsMixedPool(t *testing.T) {
	ctx := artifactFrameDriftBusContext()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/agent/analyzer.go", Line: 250},
			{File: "internal/agent/analyzer.go", Line: 861},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "observed",
				Label:       "日志帧",
				Text:        "运行时日志观察到 buildAnalysisIR 旧行号。",
				CitationRef: 0,
			}, {
				ID:          "current",
				Label:       "当前源码",
				Text:        "当前源码锚点仍可引用。",
				CitationRef: 1,
			}},
		}},
	}

	fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx)
	if fixed == 0 {
		t.Fatal("expected drifted observed frame citation to normalize")
	}
	if len(doc.Citations) != 1 || doc.Citations[0].Line != 861 {
		t.Fatalf("citation pool = %+v, want only current anchored source line", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != -1 {
		t.Fatalf("observed frame citation_ref = %d, want -1", got)
	}
	if got := doc.Blocks[0].Items[1].CitationRef; got != 0 {
		t.Fatalf("current source citation_ref = %d, want remapped 0", got)
	}
	if hints := preCheckArtifactObservedFrameCitations(doc, ctx); len(hints) != 0 {
		t.Fatalf("normalized artifact citation should not be rejected, got %+v", hints)
	}
}

func TestNormalizeRuntimeArtifactCitationRefs_TraceArtifactPathDropsCitationPool(t *testing.T) {
	ctx := runtimeSourceOptionalTraceBusContextForTest()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{
			File:  "hiprofiler_data_20260624_17.04.05.htrace.systrace",
			Line:  1087530,
			Quote: `wp.wattpad-47216 perf_sample symbol="android::PaintGlue::getRunCharacterAdvance`,
		}},
		Blocks: []types.AnswerBlock{{
			ID:   "hops",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "perf",
				Label:       "perf_sample",
				Text:        "trace_query 观测到文字渲染宽度测量消耗 CPU。",
				CitationRef: 0,
			}},
		}},
	}

	fixed := normalizeRuntimeArtifactCitationRefs(doc, ctx)
	if fixed == 0 {
		t.Fatal("expected trace artifact citation to normalize")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("trace artifact citation must not remain in current-source citation pool: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != -1 {
		t.Fatalf("trace artifact citation_ref = %d, want -1", got)
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("normalized trace artifact answer should not be rejected, got %+v", hints)
	}
}

func TestNormalizeRuntimeArtifactVisibleCitationSentinels_SanitizesOnlyTypedObservationOnly(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{Func: "init"}}}},
	})
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: mut.LogTriage(),
			DiagnosticProfile: types.DiagnosticIntentProfile{
				IsDiagnostic: true,
			},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"只分析日志"},
				Confidence:        0.9,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "caveat",
		Kind:  types.BlockCaveat,
		Title: "运行时观察（citation_ref=-1）",
		Text:  "因此 citation_ref 标记为 -1，不绑定当前源码。",
		Items: []types.AnswerBlockItem{{
			ID:    "row",
			Label: "`citation_ref=-1`",
			Text:  "运行时 panic 日志的观察结果（citation_ref=-1）",
			Cells: []string{"事实", "citation_ref = -1"},
		}},
	}}, Caveats: []string{"这些日志事实来自外部工件，citation_ref=-1。"}}

	fixed := normalizeRuntimeArtifactVisibleCitationSentinels(doc, ctx)
	if fixed == 0 {
		t.Fatal("expected visible sentinel text to be sanitized")
	}
	rendered := doc.Blocks[0].Title + "\n" + doc.Blocks[0].Text + "\n" +
		doc.Blocks[0].Items[0].Label + "\n" + doc.Blocks[0].Items[0].Text + "\n" +
		doc.Blocks[0].Items[0].Cells[1] + "\n" + doc.Caveats[0]
	if strings.Contains(rendered, "citation_ref") {
		t.Fatalf("runtime artifact visible text should not expose citation_ref carrier:\n%s", rendered)
	}
	if !strings.Contains(rendered, "来自附件运行时材料，未绑定当前仓库源码引用") {
		t.Fatalf("sanitized text should explain artifact provenance, got:\n%s", rendered)
	}
}

func TestNormalizeRuntimeArtifactVisibleCitationSentinels_DoesNotTouchCurrentSourceAnswers(t *testing.T) {
	ctx := &types.BusContext{
		Language: "zh",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "解释 Codrax 的 citation_ref=-1 语义",
			Intent:     types.IntentExplain,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s",
		Kind: types.BlockSummary,
		Text: "Codrax 的 `citation_ref=-1` 表示没有当前仓库引用。",
	}}}
	if fixed := normalizeRuntimeArtifactVisibleCitationSentinels(doc, ctx); fixed != 0 {
		t.Fatalf("non-runtime answers must not be sanitized, fixed=%d", fixed)
	}
	if !strings.Contains(doc.Blocks[0].Text, "citation_ref=-1") {
		t.Fatalf("current-source/Codrax-internal text should remain literal: %q", doc.Blocks[0].Text)
	}
}

func TestNormalizeRuntimeObservationOnlyDecisionBlocks_DropsSourceOptionalTraceStatusDecision(t *testing.T) {
	ctx := runtimeSourceOptionalTraceBusContextForTest()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "/tmp/.codrax/blob/session/attached_trace.txt", Line: 12}},
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: "trace_query 观测到 CookieMonsterCl-59843 runnable 等待导致主线程 sleep。",
			},
			{
				ID:                   "decision",
				Kind:                 types.BlockDecision,
				CurrentStatusVerdict: types.CurrentStatusNotEnoughEvidence,
				Text:                 "not_enough_evidence: 未读取源码，无法验证当前代码是否已修复。",
			},
		},
	}

	fixed := normalizeRuntimeObservationOnlyDecisionBlocks(doc, &types.AnswerSemanticView{}, ctx)
	if fixed != 1 {
		t.Fatalf("fixed=%d, want decision block removal; doc=%+v", fixed, doc)
	}
	visible := answerDocumentTestVisibleSurface(doc)
	if strings.Contains(visible, "not_enough_evidence") || strings.Contains(visible, "当前代码是否已修复") {
		t.Fatalf("runtime trace answer should not keep source-status decision prose:\n%s", visible)
	}
	if !strings.Contains(visible, "CookieMonsterCl-59843") {
		t.Fatalf("summary payload should be preserved:\n%s", visible)
	}
}

func TestNormalizeRuntimeObservationOnlyDecisionBlocks_KeepsOnlyVisibleDecision(t *testing.T) {
	ctx := runtimeSourceOptionalTraceBusContextForTest()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:                   "decision",
		Kind:                 types.BlockDecision,
		CurrentStatusVerdict: types.CurrentStatusNotEnoughEvidence,
		Text:                 "trace-only answer has no other visible payload yet",
	}}}

	if fixed := normalizeRuntimeObservationOnlyDecisionBlocks(doc, &types.AnswerSemanticView{}, ctx); fixed != 0 {
		t.Fatalf("only visible block must be preserved, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Kind != types.BlockDecision {
		t.Fatalf("decision-only payload should stay intact: %+v", doc.Blocks)
	}
}

func TestNormalizeRuntimeObservationOnlyDecisionBlocks_KeepsCurrentSourceCitedDecision(t *testing.T) {
	ctx := runtimeSourceOptionalTraceBusContextForTest()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/scheduler.go", Line: 42}},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "trace fact plus current source check."},
			{ID: "decision", Kind: types.BlockDecision, Text: "current source backed decision", Items: []types.AnswerBlockItem{{ID: "d", CitationRef: 0}}},
		},
	}

	if fixed := normalizeRuntimeObservationOnlyDecisionBlocks(doc, &types.AnswerSemanticView{}, ctx); fixed != 0 {
		t.Fatalf("current-source cited decisions must not be removed, fixed=%d doc=%+v", fixed, doc)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("current-source cited decision should remain: %+v", doc.Blocks)
	}
}

func runtimeSourceOptionalTraceBusContextForTest() *types.BusContext {
	mut := types.NewMutableState("trace window")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:root:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace.txt", ArtifactKind: "trace"},
			Subject:         "CookieMonsterCl-59843",
			Predicate:       "root_cause_primary",
			Summary:         "CookieMonsterCl-59843 runnable wait delayed the target thread",
		}},
	})
	return &types.BusContext{
		Mutable:         mut,
		AttachedHitrace: "com.baidu.tieba-59566 (59566) [000] .... 34579.472865: sched_switch: prev_state=S",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				ExactTargets: []string{"com.baidu.tieba-59566", "34579.472865s"},
			},
		}},
	}
}

func TestNormalizeExternalObservationVisibleCitationSentinels_SanitizesVCSOnlyAnswers(t *testing.T) {
	mut := types.NewMutableState("history")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "recent history no-hit",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
			{Name: "target", Value: "missing-marker"},
			{Name: "commit_range", Value: "HEAD~20..HEAD"},
			{Name: "result_count", Value: "0"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "summary",
		Kind:  types.BlockSummary,
		Title: "版本历史观察（citation_ref=-1）",
		Text:  "该结论来自 git history，citation_ref=-1。",
		Items: []types.AnswerBlockItem{{
			ID:    "row",
			Label: "`citation_ref=-1`",
			Text:  "最近 20 次提交内 0 命中（citation_ref=-1）。",
			Cells: []string{"范围", "citation_ref = -1"},
		}},
	}}, Caveats: []string{"版本历史观察 citation_ref=-1。"}}

	fixed := normalizeExternalObservationVisibleCitationSentinels(doc, ctx)
	if fixed == 0 {
		t.Fatal("expected external-observation visible sentinel text to be sanitized")
	}
	rendered := doc.Blocks[0].Title + "\n" + doc.Blocks[0].Text + "\n" +
		doc.Blocks[0].Items[0].Label + "\n" + doc.Blocks[0].Items[0].Text + "\n" +
		doc.Blocks[0].Items[0].Cells[1] + "\n" + doc.Caveats[0]
	if strings.Contains(rendered, "citation_ref") {
		t.Fatalf("external observation visible text should not expose citation_ref carrier:\n%s", rendered)
	}
	if !strings.Contains(rendered, "来自外部观察结果，未绑定当前仓库源码引用") {
		t.Fatalf("sanitized text should explain external provenance, got:\n%s", rendered)
	}
}

func TestNormalizeExternalObservationVisibleCitationSentinels_DoesNotTouchMixedCurrentSource(t *testing.T) {
	mut := types.NewMutableState("history plus current source")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "commit",
		Value: "abc123",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup:  true,
				IsCrossComponent: true,
			},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "current-source",
			Kind:            types.EvidenceDirect,
			Source:          "internal/tool/emit_answer_document_v2.go",
			LineStart:       1,
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s",
		Kind: types.BlockSummary,
		Text: "这里讨论 Codrax 内部的 `citation_ref=-1` 兼容语义。",
	}}}

	if fixed := normalizeExternalObservationVisibleCitationSentinels(doc, ctx); fixed != 0 {
		t.Fatalf("mixed current-source answers must not be sanitized, fixed=%d", fixed)
	}
	if !strings.Contains(doc.Blocks[0].Text, "citation_ref=-1") {
		t.Fatalf("mixed current-source literal text should remain visible: %q", doc.Blocks[0].Text)
	}
}

func TestNormalizeExternalObservationVisibleCitationSentinels_PreciseCurrentSourceAuthorityDoesNotTreatAsExternalOnly(t *testing.T) {
	mut := types.NewMutableState("history plus precise current-source obligation")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "commit",
		Value: "abc123",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Language: "zh",
		Mutable:  mut,
		TurnRouteHint: types.TurnRouteHint{
			Source:          "mixed",
			NeedsRepoAccess: true,
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				SourceQuotes:                        []string{"internal/tracequery/parse.go:42"},
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			},
		}},
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active ||
		authority.CurrentSourceRequirement != types.RuntimeSourceRequirementPrecise ||
		!authority.CanHardBlockCompletion {
		t.Fatalf("test setup should expose precise current-source authority, got %+v", authority)
	}
	if answerDocumentExternalObservationOnly(ctx) {
		t.Fatal("precise current-source authority must disable external-observation-only cleanup")
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s",
		Kind: types.BlockSummary,
		Text: "这里讨论当前解析实现与历史变化，`citation_ref=-1` 是用户可见文本的一部分。",
	}}}

	if fixed := normalizeExternalObservationVisibleCitationSentinels(doc, ctx); fixed != 0 {
		t.Fatalf("precise current-source authority should not sanitize as external-only, fixed=%d", fixed)
	}
	if !strings.Contains(doc.Blocks[0].Text, "citation_ref=-1") {
		t.Fatalf("current-source-lane visible text should remain unchanged: %q", doc.Blocks[0].Text)
	}
}

func TestNormalizeExternalObservationVisibleCitationSentinels_SanitizesCommandOnlyAnswers(t *testing.T) {
	mut := types.NewMutableState("count lines")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "line count",
		Value: "42",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "command_measurement"},
			{Name: "tool_result", Value: "exec-command-001"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Language: "en",
		Mutable:  mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsCountQuestion: true,
				IsScalarAnswer:  true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "scalar",
		Kind: types.BlockScalar,
		Text: "42 (citation_ref=-1)",
	}}}

	if fixed := normalizeExternalObservationVisibleCitationSentinels(doc, ctx); fixed == 0 {
		t.Fatal("expected command-only external observation sentinel to be sanitized")
	}
	if strings.Contains(doc.Blocks[0].Text, "citation_ref") ||
		!strings.Contains(doc.Blocks[0].Text, "external observation, not a current-repository source citation") {
		t.Fatalf("command-only text was not sanitized correctly: %q", doc.Blocks[0].Text)
	}
}

func TestNormalizeExternalObservationPseudoCitations_DropsLineZeroVCSCitation(t *testing.T) {
	mut := types.NewMutableState("history")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "changed path",
		Value: "internal/tool/builtin.go",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_diff"},
			{Name: "diff_path", Value: "internal/tool/builtin.go"},
			{Name: "payload_ref", Value: "blob://payload/git-show.txt"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/tool/builtin.go", Line: 0}},
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				ID:          "path",
				Label:       "internal/tool/builtin.go",
				Text:        "changed by the inspected diff",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeExternalObservationPseudoCitations(doc, ctx); fixed == 0 {
		t.Fatal("expected line-zero external citation carrier to be detached")
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("line-zero pseudo citation should be removed, got %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got >= 0 {
		t.Fatalf("item citation_ref should be detached, got %d", got)
	}
}

func TestNormalizeExternalObservationPseudoCitations_KeepsScopeFileCitation(t *testing.T) {
	mut := types.NewMutableState("history plus file layer")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "commit",
		Value: "abc123",
		Role:  types.AnswerAggregateRoleSupportingCoverage,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{
			File:          "internal/tool/builtin.go",
			Scope:         types.ScopeFile,
			FileRoleLabel: "tool implementation",
		}},
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				ID:          "file",
				Label:       "builtin.go",
				Text:        "file-level implementation context",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeExternalObservationPseudoCitations(doc, ctx); fixed != 0 {
		t.Fatalf("explicit ScopeFile citation must remain intact, fixed=%d", fixed)
	}
	if len(doc.Citations) != 1 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("ScopeFile citation should be preserved: doc=%+v", doc)
	}
}

func TestNormalizeExternalObservationPseudoCitations_KeepsCurrentSourceLineAndDropsPseudoExternal(t *testing.T) {
	mut := types.NewMutableState("diff plus current source")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "commit",
		Value: "abc123",
		Role:  types.AnswerAggregateRoleSupportingCoverage,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_diff"},
			{Name: "payload_ref", Value: "blob://payload/git-show.txt"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
			ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/agent/explorer.go", Line: 42},
			{File: "internal/tool/builtin.go", Line: 0},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{
				{ID: "current", Label: "current source", Text: "grounded present implementation", CitationRef: 0},
				{ID: "diff", Label: "changed path", Text: "diff-only changed path", CitationRef: 1},
			},
		}},
	}

	if fixed := normalizeExternalObservationPseudoCitations(doc, ctx); fixed == 0 {
		t.Fatal("expected only the line-zero external citation carrier to be detached")
	}
	if len(doc.Citations) != 1 || doc.Citations[0].File != "internal/agent/explorer.go" || doc.Citations[0].Line != 42 {
		t.Fatalf("current-source file:line citation should remain: %+v", doc.Citations)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 0 {
		t.Fatalf("current-source citation_ref should remain 0, got %d", got)
	}
	if got := doc.Blocks[0].Items[1].CitationRef; got >= 0 {
		t.Fatalf("external pseudo citation_ref should be detached, got %d", got)
	}
}

func TestPreCheckArtifactObservedFrameCitations_AllowsCurrentAnchoredLine(t *testing.T) {
	ctx := artifactFrameDriftBusContext()
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 861}},
		Blocks: []types.AnswerBlock{{
			ID:   "current",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "frame",
				Label:       "buildAnalysisIR",
				Text:        "current grounded source anchor",
				CitationRef: 0,
			}},
		}},
	}

	if hints := preCheckArtifactObservedFrameCitations(doc, ctx); len(hints) != 0 {
		t.Fatalf("current anchored source citation should pass, got %+v", hints)
	}
}

func TestPreCheckArtifactObservedFrameCitations_NoAnchorSeedDoesNotRejectCurrentCitation(t *testing.T) {
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "go",
				File:       "internal/agent/analyzer.go",
				Line:       250,
				Func:       "buildAnalysisIR",
				Raw:        "buildAnalysisIR at line 250",
				Confidence: 0.95,
			}},
		}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		IntentHint:    types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	ctx := &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
			},
		},
		Mutable: mut,
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 250}},
	}

	if hints := preCheckArtifactObservedFrameCitations(doc, ctx); len(hints) != 0 {
		t.Fatalf("without a typed current-anchor mapping, observed/current line equality should not be rejected here: %+v", hints)
	}
}

func artifactFrameDriftBusContext() *types.BusContext {
	logBundle := &types.LogBundle{
		Meta: types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Lang:       "go",
				File:       "internal/agent/analyzer.go",
				Line:       250,
				Func:       "buildAnalysisIR",
				Raw:        "buildAnalysisIR at old build line 250",
				Confidence: 0.95,
			}},
		}},
		ResolvedFiles: []string{"internal/agent/analyzer.go"},
		IntentHint:    types.IntentRootCause,
	}
	mut := types.NewMutableState("")
	mut.SetLogTriage(logBundle)
	evidence := []types.EvidenceItem{{
		ID:              "current-buildAnalysisIR",
		Kind:            types.EvidenceDirect,
		Origin:          types.ClaimOriginCrossSource,
		Source:          "internal/agent/analyzer.go",
		LineStart:       861,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "buildAnalysisIR",
		Subject:         "buildAnalysisIR",
		GroundingStatus: types.GroundingGrounded,
		DriftReason:     types.DriftReasonLineDrift,
	}}
	mut.AppendEvidence(evidence)
	return &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			Version: types.AnalysisIRVersion,
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: logBundle,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
			},
		},
		Mutable:       mut,
		EvidenceItems: evidence,
	}
}
