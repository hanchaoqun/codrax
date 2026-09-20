package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestHtraceConvertGzipTextTypedDisplay(t *testing.T) {
	const decoded = "/managed/capture 文本.systrace"
	result := hitraceconv.Result{
		InputPath: "/source/original.gz", OutputPath: decoded,
		TextTransport: &hitraceconv.GzipTextTransportResult{
			Profile: hitraceconv.GzipTextTransportProfile, DecodedPath: decoded, DecodedBytes: 1234,
		},
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			success := htraceConvertResultSuccess(lang, result)
			next := htraceConvertNextMsg(lang, result)
			want := []string{"decompressed complete trace text", "text preserved unchanged", "events not yet counted"}
			boundary := "decompression alone does not prove causality"
			if lang == "zh" {
				want = []string{"已完整解压文本 trace", "原文保持不变", "事件尚未统计"}
				boundary = "解压本身不代表因果已证"
			}
			for _, part := range append(want, decoded, "1234") {
				if !strings.Contains(success, part) {
					t.Fatalf("text transport success missing %q: %s", part, success)
				}
			}
			if !strings.Contains(next, "/htrace "+decoded) || !strings.Contains(next, boundary) || strings.Contains(next, result.InputPath) {
				t.Fatalf("next step lost decoded path or causal boundary: %s", next)
			}
			for _, claim := range []string{"converted hitrace:", "converted binary hitrace", "已转换 hitrace", "二进制", "0 events", "0 个事件", "trace_query_ready=true", "tracebundle"} {
				if strings.Contains(success+"\n"+next, claim) {
					t.Fatalf("byte transport made unsupported semantic claim %q: %s\n%s", claim, success, next)
				}
			}
		})
	}
}

func TestHtraceConvertGzipTextDisplayRequiresExactTypedProfile(t *testing.T) {
	for _, profile := range []string{"", "future_text_profile"} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(profile+"/"+lang, func(t *testing.T) {
				result := hitraceconv.Result{InputPath: "capture.gz", OutputPath: "converted.systrace", EventsWritten: 7}
				if profile != "" {
					result.TextTransport = &hitraceconv.GzipTextTransportResult{Profile: profile, DecodedPath: "unapproved-text.systrace", DecodedBytes: 123}
				}
				success := htraceConvertResultSuccess(lang, result)
				next := htraceConvertNextMsg(lang, result)
				want := "7 events"
				if lang == "zh" {
					want = "7 个事件"
				}
				if !strings.Contains(success, want) || strings.Contains(success+next, "events not yet counted") || strings.Contains(success+next, "事件尚未统计") || strings.Contains(success+next, "unapproved-text.systrace") {
					t.Fatalf("suffix or unknown profile selected typed text branch: %s\n%s", success, next)
				}
			})
		}
	}
}
