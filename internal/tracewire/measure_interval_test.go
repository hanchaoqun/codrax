package tracewire

import (
	"encoding/base64"
	"reflect"
	"testing"
)

func TestMeasureIntervalScalarAndCarrierRoundTrip(t *testing.T) {
	for _, s := range []MeasureScalar{{StorageClass: "integer", Value: "-9223372036854775808"}, {StorageClass: "integer", Value: "9007199254740993"}, {StorageClass: "real", Value: "+Inf"}, {StorageClass: "text", Value: "text\\n\"中文"}, {StorageClass: "text", Encoding: "base64", Value: base64.RawStdEncoding.EncodeToString([]byte{0x80, 0xff, 0})}, {StorageClass: "blob", Value: "AP8"}, {StorageClass: "null"}, {StorageClass: "absent"}} {
		r := MeasureInterval{RowID: -7, StartNS: MeasureScalar{StorageClass: "integer", Value: "-5"}, DurationNS: MeasureScalar{StorageClass: "integer", Value: "15"}, Value: s, FilterID: MeasureScalar{StorageClass: "null"}, MeasureType: s, FilterStatus: "unknown"}
		line, err := FormatMeasureInterval(r)
		if err != nil {
			t.Fatal(s, err)
		}
		got, ok := ParseMeasureInterval(line)
		if !ok || !reflect.DeepEqual(got, r) {
			t.Fatal("source storage lost", s, got)
		}
		if end, ok := r.EndNS(); !ok || end != 10 {
			t.Fatal("signed interval lost")
		}
		if r.TimestampNS() != 0 {
			t.Fatal("negative sorting coordinate")
		}
	}
	for _, s := range []MeasureScalar{{StorageClass: "integer", Value: "01"}, {StorageClass: "real", Value: "NaN"}, {StorageClass: "text", Value: string([]byte{0xff})}, {StorageClass: "integer", Value: "1", Encoding: "base64"}, {StorageClass: "text", Value: "YQ", Encoding: "base64"}, {StorageClass: "absent", Value: "0"}, {StorageClass: "bogus"}} {
		if s.Valid() {
			t.Fatal("invalid scalar accepted", s)
		}
	}
}

func TestMeasureIntervalRegistryReferenceAndDuration(t *testing.T) {
	i := func(v string) MeasureScalar { return MeasureScalar{StorageClass: "integer", Value: v} }
	r := MeasureInterval{StartNS: i("9223372036854775807"), DurationNS: i("1"), Value: i("3"), FilterID: i("-5"), MeasureType: MeasureScalar{StorageClass: "null"}, FilterStatus: "observed_unique", Filter: &MeasureFilter{ID: i("-5"), Name: MeasureScalar{StorageClass: "null"}, Type: i("5"), SourceArgSetID: i("9007199254740993")}}
	if !r.Valid() {
		t.Fatal("signed registry reference rejected")
	}
	if _, ok := r.EndNS(); ok {
		t.Fatal("overflow invented endpoint")
	}
	r.Filter.ID.Encoding = "base64"
	if r.Valid() {
		t.Fatal("invalid encoded filter integer accepted")
	}
	r.Filter.ID.Encoding = ""
	r.Filter.ID = i("5")
	if r.Valid() {
		t.Fatal("mismatched registry reference accepted")
	}
	r.Filter = nil
	r.FilterStatus = "ambiguous"
	if !r.Valid() {
		t.Fatal("ambiguity lost")
	}
}
