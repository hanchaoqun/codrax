package tracecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func representativeFixture248(t *testing.T, files, objects int) *Catalog {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < files; i++ {
		putFile(t, filepath.Join(root, fmt.Sprintf("dir%d/capture.data", i)), strings.Repeat("x", i+1))
	}
	c := discover(t, root)
	for _, a := range c.Snapshot().Artifacts {
		for j := 0; j < objects; j++ {
			plan(t, c, a.ID, fmt.Sprint(100+j), &Window{StartNS: 0, EndNS: 100})
		}
	}
	return c
}

func select248(t *testing.T, c *Catalog, o RepresentativeOptions) RepresentativeSnapshot {
	t.Helper()
	s, err := c.SelectRepresentatives(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func selected248(s RepresentativeSnapshot) []string {
	var ids []string
	for _, g := range s.Clusters {
		for _, m := range g.Members {
			if m.Selected {
				ids = append(ids, m.QueryID)
			}
		}
	}
	return ids
}

func TestHMC248RepresentativeExactSpecificationNotSemanticIdentity(t *testing.T) {
	c := representativeFixture248(t, 2, 1)
	a := c.Snapshot().Artifacts[0]
	base := c.Snapshot().Queries[0].Plan
	base.ArtifactID = a.ID
	for _, variant := range []string{"scope", "process", "window", "inclusive", "exclusive", "filter", "unit", "view"} {
		p := base
		switch variant {
		case "scope":
			p.Object.Scope = "process:other"
		case "process":
			p.Object.Kind = "process"
		case "window":
			p.Window = &Window{StartNS: 1, EndNS: 100}
		case "inclusive":
			v := true
			p.Window = &Window{StartNS: 0, EndNS: 100, EndInclusive: &v}
		case "exclusive":
			v := false
			p.Window = &Window{StartNS: 0, EndNS: 100, EndInclusive: &v}
		case "filter":
			p.Parameters = json.RawMessage(`{"thread":"100","limit":40,"pattern":"changed"}`)
		case "unit":
			p.Parameters = json.RawMessage(`{"thread":"100","limit":40,"unit":"other"}`)
		case "view":
			p.View = "different_view"
		}
		if _, err := c.RegisterPlannedQuery(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	s := select248(t, c, RepresentativeOptions{MaxClusters: 50})
	if len(s.Clusters) != 9 || s.Coverage.Members != 10 {
		t.Fatalf("precise specifications collapsed: %+v", s.Coverage)
	}
	if len(s.Clusters[0].Members) != 2 || s.Clusters[0].Members[0].ArtifactID == s.Clusters[0].Members[1].ArtifactID || s.Clusters[0].Members[0].ArtifactRevision == "" {
		t.Fatal("source-local members collapsed")
	}
	// Public snapshots/returned slices cannot mutate the private fixed roster.
	s.Clusters[0].Members[0].Selected = !s.Clusters[0].Members[0].Selected
	s.Clusters[0].Specification.Parameters[0] = '!'
	s.Clusters[0].Specification.Window.StartNS = 900
	next, _ := c.Representatives()
	if next.Clusters[0].Specification.Window.StartNS != 0 || !json.Valid(next.Clusters[0].Specification.Parameters) {
		t.Fatal("display mutated stored specification")
	}
}

func TestHMC248RepresentativeCoverageFrozenAndCurrentStates(t *testing.T) {
	c := representativeFixture248(t, 3, 3)
	s := select248(t, c, RepresentativeOptions{PerCluster: 1, MaxClusters: 2, Priority: "largest_input_first"})
	if s.Coverage.Selected != 2 || s.Coverage.BudgetNotSelected != 7 || s.Coverage.SkippedClusters != 1 {
		t.Fatalf("selection budget hidden: %+v", s.Coverage)
	}
	ids := selected248(s)
	q, _ := c.Query(ids[0])
	finish(t, c, q, OutcomeFailure)
	q, _ = c.Query(ids[1])
	finish(t, c, q, OutcomeCanceled)
	a := c.Snapshot().Artifacts[0]
	plan(t, c, a.ID, "new-target", nil)
	next, _ := c.Representatives()
	if next.SelectionID != s.SelectionID || !reflect.DeepEqual(selected248(next), ids) || next.Coverage.Members != 9 || next.Coverage.NotInSelection != 1 || next.Coverage.SelectedOutcomes[OutcomeFailure] != 1 || next.Coverage.SelectedOutcomes[OutcomeCanceled] != 1 {
		t.Fatalf("current outcomes reranked/changed denominator: %+v", next.Coverage)
	}
	q, _ = c.Query(ids[0])
	a = c.artifacts[q.ArtifactID]
	bind(t, c, a)
	finish(t, c, q, OutcomeSuccess)
	next, _ = c.Representatives()
	if next.Coverage.SelectedOutcomes[OutcomeSuccess] != 1 || next.Coverage.SelectedOutcomes[OutcomeNotExecuted] != 0 {
		t.Fatal("retry status not current")
	}
	for _, g := range next.Clusters {
		for _, m := range g.Members {
			if m.QueryID == q.ID && (m.Attempts != 2 || m.LatestAttempt == nil || m.LatestAttempt.PayloadRef == "" || m.LatestAttempt.SourceRevision == "") {
				t.Fatal("original result provenance lost")
			}
		}
	}
	// A failure is a retry candidate, not an unavailable input.
	retry := select248(t, c, RepresentativeOptions{Priority: "retry_first", MaxClusters: 50})
	found := false
	for _, g := range retry.Clusters {
		for _, m := range g.Members {
			if m.QueryID == ids[1] {
				found = m.Selected
			}
		}
	}
	if !found {
		t.Fatal("canceled query excluded from retry ranking")
	}
}

func TestHMC248RepresentativeUnavailableUnmatchedDiscoveryIndependent(t *testing.T) {
	c := representativeFixture248(t, 3, 0)
	as := c.Snapshot().Artifacts
	plan(t, c, as[0].ID, "100", nil)
	plan(t, c, as[1].ID, "100", nil)
	c.mu.Lock()
	a := c.artifacts[as[0].ID]
	a.Status = "unavailable"
	a.Bytes = 0
	c.artifacts[a.ID] = a
	c.discovery.Complete = false
	c.discovery.FilteredFiles = 7
	c.discovery.SymlinksSkipped = 2
	c.mu.Unlock()
	s := select248(t, c, RepresentativeOptions{Priority: "largest_input_first"})
	if s.Coverage.Candidates != 3 || s.Coverage.Members != 2 || s.Coverage.Selected != 1 || s.Coverage.Skipped != 1 || s.Coverage.Unmatched != 1 || s.Coverage.BudgetNotSelected != 0 || s.Discovery.Complete || s.Discovery.FilteredFiles != 7 || s.Discovery.SymlinksSkipped != 2 {
		t.Fatalf("independent coverage axes collapsed: %+v %+v", s.Coverage, s.Discovery)
	}
	for _, g := range s.Clusters {
		for _, m := range g.Members {
			if m.ArtifactID == as[0].ID && (m.Selected || m.SelectionReason != "source_unavailable") {
				t.Fatal("unknown input became zero-size observation")
			}
		}
	}
	// A selected source can subsequently fail validation without silently
	// changing the frozen selection or maintaining a successful current state.
	c.markStale(as[1].ID, ErrStale)
	next, _ := c.Representatives()
	if !reflect.DeepEqual(selected248(s), selected248(next)) || next.Coverage.SelectedOutcomes[OutcomeStale] != 1 {
		t.Fatal("stale selected source was replaced or presented as success")
	}
}

func TestHMC248RepresentativeCloneMergeHistoryAndDeterminism(t *testing.T) {
	c := representativeFixture248(t, 3, 2)
	s := select248(t, c, RepresentativeOptions{Priority: "largest_input_first"})
	fork := c.Clone()
	other := c.Clone()
	changed := select248(t, fork, RepresentativeOptions{PerCluster: 2})
	if own, _ := c.Representatives(); own.SelectionID != s.SelectionID {
		t.Fatal("fork changed parent selection")
	}
	if err := c.Merge(fork); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(other); err != nil {
		t.Fatal(err)
	}
	if current, _ := c.Representatives(); current.SelectionID != changed.SelectionID {
		t.Fatal("older fork replaced newer explicit selection")
	}
	repeated := select248(t, c, RepresentativeOptions{PerCluster: 2})
	if repeated.SelectionID != changed.SelectionID || !reflect.DeepEqual(repeated.Clusters, changed.Clusters) {
		t.Fatal("same input selection is nondeterministic")
	}
	old := c.Snapshot()
	var plans []Plan
	for _, q := range old.Queries {
		plans = append(plans, q.Plan)
	}
	resumed, err := Resume(context.Background(), old.Root, old, plans)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resumed.Representatives(); ok {
		t.Fatal("historical snapshot restored selection")
	}
	serialized, _ := json.Marshal(s)
	if _, err := DecodeSnapshot(serialized); err == nil {
		t.Fatal("representative display JSON became live catalog history")
	}
	if err := c.Merge(resumed); err != ErrForeignRun {
		t.Fatal("foreign selection/catalog merged")
	}
}

func TestHMC248RepresentativeInvalidOptionsAndCancellationAtomic(t *testing.T) {
	c := representativeFixture248(t, 1, 1)
	old := select248(t, c, RepresentativeOptions{})
	for _, o := range []RepresentativeOptions{{PerCluster: -1}, {PerCluster: 6}, {MaxClusters: 51}, {Priority: "model_similarity"}, {Priority: "unknown"}} {
		if _, err := c.SelectRepresentatives(context.Background(), o); err == nil {
			t.Fatalf("invalid resource/priority option accepted: %+v", o)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.SelectRepresentatives(ctx, RepresentativeOptions{PerCluster: 2}); err == nil {
		t.Fatal("canceled selection published")
	}
	if current, _ := c.Representatives(); current.SelectionID != old.SelectionID {
		t.Fatal("rejected selection replaced current roster")
	}
}
