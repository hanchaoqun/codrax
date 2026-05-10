package types

// LogBundle is the validated output of the log_triage pre-stage.
// It carries the LLM's structured view of the user-attached runtime
// log plus a system-derived layer that every downstream consumer
// reads from. Nil on MutableState means "no log attached, stage
// was skipped, or stage degraded" — every consumer MUST nil-check.
//
// The struct is split into four layers by origin and consumer:
//
//	Layer 1 — Meta:     LLM-emitted. Log-as-a-whole classification.
//	Layer 2 — Errors:   LLM-emitted. Structured error tree.
//	Layer 3 — Residue:  LLM-emitted. Chunks the LLM could not structure.
//	Layer 4 — Derived:  System-emitted. Read-only to the LLM; written
//	                    exactly once by logtriage.ValidateBundle.
//
// The boundary is load-bearing: LLM MUST NOT fill Layer 4 (the emit
// tool's JSON schema rejects those fields), and the validator MUST
// NOT mutate Layer 1-3 post-emission. This keeps the responsibility
// split grep-able and lets future consumers know exactly where any
// field came from.
type LogBundle struct {
	// ── Layer 1 — Meta ─────────────────────────────
	Meta LogMeta `json:"meta"`

	// ── Layer 2 — Errors ───────────────────────────
	// Top-level slice carries PARALLEL snapshots: multiple goroutines
	// in a Go panic dump, or two independent exception stacks captured
	// by a single log collector. Each LogError internally carries a
	// Cause pointer for chronological causal chains (Java Caused-by,
	// Rust #[source], Python __cause__). The two axes are distinct:
	// parallel (Errors slice) vs chronological (Cause recursion).
	Errors []LogError `json:"errors,omitempty"`

	// ── Layer 3 — Residue ──────────────────────────
	Residue LogResidue `json:"residue,omitempty"`

	// ── Layer 4 — System-derived ───────────────────
	// ResolvedFiles is the deterministic, os.Stat-verified list of
	// repo-relative file paths extracted from Frames whose File, Line,
	// and Confidence passed validation. Unique, ordered by Confidence
	// descending then path ascending. Fed into EvidencePlan.RequiredFiles
	// by the analyzer's post-processing read of bus.LogTriage.
	ResolvedFiles []string `json:"resolved_files,omitempty"`

	// Entities is the deterministic, deduplicated keyword list derived
	// from Frame.Func last-segments, Frame.Pkg hints, Error.Type tokens,
	// and Meta.Signals names. Capped at 32 tokens so deep stacks do not
	// drown the analyzer's AnalyzerHints.Entities list.
	Entities []string `json:"entities,omitempty"`

	// IntentHint is the system's determination of whether the log
	// represents a debugging / root-cause question. Set to
	// IntentRootCause when at least one Frame has File+Line>0 and
	// Meta.Signals intersects {panic, crash, oom}. Otherwise zero.
	// analyzer.reconcileIntent reads this to override an LLM intent
	// that failed to pick up the log signal.
	IntentHint Intent `json:"intent_hint,omitempty"`

	// Coverage reports the fraction of the raw log that the LLM
	// successfully structured vs. routed to UnknownChunks, computed
	// as 1 - sum(len(chunk)) / len(rawLog). Drives the two-step
	// escalation decision: a coverage below the configured threshold
	// triggers segmentation + per-segment extraction. Range [0.0, 1.0].
	Coverage float64 `json:"coverage"`
}

// LogMeta carries the log-as-a-whole classification the LLM emitted.
type LogMeta struct {
	// Lang is the LLM's best guess of the runtime language. Values
	// outside the canonical enum collapse to "unknown".
	Lang string `json:"lang"`

	// Signals is the LLM's categorical classification of what went
	// wrong. Multi-select. Values outside the canonical LogSignal
	// enum are rejected by the emit tool's schema. Duplicates are
	// deduplicated by ValidateBundle.
	Signals []LogSignal `json:"signals,omitempty"`

	// Summary is the LLM's one-line synopsis, capped at 200
	// characters. Rendered as a "[log_triage]" log line on emit and
	// available to future answer-rendering code that wants a one-line
	// headline. Optional.
	Summary string `json:"summary,omitempty"`

	// BugClasses is the deterministic cross-language bug-pattern
	// detection result, populated by the log_triage entry hook BEFORE
	// the LLM dispatch (so the LLM-facing skill prompt can render
	// canonical terminology hints like "数据竞争 / data race" /
	// "死锁 / deadlock" / "空指针 / nil deref"). Each entry carries
	// the matched signature substring for evidence pinning. Empty
	// when no registered pattern fired.
	//
	// Distinct from Signals (which is the LLM-emitted operational-
	// tier classification) — BugClasses is the diagnostic-tier
	// classification the LLM does NOT emit; the system stamps it
	// from a regex registry that knows cross-language signatures
	// (Go runtime / TSan / Java exceptions / Rust panics / Python
	// errors / etc.).
	BugClasses []DetectedBugClass `json:"bug_classes,omitempty"`
}

// LogError is one error snapshot with optional causal chain. The tree
// is naturally recursive: Exception A wrapping Exception B wrapping
// Exception C maps to a three-deep Cause chain. Parallel errors
// (multiple goroutines, sibling exceptions in a sidecar dump) live
// as peers in LogBundle.Errors, not via Cause.
type LogError struct {
	// Type is the exception / panic class name, e.g.
	// "java.lang.NullPointerException", "runtime error: index out of range",
	// "SIGSEGV". Carried as a first-class search entity.
	Type string `json:"type"`

	// Message is the human-readable error text. Ornamental, not used
	// for entity extraction (too variable). Capped at 500 chars by
	// the emit schema.
	Message string `json:"message,omitempty"`

	// Frames is the stack for this specific error. Outermost
	// (innermost failure site) first when the source format preserves
	// ordering. An empty slice is valid — e.g. "header-only" errors
	// where the log reported a problem but no stack.
	Frames []LogFrame `json:"frames,omitempty"`

	// Cause, when non-nil, is the wrapped / underlying error. The
	// emit schema allows arbitrary depth but ValidateBundle enforces
	// a configurable MaxCauseDepth (default 5) to defend against
	// degenerate emissions.
	Cause *LogError `json:"cause,omitempty"`
}

// LogFrame is one stack-frame. The frame is "complete" when File,
// Line>0, and Confidence>=MinResolvedConfidence are all present and
// the path resolves inside the repo — these are the frames that flow
// into ResolvedFiles. "Partial" frames (File=="" or Line==0 or low
// confidence) stay in the bundle for auditability and contribute
// Func/Pkg tokens to Entities, but are kept out of ResolvedFiles.
type LogFrame struct {
	// Lang is the per-frame language (may differ from LogMeta.Lang
	// in polyglot stacks — e.g. JNI code in a Java trace).
	Lang string `json:"lang,omitempty"`

	// File is the source file path. The LLM emits the raw form as it
	// appears in the log (may be absolute build path or basename or
	// repo-relative); ValidateBundle normalises via StripBuildPathPrefix
	// and verifies via os.Stat. Empty after validation means "path
	// unresolvable or hallucinated"; the frame is kept but not added
	// to ResolvedFiles.
	File string `json:"file,omitempty"`

	// Line is the 1-indexed line number. 0 is a legal value meaning
	// "LLM could not determine"; the frame is kept but not promoted
	// to ResolvedFiles.
	Line int `json:"line,omitempty"`

	// Func is the function / method / symbol name. Shape is language-
	// specific (see the log-triage-skill prompt for per-language
	// formatting expectations).
	Func string `json:"func,omitempty"`

	// Pkg is the module / package / namespace prefix when the LLM
	// could split it. Empty when no reliable split exists.
	Pkg string `json:"pkg,omitempty"`

	// Raw is the original log text for this frame, capped at 300
	// chars. Required on every frame — it is the audit trail when
	// File/Line resolution later drops the frame, and the source of
	// truth when the LLM's structured extraction disagrees with what
	// a human reader sees.
	Raw string `json:"raw"`

	// Confidence is the LLM's self-reported certainty that this
	// frame is real and correctly extracted, in [0.0, 1.0].
	// ValidateBundle gates ResolvedFiles promotion on
	// Confidence >= MinResolvedConfidence (default 0.6).
	Confidence float64 `json:"confidence"`
}

// LogResidue collects chunks of the raw log the LLM could not
// structure into Errors. Kept on-bundle for zero information loss —
// future consumers (the explorer's grep over UnknownChunks) can
// recover keywords from sections that looked like noise to the
// extractor. The emit tool's schema caps the slice at 8 entries of
// 500 chars each (~4 KB total) so a runaway LLM cannot inflate the
// bundle by punting everything to Residue.
type LogResidue struct {
	UnknownChunks []string `json:"unknown_chunks,omitempty"`
}

// LogSignal is the canonical enum of what-went-wrong categories the
// LLM may assign in LogMeta.Signals. The set is intentionally coarse:
// signals drive pipeline routing (which evidence to hunt first), not
// diagnosis. Fine-grained categorisation (db_timeout vs db_deadlock)
// is the evidence layer's job, not the triage stage's.
type LogSignal string

const (
	SignalPanic      LogSignal = "panic"      // runtime panic, abort, fatal
	SignalCrash      LogSignal = "crash"      // SIGSEGV, assertion, hardware fault
	SignalOOM        LogSignal = "oom"        // out-of-memory (runtime OOMKilled, heap exhaustion)
	SignalTimeout    LogSignal = "timeout"    // deadline exceeded, context canceled, hang
	SignalPermission LogSignal = "permission" // access denied, forbidden, unauthorized
	SignalDB         LogSignal = "db"         // database / storage error (any subtype)
	SignalNetwork    LogSignal = "network"    // connection refused / reset / DNS / TLS
	SignalValidation LogSignal = "validation" // bad input, parse error, schema failure
	SignalLogic      LogSignal = "logic"      // invariant violation, assertion, bug
	// SignalPerformance covers "the system was operational but
	// noticeably slow / blocked / lost frames" — the diagnostic
	// shape that does NOT match any error class above. Pre-2026-05-02
	// such evidence (e.g. log lines like "slow API call took 5s",
	// "Choreographer: dropped 5 frames", "GC pause 800ms",
	// "blocked on lock for 2s") was forced into either timeout
	// (prefix-misuse) or other (information loss). Examples that
	// belong here:
	//   - slow request / high latency / SLA breach
	//   - frame drop / jank from a regular log (PerfBundle handles
	//     HiTrace / atrace traces; this signal handles ordinary
	//     application logs that happen to mention frame loss)
	//   - long-blocked operation / lock contention
	//   - GC pause / scheduler stall
	// Distinct from `timeout` because no operation was cancelled —
	// it just took longer than expected. Distinct from PerfBundle's
	// `jank` / `cold-start-slow` because those are trace-level
	// (ts/duration) signals from a perf trace tool; this is a
	// log-level mention of a perf symptom.
	SignalPerformance LogSignal = "performance"
	SignalOther       LogSignal = "other" // nothing above fits; fallback
)

// AllLogSignals returns every canonical signal in declaration order.
// Used by the emit tool's JSON schema generator and by tests that
// verify the enum stays in sync with its consumers.
func AllLogSignals() []LogSignal {
	return []LogSignal{
		SignalPanic, SignalCrash, SignalOOM, SignalTimeout,
		SignalPermission, SignalDB, SignalNetwork,
		SignalValidation, SignalLogic, SignalPerformance,
		SignalOther,
	}
}

// WalkLogErrors walks every top-level error and its Cause chain in
// render order. The top-level Errors slice represents parallel error
// snapshots; each Cause pointer represents chronological wrapping under
// that snapshot. Nil bundle / nil visit are no-ops.
func WalkLogErrors(bundle *LogBundle, visit func(*LogError)) {
	if bundle == nil {
		return
	}
	if visit == nil {
		return
	}
	var walk func(*LogError)
	walk = func(err *LogError) {
		if err == nil {
			return
		}
		visit(err)
		walk(err.Cause)
	}
	for i := range bundle.Errors {
		walk(&bundle.Errors[i])
	}
}

// WalkLogFrames walks every frame from every top-level error and nested
// Cause in render order. Callers that need priority ordering (for
// example file+line frames before symbol-only frames) should do their
// own two-pass filtering on top of this shared traversal.
func WalkLogFrames(bundle *LogBundle, visit func(LogFrame)) {
	if visit == nil {
		return
	}
	WalkLogErrors(bundle, func(err *LogError) {
		for _, frame := range err.Frames {
			visit(frame)
		}
	})
}

// LogBundleErrorTypes walks every top-level error and its Cause chain,
// returning the ordered, de-duplicated list of non-empty Type strings.
// Downstream consumers use this shared helper instead of re-walking the
// recursive tree ad hoc in each stage.
func LogBundleErrorTypes(bundle *LogBundle) []string {
	if bundle == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	WalkLogErrors(bundle, func(err *LogError) {
		if t := err.Type; t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	})
	return out
}

// IsValidLogSignal reports whether s is one of the canonical signals.
// Used by the validator to reject LLM-emitted values outside the enum
// (defence-in-depth; the schema's enum constraint catches it first).
func IsValidLogSignal(s LogSignal) bool {
	for _, v := range AllLogSignals() {
		if v == s {
			return true
		}
	}
	return false
}

// IsExternalSourceLog reports whether the bundle represents an
// external-source log — one that carries structured errors but whose
// stack frames did not resolve to any file in this repo. The
// canonical case is a customer-trace paste (another service's Go
// panic, a Java Caused-by chain from a microservice, a Python
// traceback from a library) dropped into a codrax REPL.
//
// Downstream emit tools (emit_evidence, emit_answer_symbol,
// emit_answer_document) use this flag to redirect their per-item
// "line > 0" / "file required" rejections toward the proper
// schema-legal escape (citation_ref=-1 for value shape,
// symbols_completeness=unknown for list_of_symbols / multi-topic
// explanation skeleton) rather than letting the LLM hammer the gates
// with manufactured anchors. Pairs with the front-loaded "External-
// source log" directive rendered at the top of the Log Triage
// Validated Extraction section — same predicate, both sides.
//
// Nil-safe: returns false on a nil receiver so tool-side checks do
// not need to nil-guard every call.
func (b *LogBundle) IsExternalSource() bool {
	if b == nil {
		return false
	}
	return len(b.ResolvedFiles) == 0 && len(b.Errors) > 0
}

// LogBundleCaps collects the validator-enforced caps in one place so
// they can be referenced from tests and schema generation without
// hard-coding magic numbers across packages. Kept as package-level
// var (not const) so callers that need to override in tests can
// shadow them — production code should never mutate.
var LogBundleCaps = struct {
	// MaxEntities caps Entities after dedup. 32 mirrors the session-19
	// mergeLogTriageEntities cap so post-session-20 behaviour is
	// backwards-compatible on the analyzer's entity floor.
	MaxEntities int

	// MaxResolvedFiles caps ResolvedFiles. 10 mirrors the session-19
	// mergeLogTriageFiles cap for EvidencePlan.RequiredFiles.
	MaxResolvedFiles int

	// MaxCauseDepth bounds LogError.Cause recursion. Defence against
	// LLM emitting a degenerate chain (or a well-intentioned LLM
	// over-unwrapping a deeply-wrapped error). The 6th level and
	// below are dropped with a [log_triage] warning.
	MaxCauseDepth int

	// MinResolvedConfidence is the per-frame confidence floor for
	// promotion into ResolvedFiles. Frames below the floor stay in
	// the bundle and contribute to Entities, but do not seed the
	// explorer's file list. 0.6 is "more likely right than not" —
	// a deliberate conservative gate because a hallucinated file
	// seed costs the explorer one wasted read budget.
	MinResolvedConfidence float64
}{
	MaxEntities:           32,
	MaxResolvedFiles:      10,
	MaxCauseDepth:         5,
	MinResolvedConfidence: 0.6,
}
