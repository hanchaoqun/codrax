package hitraceconv

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseTraceStreamerVerifiedEmbeddedWinsPATHAndKnownLocations(t *testing.T) {
	pathDir := t.TempDir()
	knownDir := t.TempDir()
	releaseWriteUsableTraceStreamer(t, filepath.Join(pathDir, traceStreamerBinaryName()))
	releaseWriteUsableTraceStreamer(t, filepath.Join(knownDir, traceStreamerBinaryName()))
	releaseStarveTraceStreamerOperatorSources(t, pathDir)
	t.Setenv("TRACE_STREAMER_HOME", knownDir)

	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("#!/bin/sh\nexit 0\n")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	releaseWithEmbeddedTraceStreamerState(t, fsys, true, "")
	cache := t.TempDir()
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", cache)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !status.TraceStreamer.Available || !strings.Contains(status.TraceStreamer.Source, "embedded trace_streamer") {
		t.Fatalf("verified embedded tier did not win before ambient sources: %+v", status.TraceStreamer)
	}
	if status.TraceStreamer.Path == filepath.Join(pathDir, traceStreamerBinaryName()) ||
		status.TraceStreamer.Path == filepath.Join(knownDir, traceStreamerBinaryName()) ||
		!strings.HasPrefix(status.TraceStreamer.Path, cache+string(filepath.Separator)) {
		t.Fatalf("embedded selection exposed wrong path: %+v", status.TraceStreamer)
	}
	if releaseContains(status.TraceStreamer.Caveats, "embedded trace_streamer is not usable") {
		t.Fatalf("verified embedded selection carried a failure caveat: %+v", status.TraceStreamer.Caveats)
	}
}

func TestReleaseTraceStreamerOperatorSourcesWinBeforeEmbedded(t *testing.T) {
	for _, source := range []string{"explicit", "environment", "executable-directory"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			tool := filepath.Join(dir, traceStreamerBinaryName())
			releaseWriteUsableTraceStreamer(t, tool)
			releaseStarveTraceStreamerOperatorSources(t, t.TempDir())

			calls := 0
			oldAssets := embeddedTraceStreamerAssetsFS
			oldTag := embeddedTraceStreamerTagEnabled
			oldGap := embeddedTraceStreamerPlatformGap
			embeddedTraceStreamerAssetsFS = func() fs.FS { calls++; return nil }
			embeddedTraceStreamerTagEnabled = true
			embeddedTraceStreamerPlatformGap = "must not be observed"
			t.Cleanup(func() {
				embeddedTraceStreamerAssetsFS = oldAssets
				embeddedTraceStreamerTagEnabled = oldTag
				embeddedTraceStreamerPlatformGap = oldGap
			})

			opts := Options{TraceEngine: traceEngineAuto}
			wantSource := ""
			switch source {
			case "explicit":
				opts.TraceStreamerPath = tool
				wantSource = "configured trace_streamer"
			case "environment":
				t.Setenv("CODRAX_TRACE_STREAMER", tool)
				wantSource = "CODRAX_TRACE_STREAMER"
			case "executable-directory":
				oldExecutable := traceStreamerExecutablePath
				traceStreamerExecutablePath = func() (string, error) {
					return filepath.Join(dir, "codrax"), nil
				}
				t.Cleanup(func() { traceStreamerExecutablePath = oldExecutable })
				wantSource = "codrax executable directory"
			}
			status, err := BuildTraceToolStatus(opts)
			if err != nil {
				t.Fatal(err)
			}
			if !status.TraceStreamer.Available || status.TraceStreamer.Path != tool || status.TraceStreamer.Source != wantSource {
				t.Fatalf("operator source %s did not win: %+v", source, status.TraceStreamer)
			}
			if calls != 0 {
				t.Fatalf("embedded assets were consulted %d time(s) before operator source %s", calls, source)
			}
		})
	}
}

func TestReleaseTraceStreamerEmbeddedFailureFallsThroughAndPersistsProvenance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses /bin/sh")
	}
	dir := t.TempDir()
	pathDir := filepath.Join(dir, "path")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := writeFakeTraceStreamer(t, pathDir, 0)
	releaseStarveTraceStreamerOperatorSources(t, pathDir)

	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("broken embedded payload")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	releaseWithEmbeddedTraceStreamerState(t, fsys, true, "")
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", t.TempDir())

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != tool || !strings.Contains(status.TraceStreamer.Source, "on PATH") {
		t.Fatalf("PATH did not take over after embedded integrity failure: %+v", status.TraceStreamer)
	}
	if !releaseContains(status.TraceStreamer.Caveats, "embedded trace_streamer is not usable") {
		t.Fatalf("status lost embedded integrity failure after PATH takeover: %+v", status.TraceStreamer.Caveats)
	}

	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("trace payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  filepath.Join(dir, "capture.systrace"),
		TraceEngine: traceEngineTraceStreamer,
	})
	if err != nil {
		t.Fatalf("ambient PATH conversion after embedded failure: %v", err)
	}
	decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer)
	if !ok || !decision.Succeeded {
		t.Fatalf("PATH trace_streamer success decision missing: %+v", result.TraceDecisions)
	}
	if !releaseContains(result.Caveats, "embedded trace_streamer is not usable") {
		t.Fatalf("successful Result lost embedded integrity provenance: %+v", result.Caveats)
	}
	meta := releaseReadTraceBundle(t, result.BundlePath)
	if !releaseContains(meta.Caveats, "embedded trace_streamer is not usable") {
		t.Fatalf("successful tracebundle lost embedded integrity provenance: %+v", meta.Caveats)
	}
}

func TestReleaseTraceStreamerEmbeddedFailureFallsThroughToKnownLocation(t *testing.T) {
	knownDir := t.TempDir()
	tool := filepath.Join(knownDir, traceStreamerBinaryName())
	releaseWriteUsableTraceStreamer(t, tool)
	releaseStarveTraceStreamerOperatorSources(t, t.TempDir())
	t.Setenv("TRACE_STREAMER_HOME", knownDir)

	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("broken embedded payload")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	releaseWithEmbeddedTraceStreamerState(t, fsys, true, "")
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", t.TempDir())

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != tool || status.TraceStreamer.Source != "known OpenHarmony/SmartPerf/hmtrace location" {
		t.Fatalf("known-location provider did not take over: %+v", status.TraceStreamer)
	}
	if !releaseContains(status.TraceStreamer.Caveats, "embedded trace_streamer is not usable") {
		t.Fatalf("known-location takeover lost embedded integrity caveat: %+v", status.TraceStreamer.Caveats)
	}
}

func TestReleaseTraceStreamerPlatformGapAndSlimStaySilentAfterAmbientSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses /bin/sh")
	}
	for _, tc := range []struct {
		name       string
		tagEnabled bool
		gap        string
	}{
		{name: "unsupported-default", tagEnabled: true, gap: EmbeddedTraceStreamerPlatformGapMessage("test", "unsupported")},
		{name: "slim-opt-out", tagEnabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			pathDir := filepath.Join(dir, "path")
			if err := os.MkdirAll(pathDir, 0o755); err != nil {
				t.Fatal(err)
			}
			tool := writeFakeTraceStreamer(t, pathDir, 0)
			releaseStarveTraceStreamerOperatorSources(t, pathDir)
			releaseWithEmbeddedTraceStreamerState(t, nil, tc.tagEnabled, tc.gap)

			status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
			if err != nil {
				t.Fatal(err)
			}
			if !status.TraceStreamer.Available || status.TraceStreamer.Path != tool || !strings.Contains(status.TraceStreamer.Source, "on PATH") {
				t.Fatalf("ambient provider did not take over %s build: %+v", tc.name, status.TraceStreamer)
			}
			statusText := strings.Join(append(append([]string(nil), status.TraceStreamer.Caveats...), status.Caveats...), "\n")
			if strings.Contains(statusText, "default embedded trace_streamer tier has no bundled payload") || strings.Contains(statusText, "slim_streamer explicitly disables") {
				t.Fatalf("successful ambient provider disclosed irrelevant platform/slim gap:\n%s", statusText)
			}

			input := filepath.Join(dir, "capture.sys")
			if err := os.WriteFile(input, []byte("trace payload"), 0o644); err != nil {
				t.Fatal(err)
			}
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			result, err := ConvertFile(context.Background(), Options{
				InputPath:   input,
				OutputPath:  filepath.Join(dir, "capture.systrace"),
				TraceEngine: traceEngineTraceStreamer,
			})
			if err != nil {
				t.Fatalf("ambient conversion for %s: %v", tc.name, err)
			}
			resultText := strings.Join(result.Caveats, "\n")
			meta := releaseReadTraceBundle(t, result.BundlePath)
			bundleText := strings.Join(meta.Caveats, "\n")
			for surface, text := range map[string]string{"Result": resultText, "tracebundle": bundleText} {
				if strings.Contains(text, "default embedded trace_streamer tier has no bundled payload") || strings.Contains(text, "slim_streamer explicitly disables") {
					t.Fatalf("%s disclosed irrelevant platform/slim gap after success:\n%s", surface, text)
				}
			}
		})
	}
}

func releaseStarveTraceStreamerOperatorSources(t *testing.T, pathDir string) {
	t.Helper()
	// The fake trace_streamer integration fixture invokes cp. Keep PATH
	// otherwise isolated so an operator-machine trace_streamer cannot make
	// precedence tests nondeterministic.
	if runtime.GOOS != "windows" {
		cp, err := exec.LookPath("cp")
		if err != nil {
			t.Fatalf("discover cp for isolated fake trace_streamer PATH: %v", err)
		}
		cpLink := filepath.Join(pathDir, "cp")
		if _, err := os.Lstat(cpLink); os.IsNotExist(err) {
			if err := os.Symlink(cp, cpLink); err != nil {
				t.Fatalf("link cp into isolated fake trace_streamer PATH: %v", err)
			}
		} else if err != nil {
			t.Fatalf("inspect isolated cp link: %v", err)
		}
	}
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("PATH", pathDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")
	emptyExeDir := t.TempDir()
	oldExecutable := traceStreamerExecutablePath
	traceStreamerExecutablePath = func() (string, error) {
		return filepath.Join(emptyExeDir, "codrax"), nil
	}
	t.Cleanup(func() { traceStreamerExecutablePath = oldExecutable })
}

func releaseWithEmbeddedTraceStreamerState(t *testing.T, fsys fs.FS, enabled bool, gap string) {
	t.Helper()
	oldAssets := embeddedTraceStreamerAssetsFS
	oldTag := embeddedTraceStreamerTagEnabled
	oldGap := embeddedTraceStreamerPlatformGap
	embeddedTraceStreamerAssetsFS = func() fs.FS { return fsys }
	embeddedTraceStreamerTagEnabled = enabled
	embeddedTraceStreamerPlatformGap = gap
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
		embeddedTraceStreamerTagEnabled = oldTag
		embeddedTraceStreamerPlatformGap = oldGap
	})
}

func releaseWriteUsableTraceStreamer(t *testing.T, filePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(filePath, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
}
