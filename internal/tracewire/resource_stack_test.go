package tracewire

import (
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

func resourceWireEvent() ResourceEvent {
	s := ResourceScalar{Status: "null"}
	return ResourceEvent{SourceID: s, PID: 10, TID: 11, IPID: 1, ITID: 2, Thread: "app", Operation: "AllocEvent", CallchainID: ResourceScalar{Status: "known", Value: "0"}, Address: ResourceScalar{Status: "known", Value: "-9223372036854775808"}, Size: s, EndNS: s, StackStatus: "no_frames"}
}

func TestResourceStackWireExactSignedRowsAndMalformed(t *testing.T) {
	for _, id := range []int64{0, -1, math.MinInt64, math.MaxInt64} {
		e := resourceWireEvent()
		r := ResourceStackRecord{TimestampNS: 0, EventRowID: id, Event: &e}
		line, ok := FormatResourceStack(r)
		if !ok {
			t.Fatal("rejected legal physical rowid", id)
		}
		got, ok := ParseResourceStack(line)
		if !ok || got.EventRowID != id || got.Event.Address.Value != e.Address.Value {
			t.Fatalf("lost exact scalars: %+v", got)
		}
		b, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(line, ResourceStackPrefix+" record="))
		for _, bad := range []string{strings.Replace(line, "/v1", "/v2", 1), line + " ", ResourceStackPrefix + " record=" + base64.RawURLEncoding.EncodeToString(append(b[:len(b)-1], []byte(`,"event_row_id":"9"}`)...))} {
			if _, ok := ParseResourceStack(bad); ok {
				t.Fatal("accepted malformed/duplicate", bad)
			}
		}
	}
	e := resourceWireEvent()
	e.CallchainID.Value = "01"
	if e.Valid() {
		t.Fatal("noncanonical integer accepted")
	}
}

func TestResourceStackWireUnrelatedInputDoesNotAllocate(t *testing.T) {
	if got := testing.AllocsPerRun(100, func() { ParseResourceStack("worker-1 [000] .... 1.000000: sched_switch: prev_pid=1 next_pid=2") }); got != 0 {
		t.Fatalf("per ordinary row allocations=%g", got)
	}
}

func TestResourceStackWireLongestEscapedTextRoundTrips(t *testing.T) {
	null := ResourceScalar{Status: "null"}
	long := ResourceText{Status: "known", Value: strings.Repeat("\x01", 4096)}
	f := ResourceFrame{SourceID: null, Depth: null, IP: null, SymbolID: null, FileID: null, Offset: null, SymbolOffset: null, VAddr: long, Symbol: long, Library: long}
	r := ResourceStackRecord{Frame: &f}
	line, ok := FormatResourceStack(r)
	if !ok || len(line) <= 32768 || len(line) > ResourceStackMaxLineBytes {
		t.Fatalf("bad protocol capacity %d/%v", len(line), ok)
	}
	if got, ok := ParseResourceStack(line); !ok || got.Frame.Symbol.Value != long.Value {
		t.Fatal("valid source text lost")
	}
	f.Symbol.Value += "x"
	if _, ok := FormatResourceStack(r); ok {
		t.Fatal("oversized field accepted")
	}
	e := resourceWireEvent()
	e.Thread = strings.Repeat("<", 4096)
	r.Frame = nil
	r.Event = &e
	line, ok = FormatResourceStack(r)
	if !ok {
		t.Fatal("long thread rejected")
	}
	if _, ok := ParseResourceStack(line); !ok {
		t.Fatal("long thread reader mismatch")
	}
}
