package hitraceconv

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// embeddedTraceStreamerPayloadRatchetBytes pins the total on-disk bytes
// of internal/hitraceconv/embedded_trace_streamer (all non-dot files:
// per-platform binaries + manifests). Baseline set 2026-07-05 from the
// actually committed first wave (HED-59 ruling):
//
//	windows-amd64/trace_streamer.exe 27,586,048
//	linux-amd64/trace_streamer       12,703,088
//	windows-amd64/manifest.json             971
//	linux-amd64/manifest.json             1,095
//
// Any change — new platform payloads, binary upgrades, manifest edits —
// MUST go through an explicit ruling and update this baseline in the
// same commit. This is the anti-drift ratchet: nobody quietly grows the
// release payload without the number changing here.
const embeddedTraceStreamerPayloadRatchetBytes int64 = 40291202

func TestEmbeddedTraceStreamerDirectoryRequiresManifest(t *testing.T) {
	info, err := os.Stat(embeddedTraceStreamerDir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("stat embedded trace_streamer directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s must be a directory", embeddedTraceStreamerDir)
	}
	entries, err := os.ReadDir(embeddedTraceStreamerDir)
	if err != nil {
		t.Fatalf("read embedded trace_streamer directory: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !entry.IsDir() {
			t.Fatalf("stray file %s at %s root; only per-platform payload directories are allowed", name, embeddedTraceStreamerDir)
		}
		if !embeddedTraceStreamerBundledPlatformDirs[name] {
			t.Fatalf("platform directory %s is not in the approved embedded platform set %v; adding a platform requires an explicit ruling", name, embeddedTraceStreamerBundledPlatformDirs)
		}
		platformDir := filepath.Join(embeddedTraceStreamerDir, name)
		if err := validateEmbeddedTraceStreamerManifest(platformDir); err != nil {
			t.Fatalf("embedded trace_streamer manifest validation failed for %s: %v", name, err)
		}
		manifest, err := loadEmbeddedTraceStreamerManifestFS(os.DirFS(platformDir))
		if err != nil {
			t.Fatalf("load manifest for %s: %v", name, err)
		}
		if len(manifest.Platforms) != 1 {
			t.Fatalf("per-platform manifest %s must declare exactly one platform, got %d", name, len(manifest.Platforms))
		}
		platform := manifest.Platforms[0]
		if got := strings.TrimSpace(platform.GOOS) + "-" + strings.TrimSpace(platform.GOARCH); got != name {
			t.Fatalf("platform directory %s declares mismatched goos/goarch %s", name, got)
		}
		// Exactly manifest.json plus the declared binary — nothing else.
		files, err := os.ReadDir(platformDir)
		if err != nil {
			t.Fatalf("read platform directory %s: %v", name, err)
		}
		allowed := map[string]bool{
			embeddedTraceStreamerManifestName: true,
			path.Base(platform.Path):          true,
		}
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".") {
				continue
			}
			if !allowed[file.Name()] {
				t.Fatalf("unexpected file %s in %s; per-platform payload dirs carry exactly manifest.json plus the declared binary", file.Name(), platformDir)
			}
		}
	}
}

func TestEmbeddedTraceStreamerPayloadRatchet(t *testing.T) {
	seenDirs := map[string]bool{}
	var total int64
	err := filepath.WalkDir(embeddedTraceStreamerDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != embeddedTraceStreamerDir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != embeddedTraceStreamerDir {
				rel, relErr := filepath.Rel(embeddedTraceStreamerDir, p)
				if relErr == nil && !strings.Contains(rel, string(filepath.Separator)) {
					seenDirs[rel] = true
				}
			}
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v (the first-wave payload is committed; its absence is a regression)", embeddedTraceStreamerDir, err)
	}
	if len(seenDirs) != len(embeddedTraceStreamerBundledPlatformDirs) {
		t.Fatalf("embedded platform dirs = %v, want exactly the approved set %v", seenDirs, embeddedTraceStreamerBundledPlatformDirs)
	}
	for dir := range embeddedTraceStreamerBundledPlatformDirs {
		if !seenDirs[dir] {
			t.Fatalf("approved embedded platform dir %s is missing (present: %v)", dir, seenDirs)
		}
	}
	if total != embeddedTraceStreamerPayloadRatchetBytes {
		t.Fatalf("embedded trace_streamer payload ratchet: total bytes = %d, pinned baseline = %d; payload changes require an explicit ruling plus a same-commit baseline update", total, embeddedTraceStreamerPayloadRatchetBytes)
	}
}

func TestEmbeddedTraceStreamerManifestValidation(t *testing.T) {
	root := t.TempDir()
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("trace-streamer-binary")
	writeEmbeddedTraceStreamerBinary(t, root, binaryRel, binaryBody, 0o755)
	base := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	writeEmbeddedTraceStreamerManifest(t, root, base)
	if err := validateEmbeddedTraceStreamerManifest(root); err != nil {
		t.Fatalf("valid embedded manifest rejected: %v", err)
	}
	tests := []struct {
		name  string
		setup func(string, embeddedTraceStreamerManifest)
		want  string
	}{
		{
			name: "missing manifest",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
			},
			want: "manifest.json",
		},
		{
			name: "missing source",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.SourceURL = ""
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "source_url",
		},
		{
			name: "missing version",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Version = ""
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "version",
		},
		{
			name: "missing approval",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.ApprovalRef = ""
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "approval_ref",
		},
		{
			name: "missing size_bytes",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].SizeBytes = 0
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "size_bytes",
		},
		{
			name: "missing actual_format",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].ActualFormat = ""
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "actual_format",
		},
		{
			name: "declared size mismatch",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].SizeBytes = manifest.Platforms[0].SizeBytes + 1
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "size mismatch",
		},
		{
			name: "absolute binary path",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].Path = filepath.Join(dir, binaryRel)
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "must be relative",
		},
		{
			name: "escaping binary path",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].Path = "../trace_streamer"
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "must stay under",
		},
		{
			name: "hash mismatch",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "sha256 mismatch",
		},
		{
			name: "missing binary",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "not readable",
		},
		{
			name: "not executable",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o644)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "not executable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "not executable" && runtime.GOOS == "windows" {
				t.Skip("exec-bit enforcement needs a host filesystem with exec bits")
			}
			dir := t.TempDir()
			tt.setup(dir, cloneEmbeddedTraceStreamerManifest(base))
			err := validateEmbeddedTraceStreamerManifest(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestEmbeddedTraceStreamerRuntimeExtraction(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	cacheRoot := t.TempDir()

	first, err := extractEmbeddedTraceStreamer(fsys, cacheRoot)
	if err != nil {
		t.Fatalf("extract embedded trace_streamer: %v", err)
	}
	if first.Path == "" || !strings.Contains(first.Source, "embedded trace_streamer") || first.CacheReused {
		t.Fatalf("unexpected first extraction result: %+v", first)
	}
	if err := verifyEmbeddedTraceStreamerFileHash(first.Path, embeddedTraceStreamerSHA256(binaryBody)); err != nil {
		t.Fatalf("cached binary hash mismatch: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(first.Path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("cached embedded binary is not executable: mode=%v", info.Mode())
		}
	}

	second, err := extractEmbeddedTraceStreamer(fsys, cacheRoot)
	if err != nil {
		t.Fatalf("reuse embedded trace_streamer: %v", err)
	}
	if second.Path != first.Path || !second.CacheReused {
		t.Fatalf("expected verified cache reuse, first=%+v second=%+v", first, second)
	}
}

func TestEmbeddedTraceStreamerRuntimeExtractionReplacesCorruptedCache(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	cacheRoot := t.TempDir()

	first, err := extractEmbeddedTraceStreamer(fsys, cacheRoot)
	if err != nil {
		t.Fatalf("extract embedded trace_streamer: %v", err)
	}
	if err := os.WriteFile(first.Path, []byte("corrupted-cache-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := extractEmbeddedTraceStreamer(fsys, cacheRoot)
	if err != nil {
		t.Fatalf("re-extract over corrupted cache: %v", err)
	}
	if second.CacheReused {
		t.Fatalf("corrupted cache must be re-extracted, not reused: %+v", second)
	}
	if second.Path != first.Path {
		t.Fatalf("re-extraction must land on the same deterministic path, first=%s second=%s", first.Path, second.Path)
	}
	if err := verifyEmbeddedTraceStreamerFileHash(second.Path, embeddedTraceStreamerSHA256(binaryBody)); err != nil {
		t.Fatalf("re-extracted binary hash mismatch: %v", err)
	}
}

func TestEmbeddedTraceStreamerRuntimeExtractionRejectsUnsupportedHost(t *testing.T) {
	binaryRel := path.Join("unsupported-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].GOOS = "unsupported"
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)

	_, err := extractEmbeddedTraceStreamer(fsys, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no platform") {
		t.Fatalf("expected unsupported platform error, got %v", err)
	}
}

func TestEmbeddedTraceStreamerRuntimeExtractionRejectsHashMismatch(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)

	_, err := extractEmbeddedTraceStreamer(fsys, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected hash mismatch error, got %v", err)
	}
}

// starveExternalTraceStreamerDiscovery empties every external discovery
// tier so tests can observe pure embedded-tier behavior.
func starveExternalTraceStreamerDiscovery(t *testing.T) {
	t.Helper()
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")
}

func withEmbeddedTraceStreamerState(t *testing.T, fsys fs.FS, tagEnabled bool, platformGap string) {
	t.Helper()
	oldAssets := embeddedTraceStreamerAssetsFS
	oldTag := embeddedTraceStreamerTagEnabled
	oldGap := embeddedTraceStreamerPlatformGap
	embeddedTraceStreamerAssetsFS = func() fs.FS { return fsys }
	embeddedTraceStreamerTagEnabled = tagEnabled
	embeddedTraceStreamerPlatformGap = platformGap
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
		embeddedTraceStreamerTagEnabled = oldTag
		embeddedTraceStreamerPlatformGap = oldGap
	})
}

// Pins the unfrozen Batch 9B contract: with every external tier starved,
// an embed_streamer build selects the embedded payload as the final
// discovery tier and auto conversion proceeds on trace_streamer.
func TestTraceToolStatusSelectsEmbeddedTraceStreamerAsLastResort(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("#!/bin/sh\nexit 0\n")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	starveExternalTraceStreamerDiscovery(t)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !status.TraceStreamer.Available {
		t.Fatalf("embedded trace_streamer should be selected as last resort: %+v", status.TraceStreamer)
	}
	if !strings.Contains(status.TraceStreamer.Source, "embedded trace_streamer") {
		t.Fatalf("source should identify embedded trace_streamer: %q", status.TraceStreamer.Source)
	}
	if status.TraceStreamer.Path == "" {
		t.Fatalf("embedded selection must expose the extracted cache path: %+v", status.TraceStreamer)
	}
	if status.SelectedEngine != traceEngineTraceStreamer {
		t.Fatalf("auto should select trace_streamer when the embedded payload is usable: %+v", status)
	}
}

// Pins tier ordering: any earlier external tier hit resolves without
// even touching the embedded assets, so no extraction runs.
func TestResolveTraceStreamerToolExternalTiersWinWithoutExtraction(t *testing.T) {
	starveExternalTraceStreamerDiscovery(t)
	calls := 0
	oldAssets := embeddedTraceStreamerAssetsFS
	oldTag := embeddedTraceStreamerTagEnabled
	embeddedTraceStreamerAssetsFS = func() fs.FS { calls++; return nil }
	embeddedTraceStreamerTagEnabled = true
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
		embeddedTraceStreamerTagEnabled = oldTag
	})

	toolDir := t.TempDir()
	tool := filepath.Join(toolDir, traceStreamerBinaryName())
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if path, source, caveats := resolveTraceStreamerToolWithCaveats(Options{TraceStreamerPath: tool}); path != tool || source != "configured trace_streamer" || len(caveats) != 0 {
		t.Fatalf("explicit tier should win: path=%q source=%q caveats=%v", path, source, caveats)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", tool)
	if path, source, caveats := resolveTraceStreamerToolWithCaveats(Options{}); path != tool || source != "CODRAX_TRACE_STREAMER" || len(caveats) != 0 {
		t.Fatalf("env tier should win: path=%q source=%q caveats=%v", path, source, caveats)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("PATH", toolDir)
	if path, source, caveats := resolveTraceStreamerToolWithCaveats(Options{}); path == "" || !strings.Contains(source, "on PATH") || len(caveats) != 0 {
		t.Fatalf("PATH tier should win: path=%q source=%q caveats=%v", path, source, caveats)
	}
	if calls != 0 {
		t.Fatalf("embedded assets consulted %d times while an external tier hit; embedded must stay untouched", calls)
	}
}

// Pins fail-loud extraction: a broken embedded payload produces a
// structured caveat, no selection, and the same built-in fallback lane
// as an undiscovered tool.
func TestTraceToolStatusReportsEmbeddedExtractionFailureLoudly(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("embedded-trace-streamer-binary")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	manifest.Platforms[0].SHA256 = strings.Repeat("0", 64)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	starveExternalTraceStreamerDiscovery(t)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if status.TraceStreamer.Available || status.TraceStreamer.Path != "" {
		t.Fatalf("broken embedded payload must not be selected: %+v", status.TraceStreamer)
	}
	joined := strings.Join(status.TraceStreamer.Caveats, "\n")
	if !strings.Contains(joined, "embedded trace_streamer is not usable") {
		t.Fatalf("extraction failure must surface as a structured caveat, got %q", joined)
	}
	if status.SelectedEngine != traceEngineBuiltin {
		t.Fatalf("auto must fall back to built-in after embedded extraction failure: %+v", status)
	}
}

// Pins the unbundled-platform contract: an embed_streamer build on a
// platform outside the first wave reports an explicit structured gap
// instead of silently acting like a slim build.
func TestTraceToolStatusReportsUnbundledPlatformGap(t *testing.T) {
	gap := "embedded trace_streamer payload is enabled by the embed_streamer build tag but no binary is bundled for test/gap; the first wave bundles linux-amd64 and windows-amd64 only, so install or configure an external trace_streamer"
	withEmbeddedTraceStreamerState(t, nil, true, gap)
	starveExternalTraceStreamerDiscovery(t)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if status.TraceStreamer.Available || status.TraceStreamer.Path != "" {
		t.Fatalf("unbundled platform must not select a tool: %+v", status.TraceStreamer)
	}
	if joined := strings.Join(status.TraceStreamer.Caveats, "\n"); !strings.Contains(joined, gap) {
		t.Fatalf("unbundled platform gap must surface verbatim as a caveat, got %q", joined)
	}
	if status.SelectedEngine != traceEngineBuiltin {
		t.Fatalf("auto must fall back to built-in on an unbundled platform: %+v", status)
	}
}

func cloneEmbeddedTraceStreamerManifest(manifest embeddedTraceStreamerManifest) embeddedTraceStreamerManifest {
	manifest.Platforms = append([]embeddedTraceStreamerPlatform(nil), manifest.Platforms...)
	return manifest
}

func writeEmbeddedTraceStreamerManifest(t *testing.T, root string, manifest embeddedTraceStreamerManifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, embeddedTraceStreamerManifestName), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeEmbeddedTraceStreamerBinary(t *testing.T, root, rel string, body []byte, mode os.FileMode) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, body, mode); err != nil {
		t.Fatal(err)
	}
}

func embeddedTraceStreamerTestManifest(binaryRel string, binaryBody []byte) embeddedTraceStreamerManifest {
	return embeddedTraceStreamerManifest{
		SourceURL:   "https://gitcode.com/diting/hmtrace/tree/main",
		UpstreamRef: "6b05b2a60456910f05c149012b0d4833faa2d10e",
		Version:     "test fixture snapshot",
		LicenseID:   "Apache-2.0",
		ApprovalRef: "release-approval-123",
		Platforms: []embeddedTraceStreamerPlatform{{
			GOOS:         runtime.GOOS,
			GOARCH:       runtime.GOARCH,
			Path:         binaryRel,
			SHA256:       embeddedTraceStreamerSHA256(binaryBody),
			SizeBytes:    int64(len(binaryBody)),
			ActualFormat: "test fixture bytes",
		}},
	}
}

func embeddedTraceStreamerTestFS(t *testing.T, manifest embeddedTraceStreamerManifest, binaryRel string, binaryBody []byte, mode os.FileMode) fstest.MapFS {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		embeddedTraceStreamerManifestName: {Data: body, Mode: 0o444},
		binaryRel:                         {Data: binaryBody, Mode: mode},
	}
}
