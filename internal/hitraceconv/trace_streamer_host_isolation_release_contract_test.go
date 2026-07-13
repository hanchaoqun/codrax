package hitraceconv

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseExplicitUnavailableTraceStreamerIsHostIndependent prevents tests
// and callers from accidentally inheriting a bundled or ambient provider. An
// explicit operator path is authoritative even when the default distribution
// has an embedded payload on the current host.
func TestReleaseExplicitUnavailableTraceStreamerIsHostIndependent(t *testing.T) {
	oldAssets := embeddedTraceStreamerAssetsFS
	oldEnabled := embeddedTraceStreamerTagEnabled
	oldGap := embeddedTraceStreamerPlatformGap
	embeddedCalls := 0
	embeddedTraceStreamerAssetsFS = func() fs.FS {
		embeddedCalls++
		return nil
	}
	embeddedTraceStreamerTagEnabled = true
	embeddedTraceStreamerPlatformGap = "must not be consulted"
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
		embeddedTraceStreamerTagEnabled = oldEnabled
		embeddedTraceStreamerPlatformGap = oldGap
	})

	missing := filepath.Join(t.TempDir(), "explicitly-missing-trace_streamer")
	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto, TraceStreamerPath: missing})
	if err != nil {
		t.Fatal(err)
	}
	if embeddedCalls != 0 {
		t.Fatalf("explicit provider consulted host embedded state %d time(s)", embeddedCalls)
	}
	if status.TraceStreamer.Available || status.TraceStreamer.Path != missing || status.TraceStreamer.Source != "configured trace_streamer" {
		t.Fatalf("explicit missing provider status inherited host state: %+v", status.TraceStreamer)
	}
	if status.PreflightEngine != traceEngineBuiltin || status.FirstLane != traceEngineTraceStreamer {
		t.Fatalf("auto route lost trace_streamer-first/builtin-preflight distinction: %+v", status)
	}
	joined := strings.Join(status.TraceStreamer.Caveats, "\n")
	if len(status.TraceStreamer.Caveats) == 0 || !strings.Contains(joined, missing) {
		t.Fatalf("explicit missing path lacks path-specific status evidence: %+v", status.TraceStreamer.Caveats)
	}
}
