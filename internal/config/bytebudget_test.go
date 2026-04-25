package config

import (
	"testing"
	"time"
)

// TestResolveByteBudget_PrecedenceMatrix pins the three-way
// precedence rule: fraction (with context_window) wins; absolute is
// the fallback; code default is the floor when neither is set.
// Drift here silently changes every byte-cap knob's meaning.
func TestResolveByteBudget_PrecedenceMatrix(t *testing.T) {
	mkFrac := func(f float64) *float64 { return &f }
	mkInt := func(i int) *int { return &i }

	cases := []struct {
		name        string
		fraction    *float64
		absolute    *int
		codeDefault int
		window      int
		want        int
	}{
		{
			name:        "fraction+window wins over absolute",
			fraction:    mkFrac(0.01), // 1%
			absolute:    mkInt(99999),
			codeDefault: 32768,
			window:      200_000, // 200k tokens × 0.01 × 4 B/tok = 8 000 B
			want:        8000,
		},
		{
			name:        "fraction without window falls back to absolute",
			fraction:    mkFrac(0.1),
			absolute:    mkInt(32768),
			codeDefault: 16384,
			window:      0,
			want:        32768,
		},
		{
			name:        "nil fraction uses absolute",
			fraction:    nil,
			absolute:    mkInt(65536),
			codeDefault: 32768,
			window:      200_000,
			want:        65536,
		},
		{
			name:        "zero fraction treated as unset",
			fraction:    mkFrac(0),
			absolute:    mkInt(4096),
			codeDefault: 32768,
			window:      200_000,
			want:        4096,
		},
		{
			name:        "nil fraction nil absolute returns default",
			fraction:    nil,
			absolute:    nil,
			codeDefault: 32768,
			window:      200_000,
			want:        32768,
		},
		{
			name:        "zero absolute treated as unset",
			fraction:    nil,
			absolute:    mkInt(0),
			codeDefault: 16384,
			window:      200_000,
			want:        16384,
		},
		{
			name:        "negative fraction treated as unset (pathological)",
			fraction:    mkFrac(-0.5),
			absolute:    mkInt(9999),
			codeDefault: 32768,
			window:      200_000,
			want:        9999,
		},
		{
			name:        "tiny window with fraction rounds via int truncation",
			fraction:    mkFrac(0.01),
			absolute:    mkInt(99999),
			codeDefault: 32768,
			window:      100, // 100 × 0.01 × 4 = 4 bytes
			want:        4,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveByteBudget(c.fraction, c.absolute, c.codeDefault, c.window)
			if got != c.want {
				t.Errorf("ResolveByteBudget(fraction=%v, absolute=%v, default=%d, window=%d) = %d, want %d",
					ptrStr(c.fraction), ptrStrInt(c.absolute), c.codeDefault, c.window, got, c.want)
			}
		})
	}
}

// TestResolveTokenBudget_PrecedenceMatrix pins the same three-way
// precedence as ResolveByteBudget but for token-form budgets (no
// BytesPerToken multiplier). Used for output-side caps where the
// fraction expresses "let the cap scale with context_window."
func TestResolveTokenBudget_PrecedenceMatrix(t *testing.T) {
	mkFrac := func(f float64) *float64 { return &f }
	cases := []struct {
		name        string
		fraction    *float64
		absolute    int
		codeDefault int
		window      int
		want        int
	}{
		{"fraction+window wins", mkFrac(0.25), 99999, 0, 64000, 16000},
		{"fraction zero falls to absolute", mkFrac(0), 32000, 0, 64000, 32000},
		{"nil fraction uses absolute", nil, 8192, 0, 128000, 8192},
		{"zero absolute returns default 0 (no cap)", nil, 0, 0, 128000, 0},
		{"negative fraction → absolute", mkFrac(-0.1), 4096, 16384, 128000, 4096},
		{"absolute zero + default nonzero", nil, 0, 16384, 128000, 16384},
		{"fraction without window → absolute", mkFrac(0.5), 8000, 0, 0, 8000},
		{"fraction wins over default but absolute=0", mkFrac(0.1), 0, 9999, 100000, 10000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveTokenBudget(c.fraction, c.absolute, c.codeDefault, c.window)
			if got != c.want {
				t.Errorf("ResolveTokenBudget(fraction=%v, absolute=%d, default=%d, window=%d) = %d, want %d",
					ptrStr(c.fraction), c.absolute, c.codeDefault, c.window, got, c.want)
			}
		})
	}
}

// TestResolveDurationSeconds covers the timeout resolver: positive
// configured value wins, anything else (zero / negative) falls
// through to the code default. Negative inputs are not surfaced as
// errors — they're treated as "absent" because yaml type juggling
// can produce them and a startup hard-fail would be a regression
// from the previous hard-coded constant.
func TestResolveDurationSeconds(t *testing.T) {
	cases := []struct {
		name        string
		seconds     int
		codeDefault int
		want        time.Duration
	}{
		{"positive wins", 600, 120, 600 * time.Second},
		{"zero falls back", 0, 120, 120 * time.Second},
		{"negative falls back", -5, 120, 120 * time.Second},
		{"large value flows through", 3600, 120, 3600 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveDurationSeconds(c.seconds, c.codeDefault)
			if got != c.want {
				t.Errorf("ResolveDurationSeconds(%d, %d) = %v, want %v",
					c.seconds, c.codeDefault, got, c.want)
			}
		})
	}
}

// TestResolveRetryAttempts: positive wins; zero / negative fall back
// to the code default. A configured 1 is legal (single attempt, no
// retry), only zero is treated as absent.
func TestResolveRetryAttempts(t *testing.T) {
	cases := []struct {
		name        string
		attempts    int
		codeDefault int
		want        int
	}{
		{"positive wins", 10, 6, 10},
		{"single attempt is legal", 1, 6, 1},
		{"zero falls back", 0, 6, 6},
		{"negative falls back", -1, 6, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveRetryAttempts(c.attempts, c.codeDefault)
			if got != c.want {
				t.Errorf("ResolveRetryAttempts(%d, %d) = %d, want %d",
					c.attempts, c.codeDefault, got, c.want)
			}
		})
	}
}

// TestBytesPerToken_Default ensures the conservative 4 B/tok default
// doesn't silently shift. Operators calibrating against this value
// in their own tooling would be surprised by a change.
func TestBytesPerToken_Default(t *testing.T) {
	if BytesPerToken != 4 {
		t.Errorf("BytesPerToken default changed to %d — check callers that hard-coded the old value", BytesPerToken)
	}
}

func ptrStr(f *float64) string {
	if f == nil {
		return "nil"
	}
	return "(" + formatFloat(*f) + ")"
}
func ptrStrInt(i *int) string {
	if i == nil {
		return "nil"
	}
	return "(" + formatInt(*i) + ")"
}
func formatFloat(f float64) string {
	// stdlib strconv pulled via fmt; keep test file import-light.
	if f == 0 {
		return "0"
	}
	return "non-zero"
}
func formatInt(i int) string {
	if i == 0 {
		return "0"
	}
	return "non-zero"
}
