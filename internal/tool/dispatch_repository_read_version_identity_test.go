package tool

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

const dispatchIdentityPath = "tests/check.py"
const dispatchIdentityBody = "first\nlast\n"

func dispatchIdentityWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func dispatchIdentityFixture(t *testing.T) (*types.BusContext, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	path := filepath.Join(root, filepath.FromSlash(dispatchIdentityPath))
	dispatchIdentityWrite(t, path, dispatchIdentityBody)
	ctx := newTestBusCtx()
	ctx.RepoRoot, ctx.Mode = root, types.ModePlan
	return ctx, path
}

func dispatchIdentityResult(path string) types.ToolResult {
	return types.ToolResult{ToolName: "read_file", Success: true, Summary: "unchanged visible output", RawRef: "current-read",
		ReadCoverage: &types.ToolReadCoverage{Path: path, LineStart: 1, LineEnd: 2, TotalLines: 2, RawRef: "current-read"}}
}

// These are actual reader -> pending receipt -> record lifecycle checks, not
// a claim to exercise the public planner or grant test-registration authority.
func TestDispatchRepositoryReadVersionIdentityLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"unchanged", true}, {"unrelated_append", true}, {"directory_blob_added", true},
		{"reset_then_reregister", false}, {"mutable_replaced", false}, {"file_replaced_same_bytes", false},
		{"mtime_changed", false}, {"size_changed", false}, {"file_to_internal_link", false},
		{"file_to_external_link", false}, {"alias_to_internal_link", false}, {"alias_to_external_link", false},
		{"root_replaced_same_bytes", false}, {"mode_changed", false}, {"display_incomplete", false},
		{"wrong_coverage_path", false}, {"wrong_total_lines", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := dispatchIdentityFixture(t)
			requested := dispatchIdentityPath
			if tc.name == "alias_to_internal_link" || tc.name == "alias_to_external_link" {
				requested = "alias.py"
				alias := filepath.Join(ctx.RepoRoot, requested)
				if err := os.Symlink(path, alias); err != nil {
					t.Fatal(err)
				}
				path = alias
			}
			data, pending, err := readRepositoryFileVersion(ctx, ctx.RepoRoot, requested, path, 1024)
			if err != nil || string(data) != dispatchIdentityBody || pending.mutable == nil {
				t.Fatalf("real eligible read: data=%q pending=%+v err=%v", data, pending, err)
			}
			root, digest, originalMutable := pending.physicalRoot, pending.digest, ctx.Mutable
			if digest != fmt.Sprintf("%x", sha256.Sum256(data)) {
				t.Fatal("receipt digest differs from actual returned bytes")
			}
			result := dispatchIdentityResult(requested)
			displayComplete := true
			switch tc.name {
			case "unrelated_append":
				ctx.Mutable.AppendDispatchToolResult(types.ToolResult{ToolName: "other", Success: true})
			case "directory_blob_added":
				dispatchIdentityWrite(t, filepath.Join(ctx.RepoRoot, "visible-blob.txt"), "unrelated")
			case "reset_then_reregister":
				ctx.Mutable.ResetDispatchToolResults()
			case "mutable_replaced":
				ctx.Mutable = types.NewMutableState("other dispatch owner")
			case "file_replaced_same_bytes":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				dispatchIdentityWrite(t, path, dispatchIdentityBody)
			case "mtime_changed":
				changed := pending.fileInfo.ModTime().Add(2 * time.Second)
				if err := os.Chtimes(path, changed, changed); err != nil {
					t.Fatal(err)
				}
			case "size_changed":
				dispatchIdentityWrite(t, path, dispatchIdentityBody+"more\n")
			case "file_to_internal_link", "file_to_external_link", "alias_to_internal_link", "alias_to_external_link":
				target := filepath.Join(ctx.RepoRoot, "replacement.py")
				if tc.name == "file_to_external_link" || tc.name == "alias_to_external_link" {
					target = filepath.Join(t.TempDir(), "replacement.py")
				}
				dispatchIdentityWrite(t, target, dispatchIdentityBody)
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "root_replaced_same_bytes":
				if err := os.Rename(ctx.RepoRoot, ctx.RepoRoot+".old"); err != nil {
					t.Fatal(err)
				}
				dispatchIdentityWrite(t, path, dispatchIdentityBody)
			case "mode_changed":
				ctx.Mode = types.ModeRead
			case "display_incomplete":
				displayComplete = false
			case "wrong_coverage_path":
				result.ReadCoverage.Path = "tests/other.py"
			case "wrong_total_lines":
				result.ReadCoverage.LineEnd, result.ReadCoverage.TotalLines = 1, 1
			}
			// Reproduce the producer ordering, including a new-generation legacy
			// inspection entry. That entry must not revive an old pending version.
			recordSuccessfulRepositoryRead(ctx, ctx.RepoRoot, path, result)
			before := result
			coverageCopy := *result.ReadCoverage
			before.ReadCoverage = &coverageCopy
			pending.record(ctx, result, displayComplete)
			if !reflect.DeepEqual(before, result) {
				t.Fatal("receipt collection mutated the existing tool result")
			}
			if ctx.Mutable.CompleteDispatchRepositoryFileReadVersion(root, result.ReadCoverage.Path, digest) {
				t.Fatal("a pending read qualified before the visible result was appended")
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			if got := ctx.Mutable.CompleteDispatchRepositoryFileReadVersion(root, result.ReadCoverage.Path, digest); got != tc.want {
				t.Fatalf("complete=%t, want %t after %s", got, tc.want, tc.name)
			}
			if originalMutable != ctx.Mutable && originalMutable.CompleteDispatchRepositoryFileReadVersion(root, requested, digest) {
				t.Fatal("changed mutable owner acquired a receipt on its predecessor")
			}
		})
	}
}

func TestDispatchRepositoryReadVersionIdentityEligibilityAndCap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cap      int64
		eligible bool
		oversize bool
	}{
		{"at_cap", int64(len(dispatchIdentityBody)), true, false},
		{"over_cap", int64(len(dispatchIdentityBody) - 1), false, true},
		{"default_cap", 0, true, false}, {"max_int_cap", math.MaxInt64, true, false},
		{"read_mode", 1024, false, false}, {"outside_link", 1024, false, false},
		{"runtime_path", 1024, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := dispatchIdentityFixture(t)
			requested := dispatchIdentityPath
			switch tc.name {
			case "read_mode":
				ctx.Mode = types.ModeRead
			case "outside_link":
				external := filepath.Join(t.TempDir(), "check.py")
				dispatchIdentityWrite(t, external, dispatchIdentityBody)
				requested, path = "alias.py", filepath.Join(ctx.RepoRoot, "alias.py")
				if err := os.Symlink(external, path); err != nil {
					t.Fatal(err)
				}
			case "runtime_path":
				requested, path = ".codrax/output.txt", filepath.Join(ctx.RepoRoot, ".codrax/output.txt")
				dispatchIdentityWrite(t, path, dispatchIdentityBody)
			}
			data, pending, err := readRepositoryFileVersion(ctx, ctx.RepoRoot, requested, path, tc.cap)
			var oversized *width.ErrSourceReadOversized
			if errors.As(err, &oversized) != tc.oversize || !tc.oversize && err != nil {
				t.Fatalf("unexpected read error: %v", err)
			}
			if !tc.oversize && string(data) != dispatchIdentityBody {
				t.Fatalf("read behavior changed: %q", data)
			}
			if got := pending.mutable != nil && pending.digest != ""; got != tc.eligible {
				t.Fatalf("version eligibility=%t, want %t", got, tc.eligible)
			}
			result := dispatchIdentityResult(requested)
			recordSuccessfulRepositoryRead(ctx, ctx.RepoRoot, path, result)
			pending.record(ctx, result, true)
			ctx.Mutable.AppendDispatchToolResult(result)
			root := repositoryReadPhysicalIdentity(ctx.RepoRoot)
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(dispatchIdentityBody)))
			if got := ctx.Mutable.CompleteDispatchRepositoryFileReadVersion(root, requested, digest); got != tc.eligible {
				t.Fatalf("completed receipt=%t, want %t", got, tc.eligible)
			}
		})
	}
}
