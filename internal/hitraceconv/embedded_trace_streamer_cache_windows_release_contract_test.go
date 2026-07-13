//go:build windows

package hitraceconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseWindowsEmbeddedCacheReusesValidCanonical pins the Windows rule:
// a valid live canonical executable is reused by identity and the disposable
// staging file is removed.
func TestReleaseWindowsEmbeddedCacheReusesValidCanonical(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trace_streamer.exe")
	body := []byte("verified-windows-payload")
	if err := os.WriteFile(target, body, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeEmbeddedTraceStreamerCache(target, body, "windows"); err != nil {
		t.Fatal(err)
	}
	after, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("valid Windows canonical cache target was replaced instead of reused")
	}
	assertReleaseNoEmbeddedCacheTemps(t, dir)
}

// TestReleaseWindowsEmbeddedCacheCorruptionFailsLoudWithoutDeletingCanonical
// protects the only safe Windows behavior: never delete or overwrite a
// mismatching canonical executable. A later operator can inspect/repair it.
func TestReleaseWindowsEmbeddedCacheCorruptionFailsLoudWithoutDeletingCanonical(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trace_streamer.exe")
	corrupt := []byte("evil")
	want := []byte("good") // equal size forces the hash boundary, not size only.
	if err := os.WriteFile(target, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	err = writeEmbeddedTraceStreamerCache(target, want, "windows")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("corrupt Windows canonical did not fail loud: %v", err)
	}
	after, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatalf("corrupt Windows canonical was deleted: %v", statErr)
	}
	if !os.SameFile(before, after) {
		t.Fatal("corrupt Windows canonical identity changed")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt Windows canonical content was mutated: got=%q want=%q", got, corrupt)
	}
	assertReleaseNoEmbeddedCacheTemps(t, dir)
}

// A target created after the initial cache check must be treated as an
// external racing owner. MoveFileW without replace fails, the target survives
// byte-for-byte, and the staging file is cleaned.
func TestReleaseWindowsEmbeddedCacheRaceNeverOverwritesCanonical(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trace_streamer.exe")
	body := []byte("verified-windows-payload")
	external := []byte("external-windows-owner!!")
	if len(body) != len(external) {
		t.Fatal("test fixture must force the hash boundary with equal sizes")
	}
	originalPublish := embeddedTraceStreamerCachePublishNoReplace
	embeddedTraceStreamerCachePublishNoReplace = func(stagingPath, finalPath string) (bool, error) {
		if err := os.WriteFile(finalPath, external, 0o600); err != nil {
			return false, err
		}
		return originalPublish(stagingPath, finalPath)
	}
	t.Cleanup(func() { embeddedTraceStreamerCachePublishNoReplace = originalPublish })

	err := writeEmbeddedTraceStreamerCache(target, body, "windows")
	if err == nil {
		t.Fatal("racing Windows canonical owner was overwritten instead of failing loud")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(external) {
		t.Fatalf("racing Windows canonical content changed: got=%q want=%q", got, external)
	}
	assertReleaseNoEmbeddedCacheTemps(t, dir)
}

func assertReleaseNoEmbeddedCacheTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".trace_streamer-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("embedded cache staging files leaked: %v", matches)
	}
}
