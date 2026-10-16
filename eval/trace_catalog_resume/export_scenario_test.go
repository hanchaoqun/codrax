package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestExportScenarioChangesOnlyRequestedDeliverable(t *testing.T) {
	out := filepath.Join(t.TempDir(), "evidence-export")
	r, err := prepareScenario(context.Background(), out, "../fixtures/hmosperf_catalog_objects", "capture-review")
	if err != nil {
		t.Fatal(err)
	}
	want := question + " 并将本次检查的证据包导出为 capture-review。"
	if r.ModelCalls != 0 || r.Question != want || r.ExportTarget != "capture-review" || r.SeedOutcomes[tracecatalog.OutcomeSuccess] != 1 || r.SeedOutcomes[tracecatalog.OutcomeNotExecuted] != 3 {
		t.Fatalf("export scenario altered the operator checkpoint: %+v", r)
	}
	b, err := os.ReadFile(filepath.Join(out, "question.txt"))
	if err != nil || string(b) != want+"\n" {
		t.Fatalf("question differs: %v %s", err, b)
	}
	for _, base := range []string{r.Workspace, filepath.Join(out, "operator")} {
		if _, err := os.Stat(filepath.Join(base, "evidence-exports")); !os.IsNotExist(err) {
			t.Fatalf("operator exported before the live model: %v", err)
		}
	}
}

func TestExportScenarioRejectsPathsBeforeCreatingEvidenceDirectory(t *testing.T) {
	for _, target := range []string{"../outside", "a/b", "/absolute", "a\nnew-question"} {
		out := filepath.Join(t.TempDir(), "evidence")
		if _, err := prepareScenario(context.Background(), out, "missing", target); err == nil {
			t.Fatalf("accepted target %q", target)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("invalid target created a directory: %v", err)
		}
	}
}
