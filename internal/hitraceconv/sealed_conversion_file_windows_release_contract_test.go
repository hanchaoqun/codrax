//go:build windows

package hitraceconv

import (
	"io"
	"os"
	"testing"
)

func TestReleaseSealedConversionFileWindowsBlocksWriteAndDeleteSharing(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-share-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup Windows sharing fixture: %v", cleanupErr)
		}
	}()
	path, err := dir.ChildPath("report")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("held-generation")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("report", true)
	if err != nil {
		t.Fatal(err)
	}

	if writer, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		writer.Close()
		sealed.Close()
		t.Fatal("held Windows child allowed a second write handle")
	}
	if err := os.Remove(path); err == nil {
		sealed.Close()
		t.Fatal("held Windows child allowed delete sharing")
	}
	got, readErr := io.ReadAll(sealed.Reader())
	if readErr != nil || string(got) != string(want) {
		sealed.Close()
		t.Fatalf("held Windows child payload: got=%q err=%v", got, readErr)
	}
	if err := finishSealedConversionFile(sealed, nil); err != nil {
		t.Fatal(err)
	}

	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("closed sealed child retained Windows write denial: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}
