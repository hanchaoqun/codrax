package tracediag

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// SEC #27 pins: the provenance header identifies the trace/script by
// basename + size + sha256 — the operator's absolute path (/Users/<name>/…)
// must never appear in the report, and the digest must reconcile byte-exact
// with the trace file directly from the report artifact.
func TestProvenanceHeaderSanitizesPathsAndCarriesDigest(t *testing.T) {
	scriptPath, tracePath, dir := writeRunFixtures(t, runTestScript)
	var buf bytes.Buffer
	failed, err := Run(nil, Options{
		ScriptPath: scriptPath,
		TracePath:  tracePath,
		Version:    "test-1.0",
		BuildTime:  "2026-07-10",
		Now:        fixedNow,
	}, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0\n%s", failed, buf.String())
	}
	report := buf.String()

	// basename-only identity lines.
	sum := sha256.Sum256(mustRead(t, tracePath))
	wantTraceLine := "trace=" + baseName(tracePath) + " size_bytes="
	if !strings.Contains(report, wantTraceLine) {
		t.Errorf("report missing basename trace line %q", wantTraceLine)
	}
	if !strings.Contains(report, "sha256="+hex.EncodeToString(sum[:])) {
		t.Errorf("report missing reconciliation digest sha256=%s", hex.EncodeToString(sum[:]))
	}
	if !strings.Contains(report, "script="+baseName(scriptPath)+" version=") {
		t.Errorf("report missing basename script line")
	}

	// zero absolute-path leakage: the fixture dir (and thus any absolute
	// path under it) must not appear anywhere in the report.
	if strings.Contains(report, dir) {
		t.Errorf("report leaks the collection machine's absolute path %q:\n%s", dir, report)
	}
}

// The automatic v2 lane has its own header renderer. Keep it enrolled in the
// same basename-only policy: open-gap/customer pairing templates use v2, so a
// v1-only pin would leave the highest-volume round-trip surface unprotected.
func TestV2ProvenanceHeaderSanitizesTraceAndScriptPaths(t *testing.T) {
	const v2Script = `
version: 2
description: "v2 provenance security fixture"
steps:
  - label: raw_rows
    view: event_search
    window: "1.0..1.6"
    event_types: [sched_switch]
    max_lines: 20
`
	scriptPath, tracePath, dir := writeRunFixtures(t, v2Script)
	var buf bytes.Buffer
	failed, err := Run(nil, Options{
		ScriptPath: scriptPath,
		TracePath:  tracePath,
		Version:    "test-2.0",
		BuildTime:  "2026-07-10",
		Now:        fixedNow,
	}, &buf)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0\n%s", failed, buf.String())
	}
	report := buf.String()
	sum := sha256.Sum256(mustRead(t, tracePath))
	if !strings.Contains(report, "trace="+baseName(tracePath)+" primary_size_bytes=") {
		t.Errorf("v2 report missing basename trace identity:\n%s", report)
	}
	if !strings.Contains(report, "primary_sha256="+hex.EncodeToString(sum[:])) {
		t.Errorf("v2 report missing byte-exact primary digest:\n%s", report)
	}
	if !strings.Contains(report, "script="+baseName(scriptPath)+" version=2") {
		t.Errorf("v2 report missing basename script identity:\n%s", report)
	}
	if !strings.Contains(report, "source_fingerprint=sha256:") {
		t.Errorf("v2 report lost source-universe lock fingerprint:\n%s", report)
	}
	if strings.Contains(report, dir) {
		t.Errorf("v2 report leaks the collection machine's absolute path %q:\n%s", dir, report)
	}
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
