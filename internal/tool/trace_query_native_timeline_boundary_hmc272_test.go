package tool

import (
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC272NativeTimelineIndexConfigurationIsNotObservedCoverage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result tracequery.Result
		want   []string
	}{
		{"unbounded", tracequery.Result{}, []string{"索引未按时间限定", "索引未按行限定"}},
		{"time-bounded", tracequery.Result{IndexWindowed: true, IndexTimeStart: 1.75, IndexTimeEnd: 2.25}, []string{"1.750000..2.250000 秒", "索引未按行限定"}},
		{"line-bounded", tracequery.Result{IndexWindowed: true, IndexLineStart: 12, IndexLineEnd: 34}, []string{"时间范围：未发布，不能推定", "行范围：12..34"}},
		{"start-only", tracequery.Result{IndexWindowed: true, IndexLineStart: 12}, []string{"从第 12 行起，末行未限定"}},
		{"end-only", tracequery.Result{IndexWindowed: true, IndexLineEnd: 34}, []string{"截至第 34 行，起行未限定"}},
		{"invalid", tracequery.Result{IndexWindowed: true, IndexTimeStart: math.NaN(), IndexTimeEnd: 2, IndexLineStart: -1}, []string{"时间范围：未发布，不能推定", "行范围：未发布，不能推定"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			note := traceQueryNativeIndexWitnessNote(tc.result)
			for _, want := range append(tc.want, "索引配置时间范围", "不保证配置范围内已全部解析", "关闭事件仅按各表实际端点见证判定") {
				if !strings.Contains(note, want) {
					t.Fatalf("configuration scope %q missing from %q", want, note)
				}
			}
			if strings.Contains(note, "0..0") || strings.Contains(note, "NaN") || strings.Contains(note, "index_windowed") {
				t.Fatalf("unset/invalid configuration became a coordinate or internal display: %q", note)
			}
		})
	}
}
