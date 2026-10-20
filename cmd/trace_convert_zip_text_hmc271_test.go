package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestHMC271ZIPTextRealTransportDisplay(t *testing.T) {
	dir := t.TempDir()
	const member = "session/capture.data"
	text := []byte("# tracer: nop\nworker-42 (42) [000] .... 1.000000: sched_wakeup: comm=target pid=43 prio=120 target_cpu=000\n")
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	entry, err := w.Create(member)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(text); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	input, output := filepath.Join(dir, "资料.zip"), filepath.Join(dir, "complete.systrace")
	if err := os.WriteFile(input, archive.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	result, err := hitraceconv.ConvertFile(t.Context(), hitraceconv.Options{InputPath: input, OutputPath: output, RuntimeAnchor: filepath.Join(dir, "runtime")})
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := strings.Join(traceConvertResultLines(lang, result), "\n")
			next := traceConvertNextLine(lang, result)
			for _, part := range []string{input, output, member, fmt.Sprint(len(text))} {
				if !strings.Contains(lines, part) {
					t.Errorf("text/source/member missing %q: %s", part, lines)
				}
			}
			uncounted, boundary := "events not yet counted", "decompression alone does not prove causality"
			if lang == "zh" {
				uncounted, boundary = "事件尚未统计", "解压本身不代表因果已证"
			}
			if !strings.Contains(lines, uncounted) || !strings.Contains(next, boundary) || !strings.Contains(next, fmt.Sprintf("--htrace %q", output)) {
				t.Errorf("byte publication was mistaken for semantic conversion: %s\n%s", lines, next)
			}
			for _, claim := range []string{"converted binary", "已转换二进制", "events: 0", "事件：0", "trace_query_ready=true"} {
				if strings.Contains(lines+next, claim) {
					t.Errorf("unsupported semantic claim %q: %s\n%s", claim, lines, next)
				}
			}
			result.ArchiveTextTransport.Profile = "unknown_future"
			if strings.Contains(strings.Join(traceConvertResultLines(lang, result), "\n"), uncounted) {
				t.Fatal("unknown profile gained transport display authority")
			}
			result.ArchiveTextTransport.Profile = hitraceconv.ArchiveTextTransportProfile
		})
	}
}
