package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestApplyRuntimeArtifactSourceOptionalIR_ElidesSatisfiedTraceDiscoveryProbe(t *testing.T) {
	out := runtimeArtifactIRFixture()
	rm := runtimeArtifactIRBoundRequest()

	applyRuntimeArtifactSourceOptionalIR(&out, rm, true)

	if runtimeArtifactIRNode(&out.TaskGraph, "primary_probe") != nil {
		t.Fatal("typed target + explicit window should elide the required discovery probe")
	}
	optional := runtimeArtifactIRNode(&out.TaskGraph, "optional_probe")
	if optional == nil || optional.Objective != "optional refinement" || !optional.Optional {
		t.Fatalf("optional probe must retain its independent contract: %+v", optional)
	}
	for _, edge := range out.TaskGraph.Edges {
		if edge.From == "primary_probe" || edge.To == "primary_probe" {
			t.Fatalf("removed probe left a dangling edge: %+v", edge)
		}
	}
	for _, nodeID := range out.TaskGraph.ExecutionPolicy.CriticalPath {
		if nodeID == "primary_probe" {
			t.Fatalf("removed probe remained on critical path: %v", out.TaskGraph.ExecutionPolicy.CriticalPath)
		}
	}

	first := runtimeArtifactIRNode(&out.TaskGraph, "evidence_t0")
	second := runtimeArtifactIRNode(&out.TaskGraph, "evidence_t1")
	if first == nil || second == nil {
		t.Fatalf("evidence nodes must remain: %+v", out.TaskGraph.Nodes)
	}
	for _, node := range []*types.TaskNode{first, second} {
		if !strings.Contains(node.Objective, "Collect deterministic trace evidence through trace_query") {
			t.Fatalf("evidence node lost shared trace contract: %+v", node)
		}
		if strings.Join(node.Inputs, ",") != "user_question,runtime_artifact_context,runtime_targets,requested_artifact_scope" {
			t.Fatalf("probe-elided evidence must read direct typed inputs: %+v", node.Inputs)
		}
	}
	if !strings.HasPrefix(first.Objective, "scheduler-state unit") ||
		!strings.HasPrefix(second.Objective, "wakeup-chain unit") {
		t.Fatalf("sub-topic objectives were flattened: first=%q second=%q", first.Objective, second.Objective)
	}
	if len(first.SearchHints.KeywordIDs)+len(first.SearchHints.EntityIDs) != 0 ||
		len(second.SearchHints.KeywordIDs)+len(second.SearchHints.EntityIDs) != 0 {
		t.Fatalf("runtime-only evidence must not retain source-search hints: first=%+v second=%+v", first.SearchHints, second.SearchHints)
	}
}

func TestApplyRuntimeArtifactSourceOptionalIR_KeepsProbeWhenTypedDiscoveryIncomplete(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*types.RequestModel)
		traceRuntime bool
	}{
		{name: "not a trace", traceRuntime: false},
		{name: "no named target profile", traceRuntime: true, mutate: func(rm *types.RequestModel) {
			rm.RuntimeTargetProfile.Declaration = types.RuntimeTargetDeclarationNoNamedTarget
		}},
		{name: "inactive target", traceRuntime: true, mutate: func(rm *types.RequestModel) {
			rm.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, Source: "user_explicit"}}
		}},
		{name: "explorer cursor is not user target", traceRuntime: true, mutate: func(rm *types.RequestModel) {
			rm.RuntimeTargets[0].Source = types.RuntimeTargetSourceExplicitToolCall
		}},
		{name: "full artifact needs discovery", traceRuntime: true, mutate: func(rm *types.RequestModel) {
			rm.RuntimeArtifactScopeProfile.RequestedScope = types.RuntimeArtifactScopeFullArtifact
			rm.RuntimeArtifactScopeProfile.TimeStart = nil
			rm.RuntimeArtifactScopeProfile.TimeEnd = nil
		}},
		{name: "invalid explicit window", traceRuntime: true, mutate: func(rm *types.RequestModel) {
			end := 2.0
			rm.RuntimeArtifactScopeProfile.TimeEnd = &end
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := runtimeArtifactIRFixture()
			rm := runtimeArtifactIRBoundRequest()
			if tc.mutate != nil {
				tc.mutate(&rm)
			}
			applyRuntimeArtifactSourceOptionalIR(&out, rm, tc.traceRuntime)
			probe := runtimeArtifactIRNode(&out.TaskGraph, "primary_probe")
			if probe == nil {
				t.Fatal("incomplete typed discovery must retain the primary probe")
			}
			if edge := runtimeArtifactIREdge(&out.TaskGraph, "primary_probe", "evidence_t0"); edge == nil {
				t.Fatalf("retained probe lost hard evidence edge: %+v", out.TaskGraph.Edges)
			}
		})
	}
}

func runtimeArtifactIRFixture() compiler.Output {
	return compiler.Output{TaskGraph: types.TaskGraph{
		Nodes: []types.TaskNode{
			{ID: "primary_probe", Type: types.NodeProbe, Objective: "discover target"},
			{ID: "evidence_t0", Type: types.NodeEvidence, Objective: "scheduler-state unit", SearchHints: types.SearchHints{EntityIDs: []string{"app-100"}}},
			{ID: "evidence_t1", Type: types.NodeEvidence, Objective: "wakeup-chain unit", SearchHints: types.SearchHints{EntityIDs: []string{"cookie-200", "app-100"}}},
			{ID: "validate", Type: types.NodeValidate},
			{ID: "optional_probe", Type: types.NodeProbe, Objective: "optional refinement", Optional: true, OneShot: true},
			{ID: "finalize", Type: types.NodeFinalize},
		},
		Edges: []types.TaskEdge{
			{From: "primary_probe", To: "evidence_t0", EdgeType: types.EdgeHardDependency},
			{From: "primary_probe", To: "evidence_t1", EdgeType: types.EdgeHardDependency},
			{From: "evidence_t0", To: "validate", EdgeType: types.EdgeHardDependency},
			{From: "evidence_t1", To: "validate", EdgeType: types.EdgeHardDependency},
			{From: "validate", To: "finalize", EdgeType: types.EdgeHardDependency},
			{From: "optional_probe", To: "finalize", EdgeType: types.EdgeSoftDependency},
		},
		ExecutionPolicy: types.ExecutionPolicy{CriticalPath: []string{"primary_probe", "evidence_t0", "validate", "finalize"}},
	}}
}

func runtimeArtifactIRBoundRequest() types.RequestModel {
	start, end := 2.0, 2.02
	return types.RequestModel{
		SubTopics: []types.SubTopic{
			{Summary: "scheduler-state unit", Entities: []string{"app-100"}},
			{Summary: "wakeup-chain unit", Entities: []string{"cookie-200", "app-100"}},
		},
		RuntimeTargetProfile: &types.RuntimeTargetProfile{
			Declaration: types.RuntimeTargetDeclarationNamedTarget,
			SourceQuote: "目标线程 app-100",
		},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "app-100", Source: "user_explicit",
		}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "2.000s 到 2.020s",
		},
	}
}

func runtimeArtifactIRNode(graph *types.TaskGraph, id string) *types.TaskNode {
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == id {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func runtimeArtifactIREdge(graph *types.TaskGraph, from, to string) *types.TaskEdge {
	for i := range graph.Edges {
		if graph.Edges[i].From == from && graph.Edges[i].To == to {
			return &graph.Edges[i]
		}
	}
	return nil
}
