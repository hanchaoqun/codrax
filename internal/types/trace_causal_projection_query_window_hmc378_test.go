package types

import (
	"fmt"
	"strings"
	"testing"
)

// Trace-span rows of one thread in a capped context bucket. A row read by a
// line query carries no query window; the same thread's row read by a time
// query carries one.

func hmc378SpanRow(id, subject string, start, end, impact, windowStart, windowEnd float64) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		Subject: subject, Object: "trace_span", SpanName: id, EvidenceID: "ev-" + id,
		StartTs: start, EndTs: end, ImpactMS: impact, CumulativeImpactMS: impact,
		QueryWindowStartTs: windowStart, QueryWindowEndTs: windowEnd, ChainRelevance: "adjacent",
	}
}

func hmc378OtherThreads(n int) []TraceCausalProjectionNode {
	rows := make([]TraceCausalProjectionNode, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, hmc378SpanRow(fmt.Sprintf("o%d", i), fmt.Sprintf("worker-%d", i), 100, 100.1, float64(90-i), 99, 200))
	}
	return rows
}

// hmc378Show prints the bucket: a published row by its span name, the counted
// fold as FOLD[n].
func hmc378Show(rows []TraceCausalProjectionNode) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Subject) == "" {
			parts = append(parts, fmt.Sprintf("FOLD[%d]", row.MergedCount))
			continue
		}
		parts = append(parts, row.SpanName)
	}
	return strings.Join(parts, " | ")
}

// hmc378Accounted checks that no input row left the bucket without a count:
// each evidence id is on a published row or absorbed by the one fold, and the
// published rows plus the fold's count are the input rows.
func hmc378Accounted(t *testing.T, input, output []TraceCausalProjectionNode) {
	t.Helper()
	seen := map[string]int{}
	published, folded, folds := 0, 0, 0
	for _, row := range output {
		if strings.TrimSpace(row.Subject) != "" {
			published++
			seen[row.EvidenceID]++
			continue
		}
		folds++
		folded += row.MergedCount
		seen[row.EvidenceID]++
		for _, id := range row.MergedEvidenceIDs {
			seen[id]++
		}
	}
	if folds > 1 {
		t.Fatalf("want at most one fold row, got %d: %s", folds, hmc378Show(output))
	}
	if published+folded != len(input) {
		t.Fatalf("published %d + folded %d != %d input rows: %s", published, folded, len(input), hmc378Show(output))
	}
	for _, row := range input {
		if seen[row.EvidenceID] != 1 {
			t.Fatalf("row %s is accounted %d times: %s", row.SpanName, seen[row.EvidenceID], hmc378Show(output))
		}
	}
}

// A row inside a longer row of its thread takes no seat from another thread,
// whichever of the two rows carries the query window. Two rows that each carry
// a different query window stay two readings.
func TestHMC378WindowlessPhaseTakesNoContextSeat(t *testing.T) {
	const beside = "coll | o0 | o1 | FOLD[1] | phase"
	for _, c := range []struct {
		name                                                             string
		collWindowStart, collWindowEnd, phaseWindowStart, phaseWindowEnd float64
		phaseFirst                                                       bool
		want                                                             string
	}{
		{"same-window", 99, 200, 99, 200, false, beside},
		{"near-window", 99, 200, 99.0004, 200, false, beside},
		{"phase-without-window", 99, 200, 0, 0, false, beside},
		{"collection-without-window", 0, 0, 99, 200, false, beside},
		{"neither-has-a-window", 0, 0, 0, 0, false, beside},
		{"phase-without-window-sorted-first", 99, 200, 0, 0, true, beside},
		{"two-different-windows", 99, 200, 99.05, 200, false, "coll | phase | o0 | FOLD[2]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			coll := hmc378SpanRow("coll", "gc", 100, 100.2, 200, c.collWindowStart, c.collWindowEnd)
			phase := hmc378SpanRow("phase", "gc", 100.01, 100.19, 180, c.phaseWindowStart, c.phaseWindowEnd)
			rows := []TraceCausalProjectionNode{coll, phase}
			if c.phaseFirst {
				rows = []TraceCausalProjectionNode{phase, coll}
			}
			rows = append(rows, hmc378OtherThreads(3)...)
			got := traceCausalProjectionLimitContextNodesFold(rows, 3, "adjacent")
			if show := hmc378Show(got); show != c.want {
				t.Fatalf("bucket = %s, want %s", show, c.want)
			}
			hmc378Accounted(t, rows, got)
		})
	}
}

// With its collection cut at the cap, the row inside it folds with it. The
// fold counts both rows and keeps both evidence ids and their thread, so the
// phase is counted, never dropped.
func TestHMC378PhaseFoldedWithItsCollectionIsCounted(t *testing.T) {
	phase := hmc378SpanRow("phase", "gc", 100.01, 100.19, 180, 0, 0)
	coll := hmc378SpanRow("coll", "gc", 100, 100.2, 200, 99, 200)
	rows := append([]TraceCausalProjectionNode{phase}, hmc378OtherThreads(3)...)
	rows = append(rows, coll)
	got := traceCausalProjectionLimitContextNodesFold(rows, 3, "adjacent")
	if show := hmc378Show(got); show != "o0 | o1 | o2 | FOLD[2]" {
		t.Fatalf("bucket = %s, want the three other threads seated and one fold of two", show)
	}
	hmc378Accounted(t, rows, got)
	fold := got[len(got)-1]
	ids := strings.Join(append([]string{fold.EvidenceID}, fold.MergedEvidenceIDs...), ",")
	if fold.MergedCount != 2 || !strings.Contains(ids, "ev-coll") || !strings.Contains(ids, "ev-phase") {
		t.Fatalf("the fold must count the collection and its phase with their evidence: count=%d ids=%s", fold.MergedCount, ids)
	}
	if len(fold.MergedSubjects) != 1 || fold.MergedSubjects[0] != "gc" {
		t.Fatalf("the fold must name the folded thread: %v", fold.MergedSubjects)
	}
	if fold.OnChainOverflowFold || fold.ChainRelevance != "adjacent" || fold.MergedMaxMS != 200 {
		t.Fatalf("the fold keeps the bucket's counted form and the member maximum: %+v", fold)
	}
}

// The containment check of the capped bucket and the nesting check of the
// same-kind merge read the query window the same way.
func TestHMC378ContainmentAndNestingReadOneQueryWindowRule(t *testing.T) {
	for _, c := range []struct {
		name                                               string
		aWindowStart, aWindowEnd, bWindowStart, bWindowEnd float64
		want                                               bool
	}{
		{"same-window", 99, 200, 99, 200, true},
		{"near-window", 99, 200, 99.0004, 200.0004, true},
		{"inner-without-window", 99, 200, 0, 0, true},
		{"outer-without-window", 0, 0, 99, 200, true},
		{"neither-has-a-window", 0, 0, 0, 0, true},
		{"start-differs", 99, 200, 99.05, 200, false},
		{"end-differs", 99, 200, 99, 200.05, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			outer := hmc378SpanRow("coll", "gc", 100, 100.2, 200, c.aWindowStart, c.aWindowEnd)
			inner := hmc378SpanRow("phase", "gc", 100.01, 100.19, 180, c.bWindowStart, c.bWindowEnd)
			contained := traceCausalProjectionSpanRowContainedByAny(inner, []TraceCausalProjectionNode{outer})
			nest := traceCausalProjectionSpanRowsNest(outer, inner)
			if contained != c.want || nest != c.want {
				t.Fatalf("contained=%v nest=%v, want both %v", contained, nest, c.want)
			}
		})
	}
}

// A phase is shown when the row that contains it is shown. A row without a
// query window links a row of one window to a row of another, so the innermost
// row is decided through the phase that contains it.
func TestHMC378PhaseOfAShownPhaseIsShown(t *testing.T) {
	rows := []TraceCausalProjectionNode{
		hmc378SpanRow("coll", "gc", 100, 100.3, 300, 99, 200),
		hmc378SpanRow("phase", "gc", 100.01, 100.2, 190, 0, 0),
		hmc378SpanRow("step", "gc", 100.02, 100.1, 80, 99.5, 200),
	}
	rows = append(rows, hmc378OtherThreads(3)...)
	got := traceCausalProjectionLimitContextNodesFold(rows, 3, "adjacent")
	if show := hmc378Show(got); show != "coll | o0 | o1 | FOLD[1] | phase | step" {
		t.Fatalf("bucket = %s, want the phase and its step shown beside the seated collection", show)
	}
	hmc378Accounted(t, rows, got)
}

// Every order, cap and query-window pattern of a three-level nest beside three
// other threads: no row leaves without a count, a contained row never takes a
// seat, and it is shown exactly when a shown row contains it.
func TestHMC378CappedBucketAccountsEveryRow(t *testing.T) {
	windows := [][2]float64{{0, 0}, {99, 200}, {99.5, 200}}
	base := []TraceCausalProjectionNode{
		hmc378SpanRow("coll", "gc", 100, 100.3, 300, 0, 0),
		hmc378SpanRow("phase", "gc", 100.01, 100.2, 190, 0, 0),
		hmc378SpanRow("step", "gc", 100.02, 100.1, 80, 0, 0),
	}
	base = append(base, hmc378OtherThreads(3)...)
	var orders [][]int
	var permute func(prefix, rest []int)
	permute = func(prefix, rest []int) {
		if len(rest) == 0 {
			orders = append(orders, append([]int(nil), prefix...))
			return
		}
		for i := range rest {
			next := append(append([]int(nil), rest[:i]...), rest[i+1:]...)
			permute(append(prefix, rest[i]), next)
		}
	}
	permute(nil, []int{0, 1, 2, 3, 4, 5})
	cases := 0
	for pattern := 0; pattern < 27; pattern++ {
		for _, order := range orders {
			rows := make([]TraceCausalProjectionNode, 0, len(order))
			for _, idx := range order {
				row := base[idx]
				if idx < 3 {
					digit := pattern
					for k := 0; k < idx; k++ {
						digit /= 3
					}
					row.QueryWindowStartTs, row.QueryWindowEndTs = windows[digit%3][0], windows[digit%3][1]
				}
				rows = append(rows, row)
			}
			seatable := 0
			for _, row := range rows {
				if !traceCausalProjectionSpanRowContainedByAny(row, rows) {
					seatable++
				}
			}
			for limit := 1; limit < len(rows); limit++ {
				cases++
				got := traceCausalProjectionLimitContextNodesFold(rows, limit, "adjacent")
				label := fmt.Sprintf("pattern=%d order=%v limit=%d: %s", pattern, order, limit, hmc378Show(got))
				hmc378Accounted(t, rows, got)
				var shown []TraceCausalProjectionNode
				for _, row := range got {
					if strings.TrimSpace(row.Subject) != "" {
						shown = append(shown, row)
					}
				}
				seated := 0
				for _, row := range shown {
					inside := traceCausalProjectionSpanRowContainedByAny(row, rows)
					beside := traceCausalProjectionSpanRowContainedByAny(row, shown)
					if !inside {
						seated++
						continue
					}
					if !beside {
						t.Fatalf("%s: %s is shown without the row that contains it", label, row.SpanName)
					}
				}
				want := limit
				if seatable < want {
					want = seatable
				}
				if seated != want {
					t.Fatalf("%s: %d rows seated, want %d (contained rows take no seat)", label, seated, want)
				}
				for _, row := range rows {
					if !traceCausalProjectionSpanRowContainedByAny(row, shown) {
						continue
					}
					present := false
					for _, s := range shown {
						present = present || s.EvidenceID == row.EvidenceID
					}
					if !present {
						t.Fatalf("%s: %s is inside a shown row and was folded", label, row.SpanName)
					}
				}
			}
		}
	}
	if cases != 27*720*5 {
		t.Fatalf("ran %d cases", cases)
	}
}
