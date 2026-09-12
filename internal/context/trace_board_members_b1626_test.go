package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceBoardMemberScopeContextB1626(t *testing.T) {
	var scope types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","source_quote":"1..2 and 4..5","time_windows":[{"time_start":1,"time_end":2,"source_quote":"1..2"},{"time_start":4,"time_end":5,"source_quote":"4..5"}]}`), &scope); err != nil {
		t.Fatal(err)
	}
	ledger := types.ObservationLedger{RuntimeArtifactScopeProfile: &scope, Records: []types.ObservationRecord{
		b1626BoardParentWindow(traceBoardDomainRecord("a", "/capture/a", "app-100", "a", "1..2", "worker-a", "1.000", "on_chain", 1), 1, 2),
		b1626BoardParentWindow(traceBoardDomainRecord("b", "/capture/a", "app-100", "b", "4..5", "worker-b", "2.000", "on_chain", 1), 4, 5),
		b1626BoardParentWindow(traceBoardDomainRecord("outside", "/capture/a", "app-100", "outside", "1..5", "supporting-worker", "4.000", "on_chain", 1), 1, 5),
	}}
	before, _ := json.Marshal(ledger)
	got := formatTraceRootCauseBoardFromLedger(ledger)
	for _, window := range [][2]float64{{1, 2}, {4, 5}, {1, 5}} {
		want := types.ResolveTraceQueryWindowScope(&scope, window[0], window[1]).Format("en")
		if !strings.Contains(got, want) {
			t.Errorf("per-board request/query scope missing %q:\n%s", want, got)
		}
	}
	for _, name := range []string{"worker-a", "worker-b", "supporting-worker"} {
		if !strings.Contains(got, name) {
			t.Errorf("do not discard the independently measured board %s", name)
		}
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) {
		t.Fatal("scope display changed input evidence")
	}
}

func b1626BoardParentWindow(record types.ObservationRecord, start, end float64) types.ObservationRecord {
	record.SourceRef.QueryWindowKnown = true
	record.SourceRef.QueryWindowStartTs, record.SourceRef.QueryWindowEndTs = start, end
	return record
}

func TestTraceBoardParentWindowProvenanceB1626(t *testing.T) {
	var scope types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","time_windows":[{"time_start":1,"time_end":5,"source_quote":"1..5"},{"time_start":2,"time_end":3,"source_quote":"2..3"}]}`), &scope); err != nil {
		t.Fatal(err)
	}
	// A recursive board of parent A has the same native endpoints as member B.
	// Native identity remains unchanged; only the parent can select a member.
	base := traceBoardDomainRecord("base", "/capture/a", "app-100", "same-params", "2..3", "recursive-worker", "7.125", "on_chain", 1)
	a := b1626BoardParentWindow(base, 1, 5)
	b := b1626BoardParentWindow(base, 2, 3)
	invalid := b1626BoardParentWindow(base, 3, 2)
	near := b1626BoardParentWindow(base, 2.000001, 3)
	for _, tc := range []struct {
		name    string
		records []types.ObservationRecord
		parent  [2]float64
	}{
		{"recursive A is not B", []types.ObservationRecord{a}, [2]float64{1, 5}},
		{"genuine B", []types.ObservationRecord{b}, [2]float64{2, 3}},
		{"same parent repeated", []types.ObservationRecord{b, b}, [2]float64{2, 3}},
		{"missing parent", []types.ObservationRecord{base}, [2]float64{}},
		{"invalid parent", []types.ObservationRecord{invalid}, [2]float64{}},
		{"conflicting parents on duplicate row", []types.ObservationRecord{a, b}, [2]float64{}},
		{"known and missing parent", []types.ObservationRecord{b, base}, [2]float64{}},
		{"known and invalid parent", []types.ObservationRecord{b, invalid}, [2]float64{}},
		{"near but not unanimous parent", []types.ObservationRecord{b, near}, [2]float64{}},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				records := append([]types.ObservationRecord(nil), tc.records...)
				if reverse {
					for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
						records[i], records[j] = records[j], records[i]
					}
				}
				ledger := types.ObservationLedger{RuntimeArtifactScopeProfile: &scope, Records: records}
				before, _ := json.Marshal(ledger)
				got := formatTraceRootCauseBoardFromLedger(ledger)
				wantScope := types.ResolveTraceQueryWindowScope(&scope, tc.parent[0], tc.parent[1])
				if tc.parent != [2]float64{} {
					wantScope = wantScope.ForWindow(2, 3)
				}
				want := wantScope.Format("en")
				if !strings.Contains(got, want) {
					t.Errorf("board selected a request member without unanimous parent provenance; want %q:\n%s", want, got)
				}
				if tc.parent != [2]float64{2, 3} && strings.Contains(got, "requested window 2/2") {
					t.Errorf("native recursive window must not elect member B:\n%s", got)
				}
				for _, unchanged := range []string{"query_window=`2.000000..3.000000`", "params=`same-params`", "identity_complete=true", "7.125ms", "#1 root-cause seat"} {
					if !strings.Contains(got, unchanged) {
						t.Errorf("existing board fact %q changed:\n%s", unchanged, got)
					}
				}
				if strings.Count(got, "recursive-worker · running") != 1 {
					t.Fatal("parent provenance must not change the existing same-board row deduplication")
				}
				after, _ := json.Marshal(ledger)
				if string(before) != string(after) {
					t.Fatal("parent scope display mutated observation JSON")
				}
			})
		}
	}
}

func TestTraceBoardParentWindowBeforeBudgetAndLegacyBytesB1626(t *testing.T) {
	var scope types.RuntimeArtifactScopeProfile
	if err := json.Unmarshal([]byte(`{"requested_scope":"explicit_time_window","time_windows":[{"time_start":1,"time_end":2,"source_quote":"1..2"},{"time_start":4,"time_end":5,"source_quote":"4..5"}]}`), &scope); err != nil {
		t.Fatal(err)
	}
	var records []types.ObservationRecord
	for i := 1; i <= traceBoardChainRowCap+1; i++ {
		r := traceBoardDomainRecord(fmt.Sprint(i), "/capture/a", "app-100", "same", "4..5", fmt.Sprintf("worker-%d", i), "1.000", "on_chain", i)
		r = b1626BoardParentWindow(r, 4, 5)
		if i > traceBoardChainRowCap {
			r = b1626BoardParentWindow(r, 1, 2)
		}
		records = append(records, r)
	}
	got := formatTraceRootCauseBoardFromLedger(types.ObservationLedger{RuntimeArtifactScopeProfile: &scope, Records: records})
	if strings.Contains(got, "requested window 2/2") || !strings.Contains(got, "actual query window is unknown") {
		t.Fatalf("a parent conflict after the row display budget must remain visible as unknown provenance:\n%s", got)
	}
	if strings.Count(got, " root-cause seat — ") != traceBoardChainRowCap || !strings.Contains(got, "+1 more seated rows") {
		t.Fatalf("original global display cap changed:\n%s", got)
	}
	start, end := 4.0, 5.0
	legacy := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "4..5"}
	withParents := formatTraceRootCauseBoardFromLedger(types.ObservationLedger{RuntimeArtifactScopeProfile: legacy, Records: records})
	for i := range records {
		records[i].SourceRef.QueryWindowKnown = false
		records[i].SourceRef.QueryWindowStartTs, records[i].SourceRef.QueryWindowEndTs = 0, 0
	}
	withoutParents := formatTraceRootCauseBoardFromLedger(types.ObservationLedger{RuntimeArtifactScopeProfile: legacy, Records: records})
	if withParents != withoutParents {
		t.Fatal("legacy single-window board bytes must not depend on the new parent provenance")
	}
}
