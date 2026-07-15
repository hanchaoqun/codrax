package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func traceDBPostvalidationKnownLine(t *testing.T, tsNS int64) string {
	t.Helper()
	row, err := prepareTraceDBRenderedRow(
		tsNS,
		0,
		"waker",
		10,
		10,
		1,
		"sched_wakeup: comm=app pid=20 prio=53 target_cpu=2",
	)
	if err != nil {
		t.Fatal(err)
	}
	return row.line + "\n"
}

func adoptTraceDBPostvalidationFixture(t *testing.T, body []byte) (sealedConversionPublicationTarget, *sealedConversionFile) {
	t.Helper()
	finalPath := filepath.Join(t.TempDir(), "capture.systrace")
	target, err := prepareSealedConversionPublicationTarget(finalPath, ".codrax-sql-postvalidation-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := target.Cleanup(); cleanupErr != nil {
			t.Errorf("cleanup postvalidation fixture: %v", cleanupErr)
		}
	})
	if err := os.WriteFile(target.StagingPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sealed, err := target.stagingDir.AdoptRegularChild(target.finalLeaf, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sealed.Close(); closeErr != nil {
			t.Errorf("close postvalidation fixture: %v", closeErr)
		}
	})
	return target, sealed
}

func TestTraceDBSystraceHeldPostvalidationExactContract(t *testing.T) {
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	headerLines := strings.Count(systraceHeader, "\n")
	if coverage.Error != "" || !coverage.Found || coverage.RowsRead != headerLines+1 || coverage.RowsEmitted != 1 ||
		coverage.Table != traceDBPostvalidationCoverageTable {
		t.Fatalf("held postvalidation accounting drifted: %+v", coverage)
	}
	if _, err := os.Lstat(target.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("public output appeared during held validation: %v", err)
	}
}

func TestTraceDBSystraceHeldPostvalidationFailsClosed(t *testing.T) {
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	regressed := traceDBPostvalidationKnownLine(t, 900_000)
	unknownRow, err := prepareTraceDBRenderedRow(1_000_000, 0, "worker", 10, 10, 1, "codrax_unknown_event: value=1")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		body         []byte
		expectedRows int
		wantReason   string
	}{
		{name: "header drift", body: append([]byte("!"+systraceHeader[1:]), []byte(known)...), expectedRows: 1, wantReason: traceDBPostvalidationHeaderInvalid},
		{name: "owned unknown", body: []byte(systraceHeader + unknownRow.line + "\n"), expectedRows: 1, wantReason: traceDBPostvalidationUnknownOwnedRow},
		{name: "owned unparsed", body: []byte(systraceHeader + "not an ftrace row\n"), expectedRows: 1, wantReason: traceDBPostvalidationUnparsedOwnedRow},
		{name: "clock regression", body: []byte(systraceHeader + known + regressed), expectedRows: 2, wantReason: traceDBPostvalidationClockRegression},
		{name: "count mismatch", body: []byte(systraceHeader + known), expectedRows: 2, wantReason: traceDBPostvalidationCountMismatch},
		{name: "missing lf", body: []byte(systraceHeader + strings.TrimSuffix(known, "\n")), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
		{name: "crlf", body: []byte(systraceHeader + strings.TrimSuffix(known, "\n") + "\r\n"), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
		{name: "line over cap", body: []byte(systraceHeader + strings.Repeat("x", maxTraceDBSystraceLineBytes+1) + "\n"), expectedRows: 1, wantReason: traceDBPostvalidationScanFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, sealed := adoptTraceDBPostvalidationFixture(t, test.body)
			coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, test.expectedRows)
			reason, typed := traceDBOutputInvariantReason(err)
			if !typed || reason != test.wantReason || coverage.Error != test.wantReason {
				t.Fatalf("postvalidation reason=%q typed=%t coverage=%+v err=%v, want %q", reason, typed, coverage, err, test.wantReason)
			}
			if sealedTraceDBNormalizationFailureIsFatal(err) {
				t.Fatalf("single content failure became a multi-fault sealed DB authority error: %T %v", err, err)
			}
			if strings.Contains(err.Error(), target.stagingDir.Path()) || strings.Contains(coverage.Error, target.stagingDir.Path()) {
				t.Fatalf("private staging path leaked through postvalidation: coverage=%+v err=%v", coverage, err)
			}
			if _, statErr := os.Lstat(target.FinalPath); !os.IsNotExist(statErr) {
				t.Fatalf("failed validation published a public output: %v", statErr)
			}
		})
	}
}

func TestTraceDBSystraceHeldPostvalidationPreservesCancellation(t *testing.T) {
	target, sealed := adoptTraceDBPostvalidationFixture(t, []byte(systraceHeader+traceDBPostvalidationKnownLine(t, 1_000_000)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	coverage, err := validateSealedSystraceWithTraceQuery(ctx, sealed, target.FinalPath, 1)
	if !errors.Is(err, context.Canceled) || coverage.Error != traceDBPostvalidationCanceled {
		t.Fatalf("postvalidation cancellation identity drifted: coverage=%+v err=%v", coverage, err)
	}
}

func TestTraceDBSystraceHeldPostvalidationRejectsGenerationDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("same-path mutation while a sealed Windows handle is held is covered by native generation tests")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	mutated := append([]byte(nil), body...)
	mutated[len(mutated)-2] ^= 1
	if err := os.WriteFile(target.StagingPath, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	reason, typed := traceDBOutputInvariantReason(err)
	if !typed || reason != traceDBPostvalidationGenerationInvalid || coverage.Error != traceDBPostvalidationGenerationInvalid {
		t.Fatalf("generation drift escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
	}
}

func TestTraceDBSystraceHeldValidationPrecedesExactPublication(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	body := []byte(systraceHeader + traceDBPostvalidationKnownLine(t, 1_000_000))
	target, sealed := adoptTraceDBPostvalidationFixture(t, body)
	privateInfo, err := os.Stat(target.StagingPath)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := validateSealedSystraceWithTraceQuery(context.Background(), sealed, target.FinalPath, 1)
	if err != nil || coverage.RowsEmitted != 1 {
		t.Fatalf("held validation failed: coverage=%+v err=%v", coverage, err)
	}
	if _, err := os.Lstat(target.FinalPath); !os.IsNotExist(err) {
		t.Fatalf("public output existed before exact publication: %v", err)
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := publishSealedConversionFileNoReplace(context.Background(), target, sealed, ledger); err != nil {
		t.Fatal(err)
	}
	if len(ledger.created) != 1 || !ledger.created[0].authorityBound || !ledger.created[0].sealed {
		t.Fatalf("publication did not enter exact ledger authority: %+v", ledger.created)
	}
	got, err := os.ReadFile(target.FinalPath)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("public generation was not first-visible complete: bytes=%d err=%v", len(got), err)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		publicInfo, statErr := os.Stat(target.FinalPath)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if publicInfo.Mode().Perm() != privateInfo.Mode().Perm() {
			t.Fatalf("publication permission drifted: private=%#o public=%#o", privateInfo.Mode().Perm(), publicInfo.Mode().Perm())
		}
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatal(err)
	}
	if err := ledger.releaseOwnedAuthorities(); err != nil {
		t.Fatal(err)
	}
}

func TestTraceDBSystraceExportRegistersOnlyExactPublication(t *testing.T) {
	dbPath := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	output := filepath.Join(t.TempDir(), "capture.systrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	result, err := exportTraceDBToSystraceWithLedger(context.Background(), dbPath, output, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Path != output || result.OutputBytes <= 0 || result.EventsWritten <= 0 || len(result.TraceCoverage) != 1 {
		t.Fatalf("SQL export result incomplete: %+v", result)
	}
	if len(ledger.created) != 1 || !ledger.created[0].authorityBound || !ledger.created[0].sealed {
		t.Fatalf("SQL export retained a weak/path-only publication: %+v", ledger.created)
	}
	wantRead := result.EventsWritten + strings.Count(systraceHeader, "\n")
	if result.TraceCoverage[0].RowsRead != wantRead || result.TraceCoverage[0].RowsEmitted != result.EventsWritten || result.TraceCoverage[0].Error != "" {
		t.Fatalf("SQL export postvalidation coverage drifted: %+v", result.TraceCoverage)
	}
	entries, err := os.ReadDir(filepath.Dir(output))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-sql-systrace-") {
			t.Fatalf("private SQL systrace staging survived success: %s", entry.Name())
		}
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("exact ledger rollback did not remove SQL systrace: %v", err)
	}
}
