package operation

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultOperationMemoryMaxEntries = 200
	defaultOperationMemoryTTL        = 30 * 24 * time.Hour
)

// MemoryEntry is a compact historical operation lesson. It deliberately stores
// summaries and payload refs, not full command output.
type MemoryEntry struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Workspace      string    `json:"workspace,omitempty"`
	OS             string    `json:"os,omitempty"`
	Arch           string    `json:"arch,omitempty"`
	Shell          string    `json:"shell,omitempty"`
	Capability     string    `json:"capability,omitempty"`
	Command        string    `json:"command,omitempty"`
	Args           []string  `json:"args,omitempty"`
	Outcome        string    `json:"outcome,omitempty"`
	FailureClass   string    `json:"failure_class,omitempty"`
	OutputKind     string    `json:"output_kind,omitempty"`
	OutputLines    int       `json:"output_lines,omitempty"`
	OutputBytes    int       `json:"output_bytes,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	PayloadRefs    []string  `json:"payload_refs,omitempty"`
	Lessons        []string  `json:"lessons,omitempty"`
	EnvFingerprint string    `json:"env_fingerprint,omitempty"`
}

// MemoryStore persists operation lessons as JSONL under .codrax/operation.
type MemoryStore struct {
	Path       string
	MaxEntries int
	TTL        time.Duration
}

func NewMemoryStore(path string) *MemoryStore {
	return &MemoryStore{
		Path:       strings.TrimSpace(path),
		MaxEntries: defaultOperationMemoryMaxEntries,
		TTL:        defaultOperationMemoryTTL,
	}
}

func (s *MemoryStore) Append(entries ...MemoryEntry) error {
	if s == nil || strings.TrimSpace(s.Path) == "" || len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	now := time.Now().UTC()
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		entry = normalizeMemoryEntry(entry, now, s.TTL)
		if entry.Summary == "" && len(entry.Lessons) == 0 {
			continue
		}
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return s.prune()
}

func (s *MemoryStore) RecentMatches(snapshot CapabilitySnapshot, limit int) ([]MemoryEntry, error) {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 4
	}
	entries, err := s.readAll()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	type scored struct {
		entry MemoryEntry
		score int
	}
	var scoredEntries []scored
	for _, entry := range entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			continue
		}
		score := scoreMemoryEntry(entry, snapshot)
		if score <= 0 {
			continue
		}
		scoredEntries = append(scoredEntries, scored{entry: entry, score: score})
	}
	sort.SliceStable(scoredEntries, func(i, j int) bool {
		if scoredEntries[i].score == scoredEntries[j].score {
			return scoredEntries[i].entry.CreatedAt.After(scoredEntries[j].entry.CreatedAt)
		}
		return scoredEntries[i].score > scoredEntries[j].score
	})
	if len(scoredEntries) > limit {
		scoredEntries = scoredEntries[:limit]
	}
	out := make([]MemoryEntry, 0, len(scoredEntries))
	for _, item := range scoredEntries {
		out = append(out, item.entry)
	}
	return out, nil
}

func RenderMemoryForPrompt(entries []MemoryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## operation_memory\n")
	b.WriteString("Historical command-operation lessons. Use only as soft guidance; they are not current-source evidence and may be stale.\n")
	for i, entry := range entries {
		if i >= 6 {
			break
		}
		fmt.Fprintf(&b, "- outcome=%s command=%q", dash(entry.Outcome), oneLine(entry.Command, 180))
		if entry.FailureClass != "" {
			fmt.Fprintf(&b, " failure_class=%s", entry.FailureClass)
		}
		if entry.OutputKind != "" {
			fmt.Fprintf(&b, " output_kind=%s", entry.OutputKind)
		}
		if entry.OS != "" || entry.Arch != "" {
			fmt.Fprintf(&b, " env=%s/%s", dash(entry.OS), dash(entry.Arch))
		}
		if len(entry.Lessons) > 0 {
			fmt.Fprintf(&b, " lesson=%q", oneLine(strings.Join(entry.Lessons, "; "), 260))
		} else if entry.Summary != "" {
			fmt.Fprintf(&b, " summary=%q", oneLine(entry.Summary, 260))
		}
		if len(entry.PayloadRefs) > 0 {
			fmt.Fprintf(&b, " payload_ref=%s", entry.PayloadRefs[0])
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func BuildMemoryEntries(plan CommandOperationPlan, result CommandOperationResult, snapshot CapabilitySnapshot) []MemoryEntry {
	stepByID := map[string]CommandStep{}
	for _, step := range plan.Steps {
		stepByID[step.ID] = step
	}
	now := time.Now().UTC()
	var entries []MemoryEntry
	for _, stepResult := range result.StepResults {
		step := stepByID[stepResult.StepID]
		command := commandDisplay(step)
		outputSummary := SummarizeStepOutput(stepResult)
		entry := MemoryEntry{
			CreatedAt:      now,
			Workspace:      snapshot.RepoRoot,
			OS:             snapshot.OS,
			Arch:           snapshot.Arch,
			Shell:          snapshot.Shell,
			Capability:     "computer_operation",
			Command:        command,
			Args:           append([]string(nil), step.Args...),
			Outcome:        string(stepResult.Status),
			FailureClass:   stepResult.FailureClass,
			OutputKind:     outputSummary.Kind,
			OutputLines:    outputSummary.Lines,
			OutputBytes:    outputSummary.Bytes,
			Summary:        outputSummary.Summary,
			PayloadRefs:    cleanRefs([]string{stepResult.PayloadRef}),
			Lessons:        lessonsForStepResult(stepResult, command),
			EnvFingerprint: envFingerprint(snapshot),
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *MemoryStore) readAll() ([]MemoryEntry, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []MemoryEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var entry MemoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		out = append(out, entry)
	}
	return out, sc.Err()
}

func (s *MemoryStore) prune() error {
	if s.MaxEntries <= 0 {
		s.MaxEntries = defaultOperationMemoryMaxEntries
	}
	entries, err := s.readAll()
	if err != nil || len(entries) <= s.MaxEntries {
		return err
	}
	now := time.Now().UTC()
	var kept []MemoryEntry
	for _, entry := range entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) > s.MaxEntries {
		kept = kept[len(kept)-s.MaxEntries:]
	}
	tmp := s.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, entry := range kept {
		if err := enc.Encode(entry); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func normalizeMemoryEntry(entry MemoryEntry, now time.Time, ttl time.Duration) MemoryEntry {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if ttl <= 0 {
		ttl = defaultOperationMemoryTTL
	}
	if entry.ExpiresAt.IsZero() {
		entry.ExpiresAt = entry.CreatedAt.Add(ttl)
	}
	entry.Workspace = strings.TrimSpace(entry.Workspace)
	entry.OS = strings.TrimSpace(entry.OS)
	entry.Arch = strings.TrimSpace(entry.Arch)
	entry.Shell = strings.TrimSpace(entry.Shell)
	entry.Capability = strings.TrimSpace(entry.Capability)
	entry.Command = strings.TrimSpace(entry.Command)
	entry.Outcome = strings.TrimSpace(entry.Outcome)
	entry.FailureClass = strings.TrimSpace(entry.FailureClass)
	entry.OutputKind = strings.TrimSpace(entry.OutputKind)
	entry.Summary = oneLine(entry.Summary, 500)
	entry.PayloadRefs = cleanRefs(entry.PayloadRefs)
	entry.Lessons = cleanLessons(entry.Lessons)
	entry.EnvFingerprint = strings.TrimSpace(entry.EnvFingerprint)
	if entry.ID == "" {
		sum := sha256.Sum256([]byte(entry.CreatedAt.Format(time.RFC3339Nano) + "\x00" + entry.Workspace + "\x00" + entry.Command + "\x00" + entry.Outcome))
		entry.ID = "opmem-" + hex.EncodeToString(sum[:6])
	}
	return entry
}

func scoreMemoryEntry(entry MemoryEntry, snapshot CapabilitySnapshot) int {
	score := 0
	if snapshot.RepoRoot != "" && entry.Workspace == snapshot.RepoRoot {
		score += 4
	}
	if snapshot.OS != "" && entry.OS == snapshot.OS {
		score += 2
	}
	if snapshot.Arch != "" && entry.Arch == snapshot.Arch {
		score += 1
	}
	if score == 0 {
		return 0
	}
	if entry.Capability == "computer_operation" {
		score += 1
	}
	if entry.Outcome != "" {
		score += 1
	}
	return score
}

func lessonsForStepResult(step CommandStepResult, command string) []string {
	switch step.FailureClass {
	case "command_not_found":
		return []string{"The command was not available in PATH; prefer an installed alternative or an explicit install/check plan."}
	case "permission_denied":
		return []string{"The command failed with permission denied; avoid retrying unchanged and ask for privilege/scope clarification."}
	case "timeout":
		return []string{"The command timed out; use a narrower query, bounded output, or a longer explicit timeout only with approval."}
	case "path_not_found":
		return []string{"The referenced path was missing; verify the path before repeating the command."}
	}
	if step.Status == StatusExecuted {
		if command != "" {
			return []string{"This command completed successfully in a prior operation context."}
		}
		return []string{"A prior operation step completed successfully."}
	}
	return nil
}

func commandDisplay(step CommandStep) string {
	if strings.TrimSpace(step.Shell) != "" {
		return strings.TrimSpace(step.Shell)
	}
	return strings.TrimSpace(strings.TrimSpace(step.Program) + " " + strings.Join(step.Args, " "))
}

func envFingerprint(snapshot CapabilitySnapshot) string {
	sum := sha256.Sum256([]byte(snapshot.RepoRoot + "\x00" + snapshot.OS + "\x00" + snapshot.Arch + "\x00" + snapshot.Shell))
	return hex.EncodeToString(sum[:8])
}

func cleanRefs(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func cleanLessons(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = oneLine(value, 260)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if max > 0 && len([]rune(s)) > max {
		runes := []rune(s)
		s = string(runes[:max]) + "..."
	}
	return s
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
