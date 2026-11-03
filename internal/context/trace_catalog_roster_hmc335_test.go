package context_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public discovery/selection/query/context path, including genuine
// repeated successful attempts and independent post-selection queries. Repeated
// source/specification bytes must not evict the last unselected member.
func TestHMC335PublicRepresentativeRosterWholeHandoff(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(root, strings.Repeat("capture-", 18))
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.data", "b.data", "c.data", "d.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	bus := &types.BusContext{RepoRoot: root, WorkDir: work, Mutable: types.NewMutableState("Inspect representative members"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
	hmc285CatalogCall(t, bus, map[string]any{"queries": []map[string]any{{"view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05}, {"view": "resource_stack", "pid": 202, "time_start": 10, "time_end": 10.05}}, "representatives": map[string]any{"per_cluster": 2}})
	c := bus.Mutable.TraceCatalogs()[0]
	for _, g := range c.CurrentExecution().Selection.Clusters {
		for _, m := range g.Members {
			if !m.Selected {
				continue
			}
			args, _ := json.Marshal(tracecatalog.QueryCall{Reference: tracecatalog.QueryReference{CatalogID: c.ID(), QueryID: m.QueryID}})
			for n := 0; n < 2; n++ {
				out, err := (&tool.TraceQuery{}).Execute(bus, args)
				if err != nil || !out.Success {
					t.Fatalf("real native attempt: %v %s", err, out.Summary)
				}
			}
		}
	}
	for i := 0; i < 7; i++ {
		a := c.Snapshot().Artifacts[i%4]
		params, _ := json.Marshal(map[string]any{"view": "event_search", "pattern": fmt.Sprint(i)})
		if _, err := c.RegisterDeclaredQuery(context.Background(), tracecatalog.Plan{ArtifactID: a.ID, Object: tracecatalog.Object{Kind: "capture", ID: "all"}, View: "event_search", Parameters: params}); err != nil {
			t.Fatal(err)
		}
	}
	want := c.CurrentExecution()
	before, _ := json.Marshal(want)
	for _, stage := range []types.PipelineStage{types.StageExplore, types.StageExtract, types.StageFinalize} {
		content := hmc259CatalogContext(t, bus, stage)
		if len(content) > 24<<10 {
			t.Fatalf("%s: unchanged shared budget exceeded: %d", stage, len(content))
		}
		members, navigation := hmc335DecodeRoster(t, content)
		if len(members) != 8 {
			t.Fatalf("%s: exact representative roster lost members: got=%d want=8 (%d bytes)", stage, len(members), len(content))
		}
		for _, g := range want.Selection.Clusters {
			for _, m := range g.Members {
				if !reflect.DeepEqual(members[m.QueryID], m) || !reflect.DeepEqual(navigation[m.QueryID], want.RepresentativeNavigation()[m.QueryID]) {
					t.Fatalf("%s: source/specification/attempt identity lost for %s", stage, m.QueryID)
				}
			}
		}
		for _, count := range []string{"selected_member_records_displayed=4 omitted=0", "unselected_member_records_displayed=4 omitted=0", "comparison_groups_displayed=2 omitted=0", `"new_queries_not_in_selection":7`, "outside_selection_queries_displayed="} {
			if !strings.Contains(content, count) {
				t.Fatalf("%s: exact independent census lost: %s", stage, count)
			}
		}
	}
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("display changed source, selection, execution or retry authority")
	}
}

// Independent reconstruction oracle: compact context must be lossless, not an
// abridged member preview. The legacy shape remains valid when it already fits.
func hmc335DecodeRoster(t *testing.T, content string) (map[string]tracecatalog.RepresentativeMember, map[string]tracecatalog.RepresentativeQueryNavigation) {
	t.Helper()
	members := map[string]tracecatalog.RepresentativeMember{}
	navigation := map[string]tracecatalog.RepresentativeQueryNavigation{}
	for _, line := range strings.Split(content, "\n") {
		var row struct {
			Selected   *tracecatalog.RepresentativeMember `json:"selected_member"`
			Unselected *tracecatalog.RepresentativeMember `json:"unselected_member"`
			tracecatalog.RepresentativeQueryNavigation
			Roster *struct {
				Sources []struct {
					ArtifactID       string `json:"artifact_id"`
					Path             string `json:"path"`
					ArtifactRevision string `json:"artifact_revision"`
					SourceRevision   string `json:"source_revision"`
					SourceStatus     string `json:"source_status"`
				} `json:"sources"`
				Groups []struct {
					Specification tracecatalog.QuerySpecification `json:"specification"`
				} `json:"groups"`
				Members []struct {
					GroupIndex      int                          `json:"group_index"`
					SourceIndex     int                          `json:"source_index"`
					QueryID         string                       `json:"query_id"`
					Selected        bool                         `json:"selected"`
					SelectionReason string                       `json:"selection_reason"`
					Outcome         tracecatalog.Outcome         `json:"outcome"`
					Attempts        int                          `json:"attempt_count"`
					LatestAttempt   *tracecatalog.Attempt        `json:"latest_attempt"`
					Origin          tracecatalog.QueryOrigin     `json:"query_origin"`
					AttemptOutcomes map[tracecatalog.Outcome]int `json:"attempt_outcomes"`
				} `json:"members"`
			} `json:"representative_roster"`
		}
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		m := row.Selected
		if m == nil {
			m = row.Unselected
		}
		if m != nil {
			members[m.QueryID], navigation[m.QueryID] = *m, row.RepresentativeQueryNavigation
		}
		if r := row.Roster; r != nil {
			for _, member := range r.Members {
				if member.SourceIndex < 0 || member.SourceIndex >= len(r.Sources) || member.GroupIndex < 0 || member.GroupIndex >= len(r.Groups) {
					t.Fatal("out-of-range dictionary identity")
				}
				if _, exists := members[member.QueryID]; exists {
					t.Fatal("duplicate query identity")
				}
				s := r.Sources[member.SourceIndex]
				members[member.QueryID] = tracecatalog.RepresentativeMember{QueryID: member.QueryID, ArtifactID: s.ArtifactID, Path: s.Path, ArtifactRevision: s.ArtifactRevision, SourceRevision: s.SourceRevision, SourceStatus: s.SourceStatus, Selected: member.Selected, SelectionReason: member.SelectionReason, Outcome: member.Outcome, Attempts: member.Attempts, LatestAttempt: member.LatestAttempt}
				navigation[member.QueryID] = tracecatalog.RepresentativeQueryNavigation{Specification: r.Groups[member.GroupIndex].Specification, Origin: member.Origin, AttemptOutcomes: member.AttemptOutcomes}
			}
		}
	}
	return members, navigation
}
