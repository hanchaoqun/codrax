package tracequery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func traceBundleV2JSONForTest(t *testing.T, bundlePath string, legacyJSON []byte) []byte {
	t.Helper()
	var bundle traceBundleFile
	if err := json.Unmarshal(legacyJSON, &bundle); err != nil {
		t.Fatalf("decode tracebundle fixture before V2 binding: %v", err)
	}
	bundle.Schema = tracebundle.SchemaV2
	bundle.schemaMode = 0

	baseDir := filepath.Dir(bundlePath)
	resolve := func(raw string) string {
		t.Helper()
		if filepath.IsAbs(raw) {
			return filepath.Clean(raw)
		}
		return filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(raw)))
	}
	relative := func(raw string) string {
		t.Helper()
		rel, err := filepath.Rel(baseDir, resolve(raw))
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("fixture child %q is not beneath bundle %s", raw, bundlePath)
		}
		return filepath.ToSlash(filepath.Clean(rel))
	}

	primaryRaw := bundle.Systrace
	if primaryRaw != "" {
		found := false
		for _, artifact := range bundle.Artifacts {
			if artifact.Type == "systrace" && resolve(artifact.Path) == resolve(primaryRaw) {
				found = true
				break
			}
		}
		if !found {
			bundle.Artifacts = append([]traceBundleArtifact{{Type: "systrace", Path: primaryRaw}}, bundle.Artifacts...)
		}
		bundle.Systrace = relative(primaryRaw)
	}

	pathMap := make(map[string]string, len(bundle.Artifacts))
	members := make([]tracebundle.CaptureMember, 0, len(bundle.Artifacts))
	for index := range bundle.Artifacts {
		artifact := &bundle.Artifacts[index]
		kind, causal, err := traceBundleCausalKind(artifact.Type, artifact.Path)
		if err != nil {
			t.Fatalf("fixture artifact %d: %v", index, err)
		}
		original := artifact.Path
		wirePath := relative(original)
		artifact.Path = wirePath
		pathMap[original] = wirePath
		if !causal {
			continue
		}
		body, err := os.ReadFile(resolve(original))
		if err != nil {
			t.Fatalf("read fixture child %s: %v", original, err)
		}
		bytes := int64(len(body))
		digest := sha256.Sum256(body)
		artifact.Bytes = &bytes
		artifact.SHA256 = hex.EncodeToString(digest[:])
		members = append(members, tracebundle.CaptureMember{
			Type: kind, Path: wirePath, Bytes: bytes, SHA256: artifact.SHA256,
		})
	}
	for index := range bundle.PerfClockAlignments {
		alignment := &bundle.PerfClockAlignments[index]
		if mapped := pathMap[alignment.ArtifactPath]; mapped != "" {
			alignment.ArtifactPath = mapped
		} else {
			alignment.ArtifactPath = relative(alignment.ArtifactPath)
		}
	}
	captureID, err := tracebundle.CaptureID(members)
	if err != nil {
		t.Fatalf("compute fixture capture ID: %v", err)
	}
	bundle.CaptureID = captureID
	body, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		t.Fatalf("encode V2 tracebundle fixture: %v", err)
	}
	return append(body, '\n')
}

func writeTraceBundleV2ForTest(t *testing.T, bundlePath string, legacyJSON []byte) {
	t.Helper()
	if err := os.WriteFile(bundlePath, traceBundleV2JSONForTest(t, bundlePath, legacyJSON), 0o644); err != nil {
		t.Fatalf("write V2 tracebundle fixture: %v", err)
	}
}
