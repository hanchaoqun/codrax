package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Fallback describes what actually happened in this route, not an intrinsic
// property of the provider implementation. Explicit builtin is a single lane;
// auto builtin is the second lane after trace_streamer.
func TestReleaseTraceProviderFallbackFlagMatchesRequestedRoute(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		opts         Options
		wantFallback bool
	}{
		{
			name: "explicit-builtin",
			opts: Options{
				InputPath:   input,
				OutputPath:  filepath.Join(dir, "explicit.systrace"),
				TraceEngine: traceEngineBuiltin,
			},
			wantFallback: false,
		},
		{
			name: "auto-builtin-after-unavailable-sql",
			opts: Options{
				InputPath:         input,
				OutputPath:        filepath.Join(dir, "auto.systrace"),
				TraceEngine:       traceEngineAuto,
				TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
			},
			wantFallback: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ConvertFile(context.Background(), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameBuiltinSys)
			if !ok || !decision.Succeeded || decision.Fallback != tc.wantFallback {
				t.Fatalf("Result fallback provenance drifted: decision=%+v all=%+v", decision, result.TraceDecisions)
			}
			meta := releaseReadTraceBundle(t, result.BundlePath)
			bundleDecision, ok := releaseTraceDecision(meta.TraceDecisions, traceProviderNameBuiltinSys)
			if !ok || bundleDecision.Fallback != tc.wantFallback {
				t.Fatalf("tracebundle fallback provenance drifted: decision=%+v all=%+v", bundleDecision, meta.TraceDecisions)
			}
		})
	}
}
