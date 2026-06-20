package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAppendSystemCaveats_DegradedTerminationVisibleBothLangs(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		mu := types.NewMutableState("degraded")
		mu.MarkTerminationFloorDegraded("ratio 10% < 50%")
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: lang}}
		got := o.appendSystemCaveatsToAnswer("answer body")
		if !strings.Contains(got, "answer body") {
			t.Fatalf("%s: answer body must be preserved", lang)
		}
		if lang == "zh" && !strings.Contains(got, "低于配置的最低标准") {
			t.Fatalf("zh: degraded caveat missing, got %q", got)
		}
		if lang == "en" && !strings.Contains(got, "below the configured floor") {
			t.Fatalf("en: degraded caveat missing, got %q", got)
		}
		// Internal diagnostic detail must NOT leak into the user answer.
		if strings.Contains(got, "ratio 10%") {
			t.Fatalf("%s: internal detail leaked, got %q", lang, got)
		}
	}
}

func TestAppendSystemCaveats_NoDegradationNoCaveat(t *testing.T) {
	mu := types.NewMutableState("clean")
	mu.SetTerminationProfile(types.TerminationProfile{Kind: types.TerminationStopCondition})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	got := o.appendSystemCaveatsToAnswer("answer body")
	if got != "answer body" {
		t.Fatalf("non-degraded termination must not add caveats, got %q", got)
	}
}

func TestAppendSystemCaveats_SuppressesDegradedTerminationForGroundedPrincipalAnswer(t *testing.T) {
	mu := types.NewMutableState("grounded")
	mu.MarkTerminationFloorDegraded("ratio 10% < 50%")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "owner",
				Label:       "SubAgentRuntime.Run",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "internal/agent/subagent_runtime.go",
			Line: 218,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"internal/agent/subagent_runtime.go"},
			SupportedPaths:      []string{"internal/agent/subagent_runtime.go"},
			OwnerSupportedPaths: []string{"internal/agent/subagent_runtime.go"},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	got := o.appendSystemCaveatsToAnswer("answer body")
	if strings.Contains(got, "低于配置的最低标准") {
		t.Fatalf("grounded principal answer should suppress generic degraded caveat, got %q", got)
	}
	if got != "answer body" {
		t.Fatalf("unexpected answer surface: %q", got)
	}
}

func TestAppendSystemCaveats_SuppressesDegradedTerminationForGroundedEnumerationSection(t *testing.T) {
	mu := types.NewMutableState("grounded enum")
	mu.MarkTerminationFloorDegraded("ratio 10% < 50%")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "entries",
			Kind:        types.BlockSection,
			Title:       "@Entry 页面入口",
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "index",
				Label:       "Index",
				CitationRef: 0,
			}, {
				ID:          "list",
				Label:       "ListPage",
				CitationRef: 1,
			}},
		}},
		Citations: []types.Citation{
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets", Line: 7},
			{File: "internal/thirdparty/tree-sitter-arkts/corpus/sources/05_foreach_lazyforeach.ets", Line: 32},
		},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	got := o.appendSystemCaveatsToAnswer("answer body")
	if strings.Contains(got, "低于配置的最低标准") {
		t.Fatalf("grounded enumeration answer should suppress generic degraded caveat, got %q", got)
	}
}

func TestAppendSystemCaveats_KeepsDegradedTerminationForObservedOnlyAnswer(t *testing.T) {
	mu := types.NewMutableState("observed")
	mu.MarkTerminationFloorDegraded("ratio 10% < 50%")
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			Items: []types.AnswerBlockItem{{
				ID:          "observed",
				Label:       "Observed",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File: "pkg/observed.py",
			Line: 12,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/observed.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/observed.py",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	got := o.appendSystemCaveatsToAnswer("answer body")
	if !strings.Contains(got, "低于配置的最低标准") {
		t.Fatalf("observed-only localization should keep degraded caveat, got %q", got)
	}
}

func TestAppendSystemCaveats_PreStageDegradationVisible(t *testing.T) {
	mu := types.NewMutableState("prestage")
	mu.AddPreStageDegradation(types.PreStageDegradation{Stage: types.StageLogTriage, Kind: types.PreStageDegradationEmitRejected, Summary: "structured rows rejected"})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "zh"}}
	got := o.appendSystemCaveatsToAnswer("body")
	if !strings.Contains(got, "未能完成结构化解析") {
		t.Fatalf("zh degradation caveat missing, got %q", got)
	}
	o2 := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	if got2 := o2.appendSystemCaveatsToAnswer("body"); !strings.Contains(got2, "could not be structurally parsed") {
		t.Fatalf("en degradation caveat missing, got %q", got2)
	}
}
