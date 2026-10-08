package tracequery

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func nativeCPUScalar(v int64) *int64 { return &v }

func nativeCPURecord(cpu int, kind string, start, duration, value int64) tracewire.CPUMeasureInterval {
	encoding := "khz"
	if kind == "idle" {
		encoding = "native_sql_idle"
	}
	return tracewire.CPUMeasureInterval{RowID: start, FilterID: int64(cpu + 1), CPU: cpu, Kind: kind, Encoding: encoding, StartNS: nativeCPUScalar(start), DurationNS: nativeCPUScalar(duration), Value: nativeCPUScalar(value)}
}

func nativeCPUText(t *testing.T, rows ...tracewire.CPUMeasureInterval) string {
	t.Helper()
	var b strings.Builder
	for _, row := range rows {
		line, err := tracewire.FormatCPUMeasureInterval(row)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func TestCPUStateFrequencyNativeExplicitIntervals(t *testing.T) {
	// Deliberately nonchronological physical order; SQL intervals are records,
	// not ftrace next-update transitions. Negative carry-in is not rebased.
	text := nativeCPUText(t,
		nativeCPURecord(0, "frequency", 20000000, 20000000, 2000000),
		nativeCPURecord(0, "idle", 0, 40000000, 0),
		nativeCPURecord(0, "frequency", -5000000, 15000000, 1000000),
		nativeCPURecord(1, "frequency", 0, 40000000, 1500000),
		nativeCPURecord(1, "idle", 5000000, 15000000, math.MaxUint32),
		nativeCPURecord(2, "idle", 40000000, 10000000, 0),
	)
	idx, got := cpuJointFixture(t, text, Query{TimeStart: 0, TimeStartSet: true, TimeEnd: .04})
	if got.Status != "available" || got.CPUCount != 2 || got.CPUs[0].CoreClass != "unknown" {
		t.Fatalf("source CPU/window authority: %+v", got)
	}
	assertJointNear(t, got.CPUTimeMs, 80)
	assertJointNear(t, got.KnownJointMs, 45)
	assertJointNear(t, got.UnknownJointMs, 35)
	assertJointNear(t, got.CPUs[0].FrequencyKnownMs, 30)
	assertJointNear(t, got.CPUs[1].IdleKnownMs, 15)
	for _, cpu := range got.CPUs {
		for _, iv := range cpu.Intervals {
			if iv.StateEncoding != "native_sql_idle" || iv.StateKnown && iv.State != "native_idle" {
				t.Fatal("native code was interpreted as ftrace idle/Running")
			}
		}
	}
	if got.CPUs[0].Intervals[0].StartTs != 0 || *got.CPUs[0].Intervals[0].FrequencyKHz != 1000000 {
		t.Fatal("negative carry-in lost or next frequency borrowed")
	}
	for _, ev := range idx.Events {
		if ev.Type == EventCPUIdle || ev.Type == EventCPUFrequency {
			t.Fatal("native intervals became ftrace controls")
		}
	}
}

func TestCPUStateFrequencyNativeUnknownDurationAndOverlap(t *testing.T) {
	for name, uncertain := range map[string]tracewire.CPUMeasureInterval{
		"null_duration": func() tracewire.CPUMeasureInterval {
			r := nativeCPURecord(0, "frequency", 10000000, 1, 2000000)
			r.DurationNS, r.Issue = nil, "unknown_duration"
			return r
		}(),
		"invalid_value_and_duration": func() tracewire.CPUMeasureInterval {
			r := nativeCPURecord(0, "frequency", 10000000, 1, 2000000)
			r.DurationNS, r.Value, r.Issue = nil, nil, "invalid_value"
			return r
		}(),
		"invalid_duration": func() tracewire.CPUMeasureInterval {
			r := nativeCPURecord(0, "frequency", 10000000, 1, 2000000)
			r.DurationNS, r.Issue = nil, "invalid_duration"
			return r
		}(),
		"same_value_overlap":      nativeCPURecord(0, "frequency", 10000000, 10000000, 1000000),
		"different_value_overlap": nativeCPURecord(0, "frequency", 10000000, 10000000, 2000000),
		"zero_duration":           nativeCPURecord(0, "frequency", 10000000, 0, 2000000),
		"right_boundary_unknown": func() tracewire.CPUMeasureInterval {
			r := nativeCPURecord(0, "frequency", 40000000, 1, 2000000)
			r.DurationNS, r.Issue = nil, "unknown_duration"
			return r
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			_, got := cpuJointFixture(t, nativeCPUText(t, nativeCPURecord(0, "idle", 0, 40000000, 0), nativeCPURecord(0, "frequency", 0, 40000000, 1000000), uncertain), Query{TimeStart: 0, TimeStartSet: true, TimeEnd: .04})
			want := 10.0
			if strings.Contains(name, "overlap") {
				want = 30
			}
			if name == "zero_duration" || name == "right_boundary_unknown" {
				want = 40
			}
			assertJointNear(t, got.KnownJointMs, want)
			assertJointNear(t, got.UnknownJointMs, 40-want)
		})
	}
}

func TestCPUStateFrequencyNativeInvalidityStaysInItsLane(t *testing.T) {
	bad := nativeCPURecord(0, "idle", 0, 10000000, 1)
	bad.StartNS, bad.Issue = nil, "invalid_timestamp"
	_, got := cpuJointFixture(t, nativeCPUText(t,
		nativeCPURecord(0, "idle", 0, 40000000, 0), bad, nativeCPURecord(0, "frequency", 0, 40000000, 1000000),
		nativeCPURecord(1, "idle", 0, 40000000, 2), nativeCPURecord(1, "frequency", 0, 40000000, 2000000)), Query{TimeStart: 0, TimeStartSet: true, TimeEnd: .04})
	if got.CPUs[0].IdleKnownMs != 0 || got.CPUs[0].IdleUnavailable == "" {
		t.Fatal("unknown timestamp borrowed known lane coverage")
	}
	assertJointNear(t, got.CPUs[0].FrequencyKnownMs, 40)
	assertJointNear(t, got.CPUs[1].JointKnownMs, 40)
}

func TestCPUStateFrequencyNativeMalformedCarrierBlocksBothPaths(t *testing.T) {
	valid := nativeCPUText(t, nativeCPURecord(0, "idle", 0, 40000000, 0), nativeCPURecord(0, "frequency", 0, 40000000, 1000000))
	for _, bad := range []string{"# codrax_cpu_measure_interval/v1 record=broken\n", "# codrax_cpu_measure_interval/v2 record=broken\n"} {
		idx, got := cpuJointFixture(t, valid+bad, Query{TimeStart: 0, TimeStartSet: true, TimeEnd: .04})
		if idx.CPUIntervalMalformed != 1 || got.Reason != "malformed_native_cpu_interval_carrier" {
			t.Fatalf("invalid native input silently disappeared: %+v", got)
		}
	}
}

func TestCPUStateFrequencyNativeCanonicalParseTimestamp(t *testing.T) {
	text := strings.TrimSpace(nativeCPUText(t, nativeCPURecord(0, "frequency", 123456789, 1, 1000000)))
	sec, ok := ParseTimestamp(text)
	ns, exact := ParseLineTimestampNS(text)
	if !ok || !exact || sec != .123456789 || ns != 123456789 {
		t.Fatal("native interval timestamp entry points disagree")
	}
	ev, parsed := ParseLine(1, text, nil)
	if !parsed || !ev.CPUForFieldValid || ev.CPU != -1 || eventCPUForStats(ev) != 0 {
		t.Fatal("native CPU owner replaced by an emitter")
	}
	if got := CPUGlobalEventSearchTypes([]EventType{EventCPUMeasureInterval}); len(got) != 1 || !perfBundleRowIsSchedulerOrCPU(EventCPUMeasureInterval) {
		t.Fatal("new CPU-owned carrier missed shared scope/admission registration")
	}
}

func TestCPUStateFrequencyNativeBackfilledWindowIsNotCaptureProof(t *testing.T) {
	_, got := cpuJointFixture(t, nativeCPUText(t, nativeCPURecord(0, "idle", 0, 40000000, 0), nativeCPURecord(0, "frequency", 20000000, 20000000, 1000000)), Query{})
	if got.Window.EndTs != .02 || !strings.Contains(strings.Join(got.Caveats, " "), "do not independently establish whole-capture") {
		t.Fatalf("interval endpoint minted full capture duration: %+v", got)
	}
}

func TestCPUStateFrequencyNativeIdentityBundleAndCompleteSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.systrace")
	var records []tracewire.CPUMeasureInterval
	for i := int64(0); i < 100; i++ {
		records = append(records, nativeCPURecord(0, "frequency", i*1000000, 1000000, 1000000+i))
	}
	records = append(records, nativeCPURecord(0, "idle", 0, 100000000, 0))
	if err := os.WriteFile(path, []byte(nativeCPUText(t, records...)), 0600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, bundle, `{"version":"test","systrace":"capture.systrace"}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: ViewCPUStateFrequency, TimeStart: 0, TimeStartSet: true, TimeEnd: .1}
	got := Run(idx, q).CPUStateFrequency
	if got.Status != "available" || got.CPUs[0].OmittedIntervals == 0 {
		t.Fatalf("verified identity mapping unavailable: %+v", got)
	}
	c := newCPUStateFrequencyCollector()
	for _, ev := range idx.Events {
		c.observe(ev)
	}
	count := 0
	buildNativeCPUStateFrequencyCPU(0, c.native, q, func(iv CPUStateFrequencyInterval) { count++ })
	if count != 100 {
		t.Fatalf("authority callback truncated to display: %d", count)
	}
	idx.TraceArtifacts[0].ClockAlignment = TraceClockAlignmentAffine
	if Run(idx, q).CPUStateFrequency.Status != "unavailable" {
		t.Fatal("affine mapping applied only to event start but not native endpoints")
	}
}
