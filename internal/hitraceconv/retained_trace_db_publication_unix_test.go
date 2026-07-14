//go:build linux || darwin

package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReleaseRetainedTraceDBRejectsReboundSealedSource(t *testing.T) {
	dir := t.TempDir()
	finalDB := filepath.Join(dir, "operator.trace.db")
	target, err := prepareTraceStreamerDBTarget(Options{TraceDBOutputPath: finalDB}, filepath.Join(dir, "input.sys"), filepath.Join(dir, "out.systrace"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupTraceStreamerDBTarget(target.Cleanup); err != nil {
			t.Errorf("cleanup staging: %v", err)
		}
	}()
	original := []byte("sealed generation A")
	if err := os.WriteFile(target.StagingPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	outputs, err := adoptTraceStreamerDBOutputs(target.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer outputs.close()
	displaced := target.StagingPath + ".generation-a"
	if err := os.Rename(target.StagingPath, displaced); err != nil {
		t.Fatal(err)
	}
	replacement := []byte("replacement generation B")
	if err := os.WriteFile(target.StagingPath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	err = publishRetainedTraceDBOutputs(context.Background(), target, outputs, ledger)
	if err == nil || (!strings.Contains(err.Error(), "generation") && !strings.Contains(err.Error(), "identity changed")) {
		t.Fatalf("rebound sealed source was published or misclassified: %v", err)
	}
	if _, err := os.Lstat(finalDB); !os.IsNotExist(err) {
		t.Fatalf("final DB appeared after source generation drift: %v", err)
	}
	got, err := os.ReadFile(target.StagingPath)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("replacement source was changed: got=%q err=%v", got, err)
	}
}

func TestReleaseRetainedTraceDBLedgerNeverWeakensSameInodeGenerationRollback(t *testing.T) {
	dir := t.TempDir()
	finalDB := filepath.Join(dir, "operator.trace.db")
	target, err := prepareTraceStreamerDBTarget(Options{TraceDBOutputPath: finalDB}, filepath.Join(dir, "input.sys"), filepath.Join(dir, "out.systrace"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanupTraceStreamerDBTarget(target.Cleanup) }()
	original := []byte("sealed-generation")
	mutated := bytes.Repeat([]byte("X"), len(original))
	if err := os.WriteFile(target.StagingPath, original, 0o600); err != nil {
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
	before, err := os.Lstat(finalDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalDB, mutated, before.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(finalDB, time.Unix(0, before.ModTime().UnixNano()), time.Unix(0, before.ModTime().UnixNano())); err != nil {
		t.Fatal(err)
	}
	if err := ledger.validateOwnedPaths(); err == nil {
		t.Fatal("same-inode restored-mtime mutation passed exact commit validation")
	}
	if err := ledger.cleanup(); err == nil {
		t.Fatal("same-inode generation drift silently downgraded to path cleanup")
	}
	got, err := os.ReadFile(finalDB)
	if err != nil || !bytes.Equal(got, mutated) {
		t.Fatalf("mutated same-inode generation was deleted or changed: got=%q err=%v", got, err)
	}
}

func TestReleaseRetainedTraceDBLedgerPreservesReplacedFinalOwner(t *testing.T) {
	dir := t.TempDir()
	finalDB := filepath.Join(dir, "operator.trace.db")
	target, err := prepareTraceStreamerDBTarget(Options{TraceDBOutputPath: finalDB}, filepath.Join(dir, "input.sys"), filepath.Join(dir, "out.systrace"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanupTraceStreamerDBTarget(target.Cleanup) }()
	if err := os.WriteFile(target.StagingPath, []byte("sealed DB"), 0o600); err != nil {
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
	displaced := filepath.Join(dir, "displaced-owned-generation")
	if err := os.Rename(finalDB, displaced); err != nil {
		t.Fatal(err)
	}
	external := []byte("external replacement owner")
	if err := os.WriteFile(finalDB, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ledger.validateOwnedPaths(); err == nil {
		t.Fatal("final path replacement passed retained publication commit validation")
	}
	if err := ledger.cleanup(); err == nil {
		t.Fatal("ambiguous final replacement cleanup did not fail loud")
	}
	got, err := os.ReadFile(finalDB)
	if err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external replacement owner was changed: got=%q err=%v", got, err)
	}
}
