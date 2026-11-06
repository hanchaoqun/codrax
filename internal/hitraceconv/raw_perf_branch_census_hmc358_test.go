package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"
)

func TestHMC358RawBranchCountZeroMissingAndMalformed(t *testing.T) {
	base := make([]byte, 24) // PID/TID, timestamp, period
	binary.LittleEndian.PutUint32(base, 10)
	binary.LittleEndian.PutUint32(base[4:], 11)
	binary.LittleEndian.PutUint64(base[8:], 1_000_000_000)
	binary.LittleEndian.PutUint64(base[16:], 3)
	for _, tc := range []struct {
		name            string
		declared, valid bool
		tail            int
		count           uint64
		want            string
	}{
		{"field_absent", false, true, 0, 0, ""},
		{"recorded_empty", true, true, 8, 0, "branch_count=0"},
		{"one_recorded_slot", true, true, 32, 1, "branch_count=1"},
		{"missing_count_bytes", true, false, 0, 0, ""},
		{"truncated_entry", true, false, 31, 1, ""},
		{"over_bound", true, false, 8, 4097, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attr := rawPerfAttr{SampleType: perfSampleTID | perfSampleTime | perfSamplePeriod}
			if tc.declared {
				attr.SampleType |= perfSampleBranchStack
			}
			tail := make([]byte, tc.tail)
			if len(tail) >= 8 {
				binary.LittleEndian.PutUint64(tail, tc.count)
			}
			sample, ok := parseRawPerfSample(append(bytes.Clone(base), tail...), attr)
			if ok != tc.valid {
				t.Fatalf("admission=%v want=%v", ok, tc.valid)
			}
			if !ok {
				return
			}
			if sample.BranchStackCountKnown != tc.declared || sample.BranchStackCount != tc.count || sample.Period != 3 {
				t.Fatalf("wrong count, presence or weight: %+v", sample)
			}
			var out bytes.Buffer
			if err := writeRawPerfDataPerfTrace(context.Background(), &out, rawPerfData{Samples: []rawPerfSample{sample}}); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && strings.Contains(out.String(), "branch_count=") ||
				tc.want != "" && !strings.Contains(out.String(), tc.want) {
				t.Fatalf("wrong wire count: %s", out.String())
			}
		})
	}
}
