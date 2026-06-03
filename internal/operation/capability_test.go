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
