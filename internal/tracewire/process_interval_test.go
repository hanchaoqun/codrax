package tracewire

import (
	"reflect"
	"strings"
	"testing"
)

func TestProcessIntervalClosedWire(t *testing.T) {
	name, ref := "加载 | α\nstage", int64(0)
	for _, issue := range []string{"", "null_reference", "invalid_reference"} {
		for _, endpoint := range []string{"begin", "end"} {
			p := ProcessInterval{Endpoint: endpoint, Origin: MarkerNameOrigin{SourceTable: "app_startup", Name: HiSysEventName{Name: &name, Reference: &ref, Status: "resolved"}, Record: &MarkerSourceRecord{RowID: -1, OwnerIssue: issue, StartNS: 9007199254740993, EndNS: 9007199254741999}}}
			wire, err := FormatProcessInterval(p)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := ParseProcessInterval(wire)
			if !ok || !reflect.DeepEqual(got, p) {
				t.Fatalf("roundtrip: %+v %t", got, ok)
			}
			for _, bad := range []string{wire + " ", strings.Replace(wire, "endpoint="+endpoint, "endpoint=B", 1), strings.Replace(wire, "/v1", "/v2", 1), wire + " tid=1 cpu=0"} {
				if _, ok := ParseProcessInterval(bad); ok {
					t.Fatal("admitted altered process carrier")
				}
			}
		}
	}
}
