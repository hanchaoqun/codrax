package tool

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This opt-in test reads the local reference test_brbe.data capture without
// copying it into the repository. It proves CPU sample transport only: the
// fixture name is not a claim that BRBE branch records are interpreted.
func TestHMC176BGzipRepresentativePerfSamples(t *testing.T) {
	source := strings.TrimSpace(os.Getenv("CODRAX_TEST_GZIP_PERF_SOURCE"))
	if source == "" {
		t.Skip("set CODRAX_TEST_GZIP_PERF_SOURCE to the local representative test_brbe.data capture")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(source)
	if err != nil || !before.Mode().IsRegular() {
		t.Fatalf("representative source must be a readable regular file: %v", err)
	}
	beforeEntries := hmc176bRepresentativeSourceEntries(t, filepath.Dir(source))
	originalSHA, originalBytes := hmc176bRepresentativeDigest(t, source)
	t.Cleanup(func() {
		afterSHA, afterBytes := hmc176bRepresentativeDigest(t, source)
		after, err := os.Stat(source)
		if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || originalSHA != afterSHA || originalBytes != afterBytes {
			t.Errorf("read-only representative source changed: err=%v bytes=%d->%d sha256=%s->%s", err, originalBytes, afterBytes, originalSHA, afterSHA)
		}
		if afterEntries := hmc176bRepresentativeSourceEntries(t, filepath.Dir(source)); !reflect.DeepEqual(beforeEntries, afterEntries) {
			t.Errorf("conversion wrote beside representative source: before=%v after=%v", beforeEntries, afterEntries)
		}
	})
	hmc176DisableExternalProviders(t)
	gzipPath := filepath.Join(t.TempDir(), "representative compressed 原件.txt")
	hmc176bGzipRepresentativeSource(t, source, gzipPath)
	gzipSHA, gzipBytes := hmc176bRepresentativeDigest(t, gzipPath)
	var gzipMeasurements hmc176bRepresentativeSampleMeasurements
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "gzip", path: gzipPath},
		{name: "bare_reference", path: source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, preparer, conversions := hmc17NamedPathContext(t)
			bus.AttachedHitrace = hmc176UnrelatedText
			// perf_stats is the public CPU-hotspot view; cpu_hotspots is
			// not a supported TraceQuery view or alias.
			params := map[string]any{"source": "path", "path": tc.path, "view": "perf_stats"}
			payload := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if payload.EventCount != 12000 || payload.PerfStats == nil || payload.PerfStats.SampleCount != 12000 || payload.WindowStats == nil || len(payload.WindowStats.EventCounts) != 1 || payload.WindowStats.EventCounts[tracequery.EventPerfSample] != 12000 {
				t.Fatalf("representative capture did not retain exactly 12000 sample-only events: events=%d perf=%+v window=%+v", payload.EventCount, payload.PerfStats, payload.WindowStats)
			}
			if len(payload.WindowStats.TopRunning) != 0 || len(payload.WindowStats.RunnableTop) != 0 || payload.WindowStats.CPUOccupancy != nil {
				t.Fatal("sample population was promoted to scheduler running/runnable evidence")
			}
			measurements := hmc176bRepresentativeMeasurements(payload)
			if len(measurements.Hotspots) == 0 {
				t.Fatal("CPU sample hotspot view published no measured symbol aggregates")
			}
			material, err := preparer.Prepare(context.Background(), tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if material.SourcePath() != tc.path || material.QueryPath() == tc.path || material.SelfContainedText() || payload.SourcePath != material.QueryPath() || conversions.Load() != 1 {
				t.Fatalf("representative query lost source/derived separation or prepared reuse: material=%+v conversions=%d", material, conversions.Load())
			}
			var receipt struct {
				Conversion hitraceconv.Result `json:"conversion"`
			}
			body, err := os.ReadFile(filepath.Join(filepath.Dir(material.QueryPath()), "preparation.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(body, &receipt); err != nil {
				t.Fatal(err)
			}
			if hitraceconv.QueryReadySystracePath(receipt.Conversion) != "" || hitraceconv.QueryReadyPerfTracePath(receipt.Conversion.Artifacts) == "" {
				t.Fatal("representative perf lost sample capability or gained scheduling capability")
			}
			for _, artifact := range receipt.Conversion.Artifacts {
				if artifact.Path != tc.path {
					hmc176bAssertManaged(t, filepath.Dir(material.QueryPath()), artifact.Path)
				}
				if artifact.PerfTransform != nil || artifact.Standalone != nil {
					t.Fatal("top-level representative borrowed embedded HIPERF transform authority")
				}
			}
			for _, artifact := range payload.TraceArtifacts {
				hmc176bAssertManaged(t, filepath.Dir(material.QueryPath()), artifact.SourcePath)
			}
			transport := receipt.Conversion.GzipInputProvenance
			if tc.name == "gzip" {
				if transport == nil || transport.Profile != "gzip_input_v1" || transport.SourceBytes != gzipBytes || transport.SourceSHA256 != gzipSHA || transport.DecodedFormat != "linux_perf_data" || transport.DecodedBytes != originalBytes || transport.DecodedSHA256 != originalSHA || transport.SourceGeneration == "" || transport.DecodedGeneration == "" {
					t.Fatalf("representative gzip receipt did not bind complete outer and inner bytes: %+v", transport)
				}
				gzipMeasurements = measurements
			} else {
				if transport != nil {
					t.Fatal("bare representative source acquired gzip transport provenance")
				}
				if !reflect.DeepEqual(measurements, gzipMeasurements) {
					t.Fatalf("gzip and bare CPU sample measurements differ: gzip=%+v bare=%+v", gzipMeasurements, measurements)
				}
			}
			warm := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, params))
			if warm.PerfStats == nil || warm.PerfStats.SampleCount != 12000 || conversions.Load() != 1 {
				t.Fatalf("warm representative query lost samples or reconverted: conversions=%d", conversions.Load())
			}
			if bus.AttachedHitrace != hmc176UnrelatedText || bus.AttachedTraceMaterial != nil {
				t.Fatal("named representative query replaced unrelated attachment")
			}
			t.Logf("%s: samples=%d events=%d cohorts=%d period=%d decoded_bytes=%d; sample-only, no BRBE semantic claim", tc.name, payload.PerfStats.SampleCount, payload.EventCount, payload.PerfStats.CohortCount, payload.PerfStats.TotalPeriod, originalBytes)
		})
	}
	if afterSHA, afterBytes := hmc176bRepresentativeDigest(t, gzipPath); afterSHA != gzipSHA || afterBytes != gzipBytes {
		t.Fatal("temporary gzip original changed during queries")
	}
}

type hmc176bRepresentativeSampleMeasurements struct {
	Events, Samples, Cohorts int
	Period                   int64
	Start, End               float64
	Hotspots                 []hmc176bRepresentativeHotspot
}

type hmc176bRepresentativeHotspot struct {
	Symbol, DSO, Event, Unit string
	Samples                  int
	Period                   int64
}

func hmc176bRepresentativeMeasurements(payload tracequery.Result) hmc176bRepresentativeSampleMeasurements {
	stats := payload.PerfStats
	out := hmc176bRepresentativeSampleMeasurements{Events: payload.EventCount, Samples: stats.SampleCount, Cohorts: stats.CohortCount, Period: stats.TotalPeriod, Start: payload.TimeStart, End: payload.TimeEnd}
	// Compare public hotspot measurements, not source-specific thread or
	// artifact identities: the two physical captures have distinct origins.
	for _, cohort := range stats.Cohorts {
		for _, hotspot := range cohort.TopSymbols {
			out.Hotspots = append(out.Hotspots, hmc176bRepresentativeHotspot{Symbol: hotspot.Symbol, DSO: hotspot.DSO, Event: cohort.Event, Unit: cohort.WeightUnit, Samples: hotspot.SampleCount, Period: hotspot.Period})
		}
	}
	return out
}

func hmc176bGzipRepresentativeSource(t *testing.T, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	writer := gzip.NewWriter(output)
	if _, err := io.Copy(writer, input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func hmc176bRepresentativeDigest(t *testing.T, path string) (string, int64) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil)), size
}

func hmc176bRepresentativeSourceEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}
