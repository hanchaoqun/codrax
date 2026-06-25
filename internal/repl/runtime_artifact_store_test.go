package repl

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeArtifactStorePutLoadLatest(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	payload := "# codrax-source: app.log\n2026-06-25T10:00:00Z ERROR boom\n"
	ref, err := store.Put("log", payload, "app.log")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !ref.Valid() {
		t.Fatalf("ref should be valid: %+v", ref)
	}
	if ref.Kind != "log" || ref.Source != "app.log" {
		t.Fatalf("unexpected ref metadata: %+v", ref)
	}
	if ref.Bytes != len(payload) {
		t.Fatalf("bytes=%d, want %d", ref.Bytes, len(payload))
	}
	snapshot := RuntimeArtifactSnapshot{Log: ref}
	if err := store.SaveLatest(snapshot); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}
	loadedSnapshot, err := store.LoadLatest()
	if err != nil {
		t.Fatalf("LoadLatest: %v", err)
	}
	if loadedSnapshot.Log.ID != ref.ID {
		t.Fatalf("latest log id=%q, want %q", loadedSnapshot.Log.ID, ref.ID)
	}
	loaded, err := store.Load(ref, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != payload {
		t.Fatalf("loaded payload changed: %q", loaded)
	}
}

func TestRuntimeArtifactStoreLoadCapsBytes(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	ref, err := store.Put("trace", strings.Repeat("x", 32), "trace.systrace")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	loaded, err := store.Load(ref, 5)
	if err != nil {
		t.Fatalf("Load capped: %v", err)
	}
	if loaded != "xxxxx" {
		t.Fatalf("capped load=%q, want five bytes", loaded)
	}
}

func TestRuntimeArtifactStoreRejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeArtifactStore(filepath.Join(root, "runtime_artifacts"))
	ref := RuntimeArtifactRef{
		SchemaVersion: runtimeArtifactStoreSchemaVersion,
		ID:            "log-escape",
		Kind:          "log",
		Path:          filepath.Join(root, "outside.log"),
		Bytes:         1,
	}
	if _, err := store.Load(ref, 0); err == nil {
		t.Fatalf("Load should reject paths outside the store root")
	}
}
