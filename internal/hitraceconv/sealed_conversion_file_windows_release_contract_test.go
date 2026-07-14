//go:build windows

package hitraceconv

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

func TestReleaseRetainedTraceDBWindowsReadOnlyRollbackUsesHeldAuthority(t *testing.T) {
	dir := t.TempDir()
	finalDB := filepath.Join(dir, "readonly.trace.db")
	target, err := prepareTraceStreamerDBTarget(Options{TraceDBOutputPath: finalDB}, filepath.Join(dir, "input.sys"), filepath.Join(dir, "out.systrace"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := cleanupTraceStreamerDBTarget(target.Cleanup); cleanupErr != nil {
			t.Errorf("cleanup retained Windows fixture: %v", cleanupErr)
		}
	}()
	if err := os.WriteFile(target.StagingPath, []byte("readonly retained DB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target.StagingPath, 0o400); err != nil {
		t.Fatal(err)
	}
	outputs, err := adoptTraceStreamerDBOutputs(target.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer outputs.close()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishRetainedTraceDBOutputs(context.Background(), target, outputs, ledger); err != nil {
		t.Fatal(err)
	}
	if err := ledger.removeOwnedPath(target.finalBindingPath); err != nil {
		t.Fatalf("rollback read-only retained DB through held authority: %v", err)
	}
	if _, err := os.Lstat(finalDB); !os.IsNotExist(err) {
		t.Fatalf("read-only retained DB survived exact rollback: %v", err)
	}
}
