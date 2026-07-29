package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// PIB-5c (ledger docs/design/pi_borrow_analysis_20260729.md §7.5):
// @path tokens become fs-validated must-read pins.

func TestExtractPinnedFiles_FSValidatedOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "repl"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"internal/repl/input.go", "main.go"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	pins := extractPinnedFiles(root, "看一下 @internal/repl/input.go 和 @main.go, 忽略 @ghost.go 与邮箱 a@b.c 以及 @internal（目录）.")
	if len(pins) != 2 || pins[0] != "internal/repl/input.go" || pins[1] != "main.go" {
		t.Fatalf("pins = %v, want the two existing files in order", pins)
	}

	// Escape and abs attempts never pin.
	for _, hostile := range []string{"@../etc/passwd", "@/etc/passwd", "@a/../../b"} {
		if got := extractPinnedFiles(root, "check "+hostile); len(got) != 0 {
			t.Errorf("hostile token %q must not pin; got %v", hostile, got)
		}
	}

	// Duplicate tokens pin once; trailing punctuation stripped.
	pins = extractPinnedFiles(root, "@main.go, then @main.go.")
	if len(pins) != 1 || pins[0] != "main.go" {
		t.Errorf("dedup/punctuation handling wrong: %v", pins)
	}

	// No @ fast path.
	if got := extractPinnedFiles(root, "plain question"); got != nil {
		t.Errorf("no-@ input must return nil; got %v", got)
	}
}
