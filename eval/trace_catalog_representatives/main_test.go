package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func fixture(t *testing.T) (string, preparation) {
	t.Helper()
	for _, executable := range []string{"git", "sqlite3"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " unavailable")
		}
	}
	out := filepath.Join(t.TempDir(), "result")
	p, err := prepare(context.Background(), out, "../fixtures/hmosperf_catalog_objects")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(p.Workspace), p
}

// Fault injection is only at the preparer boundary. Every attempt, including
// failures, still originates from the public TraceQuery tool and catalog ref.
type failingPreparer289 struct {
	types.TraceInputPreparer
	failNext bool
}

func (p *failingPreparer289) Prepare(ctx context.Context, path string) (*attachment.TraceMaterial, error) {
	if p.failNext {
		p.failNext = false
		return nil, errors.New("HMC289 transient preparation failure")
	}
	return p.TraceInputPreparer.Prepare(ctx, path)
}

type publicScenario289 struct {
	extras, retry, unselected, unselectedFailure, lateFailure, pollutedSelection bool
}

func publicSelection289(t *testing.T, scenario publicScenario289) (preparation, auditInput, tracecatalog.Snapshot, selectionDocument) {
	t.Helper()
	out, prep := fixture(t)
	preparer := &failingPreparer289{TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "prepared")})}
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: prep.Workspace, WorkDir: filepath.Join(out, "public-run"), Mutable: types.NewMutableState(question), TraceInputPreparer: preparer}
	call := func(raw json.RawMessage, success bool) {
		t.Helper()
		result, err := (&tool.TraceQuery{}).Execute(bus, raw)
		if (err == nil && result.Success) != success {
			t.Fatalf("public query success=%v want=%v: %v %s", result.Success, success, err, result.Summary)
		}
	}
	if scenario.extras {
		prior := invokeCatalog(t, bus, map[string]any{"root": "captures", "queries": []map[string]any{{"view": "event_search", "pattern": "AllocEvent"}}})
		c := bus.Mutable.TraceCatalogs()[0]
		q := c.Snapshot().Queries[0]
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": prior.CatalogID, "query_id": q.ID}})
		call(raw, true)
	}
	plans := []map[string]any{
		{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05},
		{"view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05},
	}
	if scenario.pollutedSelection {
		plans = append(plans, map[string]any{"view": "event_search", "pid": 101, "time_start": 10, "time_end": 10.05}, map[string]any{"view": "event_search", "pid": 202, "time_start": 10, "time_end": 10.05})
	}
	selected := invokeCatalog(t, bus, map[string]any{"root": "captures", "queries": plans, "representatives": map[string]any{"per_cluster": 2, "max_clusters": 10}})
	c := bus.Mutable.TraceCatalogs()[0]
	s, _ := c.Representatives()
	var ids []string
	var unselectedID string
	for _, group := range s.Clusters {
		for _, member := range group.Members {
			if member.Selected {
				ids = append(ids, member.QueryID)
			} else {
				unselectedID = member.QueryID
			}
		}
	}
	sort.Strings(ids)
	queryRef := func(id string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": selected.CatalogID, "query_id": id}})
		return raw
	}
	// Execute the IDs actually frozen by public selection. This deterministic
	// tool test never seeds those IDs into the separate live preparation.
	for _, id := range ids {
		if scenario.retry && id == ids[0] {
			preparer.failNext = true
			call(queryRef(id), false)
		}
		call(queryRef(id), true)
	}
	if scenario.unselected || scenario.unselectedFailure {
		preparer.failNext = scenario.unselectedFailure
		call(queryRef(unselectedID), !scenario.unselectedFailure)
	}
	if scenario.extras {
		invokeCatalog(t, bus, map[string]any{"root": "captures", "queries": []map[string]any{{"view": "event_search", "pattern": "FreeEvent"}}})
		fresh, _ := c.Representatives()
		if len(fresh.ExcludedQueryIDs) != 4 || len(fresh.NewQueryIDs) != 4 || fresh.SelectionID != selected.SelectionID {
			t.Fatalf("public scope/new-query fixture differs: %+v", fresh)
		}
		call(queryRef(fresh.NewQueryIDs[0]), true)
	}
	if scenario.lateFailure {
		preparer.failNext = true
		call(queryRef(ids[0]), false)
	}
	last := invokeCatalog(t, bus, map[string]any{"action": "representative_status", "catalog_id": selected.CatalogID})
	current, err := tracecatalog.Load(context.Background(), last.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(last.Selection)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot selectionDocument
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatal(err)
	}
	return prep, auditInput{out, last.SavedIndex, last.Selection, last.SelectionSHA}, current, snapshot
}

func TestHMC289PublicScopedAuditKeepsExtrasRetriesAndExecutedUnselectedSeparate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scenario publicScenario289
	}{
		{"before_and_after_extra_queries", publicScenario289{extras: true}},
		{"failed_then_successful_selected", publicScenario289{retry: true}},
		{"actually_queried_unselected_stays_unselected", publicScenario289{unselected: true}},
		{"failed_unselected_stays_unselected_and_failed", publicScenario289{unselectedFailure: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, input, c, s := publicSelection289(t, tc.scenario)
			if err := auditSnapshots(p, c, s); err != nil {
				t.Fatalf("complete task selection with independent ordinary work rejected: %v", err)
			}
			if err := audit(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			if s.Coverage.Members != 8 || s.Coverage.Selected != 4 || s.Coverage.BudgetNotSelected != 4 || len(s.Clusters) != 2 {
				t.Fatalf("core matrix changed: %+v", s.Coverage)
			}
		})
	}
}

func TestHMC289PublicAuditRejectsLatestFailureAndPollutedSelection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scenario publicScenario289
	}{
		{"success_then_failure", publicScenario289{lateFailure: true}},
		{"sixteen_members_four_groups_is_not_eight_two", publicScenario289{pollutedSelection: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, c, s := publicSelection289(t, tc.scenario)
			if err := auditSnapshots(p, c, s); err == nil {
				t.Fatal("incomplete current representative work passed")
			}
		})
	}
}

func TestHMC289ScopedAuditRejectsForgedPartitionsAndAttemptHistory(t *testing.T) {
	p, _, current, selection := publicSelection289(t, publicScenario289{extras: true, retry: true, unselected: true})
	if err := auditSnapshots(p, current, selection); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*tracecatalog.Snapshot, *selectionDocument)
	}{
		{"unknown_scope", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.SelectionScope = "other" }},
		{"whole_catalog_with_exclusions", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.SelectionScope = "all_current_queries" }},
		{"unknown_new_id", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.NewQueryIDs[0] = "forged" }},
		{"unknown_excluded_id", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.ExcludedQueryIDs[0] = "forged" }},
		{"duplicate_new_id", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.NewQueryIDs[1] = s.NewQueryIDs[0] }},
		{"new_excluded_overlap", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.NewQueryIDs[0] = s.ExcludedQueryIDs[0] }},
		{"core_hidden_as_extra", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.ExcludedQueryIDs[0] = s.Clusters[0].Members[0].QueryID
		}},
		{"missing_extra", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.NewQueryIDs = s.NewQueryIDs[1:]
			s.Coverage.NotInSelection--
		}},
		{"missing_core", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Clusters[0].Members = s.Clusters[0].Members[1:]
		}},
		{"wrong_group", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Clusters[0].Specification.View = "event_search"
		}},
		{"duplicate_group_id", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Clusters[1].ID = s.Clusters[0].ID }},
		{"duplicate_query_id", func(c *tracecatalog.Snapshot, s *selectionDocument) { c.Queries[1].ID = c.Queries[0].ID }},
		{"unknown_query_source", func(c *tracecatalog.Snapshot, s *selectionDocument) { c.Queries[0].ArtifactID = "foreign" }},
		{"false_extra_count", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Coverage.Excluded++ }},
		{"false_outcome_distribution", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Coverage.Outcomes[tracecatalog.OutcomeSuccess]--
			s.Coverage.Outcomes[tracecatalog.OutcomeEmpty]++
		}},
		{"missing_outcomes", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Coverage.Outcomes = nil }},
		{"extra_attempt_foreign_source", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if c.Queries[i].View == "event_search" && len(c.Queries[i].Attempts) > 0 {
					c.Queries[i].Attempts[0].SourceRevision = "foreign"
					return
				}
			}
			t.Fatal("missing extra attempt fixture")
		}},
		{"duplicate_attempt_id", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) == 2 {
					c.Queries[i].Attempts[1].ID = c.Queries[i].Attempts[0].ID
					return
				}
			}
			t.Fatal("missing retry fixture")
		}},
		{"duplicate_attempt_sequence", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) == 2 {
					c.Queries[i].Attempts[1].Sequence = c.Queries[i].Attempts[0].Sequence
					return
				}
			}
			t.Fatal("missing retry fixture")
		}},
		{"history_not_current_state", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) == 2 {
					c.Queries[i].Outcome = tracecatalog.OutcomeFailure
					return
				}
			}
			t.Fatal("missing retry fixture")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c tracecatalog.Snapshot
			var s selectionDocument
			cb, _ := json.Marshal(current)
			sb, _ := json.Marshal(selection)
			if json.Unmarshal(cb, &c) != nil || json.Unmarshal(sb, &s) != nil {
				t.Fatal("clone")
			}
			tc.mutate(&c, &s)
			if err := auditSnapshots(p, c, s); err == nil {
				t.Fatal("forged partition or history passed")
			}
		})
	}
}

func TestPrepareOnlyCopiesOracleAndGit(t *testing.T) {
	out, p := fixture(t)
	if p.ModelCalls != 0 || p.ToolCalls != 0 || len(p.SourceSHA) != 4 || len(p.Expected) != 8 || p.GitHEAD == "" {
		t.Fatalf("invalid zero-model preparation: %+v", p)
	}
	entries, err := os.ReadDir(p.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != ".git" && entry.Name() != "captures" {
			t.Fatalf("preparation created model-visible oracle/checkpoint/results: %s", entry.Name())
		}
	}
	for _, copy := range copies {
		body, err := os.ReadFile(filepath.Join(p.Workspace, copy.Destination))
		if err != nil || hash(body) != p.SourceSHA[copy.Destination] {
			t.Fatalf("original changed: %s %v", copy.Destination, err)
		}
		oracle, err := os.ReadFile(filepath.Join(out, "operator", strings.ReplaceAll(copy.Destination, "/", "_")+".oracle.json"))
		if err != nil || !json.Valid(oracle) || !strings.Contains(string(oracle), "event_frame_rows") {
			t.Fatalf("independent full event/frame oracle missing: %v", err)
		}
	}
	if p.SourceSHA[copies[0].Destination] != p.SourceSHA[copies[2].Destination] || p.SourceSHA[copies[1].Destination] != p.SourceSHA[copies[3].Destination] {
		t.Fatal("test copies were silently modified into new captures")
	}
	if _, err := prepare(context.Background(), out, "../fixtures/hmosperf_catalog_objects"); err == nil {
		t.Fatal("earlier evaluation was overwritten")
	}
	if _, err := prepare(context.Background(), "relative-result", "../fixtures/hmosperf_catalog_objects"); err == nil {
		t.Fatal("relative output accepted")
	}
}

type page struct {
	CatalogID    string                     `json:"catalog_id"`
	SelectionID  string                     `json:"selection_id"`
	SavedIndex   string                     `json:"saved_index"`
	Selection    string                     `json:"selection_snapshot"`
	SelectionSHA string                     `json:"selection_snapshot_sha256"`
	Calls        map[string]json.RawMessage `json:"query_calls"`
}

func invokeCatalog(t *testing.T, bus *types.BusContext, params any) page {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceCatalog{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("public catalog: %v %s", err, result.Summary)
	}
	var p page
	if err := json.Unmarshal([]byte(result.Summary), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func publicSelection(t *testing.T, out string, p preparation) (auditInput, tracecatalog.Snapshot, selectionDocument) {
	t.Helper()
	bus := &types.BusContext{Ctx: context.Background(), RepoRoot: p.Workspace, WorkDir: filepath.Join(out, "deterministic-public-run"), Mutable: types.NewMutableState(question),
		TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(out, "deterministic-public-run", "prepared")})}
	first := invokeCatalog(t, bus, map[string]any{"root": "captures", "queries": []map[string]any{
		{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05},
		{"view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05},
	}})
	selected := invokeCatalog(t, bus, map[string]any{"action": "representatives", "catalog_id": first.CatalogID, "representatives": map[string]any{"per_cluster": 2, "max_clusters": 2}})
	if len(selected.Calls) != 4 {
		t.Fatalf("actual public page did not offer four representative calls: %d", len(selected.Calls))
	}
	if err := audit(context.Background(), auditInput{out, selected.SavedIndex, selected.Selection, selected.SelectionSHA}); err == nil {
		t.Fatal("unexecuted selection passed as completed")
	}
	before, err := tracecatalog.Load(context.Background(), selected.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	artifactPaths := map[string]string{}
	queryByID := map[string]tracecatalog.QueryRecord{}
	for _, a := range before.Artifacts {
		artifactPaths[a.ID] = a.Path
	}
	for _, q := range before.Queries {
		queryByID[q.ID] = q
	}
	ids := make([]string, 0, len(selected.Calls))
	for id := range selected.Calls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		// Use only the exact query call actually returned on the public page.
		result, err := (&tool.TraceQuery{}).Execute(bus, selected.Calls[id])
		if err != nil || !result.Success {
			t.Fatalf("native selected query %s: %v %s", id, err, result.Summary)
		}
		q := queryByID[id]
		matched := false
		for _, observation := range result.Observations {
			r, ok := tool.DecodeTraceResourceStack(observation)
			if !ok {
				continue
			}
			frames := 0
			for _, event := range r.Events {
				frames += len(event.Frames)
			}
			for _, want := range p.Expected {
				if artifactPaths[q.ArtifactID] == filepath.Join(p.Workspace, want.Path) && r.TargetPID == want.TID {
					matched = r.MatchedEvents == want.Events && r.OmittedEvents == 0 && frames == want.Frames && r.Window.StartTs == 10 && r.Window.EndTs == 10.05 && !r.Window.EndInclusive
				}
			}
		}
		if !matched {
			t.Fatalf("actual typed result differs from independently frozen SQLite oracle: %s", id)
		}
	}
	last := invokeCatalog(t, bus, map[string]any{"action": "representative_status", "catalog_id": first.CatalogID})
	if last.SelectionID != selected.SelectionID {
		t.Fatal("status replaced the frozen selection")
	}
	current, err := tracecatalog.Load(context.Background(), last.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(last.Selection)
	if err != nil {
		t.Fatal(err)
	}
	var selection selectionDocument
	if err := json.Unmarshal(body, &selection); err != nil {
		t.Fatal(err)
	}
	return auditInput{out, last.SavedIndex, last.Selection, last.SelectionSHA}, current, selection
}

func TestPublicRepresentativeQueriesAndStructuredAudit(t *testing.T) {
	out, p := fixture(t)
	input, current, selection := publicSelection(t, out, p)
	if err := auditSnapshots(p, current, selection); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), input); err == nil {
		t.Fatal("an audit receipt was overwritten")
	}
	if err := os.Chmod(filepath.Join(p.Workspace, copies[0].Destination), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Workspace, copies[0].Destination), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := audit(context.Background(), input); err == nil || !strings.Contains(err.Error(), "capture changed") {
		t.Fatalf("changed source accepted: %v", err)
	}
}

func TestStructuredAuditRejectsForeignStaleIncompleteAndMixedInputs(t *testing.T) {
	out, p := fixture(t)
	input, current, selection := publicSelection(t, out, p)
	clone := func() (tracecatalog.Snapshot, selectionDocument) {
		c, s := tracecatalog.Snapshot{}, selectionDocument{}
		cb, _ := json.Marshal(current)
		sb, _ := json.Marshal(selection)
		if json.Unmarshal(cb, &c) != nil || json.Unmarshal(sb, &s) != nil {
			t.Fatal("clone")
		}
		return c, s
	}
	for _, tc := range []struct {
		name   string
		mutate func(*tracecatalog.Snapshot, *selectionDocument)
	}{
		{"foreign_catalog", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.CatalogID = "foreign" }},
		{"foreign_member", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Clusters[0].Members[0].Path += ".foreign" }},
		{"wrong_query", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Clusters[0].Members[0].QueryID = s.Clusters[1].Members[0].QueryID
		}},
		{"wrong_window", func(c *tracecatalog.Snapshot, s *selectionDocument) { c.Queries[0].Window.EndNS++ }},
		{"different_kind", func(c *tracecatalog.Snapshot, s *selectionDocument) { c.Queries[0].Object.Kind = "process" }},
		{"stale_source", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Clusters[0].Members[0].ArtifactRevision = "old"
		}},
		{"missing_group", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Clusters = s.Clusters[:1] }},
		{"missing_member", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			s.Clusters[0].Members = s.Clusters[0].Members[:3]
		}},
		{"false_coverage", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.Coverage.Selected++ }},
		{"discovery_incomplete", func(c *tracecatalog.Snapshot, s *selectionDocument) { c.Discovery.Complete = false }},
		{"new_plan", func(c *tracecatalog.Snapshot, s *selectionDocument) { s.NewQueryIDs = []string{"additional"} }},
		{"stale_attempt", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) > 0 {
					c.Queries[i].Attempts[0].SourceRevision = "old"
					return
				}
			}
		}},
		{"actual_scope_changed", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) > 0 {
					c.Queries[i].Attempts[0].ExecutedParameters = json.RawMessage(`{"view":"resource_stack","time_start":10,"time_end":10.05}`)
					return
				}
			}
		}},
		{"unselected_signed_empty", func(c *tracecatalog.Snapshot, s *selectionDocument) {
			for i := range c.Queries {
				if len(c.Queries[i].Attempts) == 0 {
					c.Queries[i].Outcome = tracecatalog.OutcomeEmpty
					return
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, s := clone()
			tc.mutate(&c, &s)
			if err := auditSnapshots(p, c, s); err == nil {
				t.Fatal("invalid selection/index passed")
			}
		})
	}
	for _, broken := range []auditInput{
		{out, input.Index, input.Selection, strings.Repeat("0", 64)},
		{out, input.Index, "relative-selection.json", input.SelectionSHA},
		{out, "", input.Selection, input.SelectionSHA},
		{out, input.Index, filepath.Join(out, "absent.json"), input.SelectionSHA},
	} {
		if err := audit(context.Background(), broken); err == nil {
			t.Fatal("missing/mismatched exact returned artifacts accepted")
		}
	}
}
