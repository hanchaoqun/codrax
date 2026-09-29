package tracewire

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestMarkerSourceRecordExactAndClosed(t *testing.T) {
	for _, id := range []int64{-1, 0, 9007199254740993} {
		want := MarkerNameOrigin{SourceTable: "app_startup", Name: HiSysEventName{Status: "null_reference"}, Record: &MarkerSourceRecord{RowID: id, OwnerIPID: 0, StartNS: 9007199254740993, EndNS: 9007199254741993}}
		wire, err := EncodeMarkerNameOrigin(want)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := DecodeMarkerNameOrigin(wire)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("lost exact source record: %+v", got)
		}
		body, _ := base64.RawURLEncoding.DecodeString(wire)
		for _, bad := range []string{
			strings.Replace(string(body), `"owner_ipid":"0"`, `"owner_ipid":0`, 1),
			strings.Replace(string(body), `"owner_ipid":"0"`, `"owner_ipid":"0","owner_ipid":"0"`, 1),
			strings.Replace(string(body), `"owner_ipid":"0",`, "", 1),
			strings.Replace(string(body), `"record":{`, `"record":{"launch_instance":"invented",`, 1),
		} {
			if _, ok := DecodeMarkerNameOrigin(base64.RawURLEncoding.EncodeToString([]byte(bad))); ok {
				t.Fatal("accepted malformed record")
			}
		}
	}
	for _, r := range []MarkerSourceRecord{{OwnerIPID: -1, EndNS: 1}, {StartNS: -1, EndNS: 1}, {StartNS: 2, EndNS: 2}, {StartNS: 3, EndNS: 2}} {
		if _, err := EncodeMarkerNameOrigin(MarkerNameOrigin{SourceTable: "app_startup", Name: HiSysEventName{Status: "null_reference"}, Record: &r}); err == nil {
			t.Fatal("accepted invalid interval/reference")
		}
	}
}
