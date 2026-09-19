package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestNoSystraceExplicitOutputKeepsBundleAwayFromOriginal(t *testing.T) {
	compressed := gzipHiperfFixture(t, syntheticRawPerfData())
	standalone := syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.0", compressed)
	for _, tc := range []struct {
		name string
		body []byte
		perf bool
	}{
		{"profiler-inventory", syntheticProfilerTraceFile(syntheticProfilerPluginData("unknown_plugin", []byte("opaque"))), false},
		{"profiler-perf-only", append(syntheticProfilerTraceFile(), standalone...), true},
		{"rootless-perf-only", standalone, true},
	} {
		for _, readOnly := range []bool{false, true} {
			name := tc.name
			if readOnly {
				name += "-readonly-source-parent"
			}
			t.Run(name, func(t *testing.T) {
				opts, sourceDir := bundleOutputLocationFixture(t, tc.body, readOnly)
				result, err := ConvertFile(context.Background(), opts)
				if err != nil {
					t.Fatal(err)
				}
				assertNoSystraceBundleOutputLocation(t, opts, result)
				ready := QueryReadyPerfTracePath(result.Artifacts)
				if tc.perf {
					if ready != filepath.Join(filepath.Dir(opts.OutputPath), "selected.perftrace") {
						t.Fatalf("sample-only output not retained: %q", ready)
					}
					if err := tracequery.ValidateTraceInputPath(context.Background(), result.BundlePath); err != nil {
						t.Fatalf("sample-only bundle is not query ready: %v", err)
					}
				} else if ready != "" {
					t.Fatalf("inventory acquired sample capability: %q", ready)
				}
				assertBundleSourceUnchanged(t, sourceDir, opts.InputPath, tc.body)
			})
		}
	}
}

func TestNoSystraceExplicitOutputPreservesExistingBundle(t *testing.T) {
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("unknown_plugin", []byte("opaque")))
	opts, sourceDir := bundleOutputLocationFixture(t, body, false)
	bundle := filepath.Join(filepath.Dir(opts.OutputPath), "selected.tracebundle.json")
	if err := os.WriteFile(bundle, []byte("competitor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConvertFile(context.Background(), opts); err == nil {
		t.Fatal("inventory conversion ignored the selected bundle collision")
	}
	if got, err := os.ReadFile(bundle); err != nil || string(got) != "competitor" {
		t.Fatalf("existing bundle changed: %q %v", got, err)
	}
	if _, err := os.Lstat(opts.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("inventory conversion published systrace: %v", err)
	}
	assertBundleSourceUnchanged(t, sourceDir, opts.InputPath, body)
}

func TestRetainedDBExplicitOutputKeepsBundleAwayFromOriginal(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("local shell fixture and exact retained DB publication require Linux or Darwin")
	}
	body := []byte("modern profiler payload")
	opts, sourceDir := bundleOutputLocationFixture(t, body, true)
	fixtureDB := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
	})
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	opts.TraceEngine = traceEngineTraceStreamer
	opts.TraceStreamerPath = writeFakeTraceStreamer(t, t.TempDir(), 0)
	opts.KeepTraceDB = true
	result, err := ConvertFile(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSystraceBundleOutputLocation(t, opts, result)
	if !hasArtifact(result.Artifacts, ArtifactTraceDB) || QueryReadyPerfTracePath(result.Artifacts) != "" {
		t.Fatalf("DB-only inventory changed capabilities: %+v", result)
	}
	assertBundleSourceUnchanged(t, sourceDir, opts.InputPath, body)
}

func bundleOutputLocationFixture(t *testing.T, body []byte, readOnly bool) (Options, string) {
	t.Helper()
	sourceDir, outputDir := t.TempDir(), t.TempDir()
	input := filepath.Join(sourceDir, "original.capture")
	if err := os.WriteFile(input, body, 0o400); err != nil {
		t.Fatal(err)
	}
	if readOnly {
		if err := os.Chmod(sourceDir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sourceDir, 0o700) })
	}
	return Options{
		InputPath: input, OutputPath: filepath.Join(outputDir, "selected.systrace"),
		RuntimeAnchor: filepath.Join(outputDir, "runtime"), TraceEngine: traceEngineBuiltin, PerfParser: "raw",
	}, sourceDir
}

func assertNoSystraceBundleOutputLocation(t *testing.T, opts Options, result Result) {
	t.Helper()
	outputDir := filepath.Dir(opts.OutputPath)
	if result.OutputPath != "" || result.EventsWritten != 0 || QueryReadySystracePath(result) != "" ||
		result.BundlePath != filepath.Join(outputDir, "selected.tracebundle.json") {
		t.Fatalf("publication location was confused with systrace authority: %+v", result)
	}
	if _, err := os.Lstat(opts.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("no-row conversion created a systrace output: %v", err)
	}
	for _, artifact := range result.Artifacts {
		if filepath.Dir(artifact.Path) != outputDir || artifact.Type == ArtifactSystrace {
			t.Fatalf("artifact escaped selected output directory or claimed systrace: %+v", artifact)
		}
	}
	body, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest traceBundleMetadata
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Systrace != "" {
		t.Fatalf("bundle location minted a primary systrace: %q", manifest.Systrace)
	}
	for _, artifact := range manifest.Artifacts {
		if err := tracebundle.ValidateCapturePath(artifact.Path); err != nil {
			t.Fatalf("bundle child is not bundle-relative: %q: %v", artifact.Path, err)
		}
	}
}

func assertBundleSourceUnchanged(t *testing.T, sourceDir, input string, body []byte) {
	t.Helper()
	entries, err := os.ReadDir(sourceDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
		t.Fatalf("conversion wrote beside original: entries=%v err=%v", entries, err)
	}
	if got, err := os.ReadFile(input); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("original capture changed: %v", err)
	}
}
