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
// It is safe for concurrent use.
type Store struct {
	mu sync.Mutex

	dir       string
	turnsDir  string
	indexPath string

	recent []Turn
	index  []IndexEntry

	maxRecent  int
	maxBytes   int
	summarizer Summarizer
}

// NewStore creates (or opens) a Store rooted at dir. The directory and
// its turns/ subdir are created if missing. Existing MEMORY.md entries
// are parsed back into the in-memory index so the next run carries over
// prior compacted history.
//
// Recent uncompacted turns are NOT reloaded — they live in memory only
// for the duration of a REPL session. This keeps the index file as the
// single source of long-lived truth.
func NewStore(dir string, summarizer Summarizer) (*Store, error) {
	if dir == "" {
		dir = "memory"
	}
	turnsDir := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turnsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}
	s := &Store{
		dir:        dir,
		turnsDir:   turnsDir,
		indexPath:  filepath.Join(dir, "MEMORY.md"),
		maxRecent:  6,
		maxBytes:   20 * 1024,
		summarizer: summarizer,
	}
	if err := s.loadIndex(); err != nil {
		return nil, err
	}
	return s, nil
}

// Append records a new turn. The full text is written to turns/<id>.md
// immediately. If the recent buffer is over budget, the oldest turn is
// compacted into MEMORY.md via the summarizer.
func (s *Store) Append(turn Turn) error {
	if turn.ID == "" {
		turn.ID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
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
// Used by /clear.
func (s *Store) Clear() error {
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
}

// Recent returns a copy of the recent turn buffer.
func (s *Store) Recent() []Turn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Turn, len(s.recent))
	copy(out, s.recent)
	return out
}

// Index returns a copy of the parsed MEMORY.md entries.
func (s *Store) Index() []IndexEntry {
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
func (s *Store) BuildContext(currentRequest string) string {
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
// to MEMORY.md. The summarizer is called outside the lock so a slow LLM
// call does not block other store operations.
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

	s.mu.Lock()
	s.recent = s.recent[1:]
	s.index = append(s.index, entry)
	s.mu.Unlock()

	return s.appendIndexEntry(entry)
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

// loadIndex parses an existing MEMORY.md back into s.index. Missing
// file is not an error.
func (s *Store) loadIndex() error {
	data, err := os.ReadFile(s.indexPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	s.index = parseIndex(string(data))
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
