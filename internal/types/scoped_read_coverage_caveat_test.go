package types

import (
	"reflect"
	"sync"
	"testing"
)

func scopedCoverageTestCaveat() CompletionCaveat {
	return CompletionCaveat{Lane: DowngradeLaneForcedReadCoverage, ReasonCode: "coverage_reads_demoted", Reason: "historical completion"}
}

func scopedCoverageTestRead(c *EvidenceClosure, root, path string, start, end, total int) {
	c.RecordScopedReadCoverage(root, ToolReadCoverage{Path: path, LineStart: start, LineEnd: end, TotalLines: total, RawRef: "producer-read"})
}

func TestScopedReadCoverageCurrentViewPreservesHistoricalAuthority(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "AuditLog.java"}})
	before := c.CompletionCaveats()
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("unread source must be disclosed")
	}
	scopedCoverageTestRead(c, "/repo", "AuditLog.java", 1, 20, 20)
	if got := c.CurrentCompletionCaveats(); len(got) != 0 {
		t.Fatalf("completed exact source remains disclosed: %+v", got)
	}
	if !reflect.DeepEqual(c.CompletionCaveats(), before) || !c.HasCompletionCaveat(DowngradeLaneForcedReadCoverage) {
		t.Fatal("display reconciliation changed historical gate authority")
	}
}

func TestScopedReadCoverageMergesSourcesAndRetainsLegacyUnknown(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "scoped", true: "legacy_plus_scoped"}[legacy], func(t *testing.T) {
			c := NewEvidenceClosure("/repo")
			if legacy {
				c.AppendCompletionCaveat(scopedCoverageTestCaveat())
			}
			for _, path := range []string{"a.go", "b.go"} {
				c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: path}})
			}
			scopedCoverageTestRead(c, "/repo", "a.go", 1, 10, 10)
			if len(c.CurrentCompletionCaveats()) != 1 {
				t.Fatal("reading a.go erased the b.go obligation")
			}
			scopedCoverageTestRead(c, "/repo", "b.go", 1, 10, 10)
			want := 0
			if legacy {
				want = 1
			}
			if got := len(c.CurrentCompletionCaveats()); got != want {
				t.Fatalf("current=%d want=%d", got, want)
			}
		})
	}
}

func TestScopedReadCoverageExactRepositoryAndWindow(t *testing.T) {
	tests := []struct {
		name, readRoot, readPath string
		start, end, total        int
		ranges                   []LineRange
		resolved                 bool
	}{
		{"whole", "/repo", "src/a.go", 1, 100, 100, nil, true},
		{"different_repo", "/other", "src/a.go", 1, 100, 100, nil, false},
		{"different_subrepo", "/repo", "sibling/src/a.go", 1, 100, 100, nil, false},
		{"suffix_not_identity", "/repo", "a.go", 1, 100, 100, nil, false},
		{"partial_whole", "/repo", "src/a.go", 1, 50, 100, nil, false},
		{"unknown_whole_total", "/repo", "src/a.go", 1, 100, 0, nil, false},
		{"exact_range", "/repo", "src/a.go", 40, 60, 100, []LineRange{{40, 60}}, true},
		{"exact_range_unknown_total", "/repo", "src/a.go", 40, 60, 0, []LineRange{{40, 60}}, true},
		{"wrong_range", "/repo", "src/a.go", 1, 30, 100, []LineRange{{40, 60}}, false},
		{"range_gap", "/repo", "src/a.go", 40, 59, 100, []LineRange{{40, 60}}, false},
		{"invalid_proof", "/repo", "src/a.go", 101, 200, 100, nil, false},
		{"past_eof_demand", "/repo", "src/a.go", 1, 100, 100, []LineRange{{110, 130}}, false},
		{"valid_eof_padding", "/repo", "src/a.go", 90, 100, 100, []LineRange{{90, 115}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewEvidenceClosure("/repo")
			c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "src/a.go", LineRanges: tt.ranges}})
			scopedCoverageTestRead(c, tt.readRoot, tt.readPath, tt.start, tt.end, tt.total)
			if resolved := len(c.CurrentCompletionCaveats()) == 0; resolved != tt.resolved {
				t.Fatalf("resolved=%v want=%v", resolved, tt.resolved)
			}
		})
	}
}

func TestScopedReadCoverageForkMergeAndReset(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go", LineRanges: []LineRange{{10, 30}}}})
	stale := c.CloneForExploreDispatch()
	fork := c.CloneForExploreDispatch()
	if len(fork.CompletionCaveats()) != 1 {
		t.Fatal("forced-read history missing from fork")
	}
	scopedCoverageTestRead(c, "/repo", "a.go", 10, 20, 100)
	scopedCoverageTestRead(fork, "/repo", "a.go", 21, 30, 100)
	c.MergeFrom(fork)
	if len(c.CurrentCompletionCaveats()) != 0 {
		t.Fatal("complementary same-source fork reads did not reconcile")
	}
	c.MergeFrom(stale)
	if len(c.CurrentCompletionCaveats()) != 0 {
		t.Fatal("stale fork resurrected resolved display debt")
	}
	c.Reset()
	if len(c.CompletionCaveats()) != 0 || len(c.CurrentCompletionCaveats()) != 0 {
		t.Fatal("reset retained caveats")
	}
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go", LineRanges: []LineRange{{10, 30}}}})
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("new run borrowed previous read proofs")
	}
}

func TestScopedReadCoverageOtherLaneAndOrdinaryCoverageUnchanged(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	other := CompletionCaveat{Lane: DowngradeLaneExactResolvedDefiningProof, Reason: "unproved definition"}
	c.AppendCompletionCaveat(other)
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	c.AddReadSet(map[string]bool{"a.go": true})
	c.AddReadRanges(map[string][]LineRange{"a.go": {{1, 20}}})
	c.RecordFileTotalLines("a.go", 20)
	if len(c.CurrentCompletionCaveats()) != 2 {
		t.Fatal("unbound ordinary coverage resolved display debt")
	}
	scopedCoverageTestRead(c, "/repo", "a.go", 1, 20, 20)
	if got := c.CurrentCompletionCaveats(); !reflect.DeepEqual(got, []CompletionCaveat{other}) {
		t.Fatalf("unrelated lane changed: %+v", got)
	}
	if got := c.ReadRanges("a.go"); !reflect.DeepEqual(got, []LineRange{{1, 20}}) {
		t.Fatal("display proof changed ordinary coverage")
	}
}

func TestScopedReadCoverageMergesDifferentRepositoriesWithoutBorrowingProof(t *testing.T) {
	parent := NewEvidenceClosure("/one")
	parent.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/one", Path: "src/a.go"}})
	other := NewEvidenceClosure("/two")
	other.AddReadSet(map[string]bool{"src/a.go": true})
	other.AddReadRanges(map[string][]LineRange{"src/a.go": {{1, 10}}})
	other.RecordFileTotalLines("src/a.go", 10)
	scopedCoverageTestRead(other, "/two", "src/a.go", 1, 10, 10)
	parent.MergeFrom(other)
	if !parent.HasFullyRead("src/a.go") {
		t.Fatal("fixture must exercise the old unscoped coverage union")
	}
	if len(parent.CurrentCompletionCaveats()) != 1 {
		t.Fatal("different-repository proof cleared /one read debt")
	}
	scopedCoverageTestRead(parent, "/one", "src/a.go", 1, 10, 10)
	if len(parent.CurrentCompletionCaveats()) != 0 {
		t.Fatal("matching-repository proof did not resolve debt")
	}
}

func TestScopedReadCoverageUnknownSourcesAndInvalidReadCannotClaimResolved(t *testing.T) {
	for _, scope := range []ScopedReadCoverage{
		{},
		{RepositoryRoot: "", Path: "a.go"},
		{RepositoryRoot: "relative-repo", Path: "a.go"},
		{RepositoryRoot: "/repo", Path: "../a.go"},
		{RepositoryRoot: "/repo", Path: "/other/a.go"},
		{RepositoryRoot: "/repo", Path: "src/"},
		{RepositoryRoot: "/repo", Path: "a.go", LineRanges: []LineRange{{0, 10}}},
		{RepositoryRoot: "/repo", Path: "a.go", LineRanges: []LineRange{{10, 9}}},
	} {
		c := NewEvidenceClosure("/repo")
		c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{scope})
		c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
		scopedCoverageTestRead(c, "/repo", "a.go", 1, 10, 10)
		if len(c.CurrentCompletionCaveats()) != 1 {
			t.Fatalf("invalid/unknown source was guessed resolved: %+v", scope)
		}
	}
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), nil)
	scopedCoverageTestRead(c, "/repo", "a.go", 1, 10, 10)
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("missing source list was guessed resolved")
	}
	for _, coverage := range []ToolReadCoverage{
		{Path: "a.go", LineStart: 1, LineEnd: 10, TotalLines: 10}, // no successful producer reference
		{Path: "a.go", LineStart: 0, LineEnd: 10, TotalLines: 10, RawRef: "ref"},
		{Path: "a.go", LineStart: 2, LineEnd: 1, TotalLines: 10, RawRef: "ref"},
		{Path: "a.go", LineStart: 1, LineEnd: 10, TotalLines: -1, RawRef: "ref"},
		{Path: "a.go", LineStart: 1, LineEnd: 11, TotalLines: 10, RawRef: "ref"},
	} {
		c := NewEvidenceClosure("/repo")
		c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
		c.RecordScopedReadCoverage("/repo", coverage)
		if len(c.CurrentCompletionCaveats()) != 1 {
			t.Fatalf("invalid producer proof accepted: %+v", coverage)
		}
	}
}

func TestScopedReadCoverageLegacyRecordCannotAcquireNewScope(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	// A historical carrier with no new side state is intentionally unknown.
	c.completionCaveats = []CompletionCaveat{scopedCoverageTestCaveat()}
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	scopedCoverageTestRead(c, "/repo", "a.go", 1, 10, 10)
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("legacy unknown disappeared after an unrelated known scope")
	}
	fork := c.CloneForExploreDispatch()
	if len(fork.CurrentCompletionCaveats()) != 1 {
		t.Fatal("clone lost unknown source boundary")
	}
}

func TestScopedReadCoverageLegacyForkMergeRetainsUnknownSource(t *testing.T) {
	known := NewEvidenceClosure("/repo")
	known.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	scopedCoverageTestRead(known, "/repo", "a.go", 1, 10, 10)
	if len(known.CurrentCompletionCaveats()) != 0 {
		t.Fatal("known source should already be resolved")
	}
	legacy := NewEvidenceClosure("/repo")
	legacy.completionCaveats = []CompletionCaveat{scopedCoverageTestCaveat()}
	known.MergeFrom(legacy.CloneForExploreDispatch())
	if len(known.CurrentCompletionCaveats()) != 1 {
		t.Fatal("source-less historical fork borrowed known-source read proofs")
	}
	legacyParent := NewEvidenceClosure("/repo")
	legacyParent.completionCaveats = []CompletionCaveat{scopedCoverageTestCaveat()}
	scopedFork := NewEvidenceClosure("/repo")
	scopedFork.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "b.go"}})
	scopedCoverageTestRead(scopedFork, "/repo", "b.go", 1, 10, 10)
	legacyParent.MergeFrom(scopedFork)
	if len(legacyParent.CurrentCompletionCaveats()) != 1 {
		t.Fatal("source-less historical parent borrowed a new fork's scoped proof")
	}
}

func TestScopedReadCoverageDemandCopyUnionAndWholeFileDominance(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	ranges := []LineRange{{10, 20}}
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go", LineRanges: ranges}})
	ranges[0] = LineRange{1, 2}
	scopedCoverageTestRead(c, "/repo", "a.go", 1, 2, 50)
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("caller mutated stored demand")
	}
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go", LineRanges: []LineRange{{30, 40}}}})
	scopedCoverageTestRead(c, "/repo", "a.go", 10, 20, 50)
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("second demanded range was lost")
	}
	scopedCoverageTestRead(c, "/repo", "a.go", 30, 40, 50)
	if len(c.CurrentCompletionCaveats()) != 0 {
		t.Fatal("both demanded ranges should resolve")
	}
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("new whole-file obligation was erased by narrower ranges")
	}
	scopedCoverageTestRead(c, "/repo", "a.go", 1, 50, 50)
	if len(c.CurrentCompletionCaveats()) != 0 {
		t.Fatal("whole-file proof should resolve all source demands")
	}
}

func TestScopedReadCoverageCloneDoesNotShareProofOrExpandOtherLaneHistory(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	c.AppendCompletionCaveat(CompletionCaveat{Lane: DowngradeLaneContractChain})
	fork := c.CloneForExploreDispatch()
	if got := fork.CompletionCaveats(); len(got) != 1 || got[0].Lane != DowngradeLaneForcedReadCoverage {
		t.Fatalf("unrelated caveat clone behavior expanded: %+v", got)
	}
	scopedCoverageTestRead(fork, "/repo", "a.go", 1, 10, 10)
	if len(c.CurrentCompletionCaveats()) != 2 {
		t.Fatal("fork proof mutated parent before merge")
	}
	view := c.CurrentCompletionCaveats()
	view[0].Reason = "caller mutation"
	if c.CompletionCaveats()[0].Reason != "historical completion" {
		t.Fatal("display view mutated history")
	}
	c.MergeFrom(fork)
	if got := c.CurrentCompletionCaveats(); len(got) != 1 || got[0].Lane != DowngradeLaneContractChain {
		t.Fatalf("wrong current lanes after merge: %+v", got)
	}
	c.Reset()
	if len(c.CompletionCaveats()) != 0 {
		t.Fatal("reset must clear all historical lanes")
	}
}

func TestScopedReadCoverageCanonicalIdentityIsNotPatternMatching(t *testing.T) {
	for _, path := range []string{"src/a.go", "fixtures/expected[1].output"} {
		c := NewEvidenceClosure("/repo")
		c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo/./", Path: "/repo/" + path}})
		scopedCoverageTestRead(c, "/repo", "./"+path, 1, 10, 10)
		if len(c.CurrentCompletionCaveats()) != 0 {
			t.Fatalf("same exact canonical literal failed: %s", path)
		}
	}
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "src/*.go"}})
	scopedCoverageTestRead(c, "/repo", "src/a.go", 1, 10, 10)
	if len(c.CurrentCompletionCaveats()) != 1 {
		t.Fatal("glob-looking bytes must not match a different literal")
	}
}

func TestScopedReadCoverageConcurrentObservationAndCurrentView(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.AppendScopedReadCoverageCaveat(scopedCoverageTestCaveat(), []ScopedReadCoverage{{RepositoryRoot: "/repo", Path: "a.go"}})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				scopedCoverageTestRead(c, "/repo", "a.go", 1, 10, 10)
				_ = c.CurrentCompletionCaveats()
				_ = c.CloneForExploreDispatch()
			}
		}()
	}
	wg.Wait()
	if len(c.CurrentCompletionCaveats()) != 0 || !c.HasCompletionCaveat(DowngradeLaneForcedReadCoverage) {
		t.Fatal("concurrent display state lost history or proof")
	}
}
