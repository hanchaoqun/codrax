package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Opt in with the authorized reference capture. This copies its exact bytes to
// an ordinary name, not a new collection or an invented binary/text conversion.
// The independent oracle uses physical raw rows and decimal timestamps; native
// parsing below must agree without increasing the existing whole-index budget.
func TestHMC268NamedGZIPTextRepresentativeCompleteWindowQuery(t *testing.T) {
	source := os.Getenv("CODRAX_TEST_GZIP_TRACE")
	if source == "" {
		t.Skip("set CODRAX_TEST_GZIP_TRACE to the authorized real capture")
	}
	const sourceSHA = "4d66880019663f81462fe5df1615da58b368b6c5225e211dae88600cfc5396e9"
	const decodedSHA = "f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572"
	before, err := filegeneration.FromPath(source)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(wire)
	if len(wire) != 3602086 || hex.EncodeToString(sum[:]) != sourceSHA {
		t.Fatal("opt-in capture differs from independent oracle")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "现场调度资料.data")
	if err := os.WriteFile(input, wire, 0400); err != nil {
		t.Fatal(err)
	}
	opts := Options{InputPath: input, OutputPath: filepath.Join(dir, "complete.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")}
	probe := ProbeInputRoutingCapability(context.Background(), input, opts.RuntimeAnchor)
	if probe.NativeReader != "trace_query" || probe.Status != "format_candidate" {
		t.Fatalf("ordinary named capture unavailable: %+v", probe)
	}
	result, err := PrepareFile(context.Background(), opts)
	if err != nil || result.TextTransport == nil || result.TextTransport.SourceSHA256 != sourceSHA || result.TextTransport.DecodedSHA256 != decodedSHA || result.OutputBytes != 54122686 {
		t.Fatalf("full byte transport lost identity: %+v %v", result, err)
	}
	file, err := os.Open(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	decodedBytes, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || decodedBytes != 54122686 || hex.EncodeToString(digest.Sum(nil)) != decodedSHA {
		t.Fatal("complete decoded bytes differ", copyErr, closeErr)
	}
	const start, end = 170438.0, 170438.1
	parsed := 0
	wakers := map[int]int{}
	wakerTGID := map[int]int{}
	metadata, err := tracequery.StreamScan(context.Background(), result.OutputPath, "", func(event tracequery.Event) bool {
		parsed++
		if event.Name == "sched_wakeup" && event.WakeePID == 3051 && event.Ts >= start && event.Ts < end {
			wakers[event.PID]++
			wakerTGID[event.PID] = event.TGID
		}
		return true
	})
	if err != nil || parsed != 429756 || metadata.ScannedLineCount != 429766 || len(metadata.Events) != 0 {
		t.Fatalf("full native streaming census differs: parsed=%d metadata=%+v err=%v", parsed, metadata, err)
	}
	if wakers[51299] != 2 || wakers[1021] != 3 || wakers[20975] != 3 || len(wakers) != 3 || wakerTGID[51299] != 3762 || wakerTGID[1021] != 908 || wakerTGID[20975] != 908 {
		t.Fatalf("native waker identity/count differs: tid=%v tgid=%v", wakers, wakerTGID)
	}
	_, err = tracequery.BuildIndex(context.Background(), result.OutputPath)
	var limited *tracequery.IndexEventLimitError
	if !errors.As(err, &limited) || limited.MaxEvents != 250000 {
		t.Fatalf("whole-index budget changed: %v", err)
	}
	window := tracequery.BuildOptions{TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true, TimePaddingBefore: 0.1, TimePaddingAfter: 0.1, AllowWindowedParse: true}
	queryPath, err := filepath.EvalSymlinks(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[tracequery.ThreadState]float64{tracequery.StateSSleep: 96.780, tracequery.StateRunnable: 1.088, tracequery.StateRunning: 2.132}
	var totals map[tracequery.ThreadState]float64
	for _, phase := range []string{"cold_window", "warm_window"} {
		idx, err := tracequery.BuildIndexWithOptions(context.Background(), result.OutputPath, window)
		if err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
		query := tracequery.Run(idx, tracequery.Query{View: "thread_timeline", PID: 3051, TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true, Limit: 100})
		if query.Timeline == nil || query.SourcePath != queryPath || query.TimeStart != start || query.TimeEnd != end {
			t.Fatalf("%s: timeline/window/source absent: %+v", phase, query)
		}
		totals = map[tracequery.ThreadState]float64{}
		for _, interval := range query.Timeline.Intervals {
			if interval.Thread.PID != 3051 || interval.StartTs < start || interval.EndTs > end {
				t.Fatalf("%s: source/window leaked: %+v", phase, interval)
			}
			totals[interval.State] += interval.DurationMs
		}
		if len(totals) != len(want) {
			t.Fatalf("%s: unexpected states: %v", phase, totals)
		}
		for state, expected := range want {
			if math.Abs(totals[state]-expected) > 0.00001 {
				t.Fatalf("%s: %s=%0.9fms expected=%0.9fms", phase, state, totals[state], expected)
			}
		}
	}
	after, err := filegeneration.FromPath(source)
	if err != nil || before.CacheToken() != after.CacheToken() {
		t.Fatal("original source generation or mode changed", err)
	}
	receipt, err := json.Marshal(map[string]any{"source_sha256": sourceSHA, "decoded_sha256": decodedSHA, "complete_bytes": decodedBytes, "parsed_events": parsed, "physical_lines": metadata.ScannedLineCount, "window_seconds": []float64{start, end}, "target_tid": 3051, "state_ms": totals, "waker_counts": wakers, "waker_tgid": wakerTGID, "whole_index_limit": limited.MaxEvents, "window_cold_warm": true, "original_unchanged": true})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(receipt))
}
