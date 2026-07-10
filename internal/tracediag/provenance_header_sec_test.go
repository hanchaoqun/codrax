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
// with the trace file (playbook trace_sha256.txt cross-check).
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

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
