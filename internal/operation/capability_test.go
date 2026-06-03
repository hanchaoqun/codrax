package operation

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCapabilitySnapshotRendersEnvFactsAndPolicy(t *testing.T) {
	facts := &types.EnvFacts{
		OS:           "linux",
		Arch:         "amd64",
		OSFamily:     "debian",
		OSVersion:    "ubuntu 24.04",
		Shell:        "bash",
		GitRepoState: "ready",
		ProbedAt:     time.Now(),
		PkgManagers: map[string]types.PkgManagerInfo{
			"git": {Path: "/usr/bin/git", Version: "2.44.0"},
			"npm": {Path: "/usr/bin/npm", Version: "10.0.0"},
		},
		ProjectFiles: map[string]string{
			"go.mod":       "/repo/go.mod",
			"package.json": "/repo/package.json",
		},
		Pythons: []types.PythonInterp{{
			Path:    "/usr/bin/python3",
			Version: "3.12.3",
			Origin:  "system",
		}},
		Nodes: []types.NodeInterp{{
			Path:        "/usr/bin/node",
			Version:     "22.0.0",
			PkgManagers: []string{"npm"},
		}},
	}
	policy := DefaultCommandPolicy()
	policy.AutoLowRisk = true
	policy.TimeoutMS = 12345
	policy.NetworkPolicy = ApprovalDenied
	policy.AllowedWriteRoots = []string{"/repo/out"}

	snapshot := BuildCapabilitySnapshot(facts, "/repo", policy)
	rendered := snapshot.RenderForPrompt()
	for _, want := range []string{
		"## capability_snapshot",
		"os: linux/amd64",
		"family=debian",
		"git_repo_state: ready",
		"auto_approve=true",
		"auto_low_risk=true",
		"timeout_ms=12345",
		"network=denied",
		"allowed_write_roots=/repo/out",
		"git=/usr/bin/git(2.44.0)",
		"npm=/usr/bin/npm(10.0.0)",
		"python=/usr/bin/python3(3.12.3)",
		"node=/usr/bin/node(22.0.0)",
		"go.mod=/repo/go.mod",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, rendered)
		}
	}
}

func TestCapabilitySnapshotRendersOperationProviderDescriptors(t *testing.T) {
	policy := DefaultCommandPolicy()
	snapshot := BuildCapabilitySnapshotWithProviders(nil, "/repo", policy, []ProviderInfo{
		{
			Name:         "mcp:slides",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run_operation",
			Description:  "Create and verify local PPTX decks from a structured request.",
			InputSchema:  `{"type":"object","properties":{"topic":{"type":"string"}}}`,
			Examples:     []string{"Generate a six slide summary deck", "Create a customer update deck"},
			Source:       "mcp",
			LazyStart:    true,
			Loaded:       false,
		},
		{
			Name:         "skill:local_ppt",
			Kind:         "presentation_generation",
			Surfaces:     []string{"slides"},
			SideEffects:  []string{"local_file_write"},
			RequiresGate: true,
			ToolName:     "run",
			Description:  "Run a local PPT skill through a manifest command.",
			WhenToUse:    []string{"Use when the user needs a local PPTX artifact."},
			WhenNotToUse: []string{"Do not use for pure source-code explanation."},
			InputSchema:  `{"output_path":"string"}`,
			Examples:     []string{"Create a training deck from notes."},
			Workflows: []WorkflowInfo{{
				Name:           "manual_to_deck",
				Summary:        "Read a manual, extract commands, then build slides.",
				Entry:          true,
				OperationKind:  "external_skill_workflow",
				TargetSurface:  "slides",
				NextProviders:  []string{"skill:ppt_builder"},
				ReturnProvider: "skill:local_ppt",
				RequiredInputs: []string{"manual_path", "output_path"},
				Steps: []WorkflowStepInfo{{
					ID:            "build_deck",
					Provider:      "skill:ppt_builder",
					OperationKind: "presentation_generation",
					TargetSurface: "slides",
					Description:   "Generate slides from extracted notes.",
				}},
				Examples: []string{"Read tool.md and build out/tool.pptx."},
			}},
			OutputContract: OutputContractInfo{ArtifactRefs: true, PayloadRef: true, NextActions: true, WorkflowState: true},
			Source:         "skill",
			LazyStart:      true,
			Loaded:         false,
		},
	})
	rendered := snapshot.RenderForPrompt()
	for _, want := range []string{
		"operation_providers",
		"mcp:slides kind=presentation_generation",
		"surfaces=slides",
		"side_effects=local_file_write",
		"gate=true lazy=true loaded=false",
		"tool=run_operation",
		"source=mcp",
		"description=Create and verify local PPTX decks",
		"input_schema=",
		"examples=Generate a six slide summary deck",
		"skill:local_ppt kind=presentation_generation",
		"tool=run",
		"source=skill",
		"when_to_use=Use when the user needs a local PPTX artifact.",
		"when_not_to_use=Do not use for pure source-code explanation.",
		"workflow[manual_to_deck]",
		"kind=external_skill_workflow",
		"target=slides",
		"next=skill:ppt_builder",
		"return=skill:local_ppt",
		"steps=build_deck provider=skill:ppt_builder",
		"output_contract=artifact_refs:true payload_ref:true next_actions:true return_action:false workflow_state:true",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, rendered)
		}
	}
	for _, forbidden := range []string{"command=", "env=", "work_dir="} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("snapshot leaked %q:\n%s", forbidden, rendered)
		}
	}
}

func TestCapabilitySnapshotPreservesSameProviderKindDifferentSurfaces(t *testing.T) {
	policy := DefaultCommandPolicy()
	snapshot := BuildCapabilitySnapshotWithProviders(nil, "/repo", policy, []ProviderInfo{
		{Name: "skill:manual_reader", Kind: "presentation_generation", Surfaces: []string{"local_file"}, ToolName: "run", Source: "skill"},
		{Name: "skill:manual_reader", Kind: "presentation_generation", Surfaces: []string{"local_file", "slides"}, ToolName: "run", Source: "skill"},
	})
	rendered := snapshot.RenderForPrompt()
	if strings.Count(rendered, "skill:manual_reader kind=presentation_generation") != 2 {
		t.Fatalf("same kind with distinct surfaces should be preserved:\n%s", rendered)
	}
	if !strings.Contains(rendered, "surfaces=local_file,slides") {
		t.Fatalf("workflow surface descriptor missing:\n%s", rendered)
	}
}
