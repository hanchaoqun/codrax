//go:build windows

package hitraceconv

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsStagingHelperModeEnv    = "CODRAX_WINDOWS_STAGING_HELPER_MODE"
	windowsStagingHelperPathLogEnv = "CODRAX_WINDOWS_STAGING_HELPER_PATH_LOG"
)

// TestMain also makes the native Windows test binary a real child-process
// fixture. This exercises CreateProcess and the provider staging paths without
// depending on PowerShell, cmd.exe, or a separately installed test tool.
func TestMain(m *testing.M) {
	if mode := strings.TrimSpace(os.Getenv(windowsStagingHelperModeEnv)); mode != "" {
		os.Exit(runWindowsStagingHelper(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runWindowsStagingHelper(mode string, args []string) int {
	input := ""
	output := ""
	for index := 0; index+1 < len(args); index++ {
		switch args[index] {
		case "-i":
			input = args[index+1]
		case "-o", "-e":
			output = args[index+1]
		}
	}
	if output == "" {
		_, _ = fmt.Fprintln(os.Stderr, "staging helper did not receive -o/-e output")
		return 24
	}
	if logPath := strings.TrimSpace(os.Getenv(windowsStagingHelperPathLogEnv)); logPath != "" {
		if err := os.WriteFile(logPath, []byte(output+"\n"), 0o600); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "write staging path log: %v\n", err)
			return 26
		}
	}
	if strings.HasSuffix(mode, "-failure") {
		_, _ = fmt.Fprintln(os.Stderr, "codrax native Windows staging helper failed as requested")
		return 23
	}
	var err error
	switch mode {
	case "simpleperf-success":
		err = os.WriteFile(output, []byte(syntheticSimpleperfReport()), 0o600)
	case "hiperf-success":
		if input == "" {
			err = fmt.Errorf("hiperf helper did not receive -i input")
			break
		}
		payload, readErr := os.ReadFile(input)
		if readErr != nil {
			err = fmt.Errorf("read held hiperf input: %w", readErr)
			break
		}
		if !bytes.Equal(payload, syntheticRawPerfData()) {
			err = fmt.Errorf("held hiperf input mismatch: got=%d want=%d", len(payload), len(syntheticRawPerfData()))
			break
		}
		err = os.WriteFile(output, syntheticHiperfProtoStream(), 0o600)
	case "trace-streamer-success":
		err = writeWindowsStagingTraceDB(output)
	default:
		err = fmt.Errorf("unknown staging helper mode %q", mode)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "staging helper error: %v\n", err)
		return 25
	}
	return 0
}

func writeWindowsStagingTraceDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	for _, statement := range traceStreamerIntegrationDBStatements() {
		if _, err := db.Exec(statement); err != nil {
			_ = db.Close()
			return fmt.Errorf("exec trace DB fixture statement: %w", err)
		}
	}
	return db.Close()
}

func TestReleasePrivateConversionDirWindowsACLAuthorityAndInheritance(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-windows-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.Validate(); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivateDirectorySecurity(t, dir.Path(), false, windows.Handle(dir.platform.guard.Fd()))
	child, err := dir.ChildPath("payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivateDirectorySecurity(t, child, true, 0)
	nested := filepath.Join(dir.Path(), "nested", "deeper")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	readOnlyChild := filepath.Join(nested, "read-only.bin")
	if err := os.WriteFile(readOnlyChild, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{readOnlyChild, nested, dir.Path()} {
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatalf("mark Windows staging path read-only %s: %v", path, err)
		}
	}
	for _, invalid := range []string{"CON", "NUL.txt", "PRN.log", "AUX", "CLOCK$", "COM1.txt", "LPT9", "payload:stream", "trailing.", "trailing ", "control\x01"} {
		if _, err := dir.ChildPath(invalid); err == nil {
			t.Fatalf("Windows-special child name %q was accepted", invalid)
		}
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("private directory survived cleanup: %v", err)
	}
}

func TestReleasePrivateConversionDirWindowsGuardBlocksReplacementAndRetryCleansLateChild(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-guard-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir.Path(), dir.Path()+".replacement-attempt"); err == nil {
		t.Fatal("held no-delete-share guard allowed private directory replacement")
	}

	dir.mu.Lock()
	if err := dir.removeChildrenLocked(); err != nil {
		dir.mu.Unlock()
		t.Fatal(err)
	}
	lateChild := filepath.Join(dir.Path(), "late-child")
	if err := os.WriteFile(lateChild, []byte("late"), 0o600); err != nil {
		dir.mu.Unlock()
		t.Fatal(err)
	}
	removeErr := removePrivateConversionDirRootPlatform(dir.path, dir.identity, &dir.platform)
	guardRetained := dir.platform.guard != nil
	dir.mu.Unlock()
	if removeErr == nil {
		t.Fatal("non-empty Windows root removal unexpectedly succeeded")
	}
	if !guardRetained {
		t.Fatal("failed Windows root removal consumed the held guard")
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatalf("Windows cleanup retry did not remove late child: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("retried Windows private directory survived: %v", err)
	}
}

func TestReleasePrivateConversionDirWindowsNoAccessEnumerationFallback(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-noaccess-fallback-*")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(dir.Path(), name), []byte(name), 0o600); err != nil {
			_ = dir.FinalizeCleanup()
			t.Fatal(err)
		}
	}
	primaryCalls := 0
	names, err := privateConversionDirWindowsDirectoryNamesWithPrimary(
		windows.Handle(dir.platform.guard.Fd()),
		dir.Path(),
		func(windows.Handle) ([]string, error) {
			primaryCalls++
			return nil, windows.ERROR_NOACCESS
		},
	)
	if err != nil {
		_ = dir.FinalizeCleanup()
		t.Fatalf("Win32 998 enumeration fallback failed: %v", err)
	}
	sort.Strings(names)
	if primaryCalls != 1 || strings.Join(names, ",") != "first,second" {
		_ = dir.FinalizeCleanup()
		t.Fatalf("fallback calls=%d names=%v, want one call and both children", primaryCalls, names)
	}
	if err := dir.FinalizeCleanup(); err != nil {
		t.Fatalf("cleanup after fallback probe: %v", err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("private directory survived fallback cleanup: %v", err)
	}
}

func TestReleasePrivateConversionDirWindowsReparseCleanupIsSentinelSafe(t *testing.T) {
	parent := t.TempDir()
	dir, err := newPrivateConversionDir(parent, "codrax-private-reparse-*")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir.Path(), "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		_ = dir.FinalizeCleanup()
		t.Skipf("Windows symlink privilege/developer mode unavailable: %v", err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(sentinel); err != nil || string(body) != "external" {
		t.Fatalf("Windows cleanup followed child reparse point: body=%q err=%v", body, err)
	}
}

func TestReleasePrivateConversionDirWindowsRejectsACLDriftAndCleans(t *testing.T) {
	user, system, err := privateConversionDirPrincipals()
	if err != nil {
		t.Fatal(err)
	}
	exact := "(A;OICI;FA;;;" + user.String() + ")"
	if !user.Equals(system) {
		exact += "(A;OICI;FA;;;" + system.String() + ")"
	}
	cases := []struct {
		name      string
		dacl      string
		protected bool
		skip      bool
	}{
		{name: "extra-everyone", dacl: exact + "(A;OICI;FR;;;WD)", protected: true},
		{name: "unprotected", dacl: exact, protected: false},
		{name: "missing-system", dacl: "(A;OICI;FA;;;" + user.String() + ")", protected: true, skip: user.Equals(system)},
		{name: "missing-current-user", dacl: "(A;OICI;FA;;;" + system.String() + ")", protected: true, skip: user.Equals(system)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip("current process runs as LocalSystem")
			}
			dir, err := newPrivateConversionDir(t.TempDir(), "codrax-private-drift-*")
			if err != nil {
				t.Fatal(err)
			}
			if err := setWindowsPrivateDirectoryDACL(dir, tc.dacl, tc.protected); err != nil {
				t.Fatal(err)
			}
			if err := dir.Validate(); !errors.Is(err, errPrivateConversionDirSecurityInvalid) {
				t.Fatalf("ACL drift error=%v, want security sentinel", err)
			}
			if err := dir.Cleanup(); err != nil {
				t.Fatalf("held-handle cleanup failed after ACL drift: %v", err)
			}
			if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
				t.Fatalf("ACL-drift directory survived cleanup: %v", err)
			}
		})
	}
}

func TestReleasePrivateConversionDirWindowsDeepParentAndConcurrentLifecycle(t *testing.T) {
	parent := t.TempDir()
	for len(parent) < 280 {
		parent = filepath.Join(parent, "deep-private-authority-segment")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := newPrivateConversionDir(parent, "codrax-private-deep-*")
	if err != nil {
		t.Fatalf("extended-length private directory creation failed: %v", err)
	}
	if err := dir.Validate(); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir.Path(), "nested", "deeper")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "payload"), []byte("deep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("deep Windows private directory survived cleanup: %v", err)
	}

	const workers = 16
	concurrentParent := t.TempDir()
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[string]struct{}, workers)
	errs := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir, err := newPrivateConversionDir(concurrentParent, "codrax-private-concurrent-*")
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			if _, duplicate := seen[dir.Path()]; duplicate {
				err = fmt.Errorf("duplicate private path %s", dir.Path())
			} else {
				seen[dir.Path()] = struct{}{}
			}
			mu.Unlock()
			if err == nil {
				err = dir.Validate()
			}
			if cleanupErr := dir.Cleanup(); err == nil {
				err = cleanupErr
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for path := range seen {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("concurrent Windows private directory survived cleanup: path=%s err=%v", path, err)
		}
	}
}

func TestReleaseWindowsProviderStagingSuccessAndFailure(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("simpleperf", func(t *testing.T) {
		for _, outcome := range []string{"success", "failure"} {
			t.Run(outcome, func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "capture.perfdata")
				if err := os.WriteFile(input, syntheticRawPerfData(), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(windowsStagingHelperModeEnv, "simpleperf-"+outcome)
				pathLog := filepath.Join(dir, "simpleperf-staging-path.txt")
				t.Setenv(windowsStagingHelperPathLogEnv, pathLog)
				result, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: executable})
				if err != nil {
					t.Fatalf("simpleperf %s conversion: %v", outcome, err)
				}
				assertPerfProviderOutcome(t, result.ProviderDecisions, perfProviderNameSimpleperfText, outcome == "success")
				assertWindowsStagingPathRemoved(t, pathLog)
				assertNoWindowsProviderStagingDirs(t, dir)
			})
		}
	})

	t.Run("hiperf", func(t *testing.T) {
		for _, outcome := range []string{"success", "failure"} {
			t.Run(outcome, func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "capture.htrace")
				body := append(syntheticProfilerTraceRoot(), syntheticStandaloneProfilerBlock(
					profilerDataTypeHiperf, "hiperf-plugin", "1.0", syntheticRawPerfData())...)
				if err := os.WriteFile(input, body, 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(windowsStagingHelperModeEnv, "hiperf-"+outcome)
				pathLog := filepath.Join(dir, "hiperf-staging-path.txt")
				t.Setenv(windowsStagingHelperPathLogEnv, pathLog)
				output := filepath.Join(dir, "out.systrace")
				result, err := ConvertFile(context.Background(), Options{
					InputPath: input, OutputPath: output,
					TraceEngine: traceEngineBuiltin, HiperfPath: executable,
				})
				if err != nil {
					t.Fatalf("hiperf %s conversion: %v", outcome, err)
				}
				assertPerfProviderOutcome(t, result.ProviderDecisions, perfProviderNameHiperfProto, outcome == "success")
				if outcome == "success" {
					got, readErr := os.ReadFile(filepath.Join(dir, "out.perf.data"))
					if readErr != nil || !bytes.Equal(got, syntheticRawPerfData()) {
						t.Fatalf("published Windows HIPERF sidecar mismatch: bytes=%d err=%v", len(got), readErr)
					}
				}
				assertWindowsStagingPathRemoved(t, pathLog)
				assertNoWindowsProviderStagingDirs(t, dir)
			})
		}
	})

	t.Run("trace-streamer", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			outcome  string
			retained bool
		}{
			{name: "transient-success", outcome: "success"},
			{name: "retained-success", outcome: "success", retained: true},
			{name: "failure", outcome: "failure"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				input := filepath.Join(dir, "capture.sys")
				if err := os.WriteFile(input, []byte("native Windows trace_streamer fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv(windowsStagingHelperModeEnv, "trace-streamer-"+tc.outcome)
				pathLog := filepath.Join(dir, "trace-streamer-staging-path.txt")
				t.Setenv(windowsStagingHelperPathLogEnv, pathLog)
				result, err := ConvertFile(context.Background(), Options{
					InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"),
					TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: executable, KeepTraceDB: tc.retained,
				})
				if tc.outcome == "failure" {
					if err == nil || !strings.Contains(err.Error(), "trace_streamer") {
						t.Fatalf("trace_streamer failure result=%+v err=%v", result, err)
					}
				} else if err != nil {
					t.Fatalf("trace_streamer %s conversion: %v", tc.name, err)
				} else if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, true) {
					t.Fatalf("trace_streamer success decision missing: %+v", result.TraceDecisions)
				}
				assertWindowsStagingPathRemoved(t, pathLog)
				assertNoWindowsProviderStagingDirs(t, dir)
			})
		}
	})
}

func assertWindowsStagingPathRemoved(t *testing.T, pathLog string) {
	t.Helper()
	body, err := os.ReadFile(pathLog)
	if err != nil {
		t.Fatalf("read provider staging path log: %v", err)
	}
	stagingPath := strings.TrimSpace(string(body))
	if stagingPath == "" {
		t.Fatal("provider staging path log is empty")
	}
	if _, err := os.Lstat(filepath.Dir(stagingPath)); !os.IsNotExist(err) {
		t.Fatalf("provider staging directory survived: path=%s err=%v", filepath.Dir(stagingPath), err)
	}
}

func assertPerfProviderOutcome(t *testing.T, decisions []PerfProviderDecision, provider string, wantSuccess bool) {
	t.Helper()
	for _, decision := range decisions {
		if decision.ProviderName != provider {
			continue
		}
		if !decision.Attempted || decision.Succeeded != wantSuccess {
			t.Fatalf("provider %s outcome=%+v want success=%t", provider, decision, wantSuccess)
		}
		return
	}
	t.Fatalf("provider %s decision missing: %+v", provider, decisions)
}

func assertNoWindowsProviderStagingDirs(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, "codrax-trace-streamer-") || strings.HasPrefix(name, ".codrax-trace-db-") ||
			strings.HasPrefix(name, ".codrax-sql-systrace-") ||
			strings.HasSuffix(name, ".simpleperf") || strings.HasSuffix(name, ".hiperf") {
			return fmt.Errorf("provider staging directory leaked: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func setWindowsPrivateDirectoryDACL(dir *privateConversionDir, daclString string, protected bool) error {
	if dir == nil || dir.platform.guard == nil {
		return fmt.Errorf("private directory Windows guard is missing")
	}
	sd, err := windows.SecurityDescriptorFromString("D:" + map[bool]string{true: "P"}[protected] + daclString)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	information := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if protected {
		information = windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	}
	return windows.SetSecurityInfo(windows.Handle(dir.platform.guard.Fd()), windows.SE_FILE_OBJECT, information, nil, nil, dacl, nil)
}

func assertWindowsPrivateDirectorySecurity(t *testing.T, path string, inherited bool, handle windows.Handle) {
	t.Helper()
	var (
		sd  *windows.SECURITY_DESCRIPTOR
		err error
	)
	if handle != 0 {
		sd, err = windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
			windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	} else {
		sd, err = windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
			windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	}
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		t.Fatal(err)
	}
	user, system, err := privateConversionDirPrincipals()
	if err != nil {
		t.Fatal(err)
	}
	if !inherited && (owner == nil || !owner.Equals(user)) {
		t.Fatalf("owner=%v, want current user %s", owner, user.String())
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PRESENT == 0 || (!inherited && control&windows.SE_DACL_PROTECTED == 0) {
		t.Fatalf("unexpected security descriptor control=0x%x inherited=%t", uint16(control), inherited)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	wantCount := uint16(2)
	if user.Equals(system) {
		wantCount = 1
	}
	if dacl == nil || dacl.AceCount != wantCount {
		t.Fatalf("DACL=%v ACE count=%d want=%d", dacl, func() uint16 {
			if dacl == nil {
				return 0
			}
			return dacl.AceCount
		}(), wantCount)
	}
	seen := map[string]bool{}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			t.Fatal(err)
		}
		if ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != privateConversionDirFullControl {
			t.Fatalf("ACE %d is not exact full-control access-allowed: %+v", index, ace)
		}
		if inherited {
			if ace.Header.AceFlags&windows.INHERITED_ACE == 0 {
				t.Fatalf("child ACE %d is not inherited: flags=0x%x", index, ace.Header.AceFlags)
			}
		} else if ace.Header.AceFlags != uint8(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) {
			t.Fatalf("root ACE %d flags=0x%x", index, ace.Header.AceFlags)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || (!sid.Equals(user) && !sid.Equals(system)) {
			t.Fatalf("ACE %d has unexpected SID %v", index, sid)
		}
		seen[sid.String()] = true
	}
	if !seen[user.String()] || (!user.Equals(system) && !seen[system.String()]) {
		t.Fatalf("DACL is missing user or LocalSystem: %+v", seen)
	}
}
