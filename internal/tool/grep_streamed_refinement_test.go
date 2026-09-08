package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests must exercise the actual streamed subprocess path. The native
// backend has a separate, already full-output-based refinement path.
func b1624cRequireStreamBackend(t *testing.T) {
	t.Helper()
	if SearchCommand() == "native" {
		t.Skip("streamed grep requires the detected rg/grep executable")
	}
}

func b1624cRuntimeFile(t *testing.T, body string) (*types.BusContext, string) {
	t.Helper()
	dir := t.TempDir()
	const name = "events.log"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{
		RepoRoot: dir,
		WorkDir:  filepath.Join(dir, ".codrax", "blob", "stream-refinement"),
		Mutable:  types.NewMutableState("stream-refinement"),
	}, name
}

// Inspect the real streaming capture, using the same explicit-file filename
// and line-prefix shape as GrepTool. This proves the test's width premise from
// actual output rather than a hand-constructed capture or a guessed byte count.
func b1624cRealCapture(t *testing.T, ctx *types.BusContext, requested, pattern string, fixed bool) runtimeArtifactGrepCapture {
	t.Helper()
	resolved := resolveToolPath(ctx, requested)
	if publication, ok := resolveTraceQueryBlobRefPath(ctx, requested); ok {
		resolved = publication
	}
	commandPath := resolved
	if rel, ok := repoRelativePathWithinRoot(ctx.RepoRoot, resolved); ok {
		commandPath = filepath.FromSlash(rel)
	}
	args := []string{"-nH"}
	if SearchCommand() == "rg" {
		args = append(args, "--color=never", "--no-heading")
	}
	if fixed {
		args = append(args, "-F")
	} else if SearchCommand() == "grep" {
		args = append(args, "-E")
	}
	args = append(args, pattern, commandPath)
	cmd := exec.Command(SearchExecutable(), args...)
	cmd.Dir = ctx.RepoRoot
	capture, err := runRuntimeArtifactGrepStream(ctx, cmd, false)
	t.Cleanup(capture.cleanup)
	if err != nil {
		t.Fatalf("actual stream capture: %v: %s", err, capture.Stderr)
	}
	return capture
}

func b1624cGrep(t *testing.T, ctx *types.BusContext, requested, pattern string, fixed bool) types.ToolResult {
	t.Helper()
	params, err := json.Marshal(map[string]any{"path": requested, "pattern": pattern, "fixed_string": fixed})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&GrepTool{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("actual GrepTool failed: %v %+v", err, result)
	}
	return result
}

func b1624cRequireShortPreview(t *testing.T, capture runtimeArtifactGrepCapture) {
	t.Helper()
	preview := strings.Join(capture.PreviewLines, "\n")
	if len(capture.PreviewLines) != grepWidthLineProductionCap() ||
		len(capture.PreviewLines) >= grepWidthLineEntryThreshold() || len(preview) >= grepWidthByteThreshold() {
		t.Fatalf("test premise: preview itself must fall below both old thresholds: lines=%d bytes=%d caps=%d/%d",
			len(capture.PreviewLines), len(preview), grepWidthLineEntryThreshold(), grepWidthByteThreshold())
	}
}

func b1624cRequireTruncatedHint(t *testing.T, result types.ToolResult, narrowReason string) {
	t.Helper()
	if result.RawRef == "" || !strings.Contains(result.Summary, "decision=broad_result_compacted") {
		t.Fatalf("actual stream did not preserve the compacted result: %+v", result)
	}
	hint := result.Refinement
	if hint == nil || !hint.ResultTruncated || hint.ReasonCode != "grep_result_truncated" {
		t.Fatalf("full streamed output was compacted, but its short preview hid the typed truncation/refinement: %+v", hint)
	}
	if hint.PreferredNextTool != "grep" {
		t.Fatalf("runtime result search must remain a bounded read, got %+v", hint)
	}
	if narrowReason != "" {
		found := false
		for _, suggestion := range hint.ParamNarrowingSuggestions {
			if suggestion.ReasonCode == narrowReason {
				found = true
			}
		}
		if !found {
			t.Fatalf("refinement must report the actual full-output trigger %q: %+v", narrowReason, hint)
		}
	}
}

func TestB1624cStreamedGrepUsesFullOutputWidthNotPreview(t *testing.T) {
	b1624cRequireStreamBackend(t)
	for _, shape := range []struct {
		name       string
		lines      int
		longAfter  int
		wantReason string
	}{
		{"line-count", 120, 120, types.ToolParamNarrowReasonEntriesOverThreshold},
		{"byte-count", 60, 48, types.ToolParamNarrowReasonByteBudgetExceeded},
	} {
		t.Run(shape.name, func(t *testing.T) {
			var source strings.Builder
			for i := 0; i < shape.lines; i++ {
				fmt.Fprintf(&source, "hit %03d", i)
				if i >= shape.longAfter {
					source.WriteString(strings.Repeat("x", 2048))
				}
				source.WriteByte('\n')
			}
			ctx, path := b1624cRuntimeFile(t, source.String())
			capture := b1624cRealCapture(t, ctx, path, "hit", true)
			if capture.Lines != shape.lines {
				t.Fatalf("actual output count=%d want %d", capture.Lines, shape.lines)
			}
			b1624cRequireShortPreview(t, capture)
			if shape.wantReason == types.ToolParamNarrowReasonEntriesOverThreshold {
				if capture.Lines <= grepWidthLineEntryThreshold() || capture.Bytes >= grepWidthByteThreshold() {
					t.Fatalf("line-only trigger was not isolated: lines=%d bytes=%d", capture.Lines, capture.Bytes)
				}
			} else if capture.Lines >= grepWidthLineEntryThreshold() || capture.Bytes <= grepWidthByteThreshold() || capture.FullInMemory {
				t.Fatalf("byte-only trigger was not isolated: %+v", capture)
			}
			result := b1624cGrep(t, ctx, path, "hit", true)
			// Full late data must remain available even though it is omitted
			// from inline preview. This repair must not change the row budget.
			last := fmt.Sprintf("hit %03d", shape.lines-1)
			full, err := os.ReadFile(result.RawRef)
			if err != nil || !strings.Contains(string(full), last) || strings.Contains(result.Summary, last) {
				t.Fatalf("raw preservation or bounded preview drifted: err=%v summary=%s", err, result.Summary)
			}
			b1624cRequireTruncatedHint(t, result, shape.wantReason)
		})
	}
}

func TestB1624cSmallAndZeroStreamedGrepDoNotInventTruncation(t *testing.T) {
	b1624cRequireStreamBackend(t)
	ctx, path := b1624cRuntimeFile(t, "hit one\nhit two\n")
	for _, pattern := range []string{"hit", "absent"} {
		t.Run(pattern, func(t *testing.T) {
			result := b1624cGrep(t, ctx, path, pattern, true)
			if strings.Contains(result.Summary, "decision=broad_result_compacted") ||
				(result.Refinement != nil && result.Refinement.ResultTruncated) {
				t.Fatalf("small/zero output acquired false truncation: %+v", result)
			}
			if pattern == "hit" && !strings.Contains(result.Summary, "hit two") {
				t.Fatal("small successful read lost its data")
			}
			if pattern == "absent" && !strings.Contains(result.Summary, "no matches found") {
				t.Fatal("zero-match observation lost")
			}
		})
	}
}

func TestB1624cPublishedQueryStreamRefinementKeepsOriginalReadAuthority(t *testing.T) {
	b1624cRequireStreamBackend(t)
	ctx, _, refs := b1624PublishedResult(t)
	registered := ctx.Mutable.TraceQueryBlobRefs()
	for _, ref := range refs {
		t.Run(filepath.Ext(ref), func(t *testing.T) {
			original, err := os.ReadFile(ref)
			if err != nil {
				t.Fatal(err)
			}
			capture := b1624cRealCapture(t, ctx, ref, ".", false)
			if capture.Lines <= grepWidthLineEntryThreshold() && capture.Bytes <= grepWidthByteThreshold() {
				t.Fatal("published-result fixture did not exercise a broad stream")
			}
			result := b1624cGrep(t, ctx, ref, ".", false)
			b1624AssertResultNavigation(t, result)
			b1624cRequireTruncatedHint(t, result, "")
			path := result.Refinement.PreferredParams["path"]
			resolved, ok := resolveTraceQueryBlobRefPath(ctx, path)
			if !ok || resolved != ref {
				t.Fatalf("refinement borrowed the derived blob instead of the original authorized query result: %+v", result.Refinement)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(result.RawRef); ok {
				t.Fatal("width refinement must not grant derived grep output a new trace-query escape permission")
			}
			if !reflect.DeepEqual(registered, ctx.Mutable.TraceQueryBlobRefs()) {
				t.Fatal("reading changed the original publication registry")
			}
			after, err := os.ReadFile(ref)
			if err != nil || string(after) != string(original) {
				t.Fatal("reading changed the original published result bytes")
			}
		})
	}
}
