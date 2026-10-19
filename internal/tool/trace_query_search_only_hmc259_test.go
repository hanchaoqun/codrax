package tool

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc259NamedSearchOnlyInput(t *testing.T, original []byte) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("native search-only identity currently targets Darwin")
	}
	path := filepath.Join(t.TempDir(), "原始 capture.sys")
	if err := os.WriteFile(path, original, 0o400); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if _, err := os.ReadDir(dir); !os.IsPermission(err) {
		t.Skipf("host does not enforce search-only directory listing: %v", err)
	}
	return path
}

func TestHMC259SearchOnlyPublicBinaryQueriesPreserveCapabilities(t *testing.T) {
	hmc176DisableExternalProviders(t)
	for _, format := range []string{"rmq", "sample_only", "gzip_sample_only"} {
		t.Run(format, func(t *testing.T) {
			original := hmc17NamedBinaryTailFixture()
			if format != "rmq" {
				original = hmc176PerfData(true)
				if format == "gzip_sample_only" {
					original = hmc176Gzip(t, original)
				}
			}
			path := hmc259NamedSearchOnlyInput(t, original)
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			params := map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 100}
			if format == "rmq" {
				params["time_start"], params["time_end"] = 1.0105, 1.012
				params["pid"], params["pattern"] = 424242, "tail-target"
			}
			payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if len(payload.Events) != 1 {
				t.Fatalf("selected full-source event not returned: %+v", payload)
			}
			m, err := preparer.Prepare(context.Background(), path)
			if err != nil || m.SourcePath() != path || m.QueryPath() == path || payload.SourcePath != m.QueryPath() || conversions.Load() != 1 {
				t.Fatalf("original/derived identity or preparation reuse lost: %+v %v", m, err)
			}
			if format == "rmq" {
				if payload.TimeStart != 1.0105 || payload.TimeEnd != 1.012 || payload.Events[0].Type != tracequery.EventSchedWakeup || strings.Contains(m.Preview(), "tail-target") {
					t.Fatalf("binary preview replaced complete tail or changed explicit window: %+v", payload)
				}
			} else {
				bundle := hmc176bReadBundle(t, m.QueryPath())
				perfViews, scheduleViews := 0, 0
				for _, artifact := range bundle.Artifacts {
					if artifact.Type == hitraceconv.ArtifactPerfTrace {
						perfViews++
					}
					if artifact.Type == hitraceconv.ArtifactSystrace {
						scheduleViews++
					}
				}
				// This fixture explicitly records CPU 0. Preserve that observed
				// coordinate without inventing sched-state or causal authority.
				if payload.Events[0].Type != tracequery.EventPerfSample || payload.Events[0].CPU != 0 || perfViews != 1 || scheduleViews != 0 {
					t.Fatalf("sample-only changed source capability or CPU coordinate: %+v %+v", payload.Events[0], bundle.Artifacts)
				}
			}
			again := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if len(again.Events) != 1 || conversions.Load() != 1 || bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named repeat changed attachment or prepared multiple captures")
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("read-only original changed: %v", err)
			}
		})
	}
}

func TestHMC259SearchOnlyPublicSupplementKeepsCausalWindowAndBusiness(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	path := hmc259NamedSearchOnlyInput(t, hmc176Gzip(t, body))
	bus, _, conversions := hmc17NamedPathContext(t)
	start, end := 1.0, 1.05
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause,
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-main", Source: "user_explicit", Confidence: 1}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.000..1.050"},
		AnalyzerHints:               types.AnalyzerHints{ExactTargets: []string{path}},
	}}
	result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": "event_search", "pid": 100, "time_start": start, "time_end": end})
	if !result.Success {
		t.Fatalf("public transport rejected search-only input: %+v", result)
	}
	bus.ToolResults = append(bus.ToolResults, result)
	out := RunTraceQuerySystemSupplement(bus)
	if !out.Attempted || len(out.Executed) == 0 || out.SkipReason != "" {
		t.Fatalf("system supplement did not use complete admitted capture: %+v", out)
	}
	foundIO, foundBusiness := false, false
	for _, result := range bus.Mutable.SystemTraceSupplementResults() {
		if !result.Success {
			continue
		}
		payload := hmc17NamedPayload(t, result)
		if payload.TimeStart != start || payload.TimeEnd != end {
			t.Fatalf("system supplement changed explicit window: %g..%g", payload.TimeStart, payload.TimeEnd)
		}
		if payload.RootCauseRank != nil {
			for _, item := range payload.RootCauseRank.Items {
				if item.Thread.PID == 900 && item.ChainRelevance == "on_chain" {
					t.Fatal("background IO was promoted onto causal chain")
				}
				foundIO = foundIO || item.Type == "io_latency" && item.Thread.PID == 200 && item.ChainRelevance == "on_chain" && item.ResourceCompletionClosure && math.Abs(item.EffectiveImpactMs-31) < 1e-6
			}
		}
		for _, observation := range result.Observations {
			if observation.SourceRef.Path == path {
				t.Fatal("text line coordinates assigned to compressed original")
			}
			foundBusiness = foundBusiness || strings.Contains(observation.Subject, "LoadDocumentIndex") || strings.Contains(observation.Object, "LoadDocumentIndex") || strings.Contains(strings.Join(observation.RichNotes, "\n"), "LoadDocumentIndex")
		}
	}
	if !foundIO || !foundBusiness || conversions.Load() != 1 {
		t.Fatalf("causal IO=%v business=%v preparations=%d", foundIO, foundBusiness, conversions.Load())
	}
}
