package dataquery

import (
	"bytes"
	"os"
	"testing"
)

// DQA LOC ratchet (2026-07-05). action_runner.go is a 12.5k-line god-file
// (2026-07-05 re-audit, OOM-batch follow-up). Same tripwire shape as
// TestIRDeliveryHotFileLineRatchet in internal/orchestrator: the baseline is
// the actual line count at pin time and may ONLY go down — shrink it by
// splitting concern-specific code (record readers, artifact registry,
// reconcile helpers, custom-transform validation, ...) into sibling files,
// then lower the baseline in the same change. Never raise the baseline to
// make room for new code; new code goes into new files.
func TestActionRunnerLOCRatchet(t *testing.T) {
	const (
		path     = "action_runner.go"
		maxLines = 12475 // baseline pinned 2026-07-05 — only allowed to decrease (lowered same-day again after splitting the assemble_answer projection entry point + answer-level helpers into assemble_answer_projection.go, DLR batch)
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	if lines > maxLines {
		t.Fatalf("%s has %d lines; the DQA LOC ratchet allows at most %d. This file may only shrink: move the new code into a sibling file (or split an existing concern out first and lower the baseline in the same change). Do not raise the baseline.",
			path, lines, maxLines)
	}
}
