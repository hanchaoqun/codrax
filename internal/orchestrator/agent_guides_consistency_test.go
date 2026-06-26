package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentGuidesDescribeWriteControllerAutoPilot(t *testing.T) {
	root := repoRootForGuideTest(t)
	for _, rel := range []string{"AGENTS.md", "CLAUDE.md"} {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		doc := string(raw)
		for _, want := range []string{
			"controller-first Auto Pilot",
			"route=write",
			"write controller drives a durable dynamic DAG",
		} {
			if !strings.Contains(doc, want) {
				t.Fatalf("%s must describe current write-mode guidance with %q", rel, want)
			}
		}
		for _, stale := range []string{
			"BuildWriteTaskGraph",
			"no classifier route may ever select write",
		} {
			if strings.Contains(doc, stale) {
				t.Fatalf("%s still contains stale write-mode guidance %q", rel, stale)
			}
		}
	}
}

func repoRootForGuideTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		next := filepath.Dir(wd)
		if next == wd {
			t.Fatal("could not locate repo root from test working directory")
		}
		wd = next
	}
}
