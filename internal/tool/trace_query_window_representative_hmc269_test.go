package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Same authorized collection and independent raw-row oracle as HMC268, but
// through the real public tool instead of directly choosing windowed indexing.
func TestHMC269RealGZIPTextPublicWindowAndStreamingTail(t *testing.T) {
	source := os.Getenv("CODRAX_TEST_GZIP_TRACE")
	if source == "" {
		t.Skip("set CODRAX_TEST_GZIP_TRACE to the authorized real capture")
	}
	before, err := filegeneration.FromPath(source)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(wire)
	const sourceSHA = "4d66880019663f81462fe5df1615da58b368b6c5225e211dae88600cfc5396e9"
	if len(wire) != 3602086 || hex.EncodeToString(sum[:]) != sourceSHA {
		t.Fatal("opt-in source differs from independent oracle")
	}
	input := filepath.Join(t.TempDir(), "capture.data")
	if err := os.WriteFile(input, wire, 0400); err != nil {
		t.Fatal(err)
	}
	const start, end = 170438.0, 170438.1
	want := map[tracequery.ThreadState]float64{tracequery.StateSSleep: 96.780, tracequery.StateRunnable: 1.088, tracequery.StateRunning: 2.132}
	bus, _, conversions := hmc17NamedPathContext(t)
	var queryPath string
	var totals map[tracequery.ThreadState]float64
	for attempt := 0; attempt < 2; attempt++ {
		got := hmc17NamedPayload(t, hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": input, "view": "thread_timeline", "pid": 3051, "time_start": start, "time_end": end, "limit": 100}))
		if got.View != "thread_timeline" || !got.IndexWindowed || got.Timeline == nil || got.TimeStart != start || got.TimeEnd != end || got.EventCount > defaultTraceIndexEventCapForTest {
			t.Fatalf("attempt=%d real public view did not recover its original scope: %+v", attempt, got)
		}
		queryPath = got.SourcePath
		totals = map[tracequery.ThreadState]float64{}
		for _, row := range got.Timeline.Intervals {
			if row.Thread.PID != 3051 || row.StartTs < start || row.EndTs > end {
				t.Fatalf("target/window drift: %+v", row)
			}
			totals[row.State] += row.DurationMs
		}
		for state, ms := range want {
			if math.Abs(totals[state]-ms) > .00001 {
				t.Fatalf("attempt=%d %s=%0.9fms want %0.9fms", attempt, state, totals[state], ms)
			}
		}
	}
	decoded, err := os.ReadFile(queryPath)
	if err != nil {
		t.Fatal(err)
	}
	decodedSum := sha256.Sum256(decoded)
	if len(decoded) != 54122686 || hex.EncodeToString(decodedSum[:]) != "f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572" || conversions.Load() != 1 {
		t.Fatalf("full preparation/reuse changed: bytes=%d conversions=%d", len(decoded), conversions.Load())
	}
	// A fresh path makes the streaming lane cold independently of the tool's
	// index anchors; the second call is warm. Both must honor physical EOF,
	// not the selected event's 170438.099967 timestamp.
	coldPath := filepath.Join(t.TempDir(), "stream.ftrace")
	if err := os.WriteFile(coldPath, decoded, 0600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		got, err := tracequery.StreamStateCluster(context.Background(), coldPath, tracequery.Query{PID: 3051, TimeStart: start, TimeEnd: end, TimeStartSet: true, TimeEndSet: true}, 100)
		if err != nil || got.WindowStats == nil {
			t.Fatal("streaming result unavailable", err)
		}
		for state, rows := range map[tracequery.ThreadState][]tracequery.ThreadDuration{
			tracequery.StateRunning: got.WindowStats.TopRunning, tracequery.StateRunnable: got.WindowStats.RunnableTop, tracequery.StateSSleep: got.WindowStats.SleepTop,
		} {
			var total float64
			for _, row := range rows {
				if row.Thread.PID == 3051 {
					total += row.DurationMs
				}
			}
			if math.Abs(total-want[state]) > .00001 {
				t.Fatalf("stream attempt=%d %s=%0.9fms want %0.9fms", attempt, state, total, want[state])
			}
		}
	}
	after, err := filegeneration.FromPath(source)
	if err != nil || before.CacheToken() != after.CacheToken() {
		t.Fatal("reference original changed", err)
	}
	receipt, _ := json.Marshal(map[string]any{"source_sha256": sourceSHA, "decoded_sha256": hex.EncodeToString(decodedSum[:]), "decoded_bytes": len(decoded), "source_unchanged": true, "preparations": conversions.Load(), "target_tid": 3051, "window_seconds": []float64{start, end}, "state_ms": totals, "public_heavy_view": "thread_timeline", "index_budget": defaultTraceIndexEventCapForTest, "cold_warm_stream_agree": true})
	t.Log(string(receipt))
}
