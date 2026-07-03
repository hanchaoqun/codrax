package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests pin the Q3 lane (donghu customer friction 2026-07-03):
// trace_query source=attached_trace WITHOUT a path, no --htrace/--atrace blob,
// but the request referenced a real trace file that typed carriers
// (RuntimeArtifactPreflight / AnalysisIR) already hold. Exactly one
// stat-verified candidate auto-resolves; zero or many keep the rejection.

func writeReferencedTraceFixture(t *testing.T, repo, name string) string {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveTraceQuerySource_SingleReferencedArtifactFromPreflight(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
			},
		},
	}

	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil {
		t.Fatalf("expected auto-resolve from typed preflight carrier, got reject: %s", reject.Summary)
	}
	if source != "path" {
		t.Fatalf("source=%q, want %q", source, "path")
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "berlin.systrace") {
		t.Fatalf("path=%q, want the referenced trace artifact", path)
	}
}

func TestResolveTraceQuerySource_SingleReferencedArtifactFromAnalysisIR(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"VSyncGenerator", "berlin.systrace"},
				},
			},
			EvidencePlan: types.EvidencePlan{
				RequiredFiles: []string{"internal/render/vsync.go"},
			},
		},
	}

	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil {
		t.Fatalf("expected auto-resolve from AnalysisIR entity lane, got reject: %s", reject.Summary)
	}
	if source != "path" || !strings.HasSuffix(filepath.ToSlash(path), "berlin.systrace") {
		t.Fatalf("source=%q path=%q, want path-backed referenced artifact", source, path)
	}
}

func TestResolveTraceQuerySource_SameArtifactAcrossCarriersDedupes(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{
					Entities:          []string{"berlin.systrace"},
					MentionedEntities: []string{"berlin.systrace"},
				},
			},
		},
	}

	// One physical file named by several typed lanes must still count as ONE
	// candidate for the exact-one gate.
	if got := attachedTraceQueryReferencedArtifactCandidates(ctx); len(got) != 1 {
		t.Fatalf("candidates=%d, want 1 after dedupe: %+v", len(got), got)
	}
	_, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil || source != "path" {
		t.Fatalf("dedupe must keep the auto-resolve alive: source=%q reject=%+v", source, reject)
	}
}

func TestResolveTraceQuerySource_TwoReferencedArtifactsRejectAndList(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	writeReferencedTraceFixture(t, repo, "shanghai.ftrace")
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
				{Kind: "trace", Source: "shanghai.ftrace", Carrier: "request_path"},
			},
		},
	}

	_, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject == nil {
		t.Fatal("two stat-verified candidates must NOT auto-resolve")
	}
	for _, want := range []string{
		"attached trace blob",
		"Multiple referenced trace artifacts were detected",
		"berlin.systrace",
		"shanghai.ftrace",
		`source="path"`,
	} {
		if !strings.Contains(reject.Summary, want) {
			t.Fatalf("rejection must list candidates and the repair route; missing %q in:\n%s", want, reject.Summary)
		}
	}
}

// TestResolveTraceQuerySource_ExplicitMissingPathNeverAutoResolves pins the
// QF1' fix (adversarial review 2026-07-03): the referenced-artifact
// auto-resolve lane is a fallback for calls that named NO path. When the model
// explicitly names a path that does not exist, the call must be rejected —
// silently substituting another trace would answer about the wrong file. The
// single known candidate may appear as a hint but is never auto-adopted.
func TestResolveTraceQuerySource_ExplicitMissingPathNeverAutoResolves(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
			},
		},
	}

	missing := filepath.Join(repo, "new_capture.systrace") // never created
	_, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace", Path: missing})
	if reject == nil {
		t.Fatal("explicit non-existent path must reject, not silently auto-resolve to a different trace")
	}
	if !strings.Contains(reject.Summary, missing) {
		t.Fatalf("rejection must name the explicit path %q:\n%s", missing, reject.Summary)
	}
	if !strings.Contains(reject.Summary, "berlin.systrace") || !strings.Contains(reject.Summary, "hint only") {
		t.Fatalf("rejection must surface the known candidate as a hint:\n%s", reject.Summary)
	}

	// Companion pin: the SAME context with an EMPTY path keeps the existing
	// exact-one auto-resolve behaviour — the new gate keys purely on whether
	// the model supplied a path.
	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil || source != "path" || !strings.HasSuffix(filepath.ToSlash(path), "berlin.systrace") {
		t.Fatalf("empty path + exactly one candidate must still auto-resolve: path=%q source=%q reject=%+v", path, source, reject)
	}
}

// TestAttachedTraceReferencedCandidates_SymlinkSpellingsCountOnce pins the
// QF2' fix (adversarial review 2026-07-03): two spellings that stat to the
// SAME physical file (here via a symlink; case-variant spellings on a
// case-insensitive filesystem are the same failure class) must count as one
// candidate for the exact-one gate instead of producing a false
// multi-candidate rejection.
func TestAttachedTraceReferencedCandidates_SymlinkSpellingsCountOnce(t *testing.T) {
	repo := t.TempDir()
	real := writeReferencedTraceFixture(t, repo, "berlin.systrace")
	link := filepath.Join(repo, "berlin_link.systrace")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
				{Kind: "trace", Source: "berlin_link.systrace", Carrier: "request_path"},
			},
		},
	}

	got := attachedTraceQueryReferencedArtifactCandidates(ctx)
	if len(got) != 1 {
		t.Fatalf("symlinked spellings of one physical file must dedupe to 1 candidate, got %d: %+v", len(got), got)
	}
	_, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil || source != "path" {
		t.Fatalf("exact-one gate must pass after physical-identity dedupe: source=%q reject=%+v", source, reject)
	}
}

func TestResolveTraceQuerySource_ZeroCandidatesKeepsLegacyRejection(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{RepoRoot: repo, Mutable: types.NewMutableState("q")}

	_, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject == nil || !strings.Contains(reject.Summary, "attached trace blob") {
		t.Fatalf("no blob and no typed candidate must keep the legacy rejection, got %+v", reject)
	}
	if strings.Contains(reject.Summary, "Multiple referenced trace artifacts") {
		t.Fatalf("zero-candidate rejection must not mention a candidate list:\n%s", reject.Summary)
	}
}

func TestAttachedTraceReferencedCandidates_SkipDirectoryMissingAndAttachment(t *testing.T) {
	repo := t.TempDir()
	writeReferencedTraceFixture(t, repo, "berlin.systrace")
	// A DIRECTORY whose name is trace-shaped must not become a candidate.
	if err := os.Mkdir(filepath.Join(repo, "bundle.htrace"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		Mutable:  types.NewMutableState("q"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "berlin.systrace", Carrier: "request_path"},
				{Kind: "trace", Source: "bundle.htrace", Carrier: "request_path"},    // directory → skip
				{Kind: "trace", Source: "ghost.atrace", Carrier: "request_path"},     // missing → skip
				{Kind: "trace", Source: "attached_trace.txt", Carrier: "attachment"}, // attachment lane → skip
			},
		},
	}

	got := attachedTraceQueryReferencedArtifactCandidates(ctx)
	if len(got) != 1 || !strings.HasSuffix(filepath.ToSlash(got[0].resolved), "berlin.systrace") {
		t.Fatalf("directory/missing/attachment entries must be skipped, got %+v", got)
	}

	// With the noise skipped, exactly one candidate remains → auto-resolve.
	_, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace"})
	if reject != nil || source != "path" {
		t.Fatalf("expected auto-resolve after skipping invalid candidates: source=%q reject=%+v", source, reject)
	}
}

// TestTraceQueryPayloadRefAdvisory_SingleChokepoint pins H17: every body-level
// payload_ref line carries the audit-artifact drilldown advisory, and empty
// refs render nothing.
func TestTraceQueryPayloadRefAdvisory_SingleChokepoint(t *testing.T) {
	var b strings.Builder
	writeTraceQueryPayloadRefLine(&b, "", false)
	if b.Len() != 0 {
		t.Fatalf("empty payloadRef must render nothing, got %q", b.String())
	}

	writeTraceQueryPayloadRefLine(&b, "/tmp/work/trace_query-abcd1234.txt", true)
	got := b.String()
	if !strings.HasPrefix(got, "payload_ref=/tmp/work/trace_query-abcd1234.txt ") {
		t.Fatalf("payload_ref line lost its ref prefix: %q", got)
	}
	if !strings.Contains(got, traceQueryPayloadRefAdvisory) {
		t.Fatalf("payload_ref line must carry the H17 advisory: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("trailingBlankLine=true must keep the blank separator: %q", got)
	}

	// The three summary surfaces all route through the chokepoint.
	summary := traceQuerySummary(tracequery.Result{View: "event_search"}, traceQueryParams{}, "path", "/tmp/work/trace_query-abcd1234.txt")
	if !strings.Contains(summary, "payload_ref=/tmp/work/trace_query-abcd1234.txt "+traceQueryPayloadRefAdvisory) {
		t.Fatalf("traceQuerySummary must emit the advisory-carrying payload_ref line:\n%s", summary)
	}
	discovery := traceQueryRecipeDiscoverySummary("x.systrace", "path", traceQueryParams{}, 1, 1, nil, false, "/tmp/work/trace_query-abcd1234.txt")
	if !strings.Contains(discovery, "payload_ref=/tmp/work/trace_query-abcd1234.txt "+traceQueryPayloadRefAdvisory) {
		t.Fatalf("recipe discovery summary must emit the advisory-carrying payload_ref line:\n%s", discovery)
	}
	autoWindow := traceQueryAutoWindowSummary("x.systrace", "path", traceQueryParams{}, "auto", nil, "/tmp/work/trace_query-abcd1234.txt")
	if !strings.Contains(autoWindow, "payload_ref=/tmp/work/trace_query-abcd1234.txt "+traceQueryPayloadRefAdvisory) {
		t.Fatalf("auto window summary must emit the advisory-carrying payload_ref line:\n%s", autoWindow)
	}
}
