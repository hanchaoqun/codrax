// Package memory persists multi-turn REPL conversations and compacts
// older turns into a searchable MEMORY.md index. The goal is to keep the
// active prompt small while still letting the model recover full prior
// context when a new request matches a compacted topic by keyword.
//
// Layout under the configured directory:
//
//	memory/
//	  MEMORY.md            // append-only index of compacted entries
//	  turns/<id>.md        // verbatim copy of every turn ever appended
//
// The store keeps the most recent maxRecent turns in memory verbatim.
// Once that cap (or maxBytes) is exceeded, the oldest recent turn is
// passed to a Summarizer (LLM-backed in production, stubbed in tests),
// the resulting IndexEntry is appended to MEMORY.md, and the turn drops
// out of the recent slice. The full text remains on disk under turns/
// for retrieval.
package memory

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Hard caps that bound how much historical turn content ever reaches
// an LLM prompt.
//
//   - maxTurnBodyBytes caps what we ever persist (and what readTurnFile
//     surfaces back from pre-existing oversized files). Set at ~one
//     glamour-rendered explorer answer so normal REPL sessions never
//     trip it; when a tool floods the response buffer the tail is
//     dropped instead of poisoning every subsequent BuildContext call.
//
//   - maxBuildContextMatches is the top-K brake on keyword-matched
//     compacted entries. Only the LLM-generated Summary line is ever
//     emitted to the prompt — full turn files stay on disk and are
//     available via /history.
//
// buildContextLimits holds the per-dispatch caps for BuildContext.
// Populated from MemorySettings at NewStore time.
type buildContextLimits struct {
	maxTurnBodyBytes       int
	maxBuildContextMatches int
}

// ansiEscapeRE matches the CSI ("ESC[…letter") sequences that leak into
// turn.Response via glamour's markdown rendering. They have no semantic
// value once stored in memory and they inflate file sizes dramatically
// when the response contains lots of colored tokens.
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// sanitizeTurnText strips ANSI escape sequences and truncates to
// the given byte limit. Truncation walks back to a UTF-8 rune
// boundary so we never emit half a multibyte character (the REPL
// history is overwhelmingly CJK).
func sanitizeTurnText(s string, maxBytes int) string {
	s = ansiEscapeRE.ReplaceAllString(s, "")
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "\n…[truncated]"
}

// Kind classifies how a Turn was produced (and, transitively, which
// retrieval policy BuildContext should use when assembling prior-
// conversation context for a new turn of the same Kind).
//
// Empty-string Kind is the legacy value — used by any turn loaded
// from a pre-upgrade MEMORY.md / turns/ file, and also the default
// when BuildContext is called without an explicit BuildOpts.Kind.
// Retrieval for empty Kind behaves byte-for-byte identically to the
// pre-upgrade pre-session-32 BuildContext, preserving every existing
// test and cross-version file on disk.
type Kind string

const (
	// KindChitchat = casual /chat REPL turn. Retrieval favors
	// keeping the recent session thread intact (3 same-session turns
	// pinned verbatim up to 800 chars) over broad compacted recall.
	KindChitchat Kind = "chitchat"
	// KindPipeline = normal REPL analysis turn that goes through
	// the full orchestrator pipeline. Retrieval biases toward
	// entity-weighted compacted recall (entities count 3x keywords)
	// because pipeline answers anchor on source symbols.
	KindPipeline Kind = "pipeline"
	// KindPlan = REPL turn that consumed a ChangePlan via /approve
	// (or rejected via /reject). Recorded by handleApproveCmd /
	// handleRejectCmd so the memory index chains plan outcomes to
	// their originating user requests (via Turn.Refs →
	// ChangePlan.TriggerTurnID). Retrieval shares the pipeline
	// policy for now — plan turns are infrequent but behave like
	// pipeline turns for entity-based recall.
	KindPlan Kind = "plan"
)

// Turn is one user request + assembled assistant response.
type Turn struct {
	ID        string
	Request   string
	Response  string
	Timestamp time.Time

	// RequestForSummary is the expanded-paste form of Request. Only
	// the REPL sets it; it lives in s.recent until this turn is
	// compacted, then feeds into Summarize so the LLM can generate
	// keywords/topic from the actual paste content. It is never
	// persisted to turns/<id>.md — restart loses it, which is fine
	// because compaction normally consumes the turn before restart.
	// Empty means "no expansion; Request is authoritative" (scripted
	// mode, orphan-recovered turns).
	RequestForSummary string

	// SessionID identifies the REPL session that produced this turn.
	// Set by the REPL at New() time (format: sess-<unix-nano>).
	// Empty for orphan-loaded legacy turns and for tests that append
	// directly. Persisted to turns/<id>.md inside an HTML comment
	// header so orphan recovery restores it across restarts.
	SessionID string

	// Kind is the routing classification (see Kind constants). Empty
	// means "unclassified" and falls through to the legacy retrieval
	// policy. Persisted alongside SessionID in the turn file header.
	Kind Kind
}

// IndexEntry is a compacted summary of a Turn that lives in MEMORY.md.
// FullRef points at the original turn file under turns/.
//
// WasError marks entries that were synthesized from an error-laden
// turn (see compactOldest). Such entries bypass the LLM summarizer,
// carry an empty Keywords slice so matchIndex cannot surface them,
// and are skipped explicitly by BuildContext as defense in depth.
// The flag is persisted to MEMORY.md as `was_error: true` and
// defaults to false when absent so pre-schema entries round-trip
// unchanged.
//
// SessionID / Kind / Entities / Refs are session-32 additions:
//   - SessionID lets BuildContext boost entries from the current
//     REPL session so follow-up questions ("那个 bug 修了吗") surface
//     their own thread even with no keyword overlap.
//   - Kind mirrors Turn.Kind at the index-entry level.
//   - Entities are stable symbol-level anchors (file paths, function
//     names, identifiers). They match with higher weight than
//     Keywords because pipeline answers anchor on source symbols.
//   - Refs names prior turn IDs the turn explicitly referenced; used
//     as a 1–2 hop chain traversal from primary matches.
//
// All four new fields are optional: absent-in-file parses to zero-
// value, and absent-in-struct renders as no frontmatter line, so
// pre-upgrade MEMORY.md files round-trip unchanged through parse→
// append cycles.
type IndexEntry struct {
	ID        string
	Topic     string
	Keywords  []string
	Summary   string
	FullRef   string
	WasError  bool
	SessionID string
	Kind      Kind
	Entities  []string
	Refs      []string
}

// BuildOpts tunes BuildContext retrieval. All fields optional; zero-
// value opts reproduce the pre-session-32 BuildContext behavior
// byte-for-byte (policy lookup returns the default policy).
type BuildOpts struct {
	// Kind selects the retrieval policy (see Kind constants). Empty
	// falls back to the legacy policy: 3 compacted matches, oneLine
	// recent turns, no session pinning, no entity boost.
	Kind Kind
	// SessionID is the caller's current REPL session id. Entries and
	// recent turns whose SessionID matches are preferred during
	// retrieval: recent turns become session-pinned (kept even when
	// keyword match would drop them), and compacted entries get a
	// small score boost. Empty SessionID disables both behaviors.
	SessionID string
}

// kindPolicy holds the per-Kind retrieval tuning. Assembled inline in
// policyFor(Kind) so the defaults are version-controlled in-code; a
// yaml knob can override values later without changing the shape.
type kindPolicy struct {
	// sessionPinCount: how many same-session recent turns to keep
	// unconditionally (regardless of keyword match). 0 disables
	// session pinning (legacy behavior).
	sessionPinCount int
	// recentBodyChars: rune-cap for same-session recent turn body
	// rendering. 0 means "use oneLine" (the 200-char collapse that
	// every pre-session-32 caller sees).
	recentBodyChars int
	// compactedMatchCap: top-K on the matched compacted-entry list.
	// -1 means "use s.bcLimits.maxBuildContextMatches" (the 3-entry
	// default that preserves legacy behavior).
	compactedMatchCap int
	// entityScoreMul: score multiplier for entity hits vs keyword
	// hits in matchEntries. Pipeline weights entities higher because
	// source symbols are more stable than free-form words.
	entityScoreMul int
	// refsChainDepth: how many hops of IndexEntry.Refs to follow
	// after the primary match list is picked. 0 disables chain
	// traversal entirely (legacy behavior + first-ship conservatism).
	refsChainDepth int
}

// policyFor returns the retrieval policy for the given Kind. Empty
// Kind returns the legacy policy so BuildContext callers that did not
// pass BuildOpts see byte-identical output.
func policyFor(k Kind) kindPolicy {
	switch k {
	case KindChitchat:
		return kindPolicy{sessionPinCount: 3, recentBodyChars: 800, compactedMatchCap: 3, entityScoreMul: 2, refsChainDepth: 1}
	case KindPipeline:
		return kindPolicy{sessionPinCount: 2, recentBodyChars: 0, compactedMatchCap: 5, entityScoreMul: 3, refsChainDepth: 2}
	default:
		return kindPolicy{sessionPinCount: 0, recentBodyChars: 0, compactedMatchCap: -1, entityScoreMul: 2, refsChainDepth: 0}
	}
}

// Summarizer compresses a Turn into an IndexEntry. Implementations are
// expected to call an LLM; tests can supply a deterministic stub.
type Summarizer interface {
	Summarize(ctx context.Context, turn Turn) (IndexEntry, error)
}

// Store holds in-memory recent turns and a parsed view of MEMORY.md.
// It is safe for concurrent use both within one process (via mu) and
// across processes that share the same directory.
//
// Two file locks govern the cross-process behavior:
//
//   - flock (MEMORY.md.lock) — taken briefly per memory operation.
//     Shared for reads, exclusive for writes. Serializes mutations
//     and ensures readers see a consistent on-disk index.
//
//   - instanceLock (.instance.lock) — held SHARED for the entire
//     lifetime of the Store. Multiple peer Stores can hold it
//     simultaneously. Its only purpose is presence detection: at
//     NewStore time we briefly try to acquire EXCLUSIVE on it
//     non-blocking. Success means no peers are alive in this
//     directory and we may safely run loadOrphanRecent to recover
//     the tail of a previously-crashed session. Failure (would
//     block) means peers exist and orphan recovery is skipped —
//     those un-compacted turn files belong to active peer Stores'
//     recent buffers, not to a crashed previous lifetime.
//
// In-process s.index is reloaded from disk after acquiring either
// flock so it never lags behind a peer's writes.
type Store struct {
	mu sync.Mutex

	dir       string
	turnsDir  string
	indexPath string
	lockPath  string

	flock        *fileLock
	instanceLock *fileLock

	// sidecarPath is a per-Store marker file under <dir>/.instance-*
	// created at NewStore and removed at Close. LivePeerCount lists
	// sidecars whose embedded PID is still alive (and is not our
	// own) to answer "are there other codrax instances active in
	// this directory right now?" — used by REPL /clear to warn the
	// user that the wipe affects shared state.
	sidecarPath string

	recent []Turn
	index  []IndexEntry

	maxRecent  int
	maxBytes   int
	bcLimits   buildContextLimits
	summarizer Summarizer

	// compactInFlight is a single-flight latch: at most one
	// background compaction goroutine runs at a time. Guarded by
	// s.mu. compactWG lets Close (and tests) wait for in-flight
	// compactions to settle before tearing the store down or asserting
	// on index state.
	compactInFlight bool
	compactWG       sync.WaitGroup
}

// sidecarPrefix is the filename prefix for per-Store presence
// markers under the memory directory. Format:
// .instance-<pid>-<unix-nano>. The nano disambiguates two Stores in
// the same process (test scenario, mostly), and the PID lets a peer
// recover from a crash by skipping markers whose owner is dead.
const sidecarPrefix = ".instance-"

// NewStore creates (or opens) a Store rooted at dir. The directory and
// its turns/ subdir are created if missing. Existing MEMORY.md entries
// are parsed back into the in-memory index so the next run carries over
// prior compacted history.
//
// "Orphan" turns — files under turns/ that have no matching entry in
// MEMORY.md, i.e. turns from a previous session that were never
// compacted — are recovered into the recent buffer (up to maxRecent),
// so a restart does not silently drop the tail of the last conversation.
// This is the only place that reads turn files back; the rest of the
// store treats turns/ as write-once history.
func NewStore(dir string, summarizer Summarizer, ms types.MemorySettings) (*Store, error) {
	if dir == "" {
		dir = "memory"
	}
	turnsDir := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turnsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	lockPath := filepath.Join(dir, "MEMORY.md.lock")
	flock, err := newFileLock(lockPath)
	if err != nil {
		return nil, err
	}
	instanceLock, err := newFileLock(filepath.Join(dir, ".instance.lock"))
	if err != nil {
		_ = flock.close()
		return nil, err
	}
	// Use a create-temp pattern instead of pid+timestamp alone. On
	// Windows multiple NewStore calls in the same process can land in
	// the same clock tick, and silently sharing one sidecar would make
	// LivePeerCount undercount peers and let one Close() remove another
	// Store's marker.
	sidecarFile, err := os.CreateTemp(dir, fmt.Sprintf("%s%d-", sidecarPrefix, os.Getpid()))
	if err != nil {
		_ = instanceLock.close()
		_ = flock.close()
		return nil, fmt.Errorf("create instance sidecar: %w", err)
	}
	sidecar := sidecarFile.Name()
	if err := sidecarFile.Close(); err != nil {
		_ = os.Remove(sidecar)
		_ = instanceLock.close()
		_ = flock.close()
		return nil, fmt.Errorf("close instance sidecar: %w", err)
	}
	ms = types.ResolvedMemorySettings(ms)
	s := &Store{
		dir:          dir,
		turnsDir:     turnsDir,
		indexPath:    filepath.Join(dir, "MEMORY.md"),
		lockPath:     lockPath,
		flock:        flock,
		instanceLock: instanceLock,
		sidecarPath:  sidecar,
		maxRecent:    ms.MaxRecentTurns,
		maxBytes:     ms.MaxRecentBytes,
		bcLimits: buildContextLimits{
			maxTurnBodyBytes:       ms.MaxTurnBodyBytes,
			maxBuildContextMatches: ms.MaxBuildContextMatches,
		},
		summarizer: summarizer,
	}

	// Presence detection: try to grab an EXCLUSIVE lock on
	// .instance.lock without blocking. Success means no peer Store is
	// currently holding the lifetime shared lock — i.e. we are the
	// only Store on this directory right now and may recover the
	// tail of any prior crashed session as our own. Failure (would
	// block) means at least one peer Store is alive; the un-compacted
	// turn files in turns/ belong to its recent buffer and pulling
	// them into ours would double-count.
	//
	// We hold the EXCLUSIVE lock across loadOrphanRecent so no peer
	// can sneak in mid-recovery and start writing turn files we'd
	// then misinterpret as orphans of our own crashed session. Only
	// after recovery completes do we downgrade to SHARED, which is
	// the lock state held for the lifetime of the Store and is what
	// later peers' tryLock will observe.
	alone := false
	got, err := s.instanceLock.tryLock()
	if err != nil {
		_ = s.instanceLock.close()
		_ = s.flock.close()
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}
	alone = got

	// Initial index load runs under the shared MEMORY.md lock — a
	// separate file from .instance.lock — so it composes with our
	// instance-lock state regardless of which mode we're in.
	if err := s.withSharedLock(s.loadIndexLocked); err != nil {
		_ = s.Close()
		return nil, err
	}

	if alone {
		if err := s.loadOrphanRecent(); err != nil {
			_ = s.Close()
			return nil, err
		}
		// Atomic on Unix (flock(LOCK_SH) downgrades in place).
		// Brief window on Windows, harmless because no Append has
		// started yet at NewStore time.
		if err := s.instanceLock.downgradeToShared(); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("downgrade instance lock: %w", err)
		}
	} else {
		// We never had exclusive; just take shared and join the
		// pool of live peer Stores.
		if err := s.instanceLock.rlock(); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("hold instance lock: %w", err)
		}
	}
	return s, nil
}

// Close releases both file locks and their underlying fds. Calling
// Close on a nil receiver is safe. Currently optional in production
// — main.go relies on process exit for cleanup — but exposed for
// tests and for callers that want deterministic teardown. The
// instance lock release is what lets a subsequent NewStore detect
// "I am alone" and recover orphan turns; without an explicit Close,
// the OS releases on process exit, which is functionally equivalent
// for the multi-instance scenario.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	// Wait for any in-flight background compaction to finish before
	// tearing down the flock / sidecar. compactOldest uses the same
	// flock, so closing it while a goroutine still holds a reference
	// would surface as a spurious lock error on the tail summarize.
	s.compactWG.Wait()
	var firstErr error
	if s.sidecarPath != "" {
		if err := os.Remove(s.sidecarPath); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
			firstErr = err
		}
	}
	if err := s.instanceLock.close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.flock.close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// LivePeerCount returns the number of OTHER codrax Store instances
// that currently have this memory directory open. The count is
// derived from sidecar marker files written by NewStore: every
// sidecar that is not ours and whose embedded PID is still a live
// process counts as one peer. Stale sidecars left by crashed
// processes are silently ignored.
//
// Used by REPL /clear to format an honest warning before wiping
// shared state. The result is best-effort: a peer that starts
// between the call and the wipe will not be reflected, and a peer
// that crashed without removing its sidecar will not be counted
// (its PID is dead). Neither inaccuracy is harmful — the worst
// case is a slightly misleading prompt, and the underlying Clear
// is still safely serialized via the cross-process lock.
func (s *Store) LivePeerCount() (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("scan memory dir: %w", err)
	}
	selfBase := filepath.Base(s.sidecarPath)
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, sidecarPrefix) {
			continue
		}
		if name == selfBase {
			continue
		}
		pid := pidFromSidecar(name)
		if pid <= 0 {
			continue
		}
		if pidAlive(pid) {
			count++
		}
	}
	return count, nil
}

// pidFromSidecar parses the PID out of a sidecar filename. Format
// is .instance-<pid>-<nano>, so we look at the segment between the
// first '-' after the prefix and the next '-'. Returns 0 on parse
// failure, which causes LivePeerCount to skip the file (treating
// it as alien).
func pidFromSidecar(name string) int {
	core := strings.TrimPrefix(name, sidecarPrefix)
	dash := strings.Index(core, "-")
	if dash <= 0 {
		return 0
	}
	pid, err := strconv.Atoi(core[:dash])
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// withExclusiveLock takes the cross-process exclusive lock, refreshes
// s.index from disk so the closure sees any writes a peer made since
// last call, runs the closure, then releases the lock. The mutation
// closure is responsible for keeping s.index in sync with whatever it
// writes to disk inside the critical section.
func (s *Store) withExclusiveLock(fn func() error) error {
	if err := s.flock.lock(); err != nil {
		return fmt.Errorf("acquire memory lock: %w", err)
	}
	defer func() { _ = s.flock.unlock() }()
	if err := s.loadIndexLocked(); err != nil {
		return err
	}
	return fn()
}

// withSharedLock is the read-side counterpart to withExclusiveLock.
// Multiple processes may hold the shared lock concurrently, but no
// exclusive holder can interleave. We still reload s.index here so
// the read sees the latest committed state from any peer.
func (s *Store) withSharedLock(fn func() error) error {
	if err := s.flock.rlock(); err != nil {
		return fmt.Errorf("acquire memory rlock: %w", err)
	}
	defer func() { _ = s.flock.unlock() }()
	return fn()
}

// loadOrphanRecent scans turns/ for files that are not referenced by
// any MEMORY.md entry and resurrects the most recent maxRecent of them
// into s.recent. Filenames are <id>.md where id is "turn-<unix-nano>"
// in production (so lexicographic sort = chronological); for synthetic
// IDs the test stub uses zero-padded numbers, which also sort correctly.
//
// Failures parsing an individual file are logged via fmt.Fprintf to
// stderr but do not abort startup — a single corrupt turn file should
// not block the REPL from coming up.
func (s *Store) loadOrphanRecent() error {
	entries, err := os.ReadDir(s.turnsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("scan turns dir: %w", err)
	}

	// Build the set of IDs already represented in the compacted index
	// so we don't double-count them in recent.
	compacted := make(map[string]struct{}, len(s.index))
	for _, e := range s.index {
		compacted[e.ID] = struct{}{}
	}

	// Collect candidate filenames (without .md suffix == turn ID).
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		id := strings.TrimSuffix(name, ".md")
		if _, alreadyCompacted := compacted[id]; alreadyCompacted {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// Take the newest maxRecent.
	if len(ids) > s.maxRecent {
		ids = ids[len(ids)-s.maxRecent:]
	}

	for _, id := range ids {
		turn, err := s.readTurnFile(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memory: skipping unreadable turn %s: %v\n", id, err)
			continue
		}
		s.recent = append(s.recent, turn)
	}
	return nil
}

// readTurnFile parses a turn file produced by writeTurnFile back into
// a Turn. The format is fixed by writeTurnFile so the parser is a
// simple split on the section markers; if either marker is missing
// the file is treated as corrupt.
func (s *Store) readTurnFile(id string) (Turn, error) {
	path := filepath.Join(s.turnsDir, id+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Turn{}, err
	}
	text := string(data)

	const (
		reqMarker  = "## Request\n\n"
		respMarker = "\n\n## Response\n\n"
	)
	reqStart := strings.Index(text, reqMarker)
	if reqStart < 0 {
		return Turn{}, errors.New("missing Request marker")
	}
	bodyStart := reqStart + len(reqMarker)
	respStart := strings.Index(text[bodyStart:], respMarker)
	if respStart < 0 {
		return Turn{}, errors.New("missing Response marker")
	}
	request := text[bodyStart : bodyStart+respStart]
	response := strings.TrimRight(text[bodyStart+respStart+len(respMarker):], "\n")

	// Best-effort timestamp recovery from the "_<RFC3339>_" header line.
	ts := time.Time{}
	if i := strings.Index(text, "\n_"); i >= 0 {
		end := strings.Index(text[i+2:], "_")
		if end > 0 {
			if parsed, perr := time.Parse(time.RFC3339, text[i+2:i+2+end]); perr == nil {
				ts = parsed
			}
		}
	}

	// Session-32 metadata recovery. HTML comments sit between the
	// timestamp and "## Request"; legacy files missing them leave
	// Kind / SessionID as zero values, which makes BuildContext treat
	// the turn as cross-session (the safe degradation).
	headerRegion := text
	if i := strings.Index(text, reqMarker); i > 0 {
		headerRegion = text[:i]
	}
	kind := Kind(extractHTMLCommentValue(headerRegion, "kind"))
	session := extractHTMLCommentValue(headerRegion, "session_id")

	// Sanitize on read so a legacy oversized turn file (written before
	// Append learned to cap) cannot smuggle multi-megabyte content into
	// s.recent and from there into the summarizer. This is the
	// remediation path for pre-existing directories.
	return Turn{
		ID:        id,
		Request:   sanitizeTurnText(request, s.bcLimits.maxTurnBodyBytes),
		Response:  sanitizeTurnText(response, s.bcLimits.maxTurnBodyBytes),
		Timestamp: ts,
		Kind:      kind,
		SessionID: session,
	}, nil
}

// extractHTMLCommentValue scans text for a line of the form
// `<!-- <key>: <value> -->` and returns the trimmed value. Empty
// when the key is absent; legacy turn files (no comments) always
// hit this branch. Used by readTurnFile to round-trip Kind/SessionID
// through orphan recovery.
func extractHTMLCommentValue(text, key string) string {
	prefix := "<!-- " + key + ":"
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(prefix):]
	end := strings.Index(rest, "-->")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// Append records a new turn. The full text is written to turns/<id>.md
// immediately. If the recent buffer is over budget, the oldest turn is
// compacted into MEMORY.md via the summarizer.
//
// Turn IDs embed the process PID alongside the wall-clock timestamp
// so two codrax instances can append into the same memory directory
// without ever colliding on a turn filename. Lexicographic sort still
// approximates chronological order: timestamps come first, the PID
// only disambiguates within the same nanosecond.
func (s *Store) Append(turn Turn) error {
	if turn.ID == "" {
		turn.ID = fmt.Sprintf("turn-%d-%d", time.Now().UnixNano(), os.Getpid())
	}
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now()
	}
	// Scrub ANSI and enforce the per-turn byte cap once, up front, so
	// the same bounded value lands on disk AND in the in-process recent
	// buffer. Downstream consumers (writeTurnFile, recentBytesLocked,
	// compactOldest → Summarize) never see the raw glamour output.
	turn.Request = sanitizeTurnText(turn.Request, s.bcLimits.maxTurnBodyBytes)
	turn.Response = sanitizeTurnText(turn.Response, s.bcLimits.maxTurnBodyBytes)
	// RequestForSummary is bounded by the same cap so a 40 KB paste
	// doesn't linger unbounded in s.recent awaiting compaction, and so
	// the LLM summarizer call has a predictable upper bound regardless
	// of how large the user's paste was. The first N KB of a paste is
	// usually plenty for the summarizer to produce topic/keywords.
	if turn.RequestForSummary != "" {
		turn.RequestForSummary = sanitizeTurnText(turn.RequestForSummary, s.bcLimits.maxTurnBodyBytes)
	}
	if err := s.writeTurnFile(turn); err != nil {
		return err
	}

	s.mu.Lock()
	s.recent = append(s.recent, turn)
	s.mu.Unlock()

	// Compaction runs in a background goroutine so the REPL's
	// next prompt isn't blocked on a slow LLM summarizer call.
	// On abrupt exit any un-compacted turn file is rehydrated
	// by loadOrphanRecent on next startup — worst case we
	// re-run a summarize that was about to finish anyway.
	s.scheduleCompact()
	return nil
}

// scheduleCompact starts a background compaction if one isn't already
// running and the recent buffer is over budget. Single-flight via
// s.compactInFlight under s.mu: at most one goroutine is live, but
// Appends that arrive during compaction still get serviced via the
// re-check loop inside the goroutine.
func (s *Store) scheduleCompact() {
	s.mu.Lock()
	if !s.needCompactLocked() || s.compactInFlight {
		s.mu.Unlock()
		return
	}
	s.compactInFlight = true
	s.compactWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.compactWG.Done()
		for {
			if err := s.maybeCompact(); err != nil {
				logging.Error("[memory] background compaction failed: %v", err)
				// On error, exit the loop so we don't spin on a
				// persistent fault (full disk, dead summarizer).
				// The next Append re-triggers scheduleCompact, which
				// gives transient faults room to recover.
				s.mu.Lock()
				s.compactInFlight = false
				s.mu.Unlock()
				return
			}
			// maybeCompact brought us under budget; re-check under
			// the lock to catch any Append that slipped in during
			// the previous round. Still holding the latch so
			// concurrent Append.scheduleCompact calls short-circuit.
			s.mu.Lock()
			if s.needCompactLocked() {
				s.mu.Unlock()
				continue
			}
			s.compactInFlight = false
			s.mu.Unlock()
			return
		}
	}()
}

// needCompactLocked reports whether the recent buffer is over budget
// and can actually shed a turn (the newest turn is always kept).
// Caller must hold s.mu.
func (s *Store) needCompactLocked() bool {
	over := len(s.recent) > s.maxRecent || s.recentBytesLocked() > s.maxBytes
	return over && len(s.recent) > 1
}

// Compact forces compaction of all but the newest turn. Used by /compact.
//
// Waits for any background compaction to drain first so the synchronous
// loop doesn't race it for the same turn heads, then runs compactOldest
// directly (not via scheduleCompact) — callers of /compact want the
// work done before control returns.
func (s *Store) Compact() error {
	s.compactWG.Wait()
	for {
		s.mu.Lock()
		if len(s.recent) <= 1 {
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		if err := s.compactOldest(); err != nil {
			return err
		}
	}
}

// Clear wipes recent turns, the on-disk index, and the turns directory.
// Used by /clear. In the multi-instance case the wipe is shared:
// peers will see an empty MEMORY.md on their next read because Clear
// runs under the cross-process exclusive lock. Their per-process
// recent buffers are unaffected — Clear has no way to reach into
// another process's memory — so a peer will continue its current
// conversation thread until its own next /clear or restart.
//
// Waits for any in-flight background compaction first: compactOldest
// samples oldest outside the flock and then re-acquires it to append
// to MEMORY.md, so a Clear that interleaved between those two steps
// would be immediately followed by the goroutine re-appending a
// stale entry.
func (s *Store) Clear() error {
	s.compactWG.Wait()
	return s.withExclusiveLock(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.recent = nil
		s.index = nil
		if err := os.Remove(s.indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		entries, err := os.ReadDir(s.turnsDir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			_ = os.Remove(filepath.Join(s.turnsDir, e.Name()))
		}
		return nil
	})
}

// Recent returns a copy of the recent turn buffer.
func (s *Store) Recent() []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Turn, len(s.recent))
	copy(out, s.recent)
	return out
}

// Index returns a copy of the parsed MEMORY.md entries. The on-disk
// state is reloaded under the shared cross-process lock so callers
// see any entries written by sibling instances since the last call.
func (s *Store) Index() []IndexEntry {
	_ = s.withSharedLock(s.loadIndexLocked)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]IndexEntry, len(s.index))
	copy(out, s.index)
	return out
}

// BuildContext returns a prompt block describing prior conversation
// relevant to the current request. It includes:
//
//   - any IndexEntry whose keywords / entities overlap with the
//     current request, rendered as a single bullet with Topic +
//     LLM-generated Summary. Full turn text is never inlined —
//     callers who want the detail read turns/<id>.md directly.
//
//   - recent (uncompacted) turns verbatim, in order. Rendering cap
//     depends on the caller's BuildOpts.Kind: chitchat and pipeline
//     have distinct policies (see policyFor); the no-opts caller
//     gets oneLine truncation identical to the pre-session-32
//     behavior.
//
// The empty string is returned when there is no prior context.
//
// The compacted-index portion is reloaded from disk under the shared
// cross-process lock so a peer instance's compactions become visible
// to this instance immediately. Recent turns stay per-process — each
// REPL is its own conversation thread.
//
// BuildContext is variadic to preserve call-site compatibility with
// every existing test and caller. Passing zero opts yields the
// legacy retrieval policy (Kind="" in policyFor); passing a populated
// BuildOpts switches to the session-pinned / entity-weighted path.
func (s *Store) BuildContext(currentRequest string, optOverride ...BuildOpts) string {
	var opts BuildOpts
	if len(optOverride) > 0 {
		opts = optOverride[0]
	}
	policy := policyFor(opts.Kind)

	_ = s.withSharedLock(s.loadIndexLocked)
	s.mu.Lock()
	recent := append([]Turn(nil), s.recent...)
	idx := append([]IndexEntry(nil), s.index...)
	s.mu.Unlock()

	if len(recent) == 0 && len(idx) == 0 {
		return ""
	}

	var b strings.Builder
	if len(idx) > 0 {
		compactedCap := policy.compactedMatchCap
		if compactedCap < 0 {
			compactedCap = s.bcLimits.maxBuildContextMatches
		}
		matches := scoreIndex(idx, currentRequest, opts, policy)
		if len(matches) > compactedCap {
			matches = matches[:compactedCap]
		}
		if policy.refsChainDepth > 0 && len(matches) > 0 {
			matches = expandRefsChain(matches, idx, policy.refsChainDepth)
		}
		emitted := 0
		for _, e := range matches {
			// Defense in depth: WasError entries carry empty
			// Keywords / Entities, so scoreIndex should never
			// surface them. This guard covers hand-edited
			// MEMORY.md files and any future match rule that
			// doesn't key on keywords.
			if e.WasError {
				continue
			}
			if emitted == 0 {
				b.WriteString("### Relevant compacted memory\n")
			}
			fmt.Fprintf(&b, "- **%s** (%s) — %s\n", e.Topic, e.ID, e.Summary)
			emitted++
		}
		if emitted > 0 {
			b.WriteString("\n")
		}
	}
	if len(recent) > 0 {
		b.WriteString("### Recent conversation\n")
		renderRecentTurns(&b, recent, opts, policy)
	}
	return b.String()
}

// scoreIndex ranks compacted entries by keyword-hit + weighted
// entity-hit + optional same-session tie-breaker. Replaces the
// pre-session-32 matchIndex while remaining byte-compatible with
// its behavior when called with zero BuildOpts and legacy entries
// (no Entities, no SessionID) — the new contributions all collapse
// to zero in that case.
//
// Keyword matching is token-overlap (same as legacy). Entity
// matching is substring containment against the lower-cased request
// because entities often contain non-alphanumerics (file paths,
// dotted names) that tokenize would split. Entities under 3 runes
// are skipped to avoid spurious hits on sentinels like "go" / "id".
func scoreIndex(idx []IndexEntry, request string, opts BuildOpts, policy kindPolicy) []IndexEntry {
	if request == "" || len(idx) == 0 {
		return nil
	}
	tokens := tokenize(request)
	lowerReq := strings.ToLower(request)
	if len(tokens) == 0 && lowerReq == "" {
		return nil
	}
	type scored struct {
		e     IndexEntry
		score int
	}
	var out []scored
	for _, e := range idx {
		kwHits := 0
		for _, kw := range e.Keywords {
			if _, hit := tokens[strings.ToLower(kw)]; hit {
				kwHits++
			}
		}
		entHits := 0
		if len(e.Entities) > 0 && policy.entityScoreMul > 0 {
			for _, ent := range e.Entities {
				le := strings.ToLower(strings.TrimSpace(ent))
				if len([]rune(le)) < 3 {
					continue
				}
				if strings.Contains(lowerReq, le) {
					entHits++
				}
			}
		}
		s := kwHits + entHits*policy.entityScoreMul
		// Same-session tie-breaker. Only applies when the entry
		// already has some relevance signal (s > 0); bare session
		// membership is not enough to surface an otherwise-
		// irrelevant entry.
		if s > 0 && opts.SessionID != "" && e.SessionID == opts.SessionID {
			s++
		}
		if s > 0 {
			out = append(out, scored{e, s})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	result := make([]IndexEntry, len(out))
	for i, v := range out {
		result[i] = v.e
	}
	return result
}

// expandRefsChain walks IndexEntry.Refs from the primary match list
// up to depth hops, appending any target entries that were not
// already in the primary list. Primary matches stay first; each hop
// appends in discovery order. Cycles are broken by the `seen` set.
func expandRefsChain(primary []IndexEntry, idx []IndexEntry, depth int) []IndexEntry {
	byID := make(map[string]IndexEntry, len(idx))
	for _, e := range idx {
		byID[e.ID] = e
	}
	seen := make(map[string]bool, len(primary))
	out := make([]IndexEntry, 0, len(primary))
	for _, e := range primary {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	frontier := append([]IndexEntry(nil), out...)
	for hop := 0; hop < depth; hop++ {
		var next []IndexEntry
		for _, e := range frontier {
			for _, ref := range e.Refs {
				ref = strings.TrimSpace(ref)
				if ref == "" || seen[ref] {
					continue
				}
				target, ok := byID[ref]
				if !ok {
					continue
				}
				seen[ref] = true
				out = append(out, target)
				next = append(next, target)
			}
		}
		if len(next) == 0 {
			break
		}
		frontier = next
	}
	return out
}

// renderRecentTurns emits the "### Recent conversation" bullet list.
// Turns whose SessionID matches opts.SessionID are considered in-
// thread; up to policy.sessionPinCount of the most recent in-thread
// turns get rendered with policy.recentBodyChars body cap (so a
// chitchat /chat follow-up sees the prior exchange with enough
// context to stay coherent). All other turns use oneLine collapse —
// which is also the branch every no-opts caller hits, keeping
// legacy output byte-identical.
func renderRecentTurns(b *strings.Builder, recent []Turn, opts BuildOpts, policy kindPolicy) {
	pinned := make([]bool, len(recent))
	if opts.SessionID != "" && policy.sessionPinCount > 0 && policy.recentBodyChars > 0 {
		// Walk newest → oldest so we pin the most recent in-thread
		// turns; emit in forward order to keep chronology.
		pinLeft := policy.sessionPinCount
		for i := len(recent) - 1; i >= 0 && pinLeft > 0; i-- {
			if recent[i].SessionID == opts.SessionID {
				pinned[i] = true
				pinLeft--
			}
		}
	}
	for i, t := range recent {
		var reqLine, respLine string
		if pinned[i] {
			reqLine = capBody(t.Request, policy.recentBodyChars)
			respLine = sanitizeErrorResponse(capBody(t.Response, policy.recentBodyChars))
		} else {
			reqLine = oneLine(t.Request)
			// Read-side heuristic: sanitize error-laden responses
			// from legacy turn files. Matches are replaced with
			// the same placeholder the write-side emits so old and
			// new turn files present the same shape in prompts.
			respLine = sanitizeErrorResponse(oneLine(t.Response))
		}
		fmt.Fprintf(b, "- You (%s): %s\n", t.Timestamp.Format("15:04:05"), reqLine)
		fmt.Fprintf(b, "  Codrax: %s\n", respLine)
	}
}

// capBody truncates text on a UTF-8 rune boundary at maxRunes,
// appending an ellipsis when truncation occurred, and collapses
// embedded newlines to spaces so the rendered bullet stays single-
// line. Falls back to oneLine when maxRunes <= 0.
func capBody(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return oneLine(s)
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// maybeCompact triggers compactOldest until the recent buffer is back
// within both maxRecent and maxBytes budgets.
func (s *Store) maybeCompact() error {
	for {
		s.mu.Lock()
		over := len(s.recent) > s.maxRecent || s.recentBytesLocked() > s.maxBytes
		// Always keep the most recent turn intact.
		if !over || len(s.recent) <= 1 {
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		if err := s.compactOldest(); err != nil {
			return err
		}
	}
}

func (s *Store) recentBytesLocked() int {
	n := 0
	for _, t := range s.recent {
		n += len(t.Request) + len(t.Response) + len(t.RequestForSummary)
	}
	return n
}

// compactOldest pops recent[0], summarizes it, and appends the result
// to MEMORY.md. The summarizer is called outside any lock so a slow
// LLM call does not block other store operations or block peers in
// the multi-instance scenario.
//
// The disk write and the in-memory index update are paired under the
// cross-process exclusive lock. We deliberately mutate s.index AFTER
// the disk append succeeds so a failed write does not leave the
// in-memory cache claiming an entry that was never persisted. The
// reload-on-acquire inside withExclusiveLock ensures s.index reflects
// any peer's writes that committed since this process last ran a
// memory operation, so the local append never silently shadows them.
func (s *Store) compactOldest() error {
	s.mu.Lock()
	if len(s.recent) == 0 {
		s.mu.Unlock()
		return nil
	}
	oldest := s.recent[0]
	s.mu.Unlock()

	var entry IndexEntry
	// Short-circuit error-laden turns BEFORE calling the LLM
	// summarizer. Otherwise the summarizer reads raw pipeline
	// failure text ("error: task X stuck at stage explore after 5
	// visits", OpenAI 400 JSON) and produces natural-language
	// descriptions like "hit a 5-visit oscillation" that evade
	// the read-side sanitizeErrorResponse phrase list. Detection
	// reuses the structural check from sanitizeErrorResponse so
	// the write-side placeholder (post-F4 turn files) and
	// raw-text legacy turn files (pre-F4, not yet compacted) are
	// both caught by the same path. Keywords intentionally left
	// empty so matchIndex never surfaces the entry — it stays
	// on disk as a historical marker only.
	if sanitizeErrorResponse(oldest.Response) == errorResponsePlaceholder {
		entry = IndexEntry{
			ID:        oldest.ID,
			Topic:     "(failed attempt)",
			Summary:   errorResponsePlaceholder,
			FullRef:   filepath.Join("turns", oldest.ID+".md"),
			WasError:  true,
			Kind:      oldest.Kind,
			SessionID: oldest.SessionID,
		}
	} else {
		if s.summarizer == nil {
			return errors.New("memory: no summarizer configured")
		}
		// Swap in the expanded-paste form for summarization only.
		// Request (display) is what persists; RequestForSummary is
		// the source of keyword/topic signal when paste folding hid
		// the actual content from what we store.
		summarizeInput := oldest
		if oldest.RequestForSummary != "" {
			summarizeInput.Request = oldest.RequestForSummary
		}
		var err error
		entry, err = s.summarizer.Summarize(context.Background(), summarizeInput)
		if err != nil {
			return fmt.Errorf("summarize turn %s: %w", oldest.ID, err)
		}
		if entry.ID == "" {
			entry.ID = oldest.ID
		}
		if entry.FullRef == "" {
			entry.FullRef = filepath.Join("turns", oldest.ID+".md")
		}
		// Propagate session-32 routing metadata from the turn so the
		// summarizer implementation doesn't have to care about these
		// plumbing fields. Empty values (legacy or pre-upgrade turns)
		// stay empty — the kind-aware retrieval paths degrade to the
		// legacy behavior in that case.
		if entry.Kind == "" {
			entry.Kind = oldest.Kind
		}
		if entry.SessionID == "" {
			entry.SessionID = oldest.SessionID
		}
	}

	return s.withExclusiveLock(func() error {
		if err := s.appendIndexEntry(entry); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.index = append(s.index, entry)
		// Pop the compacted turn from recent. We re-check the head
		// instead of blindly slicing because another goroutine in
		// the same process could (in theory) have already trimmed
		// it; in practice maybeCompact serializes via this same
		// flock, but defensiveness here is cheap.
		if len(s.recent) > 0 && s.recent[0].ID == oldest.ID {
			s.recent = s.recent[1:]
		}
		return nil
	})
}

// writeTurnFile dumps a turn to turns/<id>.md.
//
// Session-32 metadata (Kind / SessionID) is emitted as HTML comments
// between the timestamp and the Request section. HTML comments are
// invisible when the markdown is rendered, don't disturb the existing
// reqMarker / respMarker anchors that readTurnFile uses, and a legacy
// file missing the comments round-trips with empty metadata so the
// orphan-recovery path degrades gracefully.
func (s *Store) writeTurnFile(turn Turn) error {
	path := filepath.Join(s.turnsDir, turn.ID+".md")
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", turn.ID)
	fmt.Fprintf(&b, "_%s_\n\n", turn.Timestamp.Format(time.RFC3339))
	if turn.Kind != "" {
		fmt.Fprintf(&b, "<!-- kind: %s -->\n", turn.Kind)
	}
	if turn.SessionID != "" {
		fmt.Fprintf(&b, "<!-- session_id: %s -->\n", turn.SessionID)
	}
	if turn.Kind != "" || turn.SessionID != "" {
		b.WriteString("\n")
	}
	b.WriteString("## Request\n\n")
	b.WriteString(turn.Request)
	b.WriteString("\n\n## Response\n\n")
	b.WriteString(turn.Response)
	b.WriteString("\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// appendIndexEntry appends an IndexEntry to MEMORY.md using the
// frontmatter format documented at the top of the package.
//
// Session-32 fields (kind / session_id / entities / refs) are emitted
// only when non-empty — an all-zero optional block keeps MEMORY.md
// bytes identical to what the pre-upgrade writer produced, so a
// MEMORY.md authored by either vintage round-trips cleanly through
// the other's parser.
func (s *Store) appendIndexEntry(e IndexEntry) error {
	f, err := os.OpenFile(s.indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "---")
	fmt.Fprintf(w, "id: %s\n", e.ID)
	fmt.Fprintf(w, "topic: %q\n", e.Topic)
	fmt.Fprintf(w, "keywords: [%s]\n", strings.Join(e.Keywords, ", "))
	fmt.Fprintf(w, "full_ref: %s\n", e.FullRef)
	if e.WasError {
		fmt.Fprintln(w, "was_error: true")
	}
	if e.Kind != "" {
		fmt.Fprintf(w, "kind: %s\n", e.Kind)
	}
	if e.SessionID != "" {
		fmt.Fprintf(w, "session_id: %s\n", e.SessionID)
	}
	if len(e.Entities) > 0 {
		fmt.Fprintf(w, "entities: [%s]\n", strings.Join(e.Entities, ", "))
	}
	if len(e.Refs) > 0 {
		fmt.Fprintf(w, "refs: [%s]\n", strings.Join(e.Refs, ", "))
	}
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w, e.Summary)
	fmt.Fprintln(w)
	return w.Flush()
}

// loadIndexLocked parses MEMORY.md from disk into s.index. The
// "Locked" suffix is a contract reminder: callers must already hold
// the cross-process flock (shared or exclusive) so the file is not
// being mutated mid-read. Missing file is not an error — a fresh
// memory dir simply has no entries yet. The in-process s.mu is taken
// briefly to publish the parsed slice without tearing concurrent
// readers in the same process.
func (s *Store) loadIndexLocked() error {
	data, err := os.ReadFile(s.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		s.mu.Lock()
		s.index = nil
		s.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	parsed := parseIndex(string(data))
	s.mu.Lock()
	s.index = parsed
	s.mu.Unlock()
	return nil
}

// parseIndex is a tiny frontmatter parser tailored to the format
// produced by appendIndexEntry. It tolerates extra whitespace and
// unknown keys but expects each entry to be delimited by `---` lines.
func parseIndex(text string) []IndexEntry {
	var out []IndexEntry
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) != "---" {
			i++
			continue
		}
		i++ // past opening ---
		entry := IndexEntry{}
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			line := lines[i]
			i++
			k, v, ok := splitKV(line)
			if !ok {
				continue
			}
			switch k {
			case "id":
				entry.ID = strings.Trim(v, `"`)
			case "topic":
				entry.Topic = strings.Trim(v, `"`)
			case "keywords":
				entry.Keywords = parseYAMLList(v)
			case "full_ref":
				entry.FullRef = strings.TrimSpace(v)
			case "was_error":
				entry.WasError = strings.EqualFold(strings.TrimSpace(v), "true")
			case "kind":
				entry.Kind = Kind(strings.TrimSpace(v))
			case "session_id":
				entry.SessionID = strings.TrimSpace(v)
			case "entities":
				entry.Entities = parseYAMLList(v)
			case "refs":
				entry.Refs = parseYAMLList(v)
			}
		}
		i++ // past closing ---
		// Body until blank line or next ---
		var body strings.Builder
		for i < len(lines) && strings.TrimSpace(lines[i]) != "---" {
			body.WriteString(lines[i])
			body.WriteString("\n")
			i++
		}
		entry.Summary = strings.TrimSpace(body.String())
		if entry.ID != "" {
			out = append(out, entry)
		}
	}
	return out
}

func splitKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// parseYAMLList parses the `[a, b, c]` shorthand used by
// appendIndexEntry for keywords / entities / refs. Missing brackets
// and trailing whitespace are tolerated, empty entries are dropped,
// and items are returned in file order so the entity weighting in
// BuildContext stays deterministic.
func parseYAMLList(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimSuffix(v, "]"), "[")
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func tokenize(s string) map[string]struct{} {
	out := map[string]struct{}{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out[strings.ToLower(cur.String())] = struct{}{}
			cur.Reset()
		}
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// errorResponsePlaceholder is the identical replacement emitted by
// both the write-side (repl.recordTurn) and the read-side
// (BuildContext's Recent conversation section) for error-laden
// pipeline outputs. Keeping the two sides in lock-step means
// legacy turn files on disk (pre-fix) and new turn files
// (post-fix) both render the same way in memory prompts.
const errorResponsePlaceholder = "(previous attempt ended in error — details omitted from memory)"

// sanitizeErrorResponse returns a clean placeholder when the input
// looks like an error-laden pipeline response, otherwise passes it
// through unchanged. The detection is purely structural:
//
//  1. Already-sanitized placeholder → passes through (idempotent).
//  2. Leading "error:" prefix — the format that
//     internal/render.RenderResult emits when
//     busCtx.TaskState.LastError is non-empty.
//  3. Any line containing specific internal-failure phrases:
//     - "stuck at stage ... after N visits" (oscillation guard)
//     - "context_length_exceeded" (OpenAI 400 on context window)
//     - `{"error":{"code":` (raw OpenAI error payload)
//
// The phrase list is intentionally short and concrete — each entry
// names an internal failure mode that has no business leaking into
// the LLM's prior-conversation context. Legitimate answers that
// legitimately mention the word "error" (e.g. "the error-handling
// code lives at ...") do NOT match any of these patterns.
func sanitizeErrorResponse(s string) string {
	if s == "" {
		return s
	}
	if s == errorResponsePlaceholder {
		return s
	}
	// The render layer emits "\n  error: <LastError>\n" which oneLine
	// collapses to "error: <LastError>" with possible leading spaces.
	trimmed := strings.TrimLeft(s, " \t")
	if strings.HasPrefix(trimmed, "error:") {
		return errorResponsePlaceholder
	}
	// Defense in depth: signature phrases that identify specific
	// internal failure text anywhere in the response, even if the
	// "error:" prefix was stripped by upstream formatting.
	for _, sig := range []string{
		"stuck at stage",
		"oscillation guard tripped",
		"context_length_exceeded",
		`{"error":{"code":`,
	} {
		if strings.Contains(s, sig) {
			return errorResponsePlaceholder
		}
	}
	return s
}
