package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// eval_log_tag_contract_test.go — eval 批 6 A2 (2026-06-11).
//
// Eval case specs assert internal log lines via EXPECT_LOG_MATCHES_REGEX,
// and those regexes hardcode log tags like `\[cmd/route\]` as free-form
// strings with no tether to the emitting code. A tag rename in source
// silently breaks the spec: c4a9eae8 renamed `[cmd/operation]` →
// `[cmd/route]` and two case specs kept failing invisibly for six days
// until a live eval run hit them.
//
// This test is the tether: every literal `[tag]` referenced by an
// EXPECT_LOG_*_REGEX block in eval/cases/*.case must appear verbatim in
// some Go source file under cmd/ or internal/. Renaming a logged tag now
// fails `go test` until the case specs move with it.

// caseLogTagPattern matches the escaped tag form the specs use inside
// shell-quoted EREs: `\\[cmd/route\\]`. Wildcard tags (e.g. `\\[.*data\\]`)
// contain regex metacharacters and are skipped — they cannot drift the
// same way a literal can.
var caseLogTagPattern = regexp.MustCompile(`\\\\\[([a-zA-Z0-9/_.-]+)\\\\\]`)

func TestEvalCaseLogTagsExistInSource(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(currentFile))

	cases, err := filepath.Glob(filepath.Join(repoRoot, "eval", "cases", "*.case"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("glob eval/cases: err=%v matches=%d", err, len(cases))
	}

	// tag → first case file referencing it
	tags := map[string]string{}
	for _, cf := range cases {
		data, err := os.ReadFile(cf)
		if err != nil {
			t.Fatalf("read %s: %v", cf, err)
		}
		for _, m := range caseLogTagPattern.FindAllStringSubmatch(string(data), -1) {
			tag := m[1]
			if strings.ContainsAny(tag, "*?+()|") {
				continue
			}
			if _, seen := tags[tag]; !seen {
				tags[tag] = filepath.Base(cf)
			}
		}
	}
	if len(tags) == 0 {
		t.Fatal("no literal log tags extracted from case specs — extraction pattern drifted?")
	}

	// Collect all Go source bytes once (cmd/ + internal/), excluding tests:
	// the tag must be backed by a production emit site, not a test fixture.
	var source strings.Builder
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.Walk(filepath.Join(repoRoot, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			source.Write(data)
			source.WriteByte('\n')
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	src := source.String()
	for tag, from := range tags {
		if !strings.Contains(src, "["+tag+"]") {
			t.Errorf("eval case spec %s expects log tag %q but no non-test Go source under cmd/ or internal/ emits it — if the tag was renamed, update the case spec(s) referencing it", from, "["+tag+"]")
		}
	}
}
