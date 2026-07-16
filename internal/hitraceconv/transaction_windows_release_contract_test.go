//go:build windows

package hitraceconv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseWindowsRecordOpenFileValidatesWhileWriterHandleIsLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live-writer-output.bin")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("live-writer-generation")); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := ledger.recordOpenFile(path, writer); err != nil {
		writer.Close()
		t.Fatalf("record Windows output while writer handle remains live: %v", err)
	}
	if err := writer.Sync(); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.sealOwnedPath(path, int64(len("live-writer-generation"))); err != nil {
		t.Fatalf("seal Windows live-writer output: %v", err)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("validate Windows live-writer output: %v", err)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("cleanup Windows live-writer output: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Windows live-writer output survived rollback: %v", err)
	}
}
