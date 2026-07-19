package cmd

// evalcase_mt_attach_pin_test.go — EVALCASE-DH batch, MT-1 guard pin
// (mining ledger evalcase_xa_cmp_mining.md §3 MT-P1 oracle): repeating
// --htrace/--atrace with multiple physical captures is refused BEFORE any
// LLM work with the exact actionable string — flattening independent clocks
// would fabricate a causal timeline. Pinned VERBATIM (the substring pin in
// multi_attach_test.go stays; this is the promise-face string itself) so a
// future multi-trace intake redesign must consciously rewrite the sentence.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvalcaseMT1MultiTraceFlattenRefusalVerbatim(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.systrace")
	second := filepath.Join(dir, "second.ftrace")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("sched_switch: prev_pid=1 next_pid=2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := loadMultiPathSlice("trace", []string{first, second}, "", 1<<20)
	if err == nil {
		t.Fatal("MT-1: two physical trace attachments must be refused")
	}
	const want = "multiple physical trace attachments cannot be flattened into one causal timeline; name each path in the question or use a provenance-carrying .tracebundle.json"
	if err.Error() != want {
		t.Fatalf("MT-1: refusal string drifted:\n got %q\nwant %q", err.Error(), want)
	}
	// The single-attachment arm stays open (the refusal is about plurality,
	// never about the channel).
	if _, err := loadMultiPathSlice("trace", []string{first}, "", 1<<20); err != nil {
		t.Fatalf("MT-1: single attachment must load: %v", err)
	}
}
