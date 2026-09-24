package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC17ExistingSQLiteNamedQueryAndColdSupplement(t *testing.T) {
	for _, lane := range []string{"query", "cold supplement"} {
		t.Run(lane, func(t *testing.T) {
			body, err := os.ReadFile("../../eval/fixtures/hmosperf_existing_sqlite/capture.data")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "capture.data")
			if err := os.WriteFile(path, body, 0o400); err != nil {
				t.Fatal(err)
			}
			bus, preparer, preparations := hmc17NamedPathContext(t)
			if lane == "query" {
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "jank_event_sync", "time_start": 1.0, "time_end": 1.2, "event_field_filters": []map[string]string{{"field": "jank_frames", "op": "gte", "value": "2"}}, "limit": 10})
				payload := hmc17NamedPayload(t, result)
				if len(payload.Events) != 2 {
					t.Fatalf("filtered database marker inventory: %+v", payload.Events)
				}
				for _, row := range payload.Events {
					if !strings.Contains(row.Raw, "appid=27599") {
						t.Fatalf("marker payload lost: %+v", row)
					}
				}
			} else {
				// Cold means no model query or index yet. A settled input
				// receipt identifies this otherwise-generic filename as trace,
				// just as typed pre-entry/CLI/REPL do. A random DB path string
				// alone must not acquire trace intent.
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
					t.Fatalf("cold SQLite supplement did not execute: %+v", out)
				}
				for _, result := range bus.Mutable.SystemTraceSupplementResults() {
					payload := hmc17NamedPayload(t, result)
					if payload.TimeStart != start || payload.TimeEnd != end || payload.SourcePath == path {
						t.Fatalf("supplement changed explicit window/source coordinates: %+v", payload)
					}
				}
			}
			m, err := preparer.Prepare(t.Context(), path)
			if err != nil || m == nil || preparations.Load() != 1 {
				t.Fatalf("SQLite preparation not shared: %v %v %d", m, err, preparations.Load())
			}
			if bus.AttachedTraceMaterial != nil || bus.AttachedHitrace != "" {
				t.Fatal("named SQLite became a sticky attachment")
			}
			if err := os.WriteFile(path+"-wal", []byte("active"), 0o600); err != nil {
				t.Fatal(err)
			}
			result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search"})
			if result.Success || len(result.Observations) != 0 {
				t.Fatal("cached query ignored new SQLite sidecar")
			}
		})
	}
}
