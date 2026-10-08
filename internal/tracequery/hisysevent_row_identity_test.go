package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHiSysObservationSourceRowsSurviveIndexStreamAndSemantics(t *testing.T) {
	var lines []string
	for _, id := range []int64{-1, 0, 9007199254740993} {
		row := tracewire.HiSysEvent{TimestampNS: 1000001, SourceRowID: &id,
			Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}}
		line, err := tracewire.FormatHiSysEventObservation(row)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	path := filepath.Join(t.TempDir(), "source.systrace")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", EventTypes: []EventType{EventHiSystemEvent}, TimeStart: .001, TimeEnd: .001001}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for name, events := range map[string][]EventView{"index": indexed.Events, "stream": streamed.Events} {
		if len(events) != 3 {
			t.Fatalf("%s collapsed same-timestamp source rows: %+v", name, events)
		}
		for i, e := range events {
			want := []int64{-1, 0, 9007199254740993}[i]
			if e.HiSysEvent == nil || e.HiSysEvent.SourceRowID == nil || *e.HiSysEvent.SourceRowID != want || e.PID != 0 || e.TGID != 0 || e.CPU != -1 {
				t.Fatalf("%s changed source identity/role: %+v", name, e)
			}
			fields := map[string]string{}
			semantics := ProjectTraceEventSemantics(e.Event)
			if semantics == nil {
				t.Fatalf("%s dropped typed semantics", name)
			}
			for _, field := range semantics.Fields {
				if field.Status == "known" && field.Value != nil {
					fields[field.Key] = *field.Value
				}
			}
			if fields["source.row_id"] != strconv.FormatInt(want, 10) || fields["source.table"] != "hisys_all_event" {
				t.Fatalf("%s source locator missing from handoff: %+v", name, fields)
			}
		}
	}
}

func TestHiSysObservationSourceRowMemoryAndLegacySemantics(t *testing.T) {
	row := tracewire.HiSysEvent{Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}}
	e := Event{Type: EventHiSystemEvent, PluginFields: &PluginFields{HiSysEvent: &row}}
	before := eventSideTableBytes(&e)
	legacy := ProjectTraceEventSemantics(e)
	field := eventSemanticField(t, legacy, "source.row_id")
	if field.Status != "unavailable" || field.Value != nil {
		t.Fatalf("legacy observation acquired physical row identity: %+v", field)
	}
	id := int64(0)
	row.SourceRowID = &id
	if got := eventSideTableBytes(&e) - before; got != 8 {
		t.Fatalf("source identity allocation not accounted: got %d want 8", got)
	}
	expectEventSemanticValue(t, ProjectTraceEventSemantics(e), "source.row_id", "0")
}
