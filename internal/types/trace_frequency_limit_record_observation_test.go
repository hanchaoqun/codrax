package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestB1630CFrequencyLimitRecordObservationSeparatesCountAndSelectedRecord(t *testing.T) {
	w := TraceFrequencyLimitAuthority{
		CPU: 4, MinFrequencyKHz: 558000, MaxFrequencyKHz: 2100000, LimitRowCount: 28,
		WitnessLine: 17113, WitnessTs: 13762.940114, WindowStartTs: 13762.791708, WindowEndTs: 13763.024898,
		Authority: "direct_in_window_policy_limit",
	}
	before, _ := json.Marshal(w)
	for _, tc := range []struct {
		lang string
		want []string
	}{
		{"zh-CN", []string{"当前查询范围内有效策略记录共 28 条", "正上限最低的一条代表记录", "558000–2100000 kHz", "不是这组上下限重复出现的次数", "不证明整窗保持该策略或策略持续时长"}},
		{"en", []string{"28 valid frequency-policy records", "current query scope", "one representative record with the lowest positive upper bound", "558000–2100000 kHz", "not a repetition count", "not whole-window constancy or policy duration"}},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			got := FormatTraceFrequencyLimitRecordObservation(w, tc.lang)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("two read sets became ambiguous; missing %q: %s", want, got)
				}
			}
			if !strings.HasSuffix(got, TraceFrequencyLimitRecordCaliber(tc.lang)) {
				t.Fatalf("observation and numeric guidance need the same caliber boundary: %s", got)
			}
			if got != FormatTraceFrequencyLimitRecordObservation(w, tc.lang) {
				t.Fatal("display is not idempotent")
			}
		})
	}
	after, _ := json.Marshal(w)
	if string(before) != string(after) {
		t.Fatal("formatter changed the original selected record/census")
	}
}

func TestB1630CFrequencyLimitZeroMaximumRemainsInventoryNotPositiveAuthority(t *testing.T) {
	w := TraceFrequencyLimitAuthority{CPU: 4, MinFrequencyKHz: 0, MaxFrequencyKHz: 0, LimitRowCount: 3}
	for _, lang := range []string{"zh", "en"} {
		got := FormatTraceFrequencyLimitRecordObservation(w, lang)
		wants := []string{"3 valid frequency-policy records", "0–0 kHz", "No positive policy ceiling is supplied", "does not prove policy absence"}
		if lang == "zh" {
			wants = []string{"有效策略记录共 3 条", "0–0 kHz", "未提供正的策略上限", "不能据此断言没有策略"}
		}
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("zero-only inventory lost its actual values/boundary %q: %s", want, got)
			}
		}
		for _, forbidden := range []string{"最低的一条代表记录", "lowest positive upper bound", "第 0 行", "0.000000", "duration=", "持续 0"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("unknown/nonpositive input gained positive policy or fabricated coordinates %q: %s", forbidden, got)
			}
		}
	}
}
