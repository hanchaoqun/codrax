package cmd

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestTraceConvertGzipTextRealTransportDisplay(t *testing.T) {
	dir := t.TempDir()
	for _, key := range []string{"CODRAX_TRACE_STREAMER", "CODRAX_HIPERF_HOST", "CODRAX_SIMPLEPERF_REPORT_SAMPLE"} {
		t.Setenv(key, filepath.Join(dir, "missing-provider"))
	}
	text := []byte("# trace text remains unchanged\r\nworker-42 (42) [000] .... 1.000000: sched_wakeup: comm=target pid=43 prio=120 target_cpu=000\r\n# complete tail without newline")
	var encoded bytes.Buffer
	writer := gzip.NewWriter(&encoded)
	writer.Name = "not-the-output.perf.data"
	if _, err := writer.Write(text); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "capture 原件.bin")
	output := filepath.Join(dir, "prepared 文本.systrace")
	if err := os.WriteFile(input, encoded.Bytes(), 0400); err != nil {
		t.Fatal(err)
	}
	result, err := hitraceconv.ConvertFile(context.Background(), hitraceconv.Options{
		InputPath: input, OutputPath: output, RuntimeAnchor: filepath.Join(dir, "runtime"),
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := result.TextTransport
	if transport == nil || transport.Profile != hitraceconv.GzipTextTransportProfile || transport.DecodedPath != output || transport.DecodedBytes != int64(len(text)) || result.EventsWritten != 0 || len(result.Artifacts) != 0 || result.BundlePath != "" || result.GzipInputProvenance != nil {
		t.Fatalf("text-only transport gained a semantic converter claim: %+v", result)
	}
	if decoded, err := os.ReadFile(output); err != nil || !bytes.Equal(decoded, text) {
		t.Fatalf("complete text bytes changed: err=%v", err)
	}
	if original, err := os.ReadFile(input); err != nil || !bytes.Equal(original, encoded.Bytes()) {
		t.Fatalf("original gzip changed: err=%v", err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			lines := strings.Join(traceConvertResultLines(lang, result), "\n")
			next := traceConvertNextLine(lang, result)
			want := []string{"decompressed complete trace text", "text preserved unchanged", "events not yet counted"}
			boundary := "decompression alone does not prove causality"
			if lang == "zh" {
				want = []string{"已完整解压文本 trace", "原文保持不变", "事件尚未统计"}
				boundary = "解压本身不代表因果已证"
			}
			for _, part := range append(want, input, output, fmt.Sprint(len(text))) {
				if !strings.Contains(lines, part) {
					t.Fatalf("text transport display missing %q: %s", part, lines)
				}
			}
			if !strings.Contains(next, fmt.Sprintf("--htrace %q", output)) || !strings.Contains(next, boundary) || strings.Contains(next, input) || strings.Contains(next, writer.Name) {
				t.Fatalf("next step lost receipt-approved decoded path or causal boundary: %s", next)
			}
			for _, claim := range []string{"converted binary hitrace", "已转换二进制", "events: 0", "0 events", "事件：0", "0 个事件", "trace_query_ready=true", "provider_decision["} {
				if strings.Contains(lines+"\n"+next, claim) {
					t.Fatalf("byte transport made unsupported semantic claim %q: %s\n%s", claim, lines, next)
				}
			}
		})
	}
}

func TestTraceConvertGzipTextDisplayRequiresExactTypedProfile(t *testing.T) {
	for _, profile := range []string{"", "future_text_profile"} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(profile+"/"+lang, func(t *testing.T) {
				result := hitraceconv.Result{InputPath: "capture.gz", OutputPath: "converted.systrace", EventsWritten: 7}
				if profile != "" {
					result.TextTransport = &hitraceconv.GzipTextTransportResult{Profile: profile, DecodedPath: "unapproved-text.systrace", DecodedBytes: 123}
				}
				lines := strings.Join(traceConvertResultLines(lang, result), "\n")
				next := traceConvertNextLine(lang, result)
				want := "events: 7"
				if lang == "zh" {
					want = "事件：7"
				}
				if !strings.Contains(lines, want) || strings.Contains(lines+next, "events not yet counted") || strings.Contains(lines+next, "事件尚未统计") || strings.Contains(lines+next, "unapproved-text.systrace") {
					t.Fatalf("suffix or unknown profile selected typed text branch: %s\n%s", lines, next)
				}
			})
		}
	}
}
