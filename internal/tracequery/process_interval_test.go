package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestProcessIntervalIndexedStreamingWindowAndNoExecution(t *testing.T) {
	base := tracewire.ProcessInterval{Origin: tracewire.MarkerNameOrigin{SourceTable: "app_startup", Name: tracewire.HiSysEventName{Status: "null_reference"}, Record: &tracewire.MarkerSourceRecord{RowID: 0, OwnerIssue: "null_reference", StartNS: 2000000001, EndNS: 2250000003}}}
	var body string
	for _, endpoint := range []string{"begin", "end"} {
		base.Endpoint = endpoint
		wire, err := tracewire.FormatProcessInterval(base)
		if err != nil {
			t.Fatal(err)
		}
		if ns, ok := ParseLineTimestampNS(wire); !ok || ns != uint64(base.TimestampNS()) {
			t.Fatal("lost exact source timestamp")
		}
		scan := &lineScan{line: wire, lineNo: 1}
		if ts, ok := scan.timestamp(); !ok || ts != float64(base.TimestampNS())/1e9 {
			t.Fatal("window scanner missed process interval")
		}
		body += wire + "\n"
	}
	path := filepath.Join(t.TempDir(), "process.systrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", EventTypes: []EventType{EventTraceMark}, TraceMarkActions: []TraceMarkAction{TraceMarkActionSourceEnd}, TimeStart: 2.2, TimeEnd: 2.3, TimeStartSet: true, TimeEndSet: true, Limit: 10}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []Result{indexed, streamed} {
		if len(got.Events) != 1 {
			t.Fatalf("end-only window lost record: %+v", got)
		}
		e := got.Events[0].Event
		if e.CPU != -1 || e.PID != 0 || e.TGID != 0 || e.SpanPID != 0 || e.SpanAction != "source_end" || e.MarkerNameOrigin.Record.StartNS != 2000000001 || e.MarkerNameOrigin.Record.OwnerIssue != "null_reference" {
			t.Fatalf("wrong subject/interval: %+v", e)
		}
		encoded, _ := json.Marshal(e)
		var restored Event
		if err := json.Unmarshal(encoded, &restored); err != nil || restored.MarkerNameOrigin.Record.OwnerIssue != "null_reference" {
			t.Fatal("cache-facing JSON lost owner status")
		}
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 2, TimeEnd: 2.3, TimeStartSet: true, TimeEndSet: true})
	if len(stats.TraceSpans) != 0 {
		t.Fatal("process interval minted thread stack/duration")
	}
	ownerPID := int64(27599)
	for _, event := range idx.Events {
		event.MarkerNameOrigin.Record.OwnerPID = &ownerPID
		if eventMentionsPID(event, int(ownerPID)) {
			t.Fatal("source process owner leaked into emitter thread selection")
		}
	}
}
