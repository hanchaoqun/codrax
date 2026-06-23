package hitraceconv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	embeddedTraceStreamerDir          = "embedded_trace_streamer"
	embeddedTraceStreamerManifestName = "manifest.json"
)

type embeddedTraceStreamerManifest struct {
	SourceURL   string                          `json:"source_url"`
	UpstreamRef string                          `json:"upstream_ref"`
	LicenseID   string                          `json:"license_id"`
	ApprovalRef string                          `json:"approval_ref"`
	Platforms   []embeddedTraceStreamerPlatform `json:"platforms"`
}

type embeddedTraceStreamerPlatform struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

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
	binaryRel := filepath.Join(runtime.GOOS+"-"+runtime.GOARCH, "trace_streamer")
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
			SHA256: embeddedTraceStreamerTestSHA256(binaryBody),
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

func cloneEmbeddedTraceStreamerManifest(manifest embeddedTraceStreamerManifest) embeddedTraceStreamerManifest {
	manifest.Platforms = append([]embeddedTraceStreamerPlatform(nil), manifest.Platforms...)
	return manifest
}

func validateEmbeddedTraceStreamerManifest(root string) error {
	body, err := os.ReadFile(filepath.Join(root, embeddedTraceStreamerManifestName))
	if err != nil {
		return fmt.Errorf("read %s: %w", embeddedTraceStreamerManifestName, err)
	}
	var manifest embeddedTraceStreamerManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return fmt.Errorf("parse %s: %w", embeddedTraceStreamerManifestName, err)
	}
	if strings.TrimSpace(manifest.SourceURL) == "" {
		return fmt.Errorf("source_url is required")
	}
	if strings.TrimSpace(manifest.UpstreamRef) == "" {
		return fmt.Errorf("upstream_ref is required")
	}
	if strings.TrimSpace(manifest.LicenseID) == "" {
		return fmt.Errorf("license_id is required")
	}
	if strings.TrimSpace(manifest.ApprovalRef) == "" {
		return fmt.Errorf("approval_ref is required")
	}
	if len(manifest.Platforms) == 0 {
		return fmt.Errorf("at least one platform binary is required")
	}
	seen := map[string]bool{}
	for _, platform := range manifest.Platforms {
		if err := validateEmbeddedTraceStreamerPlatform(root, platform, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateEmbeddedTraceStreamerPlatform(root string, platform embeddedTraceStreamerPlatform, seen map[string]bool) error {
	goos := strings.TrimSpace(platform.GOOS)
	goarch := strings.TrimSpace(platform.GOARCH)
	if goos == "" || goarch == "" {
		return fmt.Errorf("platform goos/goarch are required")
	}
	key := goos + "/" + goarch
	if seen[key] {
		return fmt.Errorf("duplicate embedded trace_streamer platform %s", key)
	}
	seen[key] = true
	path, err := embeddedTraceStreamerPath(root, platform.Path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("embedded trace_streamer binary %s is not readable: %w", platform.Path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("embedded trace_streamer binary %s is a directory", platform.Path)
	}
	if goos != "windows" && info.Mode()&0o111 == 0 {
		return fmt.Errorf("embedded trace_streamer binary %s is not executable", platform.Path)
	}
	return verifyEmbeddedTraceStreamerHash(path, platform.SHA256)
}

func embeddedTraceStreamerPath(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("embedded trace_streamer path must be relative: %q", rel)
	}
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("embedded trace_streamer path must stay under %s: %q", root, rel)
	}
	return filepath.Join(root, clean), nil
}

func verifyEmbeddedTraceStreamerHash(path, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return fmt.Errorf("sha256 is required for %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	got := embeddedTraceStreamerTestSHA256(body)
	if got != want {
		return fmt.Errorf("sha256 mismatch for %s: got %s want %s", path, got, want)
	}
	return nil
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
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
}

func embeddedTraceStreamerTestSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
