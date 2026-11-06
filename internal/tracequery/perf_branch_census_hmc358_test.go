package tracequery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHMC358BranchCensusIsScopedAndIndependentOfWeight(t *testing.T) {
	raw := strings.Replace(perfTextCodecBody(), "source=fixture", "source=raw_perfdata_fallback", 1)
	for _, tc := range []struct {
		name     string
		suffixes []string
		want     string
	}{
		{"unavailable", []string{""}, ""},
		{"recorded_zero", []string{" branch_count=0"}, `{"samples_with_count":1,"samples_with_entries":0,"samples_without_count":0,"recorded_entries":0}`},
		{"mixed", []string{" branch_count=0", " branch_count=2", " branch_count=7", "", " branch_count=4097", " branch_count=1 branch_count=0", " branch_count=-1"}, `{"samples_with_count":3,"samples_with_entries":2,"samples_without_count":4,"recorded_entries":9}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			acc := newPerfQualityAcc()
			for _, suffix := range tc.suffixes {
				row := parsePerfTextCodecRow(t, raw+suffix)
				if row.PerfFields.Period != 11 || row.PerfFields.TID != 34 {
					t.Fatal("branch metadata altered separate weight or identity")
				}
				acc.add(row, 11, "")
			}
			for _, weighted := range []bool{false, true} {
				body, _ := json.Marshal(acc.summary(11*int64(len(tc.suffixes)), weighted))
				var quality map[string]json.RawMessage
				if err := json.Unmarshal(body, &quality); err != nil {
					t.Fatal(err)
				}
				if got := string(quality["branch_records"]); got != tc.want {
					t.Errorf("branch census: got=%s want=%s", got, tc.want)
				}
			}
		})
	}
	acc := newPerfQualityAcc()
	acc.add(parsePerfTextCodecRow(t, perfTextCodecBody()+" branch_count=7"), 11, "")
	body, _ := json.Marshal(acc.summary(11, true))
	if strings.Contains(string(body), "branch_records") {
		t.Fatal("unqualified producer acquired a raw branch census")
	}
}
