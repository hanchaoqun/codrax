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
	if err := validateEmbeddedTraceStreamerManifest(embeddedTraceStreamerDir); err != nil {
		t.Fatalf("embedded trace_streamer manifest validation failed: %v", err)
	}
}

func TestEmbeddedTraceStreamerManifestValidation(t *testing.T) {
	root := t.TempDir()
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("trace-streamer-binary")
	writeEmbeddedTraceStreamerBinary(t, root, binaryRel, binaryBody, 0o755)
	base := embeddedTraceStreamerManifest{
		SourceURL:   "https://gitcode.com/diting/hmtrace/tree/main",
		UpstreamRef: "6b05b2a60456910f05c149012b0d4833faa2d10e",
		LicenseID:   "Apache-2.0",
		ApprovalRef: "release-approval-123",
		Platforms: []embeddedTraceStreamerPlatform{{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
			Path:   binaryRel,
			SHA256: embeddedTraceStreamerSHA256(binaryBody),
		}},
	}
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
			name: "missing approval",
			setup: func(dir string, manifest embeddedTraceStreamerManifest) {
				manifest.ApprovalRef = ""
				writeEmbeddedTraceStreamerBinary(t, dir, binaryRel, binaryBody, 0o755)
				writeEmbeddedTraceStreamerManifest(t, dir, manifest)
			},
			want: "approval_ref",
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

func TestTraceToolStatusDefersEmbeddedTraceStreamerDiscovery(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("#!/bin/sh\nexit 0\n")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	oldAssets := embeddedTraceStreamerAssetsFS
	embeddedTraceStreamerAssetsFS = func() fs.FS { return fsys }
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
	})
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("CODRAX_TRACE_STREAMER_CACHE", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if status.TraceStreamer.Available || strings.Contains(status.TraceStreamer.Source, "embedded trace_streamer") {
		t.Fatalf("embedded trace_streamer should be deferred, not actively selected: %+v", status.TraceStreamer)
	}
	if status.SelectedEngine != traceEngineBuiltin {
		t.Fatalf("auto should fall back to built-in for trace-only when no external trace_streamer is found: %+v", status)
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
		LicenseID:   "Apache-2.0",
		ApprovalRef: "release-approval-123",
		Platforms: []embeddedTraceStreamerPlatform{{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
			Path:   binaryRel,
			SHA256: embeddedTraceStreamerSHA256(binaryBody),
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
