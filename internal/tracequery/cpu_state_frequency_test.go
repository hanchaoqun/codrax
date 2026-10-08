package tracequery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func cpuJointFixture(t *testing.T, text string, q Query) (*Index, *CPUStateFrequencyResult) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q.View = ViewCPUStateFrequency
	indexed := Run(idx, q).CPUStateFrequency
	streamed, err := StreamCPUStateFrequency(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(indexed, streamed.CPUStateFrequency) {
		a, _ := json.Marshal(indexed)
		b, _ := json.Marshal(streamed.CPUStateFrequency)
		t.Fatalf("index/stream mismatch\n%s\n%s", a, b)
	}
	if !ValidCPUStateFrequency(*indexed) {
		t.Fatalf("invalid public result: %+v", indexed)
	}
	return idx, indexed
}

func cpuJointLine(ts, kind, value, cpu string) string {
	return "idle-0 (0) [003] .... " + ts + ": " + kind + ": state=" + value + " cpu_id=" + cpu + "\n"
}
func assertJointNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("got %.9f want %.9f", got, want)
	}
}

func TestCPUStateFrequencyCarryInZeroBoundariesAndUnknown(t *testing.T) {
	text := cpuJointLine("0.000000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.000000", "cpu_idle", "0", "0") +
		cpuJointLine("1.100000", "cpu_idle", "4294967295", "0") + cpuJointLine("1.200000", "cpu_frequency", "2000000", "0") +
		cpuJointLine("1.250000", "cpu_frequency", "0", "0") + cpuJointLine("1.300000", "cpu_frequency", "2000000", "0") +
		cpuJointLine("1.400000", "cpu_idle", "1", "0") + cpuJointLine("1.500000", "cpu_frequency", "3000000", "0") +
		cpuJointLine("0.000000", "cpu_frequency", "1500000", "7")
	_, p := cpuJointFixture(t, text, Query{TimeStart: 1, TimeEnd: 1.5})
	if p.CPUCount != 2 || p.CPUs[0].CoreClass != "unknown" {
		t.Fatalf("CPU identity: %+v", p)
	}
	assertJointNear(t, p.CPUTimeMs, 1000)
	assertJointNear(t, p.KnownJointMs, 450)
	assertJointNear(t, p.UnknownJointMs, 550)
	c := p.CPUs[0]
	if c.TotalIntervals != 6 || len(c.Groups) != 5 {
		t.Fatalf("joint partitions: %+v", c)
	}
	if c.Intervals[0].IdleState == nil || *c.Intervals[0].IdleState != 0 || c.Intervals[0].StartTs != 1 || c.Intervals[0].IdleLine != 2 {
		t.Fatal("zero idle index or carry-in lost")
	}
	if c.Intervals[3].FrequencyKnown || c.Intervals[3].FrequencyKHz != nil {
		t.Fatal("zero-frequency gap replaced by a measurement")
	}
	if p.CPUs[1].IdleKnownMs != 0 || p.CPUs[1].JointKnownMs != 0 || p.CPUs[1].FrequencyKnownMs != 500 {
		t.Fatal("missing idle fabricated Running or no-frequency donor applied")
	}
}

func TestCPUStateFrequencySameTimestampLastPhysicalTransitionAndEmptyHead(t *testing.T) {
	text := cpuJointLine("0.050000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.050000", "cpu_idle", "1", "0") +
		cpuJointLine("0.050000", "cpu_frequency", "2000000", "0") + cpuJointLine("0.050000", "cpu_idle", "2", "0") +
		cpuJointLine("0.100000", "cpu_idle", "4294967295", "0")
	_, p := cpuJointFixture(t, text, Query{TimeStart: 0, TimeStartSet: true, TimeEnd: .1})
	if len(p.CPUs[0].Intervals) != 2 {
		t.Fatalf("zero-width duplicate: %+v", p.CPUs[0])
	}
	a, b := p.CPUs[0].Intervals[0], p.CPUs[0].Intervals[1]
	if a.StateKnown || a.FrequencyKnown || a.StartTs != 0 || b.State != "idle" || *b.IdleState != 2 || *b.FrequencyKHz != 2000000 {
		t.Fatal("head/right-boundary/same-time semantics drifted")
	}
}

func TestCPUStateFrequencyRollbackAndMalformedIsolateCPUAndLane(t *testing.T) {
	for _, middle := range []string{cpuJointLine("0.700000", "cpu_idle", "2", "0") + cpuJointLine("0.600000", "cpu_idle", "1", "0"), cpuJointLine("0.700000", "cpu_idle", "broken", "0")} {
		text := cpuJointLine("0.000000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.000000", "cpu_idle", "1", "0") + middle +
			cpuJointLine("0.000000", "cpu_frequency", "2000000", "1") + cpuJointLine("0.000000", "cpu_idle", "4294967295", "1")
		_, p := cpuJointFixture(t, text, Query{TimeStart: .5, TimeEnd: 1})
		if p.CPUs[0].IdleKnownMs != 0 || p.CPUs[0].FrequencyKnownMs != 500 || p.CPUs[1].JointKnownMs != 500 {
			t.Fatalf("wrong poison scope %+v", p.CPUs)
		}
	}
}

func TestCPUFrequencyResidencyNeverBridgesUnknownGap(t *testing.T) {
	var events []Event
	for i, kv := range []struct {
		ts  float64
		khz int64
	}{{0, 1000000}, {1, 0}, {2, 1000000}} {
		events = append(events, Event{Type: EventCPUFrequency, Name: "cpu_frequency", CPUForFieldValid: true, Ts: kv.ts, Frequency: kv.khz, Line: i + 1})
	}
	got, _ := computeCPUFrequencyResidency(events, Query{TimeStart: 0, TimeEnd: 3})
	if len(got) != 2 || got[0].EndTs != 1 || got[1].StartTs != 2 {
		t.Fatalf("frequency gap hidden: %+v", got)
	}
}

func TestCPUStateFrequencySQLDurationWithheld(t *testing.T) {
	text := testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)) + "\n" + cpuJointLine("0.000000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.000000", "cpu_idle", "0", "0")
	idx, p := cpuJointFixture(t, text, Query{TimeStart: 0, TimeStartSet: true, TimeEnd: 1})
	if p.Status != "unavailable" || p.Reason != "sql_measure_interval_semantics_not_preserved" || len(p.CPUs) != 0 {
		t.Fatalf("SQL state/duration silently reinterpreted: %+v", p)
	}
	idx.Windowed, idx.RelationScoped = true, true
	if got := Run(idx, Query{View: ViewCPUStateFrequency, TimeStart: 0, TimeStartSet: true, TimeEnd: 1}).CPUStateFrequency; got.Reason != "sql_measure_interval_semantics_not_preserved" {
		t.Fatal("SQL semantics boundary hidden by incidental index shape")
	}
}

func TestCPUStateFrequencyFiltersWindowCarveAndSourceChanges(t *testing.T) {
	idx, p := cpuJointFixture(t, cpuJointLine("0.000000", "cpu_frequency", "1000000", "0")+cpuJointLine("0.000000", "cpu_idle", "0", "0"), Query{TimeStart: 0, TimeStartSet: true, TimeEnd: 1})
	for _, q := range []Query{{PID: 3}, {Thread: "idle"}, {LineStart: 1}, {Pattern: "idle"}, {Patterns: []string{"idle"}}, {EventTypes: []EventType{EventCPUIdle}}} {
		if _, err := StreamCPUStateFrequency(context.Background(), idx.Path, q); err == nil {
			t.Fatal("ignored filter")
		}
	}
	idx.Windowed = true
	if got := Run(idx, Query{View: ViewCPUStateFrequency, TimeStart: .1, TimeEnd: 1}).CPUStateFrequency; got.Status != "unavailable" {
		t.Fatal("window carve invented predecessor")
	}
	idx.Windowed, idx.RelationScoped = false, true
	if got := Run(idx, Query{View: ViewCPUStateFrequency, TimeStart: .1, TimeEnd: 1}).CPUStateFrequency; got.Status != "unavailable" {
		t.Fatal("relation-scoped index invented complete CPU controls")
	}
	copy := *p
	copy.KnownJointMs++
	if ValidCPUStateFrequency(copy) {
		t.Fatal("invalid totals accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StreamCPUStateFrequency(ctx, idx.Path, Query{TimeEnd: 1}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestTraceCapabilityCatalogPreparedInputsStayAccurate(t *testing.T) {
	c, err := TraceCapabilities(ViewCPUStateFrequency, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range c.InputFormats {
		switch f.ID {
		case "sqlite":
			if f.Handling != "conditional_read_only_snapshot" || !strings.Contains(f.Prerequisite, "main/WAL") {
				t.Fatal(f)
			}
		case "native_trace":
			if !strings.Contains(f.Prerequisite, "complete EOF") || strings.Contains(f.Limitation, "Binary stdin/inline") {
				t.Fatal(f)
			}
		}
	}
}

func TestCPUStateFrequencyRejectedEnvelopeCannotBridgeCarry(t *testing.T) {
	for _, bad := range []string{
		"idle-0 (0) [003] .... bad-time: cpu_idle: state=2 cpu_id=0\n",
		"idle-0 (0) [5000] .... 0.600000: cpu_idle: state=2 cpu_id=0\n",
	} {
		text := cpuJointLine("0.000000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.000000", "cpu_idle", "1", "0") + bad + cpuJointLine("0.000000", "cpu_frequency", "2000000", "1") + cpuJointLine("0.000000", "cpu_idle", "4294967295", "1")
		_, p := cpuJointFixture(t, text, Query{TimeStart: .5, TimeEnd: 1})
		if p.CPUs[0].IdleKnownMs != 0 || p.CPUs[0].FrequencyKnownMs != 500 || p.CPUs[1].JointKnownMs != 500 {
			t.Fatalf("rejected transition was bridged: %+v", p)
		}
	}
}

func TestCPUStateFrequencyDisplayBoundsKeepCompleteTotals(t *testing.T) {
	var text strings.Builder
	text.WriteString(cpuJointLine("0.000000", "cpu_frequency", "1000000", "0"))
	for i := 0; i < 100; i++ {
		text.WriteString(cpuJointLine(fmt.Sprintf("%.6f", float64(i)/100), "cpu_idle", fmt.Sprint(i), "0"))
	}
	_, p := cpuJointFixture(t, text.String(), Query{TimeStart: 0, TimeStartSet: true, TimeEnd: 1})
	c := p.CPUs[0]
	if c.TotalIntervals != 100 || c.OmittedIntervals != 36 || c.GroupCount != 100 || c.OmittedGroups != 36 {
		t.Fatalf("wrong display accounting: %+v", c)
	}
	assertJointNear(t, c.JointKnownMs, 1000)
	c.Intervals = c.Intervals[:16]
	c.OmittedIntervals = c.TotalIntervals - len(c.Intervals)
	c.Groups = c.Groups[:16]
	c.OmittedGroups = c.GroupCount - len(c.Groups)
	p.CPUs[0] = c
	if !ValidCPUStateFrequency(*p) {
		t.Fatal("valid compact handoff rejected")
	}
	p.CPUs[0].Intervals[0].StartTs = .01
	if ValidCPUStateFrequency(*p) {
		t.Fatal("non-prefix time claims accepted")
	}
}

func TestCPUStateFrequencyUnknownCPUDoesNotBorrowEmitter(t *testing.T) {
	text := cpuJointLine("0.000000", "cpu_frequency", "1000000", "7") + cpuJointLine("0.000000", "cpu_idle", "1", "7") +
		"idle-0 (0) [003] .... 0.500000: cpu_idle: state=2\n"
	_, p := cpuJointFixture(t, text, Query{TimeStart: 0, TimeStartSet: true, TimeEnd: 1})
	if p.CPUCount != 1 || p.CPUs[0].CPU != 7 || p.CPUs[0].IdleKnownMs != 0 || p.CPUs[0].FrequencyKnownMs != 1000 {
		t.Fatal("unknown payload CPU borrowed emitter or retained unproven idle")
	}
}

func TestCPUStateFrequencySampleCapAndExplicitTopology(t *testing.T) {
	c := newCPUStateFrequencyCollector()
	c.samples = cpuStateFrequencySampleLimit
	if c.observe(Event{Type: EventCPUIdle, CPUForFieldValid: true, Line: 1}) || !c.overflow {
		t.Fatal("sample cap ignored")
	}
	p := c.finish(&Index{Path: "capture"}, Query{TimeEnd: 1})
	if !ValidCPUStateFrequency(*p) || p.Reason != "cpu_control_sample_limit" {
		t.Fatal("cap fabricated partial sums")
	}
	_, p = cpuJointFixture(t, cpuJointLine("0.000000", "cpu_frequency", "1000000", "7")+cpuJointLine("0.000000", "cpu_idle", "0", "7"), Query{TimeEnd: 1, CoreTopology: "big=7"})
	if p.CPUs[0].CoreClass != "big" || p.CPUs[0].TopologySource != "explicit" {
		t.Fatal("explicit topology lost")
	}
}

func TestCPUStateFrequencyHandoffCrossChecksIntervalGroupValues(t *testing.T) {
	_, p := cpuJointFixture(t, cpuJointLine("0.000000", "cpu_frequency", "1000000", "0")+cpuJointLine("0.000000", "cpu_idle", "1", "0")+cpuJointLine("0.500000", "cpu_idle", "2", "0"), Query{TimeStart: 0, TimeStartSet: true, TimeEnd: 1})
	changed := uint32(3)
	p.CPUs[0].Intervals[0].IdleState = &changed
	if ValidCPUStateFrequency(*p) {
		t.Fatal("internally inconsistent but individually legal interval/group values passed")
	}
	p.CPUs[0].Groups = p.CPUs[0].Groups[:1]
	p.CPUs[0].OmittedGroups = 1
	if ValidCPUStateFrequency(*p) {
		t.Fatal("partial groups concealed inconsistent displayed combination")
	}
}

func TestCPUStateFrequencySQLProvenanceSurvivesCompositeAndWindow(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	path := filepath.Join(dir, "capture.tracebundle.json")
	text := testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)) + "\n" + cpuJointLine("0.000000", "cpu_frequency", "1000000", "0") + cpuJointLine("0.000000", "cpu_idle", "0", "0")
	writeBundleMembershipFixture(t, child, text)
	writeBundleMembershipFixture(t, path, `{"version":"test","systrace":"capture.systrace"}`)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	cropped := deriveWindowedIndex(idx, BuildOptions{TimeStart: .1, TimeStartSet: true, TimeEnd: .5, TimeEndSet: true, AllowWindowedParse: true})
	for _, candidate := range []*Index{idx, cropped} {
		if candidate.TraceDBTextCarrierRows != 1 || candidate.TraceDBTextSchemaRecords != 1 || candidate.TraceDBTextRecords != 1 {
			t.Fatal("SQL storage provenance lost in index transformation")
		}
		p := Run(candidate, Query{View: ViewCPUStateFrequency, TimeStart: .1, TimeEnd: .5}).CPUStateFrequency
		if !ValidCPUStateFrequency(*p) || p.Reason != "sql_measure_interval_semantics_not_preserved" {
			t.Fatalf("SQL bundle silently promoted to measured joint coverage: %+v", p)
		}
	}
	idx.TraceDBTextCarrierRows = 0
	if p := Run(idx, Query{View: ViewCPUStateFrequency, TimeStart: .1, TimeEnd: .5}).CPUStateFrequency; p.Reason != "requires_complete_single_source_cpu_control_scan" {
		t.Fatal("single-child composite invented physical-order authority")
	}
}

func TestCPUStateFrequencyHandoffRejectsNonfiniteDurations(t *testing.T) {
	_, p := cpuJointFixture(t, cpuJointLine("0.000000", "cpu_frequency", "1000000", "0")+cpuJointLine("0.000000", "cpu_idle", "0", "0"), Query{TimeEnd: 1})
	for _, mutate := range []func(*CPUStateFrequencyResult){
		func(x *CPUStateFrequencyResult) { x.WindowWallMs = math.Inf(1) },
		func(x *CPUStateFrequencyResult) { x.CPUTimeMs = math.Inf(1) },
		func(x *CPUStateFrequencyResult) { x.WindowWallMs = math.NaN() },
	} {
		copy := *p
		mutate(&copy)
		if ValidCPUStateFrequency(copy) {
			t.Fatal("nonfinite typed quantity accepted")
		}
	}
}
