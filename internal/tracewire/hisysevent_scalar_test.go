package tracewire

import "testing"

func TestHiSysInvalidSourceTIDWireRoundTrip(t *testing.T) {
	for _, tc := range []struct{ class, text string }{{"text", "100"}, {"real", "100"}, {"integer", "-1"}, {"integer", "2147483648"}} {
		row := HiSysEvent{TimestampNS: 9007199254740993, Domain: HiSysEventName{Status: "null_reference"}, Event: HiSysEventName{Status: "null_reference"}, Contents: HiSysEventContents{StorageClass: "null"}, SourceTIDRaw: &HiSysEventContents{StorageClass: tc.class, Text: &tc.text}}
		line, err := FormatHiSysEventObservation(row)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := ParseHiSysEventObservation(line)
		if !ok || got.TimestampNS != row.TimestampNS || got.SourceTID != nil || got.SourceTIDRaw == nil || *got.SourceTIDRaw.Text != tc.text || got.SourceTIDRaw.StorageClass != tc.class {
			t.Fatalf("wire corruption: %+v", got)
		}
		valid := int64(100)
		row.SourceTID = &valid
		if _, err := FormatHiSysEventObservation(row); err == nil {
			t.Fatal("simultaneous valid and invalid source identity accepted")
		}
	}
	for _, value := range []string{"0", "100", "2147483647"} {
		row := HiSysEvent{Domain: HiSysEventName{Status: "null_reference"}, Event: HiSysEventName{Status: "null_reference"}, Contents: HiSysEventContents{StorageClass: "null"}, SourceTIDRaw: &HiSysEventContents{StorageClass: "integer", Text: &value}}
		if _, err := FormatHiSysEventObservation(row); err == nil {
			t.Fatal("valid integer allowed on invalid lane")
		}
	}
}
