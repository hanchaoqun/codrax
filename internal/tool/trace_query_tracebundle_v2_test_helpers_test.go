package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// writeToolTraceBundleV2Fixture upgrades a readable test manifest to the real
// V2 wire contract. It measures the fixture children instead of embedding fake
// provenance, so consumer tests exercise the same bytes/sha256/capture_id
// binding as production manifests while preserving unrelated metadata fields.
func writeToolTraceBundleV2Fixture(t *testing.T, bundlePath string, manifest []byte) {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(manifest, &document); err != nil {
		t.Fatalf("decode tracebundle fixture: %v", err)
	}
	rawArtifacts, ok := document["artifacts"].([]any)
	if !ok {
		t.Fatal("tracebundle fixture artifacts must be a JSON array")
	}
	members := make([]tracebundle.CaptureMember, 0, len(rawArtifacts))
	for index, rawArtifact := range rawArtifacts {
		artifact, ok := rawArtifact.(map[string]any)
		if !ok {
			t.Fatalf("tracebundle fixture artifact %d must be a JSON object", index)
		}
		kind, ok := artifact["type"].(string)
		if !ok {
			t.Fatalf("tracebundle fixture artifact %d type must be a string", index)
		}
		if kind != "systrace" && kind != "perftrace" {
			continue
		}
		wirePath, ok := artifact["path"].(string)
		if !ok {
			t.Fatalf("tracebundle fixture artifact %d path must be a string", index)
		}
		childPath := filepath.Join(filepath.Dir(bundlePath), filepath.FromSlash(wirePath))
		body, err := os.ReadFile(childPath)
		if err != nil {
			t.Fatalf("read tracebundle fixture child %q: %v", wirePath, err)
		}
		digest := sha256.Sum256(body)
		size := int64(len(body))
		sha := hex.EncodeToString(digest[:])
		artifact["bytes"] = size
		artifact["sha256"] = sha
		members = append(members, tracebundle.CaptureMember{
			Type: kind, Path: wirePath, Bytes: size, SHA256: sha,
		})
	}
	captureID, err := tracebundle.CaptureID(members)
	if err != nil {
		t.Fatalf("compute tracebundle fixture capture_id: %v", err)
	}
	document["schema"] = tracebundle.SchemaV2
	document["capture_id"] = captureID
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatalf("encode tracebundle fixture: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(bundlePath, body, 0o644); err != nil {
		t.Fatalf("write tracebundle fixture: %v", err)
	}
}
