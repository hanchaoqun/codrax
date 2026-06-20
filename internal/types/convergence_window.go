package types

// ConvergenceWindow is the shared low-delta / no-progress convergence primitive:
// a bounded rolling history of typed signatures answering "has the signature
// stopped changing across recent attempts?". T is any comparable typed
// signature — a fingerprint struct, a hash, a deterministic typed-count string.
//
// It is typed-only by construction and never inspects user prose, model
// rationale, or rendered summaries: the caller reduces its domain state to a
// comparable signature, and the window only compares signatures for equality.
//
// Two existing detectors share this tail-equality shape — the read-loop
// closure-fingerprint stall (lastNFingerprintsEqual over ClosureFingerprint)
// and the data-lane relation no-progress counter (RecentRelationNoProgressCount
// over a ProgressSignature string). New convergence boundaries (e.g. the
// pre-complete downgrade low-delta boundary) build on this primitive instead of
// re-deriving the tail-equality walk or adding another one-shot latch.
type ConvergenceWindow[T comparable] struct {
	history []T
	max     int
}

// NewConvergenceWindow returns a window bounded to the last maxLen signatures.
func NewConvergenceWindow[T comparable](maxLen int) *ConvergenceWindow[T] {
	if maxLen < 1 {
		maxLen = 1
	}
	return &ConvergenceWindow[T]{max: maxLen}
}

// Push appends a signature, dropping the oldest entry past the bound.
func (w *ConvergenceWindow[T]) Push(sig T) {
	w.history = append(w.history, sig)
	if len(w.history) > w.max {
		w.history = append(w.history[:0:0], w.history[len(w.history)-w.max:]...)
	}
}

// Len reports the number of retained signatures.
func (w *ConvergenceWindow[T]) Len() int { return len(w.history) }

// ConsecutiveEqualTail reports how many trailing signatures equal the most
// recent one (0 for an empty window, >=1 otherwise).
func (w *ConvergenceWindow[T]) ConsecutiveEqualTail() int {
	return ConsecutiveEqualTail(w.history)
}

// StalledAt reports whether the last n signatures are all equal (n>=2) — the
// "no new typed delta across n attempts" convergence condition.
func (w *ConvergenceWindow[T]) StalledAt(n int) bool {
	return StalledAtTail(w.history, n)
}

// ConsecutiveEqualTail returns how many entries at the end of history equal the
// last entry. It is the slice form for callers that already hold a history
// (e.g. a ClosureFingerprint slice) and do not need a window object.
func ConsecutiveEqualTail[T comparable](history []T) int {
	if len(history) == 0 {
		return 0
	}
	last := history[len(history)-1]
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i] != last {
			break
		}
		count++
	}
	return count
}

// StalledAtTail reports whether the last n entries of history are all equal
// (n>=2). It is the generic form of the read-loop's lastNFingerprintsEqual and
// of any per-attempt low-delta convergence check.
func StalledAtTail[T comparable](history []T, n int) bool {
	if n < 2 || len(history) < n {
		return false
	}
	return ConsecutiveEqualTail(history) >= n
}
