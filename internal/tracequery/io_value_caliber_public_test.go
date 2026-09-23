package tracequery

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The public parser/query path deliberately measures different physical
// intervals: request 0.100ms, completion-closed issuer wait 0.090ms.
func ioValueCaliberIndex(t *testing.T, family, state string, closed bool) *Index {
	t.Helper()
	issue, complete := "block_rq_issue", "block_rq_complete"
	issueFields, completeFields := "8,0 R 4096 () 123 + 8 [issuer]", "8,0 R () 123 + 8 [0]"
	if family == "bio" {
		issue, complete = "block_bio_queue", "block_bio_complete"
		issueFields, completeFields = "8,0 R 123 + 8 [issuer]", "8,0 R 123 + 8 [0]"
	}
	var body strings.Builder
	body.WriteString("idle-0 (0) [000] .... 4.999900: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unrelated next_pid=99 next_prio=20\n")
	body.WriteString("idle-0 (0) [001] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=issuer next_pid=41 next_prio=20\n")
	fmt.Fprintf(&body, "issuer-41 (41) [001] .... 5.000100: %s: %s\n", issue, issueFields)
	fmt.Fprintf(&body, "issuer-41 (41) [001] .... 5.000120: sched_switch: prev_comm=issuer prev_pid=41 prev_prio=20 prev_state=%s ==> next_comm=idle next_pid=0 next_prio=120\n", state)
	fmt.Fprintf(&body, "irq-2 (2) [001] .... 5.000200: %s: %s\n", complete, completeFields)
	if closed {
		body.WriteString("irq-2 (2) [001] .... 5.000210: sched_wakeup: comm=issuer pid=41 prio=20 target_cpu=001\n")
	}
	body.WriteString("idle-0 (0) [001] .... 5.000230: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=issuer next_pid=41 next_prio=20\n")
	body.WriteString("unrelated-99 (99) [000] .... 5.001000: sched_switch: prev_comm=unrelated prev_pid=99 prev_prio=20 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n")
	return buildTraceIndex(t, "io_value_caliber.systrace", body.String())
}

func TestIOValueCaliberPublicClippedWindow(t *testing.T) {
	for _, pid := range []int{41, 99} {
		t.Run(fmt.Sprint(pid), func(t *testing.T) {
			idx := ioValueCaliberIndex(t, "rq", "S", true)
			result := Run(idx, Query{View: "root_cause_rank", PID: pid, TimeStart: 5.00014, TimeEnd: 5.001, MinDurationMs: .001, Limit: 64})
			if result.RootCauseRank == nil {
				t.Fatal("missing rank")
			}
			found := false
			for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
				if row.Type != "io_latency" || row.Thread.PID != 41 {
					continue
				}
				found = true
				caliber, projected, actual := "block_rq_issue_to_complete", .060, .100
				if pid == 41 {
					caliber, projected, actual = "completion_closed_issuer_blocked", .070, .090
				}
				assertIOValueCaliberJSON(t, row, caliber)
				if !near(row.CumulativeImpactMs, projected, .000001) || !near(row.ActualImpactMs, actual, .000001) || !row.ResourceCompletionClosure {
					t.Errorf("window projection altered native value/proof: %+v", row)
				}
				if (row.ChainRelevance == "on_chain") != (pid == 41) {
					t.Errorf("window crop or IO ruler lent chain eligibility: %+v", row)
				}
			}
			if !found {
				t.Fatal("clipped public query lost native IO")
			}
		})
	}
}

func TestIOValueCaliberPublicNativeFamily(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed_%t", mixed), func(t *testing.T) {
			body := "idle-0 (0) [000] .... 5.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=99 next_prio=20\n" +
				"issuer-41 (41) [001] .... 5.000100: block_rq_issue: 8,0 R 4096 () 123 + 8 [issuer]\n" +
				"irq-2 (2) [001] .... 5.000200: block_rq_complete: 8,0 R () 123 + 8 [0]\n"
			if mixed {
				body += "issuer-41 (41) [001] .... 5.000300: block_bio_queue: 8,0 R 456 + 8 [issuer]\nirq-2 (2) [001] .... 5.000500: block_bio_complete: 8,0 R 456 + 8 [0]\n"
			} else {
				body += "issuer-41 (41) [001] .... 5.000300: block_rq_issue: 8,0 R 4096 () 456 + 8 [issuer]\nirq-2 (2) [001] .... 5.000500: block_rq_complete: 8,0 R () 456 + 8 [0]\n"
			}
			body += "target-99 (99) [000] .... 5.001000: sched_switch: prev_comm=target prev_pid=99 prev_prio=20 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120\n"
			result := Run(buildTraceIndex(t, "family.systrace", body), Query{View: "root_cause_rank", PID: 99, TimeStart: 5, TimeEnd: 5.001, MinDurationMs: .001, Limit: 64})
			if result.RootCauseRank == nil || result.WindowStats == nil || len(result.WindowStats.IOLatencies) != 2 {
				t.Fatal("public family fixture requires two exact physical requests")
			}
			want := "block_rq_issue_to_complete"
			if mixed {
				want = "mixed"
			}
			found := 0
			for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
				if row.Type != "io_latency" {
					continue
				}
				found++
				assertIOValueCaliberJSON(t, row, want)
				if row.MemberCount != 2 || !near(row.CumulativeImpactMs, .300, .000001) || row.ResourceCompletionClosure || row.ChainRelevance != "background" {
					t.Errorf("fold changed value or promoted background IO: %+v", row)
				}
			}
			if found != 1 {
				t.Fatalf("want one family, got %d", found)
			}
		})
	}
}

func TestIOValueCaliberSameTypeFoldPreservesAllOtherFields(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"block_rq_issue_to_complete", "block_rq_issue_to_complete", "block_rq_issue_to_complete"},
		{"completion_closed_issuer_blocked", "completion_closed_issuer_blocked", "completion_closed_issuer_blocked"},
		{"block_bio_queue_to_complete", "block_bio_queue_to_complete", "block_bio_queue_to_complete"},
		{"block_rq_issue_to_complete", "block_bio_queue_to_complete", "mixed"},
		{"block_rq_issue_to_complete", "completion_closed_issuer_blocked", "mixed"},
		{"", "completion_closed_issuer_blocked", "mixed"},
		{"completion_closed_issuer_blocked", "", "mixed"},
		{"future_unknown", "block_bio_queue_to_complete", "mixed"},
		{"", "future_unknown", ""},
		{"mixed", "completion_closed_issuer_blocked", "mixed"},
	}
	for _, tc := range cases {
		t.Run(tc.a+"/"+tc.b, func(t *testing.T) {
			thread := ThreadRef{Comm: "issuer", PID: 41}
			members := []RootCauseRankItem{
				rcmRankItem("io_latency", thread, 2, 10.010, 10.012, 100, 101),
				rcmRankItem("io_latency", thread, 1, 10.020, 10.021, 102, 103),
			}
			q := Query{TimeStart: 10, TimeEnd: 10.2}
			baseline := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), members...))
			members[0].IOValueCaliber, members[1].IOValueCaliber = tc.a, tc.b
			got := foldSameThreadTypeRankFamilies(q, false, members)
			if len(got) != 1 {
				t.Fatalf("expected one fold, got %d", len(got))
			}
			assertIOValueCaliberJSON(t, got[0], tc.want)
			second := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), got...))
			if !reflect.DeepEqual(got, second) {
				t.Fatal("re-fold must preserve the complete typed family")
			}
			got[0].IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("IO caliber must not change value, ranking, source proof or any other fold field")
			}
		})
	}
}

func TestIOValueCaliberFacetFoldUsesOnlyNumericalContributors(t *testing.T) {
	for _, withOtherUnion := range []bool{false, true} {
		t.Run(fmt.Sprintf("other_union_%t", withOtherUnion), func(t *testing.T) {
			members := []RootCauseRankItem{
				uxr1FacetItem("io_latency", .099, 10.01000, 10.010099, 120, ""),
				uxr1FacetItem("page_cache_churn", .600, 10.00990, 10.01020, 100, "inode=one"),
			}
			if withOtherUnion {
				members = append(members, uxr1FacetItem("io_wait", .091, 10.01000, 10.010091, 140, ""))
			}
			baseline := RootCauseRankResult{Items: append([]RootCauseRankItem(nil), members...)}
			reconcileAdjacentIOFacetFamilySeats(&baseline)
			members[0].IOValueCaliber = "block_rq_issue_to_complete"
			got := RootCauseRankResult{Items: members}
			reconcileAdjacentIOFacetFamilySeats(&got)
			if len(got.Items) != 1 || got.Items[0].Source != adjacentIOFacetFamilySource {
				t.Fatalf("expected one facet family: %+v", got.Items)
			}
			want := "block_rq_issue_to_complete"
			if withOtherUnion {
				want = "mixed"
			}
			assertIOValueCaliberJSON(t, got.Items[0], want)
			for _, row := range got.AbsorbedItems {
				if row.Type == "io_latency" {
					assertIOValueCaliberJSON(t, row, "block_rq_issue_to_complete")
				}
			}
			twice := RootCauseRankResult{Items: append([]RootCauseRankItem(nil), got.Items...), AbsorbedItems: append([]RootCauseRankItem(nil), got.AbsorbedItems...)}
			reconcileAdjacentIOFacetFamilySeats(&twice)
			assertIOValueCaliberJSON(t, twice.Items[0], want)
			for i := range got.Items {
				got.Items[i].IOValueCaliber = ""
			}
			for i := range got.AbsorbedItems {
				got.AbsorbedItems[i].IOValueCaliber = ""
			}
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("facet caliber must not change numerical fields, proof or absorption")
			}
		})
	}
}

func TestIOValueCaliberMaxFoldKeepsSelectedValueOwner(t *testing.T) {
	for _, tc := range []struct {
		name, a, b, want string
		aMS, bMS         float64
		reverse          bool
	}{
		{"first_larger", "block_rq_issue_to_complete", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", 5, 3, false},
		{"second_larger", "block_rq_issue_to_complete", "completion_closed_issuer_blocked", "completion_closed_issuer_blocked", 3, 5, false},
		{"tie_keeps_earlier_line", "block_rq_issue_to_complete", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", 5, 5, false},
		{"tie_reversed_keeps_earlier_line", "block_rq_issue_to_complete", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", 5, 5, true},
		{"unknown_larger_cannot_borrow", "future_unknown", "completion_closed_issuer_blocked", "", 5, 3, false},
		{"unknown_smaller_cannot_pollute", "block_rq_issue_to_complete", "", "block_rq_issue_to_complete", 5, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			thread := ThreadRef{Comm: "issuer", PID: 41}
			members := []RootCauseRankItem{
				rcmRankItem("io_latency", thread, tc.aMS, 10.010, 10.012, 100, 101),
				rcmRankItem("io_latency", thread, tc.bMS, 10.011, 10.013, 102, 103),
			}
			q := Query{TimeStart: 10, TimeEnd: 10.2}
			baseline := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), members...))
			members[0].IOValueCaliber, members[1].IOValueCaliber = tc.a, tc.b
			if tc.reverse {
				members[0], members[1] = members[1], members[0]
			}
			got := foldSameThreadTypeRankFamilies(q, false, members)
			if len(got) != 1 || got[0].MemberFoldCaliber != RootCauseMemberFoldCaliberMaxOverlapFallback {
				t.Fatalf("fixture must use unchanged maximum fallback: %+v", got)
			}
			assertIOValueCaliberJSON(t, got[0], tc.want)
			got[0].IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("IO caliber must not change maximum selection, tie-break, value or any other field")
			}
		})
	}
}

func TestIOValueCaliberSumIgnoresValuelessMembers(t *testing.T) {
	for _, tc := range []struct {
		name, a, b, want string
		aMS, bMS         float64
	}{
		{"zero_unknown", "block_rq_issue_to_complete", "", "block_rq_issue_to_complete", 2, 0},
		{"zero_other_ruler", "block_rq_issue_to_complete", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", 2, 0},
		{"zero_seed", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", "block_rq_issue_to_complete", 0, 2},
		{"all_zero", "completion_closed_issuer_blocked", "block_rq_issue_to_complete", "", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			thread := ThreadRef{Comm: "issuer", PID: 41}
			members := []RootCauseRankItem{
				rcmRankItem("io_latency", thread, tc.aMS, 10.010, 10.012, 100, 101),
				rcmRankItem("io_latency", thread, tc.bMS, 10.020, 10.022, 102, 103),
			}
			q := Query{TimeStart: 10, TimeEnd: 10.2}
			baseline := foldSameThreadTypeRankFamilies(q, false, append([]RootCauseRankItem(nil), members...))
			members[0].IOValueCaliber, members[1].IOValueCaliber = tc.a, tc.b
			got := foldSameThreadTypeRankFamilies(q, false, members)
			if len(got) != 1 || got[0].MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
				t.Fatalf("fixture must retain original disjoint sum: %+v", got)
			}
			assertIOValueCaliberJSON(t, got[0], tc.want)
			got[0].IOValueCaliber = ""
			if !reflect.DeepEqual(got, baseline) {
				t.Fatal("ignoring valueless ruler must not change any measurement or proof")
			}
		})
	}
}

func assertIOValueCaliberJSON(t *testing.T, row any, want string) {
	t.Helper()
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if got, _ := fields["io_value_caliber"].(string); got != want {
		t.Errorf("public JSON lost the selected value ruler: got %q, want %q: %s", got, want, b)
	}
}

func TestIOValueCaliberPublicNativeQueries(t *testing.T) {
	for _, family := range []string{"rq", "bio"} {
		for _, state := range []string{"S", "D"} {
			for _, closed := range []bool{false, true} {
				for _, pid := range []int{41, 99} {
					name := fmt.Sprintf("%s/%s/closed_%t/target_%d", family, state, closed, pid)
					t.Run(name, func(t *testing.T) {
						idx := ioValueCaliberIndex(t, family, state, closed)
						q := Query{View: "root_cause_rank", PID: pid, TimeStart: 5, TimeEnd: 5.001, MinDurationMs: .001, Limit: 64}
						result := Run(idx, q)
						if result.WindowStats == nil || result.RootCauseRank == nil || len(result.WindowStats.IOLatencies) != 1 {
							t.Fatal("real parser/Run did not produce the exact native IO pair")
						}
						io := result.WindowStats.IOLatencies[0]
						if io.CompletionWokeIssuer != closed || fmt.Sprintf("%.3f", io.DurationMs) != "0.100" {
							t.Fatalf("native measurement/proof premise changed: %+v", io)
						}
						residence := "block_rq_issue_to_complete"
						if family == "bio" {
							residence = "block_bio_queue_to_complete"
						}
						rankCaliber, rankMS := residence, "0.100"
						if closed && pid == 41 {
							rankCaliber, rankMS = "completion_closed_issuer_blocked", "0.090"
						}
						rankCount := 0
						for _, row := range append(result.RootCauseRank.Items, result.RootCauseRank.AbsorbedItems...) {
							if row.Type != "io_latency" || row.Thread.PID != 41 {
								continue
							}
							rankCount++
							assertIOValueCaliberJSON(t, row, rankCaliber)
							if fmt.Sprintf("%.3f", row.CumulativeImpactMs) != rankMS || row.ResourceCompletionClosure != closed {
								t.Errorf("caliber must not alter rank measurement/proof: %+v", row)
							}
							if (row.ChainRelevance == "on_chain") != (closed && pid == 41) {
								t.Errorf("caliber changed causal admission: %+v", row)
							}
							if pid == 99 && strings.Contains(row.Summary, "issuing chain thread") {
								t.Error("off-chain completion proof falsely labels issuer as chain member")
							}
						}
						q.View = "critical_blocking_calls"
						blocking := Run(idx, q)
						if blocking.CriticalBlocking == nil {
							t.Fatal("public critical-blocking query produced no result")
						}
						blockCaliber, blockMS := residence, "0.100"
						if closed {
							blockCaliber, blockMS = "completion_closed_issuer_blocked", "0.090"
						}
						blockCount := 0
						for _, row := range blocking.CriticalBlocking.Items {
							if row.Type != "io_latency" || row.Thread.PID != 41 {
								continue
							}
							blockCount++
							assertIOValueCaliberJSON(t, row, blockCaliber)
							if fmt.Sprintf("%.3f", row.DurationMs) != blockMS || row.ResourceCompletionClosure != closed {
								t.Errorf("caliber must not alter blocking measurement/proof: %+v", row)
							}
						}
						if rankCount != 1 || blockCount != 1 {
							t.Fatalf("expected one physical IO row per view: rank=%d blocking=%d", rankCount, blockCount)
						}
					})
				}
			}
		}
	}
}
