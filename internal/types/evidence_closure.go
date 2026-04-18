package types

import (
	"hash/fnv"
	"sort"
	"strings"
	"sync"
)

// EvidenceClosure is the per-Run cross-stage tracker for the four
// CGEC (Citation-Grounded Evidence Closure) invariants:
//
//   I1: every file:line surfaced in a downstream prompt must be in
//       ReadSet (chain promotion enforces this — chains anchored
//       outside ReadSet get demoted to PendingReads).
//
//   I2: every emit_*-accepted citation must be in ReadSet
//       (emit_answer_document grounder writes RepairDirectives
//       describing the gap when a citation is rejected).
//
//   I3: emit_investigation_complete may not flip the flag unless the
//       contract checker would pass on the current EvidenceItems +
//       ReadSet snapshot (pre-complete simulation lives in
//       emit_investigation_complete.Execute).
//
//   I4: between two retries at least one of (ReadSet, EvidenceCount,
//       ChainTermSet) must change — otherwise the orchestrator is
//       in a stall and triggers Lazy Auto-Read.
//
// EvidenceClosure is the single source of truth that all four
// enforcers read from / write to. Lives on MutableState so every
// stage shares the same view. The orchestrator runTaskGraph calls
// MutableState.ResetEvidenceClosure at task entry so cross-task
// contamination is impossible (mirror of ResetTurnAArtifacts /
// ResetAnswerDocument lifecycle).
//
// Concurrency: EvidenceClosure has its own mutex because individual
// fields are written from independent code paths (chain producer,
// grounder, scheduler) and a single MutableState mutex would force
// them to serialize. Internal locking; callers go through methods.
type EvidenceClosure struct {
	mu sync.RWMutex

	// readSet is the canonical set of repo-relative file paths Turn A
	// successfully fetched via read_file (or the framework forced-read
	// via Lazy Auto-Read). Mirror of TurnAArtifacts.ReadFiles but
	// kept current per-explore-window so the chain producer and the
	// pre-complete check can read it without round-tripping through
	// extractFileCoverage on the full tool history.
	readSet map[string]bool

	// scannedSet is the broader set of files the deterministic
	// concrete-value scanner consulted from disk (keyword-scored
	// files + symbol-definition files mentioned in investigation
	// notes). Used by the chain promotion enforcer to distinguish
	// "scanned but unread" (PendingRead candidate) from "indexed
	// only" (no-op).
	scannedSet map[string]bool

	// citedRefs records every (file, lineNumber) that has appeared
	// in an emit_*-accepted citation pool so far. Lets the
	// convergence detector spot "the LLM keeps citing the same line"
	// signatures. Map: file → sorted unique line numbers.
	citedRefs map[string][]int

	// pendingReads is the queue of files the framework has decided
	// MUST be read before emit_investigation_complete can succeed.
	// Populated by chain promotion (anchor outside ReadSet) and by
	// emit_answer_document grounder (citation in unread file). The
	// orchestrator surfaces these to the next explore round's prompt
	// as a "Forced Read List" and falls back to Lazy Auto-Read when
	// the LLM still skips them after a configurable budget.
	pendingReads []PendingRead

	// unverifiedFinds is the list of analyzer-emitted file paths /
	// symbol identifiers that the findings validator could not match
	// against the repo. Surfaced to downstream stages as a hygiene
	// warning (rendered with strikethrough + ⚠️) so the extractor
	// and finalizer do not bake hallucinated artefacts into the
	// answer.
	unverifiedFinds []UnverifiedFinding

	// subjectMatches caches the subject.Score result per chain
	// summary so the chain ranker (called multiple times across
	// dedup + cap + render) does not re-compute the heuristic. Key
	// is the chain summary string; value is the score in [0, 1].
	subjectMatches map[string]float64

	// fingerprints is the rolling history of ClosureFingerprint
	// values, one entry per explore round. The convergence detector
	// compares the latest two entries to spot stalls (no progress
	// across a retry) and three-in-a-row to force-finalize.
	fingerprints []ClosureFingerprint

	// repairs is the queue of structured RepairDirective values that
	// downstream enforcers (grounder, pre-complete check, stall
	// detector) emit when a contract violation needs to be surfaced
	// to the next explore round. Drained by ConsumeRepairs at retry-
	// hint render time so each directive fires exactly once.
	repairs []RepairDirective

	// stats accumulates CGEC enforcer fire counters across the
	// current task. Each enforcer increments its own field
	// (chain promotion → ChainsDemoted, findings_validator →
	// UnverifiedFinds, grounder → RepairsRaised, pre-complete →
	// PreCompleteDowngrades, runForcedReads → ForcedReads,
	// detectStallAndAct → StallSoftHits / StallHardHits).
	// Read by the orchestrator at task-end to emit a one-line
	// summary; reset to zero by Reset() on per-task entry.
	stats ClosureStats
}

// ClosureStats is the structured per-task counter snapshot the
// orchestrator emits after each task so operators / eval harnesses
// can grep one line for invariant fire counts. All fields default
// to zero; counters increment via the corresponding Bump method
// below, which threads through the closure mutex so concurrent
// emit_*-tool calls cannot race.
//
// ExpandSearchRaised and ShapeSwapRaised are separate from the
// generic RepairsRaised counter because Session 10 Group B wires
// them as first-class producers with their own semantics — operator
// observability treats "the framework asked the LLM to grep wider"
// and "the framework asked the LLM to swap answer shape" as
// distinct signals worth their own column in the task summary.
type ClosureStats struct {
	ChainsDemoted         int // I1: chains stripped from prompt by chain promotion
	UnverifiedFinds       int // I1: hallucinated paths/symbols flagged by findings_validator
	RepairsRaised         int // I2: structured RepairDirectives written to closure (any kind)
	ExpandSearchRaised    int // B1: RepairExpandSearch directives raised (Phase0 / stall / preComplete)
	ShapeSwapRaised       int // B2: RepairSwapShape directives raised (grounder / preComplete / retry)
	PreCompleteDowngrades int // I3: emit_investigation_complete downgrade events
	ForcedReads           int // I4: files read on the LLM's behalf (Lazy Auto-Read)
	StallSoftHits         int // I4: convergence soft threshold hits
	StallHardHits         int // I4: convergence hard threshold hits (force-complete)
}

// HasActivity returns true when at least one enforcer fired this
// task. Used by the orchestrator's emit gate so a no-op task does
// not pollute the trace with an empty summary.
func (s ClosureStats) HasActivity() bool {
	return s.ChainsDemoted+s.UnverifiedFinds+s.RepairsRaised+
		s.ExpandSearchRaised+s.ShapeSwapRaised+
		s.PreCompleteDowngrades+s.ForcedReads+
		s.StallSoftHits+s.StallHardHits > 0
}

// PendingRead is one entry in EvidenceClosure.pendingReads. Anchor is
// the file we want to bring into ReadSet; Rationale is the prose the
// retry hint will render so the LLM understands WHY (e.g. "chain X
// anchors here but file unread"); Origin is a short tag of which
// enforcer raised it ("chain_promotion", "grounder_reject",
// "subject_constraint") so we can attribute / debug.
type PendingRead struct {
	File      string
	Rationale string
	Origin    string
}

// UnverifiedFinding is one entry in EvidenceClosure.unverifiedFinds.
// Token is the literal text from the analyzer's report (a path or
// `backtick-quoted` symbol); Kind is "path" or "symbol"; Reason is
// the validator's diagnosis ("file does not exist in repo", "symbol
// not found in graph"). The findings_validator emits these and the
// renderer in context/builder.go decorates them at prompt-build time.
type UnverifiedFinding struct {
	Token  string
	Kind   string // "path" | "symbol"
	Reason string
}

// ClosureFingerprint is the snapshot used by the convergence detector
// (I4 enforcer). Two adjacent fingerprints with all four fields
// equal means an explore round did not change ReadSet, did not
// produce new evidence, did not surface new chain terminals, AND
// did not pull a new (file, line) into the citation pool —
// retrying produces the same output and the orchestrator must
// intervene (Lazy Auto-Read, force-finalize). The hashes use FNV-32
// because the values are short keys and we only compare equality.
//
// CitedRefsHash is the fourth dimension added in G7. Without it the
// convergence detector cannot spot the "LLM keeps citing the same
// (file, line) tuples across retries" pattern that emit_answer_document
// surfaces. ReadSet/Evidence/ChainTerm collectively miss that signal:
// the LLM might not read new files but might re-cite existing ones
// with different quote text — that's still a stall.
type ClosureFingerprint struct {
	ReadSetHash   uint32
	EvidenceHash  uint32
	ChainTermSet  uint32
	CitedRefsHash uint32
}

// NewEvidenceClosure constructs an empty closure. Callers should pin
// the returned pointer onto MutableState via SetEvidenceClosure once
// per Run.
func NewEvidenceClosure() *EvidenceClosure {
	return &EvidenceClosure{
		readSet:        make(map[string]bool),
		scannedSet:     make(map[string]bool),
		citedRefs:      make(map[string][]int),
		subjectMatches: make(map[string]float64),
	}
}

// SetReadSet atomically replaces the current ReadSet snapshot. Called
// by the explorer once per round after extractFileCoverage runs.
func (c *EvidenceClosure) SetReadSet(files map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSet = make(map[string]bool, len(files))
	for f, v := range files {
		if v {
			c.readSet[f] = true
		}
	}
}

// ReadSet returns a defensive copy of the current ReadSet so callers
// can iterate without holding the closure lock.
func (c *EvidenceClosure) ReadSet() map[string]bool {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]bool, len(c.readSet))
	for f := range c.readSet {
		out[f] = true
	}
	return out
}

// HasRead returns true when the named file is in ReadSet. Hot-path
// lookup used by the chain promotion enforcer for every chain.
func (c *EvidenceClosure) HasRead(file string) bool {
	if c == nil || file == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.readSet[file]
}

// SetScannedSet atomically replaces the ScannedSet snapshot.
func (c *EvidenceClosure) SetScannedSet(files map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scannedSet = make(map[string]bool, len(files))
	for f, v := range files {
		if v {
			c.scannedSet[f] = true
		}
	}
}

// AddPendingRead enqueues a forced-read directive. De-duplicates by
// File + Origin so two enforcers raising the same file do not
// double-render in the retry hint.
func (c *EvidenceClosure) AddPendingRead(p PendingRead) {
	if c == nil || p.File == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addPendingReadLocked(p)
}

// addPendingReadLocked is the lock-held helper shared by AddPendingRead
// and the A1 bridge inside AddRepair. Caller MUST hold c.mu.
func (c *EvidenceClosure) addPendingReadLocked(p PendingRead) {
	if p.File == "" {
		return
	}
	for _, existing := range c.pendingReads {
		if existing.File == p.File && existing.Origin == p.Origin {
			return
		}
	}
	c.pendingReads = append(c.pendingReads, p)
}

// PendingReads returns a defensive copy of the queue.
func (c *EvidenceClosure) PendingReads() []PendingRead {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.pendingReads) == 0 {
		return nil
	}
	out := make([]PendingRead, len(c.pendingReads))
	copy(out, c.pendingReads)
	return out
}

// ClearPendingReadFor removes every PendingRead whose File equals the
// argument. Called by the Lazy Auto-Read path after a forced read
// succeeds and by the explorer once it sees an LLM-driven read_file
// for the same path.
func (c *EvidenceClosure) ClearPendingReadFor(file string) {
	if c == nil || file == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.pendingReads[:0]
	for _, p := range c.pendingReads {
		if p.File == file {
			continue
		}
		kept = append(kept, p)
	}
	c.pendingReads = kept
}

// AppendUnverifiedFinding records one analyzer hallucination probe
// failure. Idempotent: same Token+Kind tuple is recorded once.
func (c *EvidenceClosure) AppendUnverifiedFinding(u UnverifiedFinding) {
	if c == nil || u.Token == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.unverifiedFinds {
		if existing.Token == u.Token && existing.Kind == u.Kind {
			return
		}
	}
	c.unverifiedFinds = append(c.unverifiedFinds, u)
}

// UnverifiedFindings returns a defensive copy.
func (c *EvidenceClosure) UnverifiedFindings() []UnverifiedFinding {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.unverifiedFinds) == 0 {
		return nil
	}
	out := make([]UnverifiedFinding, len(c.unverifiedFinds))
	copy(out, c.unverifiedFinds)
	return out
}

// RecordCitation records a (file, line) pair that an emit_*-accepted
// citation pool referenced. Used by the convergence detector and by
// the pre-complete simulator's "do we have any cite-eligible
// evidence" check.
func (c *EvidenceClosure) RecordCitation(file string, line int) {
	if c == nil || file == "" || line <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.citedRefs == nil {
		c.citedRefs = make(map[string][]int)
	}
	for _, existing := range c.citedRefs[file] {
		if existing == line {
			return
		}
	}
	c.citedRefs[file] = append(c.citedRefs[file], line)
}

// CitedRefs returns a copy of the file → lines map.
func (c *EvidenceClosure) CitedRefs() map[string][]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string][]int, len(c.citedRefs))
	for f, lines := range c.citedRefs {
		dup := make([]int, len(lines))
		copy(dup, lines)
		out[f] = dup
	}
	return out
}

// SetSubjectMatch caches a chain → subject score lookup.
func (c *EvidenceClosure) SetSubjectMatch(chain string, score float64) {
	if c == nil || chain == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subjectMatches == nil {
		c.subjectMatches = make(map[string]float64)
	}
	c.subjectMatches[chain] = score
}

// SubjectMatch returns the cached score for chain (or 0 + false).
func (c *EvidenceClosure) SubjectMatch(chain string) (float64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.subjectMatches[chain]
	return v, ok
}

// AppendFingerprint records a per-round closure snapshot. Returns the
// updated history length so the convergence detector can decide
// whether to compare adjacent entries this round.
func (c *EvidenceClosure) AppendFingerprint(fp ClosureFingerprint) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fingerprints = append(c.fingerprints, fp)
	return len(c.fingerprints)
}

// Fingerprints returns a defensive copy of the history.
func (c *EvidenceClosure) Fingerprints() []ClosureFingerprint {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.fingerprints) == 0 {
		return nil
	}
	out := make([]ClosureFingerprint, len(c.fingerprints))
	copy(out, c.fingerprints)
	return out
}

// AddRepair enqueues a RepairDirective. De-duplicates by Kind +
// sorted-Files + Subject so repeated firings of the same enforcer do
// not double-render the retry hint. Increments stats.RepairsRaised
// on every NEW (non-deduplicated) directive so the orchestrator's
// task-end summary reports a single accurate count regardless of
// caller (emit_answer_document, pre-complete, stall detector all
// flow through this single chokepoint).
//
// A1 bridge: every RepairReadFile directive simultaneously writes
// its Files onto the PendingReads queue. Without this mirror, the
// grounder's citation-drop feedback would ONLY reach the retry hint
// prompt, where an inattentive LLM can ignore or mis-read it. The
// mirror lets runForcedReads (I4) pick the same files up from the
// PendingReads queue so the framework can READ the files on the
// LLM's behalf as a fallback. ConsumeRepairs drains the Repairs
// queue but does NOT clear the mirrored PendingReads — they stay
// until runForcedReads consumes them (ClearPendingReadFor) or the
// LLM reads the file naturally (marked done via SetReadSet +
// ClearPendingReadFor from runForcedReads).
func (c *EvidenceClosure) AddRepair(r RepairDirective) {
	if c == nil || r.Kind == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.repairs {
		if existing.Kind == r.Kind && existing.Subject == r.Subject && sameFileSet(existing.Files, r.Files) {
			return
		}
	}
	c.repairs = append(c.repairs, r)
	c.stats.RepairsRaised++
	// Per-kind counter bumps: each RepairKind has its own column in
	// the CGEC summary line so operators can tell at a glance which
	// enforcer family is driving retries. New kinds added to
	// RepairKind MUST add a case here so TestAllRepairKindsHaveProducer
	// does not regress.
	switch r.Kind {
	case RepairReadFile:
		origin := "auto_bridge"
		if r.Origin != "" {
			origin = "auto_bridge." + r.Origin
		}
		for _, f := range r.Files {
			c.addPendingReadLocked(PendingRead{
				File:      f,
				Rationale: r.Rationale,
				Origin:    origin,
			})
		}
	case RepairExpandSearch:
		c.stats.ExpandSearchRaised++
	case RepairSwapShape:
		c.stats.ShapeSwapRaised++
	}
}

// PendingRepairs returns a defensive copy of the queue WITHOUT
// draining it. Use ConsumeRepairs to drain.
func (c *EvidenceClosure) PendingRepairs() []RepairDirective {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.repairs) == 0 {
		return nil
	}
	out := make([]RepairDirective, len(c.repairs))
	copy(out, c.repairs)
	return out
}

// ConsumeRepairs returns and clears the queue in one atomic step. The
// retry-hint renderer calls this so each directive surfaces in
// exactly one prompt.
func (c *EvidenceClosure) ConsumeRepairs() []RepairDirective {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.repairs) == 0 {
		return nil
	}
	out := c.repairs
	c.repairs = nil
	return out
}

// Reset wipes the closure back to NewEvidenceClosure() state. Called
// by MutableState.ResetEvidenceClosure at task entry.
func (c *EvidenceClosure) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readSet = make(map[string]bool)
	c.scannedSet = make(map[string]bool)
	c.citedRefs = make(map[string][]int)
	c.pendingReads = nil
	c.unverifiedFinds = nil
	c.subjectMatches = make(map[string]float64)
	c.fingerprints = nil
	c.repairs = nil
	c.stats = ClosureStats{}
}

// Stats returns a snapshot of the per-task counters. Cheap (struct
// copy under read lock) so callers can poll across enforcer
// invocations.
func (c *EvidenceClosure) Stats() ClosureStats {
	if c == nil {
		return ClosureStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// BumpChainsDemoted / BumpUnverifiedFinds / etc. are typed counter
// increments. Each takes a delta (always positive in practice) so
// batch operations (e.g. one applyChainPromotion call demoting N
// chains) increment in one mutex acquire.

func (c *EvidenceClosure) BumpChainsDemoted(n int)         { c.bumpStat(func(s *ClosureStats) { s.ChainsDemoted += n }) }
func (c *EvidenceClosure) BumpUnverifiedFinds(n int)       { c.bumpStat(func(s *ClosureStats) { s.UnverifiedFinds += n }) }
func (c *EvidenceClosure) BumpRepairsRaised(n int)         { c.bumpStat(func(s *ClosureStats) { s.RepairsRaised += n }) }
func (c *EvidenceClosure) BumpExpandSearchRaised(n int)    { c.bumpStat(func(s *ClosureStats) { s.ExpandSearchRaised += n }) }
func (c *EvidenceClosure) BumpShapeSwapRaised(n int)       { c.bumpStat(func(s *ClosureStats) { s.ShapeSwapRaised += n }) }
func (c *EvidenceClosure) BumpPreCompleteDowngrades(n int) { c.bumpStat(func(s *ClosureStats) { s.PreCompleteDowngrades += n }) }
func (c *EvidenceClosure) BumpForcedReads(n int)           { c.bumpStat(func(s *ClosureStats) { s.ForcedReads += n }) }
func (c *EvidenceClosure) BumpStallSoftHits(n int)         { c.bumpStat(func(s *ClosureStats) { s.StallSoftHits += n }) }
func (c *EvidenceClosure) BumpStallHardHits(n int)         { c.bumpStat(func(s *ClosureStats) { s.StallHardHits += n }) }

func (c *EvidenceClosure) bumpStat(mut func(*ClosureStats)) {
	if c == nil || mut == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	mut(&c.stats)
}

// HashFileSet computes the FNV-32 hash of a sorted file set. Helper
// used by ClosureFingerprint construction so the convergence detector
// hashes are stable across iterator order.
func HashFileSet(files map[string]bool) uint32 {
	keys := make([]string, 0, len(files))
	for f, v := range files {
		if v {
			keys = append(keys, f)
		}
	}
	sort.Strings(keys)
	h := fnv.New32a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum32()
}

// HashStringSet hashes a string slice independent of order.
func HashStringSet(values []string) uint32 {
	dup := make([]string, len(values))
	copy(dup, values)
	sort.Strings(dup)
	h := fnv.New32a()
	for _, v := range dup {
		_, _ = h.Write([]byte(v))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum32()
}

// sameFileSet returns true when a and b contain the same file paths
// regardless of order.
func sameFileSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// MutableState accessors below — they live in this file (instead of
// context.go) so all EvidenceClosure-aware code is co-located. Go
// allows methods on a type defined in any file of the same package.

// EvidenceClosure returns the per-Run closure tracker. Lazily
// initialized: if the field is nil the first reader installs a fresh
// instance, so callers never have to check for nil before use. The
// orchestrator's runTaskGraph calls ResetEvidenceClosure at task
// entry to guarantee cross-task isolation; tests that bypass the
// orchestrator can rely on lazy init.
func (m *MutableState) EvidenceClosure() *EvidenceClosure {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	if m.evidenceClosure != nil {
		c := m.evidenceClosure
		m.mu.RUnlock()
		return c
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.evidenceClosure == nil {
		m.evidenceClosure = NewEvidenceClosure()
	}
	return m.evidenceClosure
}

// SetEvidenceClosure atomically replaces the closure pointer. Pass
// nil to clear (the next EvidenceClosure() call will lazy-init).
func (m *MutableState) SetEvidenceClosure(c *EvidenceClosure) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evidenceClosure = c
}

// ResetEvidenceClosure clears the closure at task entry. Mirror of
// ResetTurnAArtifacts / ResetAnswerDocument: the per-task scheduler
// loop calls this so per-task closures cannot bleed.
func (m *MutableState) ResetEvidenceClosure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.evidenceClosure != nil {
		m.evidenceClosure.Reset()
	}
}

// canonicalizeRepoPath is a tiny helper kept here so callers do not
// have to import filepath. Returns the input with any leading "./"
// stripped and trailing whitespace trimmed. Symmetric with the
// canonicaliser in internal/tool/ground/path.go but does not touch
// repo-root resolution (closure stores repo-relative paths only).
func canonicalizeRepoPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "./") {
		p = p[2:]
	}
	return p
}

// CanonicalReadFiles is a convenience helper that returns the same
// data as ReadSet but as a sorted slice of canonical paths. Useful
// for prompt rendering and for stable test assertions.
func (c *EvidenceClosure) CanonicalReadFiles() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.readSet))
	for f := range c.readSet {
		if f == "" {
			continue
		}
		out = append(out, canonicalizeRepoPath(f))
	}
	sort.Strings(out)
	return out
}
