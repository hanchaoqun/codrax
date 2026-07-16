package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Release contract: a successful explicit trace_streamer conversion uses its
// SQLite DB only as an internal staging artifact unless the operator opts in to
// retention. No deleted staging path may escape through Result, decisions, or
// tracebundle metadata.
func TestReleaseTraceStreamerTransientDBLeavesNoArtifactOrFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, []byte("trace payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	argsLog := filepath.Join(dir, "args.log")
	tool := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")

	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        filepath.Join(dir, "capture.systrace"),
		TraceEngine:       traceEngineTraceStreamer,
		TraceStreamerPath: tool,
	})
	if err != nil {
		t.Fatalf("explicit trace_streamer conversion: %v", err)
	}
	stagingDB := releaseTraceStreamerDBArg(t, argsLog)
	for _, path := range []string{stagingDB, stagingDB + ".ohos.ts", filepath.Dir(stagingDB)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("transient trace DB path survived successful conversion: %s err=%v", path, err)
		}
	}
	if artifact, ok := releaseArtifactByType(result.Artifacts, ArtifactTraceDB); ok {
		t.Fatalf("transient trace DB escaped as Result artifact: %+v", artifact)
	}
	decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer)
	if !ok || !decision.Succeeded || decision.DBPath != "" {
		t.Fatalf("successful transient provider decision leaked/omitted DB state: %+v", result.TraceDecisions)
	}
	meta := releaseReadTraceBundle(t, result.BundlePath)
	if artifact, ok := releaseArtifactByType(meta.Artifacts, ArtifactTraceDB); ok {
		t.Fatalf("transient trace DB escaped into tracebundle: %+v", artifact)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle, []byte(stagingDB)) {
		t.Fatalf("tracebundle leaked deleted staging DB path %q:\n%s", stagingDB, bundle)
	}
}

// Release contract: both retention surfaces execute trace_streamer against a
// private staging directory, then publish the DB and optional timestamp
// companion as the requested final pair. Result/decision/bundle provenance
// names only the published path.
func TestReleaseTraceStreamerRetainedDBPublishesStagedPair(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses /bin/sh")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("exact retained DB publication is intentionally fail-closed on this platform")
	}
	for _, tc := range []struct {
		name      string
		configure func(*Options, string)
		final     func(string, string) string
	}{
		{
			name: "keep-derived-path",
			configure: func(opts *Options, _ string) {
				opts.KeepTraceDB = true
			},
			final: func(input, output string) string {
				return traceSidecarBase(input, output) + ".trace.db"
			},
		},
		{
			name: "explicit-output-path",
			configure: func(opts *Options, final string) {
				opts.TraceDBOutputPath = final
			},
			final: func(_, _ string) string { return "" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			explicitFinal := filepath.Join(dir, "operator.trace.db")
			if err := os.WriteFile(input, []byte("trace payload"), 0o644); err != nil {
				t.Fatal(err)
			}
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			argsLog := filepath.Join(dir, "args.log")
			tool := writeFakeTraceStreamer(t, dir, 0)
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
			t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")

			opts := Options{
				InputPath:         input,
				OutputPath:        output,
				TraceEngine:       traceEngineTraceStreamer,
				TraceStreamerPath: tool,
			}
			tc.configure(&opts, explicitFinal)
			finalDB := tc.final(input, output)
			if finalDB == "" {
				finalDB = explicitFinal
			}
			result, err := ConvertFile(context.Background(), opts)
			if err != nil {
				t.Fatalf("retained trace_streamer conversion: %v", err)
			}

			stagingDB := releaseTraceStreamerDBArg(t, argsLog)
			if stagingDB == finalDB || filepath.Dir(stagingDB) == filepath.Dir(finalDB) {
				t.Fatalf("trace_streamer wrote directly to public DB path: staging=%q final=%q", stagingDB, finalDB)
			}
			for _, path := range []string{stagingDB, stagingDB + ".ohos.ts", filepath.Dir(stagingDB)} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("private staging path survived publish: %s err=%v", path, err)
				}
			}
			fixtureBody, err := os.ReadFile(fixtureDB)
			if err != nil {
				t.Fatal(err)
			}
			finalBody, err := os.ReadFile(finalDB)
			if err != nil {
				t.Fatalf("published DB missing: %v", err)
			}
			if !bytes.Equal(finalBody, fixtureBody) {
				t.Fatal("published DB bytes differ from trace_streamer staging output")
			}
			companionBody, err := os.ReadFile(finalDB + ".ohos.ts")
			if err != nil || !bytes.Contains(companionBody, []byte("fake ohos timestamp sidecar")) {
				t.Fatalf("published timestamp companion malformed: body=%q err=%v", companionBody, err)
			}
			artifact, ok := releaseArtifactByType(result.Artifacts, ArtifactTraceDB)
			if !ok || artifact.Path != finalDB || artifact.Bytes == 0 || !releaseContains(artifact.Caveats, "timestamp_companion="+finalDB+".ohos.ts") {
				t.Fatalf("retained Result provenance malformed: artifact=%+v all=%+v", artifact, result.Artifacts)
			}
			decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer)
			if !ok || !decision.Succeeded || decision.DBPath != finalDB {
				t.Fatalf("retained provider decision malformed: %+v", result.TraceDecisions)
			}
			meta := releaseReadTraceBundle(t, result.BundlePath)
			bundleArtifact, ok := releaseArtifactByType(meta.Artifacts, ArtifactTraceDB)
			if !ok || releaseBundleArtifactPhysicalPath(result.BundlePath, bundleArtifact.Path) != finalDB || !releaseContains(bundleArtifact.Caveats, "timestamp_companion="+finalDB+".ohos.ts") {
				t.Fatalf("retained tracebundle provenance malformed: %+v", meta.Artifacts)
			}
		})
	}
}

// A racing companion-file creator must not leave a half-published DB. The
// external companion is not conversion-owned and must remain byte-preserved.
func TestReleaseTraceStreamerCompanionPublishFailureRollsBackDBOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses /bin/sh")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("exact retained DB publication is intentionally fail-closed on this platform")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	output := filepath.Join(dir, "capture.systrace")
	finalDB := filepath.Join(dir, "operator.trace.db")
	finalCompanion := finalDB + ".ohos.ts"
	if err := os.WriteFile(input, []byte("trace payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	argsLog := filepath.Join(dir, "args.log")
	tool := filepath.Join(dir, "trace_streamer_race")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$TRACE_STREAMER_ARGS_LOG"
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-e" ]; then shift; out="$1"; fi
  shift
done
cp "$TRACE_STREAMER_FIXTURE_DB" "$out"
printf 'staged timestamp\n' > "$out.ohos.ts"
printf 'external owner\n' > "$TRACE_STREAMER_RACE_COMPANION"
`
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_RACE_COMPANION", finalCompanion)

	_, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceEngine:       traceEngineTraceStreamer,
		TraceStreamerPath: tool,
		TraceDBOutputPath: finalDB,
	})
	if err == nil || !strings.Contains(err.Error(), "publish trace DB timestamp companion") {
		t.Fatalf("expected companion publish collision, got %v", err)
	}
	if _, err := os.Lstat(finalDB); !os.IsNotExist(err) {
		t.Fatalf("half-published DB survived companion failure: err=%v", err)
	}
	body, err := os.ReadFile(finalCompanion)
	if err != nil || string(body) != "external owner\n" {
		t.Fatalf("non-owned racing companion was changed: body=%q err=%v", body, err)
	}
	stagingDB := releaseTraceStreamerDBArg(t, argsLog)
	for _, path := range []string{stagingDB, stagingDB + ".ohos.ts", filepath.Dir(stagingDB)} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("staging path survived failed pair publish: %s err=%v", path, err)
		}
	}
}

type releaseTraceBundle struct {
	Artifacts      []Artifact              `json:"artifacts"`
	TraceDecisions []TraceProviderDecision `json:"trace_provider_decisions"`
	Caveats        []string                `json:"caveats"`
}

func releaseReadTraceBundle(t *testing.T, bundlePath string) releaseTraceBundle {
	t.Helper()
	body, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var meta releaseTraceBundle
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatalf("parse tracebundle: %v\n%s", err, body)
	}
	return meta
}

func releaseBundleArtifactPhysicalPath(bundlePath, artifactPath string) string {
	if filepath.IsAbs(artifactPath) {
		return filepath.Clean(artifactPath)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(bundlePath), filepath.FromSlash(artifactPath)))
}

func releaseTraceStreamerDBArg(t *testing.T, argsLog string) string {
	t.Helper()
	body, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-e" {
			if strings.TrimSpace(args[i+1]) == "" {
				t.Fatalf("trace_streamer -e argument is empty: %q", body)
			}
			return args[i+1]
		}
	}
	t.Fatalf("trace_streamer args have no -e DB path: %q", body)
	return ""
}

func releaseArtifactByType(artifacts []Artifact, typ string) (Artifact, bool) {
	for _, artifact := range artifacts {
		if artifact.Type == typ {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func releaseTraceDecision(decisions []TraceProviderDecision, provider string) (TraceProviderDecision, bool) {
	for _, decision := range decisions {
		if decision.ProviderName == provider {
			return decision, true
		}
	}
	return TraceProviderDecision{}, false
}

func releaseContains(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
