package tracewire

import (
	"encoding/base64"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestHiSysObservationRowIdentityWire(t *testing.T) {
	legacy := hisysWireFixture()
	line, err := FormatHiSysEventObservation(legacy)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ParseHiSysEventObservation(line)
	if !ok || got.SourceRowID != nil || !reflect.DeepEqual(got, legacy) {
		t.Fatal("legacy observation acquired fabricated identity")
	}
	for _, id := range []int64{math.MinInt64, -1, 0, 9007199254740993, math.MaxInt64} {
		row := legacy
		row.SourceRowID = &id
		line, err := FormatHiSysEventObservation(row)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := ParseHiSysEventObservation(line)
		if !ok || !reflect.DeepEqual(got, row) {
			t.Fatalf("source rowid did not roundtrip exactly: %+v, %v", got, ok)
		}
		prefix, payload, _ := strings.Cut(line, " payload=")
		body, _ := base64.RawURLEncoding.DecodeString(payload)
		for _, bad := range []string{`"source_rowid":0,`, `"source_rowid":"00",`, `"source_rowid":null,`, `"source_rowid":"9223372036854775808",`} {
			// A duplicated field is not permitted to replace the valid identity,
			// nor may lossy/numeric aliases be normalized into source authority.
			wire := prefix + " payload=" + base64.RawURLEncoding.EncodeToString([]byte("{"+bad+string(body[1:])))
			if _, ok := ParseHiSysEventObservation(wire); ok {
				t.Fatalf("accepted noncanonical source identity: %s", bad)
			}
		}
	}
}
