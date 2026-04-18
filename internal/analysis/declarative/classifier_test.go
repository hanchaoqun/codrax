package declarative

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyPath_FilenamePatterns(t *testing.T) {
	c := New(DefaultConfig())
	cases := []struct {
		path string
		want Kind
	}{
		{"internal/orchestrator/topology.go", KindTopology},
		{"internal/skill/defaults.go", KindDefaults},
		{"internal/tool/defaults.go", KindDefaults},
		{"internal/tool/builtin.go", KindNone},
		{"cmd/routes.go", KindRoutes},
		{"pkg/enum.go", KindEnum},
		{"providers.yaml", KindSchema},
		{"codrax.yaml", KindSchema},
		{"package.json", KindSchema},
		{"Cargo.toml", KindSchema},
		{"internal/agent/analyzer.go", KindNone},
	}
	for _, tc := range cases {
		got, conf := c.ClassifyPath(tc.path)
		if got != tc.want {
			t.Errorf("ClassifyPath(%q) = %s, want %s (conf=%.2f)", tc.path, got, tc.want, conf)
		}
	}
}

func TestClassify_SmallFileLiteralDensity(t *testing.T) {
	c := New(DefaultConfig())
	// Mimic topology.go's structure.
	smallDecl := []byte(`package orchestrator

var pipelineTopology = map[PipelineStage]struct {
    Agent    AgentName
    Skill    string
}{
    StageAnalyze:  {Agent: AgentAnalyzer, Skill: "analysis-skill"},
    StageExplore:  {Agent: AgentExplorer, Skill: "explore-skill"},
    StageExtract:  {Agent: AgentExtractor, Skill: "extract-skill"},
    StageFinalize: {Agent: AgentFinalizer, Skill: "answer-document-skill"},
}
`)
	// Rename the path so the filename-pattern path does NOT hit —
	// we want to verify the literal-density path alone can classify.
	kind, conf := c.Classify("internal/foo/mapping_ext.go", smallDecl)
	if kind != KindTopology {
		t.Errorf("Classify small literal file = %s, want KindTopology (conf=%.2f)", kind, conf)
	}

	// Large function-body file should NOT classify as declarative.
	bigFunc := []byte(`package agent

func HandleRequest(req *Request) (*Response, error) {
    x := 0
    for i := range req.Items {
        x = x + i
    }
    if x > 10 {
        return nil, fmt.Errorf("too many")
    }
    return &Response{Count: x}, nil
}
`)
	kind, _ = c.Classify("internal/agent/handler.go", bigFunc)
	if kind != KindNone {
		t.Errorf("Classify function-heavy file = %s, want KindNone", kind)
	}
}

func TestBoostFor_ZeroForNone(t *testing.T) {
	c := New(DefaultConfig())
	if c.BoostFor(KindNone) != 0 {
		t.Errorf("BoostFor(KindNone) = %f, want 0", c.BoostFor(KindNone))
	}
	// Topology must be highest.
	if c.BoostFor(KindTopology) <= c.BoostFor(KindRegistry) {
		t.Errorf("BoostFor(KindTopology)=%f not > BoostFor(KindRegistry)=%f", c.BoostFor(KindTopology), c.BoostFor(KindRegistry))
	}
}

// TestDeclarativeFileSet_CoversKnownSurfaces pins the current codrax
// declarative files so future drift is caught. When a new
// declarative file surface is added to the repo (e.g. a new routes
// table, a new registry wiring), add it here explicitly — the test
// refusing to admit the file means either it should be in the set
// or it should be renamed.
func TestDeclarativeFileSet_CoversKnownSurfaces(t *testing.T) {
	c := New(DefaultConfig())
	// These files must classify to a non-None Kind. We don't assert
	// the specific Kind to tolerate minor pattern reshuffles; what
	// matters is that the declarative ranker / analyzer grep gate
	// recognises them at all.
	pinned := []string{
		"internal/orchestrator/topology.go",
		"internal/skill/defaults.go",
		"internal/tool/defaults.go",
	}

	repoRoot := findRepoRootForTest(t)
	for _, rel := range pinned {
		full := filepath.Join(repoRoot, rel)
		if _, err := os.Stat(full); err != nil {
			t.Logf("skip %s (not present in repo): %v", rel, err)
			continue
		}
		kind, _ := c.ClassifyPath(rel)
		if kind == KindNone {
			t.Errorf("known declarative surface %s classified as KindNone — update DefaultFilenamePatterns or rename the file", rel)
		}
	}
}

func findRepoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod ancestor of %s", dir)
	return ""
}

// TestClassifyPath_CaseInsensitive guards against a user-supplied
// uppercase pattern (e.g. the canonical `Topology.go` naming
// convention in some Go subprojects) slipping past the stem matcher.
func TestClassifyPath_CaseInsensitive(t *testing.T) {
	c := New(DefaultConfig())
	for _, p := range []string{"TOPOLOGY.go", "Topology.go", "topology.go"} {
		kind, _ := c.ClassifyPath(p)
		if kind != KindTopology {
			t.Errorf("ClassifyPath(%q) = %s, want KindTopology", p, kind)
		}
	}
	// Partial match: "topology_test.go" should still classify —
	// the matcher uses substring, not exact stem.
	if kind, _ := c.ClassifyPath("topology_test.go"); kind != KindTopology {
		t.Errorf("ClassifyPath(topology_test.go) = %s, want KindTopology", kind)
	}
}

// TestConfigOverrides_RespectUserPatterns verifies a codrax.yaml
// override replaces the default pattern list. Tests set a tiny
// list and a forbidden default term to make sure the default
// fallback is not silently reinstated.
func TestConfigOverrides_RespectUserPatterns(t *testing.T) {
	c := New(Config{
		FilenamePatterns: []string{"wiring"},
		MaxLinesForSmall: 60,
	})
	// "topology" no longer matches after override.
	if kind, _ := c.ClassifyPath("topology.go"); kind != KindNone {
		t.Errorf("ClassifyPath(topology.go) with custom patterns = %s, want KindNone", kind)
	}
	// "wiring" maps to the default KindRegistry (unknown-custom fallback).
	if kind, _ := c.ClassifyPath("internal/wire/wiring.go"); kind == KindNone {
		t.Errorf("ClassifyPath(wiring.go) = KindNone, want some non-None kind")
	}
}

// ensure strings dep is used when imports grow; keep the import
// in case future tests switch to fixture files.
var _ = strings.TrimSpace
