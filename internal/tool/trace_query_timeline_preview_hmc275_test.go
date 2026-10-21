package tool

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC275IOCompactPreviewPreservesPublishedTail(t *testing.T) {
	const body = "io-40 (40) [003] .... 1.002000: block_rq_issue: 8,0 W 4096 () 8 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.005000: block_rq_complete: 8,0 W () 8 + 8 [0]\n" +
		"io-40 (40) [003] .... 1.006000: block_rq_issue: 8,0 W 4096 () 16 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.009000: block_rq_complete: 8,0 W () 16 + 8 [0]\n"
	_, stats, row, semantic := ioInFlightPrecisionPublicQuery(t, body, 1, 1.01, 0, 0)
	group := stats.Groups[0]
	if len(group.Segments) != 5 || group.Segments[4].StartTs != 1.009 || group.Segments[4].EndTs != 1.01 || group.Segments[4].Requests != 0 {
		t.Fatalf("public native prerequisite: %+v", group.Segments)
	}
	note := traceQueryIOInFlightTimeline(group)
	assertHMC275CompactCoordinates(t, note, 5, 0, func(i int) string {
		s := group.Segments[i]
		return fmt.Sprintf("[%s,%s):%d", traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs), s.Requests)
	})
	for surface, text := range map[string]string{"producer": note, "observation": strings.Join(row.RichNotes, "\n"), "semantic": semantic} {
		if !strings.Contains(text, "[1.000000,1.002000):0") || !strings.Contains(text, "[1.009000,1.010000):0") {
			t.Errorf("%s lost a retained boundary: %s", surface, text)
		}
		if !strings.Contains(text, "sample_rows0=") {
			t.Errorf("%s omitted original row coordinates for noncontiguous preview", surface)
		}
		if !strings.Contains(text, note) {
			t.Errorf("%s did not preserve the exact selected rows/coordinates/omissions", surface)
		}
	}
	if len(types.TraceNoteKeyIOInFlightTimeline+"="+note) > 160 {
		t.Fatalf("preview exceeded existing note budget: %s", note)
	}
	projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: []types.ObservationRecord{row}})
	if projection.PrimaryRootCause != nil || len(projection.OnChainCauses)+len(projection.RankedSeats) != 0 {
		t.Fatal("display selection acquired causal authority")
	}
}

func assertHMC275CompactCoordinates(t *testing.T, value string, count, producerOmitted int, interval func(int) string) int {
	t.Helper()
	fields := strings.SplitN(value, "; ", 4)
	if len(fields) != 4 {
		t.Fatalf("malformed preview: %s", value)
	}
	var indices []string
	if rows := strings.TrimPrefix(fields[2], "sample_rows0="); rows != "" {
		indices = strings.Split(rows, ",")
	}
	var exact []string
	previous := -1
	for _, raw := range indices {
		i, err := strconv.Atoi(raw)
		if err != nil || i <= previous || i >= count {
			t.Fatalf("invalid original row order %q: %s", raw, value)
		}
		exact = append(exact, interval(i))
		previous = i
	}
	if fields[1] != fmt.Sprintf("omitted=%d", producerOmitted+count-len(indices)) || fields[3] != strings.Join(exact, ";") {
		t.Fatalf("preview does not exactly match its coordinates and omitted count: %s", value)
	}
	return len(indices)
}

func TestHMC275CompactTimelineSharedBudgetAndCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start float64
		rows  int
		want  int
	}{
		{"empty", 1, 0, 0}, {"single", 1, 1, 1}, {"both", 1, 2, 2},
		{"first_only", 1e20, 2, 1}, {"none_fit", 1e100, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			io := tracequery.IOInFlightGroup{OmittedSegments: 7}
			scheduler := tracequery.SchedulerConcurrencyGroup{OmittedSegments: 7}
			for i := 0; i < tc.rows; i++ {
				start, end := tc.start+float64(i), tc.start+float64(i+1)
				io.Segments = append(io.Segments, tracequery.IOInFlightSegment{StartTs: start, EndTs: end, Requests: i})
				scheduler.Segments = append(scheduler.Segments, tracequery.SchedulerConcurrencySegment{StartTs: start, EndTs: end, Threads: i})
			}
			for key, value := range map[string]string{
				types.TraceNoteKeyIOInFlightTimeline:           traceQueryIOInFlightTimeline(io),
				types.TraceNoteKeySchedulerConcurrencyTimeline: traceQuerySchedulerConcurrencyTimeline(scheduler),
			} {
				if len(key+"="+value) > 160 {
					t.Fatalf("exceeded note budget: %s=%s", key, value)
				}
				got := assertHMC275CompactCoordinates(t, value, tc.rows, 7, func(i int) string {
					s := io.Segments[i]
					return fmt.Sprintf("[%s,%s):%d", traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs), s.Requests)
				})
				if got != tc.want {
					t.Fatalf("%s selected %d rows, want %d: %s", key, got, tc.want, value)
				}
			}
		})
	}
}

func TestHMC275IOCompactPreviewSmallAndWideCoordinates(t *testing.T) {
	for _, n := range []int{0, 1, 2, 5, 100} {
		g := tracequery.IOInFlightGroup{OmittedSegments: 7}
		for i := 0; i < n; i++ {
			g.Segments = append(g.Segments, tracequery.IOInFlightSegment{StartTs: 170438 + float64(i)/1000, EndTs: 170438 + float64(i+1)/1000, Requests: i % 2})
		}
		text := traceQueryIOInFlightTimeline(g)
		assertHMC275CompactCoordinates(t, text, n, 7, func(i int) string {
			s := g.Segments[i]
			return fmt.Sprintf("[%s,%s):%d", traceQueryDisplaySeconds(s.StartTs), traceQueryDisplaySeconds(s.EndTs), s.Requests)
		})
		if len(types.TraceNoteKeyIOInFlightTimeline+"="+text) > 160 || !strings.Contains(text, "sample_rows0=") {
			t.Errorf("n=%d: unbounded or unlocated sample: %s", n, text)
		}
		if n == 0 && !strings.Contains(text, "omitted=7;") {
			t.Errorf("empty retained rows must preserve producer omissions: %s", text)
		}
		if n > 1 {
			tail := "[" + traceQueryDisplaySeconds(g.Segments[n-1].StartTs) + "," + traceQueryDisplaySeconds(g.Segments[n-1].EndTs) + ")"
			if !strings.Contains(text, tail) {
				t.Errorf("n=%d: fitting boundary omitted: %s", n, text)
			}
		}
	}
}
