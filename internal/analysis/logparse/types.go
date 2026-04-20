// Package logparse parses runtime log excerpts (panics, exception
// traces, sanitizer diagnostics, tracebacks) that users paste into
// codrax alongside their question, so the analyzer can seed the
// pipeline with stack-frame file:line pairs, function names, and
// error literals harvested from the log.
//
// The package is intentionally minimal: a shared LogBundle contract
// plus one parser per language family. Each parser returns a bundle
// with Lang filled in and a non-empty Frames slice on success, or an
// empty bundle on no-match. Detect walks a priority list of parsers
// and returns the first non-empty bundle.
//
// MVP language coverage:
//
//   - Go panic / goroutine stack
//   - Java exception (java.lang.* stacktrace + Caused-by chain)
//   - C/C++ AddressSanitizer / UndefinedBehaviorSanitizer
//   - C/C++ gdb `#N 0x.. in func() at file:line` format
//   - Python traceback
//   - Node.js V8 stack
//
// Deliberately out of scope:
//
//   - C/C++ glibc backtrace (no file:line — frames carry only
//     addresses, so they give the explorer no RequiredFiles seed)
//   - Custom application log formats (no standard regex, no
//     repeatable output; Tier 3 plugin territory)
//
// The package has zero pipeline-side dependencies and one entry point
// (Detect) so consumers can treat it as a pure data transform.
package logparse

// Frame is one stack-frame harvested from a log. Not every language
// fills every field: some traces include file:line but no package,
// others (Go panics) give the function name with receiver. Zero
// values mean "not available".
type Frame struct {
	// Lang identifies the parser that produced the frame
	// ("go" / "java" / "cpp" / "python" / "node"). Carried on the
	// frame itself (and on the bundle) so downstream consumers can
	// route language-specific post-processing (Java basename
	// resolution vs C/C++ build-path prefix strip) off a single field.
	Lang string

	// File is the source file path as it appeared in the log. For
	// Java this is a basename (e.g. "UserService.java"); for
	// Go/Python/Node it is typically a repo-relative path; for
	// C/C++ it is usually an absolute build-machine path and needs
	// prefix stripping before it can match repo layout. The
	// resolver.go helpers handle these quirks — keep File raw here.
	File string

	// Line is the 1-indexed line number. 0 means "no line info"
	// (rare, but possible for some degraded traces).
	Line int

	// Func is the function / method name. Shape varies:
	//   - Go:     "pkg.Type.Method" / "pkg.funcName" / "main.main"
	//   - Java:   "com.foo.Bar.method"
	//   - C/C++:  mangled or demangled symbol, may include namespace
	//   - Python: bare function name (outer scope) or "method"
	//   - Node:   "Object.foo" / "Class.prototype.method"
	//
	// Consumers that just want to add Func as a search-term entity
	// should take the last dotted segment.
	Func string

	// Pkg is the package / module / namespace prefix when the parser
	// could reliably split it off. For Java "com.foo.Bar" → "com.foo";
	// for Go "internal/agent" from the file path; empty for C/C++
	// and Python.
	Pkg string
}

// LogBundle is the result of parsing one log excerpt. A bundle is
// "empty" (no stack detected) when Lang == "" AND len(Frames) == 0.
// Empty bundles are valid zero values — Detect returns one when no
// parser matched, and the analyzer treats that as "no log-triage
// signal; proceed with plain pipeline".
type LogBundle struct {
	// Lang is the language family of the parser that produced this
	// bundle. Same domain as Frame.Lang. Empty on no-match.
	Lang string

	// Frames is the ordered stack, outermost (innermost error site)
	// first when the source format preserves ordering. Always non-nil
	// when Lang != "", but may be short if the log was truncated.
	Frames []Frame

	// ErrorLiterals collects the error message / panic string /
	// exception type / sanitizer diagnostic header. These flow into
	// AnalyzerHints.Entities so the explorer's keyword_search picks
	// up the error message as a search term. Deduplicated by Detect
	// before the bundle ships; individual parsers may emit duplicates
	// and Detect cleans them up.
	ErrorLiterals []string

	// HasStack reports whether the bundle carries at least one frame
	// with both File and Line set. Consumers use this to decide
	// whether to override Intent → RootCause and to populate
	// EvidencePlan.RequiredFiles. A bundle can have ErrorLiterals
	// without HasStack (e.g. a single-line panic with no trace) and
	// vice-versa.
	HasStack bool
}

// ParseFn is the shared parser signature. Each parser scans the raw
// input, extracts frames it recognises, and returns either a populated
// bundle (Lang set, Frames non-empty) or an empty bundle on no-match.
// Parsers MUST NOT return partial bundles with Lang set but no frames
// — that form is reserved for the "no-match" signal and would fool
// Detect into returning early with zero content.
type ParseFn func(raw string) LogBundle
