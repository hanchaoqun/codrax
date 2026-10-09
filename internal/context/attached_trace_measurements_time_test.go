package context

import (
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"testing"
)

func TestMeasurementsAttachmentSignedAndUnknownSourceTime(t *testing.T) {
	for _, tc := range []struct{ storage, value, ns, seconds string }{{"integer", "0", "0", "0.000000000"}, {"integer", "-1", "-1", "-0.000000001"}, {"integer", "-9223372036854775808", "-9223372036854775808", "-9223372036.854775808"}, {"null", "", "", ""}, {"real", "1", "", ""}, {"text", "1", "", ""}} {
		t.Run(tc.storage+tc.value, func(t *testing.T) {
			null := tracewire.MeasureScalar{StorageClass: "null"}
			r := tracewire.MeasureInterval{RowID: 1, StartNS: tracewire.MeasureScalar{StorageClass: tc.storage, Value: tc.value}, DurationNS: null, Value: tracewire.MeasureScalar{StorageClass: "integer", Value: "9007199254740993"}, FilterID: null, MeasureType: null, FilterStatus: "unknown"}
			line, err := tracewire.FormatMeasureInterval(r)
			if err != nil {
				t.Fatal(err)
			}
			assertProcessMeasurePreviewTime(t, renderAttachedTraceSemantics(tracePreviewPart{line, 1, false}), line, tc.ns, tc.seconds)
		})
	}
}
