package tracediag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Keep historical schema witnesses meaningful: remove exactly this optional
// addition first, never bless changes to unrelated old fields or tags.
func nonEventSchemaBeforeIOInFlight(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.WindowStats{}) {
		return schema
	}
	schema = nonEventSchemaBeforeSchedulerConcurrency(t, typ, schema)
	const added = "IOInFlight|*tracequery.IOInFlightStats|io_inflight,omitempty"
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		t.Fatalf("expected one reviewed optional IO field, got %d: %s", count, schema)
	}
	return strings.Join(previous, ";")
}

func TestIOInFlightRenderPreservesMeasuredZeroAndNativePrecision(t *testing.T) {
	for _, end := range []string{"0.000000000", "0.000000001"} {
		t.Run(end, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "zero.systrace")
			body := "io-40 (40) [003] .... 0.000000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]\n" +
				fmt.Sprintf("irq-2 (2) [003] .... %s: block_rq_complete: 8,0 R () 123 + 8 [0]\n", end)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			result := tracequery.Run(idx, tracequery.Query{View: "window_stats", TimeStart: 0, TimeEnd: .000000010, TimeStartSet: true, TimeEndSet: true})
			if result.WindowStats == nil || result.WindowStats.IOInFlight == nil || len(result.WindowStats.IOInFlight.Groups) != 1 || result.WindowStats.IOInFlight.Groups[0].Values == nil {
				t.Fatalf("real pair must first produce measured native values: %+v", result.WindowStats)
			}
			native := result.WindowStats.IOInFlight.Groups[0]
			before, _ := json.Marshal(result)
			full := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: &result})
			report := strings.Join(full.lines, "\n")
			wantValues := fmt.Sprintf("window_stats.io_inflight.groups[0].values: peak_requests=%d mean_requests=%s busy_ms=%s request_ms=%s", native.Values.PeakRequests,
				formatFloatToken(native.Values.MeanRequests), formatFloatToken(native.Values.BusyMs), formatFloatToken(native.Values.RequestMs))
			wants := []string{wantValues, "window_stats.io_inflight.window: start_ts=0 end_ts=0.00000001"}
			for i, segment := range native.Segments {
				wants = append(wants, fmt.Sprintf("window_stats.io_inflight.groups[0].segments[%d]: start_ts=%s end_ts=%s requests=%d", i,
					strconv.FormatFloat(segment.StartTs, 'f', -1, 64), strconv.FormatFloat(segment.EndTs, 'f', -1, 64), segment.Requests))
			}
			for _, want := range wants {
				if !strings.Contains(report, want) {
					t.Errorf("native zero/precision lost: want %q\n%s", want, report)
				}
			}
			after, _ := json.Marshal(result)
			if !bytes.Equal(before, after) {
				t.Fatal("zero-preserving display mutated the native measurement")
			}
		})
	}
	t.Run("unavailable_is_not_zero", func(t *testing.T) {
		result := &tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{IOInFlight: &tracequery.IOInFlightStats{
			Groups: []tracequery.IOInFlightGroup{{Layer: "block", EndpointFamily: "block_rq", IssueCount: 1, ValuesUnavailableReason: "no_accepted_complete_pairs"}},
		}}}
		report := strings.Join(renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: result}).lines, "\n")
		if !strings.Contains(report, "no_accepted_complete_pairs") || strings.Contains(report, "peak_requests") || strings.Contains(report, ".values:") {
			t.Fatalf("nil measurement acquired a numeric zero: %s", report)
		}
	})
}

func TestIOInFlightSchemaEvolutionIsAdditive(t *testing.T) {
	typ := reflect.TypeOf(tracequery.WindowStats{})
	_, schema := detailSchemaFingerprint(typ)
	previous := nonEventSchemaBeforeIOInFlight(t, typ, schema)
	sum := sha256.Sum256([]byte(previous))
	const want = "ba9df90dfb29d8ec606633961a517d5553522d7d2d620b2ce07b11b4eb6338f1"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("IO addition changed the old WindowStats schema: got=%s want=%s", got, want)
	}
}

func TestIOInFlightBulkDoesNotPreemptExistingMeasurementDetails(t *testing.T) {
	for _, measured := range []bool{true, false} {
		t.Run(map[bool]string{true: "measured_profile", false: "unavailable_coverage"}[measured], func(t *testing.T) {
			res := &tracequery.Result{View: "window_stats", WindowStats: &tracequery.WindowStats{
				Window:        tracequery.TimeWindow{StartTs: 1, EndTs: 2},
				TopRunning:    []tracequery.ThreadDuration{{Thread: tracequery.ThreadRef{PID: 40, Comm: "worker"}, DurationMs: 57.828, CPU: 12}},
				ComputeSupply: []tracequery.ComputeSupplySummary{{Thread: tracequery.ThreadRef{PID: 40, Comm: "worker"}, Summary: "existing frequency donor evidence"}},
			}}
			baseline := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
			res.WindowStats.IOInFlight = &tracequery.IOInFlightStats{
				Population: tracequery.IOInFlightPopulationCompletePairs, IssuerScope: tracequery.IOInFlightIssuerScopeAll,
				Coverage: []tracequery.IOInFlightPairingCoverage{{Family: "block", Status: tracequery.IOInFlightCoverageUnavailable, Reasons: []string{"pairing_topology_incomplete"}}},
			}
			if measured {
				res.WindowStats.IOInFlight.Window = &tracequery.IOInFlightWindow{StartTs: 1, EndTs: 2}
				res.WindowStats.IOInFlight.Coverage[0] = tracequery.IOInFlightPairingCoverage{Family: "block", Status: tracequery.IOInFlightCoverageAvailable, TopologyComplete: true, AcceptedPairCount: 1}
				res.WindowStats.IOInFlight.GroupCount = 1
				res.WindowStats.IOInFlight.Groups = []tracequery.IOInFlightGroup{{SourcePath: "/capture/a.systrace", Layer: "block", EndpointFamily: "block_rq", Dev: "8,0", Operation: "R", AcceptedPairCount: 1, IssueCount: 1,
					Values: &tracequery.IOInFlightValues{PeakRequests: 1, MeanRequests: 1, BusyMs: 1000, RequestMs: 1000}, Segments: []tracequery.IOInFlightSegment{{StartTs: 1, EndTs: 2, Requests: 1}},
				}}
			}
			before, _ := json.Marshal(res)
			bounded := renderStepBody(&Step{View: "window_stats", effMaxLines: len(baseline.lines)}, stepOutcome{result: res})
			if !reflect.DeepEqual(bounded.lines, baseline.lines) {
				t.Fatalf("new nested bulk displaced prior measurement details under the same cap:\n%s\nwant:\n%s", strings.Join(bounded.lines, "\n"), strings.Join(baseline.lines, "\n"))
			}
			full := renderStepBody(&Step{View: "window_stats", effMaxLines: 1000}, stepOutcome{result: res})
			if bounded.total != full.total || bounded.total <= len(bounded.lines) || !strings.Contains(strings.Join(full.lines, "\n"), "window_stats.io_inflight") {
				t.Fatalf("deferred IO details vanished or lost cap accounting: bounded=%+v full=%+v", bounded, full)
			}
			after, _ := json.Marshal(res)
			if !bytes.Equal(before, after) {
				t.Fatal("display scheduling mutated the native result")
			}
		})
	}
}
