package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func dispatchReadVersionSHA(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
}

func dispatchReadVersionComplete(t *testing.T, ctx *types.BusContext, path, body string) bool {
	t.Helper()
	root, err := filepath.EvalSymlinks(ctx.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	return ctx.Mutable.CompleteDispatchRepositoryFileReadVersion(root, path, dispatchReadVersionSHA(body))
}

func dispatchReadVersionPublish(t *testing.T, ctx *types.BusContext, path string, offset, limit int) types.ToolResult {
	t.Helper()
	r := physicalRead(t, ctx, path, offset, limit)
	if !r.Success {
		t.Fatalf("real read failed: %s", r.Summary)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	return r
}

func dispatchReadVersionLines(prefix string, count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "%s-%04d\n", prefix, i)
	}
	return b.String()
}

func TestDispatchRepositoryReadVersionPublicModesAndBytes(t *testing.T) {
	for _, mode := range []types.PipelineMode{types.ModeRead, types.ModePlan, types.ModeApply, types.ModeVerify} {
		t.Run(string(mode), func(t *testing.T) {
			body := "first\nlast\n"
			ctx := physicalReadFixture(t, "tests/check.py", body)
			ctx.Mode = mode
			dispatchReadVersionPublish(t, ctx, "tests/check.py", 0, 0)
			if got := dispatchReadVersionComplete(t, ctx, "tests/check.py", body); got != mode.IsWrite() {
				t.Fatalf("complete=%v, write mode=%v", got, mode.IsWrite())
			}
			if ctx.Mutable.ChangePlan() != nil || ctx.Mutable.ChangeReport() != nil {
				t.Fatal("reading must not produce a plan or verification report")
			}
		})
	}
	for name, body := range map[string]string{
		"empty": "", "lf": "first\nlast\n", "no_terminal_lf": "first\nlast",
		"crlf": "first\r\nlast\r\n", "real_blank_line": "first\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := physicalReadFixture(t, "tests/check.py", body)
			ctx.Mode = types.ModePlan
			dispatchReadVersionPublish(t, ctx, "tests/check.py", 0, 0)
			if !dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
				t.Fatal("complete exact-byte source read was not retained")
			}
			if dispatchReadVersionComplete(t, ctx, "tests/check.py", body+"\n") {
				t.Fatal("different terminal bytes borrowed the old read")
			}
			actual, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "tests/check.py"))
			if err != nil || string(actual) != body {
				t.Fatal("read modified original bytes")
			}
		})
	}
}

func TestDispatchRepositoryReadVersionPublicPaging(t *testing.T) {
	// Above the small-limit auto-expansion threshold: these are real pages,
	// not requests that the existing reader deliberately expands to the file.
	page := ReadFileSmallLimitThreshold + 1
	if page < 2 {
		page = 2
	}
	body := dispatchReadVersionLines("old", 3*page)
	for _, kind := range []string{"gap_then_fill", "cross_version", "read_then_changed"} {
		t.Run(kind, func(t *testing.T) {
			ctx := physicalReadFixture(t, "tests/check.py", body)
			ctx.Mode = types.ModePlan
			first := dispatchReadVersionPublish(t, ctx, "tests/check.py", 0, page)
			if first.ReadCoverage == nil || first.ReadCoverage.LineEnd != page || dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
				t.Fatal("fixture must publish only the first page")
			}
			switch kind {
			case "gap_then_fill":
				dispatchReadVersionPublish(t, ctx, "tests/check.py", 2*page, page)
				if dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
					t.Fatal("head and tail concealed an unread middle")
				}
				dispatchReadVersionPublish(t, ctx, "tests/check.py", page, page)
				if !dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
					t.Fatal("same-version contiguous pages did not complete coverage")
				}
			case "cross_version":
				changed := dispatchReadVersionLines("new", 3*page)
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "tests/check.py"), []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
				dispatchReadVersionPublish(t, ctx, "tests/check.py", page, 2*page)
				if dispatchReadVersionComplete(t, ctx, "tests/check.py", body) || dispatchReadVersionComplete(t, ctx, "tests/check.py", changed) {
					t.Fatal("pages from different byte versions were combined")
				}
				dispatchReadVersionPublish(t, ctx, "tests/check.py", 0, page)
				if !dispatchReadVersionComplete(t, ctx, "tests/check.py", changed) {
					t.Fatal("complete reread of the new version was not retained")
				}
			case "read_then_changed":
				dispatchReadVersionPublish(t, ctx, "tests/check.py", page, 2*page)
				if !dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
					t.Fatal("old full read missing before mutation")
				}
				changed := body + "later bytes\n"
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "tests/check.py"), []byte(changed), 0600); err != nil {
					t.Fatal(err)
				}
				if dispatchReadVersionComplete(t, ctx, "tests/check.py", changed) {
					t.Fatal("new file hash borrowed earlier full-read coverage")
				}
			}
		})
	}
}

func TestDispatchRepositoryReadVersionPublicIdentityAndDispatch(t *testing.T) {
	for _, kind := range []string{"other_repository", "outside_symlink", "runtime", "no_dispatch_result", "reset_and_replay"} {
		t.Run(kind, func(t *testing.T) {
			const path, body = "tests/check.py", "first\nlast\n"
			ctx := physicalReadFixture(t, path, body)
			ctx.Mode = types.ModePlan
			readCtx, readPath := ctx, path
			switch kind {
			case "other_repository", "outside_symlink":
				other := physicalReadFixture(t, path, body)
				other.Mode, other.Mutable = types.ModePlan, ctx.Mutable
				if kind == "other_repository" {
					readCtx = other
				} else {
					readPath = "borrowed.py"
					if err := os.Symlink(filepath.Join(other.RepoRoot, path), filepath.Join(ctx.RepoRoot, readPath)); err != nil {
						t.Fatal(err)
					}
				}
			case "runtime":
				readPath = ".codrax/result.txt"
				if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, ".codrax"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, readPath), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := physicalRead(t, readCtx, readPath, 0, 0)
			if !r.Success {
				t.Fatalf("read fixture failed: %s", r.Summary)
			}
			if kind == "runtime" && r.RuntimeArtifactRead == nil {
				t.Fatal("fixture did not exercise runtime/source separation")
			}
			if kind != "no_dispatch_result" {
				ctx.Mutable.AppendDispatchToolResult(r)
			}
			if kind == "reset_and_replay" {
				if !dispatchReadVersionComplete(t, ctx, path, body) {
					t.Fatal("fresh read missing before reset")
				}
				ctx.Mutable.ResetDispatchToolResults()
				ctx.Mutable.AppendDispatchToolResult(r)
			}
			if dispatchReadVersionComplete(t, ctx, readPath, body) {
				t.Fatalf("%s granted current-repository/current-dispatch version coverage", kind)
			}
			if kind == "reset_and_replay" {
				dispatchReadVersionPublish(t, ctx, path, 0, 0)
				if !dispatchReadVersionComplete(t, ctx, path, body) {
					t.Fatal("fresh dispatch reread did not restore version coverage")
				}
			}
		})
	}
}

func TestDispatchRepositoryReadVersionPublicPreview(t *testing.T) {
	for _, kind := range []string{"paged_preview", "one_truncated_line"} {
		t.Run(kind, func(t *testing.T) {
			body := strings.Repeat(strings.Repeat("x", 99)+"\n", MaxInlineBytes/100+200)
			if kind == "one_truncated_line" {
				body = strings.Repeat("x", MaxInlineBytes+PreviewHeadBytesValue()+100)
			}
			ctx := physicalReadFixture(t, "tests/check.py", body)
			ctx.Mode = types.ModePlan
			r := dispatchReadVersionPublish(t, ctx, "tests/check.py", 0, 0)
			if r.ReadCoverage == nil || dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
				t.Fatal("preview or partial line granted a complete file-version read")
			}
			if kind == "paged_preview" {
				if r.ReadCoverage.LineEnd >= r.ReadCoverage.TotalLines {
					t.Fatal("fixture did not exercise bounded preview")
				}
				for end := r.ReadCoverage.LineEnd; end < r.ReadCoverage.TotalLines; {
					next := dispatchReadVersionPublish(t, ctx, "tests/check.py", end, 100)
					if next.ReadCoverage == nil || next.ReadCoverage.LineEnd <= end {
						t.Fatal("continuation made no progress")
					}
					end = next.ReadCoverage.LineEnd
				}
				if !dispatchReadVersionComplete(t, ctx, "tests/check.py", body) {
					t.Fatal("visible continuation pages failed to complete a large file")
				}
			}
		})
	}
}

func TestDispatchRepositoryReadVersionPublicInvalidUTF8(t *testing.T) {
	const path, body = "tests/check.py", "first\ninvalid:\xff\nlast\n"
	ctx := physicalReadFixture(t, path, body)
	ctx.Mode = types.ModePlan
	r := dispatchReadVersionPublish(t, ctx, path, 0, 0)
	if r.ReadCoverage == nil || r.ReadCoverage.LineStart != 1 || r.ReadCoverage.LineEnd != 3 || r.ReadCoverage.TotalLines != 3 {
		t.Fatalf("ordinary successful read coverage changed: %+v", r.ReadCoverage)
	}
	encoded, err := json.Marshal(r.Summary)
	if err != nil {
		t.Fatal(err)
	}
	var transported string
	if err := json.Unmarshal(encoded, &transported); err != nil {
		t.Fatal(err)
	}
	if transported == r.Summary || !strings.ContainsRune(transported, '\ufffd') {
		t.Fatal("fixture did not exercise JSON replacement of invalid source bytes")
	}
	actual, err := os.ReadFile(filepath.Join(ctx.RepoRoot, path))
	if err != nil || string(actual) != body {
		t.Fatal("successful read must preserve the original invalid bytes")
	}
	if dispatchReadVersionComplete(t, ctx, path, body) {
		t.Fatal("JSON-replaced text granted complete inspection of the original byte version")
	}
}
