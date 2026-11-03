package context

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func hmc335RosterFixture() (tracecatalog.RepresentativeSnapshot, map[string]tracecatalog.RepresentativeQueryNavigation) {
	spec := tracecatalog.QuerySpecification{Object: tracecatalog.Object{Kind: "thread", ID: "17"}, View: "thread_state", Parameters: json.RawMessage(`{"view":"thread_state","pid":17}`)}
	m := tracecatalog.RepresentativeMember{QueryID: "query-a", ArtifactID: "artifact", Path: "/same/path", ArtifactRevision: "revision-a", SourceRevision: "bytes-a", SourceStatus: "available", Selected: true, SelectionReason: "budget", Outcome: tracecatalog.OutcomeFailure, Attempts: 1, LatestAttempt: &tracecatalog.Attempt{ID: "attempt-a", Sequence: 1, Completion: tracecatalog.Completion{Outcome: tracecatalog.OutcomeFailure, SourceRevision: "bytes-a", Error: &tracecatalog.Issue{Code: "decode_failed", Message: "original failure"}}}}
	n := m
	n.QueryID, n.ArtifactRevision, n.SourceRevision, n.SourceStatus, n.Selected = "query-b", "revision-b", "bytes-b", "stale", false
	n.LatestAttempt = nil
	n.Attempts, n.Outcome = 0, tracecatalog.OutcomeNotExecuted
	s := tracecatalog.RepresentativeSnapshot{CatalogID: "catalog", SelectionID: "selection", NavigationOnly: true, Coverage: tracecatalog.RepresentativeCoverage{RepresentativeMemberCoverage: tracecatalog.RepresentativeMemberCoverage{Members: 2, Selected: 1}}, Clusters: []tracecatalog.RepresentativeCluster{{ID: "group", Specification: spec, Members: []tracecatalog.RepresentativeMember{m, n}}}}
	navigation := map[string]tracecatalog.RepresentativeQueryNavigation{
		m.QueryID: {Specification: spec, Origin: tracecatalog.QueryOriginDeclared, AttemptOutcomes: map[tracecatalog.Outcome]int{tracecatalog.OutcomeFailure: 1}},
		n.QueryID: {Specification: spec, Origin: tracecatalog.QueryOriginResumed, AttemptOutcomes: map[tracecatalog.Outcome]int{}},
	}
	return s, navigation
}

func TestHMC335RosterDictionaryExactSourceTuples(t *testing.T) {
	s, navigation := hmc335RosterFixture()
	before, _ := json.Marshal(s)
	content := formatCompleteRepresentativeRoster(s, navigation, "intro\n", 12<<10)
	if content == "" || !strings.Contains(content, "selected_member_records_displayed=1 omitted=0") || !strings.Contains(content, "unselected_member_records_displayed=1 omitted=0") {
		t.Fatalf("complete roster/census missing: %s", content)
	}
	var row struct {
		Roster struct {
			Sources []representativeRosterSource `json:"sources"`
			Groups  []representativeRosterGroup  `json:"groups"`
			Members []representativeRosterMember `json:"members"`
		} `json:"representative_roster"`
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, `{"catalog_id":`) {
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(row.Roster.Sources) != 2 || len(row.Roster.Groups) != 1 || len(row.Roster.Members) != 2 {
		t.Fatal("equal path merged different source generations or lost members")
	}
	for i, m := range row.Roster.Members {
		want := s.Clusters[0].Members[i]
		source := row.Roster.Sources[m.SourceIndex]
		if m.GroupIndex != 0 || m.SourceIndex != i || m.QueryID != want.QueryID || source.ArtifactRevision != want.ArtifactRevision || source.SourceRevision != want.SourceRevision || source.SourceStatus != want.SourceStatus || !reflect.DeepEqual(m.LatestAttempt, want.LatestAttempt) || !reflect.DeepEqual(m.AttemptOutcomes, navigation[m.QueryID].AttemptOutcomes) {
			t.Fatal("dictionary changed source-local original fields or stable member order")
		}
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("display mutated detached source")
	}
}

func TestHMC335RosterIncompleteAndWholeBudgetFallback(t *testing.T) {
	for _, change := range []string{"missing_join", "duplicate_identity", "specification_bytes", "wrong_count", "budget", "large_original_error"} {
		t.Run(change, func(t *testing.T) {
			s, navigation := hmc335RosterFixture()
			budget := 12 << 10
			switch change {
			case "missing_join":
				delete(navigation, "query-b")
			case "duplicate_identity":
				s.Clusters[0].Members[1].QueryID = "query-a"
			case "specification_bytes":
				n := navigation["query-b"]
				n.Specification.Parameters = json.RawMessage(`{"pid":17,"view":"thread_state"}`)
				navigation["query-b"] = n
			case "wrong_count":
				s.Coverage.Members++
			case "budget":
				budget = 1200
			case "large_original_error":
				s.Clusters[0].Members[0].LatestAttempt.Error.Message = strings.Repeat("unaltered-original-error", 1200)
			}
			before, _ := json.Marshal(s)
			if got := formatCompleteRepresentativeRoster(s, navigation, "intro\n", budget); got != "" {
				t.Fatalf("incomplete or oversized roster gained whole-roster claim: %s", got)
			}
			after, _ := json.Marshal(s)
			if string(before) != string(after) {
				t.Fatal("fallback rewrote original data")
			}
		})
	}
}
