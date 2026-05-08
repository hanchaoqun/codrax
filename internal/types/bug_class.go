package types

// BugClass categorises a runtime failure pattern detected from an
// attached log / trace / future MCP-piped diagnostic. Sister concept
// to LogSignal:
//
//   - LogSignal answers WHAT-CLASS-OF-FAILURE — operational tier
//     ("panic / crash / oom / timeout / performance / db / network /
//     validation / logic / permission")
//   - BugClass answers WHAT-SPECIFIC-BUG — diagnostic tier
//     ("race / deadlock / nil_deref / bounds / type_assertion /
//     stack_overflow / div_by_zero / resource_exhaustion / ...")
//
// Both can fire on the same log: a Go race condition log carries
// SignalPanic + BugClassRace (the former says "the program crashed",
// the latter says "the crash is a data race"). Downstream consumers
// (skill prompts, finalize answer renderer) read the typed BugClass
// to use canonical terminology rather than re-deriving from raw
// panic text — eliminates the LLM's room to invent alternative
// causes from unrelated repo code.
//
// Internal enum values are NEVER rendered into LLM-facing prompts —
// the HumanZh / HumanEn fields on the per-pattern registry entry
// carry the canonical user-facing terms (which LLMs already
// understand from training). See bug_class_human.go for the
// per-class human label maps.
type BugClass string

const (
	// BugClassRace — data race / race condition / concurrent
	// modification across threads or goroutines.
	BugClassRace BugClass = "race"

	// BugClassDeadlock — circular wait / lock-order inversion / all
	// threads blocked.
	BugClassDeadlock BugClass = "deadlock"

	// BugClassNilDeref — null pointer / nil interface / forced
	// unwrap of a None / NoneType attribute access.
	BugClassNilDeref BugClass = "nil_deref"

	// BugClassBounds — array / slice / list / string index out of
	// range / bounds.
	BugClassBounds BugClass = "bounds"

	// BugClassTypeAssertion — interface conversion failure / class
	// cast / trait downcast failure.
	BugClassTypeAssertion BugClass = "type_assertion"

	// BugClassStackOverflow — uncontrolled recursion / goroutine
	// stack exceeded / RecursionError.
	BugClassStackOverflow BugClass = "stack_overflow"

	// BugClassDivByZero — integer / floating-point division by
	// zero (modulo included).
	BugClassDivByZero BugClass = "div_by_zero"

	// BugClassResourceExhaustion — file descriptor / heap / disk /
	// connection pool / quota exhausted.
	BugClassResourceExhaustion BugClass = "resource_exhaustion"

	// BugClassUnhandledAsync — uncaught promise rejection / coroutine
	// never awaited / orphaned task.
	BugClassUnhandledAsync BugClass = "unhandled_async"

	// BugClassSerialization — protobuf / JSON / msgpack /
	// flatbuffer parse / decode failure.
	BugClassSerialization BugClass = "serialization"

	// BugClassIntegrity — checksum / hash / signature / structural
	// validation mismatch on stored or transmitted data.
	BugClassIntegrity BugClass = "integrity"

	// BugClassAuth — authentication / authorisation / token /
	// credential failure (finer than SignalPermission).
	BugClassAuth BugClass = "auth"

	// BugClassTLSCert — TLS handshake / certificate verification /
	// chain-of-trust failure.
	BugClassTLSCert BugClass = "tls_cert"

	// BugClassUseAfterFree — memory accessed after deallocation.
	// Common in C / C++ / unsafe Rust; surfaced by ASan, MSan, glibc
	// abort messages, malloc-debug.
	BugClassUseAfterFree BugClass = "use_after_free"

	// BugClassBufferOverflow — out-of-bounds memory write or read
	// (heap / stack / global). Distinct from BugClassBounds (which
	// is the safe-language IndexError sibling); this class names the
	// memory-unsafe variant detected by ASan / MSan / system aborts.
	BugClassBufferOverflow BugClass = "buffer_overflow"

	// BugClassUncaughtException — generic unhandled exception that
	// did not fall into any other class. Catch-all for languages
	// whose runtime emits "Unhandled exception" / "panic: <generic>"
	// without finer signature.
	BugClassUncaughtException BugClass = "uncaught_exception"

	// BugClassFileSystem — file/dir not found / permission denied /
	// disk full / read-only filesystem. Finer than SignalPermission
	// (which is HTTP-style auth) and SignalDB (storage backend).
	BugClassFileSystem BugClass = "filesystem"

	// BugClassEncoding — character-encoding decode / transcode
	// failure (UTF-8 invalid sequence, charset incompatible) —
	// distinct from BugClassSerialization (data-format).
	BugClassEncoding BugClass = "encoding"

	// BugClassConfig — invalid / missing configuration value at
	// runtime (env var not set, yaml parse failure on user config,
	// missing required field). Distinct from Validation (input
	// validation at request boundary).
	BugClassConfig BugClass = "config"
)

// IsValidBugClass reports whether c is one of the registered classes.
// Used by JSON unmarshal validators / structural-coverage tests.
func IsValidBugClass(c BugClass) bool {
	switch c {
	case BugClassRace, BugClassDeadlock, BugClassNilDeref, BugClassBounds,
		BugClassTypeAssertion, BugClassStackOverflow, BugClassDivByZero,
		BugClassResourceExhaustion, BugClassUnhandledAsync, BugClassSerialization,
		BugClassIntegrity, BugClassAuth, BugClassTLSCert,
		BugClassUseAfterFree, BugClassBufferOverflow, BugClassUncaughtException,
		BugClassFileSystem, BugClassEncoding, BugClassConfig:
		return true
	}
	return false
}

// AllBugClasses returns every declared BugClass in stable enum order.
// Mirrors AllViolationKinds / AllRepairKinds.
func AllBugClasses() []BugClass {
	return []BugClass{
		BugClassRace, BugClassDeadlock, BugClassNilDeref, BugClassBounds,
		BugClassTypeAssertion, BugClassStackOverflow, BugClassDivByZero,
		BugClassResourceExhaustion, BugClassUnhandledAsync, BugClassSerialization,
		BugClassIntegrity, BugClassAuth, BugClassTLSCert,
		BugClassUseAfterFree, BugClassBufferOverflow, BugClassUncaughtException,
		BugClassFileSystem, BugClassEncoding, BugClassConfig,
	}
}

// DetectedBugClass is one bug-class detection event from a registry
// pattern match. Carries the canonical human-readable label
// (LLM-friendly, no internal enum strings) plus the verbatim matched
// signature substring (for evidence pinning when the LLM-facing
// prompt mentions the detection).
//
// SourceLang is the language hint that fired the pattern ("go" /
// "java" / "tsan" / "rust" / "python" / "generic") — used for
// telemetry / debugging only, NEVER rendered into LLM prompts.
type DetectedBugClass struct {
	Class   BugClass `json:"class"`
	HumanZh string   `json:"human_zh"`
	HumanEn string   `json:"human_en"`

	// MatchedSignature is the verbatim substring from the input log
	// that fired the registry pattern. Truncated to 200 chars to
	// keep telemetry / prompt budget bounded. Surface this rather
	// than the regex pattern (regex is internal implementation).
	MatchedSignature string `json:"matched_signature,omitempty"`

	SourceLang string `json:"source_lang,omitempty"`
}

// HumanLabel returns the bilingual canonical label
// "数据竞争 / data race" — kept compact so prompts can chain
// multiple detections without ballooning the budget. Format is
// stable (zh + " / " + en) so the LLM always sees the same shape.
func (d DetectedBugClass) HumanLabel() string {
	switch {
	case d.HumanZh != "" && d.HumanEn != "":
		return d.HumanZh + " / " + d.HumanEn
	case d.HumanZh != "":
		return d.HumanZh
	case d.HumanEn != "":
		return d.HumanEn
	}
	return string(d.Class)
}
