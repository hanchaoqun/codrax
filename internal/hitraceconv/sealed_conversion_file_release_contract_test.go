package hitraceconv

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestReleaseSealedConversionFileRegularEmptyMissingAndLifecycle(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-child-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup sealed-child fixture: %v", cleanupErr)
		}
	})

	if sealed, found, err := dir.TryAdoptRegularChild("missing", true); err != nil || found || sealed != nil {
		t.Fatalf("missing child result: sealed=%v found=%t err=%v", sealed, found, err)
	}
	if _, err := dir.AdoptRegularChild("missing", true); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("required missing child error=%v, want os.ErrNotExist", err)
	}

	emptyPath, err := dir.ChildPath("empty")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if sealed, found, err := dir.TryAdoptRegularChild("empty", true); !found || sealed != nil || !errors.Is(err, errSealedConversionFileEmpty) {
		t.Fatalf("non-empty gate result: sealed=%v found=%t err=%v", sealed, found, err)
	}
	empty, found, err := dir.TryAdoptRegularChild("empty", false)
	if err != nil || !found || empty == nil || empty.Size() != 0 {
		t.Fatalf("optional empty child result: sealed=%v found=%t err=%v", empty, found, err)
	}
	if got, err := io.ReadAll(empty.Reader()); err != nil || len(got) != 0 {
		t.Fatalf("empty held reader: got=%q err=%v", got, err)
	}
	if err := empty.Validate(); err != nil {
		t.Fatalf("validate empty held child: %v", err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
	if err := empty.Close(); err != nil {
		t.Fatalf("sealed close is not idempotent: %v", err)
	}
	if err := empty.Validate(); err == nil {
		t.Fatal("closed sealed child unexpectedly validated")
	}

	payloadPath, err := dir.ChildPath("payload")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("sealed-generation")
	if err := os.WriteFile(payloadPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("payload", true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(sealed.Reader())
	if err != nil || string(got) != string(want) || sealed.Size() != int64(len(want)) {
		t.Fatalf("held payload: got=%q size=%d err=%v", got, sealed.Size(), err)
	}
	if err := finishSealedConversionFile(sealed, nil); err != nil {
		t.Fatalf("finish unchanged held child: %v", err)
	}
}

func TestReleaseSealedConversionFileProviderCallGraph(t *testing.T) {
	checks := []struct {
		file      string
		function  string
		order     []string
		forbidden []string
	}{
		{
			file:     "simpleperf_text.go",
			function: "maybeConvertSimpleperfPerfData",
			order: []string{
				`reportDir.AdoptRegularChild("report_sample.txt", true)`,
				"sealedReport.Close()",
				"parseSimpleperfReport(ctx, sealedReport.Reader())",
				"finishSealedConversionFile(sealedReport, parseErr)",
				"writeSimpleperfSamplesToPerfTraceWithLedger(ctx, samples, perfTracePath, ledger)",
			},
			forbidden: []string{
				"os.Lstat(reportPath)", "os.Open(reportPath)",
				"convertSimpleperfReportFileToPerfTraceWithLedger(ctx, reportPath",
			},
		},
		{
			file:     "hiperf_proto.go",
			function: "maybeConvertHiperfPerfData",
			order: []string{
				`protoDir.AdoptRegularChild("report_sample.proto", true)`,
				"sealedProto.Close()",
				"readHiperfProto(ctx, sealedProto.Reader())",
				"finishSealedConversionFile(sealedProto, parseErr)",
				"writeHiperfProtoDataToPerfTraceWithLedger(ctx, data, perfTracePath, ledger)",
			},
			forbidden: []string{
				"os.Lstat(protoPath)", "os.Open(protoPath)", "readHiperfProtoFile(ctx, protoPath)",
			},
		},
	}
	for _, check := range checks {
		body := sourceGenerationFunctionBody(t, check.file, check.function)
		assertSourceGenerationOrder(t, body, check.order...)
		for _, fragment := range check.forbidden {
			if strings.Contains(body, fragment) {
				t.Fatalf("%s regained child-output path reader %q:\n%s", check.function, fragment, body)
			}
		}
	}

	adoptBody := sourceGenerationFunctionBody(t, "sealed_conversion_file.go", "TryAdoptRegularChild")
	for _, required := range []string{
		"adoptPrivateConversionRegularChildPlatform(&dir.platform, name)",
		"filegeneration.FromFile(file)",
		"identity.Strong()",
		"validatePrivateConversionRegularChildPlatform(&dir.platform, name, file, info)",
		"identity.SameVersion(confirmed)",
		"size: confirmed.Size()",
	} {
		if !strings.Contains(adoptBody, required) {
			t.Fatalf("sealed child adoption lost %q:\n%s", required, adoptBody)
		}
	}
	for _, forbidden := range []string{"os.Open(", "os.Lstat(", "os.Stat("} {
		if strings.Contains(adoptBody, forbidden) {
			t.Fatalf("sealed child adoption regained public path operation %q:\n%s", forbidden, adoptBody)
		}
	}

	validateBody := sourceGenerationFunctionBody(t, "sealed_conversion_file.go", "Validate")
	for _, required := range []string{
		"filegeneration.FromFile(sealed.file)",
		"sealed.identity.SameVersion(current)",
		"validatePrivateConversionRegularChildPlatform(&sealed.dir.platform, sealed.name, sealed.file, info)",
	} {
		if !strings.Contains(validateBody, required) {
			t.Fatalf("sealed child exit gate lost %q:\n%s", required, validateBody)
		}
	}
	for _, forbidden := range []string{"os.Open(", "os.Lstat(", "os.Stat("} {
		if strings.Contains(validateBody, forbidden) {
			t.Fatalf("sealed child exit gate regained public path operation %q:\n%s", forbidden, validateBody)
		}
	}
}

func TestReleaseSealedConversionFilePlatformSourcePins(t *testing.T) {
	unixAdopt := sourceGenerationFunctionBody(t, "sealed_conversion_file_unix.go", "adoptPrivateConversionRegularChildPlatform")
	for _, required := range []string{"unix.Openat(state.guardFD, name", "unix.O_NOFOLLOW", "unix.O_CLOEXEC", "unix.O_NONBLOCK"} {
		if !strings.Contains(unixAdopt, required) {
			t.Fatalf("Unix sealed adoption lost %q:\n%s", required, unixAdopt)
		}
	}
	unixValidate := sourceGenerationFunctionBody(t, "sealed_conversion_file_unix.go", "validatePrivateConversionRegularChildPlatform")
	for _, required := range []string{
		"unix.Fstatat(state.guardFD, name", "unix.AT_SYMLINK_NOFOLLOW",
		"unix.Fstat(int(file.Fd())", "runtime.KeepAlive(file)",
	} {
		if !strings.Contains(unixValidate, required) {
			t.Fatalf("Unix sealed binding lost %q:\n%s", required, unixValidate)
		}
	}

	windowsAdopt := sourceGenerationFunctionBody(t, "sealed_conversion_file_windows.go", "adoptPrivateConversionRegularChildPlatform")
	for _, required := range []string{
		"windows.NtCreateFile(", "RootDirectory: windows.Handle(state.guard.Fd())",
		"Attributes:    privateConversionDirWindowsObjectAttrs", "windows.FILE_SHARE_READ",
		"windows.FILE_OPEN_REPARSE_POINT", "windows.FILE_ATTRIBUTE_REPARSE_POINT",
		"windows.STATUS_NO_SUCH_FILE", "windows.STATUS_OBJECT_NAME_NOT_FOUND",
		"runtime.KeepAlive(state.guard)",
	} {
		if !strings.Contains(windowsAdopt, required) {
			t.Fatalf("Windows sealed adoption lost %q:\n%s", required, windowsAdopt)
		}
	}
}
