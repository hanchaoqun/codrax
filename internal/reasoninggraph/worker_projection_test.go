package reasoninggraph

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worker"
)

func TestEventsFromWorkerRequestResultProjectsTypedEvidenceProducer(t *testing.T) {
	req := worker.NormalizeRequest(worker.Request{
		ID:     "impact-1",
		RunID:  "wf-1",
		UnitID: "batch-1",
		Kind:   worker.KindImpactAnalyzer,
		Role:   safety.EffectRoleImpactAnalyzer,
		Goal:   "inspect impact",
		Scope: worker.Scope{
			Paths:        []string{"pkg/a.py", "pkg/b.py"},
			EvidenceRefs: []string{"owner:pkg/a.py"},
		},
		InputRefs:   []loopkernel.ArtifactRef{{Kind: "change_plan", ID: "plan-1"}},
		RequestedAt: time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC),
	})
	result := worker.NormalizeResult(worker.Result{
		RequestID:    "impact-1",
		RunID:        "wf-1",
		UnitID:       "batch-1",
		Kind:         worker.KindImpactAnalyzer,
		Role:         safety.EffectRoleImpactAnalyzer,
		Status:       worker.StatusComplete,
		ReasonCode:   "impact_analysis_targets_projected",
		OutputRefs:   []loopkernel.ArtifactRef{{Kind: "impact_analysis_result", ID: "impact-1"}},
		EvidenceRefs: []string{"impact:pkg/b.py"},
		CompletedAt:  time.Date(2026, 6, 20, 1, 1, 0, 0, time.UTC),
	})

	events := EventsFromWorkerRequestResult(req, &result)
	if len(events) != 2 {
		t.Fatalf("events=%+v", events)
	}
	view := ReduceEvents(events)
	if view.GraphID != "wf-1" || len(view.WorkerEvents) != 2 || laneCount(AuditSummaryFromEvents(events, "worker_test", 5).Lanes, "worker") != 2 {
		t.Fatalf("worker view = %+v", view)
	}
	requested := view.WorkerEvents[0]
	if requested.Kind != ReasoningEventWorkerRequested ||
		requested.WorkerKind != string(worker.KindImpactAnalyzer) ||
		requested.PermissionAction != string(safety.PermissionAllow) ||
		requested.ScopePathCount != 2 ||
		requested.InputRefCount != 1 {
		t.Fatalf("request event = %+v", requested)
	}
	completed := view.WorkerEvents[1]
	if completed.Kind != ReasoningEventWorkerCompleted ||
		completed.WorkerStatus != string(worker.StatusComplete) ||
		completed.OutputRefCount != 1 ||
		completed.EvidenceRefCount != 1 {
		t.Fatalf("completed event = %+v", completed)
	}
	if len(view.EvidenceRefs) != 2 ||
		view.EvidenceRefs[0].ID != "owner:pkg/a.py" ||
		view.EvidenceRefs[1].ID != "impact:pkg/b.py" {
		t.Fatalf("evidence refs = %+v", view.EvidenceRefs)
	}
}

func TestEventsFromSubAgentRequestResultProjectsIsolatedEvidence(t *testing.T) {
	req := &types.SubAgentRequest{
		ID:        "trace-sub-1",
		SubAgent:  "SubExplorer",
		Objective: "inspect scoped files",
		Scope:     []string{"internal/agent", "internal/types"},
		ReadView:  &types.BusContext{TraceID: "trace-1"},
	}
	result := &types.SubAgentResult{
		RequestID: "trace-sub-1",
		SubAgent:  "SubExplorer",
		Facts: []types.RepoFact{{
			Key:         "subagent.fact",
			Value:       "found",
			EvidenceRef: "fact:subagent",
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID:          "ev-sub-1",
			EvidenceRef: "line:subagent",
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID:          "flow-1",
			EvidenceIDs: []string{"flow:subagent"},
		}},
		Tools: []types.ToolResult{{
			ToolName: "read_file",
			RawRef:   "blob:tool-read",
			Success:  true,
		}},
		MCPResps: []types.MCPResponse{{
			ServerName: "test",
			Method:     "lookup",
			PayloadRef: "payload:mcp",
			Success:    true,
		}},
	}

	events := EventsFromSubAgentRequestResult(req, result)
	view := ReduceEvents(events)
	if view.GraphID != "trace-1" || len(view.SubAgentEvents) != 2 {
		t.Fatalf("subagent view = %+v", view)
	}
	if view.SubAgentEvents[0].Kind != ReasoningEventSubAgentRequested ||
		view.SubAgentEvents[0].SubAgentName != "SubExplorer" ||
		view.SubAgentEvents[0].ScopePathCount != 2 {
		t.Fatalf("request summary = %+v", view.SubAgentEvents[0])
	}
	completed := view.SubAgentEvents[1]
	if completed.Kind != ReasoningEventSubAgentCompleted ||
		completed.ReasonCode != "subagent_completed" ||
		completed.EvidenceRefCount != 3 ||
		completed.ToolResultCount != 1 ||
		completed.MCPResponseCount != 1 ||
		completed.FactCount != 1 ||
		completed.FlowFindingCount != 1 {
		t.Fatalf("completed summary = %+v", completed)
	}
	audit := AuditSummaryFromEvents(events, "subagent_test", 5)
	if audit == nil || laneCount(audit.Lanes, "subagent") != 2 || len(audit.SubAgentEvents) != 2 {
		t.Fatalf("audit = %+v", audit)
	}
	if len(view.EvidenceRefs) != 3 {
		t.Fatalf("evidence refs = %+v", view.EvidenceRefs)
	}
	if len(events[1].ArtifactRefs) != 2 {
		t.Fatalf("artifact refs = %+v", events[1].ArtifactRefs)
	}
}

func TestEventsFromArtifactWorkersProjectEveryEvidenceWorkerKind(t *testing.T) {
	localizer := worker.RunLocalizer(worker.LocalizerInput{
		ID:             "localizer-1",
		RunID:          "wf-workers",
		CandidatePaths: []string{"pkg/a.py"},
		Anchors: []types.SourceLocalizationAnchor{{
			Path: "pkg/a.py",
		}},
	})
	impact := worker.RunImpactAnalyzer(worker.ImpactAnalyzerInput{
		ID:    "impact-1",
		RunID: "wf-workers",
		Analysis: &types.ImpactAnalysisResult{
			PlanID: "plan-1",
			ChangedSurfaces: []types.ImpactChangedSurface{{
				Path:        "pkg/a.py",
				EvidenceRef: "impact:pkg/a.py",
			}},
		},
	})
	critic := worker.RunPatchCritic(worker.PatchCriticInput{
		ID:    "critic-1",
		RunID: "wf-workers",
		Review: &types.PatchReviewRecord{
			ReviewID:     "review-1",
			TargetPaths:  []string{"pkg/a.py"},
			AppliedPaths: []string{"pkg/a.py"},
			Findings: []types.PatchReviewFinding{{
				Code:        "needs_probe",
				Path:        "pkg/a.py",
				EvidenceRef: "critic:pkg/a.py",
			}},
		},
	})
	proof := worker.RunProofAuditor(worker.ProofAuditorInput{
		ID:    "proof-1",
		RunID: "wf-workers",
		Plan:  &types.ChangePlan{ID: "plan-1", TargetPaths: []string{"pkg/a.py"}},
		Report: &types.ChangeReport{
			PlanID:             "plan-1",
			Passed:             true,
			VerificationStatus: types.VerificationStatusPassed,
			TestResults: []types.TestResult{{
				AssertionID: "test_pkg",
				Suite:       "pkg",
				Passed:      true,
			}},
		},
	})
	failure := worker.RunFailureAnalyzer(worker.FailureAnalyzerInput{
		ID:      "failure-1",
		RunID:   "wf-workers",
		BatchID: "batch-1",
		Report: &types.ChangeReport{
			PlanID:            "plan-1",
			Passed:            false,
			FailureKind:       types.FailureKindBuildFailure,
			FailureReasonCode: "build_failed",
			TestResults: []types.TestResult{{
				Kind:   types.TestResultKindBuildError,
				Suite:  "build",
				Passed: false,
				BuildErrors: []types.BuildError{{
					File:    "pkg/a.py",
					Line:    3,
					Message: "syntax",
				}},
			}},
		},
		DiffArtifactRef: "diff:plan-1",
	})

	cases := []struct {
		name   string
		kind   worker.Kind
		req    worker.Request
		result worker.Result
	}{
		{"localizer", worker.KindLocalizer, localizer.Request, localizer.Result},
		{"impact", worker.KindImpactAnalyzer, impact.Request, impact.Result},
		{"critic", worker.KindPatchCritic, critic.Request, critic.Result},
		{"proof", worker.KindProofAuditor, proof.Request, proof.Result},
		{"failure", worker.KindFailureAnalyzer, failure.Request, failure.Result},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events := EventsFromWorkerRequestResult(tc.req, &tc.result)
			view := ReduceEvents(events)
			if len(view.WorkerEvents) != 2 {
				t.Fatalf("worker events = %+v", view.WorkerEvents)
			}
			if view.WorkerEvents[0].WorkerKind != string(tc.kind) ||
				view.WorkerEvents[1].WorkerKind != string(tc.kind) ||
				view.WorkerEvents[1].WorkerStatus != string(worker.StatusComplete) {
				t.Fatalf("worker summaries = %+v", view.WorkerEvents)
			}
			if len(view.WorkerEvents) > 1 && view.WorkerEvents[1].PermissionAction != "" {
				t.Fatalf("result event must not imply a fresh permission decision: %+v", view.WorkerEvents[1])
			}
		})
	}
}
