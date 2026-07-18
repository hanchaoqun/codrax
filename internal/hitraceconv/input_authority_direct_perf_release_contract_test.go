package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReleaseDirectPerfReaderAtWinsOverDisplayPath(t *testing.T) {
	tests := []struct {
		name        string
		format      perfInputFormat
		opts        Options
		authority   func() []byte
		pathPayload func([]byte) []byte
		want        string
		reject      string
	}{
		{
			name:      "raw",
			format:    perfInputLinuxPerfData,
			opts:      Options{PerfParser: "raw"},
			authority: syntheticRawPerfData,
			pathPayload: func(body []byte) []byte {
				return bytes.Replace(body, []byte("app\x00"), []byte("bad\x00"), 1)
			},
			want:   `thread_comm="app"`,
			reject: `thread_comm="bad"`,
		},
		{
			name:      "simpleperf-proto",
			format:    perfInputSimpleperfReportProto,
			authority: func() []byte { return syntheticSimpleperfProtoStream(false, false) },
			pathPayload: func(body []byte) []byte {
				return bytes.Replace(body, []byte("Render Thread"), []byte("Path Reopened"), 1)
			},
			want:   `thread_comm="Render Thread"`,
			reject: `thread_comm="Path Reopened"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "display-path-input.bin")
			authorityBody := test.authority()
			pathBody := test.pathPayload(append([]byte(nil), authorityBody...))
			if bytes.Equal(authorityBody, pathBody) || len(authorityBody) != len(pathBody) {
				t.Fatalf("fixture did not create an equal-size competing path generation")
			}
			if err := os.WriteFile(path, pathBody, 0o600); err != nil {
				t.Fatal(err)
			}
			view := newScriptedStandaloneInputView(path, authorityBody)
			binding, err := newDirectPerfInputBinding(view, test.format)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cleanupErr := ledger.cleanup(); cleanupErr != nil {
					t.Errorf("cleanup: %v", cleanupErr)
				}
			}()
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				context.Background(), test.opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, filepath.Join(dir, "unused.systrace"), ledger,
			)
			if err != nil || !ok {
				t.Fatalf("direct conversion ok=%t err=%v", ok, err)
			}
			perfTrace := directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace)
			if perfTrace.Path == "" || result.BundlePath == "" {
				t.Fatalf("direct conversion lost perftrace or bundle: %+v", result)
			}
			body, err := os.ReadFile(perfTrace.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), test.want) || strings.Contains(string(body), test.reject) {
				t.Fatalf("direct reader followed DisplayPath instead of authority: want=%q reject=%q\n%s", test.want, test.reject, body)
			}
			if got := view.counts[conversionInputStageDirectPerfRead]; got != 3 || view.reads == 0 {
				t.Fatalf("direct read gates=%d reads=%d want=3/>0", got, view.reads)
			}
			for _, end := range view.readEnds {
				if end > view.Size() {
					t.Fatalf("direct read exceeded fixed input boundary: end=%d size=%d", end, view.Size())
				}
			}
			if err := ledger.validateOwnedPaths(); err != nil {
				t.Fatalf("direct publications failed ownership validation: %v", err)
			}
		})
	}
}

func TestReleaseDirectPerfStableLinksPreserveTypedOutput(t *testing.T) {
	tests := []struct {
		name   string
		body   func() []byte
		opts   Options
		format perfInputFormat
	}{
		{name: "raw", body: syntheticRawPerfData, opts: Options{PerfParser: "raw"}, format: perfInputLinuxPerfData},
		{name: "simpleperf-proto", body: func() []byte { return syntheticSimpleperfProtoStream(false, false) }, format: perfInputSimpleperfReportProto},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.bin")
			if err := os.WriteFile(source, test.body(), 0o600); err != nil {
				t.Fatal(err)
			}
			aliases := []string{source}
			symlink := filepath.Join(dir, "stable-symlink.bin")
			if err := os.Symlink(source, symlink); err != nil {
				t.Logf("stable symlink fixture unavailable: %v", err)
			} else {
				aliases = append(aliases, symlink)
			}
			hardlink := filepath.Join(dir, "stable-hardlink.bin")
			if err := os.Link(source, hardlink); err != nil {
				t.Logf("stable hardlink fixture unavailable: %v", err)
			} else {
				aliases = append(aliases, hardlink)
			}
			if len(aliases) == 1 {
				t.Skip("filesystem supports neither stable symlink nor stable hardlink fixtures")
			}

			var baselineBody []byte
			var baselineDecision PerfProviderDecision
			var baselineCapability *PerfArtifactCapability
			for index, input := range aliases {
				output := filepath.Join(dir, "out-"+string(rune('0'+index))+".systrace")
				result, err := ConvertFile(context.Background(), Options{
					InputPath:  input,
					OutputPath: output,
					PerfParser: test.opts.PerfParser,
				})
				if err != nil {
					t.Fatalf("convert stable alias %q: %v", input, err)
				}
				if result.InputBytes != int64(len(test.body())) || len(result.ProviderDecisions) != 1 {
					t.Fatalf("stable alias %q lost typed result: %+v", input, result)
				}
				artifact := directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace)
				if artifact.Path == "" || artifact.Perf == nil || artifact.Perf.InputFormat != string(test.format) {
					t.Fatalf("stable alias %q lost perftrace capability: %+v", input, artifact)
				}
				body, err := os.ReadFile(artifact.Path)
				if err != nil {
					t.Fatal(err)
				}
				decision := result.ProviderDecisions[0]
				decision.InputPath = ""
				decision.OutputPath = ""
				decision.ArtifactPath = ""
				if index == 0 {
					baselineBody = body
					baselineDecision = decision
					baselineCapability = artifact.Perf
					continue
				}
				if !bytes.Equal(body, baselineBody) || !reflect.DeepEqual(decision, baselineDecision) || !reflect.DeepEqual(artifact.Perf, baselineCapability) {
					t.Fatalf("stable alias %q changed direct typed output\nbody_equal=%t\ndecision=%+v\nwant=%+v\ncapability=%+v\nwant=%+v", input, bytes.Equal(body, baselineBody), decision, baselineDecision, artifact.Perf, baselineCapability)
				}
			}
		})
	}
}

func TestReleaseDirectPerfRawFallbackBranchesUseAuthority(t *testing.T) {
	for _, branch := range []string{"unavailable", "failed", "unreadable-output", "malformed-output"} {
		t.Run(branch, func(t *testing.T) {
			if runtime.GOOS == "windows" && branch != "unavailable" {
				t.Skip("scripted external-tool failure fixture requires a POSIX shell")
			}
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			t.Setenv("CODRAX_SIMPLEPERF_REPORT_SAMPLE", "")
			t.Setenv("CODRAX_SIMPLEPERF_PYTHON", "")
			opts := Options{}
			switch branch {
			case "failed":
				tool := filepath.Join(dir, "report_sample")
				if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 7\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				opts.SimpleperfReportPath = tool
			case "unreadable-output":
				tool := filepath.Join(dir, "report_sample")
				if _, err := os.Stat("/bin/mkdir"); err != nil {
					t.Skipf("absolute mkdir fixture unavailable: %v", err)
				}
				script := "#!/bin/sh\nout=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '-o' ]; then shift; out=\"$1\"; fi\n  shift\ndone\n/bin/mkdir \"$out\"\n"
				if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				opts.SimpleperfReportPath = tool
			case "malformed-output":
				tool := filepath.Join(dir, "report_sample")
				script := "#!/bin/sh\nout=''\nwhile [ \"$#\" -gt 0 ]; do\n  if [ \"$1\" = '-o' ]; then shift; out=\"$1\"; fi\n  shift\ndone\n: > \"$out\"\n"
				if err := os.WriteFile(tool, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				opts.SimpleperfReportPath = tool
			}
			authorityBody := syntheticRawPerfData()
			pathBody := bytes.Replace(append([]byte(nil), authorityBody...), []byte("app\x00"), []byte("bad\x00"), 1)
			path := filepath.Join(dir, "input.data")
			if err := os.WriteFile(path, pathBody, 0o600); err != nil {
				t.Fatal(err)
			}
			view := newScriptedStandaloneInputView(path, authorityBody)
			binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if cleanupErr := ledger.cleanup(); cleanupErr != nil {
					t.Errorf("cleanup: %v", cleanupErr)
				}
			})
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				context.Background(), opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, filepath.Join(dir, "unused.systrace"), ledger,
			)
			if err != nil || !ok {
				t.Fatalf("fallback branch=%s ok=%t err=%v", branch, ok, err)
			}
			artifact := directPerfArtifactByType(result.Artifacts, ArtifactPerfTrace)
			body, err := os.ReadFile(artifact.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `thread_comm="app"`) || strings.Contains(string(body), `thread_comm="bad"`) {
				t.Fatalf("fallback branch %s reopened DisplayPath:\n%s", branch, body)
			}
			wantGates := 3
			if branch != "unavailable" {
				// Official text profile inspection validates the same immutable
				// input generation before and after its bounded META_INFO read.
				wantGates += 2
			}
			if got := view.counts[conversionInputStageDirectPerfRead]; got != wantGates {
				t.Fatalf("fallback branch %s direct read gates=%d want=%d", branch, got, wantGates)
			}
			wantReason := map[string]string{
				"unavailable":       "official_tool_unavailable",
				"failed":            "official_adapter_failed",
				"unreadable-output": "official_output_unreadable",
				"malformed-output":  "official_output_unreadable",
			}[branch]
			if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != wantReason {
				t.Fatalf("fallback branch %s decisions=%+v want exactly two decisions and first reason=%s", branch, result.ProviderDecisions, wantReason)
			}
			rawDecision := result.ProviderDecisions[1]
			if rawDecision.ProviderName != perfProviderNameRawFallback ||
				!rawDecision.Selected || !rawDecision.Attempted || !rawDecision.Succeeded ||
				!rawDecision.Fallback || !rawDecision.TraceQueryReady {
				t.Fatalf("fallback branch %s lost raw fallback provenance: %+v", branch, rawDecision)
			}
		})
	}
}

func TestReleaseDirectPerfGenerationGatesRollback(t *testing.T) {
	tests := []struct {
		name   string
		format perfInputFormat
		opts   Options
		body   func() []byte
	}{
		{name: "raw", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: syntheticRawPerfData},
		{name: "simpleperf-proto", format: perfInputSimpleperfReportProto, body: func() []byte { return syntheticSimpleperfProtoStream(false, false) }},
	}
	for _, test := range tests {
		for _, failCall := range []int{1, 2, 3} {
			t.Run(test.name+"/gate-"+string(rune('0'+failCall)), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "input.bin")
				body := test.body()
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				view := newScriptedStandaloneInputView(path, body)
				view.failStage = conversionInputStageDirectPerfRead
				view.failCall = failCall
				binding, err := newDirectPerfInputBinding(view, test.format)
				if err != nil {
					t.Fatal(err)
				}
				ledger, err := newConversionFileLedger(path)
				if err != nil {
					t.Fatal(err)
				}
				output := filepath.Join(dir, "unused.systrace")
				result, ok, err := maybeConvertDirectSimpleperfPerfData(
					context.Background(), test.opts,
					traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
					binding, output, ledger,
				)
				assertDirectPerfGenerationError(t, err)
				if !ok || !reflect.DeepEqual(result, Result{}) {
					t.Fatalf("generation failure leaked result: ok=%t result=%+v", ok, result)
				}
				if failCall == 1 && view.reads != 0 {
					t.Fatalf("entry gate read source %d time(s)", view.reads)
				}
				if failCall > 1 && view.reads == 0 {
					t.Fatal("post-read gate failed before exercising the reader")
				}
				if cleanupErr := ledger.cleanup(); cleanupErr != nil {
					t.Fatalf("cleanup: %v", cleanupErr)
				}
				base := traceSidecarBase(path, output)
				for _, published := range []string{base + ".perftrace", base + ".tracebundle.json"} {
					if _, statErr := os.Lstat(published); !os.IsNotExist(statErr) {
						t.Fatalf("generation failure retained publication %s: %v", published, statErr)
					}
				}
			})
		}
	}
}

func TestReleaseDirectPerfDecodeFailureCannotHideGenerationChange(t *testing.T) {
	tests := []struct {
		name   string
		format perfInputFormat
		opts   Options
		body   []byte
	}{
		{name: "raw", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: []byte(perfMagic2)},
		{name: "simpleperf-proto", format: perfInputSimpleperfReportProto, body: []byte(simpleperfProtoMagic)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "malformed.bin")
			if err := os.WriteFile(path, test.body, 0o600); err != nil {
				t.Fatal(err)
			}
			view := newScriptedStandaloneInputView(path, test.body)
			view.failStage = conversionInputStageDirectPerfRead
			view.failCall = 2
			binding, err := newDirectPerfInputBinding(view, test.format)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				context.Background(), test.opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, filepath.Join(dir, "unused.systrace"), ledger,
			)
			assertDirectPerfGenerationError(t, err)
			if !ok || !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("decode+generation failure leaked partial success: ok=%t result=%+v err=%v", ok, result, err)
			}
			if cleanupErr := ledger.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
		})
	}
}

func TestReleaseDirectPerfPhysicalGenerationChangesFailAtReadStage(t *testing.T) {
	tests := []struct {
		name   string
		format perfInputFormat
		opts   Options
		body   func() []byte
		mutate func(*testing.T, string, []byte)
	}{
		{
			name: "raw-same-size-restored-mtime", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: syntheticRawPerfData,
			mutate: func(t *testing.T, path string, body []byte) {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				changed := bytes.Replace(append([]byte(nil), body...), []byte("app\x00"), []byte("bad\x00"), 1)
				if bytes.Equal(changed, body) || len(changed) != len(body) {
					t.Fatal("same-size mutation fixture did not change")
				}
				if err := os.WriteFile(path, changed, info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "raw-grow", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: syntheticRawPerfData,
			mutate: func(t *testing.T, path string, body []byte) {
				if err := os.WriteFile(path, append(append([]byte(nil), body...), 0), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "raw-truncate", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: syntheticRawPerfData,
			mutate: func(t *testing.T, path string, body []byte) {
				if err := os.Truncate(path, int64(len(body)-1)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "simpleperf-proto-atomic-replace", format: perfInputSimpleperfReportProto, body: func() []byte { return syntheticSimpleperfProtoStream(false, false) },
			mutate: func(t *testing.T, path string, body []byte) {
				replacement := filepath.Join(filepath.Dir(path), "replacement.bin")
				if err := os.WriteFile(replacement, body, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && strings.Contains(test.name, "atomic-replace") {
				t.Skip("Windows denies replacing the source while the authority handle is open; the platform sharing rule is itself fail-closed")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "input.bin")
			body := test.body()
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(path)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			binding, err := newDirectPerfInputBinding(authority, test.format)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedgerForAuthority(authority)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, path, body)
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				context.Background(), test.opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, filepath.Join(dir, "unused.systrace"), ledger,
			)
			assertDirectPerfGenerationError(t, err)
			if !ok || !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("physical generation change leaked result: ok=%t result=%+v", ok, result)
			}
			if cleanupErr := ledger.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
		})
	}
}

func TestReleaseDirectPerfRealAuthorityMidAndExitMutationRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows denies rewriting a file while the immutable authority handle is open; scripted gate tests cover the cross-platform contract")
	}
	for _, point := range []string{"after-entry-before-read", "after-output-before-exit"} {
		t.Run(point, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input.data")
			body := syntheticRawPerfData()
			changed := bytes.Replace(append([]byte(nil), body...), []byte("app\x00"), []byte("bad\x00"), 1)
			if bytes.Equal(body, changed) || len(body) != len(changed) {
				t.Fatal("mid-stage mutation fixture did not change at fixed size")
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			originalInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(path)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedgerForAuthority(authority)
			if err != nil {
				t.Fatal(err)
			}
			mutated := false
			var mutationErr error
			opts := Options{PerfParser: "raw"}
			opts.Progress = func(event ProgressEvent) {
				trigger := point == "after-entry-before-read" && event.Stage == "raw_perf_parse" && event.Status == ProgressStatusStarted
				trigger = trigger || point == "after-output-before-exit" && event.Stage == "perftrace_write" && event.Status == ProgressStatusComplete
				if !trigger || mutated || mutationErr != nil {
					return
				}
				mutated = true
				mutationErr = os.WriteFile(path, changed, originalInfo.Mode().Perm())
				if mutationErr == nil {
					mutationErr = os.Chtimes(path, originalInfo.ModTime(), originalInfo.ModTime())
				}
			}
			output := filepath.Join(dir, "unused.systrace")
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				context.Background(), opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, output, ledger,
			)
			if mutationErr != nil {
				t.Fatalf("mutate at %s: %v", point, mutationErr)
			}
			if !mutated {
				t.Fatalf("progress trigger %s was not reached", point)
			}
			assertDirectPerfGenerationError(t, err)
			if !ok || !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("mid-stage generation change leaked result: ok=%t result=%+v", ok, result)
			}
			if cleanupErr := ledger.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
			base := traceSidecarBase(path, output)
			for _, published := range []string{base + ".perftrace", base + ".tracebundle.json"} {
				if _, statErr := os.Lstat(published); !os.IsNotExist(statErr) {
					t.Fatalf("mid-stage generation change retained %s: %v", published, statErr)
				}
			}
		})
	}
}

func TestReleaseDirectPerfSymlinkRetargetFailsAtReadStage(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.data")
	second := filepath.Join(dir, "second.data")
	link := filepath.Join(dir, "input.data")
	body := syntheticRawPerfData()
	if err := os.WriteFile(first, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	authority, err := openConversionInputAuthority(link)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedgerForAuthority(authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	result, ok, err := maybeConvertDirectSimpleperfPerfData(
		context.Background(), Options{PerfParser: "raw"},
		traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
		binding, filepath.Join(dir, "unused.systrace"), ledger,
	)
	assertDirectPerfGenerationError(t, err)
	if !ok || !reflect.DeepEqual(result, Result{}) {
		t.Fatalf("symlink retarget leaked result: ok=%t result=%+v", ok, result)
	}
	if cleanupErr := ledger.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
}

func TestReleaseDirectPerfCancellationIsFatal(t *testing.T) {
	tests := []struct {
		name   string
		format perfInputFormat
		opts   Options
		body   func() []byte
	}{
		{name: "raw", format: perfInputLinuxPerfData, opts: Options{PerfParser: "raw"}, body: syntheticRawPerfData},
		{name: "simpleperf-proto", format: perfInputSimpleperfReportProto, body: func() []byte { return syntheticSimpleperfProtoStream(false, false) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input.bin")
			body := test.body()
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			base := newScriptedStandaloneInputView(path, body)
			view := &cancelingDirectPerfInputView{scriptedStandaloneInputView: base, cancel: cancel}
			binding, err := newDirectPerfInputBinding(view, test.format)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(path)
			if err != nil {
				t.Fatal(err)
			}
			result, ok, err := maybeConvertDirectSimpleperfPerfData(
				ctx, test.opts,
				traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
				binding, filepath.Join(dir, "unused.systrace"), ledger,
			)
			if !errors.Is(err, context.Canceled) || !ok || !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("cancellation was not fatal: ok=%t result=%+v err=%T %v", ok, result, err, err)
			}
			if view.reads != 1 {
				t.Fatalf("canceled direct reader continued after the first source read: reads=%d", view.reads)
			}
			if cleanupErr := ledger.cleanup(); cleanupErr != nil {
				t.Fatal(cleanupErr)
			}
		})
	}
}

func TestReleaseDirectPerfExpiredDeadlineStopsBeforeRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.data")
	body := syntheticRawPerfData()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	view := newScriptedStandaloneInputView(path, body)
	binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result, ok, err := maybeConvertDirectSimpleperfPerfData(
		ctx, Options{PerfParser: "raw"},
		traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
		binding, filepath.Join(dir, "unused.systrace"), ledger,
	)
	if !errors.Is(err, context.DeadlineExceeded) || !ok || !reflect.DeepEqual(result, Result{}) || view.reads != 0 {
		t.Fatalf("expired deadline did not stop before read: ok=%t reads=%d result=%+v err=%T %v", ok, view.reads, result, err, err)
	}
	if cleanupErr := ledger.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
}

func TestReleaseDirectPerfReaderAtParityAndForgedBoundary(t *testing.T) {
	rawBody := syntheticRawPerfData()
	path := filepath.Join(t.TempDir(), "raw.perf.data")
	if err := os.WriteFile(path, rawBody, 0o600); err != nil {
		t.Fatal(err)
	}
	fromPath, err := readRawPerfData(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	fromReader, err := readRawPerfDataAt(context.Background(), bytes.NewReader(rawBody), int64(len(rawBody)), path, nil)
	if err != nil || !reflect.DeepEqual(fromPath, fromReader) {
		t.Fatalf("raw ReaderAt parity err=%v\npath=%+v\nreader=%+v", err, fromPath, fromReader)
	}
	oversizedAttrs := append([]byte(nil), rawBody...)
	binary.LittleEndian.PutUint64(oversizedAttrs[32:40], ^uint64(0))
	if _, err := readRawPerfDataAt(context.Background(), bytes.NewReader(oversizedAttrs), int64(len(oversizedAttrs)), path, nil); err == nil || !strings.Contains(err.Error(), "perf attrs range") {
		t.Fatalf("oversized raw attr section was not rejected before allocation: %v", err)
	}
	oversizedData := append([]byte(nil), rawBody...)
	binary.LittleEndian.PutUint64(oversizedData[40:48], uint64(len(oversizedData)-1))
	binary.LittleEndian.PutUint64(oversizedData[48:56], 2)
	if _, err := readRawPerfDataAt(context.Background(), bytes.NewReader(oversizedData), int64(len(oversizedData)), path, nil); err == nil || !strings.Contains(err.Error(), "record range") {
		t.Fatalf("oversized raw data section was not rejected before reading records: %v", err)
	}
	oversizedHeader := append([]byte(nil), rawBody...)
	binary.LittleEndian.PutUint64(oversizedHeader[8:16], uint64(len(oversizedHeader)+1))
	if _, err := readRawPerfHeader(bytes.NewReader(oversizedHeader), int64(len(oversizedHeader))); err == nil || !strings.Contains(err.Error(), "header size") {
		t.Fatalf("oversized declared raw header was not rejected: %v", err)
	}
	shortHeader := append([]byte(nil), rawBody...)
	binary.LittleEndian.PutUint64(shortHeader[8:16], 80)
	for index := 72; index < 104; index++ {
		shortHeader[index] = byte(index)
	}
	header, err := readRawPerfHeader(bytes.NewReader(shortHeader), int64(len(shortHeader)))
	if err != nil {
		t.Fatalf("read compatible short declared header: %v", err)
	}
	if !bytes.Equal(header.Features[:8], shortHeader[72:80]) || !bytes.Equal(header.Features[8:], make([]byte, 24)) {
		t.Fatalf("short declared header consumed bytes outside its feature boundary: %x", header.Features)
	}

	protoBody := syntheticSimpleperfProtoStream(false, false)
	protoPath := filepath.Join(t.TempDir(), "simpleperf.pb")
	if err := os.WriteFile(protoPath, protoBody, 0o600); err != nil {
		t.Fatal(err)
	}
	protoFromPath, err := readSimpleperfProtoFile(context.Background(), protoPath)
	if err != nil {
		t.Fatal(err)
	}
	protoFromReader, err := readSimpleperfProtoAt(context.Background(), bytes.NewReader(protoBody), int64(len(protoBody)))
	if err != nil || !reflect.DeepEqual(protoFromPath, protoFromReader) {
		t.Fatalf("proto ReaderAt parity err=%v\npath=%+v\nreader=%+v", err, protoFromPath, protoFromReader)
	}
	oversized := append([]byte(simpleperfProtoMagic), byte(simpleperfProtoVersion), byte(simpleperfProtoVersion>>8))
	var declared [4]byte
	binary.LittleEndian.PutUint32(declared[:], ^uint32(0))
	oversized = append(oversized, declared[:]...)
	if _, err := readSimpleperfProtoAt(context.Background(), bytes.NewReader(oversized), int64(len(oversized))); err == nil || !strings.Contains(err.Error(), "exceeds fixed input remainder") {
		t.Fatalf("oversized proto record was not rejected before allocation: %v", err)
	}

	view := newScriptedStandaloneInputView(path, rawBody)
	binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
	if err != nil {
		t.Fatal(err)
	}
	binding.inputSize++
	ledger, err := newConversionFileLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	result, ok, err := maybeConvertDirectSimpleperfPerfData(
		context.Background(), Options{PerfParser: "raw"},
		traceProviderPlan{DirectPerf: true, PreflightEngine: traceEngineDirectPerf},
		binding, filepath.Join(t.TempDir(), "unused.systrace"), ledger,
	)
	var inputErr *ConversionInputError
	if !errors.As(err, &inputErr) || inputErr.Code != ConversionInputCodeInternalContract || inputErr.Stage != conversionInputStageDirectPerfRead.String() || !ok || !reflect.DeepEqual(result, Result{}) || view.reads != 0 {
		t.Fatalf("forged binding did not fail closed: ok=%t result=%+v reads=%d err=%T %v", ok, result, view.reads, err, err)
	}
}

func TestReleaseDirectPerfProductionCallGraphIsAuthorityOnly(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve direct perf release-contract path")
	}
	directBindingSource, err := os.ReadFile(filepath.Join(filepath.Dir(current), "direct_perf_input.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(directBindingSource), "type directPerfInputBinding =") || !strings.Contains(string(directBindingSource), "type directPerfInputBinding struct") {
		t.Fatalf("direct perf binding regained a type alias instead of a distinct route wrapper:\n%s", directBindingSource)
	}
	convertBody := sourceGenerationFunctionBody(t, "convert.go", "ConvertFile")
	for _, required := range []string{
		"newDirectPerfInputBinding(authority, inputFormat)",
		"maybeConvertDirectSimpleperfPerfData(ctx, opts, directPlan, directInput, output, ledger)",
	} {
		if strings.Count(convertBody, required) != 1 {
			t.Fatalf("ConvertFile lost direct authority handoff %q:\n%s", required, convertBody)
		}
	}
	directBody := sourceGenerationFunctionBody(t, "simpleperf_text.go", "maybeConvertSimpleperfPerfData")
	for _, required := range []string{
		"maybeConvertSimpleperfProtoFromInputWithDecision(",
		"maybeConvertRawPerfDataFromInputWithDecision(",
		"maybeRawPerfFallbackForSimpleperf(ctx, opts, input,",
	} {
		if !strings.Contains(directBody, required) {
			t.Fatalf("direct provider lost authority-bound branch %q:\n%s", required, directBody)
		}
	}
	for _, forbidden := range []string{
		"convertSimpleperfProtoFileToPerfTraceWithLedger(",
		"convertRawPerfDataFileToPerfTraceWithLedger(",
		"readSimpleperfProtoFile(",
		"readRawPerfData(",
	} {
		if strings.Contains(directBody, forbidden) {
			t.Fatalf("direct provider regained path reader %q:\n%s", forbidden, directBody)
		}
	}
	for _, arm := range []struct {
		file      string
		name      string
		required  string
		forbidden string
	}{
		{file: "simpleperf_proto.go", name: "maybeConvertSimpleperfProtoFromInputWithDecision", required: "convertSimpleperfProtoInputToPerfTraceWithLedger(", forbidden: "convertSimpleperfProtoFileToPerfTraceWithLedger("},
		{file: "simpleperf_text.go", name: "maybeRawPerfFallbackForSimpleperf", required: "maybeRawPerfFallbackFromInput(", forbidden: "maybeRawPerfFallback(ctx"},
		{file: "raw_perfdata.go", name: "maybeRawPerfFallbackFromInput", required: "maybeConvertRawPerfDataFromInputWithDecision(", forbidden: "maybeConvertRawPerfDataWithDecision("},
		{file: "raw_perfdata.go", name: "maybeConvertRawPerfDataFromInputWithDecision", required: "maybeConvertRawPerfDataFromInput(", forbidden: "maybeConvertRawPerfData(ctx"},
		{file: "raw_perfdata.go", name: "maybeConvertRawPerfDataFromInput", required: "convertRawPerfDataInputToPerfTraceWithLedgerPolicy(", forbidden: "convertRawPerfDataFileToPerfTraceWithLedger("},
	} {
		body := sourceGenerationFunctionBody(t, arm.file, arm.name)
		if !strings.Contains(body, arm.required) || strings.Contains(body, arm.forbidden) {
			t.Fatalf("direct authority arm %s drifted: require=%q forbid=%q\n%s", arm.name, arm.required, arm.forbidden, body)
		}
	}
	for _, item := range []struct {
		file string
		name string
	}{
		{file: "simpleperf_proto.go", name: "convertSimpleperfProtoInputToPerfTraceWithLedger"},
	} {
		body := sourceGenerationFunctionBody(t, item.file, item.name)
		if strings.Count(body, "conversionInputStageDirectPerfRead") != 3 {
			t.Fatalf("%s direct gate count drifted:\n%s", item.name, body)
		}
		for _, forbidden := range []string{"os.Open(", "os.Stat(", "os.ReadFile(", "filepath.EvalSymlinks("} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s regained path operation %q:\n%s", item.name, forbidden, body)
			}
		}
	}
	rawWrapper := sourceGenerationFunctionBody(t, "raw_perfdata.go", "convertRawPerfDataInputToPerfTraceWithLedgerPolicy")
	if !strings.Contains(rawWrapper, "input.validate()") || !strings.Contains(rawWrapper, "convertRawPerfDataBoundInputToPerfTraceWithLedgerPolicy(") {
		t.Fatalf("direct raw wrapper lost typed validation/core handoff:\n%s", rawWrapper)
	}
	rawCore := sourceGenerationFunctionBody(t, "raw_perfdata.go", "convertRawPerfDataBoundInputToPerfTraceWithLedgerPolicy")
	if strings.Count(rawCore, "input.stage") < 3 || strings.Contains(rawCore, "conversionInputStageDirectPerfRead") {
		t.Fatalf("shared raw core lost binding-owned stage gates:\n%s", rawCore)
	}
	for _, forbidden := range []string{"os.Open(", "os.Stat(", "os.ReadFile(", "filepath.EvalSymlinks("} {
		if strings.Contains(rawWrapper, forbidden) || strings.Contains(rawCore, forbidden) {
			t.Fatalf("raw binding reader regained path operation %q:\nwrapper=%s\ncore=%s", forbidden, rawWrapper, rawCore)
		}
	}
	for _, item := range []struct {
		file string
		name string
	}{
		{file: "simpleperf_proto.go", name: "readSimpleperfProtoAt"},
		{file: "raw_perfdata.go", name: "readRawPerfDataAt"},
	} {
		body := sourceGenerationFunctionBody(t, item.file, item.name)
		if !strings.Contains(body, "io.NewSectionReader(") {
			t.Fatalf("%s lost fixed ReaderAt boundary:\n%s", item.name, body)
		}
		for _, forbidden := range []string{"os.Open(", "os.Stat(", "os.ReadFile(", "filepath.EvalSymlinks("} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s regained path operation %q:\n%s", item.name, forbidden, body)
			}
		}
	}
	for _, item := range []struct {
		file string
		name string
	}{
		{file: "simpleperf_proto.go", name: "ConvertSimpleperfProtoFileToPerfTrace"},
		{file: "raw_perfdata.go", name: "ConvertRawPerfDataFileToPerfTrace"},
	} {
		body := sourceGenerationFunctionBody(t, item.file, item.name)
		if strings.Count(body, "runConversionInputTransaction(") != 1 || strings.Contains(body, "runConversionFileTransaction(") {
			t.Fatalf("%s is not a one-authority compatibility transaction:\n%s", item.name, body)
		}
	}
	transactionBody := sourceGenerationFunctionBody(t, "transaction.go", "runConversionInputTransaction")
	if strings.Count(transactionBody, "openConversionInputAuthority(") != 1 || !strings.Contains(transactionBody, "newConversionFileLedgerForAuthority(authority)") {
		t.Fatalf("direct compatibility transaction lost its single authority:\n%s", transactionBody)
	}
	commitAt := strings.Index(transactionBody, "authority.Validate(conversionInputStagePreCommit)")
	if commitAt < 0 {
		t.Fatalf("direct compatibility transaction lost pre-commit validation:\n%s", transactionBody)
	}
	assertSourceGenerationOrder(t, transactionBody[commitAt:],
		"authority.Validate(conversionInputStagePreCommit)",
		"authority.Close()",
		"ledger.validateOwnedPaths()",
		"ledger.releaseOwnedAuthorities()",
		"committed = true",
	)
}

func TestReleaseStandalonePerfBindingCannotEnterDirectProvider(t *testing.T) {
	body := syntheticRawPerfData()
	view := newScriptedStandaloneInputView(filepath.Join(t.TempDir(), "standalone.perf.data"), body)
	standalone, err := newPerfInputBinding(
		view,
		perfInputLinuxPerfData,
		conversionInputStageStandaloneExtract,
		perfInputBindingStandaloneHiperf,
	)
	if err != nil {
		t.Fatal(err)
	}
	forged := directPerfInputBinding{perfInputBinding: standalone}
	err = forged.validate()
	var inputErr *ConversionInputError
	if !errors.As(err, &inputErr) || inputErr.Code != ConversionInputCodeInternalContract || inputErr.Stage != conversionInputStageDirectPerfRead.String() {
		t.Fatalf("standalone binding crossed into the direct route: %T %v", err, err)
	}
}

func directPerfArtifactByType(artifacts []Artifact, kind string) Artifact {
	for _, artifact := range artifacts {
		if artifact.Type == kind {
			return artifact
		}
	}
	return Artifact{}
}

func assertDirectPerfGenerationError(t *testing.T, err error) {
	t.Helper()
	var inputErr *ConversionInputError
	if !errors.As(err, &inputErr) || inputErr.Code != ConversionInputCodeGenerationChanged || inputErr.Stage != conversionInputStageDirectPerfRead.String() {
		t.Fatalf("error=%T %v want source_generation_changed@direct_perf_read", err, err)
	}
}

type cancelingDirectPerfInputView struct {
	*scriptedStandaloneInputView
	cancel context.CancelFunc
	reads  int
}

func (input *cancelingDirectPerfInputView) ReadAt(buffer []byte, offset int64) (int, error) {
	n, err := input.scriptedStandaloneInputView.ReadAt(buffer, offset)
	input.reads++
	if input.reads == 1 {
		input.cancel()
	}
	return n, err
}

var _ conversionInputView = (*cancelingDirectPerfInputView)(nil)
