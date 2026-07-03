package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/width"
	"github.com/hanchaoqun/codrax/internal/types"
)

// sparseFile creates a file whose stat size is size bytes without writing
// that much data (APFS/NTFS sparse) — the whole point of the bound is that
// oversized files are refused on stat, before any allocation.
func sparseFile(t *testing.T, dir, name string, size int64) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestNormalizeCurrentSourceCitationQuotes_SkipsOversizedFile pins the
// customer-OOM fix (2026-07-03): a citation whose file is a GiB-scale
// artifact inside the repo directory must NOT be slurped by the finalize
// quote normalizer — the model's own quote stays and the run survives.
// A normal small file keeps getting its quote repaired.
func TestNormalizeCurrentSourceCitationQuotes_SkipsOversizedFile(t *testing.T) {
	repo := t.TempDir()
	sparseFile(t, repo, "berlin.systrace", width.SourceReadMaxBytes+1)
	if err := os.WriteFile(filepath.Join(repo, "small.go"), []byte("package x\nfunc Real() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "x"}},
		Citations: []types.Citation{
			{File: "berlin.systrace", Line: 941657, Quote: "model-authored trace quote"},
			{File: "berlin.systrace", Line: 12, Quote: "second citation to the same artifact"},
			{File: "small.go", Line: 2, Quote: "stale quote"},
		},
	}
	ctx := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("q")}
	fixed := normalizeCurrentSourceCitationQuotes(doc, ctx)
	if doc.Citations[0].Quote != "model-authored trace quote" || doc.Citations[1].Quote != "second citation to the same artifact" {
		t.Fatalf("oversized artifact citations must keep the model quote: %+v", doc.Citations[:2])
	}
	if doc.Citations[2].Quote != "func Real() {}" {
		t.Fatalf("small file quote must still be repaired, got %q", doc.Citations[2].Quote)
	}
	if fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
}

// TestReadFileTool_RefusesOversizedWholeRead pins the read_file whole-read
// wall: a GiB-scale file is refused with the typed repair before any
// allocation, steering to grep / trace_query.
func TestReadFileTool_RefusesOversizedWholeRead(t *testing.T) {
	repo := t.TempDir()
	sparseFile(t, repo, "huge.trace", width.Defaults().ReadFile.MaxWholeReadBytes+1)
	mu := types.NewMutableState("q")
	ctx := &types.BusContext{RepoRoot: repo, Mutable: mu}
	tool := &ReadFile{}
	res, err := tool.Execute(ctx, json.RawMessage(fmt.Sprintf(`{"path":%q}`, "huge.trace")))
	if err != nil {
		t.Fatalf("exec error: %v", err)
	}
	if res.Success {
		t.Fatal("oversized read must be refused")
	}
	if res.Repair == nil || res.Repair.Code != "read_file_too_large" {
		t.Fatalf("expected read_file_too_large repair, got %+v", res.Repair)
	}
	for _, want := range []string{"grep", "trace_query"} {
		if !strings.Contains(res.Summary+res.Repair.Hint, want) {
			t.Fatalf("refusal must steer to %s: %q", want, res.Summary)
		}
	}
}

// TestNormalizeCurrentSourceCitationQuotes_ArtifactAndExcludeGates pins the
// O2 defense layers: a citation naming the attached trace's spelling is
// runtime provenance and never read as source (even when small), and an
// observation-only run (typed ExcludesCurrentSource) leaves the whole pass
// inert.
func TestNormalizeCurrentSourceCitationQuotes_ArtifactAndExcludeGates(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "berlin.systrace"), []byte("trace line 1\ntrace line 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "small.go"), []byte("package x\nfunc Real() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := func() *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{
			Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "x"}},
			Citations: []types.Citation{
				{File: "berlin.systrace", Line: 2, Quote: "model-authored artifact quote"},
				{File: "small.go", Line: 2, Quote: "stale quote"},
			},
		}
	}

	// Artifact-spelling gate: the trace citation keeps the model quote
	// even though the file is small and readable.
	d := doc()
	ctx := &types.BusContext{
		RepoRoot:              repo,
		Mutable:               types.NewMutableState("q"),
		AttachedHitraceSource: "berlin.systrace",
	}
	if fixed := normalizeCurrentSourceCitationQuotes(d, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1 (only small.go)", fixed)
	}
	if d.Citations[0].Quote != "model-authored artifact quote" {
		t.Fatalf("artifact citation must not be rewritten from file content: %q", d.Citations[0].Quote)
	}
	if d.Citations[1].Quote != "func Real() {}" {
		t.Fatalf("source citation must still repair: %q", d.Citations[1].Quote)
	}

	// Exclude gate: observation-only run leaves everything untouched.
	d = doc()
	ctx = &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"不要分析代码"},
			},
		}},
	}
	if fixed := normalizeCurrentSourceCitationQuotes(d, ctx); fixed != 0 {
		t.Fatalf("exclude-mode run must be inert, fixed=%d", fixed)
	}
	if d.Citations[1].Quote != "stale quote" {
		t.Fatalf("exclude-mode run must not rewrite quotes: %q", d.Citations[1].Quote)
	}
}

// TestResolveTraceQuerySource_AutoResolvesPathBackedArtifact pins the O3
// mechanical repair: source=attached_trace with no attached blob but a
// model-named real trace file falls back to source=path (stat-verified,
// nothing guessed) instead of burning a retry round; with no usable path
// the original rejection stays.
func TestResolveTraceQuerySource_AutoResolvesPathBackedArtifact(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "berlin.systrace"), []byte("# trace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("q")}

	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{
		Source: "attached_trace",
		Path:   "berlin.systrace",
	})
	if reject != nil {
		t.Fatalf("expected auto-resolve to path, got reject: %s", reject.Summary)
	}
	if source != "path" || !strings.HasSuffix(filepath.ToSlash(path), "berlin.systrace") {
		t.Fatalf("source=%q path=%q, want path-backed artifact", source, path)
	}

	_, _, reject = resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject == nil || !strings.Contains(reject.Summary, "attached trace blob") {
		t.Fatalf("no path and no blob must keep the rejection, got %+v", reject)
	}
}
