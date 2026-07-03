package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

// --- degenerateTailPeriod ---

func TestDegenerateTailPeriod_PeriodicCJKSentence(t *testing.T) {
	unit := "综合所有 trace 证据，给出完整诊断结论。\n\n"
	repeats := degenerateTailWindowBytes/len(unit) + 2
	s := strings.Repeat(unit, repeats)
	s = s[len(s)-degenerateTailWindowBytes:]
	p := degenerateTailPeriod(s, degenerateMaxPeriodBytes)
	if p <= 0 {
		t.Fatalf("expected a positive period for a fully periodic window, got %d", p)
	}
	if p > len(unit) {
		t.Fatalf("smallest period %d must not exceed the unit length %d", p, len(unit))
	}
	// The window is cut at an arbitrary byte offset, so the detected
	// period must still tile the window exactly.
	for i := p; i < len(s); i++ {
		if s[i] != s[i-p] {
			t.Fatalf("window not periodic under detected period %d at index %d", p, i)
		}
	}
}

func TestDegenerateTailPeriod_NonPeriodicReturnsZero(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < degenerateTailWindowBytes; i++ {
		fmt.Fprintf(&b, "line %d: unique content about thread %d with %dms\n", i, i*7, i*3)
	}
	s := b.String()[:degenerateTailWindowBytes]
	if p := degenerateTailPeriod(s, degenerateMaxPeriodBytes); p != 0 {
		t.Fatalf("varied content must not report a period, got %d", p)
	}
}

func TestDegenerateTailPeriod_SingleDivergentByteDefusesSignal(t *testing.T) {
	unit := strings.Repeat("A", 100) + "."
	s := strings.Repeat(unit, degenerateTailWindowBytes/len(unit)+1)[:degenerateTailWindowBytes]
	mid := []byte(s)
	mid[len(mid)/2] = '#'
	if p := degenerateTailPeriod(string(mid), degenerateMaxPeriodBytes); p != 0 {
		t.Fatalf("a single divergent byte must defuse whole-window periodicity, got period %d", p)
	}
}

func TestDegenerateTailPeriod_PeriodAboveCapReturnsZero(t *testing.T) {
	unit := strings.Repeat("x", degenerateMaxPeriodBytes+1) + "!"
	s := strings.Repeat(unit, 8)
	s = s[len(s)-degenerateTailWindowBytes*2:]
	if p := degenerateTailPeriod(s, degenerateMaxPeriodBytes); p != 0 {
		t.Fatalf("period above cap must return 0, got %d", p)
	}
}

// --- truncateDegenerateTail ---

func TestTruncateDegenerateTail_KeepsPrefixPlusTwoPeriods(t *testing.T) {
	prefix := "正常前缀内容:分析已经完成。\n"
	unit := "综合所有 trace 证据，给出完整诊断结论。\n\n"
	s := prefix + strings.Repeat(unit, 50)
	out := truncateDegenerateTail(s, len(unit))
	// The backward walk is rotation-invariant: when the prefix shares
	// suffix bytes with the unit the detected run starts a few bytes
	// early, so assert structure rather than exact byte alignment.
	if !strings.HasPrefix(s, out) {
		t.Fatalf("output must be a prefix of the input")
	}
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("pre-loop prefix must survive, got %q", out)
	}
	if got := strings.Count(out, "综合所有 trace 证据"); got != 2 {
		t.Fatalf("exactly two evidence periods must survive, got %d occurrences in %q", got, out)
	}
	if len(out) > len(prefix)+3*len(unit) {
		t.Fatalf("truncation kept too much: %d bytes", len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("truncated output must remain valid UTF-8")
	}
}

func TestTruncateDegenerateTail_ShortInputUntouched(t *testing.T) {
	s := "short"
	if out := truncateDegenerateTail(s, 10); out != s {
		t.Fatalf("short input must pass through, got %q", out)
	}
}

// --- parseSSEStreamTracked integration ---

func sseContentChunk(text string) string {
	// Escape via fmt: content is plain in these tests, no quotes.
	return "data: {\"choices\":[{\"delta\":{\"content\":" + jsonQuote(text) + "}}]}\n\n"
}

func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func TestParseSSEStream_DegenerateBreakerFires(t *testing.T) {
	unit := "综合所有 trace 证据，给出完整诊断结论。\n\n"
	var stream strings.Builder
	stream.WriteString(sseContentChunk("先给出一段正常的开场分析内容。\n"))
	// Feed far more than the fire threshold so the breaker MUST stop
	// before the end (proves early exit, not end-of-stream trimming).
	repeats := (degenerateMinContentBytes + degenerateCheckStride*8) / len(unit)
	for i := 0; i < repeats; i++ {
		stream.WriteString(sseContentChunk(unit))
	}
	stream.WriteString("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	stream.WriteString("data: [DONE]\n\n")

	var progress atomic.Int64
	var firstByte atomic.Bool
	resp, err := parseSSEStreamTracked(strings.NewReader(stream.String()), nil, nil, nil, &progress, &firstByte, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != finishReasonDegenerateRepetition {
		t.Fatalf("expected StopReason=%s, got %q", finishReasonDegenerateRepetition, resp.StopReason)
	}
	if len(resp.Content) > degenerateMinContentBytes+degenerateCheckStride*2 {
		t.Fatalf("content should be truncated near the detection point, got %d bytes", len(resp.Content))
	}
	if !strings.Contains(resp.Content, "正常的开场分析内容") {
		t.Fatalf("pre-loop prefix must survive truncation")
	}
	if !strings.Contains(resp.Content, strings.TrimSpace(unit)) {
		t.Fatalf("two evidence periods of the repeated unit must survive truncation")
	}
	if !utf8.ValidString(resp.Content) {
		t.Fatalf("truncated content must remain valid UTF-8")
	}
}

func TestParseSSEStream_NormalLongContentNotTruncated(t *testing.T) {
	var stream strings.Builder
	total := 0
	for i := 0; total < degenerateMinContentBytes*3; i++ {
		text := fmt.Sprintf("第%d段:线程 %d 在窗口内运行 %dms,唤醒对端 %d。\n", i, i*13, i*7, i*29)
		stream.WriteString(sseContentChunk(text))
		total += len(text)
	}
	stream.WriteString("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	stream.WriteString("data: [DONE]\n\n")

	var progress atomic.Int64
	var firstByte atomic.Bool
	resp, err := parseSSEStreamTracked(strings.NewReader(stream.String()), nil, nil, nil, &progress, &firstByte, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("varied long content must finish normally, got StopReason=%q", resp.StopReason)
	}
	if len(resp.Content) < degenerateMinContentBytes*3 {
		t.Fatalf("varied content must not be truncated, got %d bytes", len(resp.Content))
	}
}

// --- total wall-clock cap ---

func TestStreamTotalWallClockCap(t *testing.T) {
	if got := streamTotalWallClockCap(0); got != 0 {
		t.Fatalf("unset requestTimeout must disable the cap, got %v", got)
	}
	if got := streamTotalWallClockCap(10 * time.Minute); got != 20*time.Minute {
		t.Fatalf("cap must be 2×requestTimeout, got %v", got)
	}
}

func TestStreamTotalTimeoutError_Sentinel(t *testing.T) {
	wrapped := &StreamTotalTimeoutError{Elapsed: 21 * time.Minute, Cap: 20 * time.Minute, Cause: errStreamProbe}
	if !errors.Is(wrapped, ErrStreamTotalTimeout) {
		t.Fatalf("errors.Is(wrapped, ErrStreamTotalTimeout) should be true")
	}
	if errors.Is(wrapped, ErrStreamStalled) || errors.Is(wrapped, ErrStreamFirstByteTimeout) {
		t.Fatalf("total-timeout error must be distinct from stall/first-byte sentinels")
	}
	if !strings.Contains(wrapped.Error(), "total wall-clock cap") {
		t.Fatalf("error message should name the total wall-clock cap, got %q", wrapped.Error())
	}
}

var errStreamProbe = errors.New("probe")

// TestDoStreamRequest_TotalWallClockCapFires covers the runaway case the
// customer hit 2026-07-03: visible deltas keep flowing (resetting the
// stall and visible-output watchdogs) but the request as a whole runs
// past every budget. The server emits a fresh visible delta every 30ms
// forever; with requestTimeout=400ms the total cap (800ms) must abort.
func TestDoStreamRequest_TotalWallClockCapFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				i++
				// Varied text so the degeneration breaker cannot fire first.
				_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"tok%d \"}}]}\n\n", i)
				if f != nil {
					f.Flush()
				}
			}
		}
	}))
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, AdapterOptions{
		Stream:                 true,
		RequestTimeout:         400 * time.Millisecond,
		RetryMaxAttempts:       1,
		StreamFirstByteTimeout: 400 * time.Millisecond,
		StreamStallTimeout:     5 * time.Second,
	})

	start := time.Now()
	_, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, ChatOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected total wall-clock cap error; got nil")
	}
	if !errors.Is(err, ErrStreamTotalTimeout) {
		t.Fatalf("expected ErrStreamTotalTimeout in chain; got %v", err)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("total cap took %v to fire; expected well under 4s", elapsed)
	}
}
