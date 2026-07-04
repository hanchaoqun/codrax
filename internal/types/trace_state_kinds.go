package types

// trace_state_kinds.go — TSH batch (typed-signal hygiene, 2026-07-05): the
// single authoritative universe of scheduler-state WORDS that may appear in
// TraceCausalProjectionNode.StateKind. Same failure family as the §7.4
// selected_window token incident and the rich-note key registry (NKR,
// trace_note_keys.go): the words were scattered raw literals across ≥8
// independent ToLower switches (types + internal/tool), so adding a state
// class relied on human memory and one missed switch meant silent
// misclassification.
//
// Production lanes (both pinned):
//   - the dominant_state rich note, whose values are the tracequery
//     dominant-lane strings (running / runnable / s_sleep / d_sleep /
//     io_wait — bridge-pinned in internal/tool against
//     tracequery.ThreadStateDominantLaneUniverse);
//   - the Object fallback (traceCausalProjectionCanonicalStateWord), which
//     recognizes EXACTLY this universe by construction — it reads this table.
//
// stopped / dead / unknown are deliberately NOT members: the tracequery
// per-state lanes skip stopped/dead (§7.11 B-1) and no dominant_state note
// can carry them today; extending the dominant lanes requires extending this
// universe in the same change (the tool-side bridge pin goes red otherwise).
//
// Consumption: every switch/comparison on StateKind-derived strings in
// internal/types and internal/tool is AST-pinned (trace_state_kinds_pin_test
// .go and the tool-side twin, both driven by the shared scanner in
// trace_state_kinds_scan.go): case literals must be registered words (or a
// ledgered consumer-only alias, e.g. "runnable_wait"), and every switch
// covers the universe, carries a default, or declares its fall-through
// members with a rationale.
//
// NKR×TSK intersection (TSH review F7 ruling: feature retention, lanes
// orthogonal): five of these words (running / runnable / sleep / d_state /
// io_wait) also exist as rich-note KEY names in the NKR state family
// (trace_note_keys.go 状态族) where the word is a KEY carrying a per-state ms
// duration — here the word is the VALUE domain of StateKind/dominant_state.
// Double guardianship is intentional; renaming any of the five on either
// side must visit both registries (goldens + pins) in the same change. See
// the mirror note on the NKR 状态族 block.
const (
	TraceStateKindRunning              = "running"
	TraceStateKindRunnable             = "runnable"
	TraceStateKindSleep                = "sleep"
	TraceStateKindSSleep               = "s_sleep"
	TraceStateKindSleepWait            = "sleep_wait"
	TraceStateKindDSleep               = "d_sleep"
	TraceStateKindDState               = "d_state"
	TraceStateKindIOWait               = "io_wait"
	TraceStateKindUninterruptibleSleep = "uninterruptible_sleep"
)

// TraceStateKindUniverse is the closed state-word list (canonical lowercase).
var TraceStateKindUniverse = []string{
	TraceStateKindRunning,
	TraceStateKindRunnable,
	TraceStateKindSleep,
	TraceStateKindSSleep,
	TraceStateKindSleepWait,
	TraceStateKindDSleep,
	TraceStateKindDState,
	TraceStateKindIOWait,
	TraceStateKindUninterruptibleSleep,
}

var traceStateKindSet = func() map[string]bool {
	set := make(map[string]bool, len(TraceStateKindUniverse))
	for _, word := range TraceStateKindUniverse {
		set[word] = true
	}
	return set
}()

// TraceStateKindRegistered reports whether word (already canonical: trimmed +
// lowercased by the caller) is a registered scheduler-state word. Precise
// exact-match signal; callers own their TrimSpace/ToLower convention exactly
// as the historical inline switches did.
func TraceStateKindRegistered(word string) bool {
	return traceStateKindSet[word]
}
