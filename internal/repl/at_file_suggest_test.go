package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// TTY-3b @-file completion pins (design ledger
// docs/design/tty_single_stdin_owner_20260729.md §5).

func TestAtFileSuggestions_ScoringAndBoundaries(t *testing.T) {
	index := []string{
		"internal/render/dock.go",
		"internal/render/dock_state.go",
		"internal/repl/input.go",
		"docs/dock-notes.md",
	}
	// Basename prefix beats path-contains; whole-value replacement
	// keeps the text before the token.
	got := atFileSuggestions("看一下 @dock", index)
	if len(got) < 3 || got[0].display != "@internal/render/dock.go" {
		t.Fatalf("scoring order wrong: %+v", got)
	}
	if got[0].insert != "看一下 @internal/render/dock.go" {
		t.Fatalf("insert must replace only the trailing token: %q", got[0].insert)
	}

	// Boundaries: bare "@", no token, mid-word @ (email) never suggest.
	for _, value := range []string{"@", "plain text", "mail a@b", "a@@x"} {
		if s := atFileSuggestions(value, index); len(s) != 0 {
			t.Errorf("value %q must not suggest; got %d", value, len(s))
		}
	}

	// Cap respected.
	big := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		big = append(big, filepath.Join("pkg", "file"+string(rune('a'+i%26))+".go"))
	}
	if s := atFileSuggestions("@file", big); len(s) > atFileSuggestMax {
		t.Errorf("suggestions exceed cap: %d", len(s))
	}
}

func TestBuildAtFileIndex_SkipsHiddenAndVendor(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk("main.go")
	mk("internal/a.go")
	mk(".git/config")
	mk("node_modules/dep/index.js")
	mk(".codrax/logs/x.log")

	index := buildAtFileIndex(root)
	want := map[string]bool{"main.go": true, "internal/a.go": true}
	if len(index) != len(want) {
		t.Fatalf("index = %v, want exactly %v", index, want)
	}
	for _, f := range index {
		if !want[f] {
			t.Errorf("unexpected entry %q (hidden/vendor must be skipped)", f)
		}
	}
}

// TestNativeEditor_EnterDoesNotHijackFileSuggestion pins REGFIX-D #18:
// Enter may auto-accept a SLASH suggestion, never an @-file one — the
// file panel is open whenever the line ends in an @token, so Enter used
// to rewrite the user's path to the top-scored match and submit that.
func TestNativeEditor_EnterDoesNotHijackFileSuggestion(t *testing.T) {
	fileLane := &nativeLineInput{value: []rune("看下 @internal/repl/inp"), showSuggest: true}
	if fileLane.suggestionsAreFileCompletions() != true {
		t.Fatal("a non-slash line with a visible panel is the file lane")
	}
	slashLane := &nativeLineInput{value: []rune("/hist"), showSuggest: true}
	if slashLane.suggestionsAreFileCompletions() {
		t.Fatal("a slash line must stay on the command lane (Enter accepts)")
	}
}
