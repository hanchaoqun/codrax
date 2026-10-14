package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestREPLStdinFileAndPipeRetainBorrowedReader(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "commands.txt")
		if err := os.WriteFile(path, []byte("/help\n/exit\n"), 0600); err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = in.Close() })
		got := replInputForStdin(in)
		if got != in {
			t.Fatal("redirected file must reach existing script input owner unchanged")
		}
		body, err := io.ReadAll(got)
		if err != nil || string(body) != "/help\n/exit\n" {
			t.Fatalf("stdin handoff consumed or rewrote command bytes: %q %v", body, err)
		}
		if _, err := in.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("stdin handoff closed borrowed file: %v", err)
		}
	})
	t.Run("pipe", func(t *testing.T) {
		in, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = in.Close(); _ = writer.Close() })
		got := replInputForStdin(in)
		if got != in {
			t.Fatal("piped stdin must not select the interactive /dev/tty reader")
		}
		if _, err := writer.WriteString("/exit\n"); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(got)
		if err != nil || string(body) != "/exit\n" {
			t.Fatalf("pipe bytes/EOF changed: %q %v", body, err)
		}
	})
}

func TestREPLStdinTTYAndNilKeepInteractiveOwnership(t *testing.T) {
	if got := replInputForStdin(nil); got != nil {
		t.Fatal("missing stdin became a non-nil typed reader")
	}
	if got := replInputForStdinWithTerminalCheck(nil, func(int) bool {
		t.Fatal("nil stdin must not probe an invalid descriptor")
		return false
	}); got != nil {
		t.Fatal("nil stdin lost native interactive default")
	}
	in, err := os.CreateTemp(t.TempDir(), "terminal-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	calls := 0
	got := replInputForStdinWithTerminalCheck(in, func(fd int) bool {
		calls++
		if fd != int(in.Fd()) {
			t.Fatal("TTY selection probed another stream")
		}
		return true
	})
	if got != nil || calls != 1 {
		t.Fatalf("TTY must leave Config.In nil for native input: %v calls=%d", got, calls)
	}
	if _, err := in.WriteString("still open"); err != nil {
		t.Fatalf("TTY decision closed the caller's descriptor: %v", err)
	}
}

func TestREPLStdinClosedReaderPreservesReadError(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "closed-stdin-")
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	got := replInputForStdin(in)
	if got != in {
		t.Fatal("unavailable non-TTY descriptor must not silently become interactive")
	}
	if _, err := got.Read(make([]byte, 1)); err == nil || err == io.EOF {
		t.Fatalf("closed reader must preserve its error, not success/EOF: %v", err)
	}
}
