//go:build embed_streamer && ((windows && amd64) || (linux && amd64))

package hitraceconv

import (
	"runtime"
	"testing"
)

// Payload pin for embed_streamer builds on bundled platforms: the
// compiled binary must carry exactly the host platform's payload, the
// per-platform manifest must survive the shared schema validation, and
// the embedded bytes must match the manifest's sha256 + size_bytes
// attestation. This is the compile-level half of the per-platform
// distribution ruling (windows builds embed only the windows binary,
// linux builds only the linux binary).
func TestEmbedStreamerBuildCarriesHostPlatformPayload(t *testing.T) {
	if !embeddedTraceStreamerTagEnabled {
		t.Fatal("embed_streamer build must enable the embedded payload tier")
	}
	fsys := embeddedTraceStreamerAssetsFS()
	if fsys == nil {
		t.Fatal("embed_streamer build on a bundled platform must expose the embedded assets FS")
	}
	manifest, err := loadEmbeddedTraceStreamerManifestFS(fsys)
	if err != nil {
		t.Fatalf("load embedded manifest: %v", err)
	}
	if err := validateEmbeddedTraceStreamerManifestFields(manifest); err != nil {
		t.Fatalf("embedded manifest failed shared schema validation: %v", err)
	}
	if len(manifest.Platforms) != 1 {
		t.Fatalf("per-platform payload must declare exactly one platform, got %d", len(manifest.Platforms))
	}
	platform := manifest.Platforms[0]
	if platform.GOOS != runtime.GOOS || platform.GOARCH != runtime.GOARCH {
		t.Fatalf("embedded payload platform %s/%s does not match build target %s/%s", platform.GOOS, platform.GOARCH, runtime.GOOS, runtime.GOARCH)
	}
	// readEmbeddedTraceStreamerAsset verifies both size_bytes and sha256
	// against the actual embedded bytes.
	body, _, err := readEmbeddedTraceStreamerAsset(fsys, platform)
	if err != nil {
		t.Fatalf("embedded payload bytes do not match manifest attestation: %v", err)
	}
	if int64(len(body)) != platform.SizeBytes {
		t.Fatalf("embedded payload size %d != manifest size_bytes %d", len(body), platform.SizeBytes)
	}
}
