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

func TestTraceStreamerProductionPinsSealedReadBeforeExactPublish(t *testing.T) {
	body := sourceGenerationFunctionBody(t, "trace_streamer_provider.go", "runTraceStreamerExport")
	required := []string{
		"newExternalToolInputLeaseWithProgress(",
		"inputLease.Command(ctx, lane.Path, nil, traceStreamerExportArguments",
		"runCommandWithProgressUntilExit(opts, cmd",
		"finishExternalToolCommand(ctx, inputLease, dbTarget.stagingDir, runErr)",
		"adoptTraceStreamerDBOutputs(dbTarget.stagingDir)",
		"exportTraceDBToSystraceFromSealedWithLedger(ctx, sealedOutputs.main",
		"integrityErr := sealedOutputs.validate()",
		"sealedTraceDBNormalizationFailureIsFatal(systraceErr)",
		"publishRetainedTraceDBOutputs(ctx, dbTarget, sealedOutputs, ledger)",
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
		strings.Contains(body, "artifactPathExists(dbPath +") ||
		strings.Contains(body, "os.Lstat(dbPath)") ||
		strings.Contains(body, "publishStagedTraceDB(") {
		t.Fatalf("production regained path-reopen normalization/publication discovery:\n%s", body)
	}
	publish := strings.Index(body, "publishRetainedTraceDBOutputs(ctx, dbTarget, sealedOutputs, ledger)")
	closeAfterPublish := strings.Index(body[publish:], "if closeErr := sealedOutputs.close();")
	if closeAfterPublish < 0 {
		t.Fatalf("sealed handles no longer close after exact retained publication:\n%s", body)
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

func TestTraceDBSystraceProductionPinsPrivateHeldValidationBeforeExactPublish(t *testing.T) {
	core := sourceGenerationFunctionBody(t, "streamerdb_export.go", "exportTraceDBToSystraceFromOpenWithLedger")
	assertSourceGenerationOrder(t, core,
		"sink.prepareForPublication(ctx)",
		"if closeErr := closeTraceDB(); closeErr != nil",
		"prepareSealedConversionPublicationTargetWithLedger(output, \".codrax-sql-systrace-*\", ledger)",
		"os.OpenFile(target.StagingPath",
		"target.stagingDir.AdoptRegularChild(target.finalLeaf, true)",
		"validateSealedSystraceWithTraceQueryReceipt(",
		"publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validationReceipt, ledger)",
		"result.Artifact, err = newValidatedSystraceArtifact",
	)
	for _, forbidden := range []string{
		"validateSystraceWithTraceQuery",
		"tracequery.BuildIndex",
		"os.OpenFile(output",
		"recordOpenFile(output",
		"os.Lstat(output)",
		"sealOwnedPath(output",
		"publishConversionFileNoReplace",
	} {
		if strings.Contains(core, forbidden) {
			t.Fatalf("SQL systrace core regained weak/path-based publication token %q:\n%s", forbidden, core)
		}
	}
	for _, singleton := range []string{
		"prepareSealedConversionPublicationTargetWithLedger(output, \".codrax-sql-systrace-*\", ledger)",
		"os.OpenFile(target.StagingPath",
		"target.stagingDir.AdoptRegularChild(target.finalLeaf, true)",
		"validateSealedSystraceWithTraceQueryReceipt(",
		"publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validationReceipt, ledger)",
	} {
		if count := strings.Count(core, singleton); count != 1 {
			t.Fatalf("SQL systrace single-authority token %q count=%d, want 1:\n%s", singleton, count, core)
		}
	}
	validationAt := strings.Index(core, "validateSealedSystraceWithTraceQueryReceipt(")
	publishAt := strings.Index(core, "publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validationReceipt, ledger)")
	if validationAt < 0 || publishAt <= validationAt ||
		!strings.Contains(core[validationAt:publishAt], "target.finalBindingPath") ||
		!strings.Contains(core[validationAt:publishAt], "if validationErr != nil") ||
		!strings.Contains(core[validationAt:publishAt], "return result, validationErr") {
		t.Fatalf("SQL systrace publication is no longer dominated by the validation error gate:\n%s", core)
	}
	cleanupAt := strings.Index(core, "cleanupErr := targetCleanup()")
	artifactAt := strings.Index(core, "result.Artifact, err = newValidatedSystraceArtifact")
	if cleanupAt < 0 || artifactAt <= cleanupAt || !strings.Contains(core[cleanupAt:artifactAt], "ledger.removeOwnedPath(output)") {
		t.Fatalf("post-publication staging cleanup can leave an undisclosed ledger artifact:\n%s", core)
	}

	validator := sourceGenerationFunctionBody(t, "trace_validation.go", "validateOwnedTraceOutput")
	assertSourceGenerationOrder(t, validator,
		"source.Validate()",
		"source.withOpenFile",
		"bytes.Equal(header, []byte(systraceHeader))",
		"tracequery.StreamScanHeldFileWithLineObserver",
	)
	scanAt := strings.Index(validator, "tracequery.StreamScanHeldFileWithLineObserver")
	firstValidateAt := strings.Index(validator, "source.Validate()")
	lastValidateAt := strings.LastIndex(validator, "source.Validate()")
	if strings.Count(validator, "source.Validate()") != 2 || firstValidateAt < 0 || scanAt <= firstValidateAt || lastValidateAt <= scanAt {
		t.Fatalf("held SQL systrace validator lost post-scan generation validation:\n%s", validator)
	}
	if !strings.Contains(validator, "event.Line <= headerLines") {
		t.Fatalf("held SQL systrace validator no longer prevents header/owned-row count compensation:\n%s", validator)
	}
	for _, forbidden := range []string{"tracequery.BuildIndex", "os.Open(", "os.OpenFile("} {
		if strings.Contains(validator, forbidden) {
			t.Fatalf("held SQL systrace validator regained path reopen %q:\n%s", forbidden, validator)
		}
	}

	standalone := sourceGenerationFunctionBody(t, "streamerdb_export.go", "exportTraceDBToSystrace")
	assertSourceGenerationOrder(t, standalone,
		"exportTraceDBToSystraceWithLedger(ctx, dbPath, output, ledger)",
		"ledger.validateOwnedPaths()",
		"ledger.releaseOwnedAuthorities()",
		"committed = true",
	)
}

func TestRetainedTraceDBExactPublicationStructurePinned(t *testing.T) {
	pair := sourceGenerationFunctionBody(t, "retained_trace_db_publication.go", "publishRetainedTraceDBOutputs")
	assertSourceGenerationOrder(t, pair,
		"outputs.validate()",
		"outputs.companion, target.finalCompanionLeaf()",
		"outputs.main, target.finalLeaf",
	)
	companionAt := strings.Index(pair, "outputs.companion, target.finalCompanionLeaf()")
	mainAt := strings.Index(pair, "outputs.main, target.finalLeaf")
	if companionAt < 0 || mainAt <= companionAt || !strings.Contains(pair[companionAt:mainAt], "ctx.Err()") {
		t.Fatalf("retained pair lost cancellation gate between companion and DB commit marker:\n%s", pair)
	}
	for _, forbidden := range []string{"os.Lstat(target.StagingPath)", "publishStagedTraceDB", "publishConversionFileNoReplace"} {
		if strings.Contains(pair, forbidden) {
			t.Fatalf("retained pair publication regained path authority %q:\n%s", forbidden, pair)
		}
	}

	linux := sourceGenerationFunctionBody(t, "retained_trace_db_publication_linux.go", "publishSealedConversionFilePlatform")
	for _, required := range []string{
		"source.Validate()", "unix.O_TMPFILE", "unix.IoctlFileClone", "copyStandaloneRange(ctx",
		"temp.Chmod(sourcePerm)", "info.Mode().Perm() != sourcePerm",
		"unix.Linkat(int(temp.Fd()), \"\"", "unix.AT_EMPTY_PATH", "linkLinuxRetainedTraceDBThroughHeldProcFD",
	} {
		if !strings.Contains(linux, required) {
			t.Fatalf("Linux exact retained publication lost %q:\n%s", required, linux)
		}
	}
	linuxProc := sourceGenerationFunctionBody(t, "retained_trace_db_publication_linux.go", "linkLinuxRetainedTraceDBThroughHeldProcFD")
	for _, required := range []string{
		`unix.Open("/proc/self/fd"`, "unix.O_NOFOLLOW", "unix.Fstatfs", "unix.PROC_SUPER_MAGIC",
		"unix.Fstatat(procFD, fdLeaf", "unix.Fstat(tempFD", "procEntry.Dev != temp.Dev", "procEntry.Ino != temp.Ino",
		"unix.Linkat(procFD, fdLeaf", "unix.AT_SYMLINK_FOLLOW",
	} {
		if !strings.Contains(linuxProc, required) {
			t.Fatalf("Linux held proc fallback lost %q:\n%s", required, linuxProc)
		}
	}
	for _, forbidden := range []string{"os.Open(source", "os.Link(", "os.Rename("} {
		if strings.Contains(linux, forbidden) {
			t.Fatalf("Linux retained publication regained source-path fallback %q:\n%s", forbidden, linux)
		}
	}

	darwin := sourceGenerationFunctionBody(t, "retained_trace_db_publication_darwin.go", "publishSealedConversionFilePlatform")
	for _, required := range []string{
		"source.Validate()", "unix.Fclonefileat", "dir.AdoptRegularChild", "snapshot.publishAndDetachOpenFile",
		"unix.RenameatxNp", "unix.RENAME_EXCL",
	} {
		if !strings.Contains(darwin, required) {
			t.Fatalf("Darwin exact retained publication lost %q:\n%s", required, darwin)
		}
	}

	windowsBody := sourceGenerationFunctionBody(t, "retained_trace_db_publication_windows.go", "publishSealedConversionFilePlatform")
	for _, required := range []string{
		"source.Validate()", "duplicatePublishedConversionParentPlatform", "source.publishAndDetachOpenFile",
		"renameRetainedTraceDBWindows", "newRetainedTraceDBPublication",
	} {
		if !strings.Contains(windowsBody, required) {
			t.Fatalf("Windows exact retained publication lost %q:\n%s", required, windowsBody)
		}
	}
	windowsRename := sourceGenerationFunctionBody(t, "retained_trace_db_publication_windows.go", "renameRetainedTraceDBWindows")
	for _, required := range []string{"ReplaceIfExists = 0", "RootDirectory = parent", "windows.NtSetInformationFile", "windows.FileRenameInformation"} {
		if !strings.Contains(windowsRename, required) {
			t.Fatalf("Windows root-relative no-replace rename lost %q:\n%s", required, windowsRename)
		}
	}
	windowsFS := sourceGenerationFunctionBody(t, "retained_trace_db_publication_windows.go", "validateRetainedTraceDBWindowsFileSystem")
	for _, required := range []string{"validateWindowsExactGenerationFileSystem", `"retained trace DB destination"`} {
		if !strings.Contains(windowsFS, required) {
			t.Fatalf("Windows retained publication lost NTFS exact-identity gate %q:\n%s", required, windowsFS)
		}
	}
	windowsExactFS := sourceGenerationFunctionBody(t, "windows_exact_generation.go", "validateWindowsExactGenerationFileSystem")
	for _, required := range []string{"windows.GetVolumeInformationByHandle", `strings.EqualFold(fileSystem, "NTFS")`} {
		if !strings.Contains(windowsExactFS, required) {
			t.Fatalf("shared Windows exact-generation gate lost %q:\n%s", required, windowsExactFS)
		}
	}
	for _, forbidden := range []string{"MoveFile", "os.Rename", "syscall.MoveFile"} {
		if strings.Contains(windowsBody, forbidden) || strings.Contains(windowsRename, forbidden) {
			t.Fatalf("Windows retained publication regained path rename %q", forbidden)
		}
	}
	otherUnix := sourceGenerationFunctionBody(t, "retained_trace_db_publication_unix_other.go", "publishSealedConversionFilePlatform")
	if !strings.Contains(otherUnix, "unsupported on this Unix platform") {
		t.Fatalf("unsupported Unix retained publication no longer fails closed:\n%s", otherUnix)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
