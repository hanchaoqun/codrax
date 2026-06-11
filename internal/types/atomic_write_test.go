package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileSync_WritesAndCleansTmp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "artifact.json")
	if err := AtomicWriteFileSync(p, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("AtomicWriteFileSync: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("content mismatch: %q err=%v", data, err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file must not survive")
	}
	// overwrite path keeps working
	if err := AtomicWriteFileSync(p, []byte(`{"ok":2}`), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
}
