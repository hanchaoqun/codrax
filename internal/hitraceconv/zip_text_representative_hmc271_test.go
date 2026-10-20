package hitraceconv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The archive is transport repackaging of the reference's exact recorded
// ftrace bytes, not a new capture. No synthetic rows or timestamps are added.
func TestHMC271ZIPTextRepresentativeCompleteNativeWindow(t *testing.T) {
	source := os.Getenv("CODRAX_TEST_ZIP_TEXT_TRACE")
	if source == "" {
		t.Skip("set CODRAX_TEST_ZIP_TEXT_TRACE to the authorized real text ZIP")
	}
	before, err := filegeneration.FromPath(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	probe := ProbeInputRoutingCapability(t.Context(), source, dir)
	if probe.NativeReader != "trace_query" || probe.ArchiveCandidates == nil || probe.ArchiveCandidates.CandidateCount != 1 ||
		len(probe.ArchiveCandidates.Members) != 1 || probe.ArchiveCandidates.Members[0] != "session/capture.data" {
		t.Fatalf("representative named ZIP navigation: %+v", probe)
	}
	result, err := PrepareFile(t.Context(), Options{InputPath: source, OutputPath: filepath.Join(dir, "complete.systrace"), RuntimeAnchor: filepath.Join(dir, "runtime")})
	if err != nil || result.ArchiveTextTransport == nil {
		t.Fatalf("representative complete preparation: %+v %v", result, err)
	}
	const wantSHA = "f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572"
	receipt := result.ArchiveTextTransport
	if receipt.Archive.Member != "session/capture.data" || receipt.Archive.Selection != "unique_candidate" ||
		receipt.Archive.MemberSHA256 != wantSHA || receipt.DecodedSHA256 != wantSHA || receipt.DecodedBytes != 54122686 ||
		receipt.SourceGeneration != before.CacheToken() || result.EventsWritten != 0 || result.BundlePath != "" {
		t.Fatalf("representative receipt differs: %+v", receipt)
	}
	f, err := os.Open(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.New()
	count, copyErr := io.Copy(digest, f)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || count != receipt.DecodedBytes || hex.EncodeToString(digest.Sum(nil)) != wantSHA {
		t.Fatal("published full text differs", copyErr, closeErr)
	}
	parsed, wakes := 0, map[int]int{}
	tgids := map[int]int{}
	metadata, err := tracequery.StreamScan(t.Context(), result.OutputPath, "", func(event tracequery.Event) bool {
		parsed++
		if event.Name == "sched_wakeup" && event.WakeePID == 3051 && event.Ts >= 170438 && event.Ts < 170438.1 {
			wakes[event.PID]++
			tgids[event.PID] = event.TGID
		}
		return true
	})
	if err != nil || parsed != 429756 || metadata.ScannedLineCount != 429766 || len(metadata.Events) != 0 ||
		len(wakes) != 3 || wakes[51299] != 2 || wakes[1021] != 3 || wakes[20975] != 3 ||
		tgids[51299] != 3762 || tgids[1021] != 908 || tgids[20975] != 908 {
		t.Fatalf("native complete census/wake identity: parsed=%d metadata=%+v wakes=%v tgids=%v err=%v", parsed, metadata, wakes, tgids, err)
	}
	window := tracequery.BuildOptions{TimeStart: 170438, TimeEnd: 170438.1, TimeStartSet: true, TimeEndSet: true, TimePaddingBefore: .1, TimePaddingAfter: .1, AllowWindowedParse: true}
	for _, phase := range []string{"cold", "warm"} {
		idx, err := tracequery.BuildIndexWithOptions(context.Background(), result.OutputPath, window)
		if err != nil {
			t.Fatal(phase, err)
		}
		result := tracequery.Run(idx, tracequery.Query{View: "thread_timeline", PID: 3051, TimeStart: 170438, TimeEnd: 170438.1, TimeStartSet: true, TimeEndSet: true, Limit: 100})
		if result.Timeline == nil {
			t.Fatal("missing timeline")
		}
		totals := map[tracequery.ThreadState]float64{}
		for _, interval := range result.Timeline.Intervals {
			if interval.StartTs < 170438 || interval.EndTs > 170438.1 || interval.Thread.PID != 3051 {
				t.Fatal("window or target changed")
			}
			totals[interval.State] += interval.DurationMs
		}
		want := map[tracequery.ThreadState]float64{tracequery.StateSSleep: 96.780, tracequery.StateRunnable: 1.088, tracequery.StateRunning: 2.132}
		if len(totals) != len(want) {
			t.Fatal("state universe differs", totals)
		}
		for state, expected := range want {
			if math.Abs(totals[state]-expected) > .00001 {
				t.Fatalf("%s %s=%f expected=%f", phase, state, totals[state], expected)
			}
		}
	}
	after, err := filegeneration.FromPath(source)
	if err != nil || !before.SameVersion(after) {
		t.Fatal("read-only original generation changed", err)
	}
	t.Logf("complete_bytes=%d sha256=%s physical_lines=%d events=%d wakes=%v tgids=%v cold_warm=true original_unchanged=true",
		count, wantSHA, metadata.ScannedLineCount, parsed, wakes, tgids)
}
