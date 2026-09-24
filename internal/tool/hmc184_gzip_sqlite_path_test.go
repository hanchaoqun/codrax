package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC184GzipSQLiteNamedQueryAndColdSupplement(t *testing.T) {
	for _, lane := range []string{"query", "cold supplement"} {
		t.Run(lane, func(t *testing.T) {
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_gzip_sqlite/capture.transport")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "compressed 数据.capture")
			if err := os.WriteFile(path, body, 0o400); err != nil {
				t.Fatal(err)
			}
			bus, preparer, preparations := hmc17NamedPathContext(t)
			if lane == "query" {
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "jank_event_sync", "time_start": 1.0, "time_end": 1.2, "event_field_filters": []map[string]string{{"field": "jank_frames", "op": "gte", "value": "2"}}, "limit": 10})
				payload := hmc17NamedPayload(t, result)
				if !result.Success || len(payload.Events) != 2 || payload.TimeStart != 1 || payload.TimeEnd != 1.2 {
					t.Fatalf("compressed marker inventory/window: %+v", payload)
				}
			} else {
				// Explicit preparation provides trace intent for the opaque name;
				// a path in natural-language prose alone is not authority.
				if _, err := preparer.Prepare(t.Context(), path); err != nil {
					t.Fatal(err)
				}
				suppCoreSetConfig(t, true, 8<<20, 20*time.Second, 120)
				start, end := 1.0, 1.2
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
					Intent:                      types.IntentRootCause,
					RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 27599, Thread: "main", Source: "user_explicit", Confidence: 1}},
					RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[1.0,1.2)"},
					AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
				}}
				out := RunTraceQuerySystemSupplement(bus)
				if !out.Attempted || len(out.Executed) == 0 || out.SkipReason != "" {
					t.Fatalf("cold compressed supplement: %+v", out)
				}
				for _, result := range bus.Mutable.SystemTraceSupplementResults() {
					payload := hmc17NamedPayload(t, result)
					if payload.TimeStart != start || payload.TimeEnd != end || payload.SourcePath == path {
						t.Fatalf("supplement changed explicit coordinates: %+v", payload)
					}
				}
			}
			m, err := preparer.Prepare(t.Context(), path)
			if err != nil || m == nil || preparations.Load() != 1 {
				t.Fatalf("preparation not shared: %v %v %d", m, err, preparations.Load())
			}
			if bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("named gzip became sticky attachment")
			}
			// Same bytes and mtime, different original generation must invalidate
			// the prepared material and the warm query index together.
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			replacement := path + ".replacement"
			if err := os.WriteFile(replacement, body, 0o400); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, path); err != nil {
				t.Fatal(err)
			}
			if err := m.Validate(t.Context(), m.Preview()); err == nil {
				t.Fatal("old compressed material remained valid")
			}
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
			if result.Success || len(result.Observations) != 0 {
				t.Fatal("cached query reused replaced compressed input")
			}
		})
	}
}
