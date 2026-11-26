package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/logging"
)

func TestOpenAIStreamTiming(t *testing.T) {
	dir := t.TempDir()
	logger, err := logging.NewFromFlags(dir, "debug", false)
	if err != nil {
		t.Fatal(err)
	}
	previousLogger := logging.Default
	logging.SetDefault(logger)
	t.Cleanup(func() {
		logging.SetDefault(previousLogger)
		_ = logger.Close()
	})

	for _, tc := range []struct {
		model, chunks, usage, wantKind, wantCached string
	}{
		{
			model: "tool",
			chunks: `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"emit"}}]}}]}` + "\n" +
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
			usage:      `{"prompt_tokens":96,"completion_tokens":7,"prompt_cache_hit_tokens":64}`,
			wantKind:   "tool_call_delta",
			wantCached: "64",
		},
		{
			model:      "content",
			chunks:     `{"choices":[{"delta":{"content":"answer"}}]}`,
			usage:      `{"prompt_tokens":96,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":0}}`,
			wantKind:   "content_delta",
			wantCached: "0",
		},
		{
			model:      "no_visible_output",
			chunks:     `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"emit"}}]}}]}`,
			usage:      `{"prompt_tokens":96,"completion_tokens":7}`,
			wantKind:   "none",
			wantCached: "unknown",
		},
	} {
		t.Run(tc.model, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, ": heartbeat\n\n"+
					"data: "+`{"choices":[{"delta":{"role":"assistant","reasoning_content":"thinking"}}]}`+"\n\n")
				if tc.chunks != "" {
					_, _ = io.WriteString(w, "data: "+strings.ReplaceAll(tc.chunks, "\n", "\n\ndata: ")+"\n\n")
				}
				_, _ = io.WriteString(w, "data: "+`{"choices":[],"usage":`+tc.usage+"}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()

			adapter := NewOpenAIAdapter("k", tc.model, server.URL, testAdapterOpts(AdapterOptions{Stream: true}))
			if _, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "measure"}}, nil, ChatOptions{}); err != nil {
				t.Fatal(err)
			}
			files, err := filepath.Glob(filepath.Join(dir, "*.log"))
			if err != nil || len(files) != 1 {
				t.Fatalf("expected one log file, got %v, %v", files, err)
			}
			data, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			duration := `\d+`
			if tc.wantKind == "none" {
				duration = "unknown"
			}
			pattern := `\[llm/stream\] timing: model=` + tc.model +
				` ok=true started_at=\S+ first_output_kind=` + tc.wantKind +
				` ttft_ms=` + duration + ` after_first_output_ms=` + duration +
				` total_ms=\d+ input_tokens=96 output_tokens=7 cached_input_tokens=` + tc.wantCached + `\b`
			if !regexp.MustCompile(pattern).Match(data) {
				t.Fatalf("expected %s in logs:\n%s", pattern, data)
			}
		})
	}
}
