package types

import "strings"

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

// TypedDenialSet is the BusContext-level collection. The empty value
// is ready to use; methods are nil-safe so consumers can call them
// even when no denials have been stamped (the common case for
// single-shot runs against well-formed repos).
type TypedDenialSet struct {
	Denials []TypedDenial `json:"denials,omitempty"`
}

// Add stamps a new denial. Duplicate (Class, Token) pairs are
// silently dropped — the same token failing multiple gates of the
// same class is one logical denial.
func (s *TypedDenialSet) Add(d TypedDenial) {
	if s == nil {
		return
	}
	d.Token = strings.TrimSpace(d.Token)
	if d.Token == "" || !IsValidTypedDenialClass(d.Class) {
		return
	}
	for _, existing := range s.Denials {
		if existing.Class == d.Class && existing.Token == d.Token {
			return
		}
	}
	s.Denials = append(s.Denials, d)
}

// Len reports the number of stamped denials.
func (s *TypedDenialSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.Denials)
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
// same path AND a repo-relative variant. Substring is intentionally
// NOT used — too noisy.
func (s *TypedDenialSet) IsPathDenied(path string) bool {
	if s == nil || path == "" {
		return false
	}
	path = strings.TrimSpace(path)
	for _, d := range s.Denials {
		if !d.classIsPathShaped() {
			continue
		}
		if path == d.Token {
			return true
		}
		// Suffix match for repo-relative ↔ absolute mismatches.
		if strings.HasSuffix(path, "/"+d.Token) || strings.HasSuffix(d.Token, "/"+path) {
			return true
		}
	}
	return false
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
	if s == nil || name == "" {
		return false
	}
	name = strings.TrimSpace(name)
	for _, d := range s.Denials {
		if !d.classIsSymbolShaped() {
			continue
		}
		if name == d.Token {
			return true
		}
	}
	return false
}

// Sanitise replaces every denied token in prose with a typed
// placeholder. LLM-facing prompt builders call this on raw fields
// (frame.Raw, stall raw text, original log lines) before injecting,
// so the LLM cannot extract verbatim paths/symbols from prose to
// bypass the typed gate.
//
// The placeholder format is "<unverified-{class-suffix}>" so the
// LLM gets a consistent marker it can learn to recognise.
// Replacement is exact-string substitution; substring is intentional
// here (raw log lines may contain the token mid-line).
func (s *TypedDenialSet) Sanitise(prose string) string {
	if s == nil || prose == "" || len(s.Denials) == 0 {
		return prose
	}
	out := prose
	for _, d := range s.Denials {
		if d.Token == "" {
			continue
		}
		marker := "<unverified-" + sanitiseClassSuffix(d.Class) + ">"
		out = strings.ReplaceAll(out, d.Token, marker)
	}
	return out
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
	seen := make(map[string]bool)
	var out []string
	for _, d := range s.Denials {
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
	seen := make(map[string]bool)
	var out []string
	for _, d := range s.Denials {
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
		TypedDenialEvidenceSubjectUnverified:
		return true
	}
	// external_perf_stall_unresolved straddles both — when the gate
	// fires on a Symbol miss, the stamping caller passes the symbol
	// as Token; when the gate fires on a File miss, the path is
	// stamped. We resolve via classIsPathShaped having precedence:
	// callers stamp two separate denials when both axes fail.
	return false
}

// sanitiseClassSuffix returns a short marker for the placeholder.
func sanitiseClassSuffix(c TypedDenialClass) string {
	switch c {
	case TypedDenialExternalLogFrameUnresolved:
		return "log-frame"
	case TypedDenialExternalPerfStallUnresolved:
		return "perf-stall"
	case TypedDenialOracleSymbolUnverified:
		return "symbol"
	case TypedDenialDriftFrameRelocated:
		return "drift-frame"
	case TypedDenialEvidenceSubjectUnverified:
		return "evidence-subject"
	case TypedDenialAttachedExtractedUnscoped:
		return "attached-extracted"
	}
	return "denied"
}
