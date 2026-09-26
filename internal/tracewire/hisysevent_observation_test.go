package tracewire

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func hisysWireFixture() HiSysEvent {
	zero := int64(0)
	name := "域/事件: name\n\r\x00"
	content := "row\nprint: FAKE/NAME: forged\r\x00\t\"\\雪"
	return HiSysEvent{TimestampNS: 0, SourceTID: &zero, Domain: HiSysEventName{Status: "resolved", Name: &name, Reference: &zero},
		Event: HiSysEventName{Status: "null_reference"}, Contents: HiSysEventContents{StorageClass: "text", Text: &content}}
}

func TestHiSysObservationNonMatchingRowsAllocateNothing(t *testing.T) {
	// Every ordinary row is probed by multiple trace readers. JSON decoding
	// state must not escape to the heap until the wire prefix is accepted.
	for _, line := range []string{
		"worker-23 (23) [002] .... 4.030000: tracing_mark_write: E|23",
		"# codrax_hisysevent/v1x ts_ns=0 payload=invalid",
		"# ordinary trace comment",
	} {
		allocs := testing.AllocsPerRun(1000, func() {
			if _, ok := ParseHiSysEventObservation(line); ok {
				t.Fatal("ordinary row admitted as a HiSys observation")
			}
		})
		if allocs != 0 {
			t.Errorf("nonmatching wire probe allocates %g objects per row", allocs)
		}
	}
}

func TestHiSysObservationWireReversibleAndStrict(t *testing.T) {
	want := hisysWireFixture()
	line, err := FormatHiSysEventObservation(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(line, "\r\n\x00") {
		t.Fatal("payload introduced physical rows")
	}
	got, ok := ParseHiSysEventObservation(line)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip lost original fields: %+v", got)
	}
	for _, bad := range []string{line + " ", " " + line, strings.Replace(line, "ts_ns=0", "ts_ns=1", 1), strings.Replace(line, "ts_ns=0", "ts_ns=00", 1), line + "\n"} {
		if _, ok := ParseHiSysEventObservation(bad); ok {
			t.Errorf("malformed wire admitted: %.100q", bad)
		}
	}
	_, payload, _ := strings.Cut(line, " payload=")
	b, _ := base64.RawURLEncoding.DecodeString(payload)
	for name, jsonBody := range map[string]string{
		"duplicate": strings.Replace(string(b), `"timestamp_ns":"0"`, `"timestamp_ns":"0","timestamp_ns":"0"`, 1),
		"unknown":   strings.TrimSuffix(string(b), "}") + `,"authority":"root_cause"}`,
		"trailing":  string(b) + "{}", "invalidutf8": string(append(b, 0xff)),
		"surrogate": strings.Replace(string(b), `"status":"null_reference"`, `"status":"\ud800"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := ParseHiSysEventObservation(HiSysEventObservationPrefix + " ts_ns=0 payload=" + base64.RawURLEncoding.EncodeToString([]byte(jsonBody))); ok {
				t.Fatal("noncanonical or invalid JSON admitted")
			}
		})
	}
	for _, mutate := range []func(*HiSysEvent){
		func(e *HiSysEvent) { e.TimestampNS = -1 }, func(e *HiSysEvent) { v := int64(-1); e.SourceTID = &v },
		func(e *HiSysEvent) { v := int64(1 << 31); e.SourceTID = &v }, func(e *HiSysEvent) { s := string([]byte{0xff}); e.Contents.Text = &s },
		func(e *HiSysEvent) { s := strings.Repeat("x", MaxHiSysEventObservationBytes); e.Contents.Text = &s },
		func(e *HiSysEvent) { e.Domain.Status = "unresolved_reference" },
	} {
		e := hisysWireFixture()
		mutate(&e)
		if _, err := FormatHiSysEventObservation(e); err == nil {
			t.Fatal("invalid observation formatted")
		}
	}
}

func TestHiSysObservationStorageAndNameStatesRemainDistinct(t *testing.T) {
	empty := ""
	zero := int64(0)
	for _, c := range []HiSysEventContents{{StorageClass: "null"}, {StorageClass: "text", Text: &empty}, {StorageClass: "blob"}, {StorageClass: "blob", BytesBase64: "AP8="}} {
		e := hisysWireFixture()
		e.SourceTID = nil
		e.Domain = HiSysEventName{Name: &empty, Status: "resolved", Reference: &zero}
		e.Contents = c
		line, err := FormatHiSysEventObservation(e)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := ParseHiSysEventObservation(line)
		if !ok || !reflect.DeepEqual(e, got) {
			t.Fatalf("NULL/zero/empty storage collapsed: %+v", got)
		}
		b, _ := json.Marshal(got)
		if len(b) == 0 {
			t.Fatal("missing JSON")
		}
	}
}
