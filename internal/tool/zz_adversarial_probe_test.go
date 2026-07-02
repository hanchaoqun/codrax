package tool

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Temporary adversarial probe (review #2): for the exact input of the
// "readable nonruntime suffix-looking file does not get runtime advisory"
// subtest, compare nil ctx vs trace-active ctx.
func TestZZAdversarialProbeNilVsActiveCtx(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "notes.systrace")
	if err := os.WriteFile(tmpFile, []byte("package demo\nfunc Execute() {}\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	command := `awk '/Execute/ { print $0 }' ` + strconv.Quote(tmpFile)
	output := "func Execute() {}\n"

	gotNil := execCommandSearchShapeAdvisory(nil, command, output, nil)
	gotActive := execCommandSearchShapeAdvisory(traceAdvisoryBusContext(), command, output, nil)
	t.Logf("nil ctx advisory: %q", gotNil)
	t.Logf("active ctx advisory: %q", gotActive)
	if gotNil != gotActive {
		t.Fatalf("nil vs active ctx diverge for this input: nil=%q active=%q", gotNil, gotActive)
	}
	if gotNil != "" {
		t.Fatalf("expected empty advisory, got %q", gotNil)
	}
}
