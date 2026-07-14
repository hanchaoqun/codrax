package hitraceconv

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConversionInputAuthorityStableReaderAndFixedBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stable.sys")
	body := []byte("0123456789")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	if authority.Size() != int64(len(body)) || authority.DisplayPath() == "" || authority.CanonicalPath() == "" {
		t.Fatalf("unexpected authority metadata: %s", authority)
	}
	probe, err := authority.Probe()
	if err != nil || !bytes.Equal(probe, body) {
		t.Fatalf("probe=%q err=%v", probe, err)
	}
	section, err := authority.Section(2, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(section)
	if err != nil || string(got) != "2345" {
		t.Fatalf("section=%q err=%v", got, err)
	}
	buffer := make([]byte, 4)
	n, err := authority.ReadAt(buffer, int64(len(body)-2))
	if n != 2 || !errors.Is(err, io.EOF) || string(buffer[:n]) != "89" {
		t.Fatalf("bounded ReadAt n=%d err=%v body=%q", n, err, buffer[:n])
	}
	if err := authority.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("stable authority rejected: %v", err)
	}
	if err := authority.Validate(conversionInputStage(0)); conversionInputErrorCode(err) != ConversionInputCodeInternalContract {
		t.Fatalf("invalid validation stage was misclassified: %v", err)
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	if err := authority.Validate(conversionInputStagePreCommit); conversionInputErrorCode(err) != ConversionInputCodeClosed {
		t.Fatalf("closed authority error=%v", err)
	}
}

func TestConversionInputAuthorityRejectsNonRegularAndMissingInputs(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
		code ConversionInputErrorCode
	}{
		{name: "empty", path: "", code: ConversionInputCodeInvalidPath},
		{name: "missing", path: filepath.Join(dir, "missing.sys"), code: ConversionInputCodeOpenFailed},
		{name: "directory", path: dir, code: ConversionInputCodeNotRegular},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority, err := openConversionInputAuthority(test.path)
			if authority != nil || conversionInputErrorCode(err) != test.code {
				t.Fatalf("authority=%v err=%v code=%q want=%q", authority, err, conversionInputErrorCode(err), test.code)
			}
		})
	}
}

func TestConversionInputAuthorityDetectsEveryPhysicalMutationProfile(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*testing.T, string, os.FileInfo)
	}{
		{
			name: "same_size_restored_mtime",
			mutate: func(t *testing.T, path string, original os.FileInfo) {
				t.Helper()
				time.Sleep(2 * time.Millisecond)
				if err := os.WriteFile(path, []byte("other-generation\n"), original.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "grow",
			mutate: func(t *testing.T, path string, _ os.FileInfo) {
				t.Helper()
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := file.WriteString("grow")
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("write=%v close=%v", writeErr, closeErr)
				}
			},
		},
		{
			name: "truncate",
			mutate: func(t *testing.T, path string, _ os.FileInfo) {
				t.Helper()
				if err := os.Truncate(path, 4); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "atomic_replace",
			mutate: func(t *testing.T, path string, _ os.FileInfo) {
				t.Helper()
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, []byte("other-generation\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS == "windows" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mutating.sys")
			if err := os.WriteFile(path, []byte("first-generation\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(path)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			original, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			mutation.mutate(t, path, original)
			err = authority.Validate(conversionInputStagePreCommit)
			if conversionInputErrorCode(err) != ConversionInputCodeGenerationChanged {
				t.Fatalf("mutation passed authority: err=%v code=%q", err, conversionInputErrorCode(err))
			}
			var typed *ConversionInputError
			if !errors.As(err, &typed) || typed.Stage != conversionInputStagePreCommit.String() {
				t.Fatalf("mutation lost typed stage: %#v", err)
			}
		})
	}
}

func TestConversionInputAuthorityRejectsSymlinkRetargetAndAcceptsStableHardlink(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.sys")
	second := filepath.Join(dir, "other.sys")
	link := filepath.Join(dir, "input.sys")
	hard := filepath.Join(dir, "hard.sys")
	if err := os.WriteFile(first, []byte("first-generation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("other-generation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, hard); err != nil {
		t.Fatal(err)
	}
	hardAuthority, err := openConversionInputAuthority(hard)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := hardAuthority.Validate(conversionInputStagePreCommit); err != nil {
		t.Fatalf("stable hardlink rejected: %v", err)
	}
	_ = hardAuthority.Close()
	if err := os.Symlink(first, link); err != nil {
		// On Windows a normal test process may lack symlink privilege. The
		// hardlink assertion above still exercises alias identity there.
		if strings.Contains(strings.ToLower(err.Error()), "privilege") || errors.Is(err, os.ErrPermission) {
			return
		}
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(link)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if err := authority.Validate(conversionInputStageRoute); conversionInputErrorCode(err) != ConversionInputCodeGenerationChanged {
		t.Fatalf("symlink retarget passed authority: %v", err)
	}
}

func TestConversionInputAuthorityStageAndCodeDomainsAreClosed(t *testing.T) {
	seen := map[string]bool{}
	for stage := conversionInputStageOpen; stage <= conversionInputStagePreCommit; stage++ {
		if !stage.valid() || stage.String() == "invalid" || seen[stage.String()] {
			t.Fatalf("invalid or duplicate conversion input stage %d=%q", stage, stage.String())
		}
		seen[stage.String()] = true
	}
	if conversionInputStage(0).valid() || conversionInputStage(255).valid() {
		t.Fatal("out-of-domain conversion input stage was admitted")
	}
	knownCodes := []ConversionInputErrorCode{
		ConversionInputCodeInvalidPath,
		ConversionInputCodeOpenFailed,
		ConversionInputCodeNotRegular,
		ConversionInputCodeStrongIdentityUnavailable,
		ConversionInputCodePathBindingFailed,
		ConversionInputCodeGenerationChanged,
		ConversionInputCodeInvalidRange,
		ConversionInputCodeClosed,
		ConversionInputCodeInternalContract,
	}
	seen = map[string]bool{}
	for _, code := range knownCodes {
		if !code.valid() || strings.TrimSpace(string(code)) == "" || seen[string(code)] {
			t.Fatalf("invalid or duplicate conversion input code %q", code)
		}
		seen[string(code)] = true
	}
	if ConversionInputErrorCode("rogue").valid() {
		t.Fatal("unknown conversion input code was admitted")
	}
	rogue := conversionInputFailure(ConversionInputErrorCode("rogue"), conversionInputStageOpen, "input", nil)
	if conversionInputErrorCode(rogue) != ConversionInputCodeInternalContract {
		t.Fatalf("unknown producer code did not fail into the closed contract lane: %v", rogue)
	}
	invalidStage := conversionInputFailure(ConversionInputCodePathBindingFailed, conversionInputStage(255), "input", nil)
	var typed *ConversionInputError
	if !errors.As(invalidStage, &typed) || typed.Code != ConversionInputCodeInternalContract || typed.Stage != "invalid" {
		t.Fatalf("invalid producer stage did not fail into the closed contract lane: %v", invalidStage)
	}
}

func TestConversionInputAuthorityPreservesDisplayPathWithoutExposingItAsIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "display.sys")
	if err := os.WriteFile(path, []byte("stable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	requested := "  " + relative + "  "
	authority, err := openConversionInputAuthority(requested)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	if got, want := authority.DisplayPath(), strings.TrimSpace(requested); got != want {
		t.Fatalf("display path drifted: got=%q want=%q", got, want)
	}
	if sameConversionCanonicalPath(authority.DisplayPath(), authority.CanonicalPath()) {
		t.Fatalf("relative display path was silently replaced by canonical identity: display=%q canonical=%q", authority.DisplayPath(), authority.CanonicalPath())
	}
}

func TestConversionInputAuthorityConcurrentReadersAndCloseStayTyped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.sys")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	start := make(chan struct{})
	errorsSeen := make(chan error, workers*4)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			<-start
			switch worker % 4 {
			case 0:
				_, err := authority.ReadAt(make([]byte, 32), 0)
				errorsSeen <- err
			case 1:
				section, err := authority.Section(0, 32)
				if err == nil {
					_, err = io.ReadAll(section)
				}
				errorsSeen <- err
			case 2:
				_, err := authority.Probe()
				errorsSeen <- err
			default:
				errorsSeen <- authority.Validate(conversionInputStagePreCommit)
			}
		}(worker)
	}
	close(start)
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		wait.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("input authority readers or Close deadlocked")
	}
	close(errorsSeen)
	for err := range errorsSeen {
		if err == nil || errors.Is(err, io.EOF) || conversionInputErrorCode(err) == ConversionInputCodeClosed {
			continue
		}
		t.Fatalf("concurrent authority operation leaked raw or unexpected error: %v", err)
	}
}

func unavailableConversionInputAuthority(t *testing.T, err error) bool {
	t.Helper()
	if conversionInputErrorCode(err) != ConversionInputCodeStrongIdentityUnavailable {
		return false
	}
	var typed *ConversionInputError
	if !errors.As(err, &typed) || typed.Stage != conversionInputStageOpen.String() {
		t.Fatalf("weak platform did not fail closed with typed open error: %v", err)
	}
	return true
}

func conversionInputErrorCode(err error) ConversionInputErrorCode {
	var typed *ConversionInputError
	if errors.As(err, &typed) {
		return typed.Code
	}
	return ConversionInputErrorCode("")
}
