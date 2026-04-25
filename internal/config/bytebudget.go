package config

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BytesPerToken re-exports the canonical estimate that lives in
// internal/types so every resolver + the BaseAgent pressure watchdog
// agree on the ratio. Kept as a package-local constant alias so
// existing callers `config.BytesPerToken` keep compiling without
// reaching into types/. See types.BytesPerToken for the rationale.
const BytesPerToken = types.BytesPerToken

// ResolveByteBudget returns the effective byte budget for a fraction /
// absolute / default triple:
//
//   - fraction × contextWindow × BytesPerToken — preferred when both
//     fraction is set (> 0) and contextWindow is positive
//   - absolute — when fraction is unusable (nil, zero, or context
//     window unknown) but the absolute yaml field was set
//   - codeDefault — when neither fraction nor absolute is set
//
// This is a pure helper; the caller owns logging, clamping, and
// multi-agent per-window fan-out. A nil fraction pointer counts as
// "unset" (not "zero"); a zero float64 value does too (a fraction
// of 0 would yield 0 bytes and is a pathological configuration —
// we treat it as "not configured" on purpose).
func ResolveByteBudget(fraction *float64, absolute *int, codeDefault int, contextWindow int) int {
	if fraction != nil && *fraction > 0 && contextWindow > 0 {
		return int(float64(contextWindow) * (*fraction) * float64(BytesPerToken))
	}
	if absolute != nil && *absolute > 0 {
		return *absolute
	}
	return codeDefault
}

// ResolveTokenBudget is the token-budget twin of ResolveByteBudget.
// Same fraction-then-absolute-then-default precedence; differs only
// in that the multiplier is unitless (we are budgeting tokens, not
// bytes — no BytesPerToken). Used for output-side caps such as
// max_output_tokens, where the fraction form expresses "let the cap
// scale with the model's declared context window."
func ResolveTokenBudget(fraction *float64, absolute, codeDefault, contextWindow int) int {
	if fraction != nil && *fraction > 0 && contextWindow > 0 {
		v := int(float64(contextWindow) * (*fraction))
		if v > 0 {
			return v
		}
	}
	if absolute > 0 {
		return absolute
	}
	return codeDefault
}

// ResolveDurationSeconds returns the effective duration for a
// seconds-form yaml field. Zero means "absent" — the code default
// applies. Negative inputs are treated as absent (a defensive guard
// rather than a hard error, since yaml type juggling can produce
// surprising values).
func ResolveDurationSeconds(seconds, codeDefaultSeconds int) time.Duration {
	if seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(codeDefaultSeconds) * time.Second
}

// ResolveRetryAttempts returns the effective HTTP attempt count.
// Same zero-means-absent rule. A configured 1 is legal (no retry —
// single attempt only); only zero falls back to the code default.
func ResolveRetryAttempts(attempts, codeDefault int) int {
	if attempts > 0 {
		return attempts
	}
	return codeDefault
}
