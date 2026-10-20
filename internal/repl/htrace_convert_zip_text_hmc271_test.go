package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestHMC271ZIPTextREPLDisplay(t *testing.T) {
	result := hitraceconv.Result{
		InputPath: "/source/captures.zip", OutputPath: "/managed/complete.systrace",
		ArchiveTextTransport: &hitraceconv.ArchiveTextTransportResult{
			Profile: hitraceconv.ArchiveTextTransportProfile, DecodedPath: "/managed/complete.systrace", DecodedBytes: 1234,
			Archive: hitraceconv.TraceArchiveProvenance{Member: "session/capture.data"},
		},
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			success, next := htraceConvertResultSuccess(lang, result), htraceConvertNextMsg(lang, result)
			uncounted, boundary := "events not yet counted", "decompression alone does not prove causality"
			if lang == "zh" {
				uncounted, boundary = "事件尚未统计", "解压本身不代表因果已证"
			}
			for _, want := range []string{result.OutputPath, "1234", "session/capture.data", uncounted} {
				if !strings.Contains(success, want) {
					t.Fatalf("missing text publication %q: %s", want, success)
				}
			}
			if !strings.Contains(next, "/htrace "+result.OutputPath) || !strings.Contains(next, boundary) {
				t.Fatalf("wrong next step: %s", next)
			}
			for _, claim := range []string{"converted hitrace", "已转换 hitrace", "0 events", "0 个事件", "trace_query_ready=true"} {
				if strings.Contains(success+next, claim) {
					t.Fatalf("byte publication claimed %q: %s %s", claim, success, next)
				}
			}
			result.ArchiveTextTransport.Profile = "unknown"
			if strings.Contains(htraceConvertResultSuccess(lang, result), uncounted) {
				t.Fatal("unknown receipt accepted")
			}
			result.ArchiveTextTransport.Profile = hitraceconv.ArchiveTextTransportProfile
		})
	}
}
