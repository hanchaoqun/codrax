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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Turn is one user request + assembled assistant response.
type Turn struct {
	ID        string
	Request   string
	Response  string
	Timestamp time.Time
}

// IndexEntry is a compacted summary of a Turn that lives in MEMORY.md.
// FullRef points at the original turn file under turns/.
type IndexEntry struct {
	ID       string
	Topic    string
	Keywords []string
	Summary  string
	FullRef  string
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
	summarizer Summarizer
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
func NewStore(dir string, summarizer Summarizer) (*Store, error) {
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
	sidecar := filepath.Join(dir, fmt.Sprintf("%s%d-%d", sidecarPrefix, os.Getpid(), time.Now().UnixNano()))
	if f, err := os.OpenFile(sidecar, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644); err == nil {
		_ = f.Close()
	} else if !errors.Is(err, os.ErrExist) {
		_ = instanceLock.close()
		_ = flock.close()
		return nil, fmt.Errorf("create instance sidecar: %w", err)
	}
	s := &Store{
		dir:          dir,
		turnsDir:     turnsDir,
		indexPath:    filepath.Join(dir, "MEMORY.md"),
		lockPath:     lockPath,
		flock:        flock,
		instanceLock: instanceLock,
		sidecarPath:  sidecar,
		maxRecent:    6,
		maxBytes:     20 * 1024,
		summarizer:   summarizer,
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

	return Turn{
		ID:        id,
		Request:   request,
		Response:  response,
		Timestamp: ts,
	}, nil
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
	if err := s.writeTurnFile(turn); err != nil {
		return err
	}

	s.mu.Lock()
	s.recent = append(s.recent, turn)
	s.mu.Unlock()

	return s.maybeCompact()
}

// Compact forces compaction of all but the newest turn. Used by /compact.
func (s *Store) Compact() error {
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
func (s *Store) Clear() error {
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
//   - all recent (uncompacted) turns verbatim, in order;
//   - any IndexEntry whose keywords overlap with the current request,
//     each followed by the inlined contents of its full_ref turn file
//     so the model can recover the original detail.
//
// The empty string is returned when there is no prior context.
//
// The compacted-index portion is reloaded from disk under the shared
// cross-process lock so a peer instance's compactions become visible
// to this instance immediately. Recent turns stay per-process — each
// REPL is its own conversation thread.
func (s *Store) BuildContext(currentRequest string) string {
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
		matches := matchIndex(idx, currentRequest)
		if len(matches) > 0 {
			b.WriteString("### Relevant compacted memory\n")
			for _, e := range matches {
				fmt.Fprintf(&b, "- **%s** (%s) — %s\n", e.Topic, e.ID, e.Summary)
				if full, err := os.ReadFile(filepath.Join(s.dir, e.FullRef)); err == nil {
					b.WriteString("  Full turn:\n  ")
					b.WriteString(strings.ReplaceAll(string(full), "\n", "\n  "))
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}
	}
	if len(recent) > 0 {
		b.WriteString("### Recent conversation\n")
		for _, t := range recent {
			fmt.Fprintf(&b, "- You (%s): %s\n", t.Timestamp.Format("15:04:05"), oneLine(t.Request))
			fmt.Fprintf(&b, "  Codrax: %s\n", oneLine(t.Response))
		}
	}
	return b.String()
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
		n += len(t.Request) + len(t.Response)
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

	if s.summarizer == nil {
		return errors.New("memory: no summarizer configured")
	}
	entry, err := s.summarizer.Summarize(context.Background(), oldest)
	if err != nil {
		return fmt.Errorf("summarize turn %s: %w", oldest.ID, err)
	}
	if entry.ID == "" {
		entry.ID = oldest.ID
	}
	if entry.FullRef == "" {
		entry.FullRef = filepath.Join("turns", oldest.ID+".md")
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
func (s *Store) writeTurnFile(turn Turn) error {
	path := filepath.Join(s.turnsDir, turn.ID+".md")
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", turn.ID)
	fmt.Fprintf(&b, "_%s_\n\n", turn.Timestamp.Format(time.RFC3339))
	b.WriteString("## Request\n\n")
	b.WriteString(turn.Request)
	b.WriteString("\n\n## Response\n\n")
	b.WriteString(turn.Response)
	b.WriteString("\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// appendIndexEntry appends an IndexEntry to MEMORY.md using the
// frontmatter format documented at the top of the package.
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
				v = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(v), "]"), "[")
				for _, kw := range strings.Split(v, ",") {
					if k := strings.TrimSpace(kw); k != "" {
						entry.Keywords = append(entry.Keywords, k)
					}
				}
			case "full_ref":
				entry.FullRef = strings.TrimSpace(v)
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

// matchIndex returns entries whose keywords overlap with words in the
// current request. Matching is case-insensitive whole-word containment;
// matches are sorted by overlap count descending.
func matchIndex(idx []IndexEntry, request string) []IndexEntry {
	if request == "" {
		return nil
	}
	tokens := tokenize(request)
	if len(tokens) == 0 {
		return nil
	}
	type scored struct {
		e     IndexEntry
		score int
	}
	var scoredOut []scored
	for _, e := range idx {
		s := 0
		for _, kw := range e.Keywords {
			lk := strings.ToLower(kw)
			if _, hit := tokens[lk]; hit {
				s++
			}
		}
		if s > 0 {
			scoredOut = append(scoredOut, scored{e, s})
		}
	}
	sort.SliceStable(scoredOut, func(i, j int) bool { return scoredOut[i].score > scoredOut[j].score })
	out := make([]IndexEntry, len(scoredOut))
	for i, s := range scoredOut {
		out[i] = s.e
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
