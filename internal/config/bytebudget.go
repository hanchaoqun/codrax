package config

import "github.com/hanchaoqun/codrax/internal/types"

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
