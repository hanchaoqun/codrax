package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestTraceEventInventoryCoordinatesIndexedStreamingAndRawPreservation(t *testing.T) {
	marker, err := FormatCPUUnavailableTraceMark(CPUUnavailableTraceMark{TimestampNS: 1001000000, TID: 11, TGID: 10, SpanPID: 20, Action: "B", Comm: "app", Name: "initialize lib.so", Reason: TraceMarkCPUReasonUnknownStart})
	if err != nil {
		t.Fatal(err)
	}
	wakeup, err := FormatCPUUnavailableWakeup(CPUUnavailableWakeup{TimestampNS: 1002000000, EventName: "sched_wakeup", WakerTID: 11, WakerTGID: 10, WakeeTID: 12, TargetCPU: 0, PrioritySource: WakeePrioritySourceUnknown, WakerComm: "app", WakeeComm: "target", Reason: SchedulerEmitterCPUReasonUnknown})
	if err != nil {
		t.Fatal(err)
	}
	sourceTID := int64(11)
	hisys, err := tracewire.FormatHiSysEventObservation(tracewire.HiSysEvent{TimestampNS: 1003000000, SourceTID: &sourceTID, Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`swapper-0 (0) [000] .... 1.000000: print: B|0|idle marker`, marker, wakeup, hisys,
		`io-31 (30) [003] .... 1.004000: block_rq_issue: 8,0 R 0 () 0 + 8 [io]`,
		`physical-11 (10) [000] .... 1.005000: hi_sysevent: domain=POWER eventname=TEST`,
		`clock-31 [002] .... 1.006000: cpu_frequency: state=1000000 cpu_id=0`,
	}
	path := writeTraceMarkIntegrityTrace(t, "coordinates.trace", lines...)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", TimeStart: 1, TimeStartSet: true, TimeEnd: 1.01, TimeEndSet: true, Limit: 40}
	a := Run(idx, q)
	b, err := StreamEventSearch(context.Background(), path, q)
	if err != nil || len(a.Events) != len(lines) || len(b.Events) != len(lines) {
		t.Fatalf("rows=%d/%d %v", len(a.Events), len(b.Events), err)
	}
	for n, row := range a.Events {
		before, _ := json.Marshal(row)
		p := ProjectTraceEventInventoryCoordinates(row.Event)
		if !reflect.DeepEqual(p, ProjectTraceEventInventoryCoordinates(b.Events[n].Event)) {
			t.Fatalf("stream drift row%d", n)
		}
		after, _ := json.Marshal(row)
		if string(before) != string(after) || row.Raw != lines[n] {
			t.Fatalf("source row%d changed", n)
		}
		switch n {
		case 0:
			if !p.CPUKnown || p.CPU != 0 || !p.EmitterTIDKnown || p.EmitterTID != 0 || p.EmitterTGIDKnown {
				t.Fatalf("idle/unknown mix: %+v", p)
			}
		case 1, 2:
			if p.CPUKnown || p.CPU != -1 || !p.EmitterTIDKnown || p.EmitterTID != 11 || p.CPUUnknownReason == "" {
				t.Fatalf("unknown CPU promoted %+v", p)
			}
		case 3:
			if p.CPUKnown || p.EmitterTIDKnown || p.EmitterTGIDKnown || p.EmitterTID != -1 {
				t.Fatalf("source TID became emitter %+v", p)
			}
		case 4, 5:
			if !p.CPUKnown || !p.EmitterTIDKnown {
				t.Fatalf("physical coordinates erased %+v", p)
			}
		case 6:
			if p.CPU != 2 || !p.CPUKnown || p.EmitterTGIDKnown {
				t.Fatalf("payload CPU substituted header %+v", p)
			}
		}
	}
	if a.Events[1].CPU != 0 {
		t.Fatal("historical marker Event changed instead of display projection")
	}
	expectEventSemanticValue(t, ProjectTraceEventSemantics(a.Events[1].Event), "marker.name", "initialize lib.so")
}

func TestTraceEventInventoryCoordinatesTypedNegativeFamilies(t *testing.T) {
	for _, typ := range []EventType{EventFrameMap, EventFrameCallstack, EventFrameGPU, EventTraceDBRecord, EventCPUMeasureInterval, EventResourceStack, EventEBPFInterval} {
		p := ProjectTraceEventInventoryCoordinates(Event{Type: typ, CPU: 0})
		if p.CPUKnown || p.EmitterTIDKnown || p.EmitterTGIDKnown || p.CPU != -1 || p.EmitterTID != -1 {
			t.Fatalf("%s placeholder claimed %+v", typ, p)
		}
	}
	process := ProjectTraceEventInventoryCoordinates(Event{Type: EventTraceMark, SpanAction: "source_begin", PID: 99, TGID: 99, SpanPID: 99, CPU: 0})
	if process.CPUKnown || process.EmitterTIDKnown {
		t.Fatal("process owner became executing thread")
	}
	falseValue, trueValue := false, true
	perf := Event{Type: EventPerfSample, CPU: 0, PID: 11, TGID: 10, PerfFields: &PerfFields{CPUKnown: &falseValue, ThreadIdentityKnown: &falseValue}}
	p := ProjectTraceEventInventoryCoordinates(perf)
	if p.CPUKnown || p.EmitterTIDKnown || p.EmitterTGIDKnown {
		t.Fatal("perf hard negatives bypassed")
	}
	perf.PerfFields.CPUKnown, perf.PerfFields.ThreadIdentityKnown = &trueValue, &trueValue
	p = ProjectTraceEventInventoryCoordinates(perf)
	if !p.CPUKnown || p.CPU != 0 || !p.EmitterTIDKnown || p.EmitterTID != 11 {
		t.Fatalf("known perf inventory lost %+v", p)
	}
}
