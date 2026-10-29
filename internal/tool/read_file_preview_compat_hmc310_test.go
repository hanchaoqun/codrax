package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC310PublicReadablePreviewDoesNotGrantUnseenAuthority(t *testing.T) {
	const path = "tests/long.go"
	body := strings.Repeat("x", MaxInlineBytes*2) + "\n"
	ctx := physicalReadFixture(t, path, body)
	ctx.Mode = types.ModePlan
	appendAdvisoryReadCoverageCaveat(ctx, []types.PendingRead{{File: path}})
	out := dispatchReadVersionPublish(t, ctx, path, 0, 1)
	c := out.ReadCoverage
	if c == nil || c.Path != path || c.RawRef != out.RawRef || c.LineStart != 0 || c.LineEnd != 0 || c.TotalLines != 1 {
		t.Fatalf("readable source identity must survive with zero visible rows: %+v", out)
	}
	if out.RawRef == "" || len(out.Observations) != 0 || types.ReadFileHasKnownEmptyLines(out, ctx.RepoRoot) {
		t.Fatal("hidden bytes became an observation or empty-file proof")
	}
	reads, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{out}, ctx.RepoRoot)
	if len(reads)+len(ranges)+len(totals) != 0 || dispatchReadVersionComplete(t, ctx, path, body) || protectedBaselineObservedInCurrentRepository(ctx, path) {
		t.Fatal("a successful preview acquired source/completion/version authority")
	}
	if len(ctx.Mutable.EvidenceClosure().CurrentCompletionCaveats()) == 0 {
		t.Fatal("zero visible lines settled an unread source")
	}
	missing := physicalRead(t, ctx, "missing.go", 0, 1)
	if missing.Success || missing.ReadCoverage != nil {
		t.Fatal("real missing source must still fail")
	}
}
