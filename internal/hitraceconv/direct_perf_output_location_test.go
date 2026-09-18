package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestDirectPerfExplicitOutputKeepsBundleAwayFromOriginal(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := t.TempDir()
	input := filepath.Join(sourceDir, "original.capture")
	body := syntheticSimpleperfProtoStream(false, false)
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(outputDir, "selected.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err != nil {
		t.Fatal(err)
	}
	if result.InputPath != input || result.BundlePath != filepath.Join(outputDir, "selected.tracebundle.json") || result.OutputPath != "" {
		t.Fatalf("direct perf original/output authority drifted: %+v", result)
	}
	if ready := QueryReadyPerfTracePath(result.Artifacts); ready != filepath.Join(outputDir, "selected.perftrace") {
		t.Fatalf("wrong sample output: %q", ready)
	}
	if err := tracequery.ValidateTraceInputPath(context.Background(), result.BundlePath); err != nil {
		t.Fatalf("explicit-output bundle is not query ready: %v", err)
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
		t.Fatalf("derived output written beside original: entries=%v err=%v", entries, err)
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("original input changed: %v", err)
	}
	preserved := false
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData && artifact.Path == input && artifact.Bytes == int64(len(body)) {
			preserved = true
		}
	}
	if !preserved {
		t.Fatal("original perf provenance was replaced with a staged input")
	}
}
