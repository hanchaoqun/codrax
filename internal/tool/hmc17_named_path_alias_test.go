package tool

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The selected token is an observed symlink, not its current destination.
// Warming both physical captures must not allow candidate de-duplication to
// replace that token with B's valid query path after the symlink moves A→B.
func hmc17RetargetedNamedAlias(t *testing.T) *types.BusContext {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs host-specific privileges on Windows")
	}
	bus, preparer, conversions := hmc17NamedPathContext(t)
	dir := t.TempDir()
	first, second, alias := filepath.Join(dir, "a.sys"), filepath.Join(dir, "b.sys"), filepath.Join(dir, "selected.sys")
	original := hmc17NamedBinaryTailFixture()
	other := bytes.ReplaceAll(original, []byte("tail-target"), []byte("evil-target"))
	for path, body := range map[string][]byte{first: original, second: other} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, alias, second} {
		if _, err := preparer.Prepare(context.Background(), path); err != nil {
			t.Fatalf("prepare original capture/observed alias: %s: %v", path, err)
		}
	}
	if conversions.Load() != 2 {
		t.Fatalf("fixture prepared %d physical captures, want 2", conversions.Load())
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, alias); err != nil {
		t.Fatal(err)
	}
	start, end := 1.0105, 1.012
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:                      types.IntentRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 424242, Thread: "tail-target", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.0105..1.012"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{alias}},
	}}
	return bus
}

func TestHMC17NamedRetargetedAliasSourceOmittedCannotBorrowOtherPreparedCapture(t *testing.T) {
	bus := hmc17RetargetedNamedAlias(t)
	result := hmc17NamedQuery(t, bus, map[string]any{
		"view": "event_search", "pattern": "evil-target", "time_start": 1.0105, "time_end": 1.012,
	})
	if result.Success || len(result.Observations) != 0 {
		t.Fatalf("omitted-source selection bypassed the observed alias generation and borrowed capture B: %+v", result)
	}
}

func TestHMC17NamedRetargetedAliasColdSupplementCannotBorrowOtherPreparedCapture(t *testing.T) {
	bus := hmc17RetargetedNamedAlias(t)
	suppCoreSetConfig(t, true, 128<<20, 20*time.Second, 120)
	out := RunTraceQuerySystemSupplement(bus)
	if len(out.Executed) != 0 {
		t.Fatalf("cold supplement bypassed selected alias generation and queried capture B: %+v", out)
	}
	for _, result := range bus.Mutable.SystemTraceSupplementResults() {
		if result.Success || len(result.Observations) != 0 {
			t.Fatalf("cold supplement published evidence from unselected capture B: %+v", result)
		}
	}
}
