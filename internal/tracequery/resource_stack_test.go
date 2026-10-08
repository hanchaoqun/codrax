package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func resourceStackTestRows(t *testing.T, count int) string {
	t.Helper()
	null := tracewire.ResourceScalar{Status: "null"}
	text := tracewire.ResourceText{Status: "unavailable"}
	e := tracewire.ResourceEvent{SourceID: null, PID: 10, TID: 11, IPID: 1, ITID: 2, Thread: "app", Operation: "AllocEvent", CallchainID: tracewire.ResourceScalar{Status: "known", Value: "7"}, Address: null, Size: null, EndNS: null, FrameCount: count, StackStatus: "observed"}
	r := tracewire.ResourceStackRecord{TimestampNS: 1_000_000_000, EventRowID: 0, Event: &e}
	line, ok := tracewire.FormatResourceStack(r)
	if !ok {
		t.Fatal("bad test header")
	}
	body := line + "\n"
	for i := 0; i < count; i++ {
		f := tracewire.ResourceFrame{RowID: int64(i), SourceID: null, Depth: tracewire.ResourceScalar{Status: "known", Value: fmt.Sprint(i)}, IP: null, SymbolID: null, FileID: null, Offset: null, SymbolOffset: null, VAddr: text, Symbol: text, Library: text}
		r.Event, r.Frame = nil, &f
		line, ok = tracewire.FormatResourceStack(r)
		if !ok {
			t.Fatal("bad test frame")
		}
		body += line + "\n"
	}
	return body
}

func resourceStackTestIndex(t *testing.T, body string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resource.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestResourceStackFullCensusDisplayAndSourceBinding(t *testing.T) {
	body := resourceStackTestRows(t, 150)
	idx := resourceStackTestIndex(t, body)
	q := Query{View: ViewResourceStack, PID: 11, TimeStart: 1, TimeEnd: 2}
	p := Run(idx, q).ResourceStack
	if !ValidResourceStack(*p) || p.MatchedEvents != 1 || len(p.Events[0].Frames) != 128 || p.Events[0].OmittedFrames != 22 || p.Events[0].UnknownSymbols != 150 || p.Events[0].MissingDepths != 0 {
		t.Fatalf("bad full census: %+v", p)
	}
	var streamed []Event
	_, err := StreamScan(context.Background(), idx.Path, "", func(ev Event) bool { streamed = append(streamed, ev); return true })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(streamed, idx.Events) {
		t.Fatal("stream/index parser disagreement")
	}
	for _, mutate := range []func(*Index){func(i *Index) { i.Windowed = true }, func(i *Index) { i.RelationScoped = true }, func(i *Index) { i.ResourceStackMalformed++ }, func(i *Index) { i.Events = i.Events[:len(i.Events)-1] }, func(i *Index) { i.Events = append(append([]Event(nil), i.Events...), i.Events[0]) }} {
		copy := *idx
		mutate(&copy)
		if p := Run(&copy, q).ResourceStack; p.Status != "unavailable" || len(p.Events) != 0 {
			t.Fatalf("unproven subset admitted: %+v", p)
		}
	}
	if p := Run(idx, Query{View: ViewResourceStack, TimeStart: 0, TimeStartSet: true, TimeEnd: 1}).ResourceStack; p.MatchedEvents != 0 {
		t.Fatal("right edge counted")
	}
}

func TestResourceStackMalformedNotEmptyAndValidator(t *testing.T) {
	idx := resourceStackTestIndex(t, resourceStackTestRows(t, 3)+"# codrax_resource_stack/v2 record=bad\n")
	p := Run(idx, Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2}).ResourceStack
	if p.Status != "unavailable" || p.Reason != "malformed_resource_stack_carrier" {
		t.Fatalf("invalid source disappeared %+v", p)
	}
	idx = resourceStackTestIndex(t, resourceStackTestRows(t, 3))
	p = Run(idx, Query{View: ViewResourceStack, TimeStart: 1, TimeEnd: 2}).ResourceStack
	for _, mutate := range []func(*ResourceStackResult){func(p *ResourceStackResult) { p.TargetScope = "other" }, func(p *ResourceStackResult) { p.Events[0].MissingDepths = 1 }, func(p *ResourceStackResult) { p.Events[0].SourceFramesComplete = false }, func(p *ResourceStackResult) { p.Events[0].Frames[0].Depth.Value = "9" }, func(p *ResourceStackResult) { p.Events[0].Source.PID = 0 }} {
		copy := *p
		copy.Events = append([]ResourceStackEvent(nil), p.Events...)
		copy.Events[0].Frames = append([]ResourceStackFrame(nil), p.Events[0].Frames...)
		mutate(&copy)
		if ValidResourceStack(copy) {
			t.Fatalf("forged summary admitted %+v", copy)
		}
	}
}

func TestResourceStackDefaultEnvelopeKeepsLastAndSinglePoint(t *testing.T) {
	idx := resourceStackTestIndex(t, resourceStackTestRows(t, 3))
	p := Run(idx, Query{View: ViewResourceStack}).ResourceStack
	if !ValidResourceStack(*p) || p.Status != "available" || p.MatchedEvents != 1 || p.Window.StartTs != 1 || p.Window.EndTs != 1 || !p.Window.EndInclusive {
		t.Fatalf("single point fabricated or lost %+v", p)
	}
	idx = resourceStackTestIndex(t, "idle-0 [000] .... 0.500000: cpu_idle: state=0 cpu_id=0\n"+resourceStackTestRows(t, 3))
	p = Run(idx, Query{View: ViewResourceStack}).ResourceStack
	if !ValidResourceStack(*p) || p.MatchedEvents != 1 || p.Window.StartTs != .5 || p.Window.EndTs != 1 || !p.Window.EndInclusive {
		t.Fatalf("last point lost %+v", p)
	}
}
