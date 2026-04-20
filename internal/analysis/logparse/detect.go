package logparse

import (
	"strings"
	"sync/atomic"
)

// enabled gates Detect globally. Defaults to on so zero-config installs
// (no codrax.yaml / no flag) still process attached logs. cmd/root.go
// calls SetEnabled(false) when codrax.yaml sets logparse_enabled=false.
var enabled atomic.Bool

func init() { enabled.Store(true) }

// SetEnabled toggles the global log-triage feature gate. When false,
// Detect short-circuits and returns an empty bundle regardless of
// input, so the analyzer's integration degrades cleanly to the
// pre-feature behaviour.
func SetEnabled(v bool) { enabled.Store(v) }

// Enabled reports the current gate state.
func Enabled() bool { return enabled.Load() }

// Detect runs the known parsers in priority order and returns the
// first non-empty bundle, or a zero-value LogBundle when no parser
// matched. Priority is ordered by regex specificity so that a log
// which contains BOTH a Java stacktrace and an embedded ASAN report
// (rare in practice but possible with cross-language runtimes) is
// classified by its most-specific anchor.
//
// Priority rationale:
//
//  1. ASAN/UBSAN — `==NNN==ERROR:` header is essentially unambiguous
//     and the frame format (`#0 0x... in func file.cpp:42:5`) does
//     not collide with any other language's stack shape.
//  2. gdb stack — `#0  0x... in fn() at file.cpp:42` is similarly
//     unique to C/C++. Checked after ASAN/UBSAN so a sanitizer
//     diagnostic that includes a gdb-style frame is not mis-tagged.
//  3. Java — `Exception in thread` / `    at pkg.Cls.m(File.java:NN)`
//     is diagnostic. Earlier than Python so a hybrid trace from a
//     Jython / Graal runtime routes to the Java parser first (the
//     `.java:NN` frame is what we need).
//  4. Go — `goroutine N [` + the tab-indented `file:NN +0xHH` form
//     is Go-specific.
//  5. Python — `Traceback (most recent call last):` header.
//  6. Node — `    at ...(file.js:NN:CC)` form.
//
// Empty input returns a zero-value bundle with no allocations.
func Detect(raw string) LogBundle {
	if !Enabled() {
		return LogBundle{}
	}
	if strings.TrimSpace(raw) == "" {
		return LogBundle{}
	}
	for _, fn := range orderedParsers() {
		bundle := fn(raw)
		if bundle.Lang == "" || len(bundle.Frames) == 0 {
			continue
		}
		bundle.ErrorLiterals = dedupeLiterals(bundle.ErrorLiterals)
		bundle.HasStack = hasStackWithLine(bundle.Frames)
		return bundle
	}
	return LogBundle{}
}

// Parsers exposes the priority-ordered parser list as a map keyed by
// language tag, for tests that want to exercise a specific parser
// without going through Detect's priority filter.
func Parsers() map[string]ParseFn {
	return map[string]ParseFn{
		"cpp_asan": parseCppASAN,
		"cpp_gdb":  parseCppGDB,
		"java":     parseJava,
		"go":       parseGo,
		"python":   parsePython,
		"node":     parseNode,
	}
}

// orderedParsers returns the parser list in Detect priority order.
// Kept as a function (not a package-level slice) so adding or
// reordering entries stays a single edit.
func orderedParsers() []ParseFn {
	return []ParseFn{
		parseCppASAN,
		parseCppGDB,
		parseJava,
		parseGo,
		parsePython,
		parseNode,
	}
}

// dedupeLiterals returns a fresh slice with duplicate strings removed
// and empty entries dropped, preserving first-seen order. Used by
// Detect to normalise the ErrorLiterals output every parser produces
// independently.
func dedupeLiterals(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hasStackWithLine reports whether frames contains at least one entry
// with both File and Line > 0 — the minimum needed for Intent =
// RootCause to be justified AND for RequiredFiles seeding to land a
// real file:line anchor.
func hasStackWithLine(frames []Frame) bool {
	for _, f := range frames {
		if f.File != "" && f.Line > 0 {
			return true
		}
	}
	return false
}
