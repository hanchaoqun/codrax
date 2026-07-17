package repl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestRuntimeArtifactStoreLoadLatestRejectsOversizedMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "latest.json"),
		[]byte(strings.Repeat(" ", int(runtimeArtifactLatestMaxBytes+1))),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	store := NewRuntimeArtifactStore(root)
	if _, err := store.LoadLatest(); err == nil || !strings.Contains(err.Error(), "exceeds metadata limit") {
		t.Fatalf("LoadLatest error=%v, want metadata-limit rejection", err)
	}
}

func TestRuntimeArtifactStoreLoadLatestRequiresCompleteClosedSchema(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	ref, err := store.Put("log", "log body\n", "app.log")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	valid := RuntimeArtifactSnapshot{
		SchemaVersion: runtimeArtifactStoreSchemaVersion,
		Log:           ref,
		UpdatedAt:     time.Now(),
	}
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	withoutRefs, err := json.Marshal(RuntimeArtifactSnapshot{
		SchemaVersion: runtimeArtifactStoreSchemaVersion,
		UpdatedAt:     time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongLane := valid
	wrongLane.Log.Kind = "trace"
	wrongLaneJSON, err := json.Marshal(wrongLane)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "unknown field", data: append(append([]byte(nil), validJSON[:len(validJSON)-1]...), []byte(`,"future":true}`)...), wantErr: "unknown field"},
		{name: "trailing JSON value", data: append(append([]byte(nil), validJSON...), []byte(` {}`)...), wantErr: "trailing JSON value"},
		{name: "no artifact refs", data: withoutRefs, wantErr: "no artifact refs"},
		{name: "lane kind mismatch", data: wrongLaneJSON, wantErr: "log lane has kind"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(store.dir, "latest.json"), test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.LoadLatest(); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadLatest error=%v, want substring %q", err, test.wantErr)
			}
		})
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

func TestRuntimeArtifactStoreLoadCapsOnRuneBoundaryAfterFullProof(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	ref, err := store.Put("trace", "ab你cd\n", "trace.systrace")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	loaded, err := store.Load(ref, 4)
	if err != nil {
		t.Fatalf("Load capped: %v", err)
	}
	if loaded != "ab" {
		t.Fatalf("rune-safe capped load=%q, want %q", loaded, "ab")
	}
}

func TestRuntimeArtifactStoreLoadRejectsPersistedSizeDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "truncated",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Truncate(path, int64(len("durable artifact\n")-1)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "appended",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString("extra\n"); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
			ref, err := store.Put("trace", "durable artifact\n", "trace.systrace")
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			test.mutate(t, ref.Path)
			if _, err := store.Load(ref, 4); err == nil || !strings.Contains(err.Error(), "size mismatch") {
				t.Fatalf("Load error=%v, want persisted-size mismatch", err)
			}
		})
	}
}

func TestRuntimeArtifactStoreLoadRejectsSameSizeReplacement(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	ref, err := store.Put("trace", "same-A\n", "trace.systrace")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	replacement := filepath.Join(filepath.Dir(ref.Path), "replacement.txt")
	if err := os.WriteFile(replacement, []byte("same-B\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, ref.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ref, 3); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Load error=%v, want whole-body checksum mismatch", err)
	}
}

func TestRuntimeArtifactStoreLoadChecksTailBeyondPublicationCap(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	ref, err := store.Put("trace", "prefix-stable-tail-A\n", "trace.systrace")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	corrupt := []byte("prefix-stable-tail-B\n")
	if len(corrupt) != ref.Bytes {
		t.Fatalf("test corruption size=%d want=%d", len(corrupt), ref.Bytes)
	}
	if err := os.WriteFile(ref.Path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ref, len("prefix")); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Load error=%v, want checksum mismatch beyond cap", err)
	}
}

func TestRuntimeArtifactStoreLoadRejectsLateBinaryWithMatchingMetadata(t *testing.T) {
	store := NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "runtime_artifacts"))
	payload := strings.Repeat("a", 70<<10) + "\x00tail\n"
	ref, err := store.Put("trace", payload, "trace.systrace")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := store.Load(ref, 64); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("Load error=%v, want full-body binary rejection", err)
	}
}

func TestRuntimeArtifactStoreRejectsEscapedPath(t *testing.T) {
	root := t.TempDir()
	store := NewRuntimeArtifactStore(filepath.Join(root, "runtime_artifacts"))
	ref, err := store.Put("log", "x\n", "inside.log")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	ref.Path = filepath.Join(root, "outside.log")
	if _, err := store.Load(ref, 0); err == nil {
		t.Fatalf("Load should reject paths outside the store root")
	}
}
