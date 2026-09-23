package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func assertPhysicalEmptyClosure(t *testing.T, c *types.EvidenceClosure) {
	t.Helper()
	total, known := c.FileTotalLinesSnapshot()["source.go"]
	if !known || total != 0 || !c.HasRead("source.go") || !c.HasFullyRead("source.go") || c.CoverageRatio("source.go") != 1 || c.HasReadLine("source.go", 1) || c.HasReadLine("source.go", 999) || c.MergedReadLines("source.go") != 0 {
		t.Fatalf("known empty lost or fabricated positive lines: totals=%v ranges=%v", c.FileTotalLinesSnapshot(), c.ReadRangesSnapshot())
	}
}

func TestReadFilePhysicalEmptyPropagationPublic(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "")
	r := physicalRead(t, ctx, "source.go", 0, 0)
	if !r.Success {
		t.Fatal(r.Summary)
	}
	c := types.NewEvidenceClosure(ctx.RepoRoot)
	c.IngestRound([]types.ToolResult{r}, ctx.RepoRoot)
	assertPhysicalEmptyClosure(t, c)
	t.Run("clone", func(t *testing.T) { assertPhysicalEmptyClosure(t, c.Clone()) })
	t.Run("merge", func(t *testing.T) {
		merged := types.NewEvidenceClosure(ctx.RepoRoot)
		merged.MergeFrom(c)
		assertPhysicalEmptyClosure(t, merged)
	})
	t.Run("snapshot_seed", func(t *testing.T) {
		seed := types.NewEvidenceClosure(ctx.RepoRoot)
		seed.IngestEvidenceReducerInput(types.EvidenceReducerInput{
			Class:   types.EvidenceReducerInputReadRunSnapshotSeed,
			ReadSet: c.ReadSet(), ReadRanges: c.ReadRangesSnapshot(), FileTotalLines: c.FileTotalLinesSnapshot(),
			ReplaceReadSet: true, ReplaceReadRanges: true, ReplaceFileTotalLines: true,
		}, ctx.RepoRoot)
		assertPhysicalEmptyClosure(t, seed)
	})
	t.Run("missing_enumeration_is_not_known_empty", func(t *testing.T) {
		unknown := r
		unknown.EnumerationAuthority = nil
		readSet, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{unknown}, ctx.RepoRoot)
		if len(readSet)+len(ranges)+len(totals) != 0 || types.ReadFileHasKnownEmptyLines(unknown, ctx.RepoRoot) {
			t.Fatal("unknown zero carrier became a physical empty-file observation")
		}
	})
	t.Run("positive_history_survives_later_empty_or_unknown", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "source.go"), []byte("one\ntwo\n"), 0600); err != nil {
			t.Fatal(err)
		}
		positive := physicalRead(t, ctx, "source.go", 1, 1)
		unknown := r
		unknown.EnumerationAuthority = nil
		for _, history := range [][]types.ToolResult{{positive, r, unknown}, {r, unknown, positive}} {
			acc := types.NewEvidenceClosure(ctx.RepoRoot)
			for _, entry := range history {
				acc.IngestRound([]types.ToolResult{entry}, ctx.RepoRoot)
			}
			if acc.FileTotalLines("source.go") != 2 || !acc.HasReadLine("source.go", 2) || acc.HasReadLine("source.go", 1) || acc.HasFullyRead("source.go") {
				t.Fatalf("cumulative positive facts were erased or expanded: %v %v", acc.FileTotalLinesSnapshot(), acc.ReadRangesSnapshot())
			}
		}
	})
	t.Run("positive_range_unknown_total_cannot_borrow_empty_total", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "source.go"), []byte("one\ntwo\n"), 0600); err != nil {
			t.Fatal(err)
		}
		partial := physicalRead(t, ctx, "source.go", 1, 1)
		coverage := *partial.ReadCoverage
		coverage.TotalLines = 0 // legacy positive range with unknown denominator
		partial.ReadCoverage = &coverage
		partial.EnumerationAuthority = nil
		for _, history := range [][]types.ToolResult{{r, partial}, {partial, r}} {
			acc := types.NewEvidenceClosure(ctx.RepoRoot)
			for _, entry := range history {
				acc.IngestRound([]types.ToolResult{entry}, ctx.RepoRoot)
			}
			if _, known := acc.FileTotalLinesSnapshot()["source.go"]; known || acc.HasFullyRead("source.go") || !acc.HasReadLine("source.go", 2) || acc.HasReadLine("source.go", 1) {
				t.Fatalf("unknown positive window acquired empty-file completeness: %v %v", acc.FileTotalLinesSnapshot(), acc.ReadRangesSnapshot())
			}
			left, right := types.NewEvidenceClosure(ctx.RepoRoot), types.NewEvidenceClosure(ctx.RepoRoot)
			left.IngestRound(history[:1], ctx.RepoRoot)
			right.IngestRound(history[1:], ctx.RepoRoot)
			left.MergeFrom(right.Clone())
			if _, known := left.FileTotalLinesSnapshot()["source.go"]; known || left.HasFullyRead("source.go") || !left.HasReadLine("source.go", 2) || left.HasReadLine("source.go", 1) {
				t.Fatal("fork merge borrowed empty completeness for a positive unknown-total range")
			}
		}
	})
}

func TestReadFilePhysicalEmptyCannotClearLineDebtPublic(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "")
	appendAdvisoryReadCoverageCaveat(ctx, []types.PendingRead{{File: "source.go", LineRanges: []types.LineRange{{Start: 1, End: 1}}}})
	r := physicalRead(t, ctx, "source.go", 0, 0)
	if !r.Success || len(ctx.Mutable.EvidenceClosure().CurrentCompletionCaveats()) == 0 {
		t.Fatal("empty file cannot satisfy a demand for an actual source line")
	}
}

func TestReadFilePhysicalEmptyRequiresBoundCarriersPublic(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "")
	actual := physicalRead(t, ctx, "source.go", 0, 0)
	for _, kind := range []string{"unknown_total", "wrong_scope", "wrong_ref", "emitted_row", "runtime_and_source", "failed"} {
		t.Run(kind, func(t *testing.T) {
			r := actual
			coverage := *actual.ReadCoverage
			r.ReadCoverage = &coverage
			authority := *actual.EnumerationAuthority
			authority.Boundaries = append([]types.ToolEnumerationBoundary(nil), authority.Boundaries...)
			r.EnumerationAuthority = &authority
			switch kind {
			case "unknown_total":
				authority.Boundaries[0].TotalKnown = false
			case "wrong_scope":
				authority.Boundaries[0].Scope = "other.go"
			case "wrong_ref":
				coverage.RawRef += "-unbound"
			case "emitted_row":
				authority.Boundaries[0].Emitted = 1
			case "runtime_and_source":
				r.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{RequestedPath: "source.go", RawRef: r.RawRef}
			case "failed":
				r.Success = false
			}
			readSet, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{r}, ctx.RepoRoot)
			if types.ReadFileHasKnownEmptyLines(r, ctx.RepoRoot) || len(readSet)+len(ranges)+len(totals) != 0 {
				t.Fatal("unbound/contradictory zero carriers became observed source authority")
			}
		})
	}
}
