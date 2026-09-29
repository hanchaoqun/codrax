package tracewire

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestMarkerNameOriginCanonicalClosedWire(t *testing.T) {
	zero, name := int64(0), "业务 | stage:α"
	for _, value := range []HiSysEventName{
		{Name: &name, Reference: &zero, Status: "resolved"}, {Reference: &zero, Status: "unresolved_reference"},
		{Status: "null_reference"}, {Status: "invalid_reference_storage_class"},
	} {
		want := MarkerNameOrigin{SourceTable: "app_startup", Name: value}
		wire, err := EncodeMarkerNameOrigin(want)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := DecodeMarkerNameOrigin(wire)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("roundtrip: %+v %v", got, ok)
		}
		b, _ := base64.RawURLEncoding.DecodeString(wire)
		for _, bad := range []string{wire + "=", wire + " ", strings.Repeat("A", MaxMarkerNameOriginBytes+1),
			base64.RawURLEncoding.EncodeToString(append(b, byte(' '))),
			base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(b), `"source_table":`, `"extra":true,"source_table":`, 1))),
			base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(string(b), `"source_table":`, `"source_table":"app_startup","source_table":`, 1))),
		} {
			if _, ok := DecodeMarkerNameOrigin(bad); ok {
				t.Fatal("accepted open/ambiguous/noncanonical metadata")
			}
		}
	}
	for _, bad := range []MarkerNameOrigin{
		{SourceTable: "other", Name: HiSysEventName{Status: "null_reference"}},
		{SourceTable: "app_startup", Name: HiSysEventName{Status: "resolved", Reference: &zero}},
		{SourceTable: "app_startup", Name: HiSysEventName{Status: "unresolved_reference", Name: &name, Reference: &zero}},
	} {
		if _, err := EncodeMarkerNameOrigin(bad); err == nil {
			t.Fatal("accepted contradictory source name")
		}
	}
}
