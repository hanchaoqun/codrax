package types

import (
	"encoding/json"
	"strings"
	"sync"
)

// TypedDenialClass enumerates the typed-signal categories a TypedDenial
// can carry. Closed enum so consumers (tool registries, prompt
// sanitisers, answer validators) can switch-dispatch on the class
// rather than parsing string tokens.
type TypedDenialClass string

const (
	// TypedDenialExternalLogFrameUnresolved fires when log_triage's
	// frameFileCorroboratesFunc cleared a Frame.File because the real
	// file does not contain the named function. The path is from an
	// external runtime log; the repo cannot ground it.
	TypedDenialExternalLogFrameUnresolved TypedDenialClass = "external_log_frame_unresolved"

	// TypedDenialExternalPerfStallUnresolved is the perf_triage mirror.
	// Fires when stall.File / stall.Symbol cannot be corroborated
	// against the real repo (no symbol of that name in the named file).
	TypedDenialExternalPerfStallUnresolved TypedDenialClass = "external_perf_stall_unresolved"

	// TypedDenialOracleSymbolUnverified fires when oracle.SymbolExists
	// (single-graph) or mg.Oracle().SymbolExists (multi-graph fan-out)
	// returns false for an LLM-emitted identifier. The token names a
	// symbol the typed graph does not know about.
	TypedDenialOracleSymbolUnverified TypedDenialClass = "oracle_symbol_unverified"

	// TypedDenialDriftFrameRelocated fires when authority's drift
	// detector returns FileMoved / Unmappable for a frame: the symbol
	// existed at some point but the current code's location does not
	// match the frame's claim.
	TypedDenialDriftFrameRelocated TypedDenialClass = "drift_frame_relocated"

	// TypedDenialEvidenceSubjectUnverified fires when ground.GroundItem
	// could not corroborate an EvidenceItem's Subject / Object against
	// any repo-grounded source.
	TypedDenialEvidenceSubjectUnverified TypedDenialClass = "evidence_subject_unverified"

	// TypedDenialAttachedExtractedUnscoped fires when a user-attached
	// artifact (config dump / paste / future MCP response) names a
	// path / symbol that resolves outside the active scope.
	TypedDenialAttachedExtractedUnscoped TypedDenialClass = "attached_extracted_unscoped"
)

// IsValidTypedDenialClass reports whether c is one of the registered
// classes. Used by JSON unmarshal validators / structural tests.
func IsValidTypedDenialClass(c TypedDenialClass) bool {
	switch c {
	case TypedDenialExternalLogFrameUnresolved,
		TypedDenialExternalPerfStallUnresolved,
		TypedDenialOracleSymbolUnverified,
		TypedDenialDriftFrameRelocated,
		TypedDenialEvidenceSubjectUnverified,
		TypedDenialAttachedExtractedUnscoped:
		return true
	}
	return false
}

// TypedDenial is the architectural negative-knowledge unit: a verbatim
// token observed in the input stream that some typed gate
// (log_triage frame corroboration / perf_triage stall corroboration /
// oracle existence / drift detection / evidence grounding) marked as
// unverifiable.
//
// Downstream consumers — prompt sanitisers, tool registries, answer
// validators — MUST treat tokens here as hard prohibitions:
//
//   - Prompt rendering: replace verbatim mentions with <unverified-X>
//     placeholders so the LLM does not see the original token.
//   - Tool registries (read_file / grep / repo_map): reject calls
//     whose primary argument matches a denied token; the LLM gets
//     a typed error message naming the denial class so it learns
//     to redirect rather than retry the same path.
//   - Answer validators: any answer prose that names a denied token
//     without an explicit "unverified / external-source" caveat
//     fails ViolDeniedTokenUndeclared.
//
// The architecture rule (R3 second-axis): a typed gate that
// downgrades any structured field MUST simultaneously stamp the
// corresponding raw token here. This keeps the gate symmetric
// across every LLM-facing surface — prose, prompt, tool call,
// answer — so a precise signal stays precise after multi-step
// renderings.
type TypedDenial struct {
	Class  TypedDenialClass `json:"class"`
	Token  string           `json:"token"`            // verbatim string (path / symbol / identifier)
	Reason string           `json:"reason,omitempty"` // human-readable for telemetry / debug
	Lang   string           `json:"lang,omitempty"`   // optional language hint
}

// TypedDenialSet is the Run-level collection. ONE set exists per Run:
// the orchestrator allocates it at Run entry and every narrowing
// helper (ToolBusContext / SubAgentContext / BuildAgentContext /
// BusContext.ShallowClone) propagates the same *TypedDenialSet
// pointer, so a denial stamped by a tool mid-dispatch is immediately
// visible to every other consumer — the L1 tool gates of subsequent
// calls, the L2 prompt sanitiser, and the orchestrator's L3 answer
// validator. (Pre-2026-06-12 the tool projection copied the set BY
// VALUE, so tool-stamped denials died on a discarded per-call copy
// and never reached any enforcement surface.)
//
// Internally synchronized: parallel explore workers and parallel
// sub-agents share the pointer, so reads (gate checks, sanitise)
// take a read-lock and stamps take the write-lock. The denial slice
// is unexported — consumers that need to iterate use Snapshot(),
// which returns a private copy. JSON shape is preserved via the
// Marshal/Unmarshal methods below ({"denials": [...]}).
//
// The empty value is ready to use; methods are nil-safe so consumers
// can call them even when no set was wired (bare test fixtures) —
// reads behave as "no denials", and Add on a nil set is a no-op
// (the channel is disabled, not broken).
type TypedDenialSet struct {
	mu      sync.RWMutex
	denials []TypedDenial
}

// NewTypedDenialSet returns an empty, ready-to-share set. The
// orchestrator calls this once at Run entry; everything downstream
// receives the pointer.
func NewTypedDenialSet() *TypedDenialSet {
	return &TypedDenialSet{}
}

// Add stamps a new denial. Duplicate (Class, Token) pairs are
// silently dropped — the same token failing multiple gates of the
// same class is one logical denial. Append-only during a Run.
func (s *TypedDenialSet) Add(d TypedDenial) {
	if s == nil {
		return
	}
	d.Token = strings.TrimSpace(d.Token)
	if d.Token == "" || !IsValidTypedDenialClass(d.Class) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.denials {
		if existing.Class == d.Class && existing.Token == d.Token {
			return
		}
	}
	s.denials = append(s.denials, d)
}

// Len reports the number of stamped denials.
func (s *TypedDenialSet) Len() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.denials)
}

// Snapshot returns a copy of the stamped denials. Consumers that
// iterate (tool refusal helpers, the L3 answer validator) use this
// instead of reaching into the slice so iteration never races with
// a concurrent stamp.
func (s *TypedDenialSet) Snapshot() []TypedDenial {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.denials) == 0 {
		return nil
	}
	out := make([]TypedDenial, len(s.denials))
	copy(out, s.denials)
	return out
}

// typedDenialSetWire is the JSON shape — identical to the pre-sync
// struct layout so any persisted form stays readable.
type typedDenialSetWire struct {
	Denials []TypedDenial `json:"denials,omitempty"`
}

// MarshalJSON preserves the {"denials": [...]} wire shape across the
// unexported-slice refactor, taking the read-lock so a marshal racing
// a stamp cannot observe a torn slice.
func (s *TypedDenialSet) MarshalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(typedDenialSetWire{Denials: s.denials})
}

// UnmarshalJSON mirrors MarshalJSON.
func (s *TypedDenialSet) UnmarshalJSON(data []byte) error {
	var wire typedDenialSetWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.denials = wire.Denials
	return nil
}

// IsPathDenied reports whether path matches any denial whose class is
// path-shaped (external_log_frame_unresolved /
// external_perf_stall_unresolved / drift_frame_relocated /
// attached_extracted_unscoped). Tool registries (read_file / grep /
// repo_map) call this in their entry guards to translate the typed
// gate into a hard tool-call refusal.
//
// Match is exact equality OR suffix match (path ends with token) so
// "internal/agent/analyzer.go" denies a read_file call with the
// same path AND a repo-relative variant. Both '/' and '\' separators
// are accepted (Windows + POSIX cross-OS) — paths are normalised to
// '/' before comparison. Substring is intentionally NOT used — too
// noisy.
func (s *TypedDenialSet) IsPathDenied(path string) bool {
	_, ok := s.FirstPathDenial(path)
	return ok
}

// FirstPathDenial returns the first path-shaped denial matching path
// under the SAME rules as IsPathDenied — one implementation, one
// lock acquisition. Tool refusal sites use this to render the
// class-specific HumanRefusalReason; a gate that fired on a suffix
// match (absolute query vs repo-relative token, or vice versa) gets
// the same precise explanation as an exact match. ok=false means no
// path-shaped denial matches.
func (s *TypedDenialSet) FirstPathDenial(path string) (TypedDenial, bool) {
	if s == nil || path == "" {
		return TypedDenial{}, false
	}
	path = normalisePathSep(strings.TrimSpace(path))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.denials {
		if !d.classIsPathShaped() {
			continue
		}
		token := normalisePathSep(d.Token)
		if path == token {
			return d, true
		}
		// Suffix match for repo-relative ↔ absolute mismatches.
		if strings.HasSuffix(path, "/"+token) || strings.HasSuffix(token, "/"+path) {
			return d, true
		}
	}
	return TypedDenial{}, false
}

// normalisePathSep converts Windows '\' to POSIX '/' so the gate
// matches the same logical file regardless of the host OS or the
// LLM-emitted form (LLMs often emit POSIX paths even on Windows).
func normalisePathSep(p string) string {
	if !strings.ContainsRune(p, '\\') {
		return p
	}
	return strings.ReplaceAll(p, "\\", "/")
}

// IsSymbolDenied reports whether name matches any denial whose class
// is symbol-shaped (oracle_symbol_unverified /
// evidence_subject_unverified / external_perf_stall_unresolved when
// stall.Symbol was the failing token). Used by symbol oracles +
// hallucination validators that already consult an oracle but should
// also short-circuit on confirmed denials.
//
// Match is exact-string only (case-sensitive). Identifier semantics
// require literal equality; case-folding belongs in the oracle's
// flat-form bridge, not this contract layer.
func (s *TypedDenialSet) IsSymbolDenied(name string) bool {
	_, ok := s.FirstSymbolDenial(name)
	return ok
}

// FirstSymbolDenial mirrors FirstPathDenial for symbol-shaped classes
// — same exact-string match as IsSymbolDenied, returning the denial
// so refusal sites can render its class-specific reason.
func (s *TypedDenialSet) FirstSymbolDenial(name string) (TypedDenial, bool) {
	if s == nil || name == "" {
		return TypedDenial{}, false
	}
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.denials {
		if !d.classIsSymbolShaped() {
			continue
		}
		if name == d.Token {
			return d, true
		}
	}
	return TypedDenial{}, false
}

// Sanitise replaces every denied token in prose with a typed
// placeholder. LLM-facing prompt builders call this on raw fields
// (frame.Raw, stall raw text, original log lines) before injecting,
// so the LLM cannot extract verbatim paths/symbols from prose to
// bypass the typed gate.
//
// The placeholder format is "<unverified-{class-suffix}>" — generic
// LLM-facing terminology with no internal pipeline names. Class
// suffixes:
//   - external-source     (log frame / perf stall path)
//   - unknown-symbol      (oracle existence miss)
//   - moved-or-removed    (drift frame relocated)
//   - unverified          (evidence subject)
//   - out-of-scope        (attached extracted token)
//
// Cross-OS path handling: when a path-shaped denial's token uses
// POSIX separators ('/'), Sanitise also matches the Windows variant
// ('\') in prose — and vice versa — so an attached log emitted on
// one OS is sanitised correctly on another.
//
// Replacement is exact-string substitution; substring is intentional
// here (raw log lines may contain the token mid-line).
func (s *TypedDenialSet) Sanitise(prose string) string {
	if s == nil || prose == "" {
		return prose
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.denials) == 0 {
		return prose
	}
	out := prose
	for _, d := range s.denials {
		if d.Token == "" {
			continue
		}
		marker := "<unverified-" + sanitiseClassSuffix(d.Class) + ">"
		out = strings.ReplaceAll(out, d.Token, marker)
		// Cross-OS variant: if the denial token has separators,
		// also replace the alternate-separator form.
		if d.classIsPathShaped() {
			if alt := alternatePathSepForm(d.Token); alt != "" && alt != d.Token {
				out = strings.ReplaceAll(out, alt, marker)
			}
		}
	}
	return out
}

// alternatePathSepForm returns the path with separators flipped:
// '/' ↔ '\'. Returns "" when the input has no separator (no flip
// needed). Used by Sanitise so a denied POSIX path also redacts
// any Windows-form mention in prose, and vice versa.
func alternatePathSepForm(p string) string {
	hasSlash := strings.ContainsRune(p, '/')
	hasBackslash := strings.ContainsRune(p, '\\')
	if hasSlash && !hasBackslash {
		return strings.ReplaceAll(p, "/", "\\")
	}
	if hasBackslash && !hasSlash {
		return strings.ReplaceAll(p, "\\", "/")
	}
	return ""
}

// PathTokens returns the deduplicated list of path-shaped tokens.
// Used by the LLM-facing prompt sanitiser to render a "do NOT read
// the following paths" hint when the prompt cannot be sanitised
// in-place (e.g. when paths appear in a tool result already
// emitted to the LLM).
func (s *TypedDenialSet) PathTokens() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var out []string
	for _, d := range s.denials {
		if !d.classIsPathShaped() {
			continue
		}
		if seen[d.Token] {
			continue
		}
		seen[d.Token] = true
		out = append(out, d.Token)
	}
	return out
}

// SymbolTokens mirrors PathTokens for symbol-shaped classes.
func (s *TypedDenialSet) SymbolTokens() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var out []string
	for _, d := range s.denials {
		if !d.classIsSymbolShaped() {
			continue
		}
		if seen[d.Token] {
			continue
		}
		seen[d.Token] = true
		out = append(out, d.Token)
	}
	return out
}

// classIsPathShaped reports whether the denial token is a file path
// (consumed by IsPathDenied / PathTokens / Sanitise's path-redaction
// branch).
func (d TypedDenial) classIsPathShaped() bool {
	switch d.Class {
	case TypedDenialExternalLogFrameUnresolved,
		TypedDenialExternalPerfStallUnresolved,
		TypedDenialDriftFrameRelocated,
		TypedDenialAttachedExtractedUnscoped:
		return true
	}
	return false
}

// classIsSymbolShaped reports whether the denial token is a symbol
// name (consumed by IsSymbolDenied / SymbolTokens).
func (d TypedDenial) classIsSymbolShaped() bool {
	switch d.Class {
	case TypedDenialOracleSymbolUnverified,
		TypedDenialEvidenceSubjectUnverified,
		// external_perf_stall_unresolved straddles both axes:
		// emit_perf_trace stamps the stall FILE (path token) and the
		// stall SYMBOL (symbol token) as two separate denials under
		// this class, and the IsSymbolDenied contract has always
		// claimed coverage for the symbol half. Listing the class
		// here makes the symbol half load-bearing at the L1 symbol
		// gates (grep pattern / repo_map entry_point); the path half
		// is unaffected because IsSymbolDenied is exact-match and a
		// path string never equals a bare symbol token.
		TypedDenialExternalPerfStallUnresolved:
		return true
	}
	return false
}

// sanitiseClassSuffix returns a short marker for the placeholder.
// Note: these are LLM-facing — kept descriptive but generic, no
// internal pipeline names ("typed gate" / "TypedDenials" / "Phase X").
func sanitiseClassSuffix(c TypedDenialClass) string {
	switch c {
	case TypedDenialExternalLogFrameUnresolved:
		return "external-source"
	case TypedDenialExternalPerfStallUnresolved:
		return "external-source"
	case TypedDenialOracleSymbolUnverified:
		return "unknown-symbol"
	case TypedDenialDriftFrameRelocated:
		return "moved-or-removed"
	case TypedDenialEvidenceSubjectUnverified:
		return "unverified"
	case TypedDenialAttachedExtractedUnscoped:
		return "out-of-scope"
	}
	return "unverified"
}

// HumanRefusalReason produces an LLM-facing prose explanation for a
// tool refusal triggered by this denial. The message:
//
//   - Uses NO internal pipeline terminology (no "TypedDenials",
//     "corroborate gate", "BugClass", "IntentHint", phase names, etc.)
//     — the LLM has no background to interpret those.
//   - Uses NO fixture-specific examples (no "race condition / NPE")
//     — over-fitting risks the LLM pattern-matching to the example
//     instead of reasoning from the actual signal.
//   - Phrases the situation in terms the LLM CAN understand:
//     "the path/symbol comes from an external runtime input
//     (attached log / trace / pasted artifact) but is not present
//     in this repository" + a generic next-step direction.
//   - `scope` is the tool name that refused ("read", "grep", etc.)
//     for context only; the message's substance depends on Class.
//
// Used by tool registry refusals (read_file / grep / repo_map) and
// by prompt-side renderers that explain the deny set to the LLM.
func (d TypedDenial) HumanRefusalReason(scope string) string {
	switch d.Class {
	case TypedDenialExternalLogFrameUnresolved:
		return "This path appears in an attached runtime log's stack frame, but the named function does not actually exist in the file at this path inside the current repository. The frame likely references code from a different version, a different application, or a build artifact that is not part of this codebase. Do not retry this path — answer the user's question from the information visible in the log itself (the failure type, the call relationships, the message text). The repository file at this path will not provide grounding."
	case TypedDenialExternalPerfStallUnresolved:
		return "This path appears in an attached performance trace's tag, but the named symbol does not actually exist in the file at this path inside the current repository. The trace likely captured a different application or a different build. Do not retry this path — answer from what the trace itself shows (the duration, the kind of stall, the call sequence). The repository file at this path will not provide grounding."
	case TypedDenialOracleSymbolUnverified:
		return "This symbol name does not exist in any source file indexed for the current repository (or workspace, for multi-repo). Searching the repository for it will return nothing useful. If the user's question is about this name, it likely refers to something outside the codebase — answer from the surrounding context (the attached input, the user's prose) without trying to ground a definition site."
	case TypedDenialDriftFrameRelocated:
		return "The position originally referenced (file/line) no longer holds the symbol the input attributes to it — the code has been moved, renamed, or removed since the input was captured. Do not retry the original location; answer from the input's own description and acknowledge that the current code position is not stable for this symbol."
	case TypedDenialEvidenceSubjectUnverified:
		return "This identifier was extracted from an evidence claim but cannot be corroborated against any indexed source in the current repository. Searching for it as if it were a real symbol will not produce evidence. Treat it as opaque."
	case TypedDenialAttachedExtractedUnscoped:
		return "This token was extracted from an attached external artifact and lies outside the current investigation scope (different repository / module / namespace). Do not search for it in the current repository."
	}
	return "This token was marked unverifiable; do not retry."
}
