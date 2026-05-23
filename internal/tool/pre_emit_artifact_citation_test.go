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
