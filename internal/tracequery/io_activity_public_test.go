package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func ioActivityPublicRun(t *testing.T, idx *Index, q Query) *IOActivityStats {
	t.Helper()
	q.View = "window_stats"
	result := Run(idx, q)
	if result.WindowStats == nil || result.WindowStats.IOActivity == nil {
		t.Fatalf("real parsed endpoint population omitted: %+v", result)
	}
	data, err := json.Marshal(result.WindowStats.IOActivity)
	if err != nil {
		t.Fatal(err)
	}
	var out IOActivityStats
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Population != IOActivityPopulationEndpointEvents || out.IssuerScope != IOInFlightIssuerScopeAll {
		t.Fatalf("wrong population: %+v", out)
	}
	return &out
}

func ioActivityPublicGroup(t *testing.T, stats *IOActivityStats, family, phase string) IOActivityGroup {
	t.Helper()
	for _, group := range stats.Groups {
		if group.EndpointFamily == family && group.Phase == phase {
			return group
		}
	}
	t.Fatalf("missing %s/%s in %+v", family, phase, stats)
	return IOActivityGroup{}
}

func ioActivityPublicValues(t *testing.T, v IOActivityValues, count int, bytes uint64) {
	t.Helper()
	if v.EventCount != count || v.KnownByteEventCount != count || v.KnownBytes == nil || *v.KnownBytes != bytes || v.BytesOverflow || v.UnknownByteEventCount+v.InvalidByteEventCount+v.OverflowByteEventCount != 0 {
		t.Fatalf("endpoint/byte population drifted: %+v want count=%d bytes=%d", v, count, bytes)
	}
	var histogram int
	for _, bin := range v.SizeBuckets {
		histogram += bin.Count
	}
	if histogram != count {
		t.Fatalf("size histogram borrowed a different population: %d != %d", histogram, count)
	}
}

func ioActivityPublicRate(t *testing.T, r *IOActivityRates, events, bytes float64) {
	t.Helper()
	if r == nil || r.KnownBytesPerSecond == nil || math.Abs(r.EventsPerSecond-events) > 1e-7 || math.Abs(*r.KnownBytesPerSecond-bytes) > 1e-5 {
		t.Fatalf("wrong wall-clock rates: %+v want=%g/%g", r, events, bytes)
	}
}

func TestIOActivityPublicIndependentEndpointFixture(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TimeStart: 2, TimeEnd: 2.25, TimeStartSet: true, TimeEndSet: true, PID: 40, Limit: 1}
	stats := ioActivityPublicRun(t, idx, q)
	if stats.GroupCount != 8 || len(stats.Groups) != 8 || stats.Coverage.SupportedEndpointCount != 19 || stats.Coverage.RejectedEndpointCount != 0 || stats.Coverage.UnresolvedSourceCount != 0 || stats.QueryPID != 40 || stats.Window == nil || stats.Window.EndInclusive {
		t.Fatalf("independent exact source/layer/phase populations changed: %+v", stats)
	}
	for _, want := range []struct {
		family, phase, caliber string
		count                  int
		bytes                  uint64
	}{
		{"block_rq", "start", "request_bytes", 7, 37888},
		{"block_rq", "done", "sector_bytes", 6, 28672},
		{"block_bio", "start", "sector_bytes", 1, 4096},
		{"block_bio", "done", "sector_bytes", 1, 4096},
		{"mmc_request", "start", "request_bytes", 1, 2048},
		{"mmc_request", "done", "transferred_bytes", 1, 1024},
	} {
		g := ioActivityPublicGroup(t, stats, want.family, want.phase)
		ioActivityPublicValues(t, g.Values, want.count, want.bytes)
		ioActivityPublicRate(t, g.Rates, float64(want.count)/.25, float64(want.bytes)/.25)
		if g.SourcePath != path || g.ByteCaliber != want.caliber {
			t.Fatalf("source or byte caliber mixed: %+v", g)
		}
	}
	rq := ioActivityPublicGroup(t, stats, "block_rq", "start")
	if rq.ReadWrite == nil || rq.ReadWrite.EventDenominator != 6 || rq.ReadWrite.ReadEventShare == nil || math.Abs(*rq.ReadWrite.ReadEventShare-5.0/6) > 1e-12 || rq.ReadWrite.KnownByteDenominator == nil || *rq.ReadWrite.KnownByteDenominator != 37888 || math.Abs(*rq.ReadWrite.ReadKnownByteShare-21504.0/37888) > 1e-12 {
		t.Fatalf("R/W denominator conflates flushes or byte counts: %+v", rq.ReadWrite)
	}
	if rq.BucketCount != 3 || len(rq.Buckets) != 3 || rq.OmittedBuckets != 0 {
		t.Fatalf("short-tail bucket coverage missing: %+v", rq)
	}
	for i, want := range []struct {
		count int
		bytes uint64
		rate  float64
	}{{4, 21504, 40}, {1, 0, 10}, {2, 16384, 40}} {
		b := rq.Buckets[i]
		ioActivityPublicValues(t, b.Values, want.count, want.bytes)
		ioActivityPublicRate(t, b.Rates, want.rate, float64(want.bytes)/(b.Window.EndTs-b.Window.StartTs))
	}
	if rq.Buckets[1].Window.StartTs != 2.1 || rq.Buckets[2].Window.EndTs != 2.25 {
		t.Fatal("decimal boundary or short tail widened")
	}
	for _, phase := range []string{"start", "done"} {
		g := ioActivityPublicGroup(t, stats, "f2fs_sync_file", phase)
		if g.Values.EventCount != 1 || g.Values.UnknownByteEventCount != 1 || g.Values.KnownBytes != nil || g.Values.KnownSizeMeanBytes != nil || g.Rates == nil || g.Rates.KnownBytesPerSecond != nil || g.ByteCaliber != "unspecified" {
			t.Fatalf("inode metadata or ret=0 became sync byte size: %+v", g)
		}
	}
}

func TestIOActivityPublicIdleAndFullPopulationBeforeCaps(t *testing.T) {
	var body strings.Builder
	for _, ts := range []float64{0.02, 0.08, 0.22, 3.95} {
		fmt.Fprintf(&body, "io-40 (40) [001] .... %.9f: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n", ts)
	}
	idx := buildTraceIndex(t, "activity-caps.systrace", body.String())
	stats := ioActivityPublicRun(t, idx, Query{TimeStart: 0, TimeEnd: 4, TimeStartSet: true, TimeEndSet: true, Limit: 1})
	g := stats.Groups[0]
	ioActivityPublicValues(t, g.Values, 4, 16384)
	ioActivityPublicRate(t, g.Rates, 1, 4096)
	if g.BucketCount != 40 || len(g.Buckets) != IOActivityBucketLimit || g.OmittedBuckets != 8 {
		t.Fatalf("bucket capacity replaced complete summary: %+v", g)
	}
	ioActivityPublicValues(t, g.Buckets[1].Values, 0, 0)
	ioActivityPublicRate(t, g.Buckets[1].Rates, 0, 0)
	if g.Buckets[1].Values.KnownSizeMeanBytes != nil {
		t.Fatal("empty size population got mean 0")
	}
}

func TestIOActivityPublicUnknownZeroInvalidAndOverflow(t *testing.T) {
	const max63 = "9223372036854775807"
	body := "io-40 (40) [001] .... 1.01: mmc_request_start: mmc0 tag=-1 opcode=17 blocks=" + max63 + " block_size=2 blk_addr=1\n" +
		"io-40 (40) [001] .... 1.02: mmc_request_start: mmc0 tag=-1 opcode=17 blocks=" + max63 + " block_size=2 blk_addr=1\n" +
		"io-40 (40) [001] .... 1.03: mmc_request_start: mmc1 tag=-1 opcode=17 blocks=" + max63 + " block_size=3 blk_addr=1\n" +
		"io-40 (40) [001] .... 1.04: mmc_request_start: mmc2 tag=-1 opcode=17 blocks=0 block_size=512 blk_addr=1\n" +
		"io-40 (40) [001] .... 1.05: block_rq_issue: 8,0 R 4294967296 () 1 + 8 [io]\n" +
		"io-40 (40) [001] .... 1.06: f2fs_direct_IO_exit: dev=8:0 ino=0x9 pos=0 len=4096 rw=read ret=-5\n"
	idx := buildTraceIndex(t, "activity-overflow.systrace", body)
	stats := ioActivityPublicRun(t, idx, Query{TimeStart: 1, TimeEnd: 1.1})
	for _, g := range stats.Groups {
		switch g.Dev {
		case "mmc0":
			if g.Values.EventCount != 2 || g.Values.KnownByteEventCount != 2 || !g.Values.BytesOverflow || g.Values.KnownBytes != nil || g.Values.KnownSizeMeanBytes != nil || g.Rates.KnownBytesPerSecond != nil || g.ReadWrite.KnownByteDenominator != nil {
				t.Fatalf("64-bit sum overflow became zero/wrapped bytes: %+v", g)
			}
		case "mmc1":
			if g.Values.OverflowByteEventCount != 1 || g.Values.KnownBytes != nil || g.Values.BytesOverflow || g.Rates.KnownBytesPerSecond != nil {
				t.Fatalf("product overflow lost distinction: %+v", g)
			}
		case "mmc2":
			ioActivityPublicValues(t, g.Values, 1, 0)
			ioActivityPublicRate(t, g.Rates, 10, 0)
		default:
			if g.EndpointFamily == "block_rq" {
				if g.Values.EventCount != 1 || g.Values.OverflowByteEventCount != 1 || g.Values.KnownBytes != nil {
					t.Fatalf("wire uint32 overflow wrong: %+v", g)
				}
			} else if g.Values.InvalidByteEventCount != 1 || g.Values.KnownBytes != nil {
				t.Fatalf("negative transfer return invented zero: %+v", g)
			}
		}
	}
}
