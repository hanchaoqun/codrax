package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestValidatePlanFullContent_RejectsPatchWithNewContentCarrier(t *testing.T) {
	ctx := &types.BusContext{RepoRoot: t.TempDir()}
	changes := []types.FileChange{{
		Path:       "widget.py",
		Kind:       "patch",
		NewContent: "print('not a diff')\n",
		Rationale:  "bad carrier",
	}}

	rej := validatePlanFullContent(ctx, "summary", changes, nil)
	if rej == "" {
		t.Fatal("kind=patch with new_content must be rejected before apply/export")
	}
	if !strings.Contains(rej, "kind=patch") || !strings.Contains(rej, "new_content") {
		t.Fatalf("rejection should name the carrier mismatch, got %q", rej)
	}
}

func TestValidatePlanFullContent_RejectsLargeModifyPrefixTruncation(t *testing.T) {
	repo := t.TempDir()
	seed := numberedLines(900)
	if err := os.WriteFile(filepath.Join(repo, "huge.py"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed huge.py: %v", err)
	}
	ctx := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "huge.py",
		Kind:       "modify",
		NewContent: strings.Join(strings.Split(seed, "\n")[:120], "\n") + "\n",
		Rationale:  "truncated file body",
	}}

	rej := validatePlanFullContent(ctx, "summary", changes, nil)
	if rej == "" {
		t.Fatal("large full-file modify that is a strict prefix must be rejected")
	}
	if !strings.Contains(rej, "truncated") || !strings.Contains(rej, "kind=patch") {
		t.Fatalf("rejection should steer to surgical patch, got %q", rej)
	}
}

func TestValidatePlanFullContent_RejectsLargeModifyCatastrophicShrink(t *testing.T) {
	repo := t.TempDir()
	seed := numberedLines(900)
	if err := os.WriteFile(filepath.Join(repo, "huge.py"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed huge.py: %v", err)
	}
	ctx := &types.BusContext{RepoRoot: repo}
	changes := []types.FileChange{{
		Path:       "huge.py",
		Kind:       "modify",
		NewContent: numberedLinesWithPrefix("rewritten", 160),
		Rationale:  "partial rewrite",
	}}

	rej := validatePlanFullContent(ctx, "summary", changes, nil)
	if rej == "" {
		t.Fatal("large full-file modify with catastrophic net deletion must be rejected")
	}
	if !strings.Contains(rej, "removes most of the file") {
		t.Fatalf("rejection should identify destructive shrink, got %q", rej)
	}
}

func TestEmitPlanChange_RejectsLargeModifyTruncationBeforeFinalize(t *testing.T) {
	repo := t.TempDir()
	seed := numberedLines(900)
	if err := os.WriteFile(filepath.Join(repo, "huge.py"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed huge.py: %v", err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("test"),
	}

	skeleton := &EmitPlanSkeleton{}
	skelParams := json.RawMessage(`{
		"request": "change a large file",
		"summary": "Modify one large Python file using a full-file body.",
		"changes": [{"path":"huge.py","kind":"modify","rationale":"change a localized behavior"}]
	}`)
	res, err := skeleton.Execute(ctx, skelParams)
	if err != nil {
		t.Fatalf("skeleton execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("skeleton rejected unexpectedly: %s", res.Summary)
	}

	change := &EmitPlanChange{}
	truncated := strings.Join(strings.Split(seed, "\n")[:120], "\n") + "\n"
	payload, err := json.Marshal(map[string]string{
		"path":        "huge.py",
		"new_content": truncated,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	res, err = change.Execute(ctx, payload)
	if err != nil {
		t.Fatalf("change execute: %v", err)
	}
	if res.Success {
		t.Fatalf("finalize should reject truncated full-file body")
	}
	if ctx.Mutable.ChangePlan() != nil {
		t.Fatal("rejected partial plan must not promote to ChangePlan")
	}
	if !strings.Contains(res.Summary, "truncated") {
		t.Fatalf("rejection should mention truncation, got %s", res.Summary)
	}
}

func numberedLines(n int) string {
	return numberedLinesWithPrefix("line", n)
}

func numberedLinesWithPrefix(prefix string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%s_%04d = %d\n", prefix, i, i)
	}
	return b.String()
}
