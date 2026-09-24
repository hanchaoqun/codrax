package tool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

const controlledProcessFixtureMode = "CODRAX_CONTROLLED_TEST_PROCESS_MODE"
const controlledProcessFixtureRoot = "CODRAX_CONTROLLED_TEST_PROCESS_ROOT"

// Only the two cancellation/timeout fixtures use this native helper process.
// It avoids freshly written shebang execution latency before the first fixture
// instruction, without bypassing the real command resolver or supervisor.
func TestMain(m *testing.M) {
	if mode := os.Getenv(controlledProcessFixtureMode); mode != "" {
		os.Exit(runControlledProcessFixture(mode))
	}
	os.Exit(m.Run())
}

func runControlledProcessFixture(mode string) int {
	root := os.Getenv(controlledProcessFixtureRoot)
	if !filepath.IsAbs(root) {
		fmt.Fprintln(os.Stderr, "controlled process fixture requires an absolute root")
		return 2
	}
	write := func(name, contents string) bool {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return false
		}
		return true
	}
	binary := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	switch {
	case mode == "python_preparation" && binary == "python3":
		if len(os.Args) != 3 || os.Args[1] != "-c" || os.Args[2] != "import sys" {
			fmt.Fprintf(os.Stderr, "unexpected Python preparation arguments: %q\n", os.Args[1:])
			return 2
		}
		if !write("python-preparation-ready", "ready") {
			return 2
		}
		time.Sleep(5 * time.Second)
		if !write("python-preparation-finished", "finished") {
			return 2
		}
		return 1
	case mode == "python_preparation" && binary == "python":
		if !write("later-python-candidate", "started") {
			return 2
		}
		return 0
	case mode == "cargo_timeout" && binary == "cargo":
		if len(os.Args) != 2 || os.Args[1] != "test" {
			fmt.Fprintf(os.Stderr, "unexpected Cargo fixture arguments: %q\n", os.Args[1:])
			return 2
		}
		wd, err := os.Getwd()
		if err != nil || filepath.Clean(wd) != filepath.Clean(root) {
			fmt.Fprintf(os.Stderr, "unexpected Cargo working directory: %q, %v\n", wd, err)
			return 2
		}
		if !write("Cargo.lock", "v2 refreshed by cargo\n") || !write("junk.out", "junk\n") {
			return 2
		}
		time.Sleep(8 * time.Second)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown controlled process fixture: mode=%q executable=%q\n", mode, binary)
		return 2
	}
}

func installControlledProcessFixture(t *testing.T, mode, root string, names ...string) string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, name := range names {
		path := filepath.Join(bin, name)
		if runtime.GOOS != "windows" {
			if err := os.Symlink(binary, path); err != nil {
				t.Fatal(err)
			}
			continue
		}
		// Windows need not grant symlink privileges to the test account.
		in, err := os.Open(binary)
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.OpenFile(path+".exe", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
		if err != nil {
			_ = in.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("copy controlled process: %v, %v", copyErr, closeErr)
		}
	}
	t.Setenv(controlledProcessFixtureMode, mode)
	t.Setenv(controlledProcessFixtureRoot, root)
	return bin
}

// These callers are deliberately nonparallel: the existing logger is restored
// and the captured candidate/command diagnostics are emitted only on failure.
func captureControlledProcessFailureDiagnostics(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	logger, err := logging.NewFromFlags(dir, "debug", false)
	if err != nil {
		t.Fatal(err)
	}
	previous := logging.Default
	logging.SetDefault(logger)
	t.Cleanup(func() {
		logging.SetDefault(previous)
		_ = logger.Close()
		if !t.Failed() {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Logf("process diagnostic inventory: %v", err)
			return
		}
		for _, entry := range entries {
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			t.Logf("controlled process diagnostics %s (read=%v):\n%s", entry.Name(), err, body)
		}
	})
}
