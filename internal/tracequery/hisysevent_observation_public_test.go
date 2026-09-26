package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHiSysObservationNativePublicWindowAndStreaming(t *testing.T) {
	var text string
	for _, ns := range []int64{0, 1000001, 2000000} {
		zero := int64(0)
		empty := ""
		contents := "one\ntwo\x00"
		row := tracewire.HiSysEvent{TimestampNS: ns, SourceTID: &zero,
			Domain:   tracewire.HiSysEventName{Status: "unresolved_reference", Reference: &zero},
			Event:    tracewire.HiSysEventName{Status: "resolved", Reference: &zero, Name: &empty},
			Contents: tracewire.HiSysEventContents{StorageClass: "text", Text: &contents}}
		line, err := tracewire.FormatHiSysEventObservation(row)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := ParseLineTimestampNS(line); !ok || got != uint64(ns) {
			t.Fatalf("exact timestamp=%d %t", got, ok)
		}
		if got, ok := parseLineTimestamp(line); !ok || got != float64(ns)/1e9 {
			t.Fatal("timestamp gate disagrees")
		}
		text += line + "\n"
	}
	path := filepath.Join(t.TempDir(), "rows.systrace")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", EventTypes: []EventType{EventHiSystemEvent}, TimeStart: .0010000005, TimeEnd: .0010000015}
	for _, windowed := range []bool{false, true} {
		var idx *Index
		var err error
		if windowed {
			idx, err = BuildIndexWithOptions(context.Background(), path, BuildOptions{AllowWindowedParse: true, TimeStart: q.TimeStart, TimeStartSet: true, TimeEnd: q.TimeEnd, TimeEndSet: true})
		} else {
			idx, err = BuildIndex(context.Background(), path)
		}
		if err != nil {
			t.Fatal(err)
		}
		r := Run(idx, q)
		if len(r.Events) != 1 {
			t.Fatalf("windowed=%t population=%+v", windowed, r.Events)
		}
		e := r.Events[0]
		if e.PluginFields == nil || e.HiSysEvent == nil || e.HiSysEvent.TimestampNS != 1000001 || e.PID != 0 || e.CPU != -1 {
			t.Fatalf("source record acquired false authority: %+v", e)
		}
		if *e.HiSysEvent.Contents.Text != "one\ntwo\x00" || e.HiSysEvent.Event.Name == nil || *e.HiSysEvent.Event.Name != "" {
			t.Fatal("payload was flattened or empty identity lost")
		}
	}
	r, err := StreamEventSearch(context.Background(), path, q)
	if err != nil || len(r.Events) != 1 {
		t.Fatalf("streaming population=%+v err=%v", r.Events, err)
	}
	r, err = StreamEventSearch(context.Background(), path, Query{View: "event_search", EventTypes: []EventType{EventHiSystemEvent}, TimeStartSet: true, TimeEnd: .0000000001})
	if err != nil || len(r.Events) != 1 || r.Events[0].Ts != 0 {
		t.Fatalf("zero event lost: %+v %v", r.Events, err)
	}
}
