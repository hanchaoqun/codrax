package tracewire

import (
	"encoding/base64"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func processWireInteger(v int64) ProcessMeasureScalar {
	return ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: strconv.FormatInt(v, 10)}
}
func processWireRecord() ProcessMeasureInterval {
	pid := 101
	return ProcessMeasureInterval{RowID: math.MinInt64, FilterID: processWireInteger(1), StartNS: processWireInteger(-5), DurationNS: processWireInteger(10), Value: processWireInteger(math.MaxInt64), IPID: processWireInteger(1), Name: `name: {"value":0}`, NameKnown: true, PID: &pid, OwnerStatus: "known"}
}

func TestProcessMeasureWireExactRoundTrip(t *testing.T) {
	base := processWireRecord()
	for _, value := range []ProcessMeasureScalar{base.Value, processWireInteger(0), {Status: "null", StorageClass: "null"}, {Status: "unavailable", StorageClass: "absent"}, {Status: "invalid_storage", StorageClass: "text", Value: ""}, {Status: "invalid_storage", StorageClass: "blob", Value: ""}, {Status: "invalid_storage", StorageClass: "real", Value: "+Inf"}} {
		r := base
		r.Value = value
		line, err := FormatProcessMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := ParseProcessMeasureInterval(line)
		if !ok || !reflect.DeepEqual(got, r) || got.TimestampNS() != 0 {
			t.Fatal("lossy wire", got, r)
		}
	}
	base.StartNS = processWireInteger(math.MaxInt64)
	if _, ok := base.EndNS(); ok {
		t.Fatal("overflow interval has endpoint")
	}
	if _, err := FormatProcessMeasureInterval(base); err != nil {
		t.Fatal("invalid source interval must remain available as raw record")
	}
}

func TestProcessMeasureWireCanonicalAndNoAllocation(t *testing.T) {
	r := processWireRecord()
	good, _ := FormatProcessMeasureInterval(r)
	raw, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(good, ProcessMeasureIntervalPrefix+" record="))
	for _, body := range []string{strings.Replace(string(raw), `"owner_status":"known"`, `"owner_status":"known","owner_status":"known"`, 1), strings.Replace(string(raw), `"owner_status":"known"`, `"owner_status":"known","extra":1`, 1)} {
		if _, ok := ParseProcessMeasureInterval(ProcessMeasureIntervalPrefix + " record=" + base64.RawURLEncoding.EncodeToString([]byte(body))); ok {
			t.Fatal("noncanonical JSON accepted")
		}
	}
	for _, bad := range []string{good + " ", strings.Replace(good, "/v1", "/v2", 1)} {
		if _, ok := ParseProcessMeasureInterval(bad); ok {
			t.Fatal("noncanonical carrier accepted")
		}
	}
	if allocations := testing.AllocsPerRun(50, func() { ParseProcessMeasureInterval("task-1 [000] .... 1.000000: print: hello") }); allocations != 0 {
		t.Fatal("allocations added to unrelated rows", allocations)
	}
	for _, bad := range []ProcessMeasureScalar{{Status: "known", StorageClass: "text", Value: "0"}, {Status: "null", StorageClass: "null", Value: "0"}, {Status: "known", StorageClass: "integer", Value: "00"}, {Status: "known", StorageClass: "integer", Value: "9223372036854775808"}, {Status: "invalid_storage", StorageClass: "blob", Value: "!"}} {
		if bad.Valid() {
			t.Fatal("invalid scalar accepted", bad)
		}
	}
}

func TestProcessMeasureWireDoesNotSilentlyRewriteText(t *testing.T) {
	r := processWireRecord()
	r.Name = string([]byte{0xff})
	if _, err := FormatProcessMeasureInterval(r); err == nil {
		t.Fatal("invalid UTF-8 was replaced silently")
	}
	r = processWireRecord()
	r.Value = ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "text", Value: strings.Repeat("x", 1<<20)}
	if _, err := FormatProcessMeasureInterval(r); err == nil {
		t.Fatal("producer emitted a carrier beyond parser limit")
	}
	r = processWireRecord()
	r.FilterID = ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "text", Value: "1"}
	if r.Valid() {
		t.Fatal("noninteger filter carried known owner")
	}
}
