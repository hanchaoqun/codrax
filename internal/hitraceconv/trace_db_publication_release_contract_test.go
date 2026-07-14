package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type cancelWhenPathExistsContext struct {
	context.Context
	path string
}

func (ctx cancelWhenPathExistsContext) Err() error {
	if _, err := os.Lstat(ctx.path); err == nil {
		return context.Canceled
	}
	return ctx.Context.Err()
}

// A final DB created after target preparation is an external racing owner.
// Publication must fail without changing its identity or bytes, and the
// conversion-owned companion published first must be rolled back.
func TestReleaseRetainedTraceDBPublicationNeverOverwritesRacingDB(t *testing.T) {
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
	if err := os.WriteFile(target.StagingPath, []byte("owned staged DB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.StagingPath+".ohos.ts", []byte("owned companion"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputs, err := adoptTraceStreamerDBOutputs(target.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer outputs.close()
	externalBody := []byte("external DB owner")
	if err := os.WriteFile(finalDB, externalBody, 0o600); err != nil {
		t.Fatal(err)
	}
	externalBefore, err := os.Lstat(finalDB)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishRetainedTraceDBOutputs(context.Background(), target, outputs, ledger); err == nil {
		t.Fatal("racing external DB was overwritten instead of failing no-replace publication")
	}
	externalAfter, err := os.Lstat(finalDB)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(finalDB)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(externalBefore, externalAfter) || !bytes.Equal(got, externalBody) {
		t.Fatalf("racing external DB changed: same_identity=%t got=%q want=%q", os.SameFile(externalBefore, externalAfter), got, externalBody)
	}
	if _, err := os.Lstat(finalDB + ".ohos.ts"); !os.IsNotExist(err) {
		t.Fatalf("conversion-owned companion survived failed DB commit: %v", err)
	}
}

// A racing companion owner is checked before the DB commit marker is
// published. Neither the external companion nor a half-published DB may be
// changed or left behind.
func TestReleaseRetainedTraceDBPublicationNeverOverwritesRacingCompanion(t *testing.T) {
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
	if err := os.WriteFile(target.StagingPath, []byte("owned staged DB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.StagingPath+".ohos.ts", []byte("owned companion"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputs, err := adoptTraceStreamerDBOutputs(target.stagingDir)
	if err != nil {
		t.Fatal(err)
	}
	defer outputs.close()
	finalCompanion := finalDB + ".ohos.ts"
	externalBody := []byte("external companion owner")
	if err := os.WriteFile(finalCompanion, externalBody, 0o600); err != nil {
		t.Fatal(err)
	}
	externalBefore, err := os.Lstat(finalCompanion)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishRetainedTraceDBOutputs(context.Background(), target, outputs, ledger); err == nil {
		t.Fatal("racing external companion was overwritten instead of failing no-replace publication")
	}
	externalAfter, err := os.Lstat(finalCompanion)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(finalCompanion)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(externalBefore, externalAfter) || !bytes.Equal(got, externalBody) {
		t.Fatalf("racing external companion changed: same_identity=%t got=%q want=%q", os.SameFile(externalBefore, externalAfter), got, externalBody)
	}
	if _, err := os.Lstat(finalDB); !os.IsNotExist(err) {
		t.Fatalf("DB commit marker appeared despite companion collision: %v", err)
	}
}

func TestReleaseRetainedTraceDBCancellationBetweenCompanionAndDBRollsBackCompanion(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact retained DB publication is intentionally fail-closed on this platform")
	}
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
	if err := os.WriteFile(target.StagingPath, []byte("owned staged DB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target.StagingPath+".ohos.ts", []byte("owned companion"), 0o600); err != nil {
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
	ctx := cancelWhenPathExistsContext{Context: context.Background(), path: finalDB + ".ohos.ts"}
	err = publishRetainedTraceDBOutputs(ctx, target, outputs, ledger)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("between-pair cancellation identity lost: %T %v", err, err)
	}
	for _, path := range []string{finalDB, finalDB + ".ohos.ts"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("retained DB pair member survived between-pair cancellation: %s err=%v", path, err)
		}
	}
}
