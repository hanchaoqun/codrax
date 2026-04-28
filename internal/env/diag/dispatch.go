// Package diag implements Layer 1 of env_recommend — turning a
// raw failure (stderr / runner / EnvFacts) into a structured
// Diagnosis. Each detector is independent; Diagnose dispatches in
// fixed priority order and returns the first match.
//
// Returning DiagUnknown is the LLM-fallback sentinel — the caller
// (Recommender) routes it to the LLM diagnose path when LLMEnabled
// is true.
package diag

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// Detector is one diagnosis check. Order matters in the dispatch
// loop: each detector gets the input and decides whether it
// applies. First detector returning a non-Unknown Diagnosis wins.
type Detector func(stderr, runner string, env *types.EnvFacts) (types.Diagnosis, bool)

// detectors is the fixed-order check chain. Most specific kinds
// (system-lib / git-state) run before generic ones (runner-missing
// / deps-missing).
var detectors = []Detector{
	detectSystemLibMissing,
	detectGitState,
	detectRunnerMissing,
	detectDepsMissing,
	detectToolchainMissing,
	detectConfigMissing,
}

// Diagnose runs the detector chain and returns the first match,
// or DiagUnknown when no detector applies. Never errors — every
// failure path returns DiagUnknown so the caller can route to the
// LLM fallback.
func Diagnose(stderr, runner string, env *types.EnvFacts) types.Diagnosis {
	for _, d := range detectors {
		if diag, ok := d(stderr, runner, env); ok {
			return diag
		}
	}
	return types.Diagnosis{
		Kind:       types.DiagUnknown,
		Runner:     runner,
		Excerpt:    excerpt(stderr, 400),
		Source:     "no_detector_matched",
		Confidence: 0.0,
	}
}

// excerpt returns the first n bytes of s with TrimSpace.
func excerpt(s string, n int) string {
	if len(s) <= n {
		return trimSpaceASCII(s)
	}
	return trimSpaceASCII(s[:n]) + " …[truncated]"
}

// trimSpaceASCII is a minimal stand-in for strings.TrimSpace —
// used here to keep the diag package import surface tiny.
func trimSpaceASCII(s string) string {
	for len(s) > 0 && isASCIISpace(s[0]) {
		s = s[1:]
	}
	for len(s) > 0 && isASCIISpace(s[len(s)-1]) {
		s = s[:len(s)-1]
	}
	return s
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
