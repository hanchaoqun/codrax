package loginput

import (
	"context"
	"testing"
)

func TestPreparedFrameFactRequiresCompleteStandaloneRecord(t *testing.T) {
	const frame = `{"record_kind":"frame","side":"target","metric_kind":"jank","count":1,"thread_id":42,"start_ns":0,"end_ns":1000,"observation_id":"o","source_id":"s","frame_metrics":{"duration_ns":1000}}`
	for _, tc := range []struct {
		name string
		src  []Input
		want bool
	}{
		{"frame", []Input{{Name: "facts", Data: []byte(frame + "\r\n")}}, true},
		{"invalid window", []Input{{Name: "facts", Data: []byte(`{"record_kind":"frame","side":"target","metric_kind":"jank","count":1,"thread_id":42,"start_ns":1000,"end_ns":0,"observation_id":"o","source_id":"s","frame_metrics":{"duration_ns":1000}}`)}}, false},
		{"mixed records", []Input{{Name: "facts", Data: []byte(frame + "\npanic: failed\n")}}, false},
		{"multiple attachments", []Input{{Name: "facts", Data: []byte(frame)}, {Name: "log", Data: []byte("panic: failed")}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Prepare(context.Background(), tc.src, Options{})
			if err != nil {
				t.Fatal(err)
			}
			fact, ok := c.PreparedFrameFact()
			if ok != tc.want {
				t.Fatalf("got prepared record = %t, want %t", ok, tc.want)
			}
			if ok {
				record, _, err := c.ReadRecord(context.Background(), fact.RecordRef)
				if err != nil || fact.Text != frame || record.FirstLine != 1 {
					t.Fatalf("lost attachment identity or record contents: %v", err)
				}
			}
		})
	}
}
