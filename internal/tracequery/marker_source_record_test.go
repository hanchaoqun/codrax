package tracequery

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"testing"
)

func TestMarkerSourceRecordEndpointBinding(t *testing.T) {
	base := ExactTraceMark{CPU: 1, TID: 100, TGID: 100, SpanPID: 100, Comm: "app", NameOrigin: &tracewire.MarkerNameOrigin{
		SourceTable: "app_startup", Name: tracewire.HiSysEventName{Status: "null_reference"},
		Record: &tracewire.MarkerSourceRecord{RowID: 0, OwnerIPID: 0, StartNS: 9007199254740993, EndNS: 9007199255740993},
	}}
	for _, action := range []string{"B", "E"} {
		mark := base
		mark.Action, mark.TimestampNS = action, uint64(base.NameOrigin.Record.StartNS)
		if action == "B" {
			mark.Name = "AppStartup:startup"
		} else {
			mark.TimestampNS = uint64(base.NameOrigin.Record.EndNS)
		}
		wire, err := FormatExactTraceMark(mark)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := parseExactTraceMark(wire)
		if !ok || *got.NameOrigin.Record != *base.NameOrigin.Record {
			t.Fatal("endpoint lost its source interval")
		}
		mark.TimestampNS++
		if _, err := FormatExactTraceMark(mark); err == nil {
			t.Fatal("accepted endpoint from another interval")
		}
	}
}
