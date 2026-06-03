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
			Source:       "skill",
			LazyStart:    true,
			Loaded:       false,
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
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("snapshot missing %q:\n%s", want, rendered)
		}
	}
}
