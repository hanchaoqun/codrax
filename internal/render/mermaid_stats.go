package render

import (
	"sort"
	"sync"
	"sync/atomic"
)

// MermaidStats is the user-visible snapshot of mermaid render
// activity for the current process. Surfaced via /mermaid in the
// REPL and by the renderability gate's diagnostic message.
//
// Counter semantics (all monotonic across the process lifetime):
//
//   - Attempts: every renderMermaidFenceBody entry. One per fenced
//     block routed into the library path.
//   - Succeeded: blocks that produced an aligned ASCII grid.
//   - FallbackSubstituted: blocks where one or more narrow
//     multi-byte runes were replaced via narrowRuneFallback. Counts
//     blocks, not runes.
//   - LibraryRejected: blocks that reached the library and failed
//     (parse error, panic, empty output) OR exhausted the
//     placeholder pool. Excludes "unsupported kind" — those never
//     reach the library.
//   - UnsupportedKind: blocks short-circuited at routing time
//     because the keyword is in mermaidUnsupportedKeywords. Stored
//     per-keyword so /mermaid can tell the user which kinds are
//     showing up.
//   - GateBlocked: increments only when the renderability gate
//     (L4) rejected an emit_answer_document submission. Recorded
//     by emit_answer_document.go via RecordMermaidGateBlocked.
//
// Race-safety: counters are atomic.Int64; UnsupportedKind goes
// through an unexported mutex (a small map; the read path takes a
// snapshot under the lock).
type MermaidStats struct {
	Attempts            int64
	Succeeded           int64
	FallbackSubstituted int64
	FallbackRunes       int64
	LibraryRejected     int64
	UnsupportedKind     map[string]int64
	GateBlocked         int64
	LastFailureReason   string
}

var (
	mermaidAttempts            atomic.Int64
	mermaidSucceeded           atomic.Int64
	mermaidFallbackSubst       atomic.Int64
	mermaidFallbackRuneCount   atomic.Int64
	mermaidLibraryRejected     atomic.Int64
	mermaidGateBlocked         atomic.Int64
	mermaidUnsupportedKindLock sync.Mutex
	mermaidUnsupportedKind     = map[string]int64{}
	mermaidLastFailureLock     sync.Mutex
	mermaidLastFailure         string
)

// recordMermaidAttempt is called once per renderMermaidFenceBody
// entry (including blocks that go on to succeed).
func recordMermaidAttempt() { mermaidAttempts.Add(1) }

// recordMermaidSucceeded marks a render path that ended in an
// aligned ASCII grid.
func recordMermaidSucceeded() { mermaidSucceeded.Add(1) }

// recordMermaidFallbackSubstituted reports one block had `n`
// narrow-multibyte rune substitutions applied. Block-level counter
// increments by 1; rune-level counter increments by n.
func recordMermaidFallbackSubstituted(n int) {
	mermaidFallbackSubst.Add(1)
	if n > 0 {
		mermaidFallbackRuneCount.Add(int64(n))
	}
}

// recordMermaidLibraryRejected captures library failures (parse,
// panic, empty output, pool exhaustion). Caller is expected to
// log the underlying error at WARN before calling.
func recordMermaidLibraryRejected() {
	mermaidLibraryRejected.Add(1)
}

// recordMermaidUnsupportedKind notes a routing-stage short-circuit
// because the keyword is outside pgavlin's subset (classDiagram,
// stateDiagram, ...).
func recordMermaidUnsupportedKind(kw string) {
	if kw == "" {
		return
	}
	mermaidUnsupportedKindLock.Lock()
	mermaidUnsupportedKind[kw]++
	mermaidUnsupportedKindLock.Unlock()
}

// RecordMermaidGateBlocked is called by the emit_answer_document
// renderability gate (L4) when a submission is rejected because a
// mermaid block cannot render. Exported because the caller lives in
// the internal/tool package.
func RecordMermaidGateBlocked() { mermaidGateBlocked.Add(1) }

// RecordMermaidLastFailure stores a one-line reason for the most
// recent failure so /mermaid can surface it. Overwrites; we only
// keep the latest.
func RecordMermaidLastFailure(reason string) {
	if reason == "" {
		return
	}
	mermaidLastFailureLock.Lock()
	mermaidLastFailure = reason
	mermaidLastFailureLock.Unlock()
}

// GetMermaidStats returns a snapshot of the counters. Safe for
// concurrent use; the returned struct is a copy.
func GetMermaidStats() MermaidStats {
	mermaidUnsupportedKindLock.Lock()
	kinds := make(map[string]int64, len(mermaidUnsupportedKind))
	for k, v := range mermaidUnsupportedKind {
		kinds[k] = v
	}
	mermaidUnsupportedKindLock.Unlock()

	mermaidLastFailureLock.Lock()
	last := mermaidLastFailure
	mermaidLastFailureLock.Unlock()

	return MermaidStats{
		Attempts:            mermaidAttempts.Load(),
		Succeeded:           mermaidSucceeded.Load(),
		FallbackSubstituted: mermaidFallbackSubst.Load(),
		FallbackRunes:       mermaidFallbackRuneCount.Load(),
		LibraryRejected:     mermaidLibraryRejected.Load(),
		UnsupportedKind:     kinds,
		GateBlocked:         mermaidGateBlocked.Load(),
		LastFailureReason:   last,
	}
}

// ResetMermaidStats clears all counters. Used by tests; not exposed
// in the REPL (a fresh process is the right way to reset in user
// flows).
func ResetMermaidStats() {
	mermaidAttempts.Store(0)
	mermaidSucceeded.Store(0)
	mermaidFallbackSubst.Store(0)
	mermaidFallbackRuneCount.Store(0)
	mermaidLibraryRejected.Store(0)
	mermaidGateBlocked.Store(0)
	mermaidUnsupportedKindLock.Lock()
	mermaidUnsupportedKind = map[string]int64{}
	mermaidUnsupportedKindLock.Unlock()
	mermaidLastFailureLock.Lock()
	mermaidLastFailure = ""
	mermaidLastFailureLock.Unlock()
}

// SortedUnsupportedKinds returns the kinds keys sorted by count
// descending, then name ascending — used for stable rendering in
// the REPL command.
func (s MermaidStats) SortedUnsupportedKinds() []string {
	keys := make([]string, 0, len(s.UnsupportedKind))
	for k := range s.UnsupportedKind {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if s.UnsupportedKind[keys[i]] != s.UnsupportedKind[keys[j]] {
			return s.UnsupportedKind[keys[i]] > s.UnsupportedKind[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}
