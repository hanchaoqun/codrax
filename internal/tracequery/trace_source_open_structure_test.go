package tracequery

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalTracePathGuardsWindowsPipesBeforeFilesystemResolution(t *testing.T) {
	source, err := os.ReadFile("parse.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func canonicalTraceIndexPath(path string) string {")
	end := strings.Index(text, "func buildIndexSingleflight(")
	if start < 0 || end <= start {
		t.Fatal("canonical trace path function boundaries not found")
	}
	body := text[start:end]
	guard := strings.Index(body, "traceSourcePathIsBlockingNamespace(path)")
	abs := strings.Index(body, "filepath.Abs(path)")
	eval := strings.Index(body, "filepath.EvalSymlinks(abs)")
	if guard < 0 || abs < 0 || eval < 0 || guard > abs || guard > eval {
		t.Fatalf("Windows named-pipe guard must precede every filesystem path resolver: guard=%d abs=%d eval=%d", guard, abs, eval)
	}
}

func TestSiblingArtifactDiscoveryCannotStatBlockingNamespace(t *testing.T) {
	source, err := os.ReadFile("parse.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func siblingTraceArtifactPaths(")
	end := strings.Index(text, "func traceArtifactBase(")
	if start < 0 || end <= start {
		t.Fatal("sibling artifact discovery function boundaries not found")
	}
	body := text[start:end]
	guard := strings.Index(body, "traceSourcePathIsBlockingNamespace(path)")
	discovery := strings.Index(body, "filegeneration.FromPath(candidate)")
	if guard < 0 || discovery < 0 || guard > discovery || strings.Contains(body, "os.Stat(") {
		t.Fatalf("sibling discovery must guard blocking namespaces before safe path identity lookup: guard=%d discovery=%d", guard, discovery)
	}
}

func TestSchedulerHeadMemoCommitsOnlyAfterSourceClose(t *testing.T) {
	source, err := os.ReadFile("scheduler_head_snapshot.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func sourceSchedulerHeadSnapshot(")
	end := strings.Index(text, "func schedulerHeadSourceIdentityMatches(")
	if start < 0 || end <= start {
		t.Fatal("scheduler-head source function boundaries not found")
	}
	body := text[start:end]
	closeAt := strings.Index(body, "f.Close()")
	storeAt := strings.Index(body, "schedulerHeadCache.store(")
	if closeAt < 0 || storeAt < 0 || closeAt > storeAt {
		t.Fatalf("scheduler-head memo must publish only after source close succeeds: close=%d store=%d", closeAt, storeAt)
	}
}
