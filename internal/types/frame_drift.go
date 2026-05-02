package types

// FrameDriftStatus classifies the drift relationship between a log
// frame's claimed (file, line, func) tuple and the current state of
// the repo. The five values form a strict hierarchy of recoverability
// from the renderer's perspective:
//
//   - DriftStatusNone:        log and current code agree at the same
//                             (file, line). Authority cap = factual.
//   - DriftStatusLineDrift:   log claim resolves to the same file +
//                             same function, line numbers shifted.
//                             Authority cap = conditional.
//   - DriftStatusTailRename:  log claim resolves to the same file but
//                             the function name has changed.
//                             Authority cap = conditional.
//   - DriftStatusFileMoved:   log claim resolves only via a different
//                             file (the function moved across files).
//                             Authority cap = historical.
//   - DriftStatusUnmappable:  log claim cannot be located in current
//                             code at all. The function/file may have
//                             been removed entirely. Authority cap =
//                             historical.
//
// The empty string is the zero value, meaning "drift detection has
// not been run". Distinct from DriftStatusNone (which is a positive
// "no drift detected" verdict).
type FrameDriftStatus string

const (
	// DriftStatusUnknown is the zero value. Treat as "no detection
	// has run yet"; consumers default to legacy behaviour (Authority
	// = factual when paired with a current-repo origin).
	DriftStatusUnknown FrameDriftStatus = ""

	DriftStatusNone        FrameDriftStatus = "none"
	DriftStatusLineDrift   FrameDriftStatus = "line_drift"
	DriftStatusTailRename  FrameDriftStatus = "tail_rename"
	DriftStatusFileMoved   FrameDriftStatus = "file_moved"
	DriftStatusUnmappable  FrameDriftStatus = "unmappable"
)

var allFrameDriftStatuses = []FrameDriftStatus{
	DriftStatusUnknown,
	DriftStatusNone,
	DriftStatusLineDrift,
	DriftStatusTailRename,
	DriftStatusFileMoved,
	DriftStatusUnmappable,
}

// AllFrameDriftStatuses returns the canonical list in declaration
// order. Callers must not mutate the returned slice.
func AllFrameDriftStatuses() []FrameDriftStatus {
	out := make([]FrameDriftStatus, len(allFrameDriftStatuses))
	copy(out, allFrameDriftStatuses)
	return out
}

// IsValid reports whether s is one of the canonical 6 values
// (including the empty zero value).
func (s FrameDriftStatus) IsValid() bool {
	for _, declared := range allFrameDriftStatuses {
		if s == declared {
			return true
		}
	}
	return false
}

// FrameDrift is the per-frame drift verdict the logtriage drift
// detector emits. Status carries the classification; AnchoredFile
// and AnchoredLine carry where the symbol resolves in current code
// (when resolvable — empty/0 for DriftStatusUnmappable). AnchoredFunc
// carries the current name (when status is DriftStatusTailRename;
// equal to the original Func otherwise).
//
// FrameDrift values are produced by
// internal/analysis/logtriage.DetectDriftForFrame and consumed by
// the emit_evidence hook (commit 3) to compute ClaimOrigin and
// AuthorityCeiling for evidence items whose anchor matches a log
// frame.
type FrameDrift struct {
	Status        FrameDriftStatus
	AnchoredFile  string
	AnchoredLine  int
	AnchoredFunc  string
	OriginalFile  string
	OriginalLine  int
	OriginalFunc  string
}

// AuthorityCeiling reports the AuthorityCeiling cap implied by a
// given drift status. Used by the emit_evidence hook to translate
// drift verdicts into ceiling values without each callsite
// re-implementing the mapping.
//
// Mapping:
//
//	DriftStatusNone        → AuthorityFactual
//	DriftStatusLineDrift   → AuthorityConditional
//	DriftStatusTailRename  → AuthorityConditional
//	DriftStatusFileMoved   → AuthorityHistorical
//	DriftStatusUnmappable  → AuthorityHistorical
//	DriftStatusUnknown     → AuthorityUnknown (drift not detected; caller may keep zero value)
func (s FrameDriftStatus) AuthorityCeiling() AuthorityCeiling {
	switch s {
	case DriftStatusNone:
		return AuthorityFactual
	case DriftStatusLineDrift, DriftStatusTailRename:
		return AuthorityConditional
	case DriftStatusFileMoved, DriftStatusUnmappable:
		return AuthorityHistorical
	}
	return AuthorityUnknown
}
