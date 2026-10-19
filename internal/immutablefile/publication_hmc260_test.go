package immutablefile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHMC260ImmutableHeldRootCannotReturnReboundPath(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "root")
	data := []byte("complete original")
	if _, err := Publish(dir, "original.json", data); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(dir, filepath.Join(base, "held-root")); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(dir, "original.json", data); err != nil {
		t.Fatal(err)
	}
	if err := Matches(root, "original.json", data); err == nil {
		t.Fatal("detached root returned newly rebound path")
	}
}

func TestHMC260ImmutableReadRejectsEscapesAndSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []string{"link.json", "../outside.json", outside, ".", "", "sub\\outside.json"} {
		if _, err := Read(dir, leaf); err == nil {
			t.Fatalf("accepted unsafe leaf %q", leaf)
		}
	}
	if err := ReplaceCurrent(dir, "link.json", []byte("changed")); err == nil {
		t.Fatal("current publication followed symlink")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "outside bytes" {
		t.Fatal("outside target changed")
	}
}
