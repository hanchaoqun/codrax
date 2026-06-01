package cmd

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestTraceConvertResultLinesFollowLanguage(t *testing.T) {
	result := hitraceconv.Result{
		InputPath:          "in.htrace",
		OutputPath:         "in.htrace.systrace",
		EventsWritten:      12,
		MissingFormatCount: 2,
		UnknownEventCount:  3,
	}

	zh := strings.Join(traceConvertResultLines("zh", result), "\n")
	if !strings.Contains(zh, "已转换二进制 hitrace") || !strings.Contains(zh, "跳过缺失格式：2") {
		t.Fatalf("zh result lines not localized enough:\n%s", zh)
	}
	if !strings.Contains(zh, "仅行头事件：3") {
		t.Fatalf("zh result lines should report header-only rows:\n%s", zh)
	}
	if strings.Contains(zh, "converted binary hitrace") {
		t.Fatalf("zh result leaked English:\n%s", zh)
	}

	en := strings.Join(traceConvertResultLines("en", result), "\n")
	if !strings.Contains(en, "converted binary hitrace") || !strings.Contains(en, "header_only_events: 3") {
		t.Fatalf("en result lines malformed:\n%s", en)
	}
	if strings.Contains(en, "已转换") {
		t.Fatalf("en result leaked Chinese:\n%s", en)
	}
}

func TestTraceConvertNextLineFollowsLanguage(t *testing.T) {
	if got := traceConvertNextLine("zh", "out.systrace"); !strings.Contains(got, "下一步") || !strings.Contains(got, "<问题>") {
		t.Fatalf("zh next line malformed: %q", got)
	}
	if got := traceConvertNextLine("en", "out.systrace"); !strings.Contains(got, "next:") || !strings.Contains(got, "<question>") {
		t.Fatalf("en next line malformed: %q", got)
	}
}
