package reasoninggraph

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromReadAnswerDocumentProjectsTypedEvidenceShadow(t *testing.T) {
	mu := types.NewMutableState("which agent can call subagent?")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"internal/agent/subagent_runtime.go"},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "ev-run-entry",
			Kind:            types.EvidenceMechanism,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/subagent_runtime.go",
			LineStart:       218,
			Subject:         "SubAgentRuntime.Run",
			Predicate:       "entrypoint_for",
			Object:          "Orchestrator",
			GroundingStatus: types.GroundingGrounded,
		}},
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateScalar,
			Label: "caller",
			Value: "Orchestrator",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		}},
		SourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			Source:              "turn_a",
			SourcePaths:         []string{"internal/agent/subagent_runtime.go"},
			SupportedPaths:      []string{"internal/agent/subagent_runtime.go"},
			OwnerSupportedPaths: []string{"internal/agent/subagent_runtime.go"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:         "internal/agent/subagent_runtime.go",
				Kind:         types.SourceLocalizationAnchorGroundedEvidence,
				Strength:     types.SourceLocalizationAnchorOwner,
				OwnerSymbol:  "SubAgentRuntime.Run",
				AnchorSymbol: "Run",
			}},
		},
	})
	ctx := &types.BusContext{
		TraceID: "trace-read-1",
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentTrace,
				Scenario: types.ScenarioArchitectureExplain,
			},
			TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{ID: "n1", Type: types.NodeEvidence}}},
			EvidencePlan: types.EvidencePlan{
				RequiredFiles: []string{"internal/agent/subagent_runtime.go"},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "Orchestrator calls subagents through SubAgentRuntime.Run.",
		}},
		Citations: []types.Citation{{
			File: "internal/agent/subagent_runtime.go",
			Line: 218,
		}},
		ReadSourceLocalization: &types.SourceLocalizationReview{
			Status:              types.SourceLocalizationSupported,
			SourcePaths:         []string{"internal/agent/subagent_runtime.go"},
			OwnerSupportedPaths: []string{"internal/agent/subagent_runtime.go"},
		},
		ReadNavigationCoverage: &types.RepoMapNavigationCoverage{
			State:          types.RepoMapNavigationCoveragePartial,
			ReasonCode:     "repo_map_navigation_partial",
			RequiredRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteRelationMap},
			CoveredRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap},
			MissingRoutes:  []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteRelationMap},
			EvidenceRefs:   []string{"blob://repo-map-task"},
		},
		ReadOwnerAnchors: []types.OwnerAnchorViewItem{{
			ID:           "owner-run",
			Path:         "internal/agent/subagent_runtime.go",
			Kind:         types.SourceLocalizationAnchorGroundedEvidence,
			Strength:     types.SourceLocalizationAnchorOwner,
			OwnerSymbol:  "SubAgentRuntime.Run",
			AnchorSymbol: "Run",
		}},
	}

	events := EventsFromReadAnswerDocument(ctx, doc)
	view := ReduceEvents(events)
	if view.GraphID != "trace-read-1" || view.EventCount == 0 {
		t.Fatalf("view header=%+v", view)
	}
	if len(view.ReadEvents) < 6 {
		t.Fatalf("read events=%+v, want analysis/evidence/aggregate/localization/navigation/answer", view.ReadEvents)
	}
	if !reasoningGraphViewHasKind(view, ReasoningEventReadAnswerProjected) ||
		!reasoningGraphViewHasKind(view, ReasoningEventReadNavigationProjected) {
		t.Fatalf("read event kinds missing: %+v", view.EventKindCounts)
	}
	if len(view.EvidenceRefs) == 0 {
		t.Fatalf("expected evidence refs from TurnA/citations/anchors")
	}
	summary := AnswerReasoningGraphSummaryFromReadAnswerDocument(ctx, doc)
	if summary == nil ||
		summary.GraphID != "trace-read-1" ||
		summary.ReadEventCount != len(view.ReadEvents) ||
		summary.AnswerBlockCount != 1 ||
		summary.CitationCount != 1 ||
		summary.EvidenceRefCount == 0 ||
		!strings.HasPrefix(summary.EventRefs[0], "trace-read-1:") {
		t.Fatalf("summary=%+v view=%+v", summary, view)
	}
}
