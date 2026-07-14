package hitraceconv

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestAdoptTraceStreamerDBOutputsFreezesStableSet(t *testing.T) {
	parent := t.TempDir()
	dir, err := newPrivateConversionDir(parent, "trace-streamer-output-*")
	if err != nil {
		t.Fatal(err)
	}
	path, err := dir.ChildPath(sealedTraceDBVirtualName)
	if err != nil {
		t.Fatal(err)
	}
	source := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	copyTestFile(t, source, path)
	companion, err := dir.ChildPath(traceStreamerTimestampCompanionName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(companion, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	outputs, err := adoptTraceStreamerDBOutputs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if outputs.Size() <= 0 || !outputs.CompanionPresent() {
		t.Fatalf("sealed output set lost main/empty-companion state: size=%d companion=%t", outputs.Size(), outputs.CompanionPresent())
	}
	if err := outputs.finish(nil); err != nil {
		t.Fatalf("stable output set failed exit validation: %v", err)
	}
	if err := dir.FinalizeCleanup(); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptTraceStreamerDBOutputsRejectsSQLiteAuxiliaryState(t *testing.T) {
	for _, name := range traceStreamerSQLiteAuxiliaryNames {
		t.Run(name, func(t *testing.T) {
			dir, err := newPrivateConversionDir(t.TempDir(), "trace-streamer-aux-*")
			if err != nil {
				t.Fatal(err)
			}
			mainPath, err := dir.ChildPath(sealedTraceDBVirtualName)
			if err != nil {
				t.Fatal(err)
			}
			copyTestFile(t, createTraceDBFixture(t, traceStreamerIntegrationDBStatements()), mainPath)
			auxPath, err := dir.ChildPath(name)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(auxPath, []byte("uncheckpointed state"), 0o600); err != nil {
				t.Fatal(err)
			}
			outputs, err := adoptTraceStreamerDBOutputs(dir)
			if outputs != nil || !errors.Is(err, errTraceStreamerDBAuxiliaryState) {
				t.Fatalf("SQLite auxiliary %q was accepted: outputs=%v err=%v", name, outputs, err)
			}
			if err := dir.FinalizeCleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSealedTraceStreamerDBOutputsRejectLateState(t *testing.T) {
	for _, test := range []struct {
		name       string
		lateName   string
		want       string
		wantAuxErr bool
	}{
		{name: "timestamp-companion", lateName: traceStreamerTimestampCompanionName, want: "appeared after output adoption"},
		{name: "wal", lateName: sealedTraceDBVirtualName + "-wal", wantAuxErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, err := newPrivateConversionDir(t.TempDir(), "trace-streamer-late-*")
			if err != nil {
				t.Fatal(err)
			}
			mainPath, err := dir.ChildPath(sealedTraceDBVirtualName)
			if err != nil {
				t.Fatal(err)
			}
			copyTestFile(t, createTraceDBFixture(t, traceStreamerIntegrationDBStatements()), mainPath)
			outputs, err := adoptTraceStreamerDBOutputs(dir)
			if err != nil {
				t.Fatal(err)
			}
			latePath, err := dir.ChildPath(test.lateName)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(latePath, []byte("late state"), 0o600); err != nil {
				t.Fatal(err)
			}
			err = outputs.finish(nil)
			if test.wantAuxErr {
				if !errors.Is(err, errTraceStreamerDBAuxiliaryState) {
					t.Fatalf("late SQLite auxiliary was accepted: %v", err)
				}
			} else if !strings.Contains(errString(err), test.want) {
				t.Fatalf("late output state was accepted/misclassified: %v", err)
			}
			if err := dir.FinalizeCleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTraceStreamerDBOutputValidationRecoveryIsAllowlisted(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: os.ErrNotExist, code: "trace_db_missing"},
		{err: errSealedConversionFileEmpty, code: "trace_db_empty"},
		{err: errSealedConversionFileNotRegular, code: "trace_db_not_regular"},
		{err: errTraceStreamerDBAuxiliaryState, code: "trace_db_auxiliary_state"},
	} {
		if code, recoverable := traceStreamerDBOutputValidationCode(test.err); code != test.code || !recoverable {
			t.Fatalf("producer-shape error classification drifted: err=%v code=%q recoverable=%t", test.err, code, recoverable)
		}
	}
	for _, fatal := range []error{
		errSealedConversionFileIdentityUnavailable,
		errSealedConversionFileIdentityChanged,
		errPrivateConversionDirIdentityChanged,
		errPrivateConversionDirSecurityInvalid,
		errors.Join(errSealedConversionFileEmpty, errors.New("close authority failed")),
	} {
		if code, recoverable := traceStreamerDBOutputValidationCode(fatal); code != "trace_db_unsealed" || recoverable {
			t.Fatalf("authority failure became fallback-safe: err=%v code=%q recoverable=%t", fatal, code, recoverable)
		}
	}
}

func TestSealedTraceDBNormalizationFatalityIsFailClosed(t *testing.T) {
	authorityErr := newSealedTraceDBAuthorityError("vfs_unregister", errors.New("injected close failure"))
	if !sealedTraceDBNormalizationFailureIsFatal(authorityErr) {
		t.Fatal("typed sealed VFS lifecycle error became a recoverable normalize failure")
	}
	if !sealedTraceDBNormalizationFailureIsFatal(errors.Join(errors.New("query failed"), errors.New("rollback failed"))) {
		t.Fatal("joined normalize/cleanup failure became recoverable")
	}
	if sealedTraceDBNormalizationFailureIsFatal(errors.New("malformed producer database")) {
		t.Fatal("single producer-content failure unexpectedly became an authority failure")
	}
}

func TestTraceStreamerProductionPinsSealedReadBeforeLegacyPublish(t *testing.T) {
	body := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "runTraceStreamerExport")
	required := []string{
		"adoptTraceStreamerDBOutputs(dbTarget.stagingDir)",
		"exportTraceDBToSystraceFromSealedWithLedger(ctx, sealedOutputs.main",
		"sealedOutputs.finish(nil)",
		"sealedTraceDBNormalizationFailureIsFatal(systraceErr)",
		"publishStagedTraceDB(dbTarget, info, ledger)",
	}
	last := -1
	for _, token := range required {
		index := strings.Index(body, token)
		if index < 0 || index <= last {
			t.Fatalf("sealed read/publish order drifted at %q: positions previous=%d current=%d", token, last, index)
		}
		last = index
	}
	if strings.Contains(body, "exportTraceDBToSystraceWithLedger(ctx, dbPath") ||
		strings.Contains(body, "artifactPathExists(dbPath +") {
		t.Fatalf("production regained path-reopen normalization/companion discovery:\n%s", body)
	}
	if lstat := strings.Index(body, "os.Lstat(dbPath)"); lstat < strings.Index(body, "sealedOutputs.finish(nil)") {
		t.Fatalf("retained compatibility Lstat moved before sealed read exit gate: lstat=%d\n%s", lstat, body)
	}
}

func TestTraceDBExportWrappersHaveOneCommonCore(t *testing.T) {
	for _, wrapper := range []string{"exportTraceDBToSystraceWithLedger", "exportTraceDBToSystraceFromSealedWithLedger"} {
		body := sourceGenerationFunctionBody(t, "streamerdb_export.go", wrapper)
		if strings.Count(body, "exportTraceDBToSystraceFromOpenWithLedger(") != 1 {
			t.Fatalf("%s no longer delegates exactly once to the common export core:\n%s", wrapper, body)
		}
	}
	core := sourceGenerationFunctionBody(t, "streamerdb_export.go", "exportTraceDBToSystraceFromOpenWithLedger")
	if strings.Contains(core, "openTraceDB(") || strings.Contains(core, "openTraceDBFromSealed(") {
		t.Fatalf("common export core regained an input-opening authority:\n%s", core)
	}
	if !strings.Contains(core, `newSealedTraceDBAuthorityError("close_database_and_vfs", closeErr)`) {
		t.Fatalf("common export core no longer types sealed DB/VFS close failures as authority failures:\n%s", core)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
