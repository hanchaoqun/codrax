package tracequery

import "testing"

func TestDecodeTraceMarkEndpointPayloadUsesCanonicalVerdict(t *testing.T) {
	tests := []struct {
		payload string
		want    TraceMarkPayloadVerdict
	}{
		{
			payload: "B|42|render|nested",
			want: TraceMarkPayloadVerdict{
				Recognized: true, Admitted: true, Action: "B",
				SpanPID: 42, Name: "render|nested",
			},
		},
		{
			payload: "E|42|D0005",
			want: TraceMarkPayloadVerdict{
				Recognized: true, Action: "E",
				InvalidCause: "invalid_end_tag",
			},
		},
		{
			payload: "S|42|work|7",
			want: TraceMarkPayloadVerdict{
				Recognized: true, Admitted: true, Action: "S",
				SpanPID: 42, Name: "work", Value: "7",
			},
		},
		{payload: "C|42|counter|1", want: TraceMarkPayloadVerdict{}},
		{payload: "ordinary print text", want: TraceMarkPayloadVerdict{}},
	}
	for _, test := range tests {
		if got := DecodeTraceMarkEndpointPayload(test.payload); got != test.want {
			t.Fatalf("payload %q: got %+v want %+v", test.payload, got, test.want)
		}
	}
}
