package types

import (
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

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

func cloneBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneNodeExecStatusMap(in map[string]NodeExecStatus) map[string]NodeExecStatus {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]NodeExecStatus, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = NormalizeNodeExecStatus(v)
	}
	return out
}

func cloneNodeExecAttemptMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" || v <= 0 {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneIntSliceMap(in map[string][]int) map[string][]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]int, len(in))
	for k, v := range in {
		out[k] = append([]int(nil), v...)
	}
	return out
}

func cloneLineRangeMap(in map[string][]LineRange) map[string][]LineRange {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]LineRange, len(in))
	for k, v := range in {
		out[k] = cloneLineRanges(v)
	}
	return out
}

func clonePendingReads(in []PendingRead) []PendingRead {
	if len(in) == 0 {
		return nil
	}
	out := make([]PendingRead, len(in))
	for i, p := range in {
		out[i] = p
		out[i].LineRanges = cloneLineRanges(p.LineRanges)
	}
	return out
}

// ClearPendingReadsMatching removes only the concrete pending-read entries that
// a precise completion preflight has reclassified as non-blocking for the
// current accepted boundary. It intentionally matches on the pending-read
// identity (canonical file + origin + surgical line ranges) instead of on a
// broad origin class so principal reads from the same producer keep carrying
// weight.
func (c *EvidenceClosure) ClearPendingReadsMatching(entries ...PendingRead) int {
	if c == nil || len(entries) == 0 {
		return 0
	}
	wanted := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if canon := c.canonicalize(entry.File); canon != "" {
			entry.File = canon
		}
		if key := pendingReadIdentityKey(entry); key != "" {
			wanted[key] = true
		}
	}
	if len(wanted) == 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pendingReads) == 0 {
		return 0
	}
	kept := c.pendingReads[:0]
	cleared := 0
	for _, pending := range c.pendingReads {
		if wanted[pendingReadIdentityKey(pending)] {
			cleared++
			continue
		}
		kept = append(kept, pending)
	}
	c.pendingReads = kept
	return cleared
}

func pendingReadIdentityKey(p PendingRead) string {
	file := strings.TrimSpace(p.File)
	origin := NormalizePendingReadOrigin(p.Origin)
	if file == "" || origin == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(file)
	b.WriteByte('\x00')
	b.WriteString(origin)
	for _, rng := range p.LineRanges {
		if rng.Start <= 0 && rng.End <= 0 {
			continue
		}
		b.WriteByte('\x00')
		b.WriteString(fmtLineRangeIdentity(rng))
	}
	return b.String()
}

func fmtLineRangeIdentity(rng LineRange) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(rng.Start))
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(rng.End))
	return b.String()
}

func cloneRepairDirectives(in []RepairDirective) []RepairDirective {
	if len(in) == 0 {
		return nil
	}
	out := make([]RepairDirective, len(in))
	for i, r := range in {
		out[i] = NormalizeRepairDirective(r)
	}
	return out
}

func cloneViolations(in []Violation) []Violation {
	if len(in) == 0 {
		return nil
	}
	out := make([]Violation, len(in))
	for i, v := range in {
		out[i] = v
		out[i].EvidenceRefs = append([]string(nil), v.EvidenceRefs...)
	}
	return out
}

func cloneClosureStats(in ClosureStats) ClosureStats {
	out := in
	if len(in.PerStage) > 0 {
		out.PerStage = make(map[string]StageStats, len(in.PerStage))
		for k, v := range in.PerStage {
			out.PerStage[k] = v
		}
	}
	return out
}

func addClosureStats(a, b ClosureStats) ClosureStats {
	out := a
	out.ChainsDemoted += b.ChainsDemoted
	out.UnverifiedFinds += b.UnverifiedFinds
	out.RepairsRaised += b.RepairsRaised
	out.ExpandSearchRaised += b.ExpandSearchRaised
	out.ViewSwapRaised += b.ViewSwapRaised
	out.PreCompleteDowngrades += b.PreCompleteDowngrades
	out.ForcedReads += b.ForcedReads
	out.StallSoftHits += b.StallSoftHits
	out.StallHardHits += b.StallHardHits
	out.ViolationsLogged += b.ViolationsLogged
	out.AutoPairedRoleDescriptions += b.AutoPairedRoleDescriptions
	if len(b.PerStage) > 0 {
		if out.PerStage == nil {
			out.PerStage = make(map[string]StageStats, len(b.PerStage))
		}
		for stage, st := range b.PerStage {
			cur := out.PerStage[stage]
			cur.Retries += st.Retries
			cur.Violations += st.Violations
			cur.Repairs += st.Repairs
			cur.Contradictions += st.Contradictions
			cur.PendingReads += st.PendingReads
			cur.ForcedReads += st.ForcedReads
			out.PerStage[stage] = cur
		}
	}
	return out
}

func clampMergeBaseLen(base, n int) int {
	if base < 0 {
		return 0
	}
	if base > n {
		return n
	}
	return base
}

func mergeUnverifiedFindings(existing, incoming []UnverifiedFinding) []UnverifiedFinding {
	if len(existing) == 0 {
		return append([]UnverifiedFinding(nil), incoming...)
	}
	out := append([]UnverifiedFinding(nil), existing...)
	index := make(map[string]int, len(out)+len(incoming))
	for i, u := range out {
		if u.Token == "" {
			continue
		}
		index[u.Token+"\x00"+u.Kind] = i
	}
	for _, u := range incoming {
		if u.Token == "" {
			continue
		}
		key := u.Token + "\x00" + u.Kind
		if i, ok := index[key]; ok {
			// Advisory ORs (monotonic — a fork's demotion survives the
			// merge).
			out[i].Advisory = out[i].Advisory || u.Advisory
			continue
		}
		index[key] = len(out)
		out = append(out, u)
	}
	return out
}

func mergeSortedUniqueInts(a, b []int) []int {
	if len(a) == 0 {
		out := append([]int(nil), b...)
		sort.Ints(out)
		return uniqueSortedInts(out)
	}
	if len(b) == 0 {
		out := append([]int(nil), a...)
		sort.Ints(out)
		return uniqueSortedInts(out)
	}
	out := append(append([]int(nil), a...), b...)
	sort.Ints(out)
	return uniqueSortedInts(out)
}

func uniqueSortedInts(in []int) []int {
	if len(in) <= 1 {
		return in
	}
	out := in[:0]
	last := 0
	for i, v := range in {
		if i == 0 || v != last {
			out = append(out, v)
			last = v
		}
	}
	return out
}

// MutableState accessors below — they live with EvidenceClosure helpers
// so EvidenceClosure-aware code remains co-located. Go allows methods on
// a type defined in any file of the same package.

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
		m.evidenceClosure = NewEvidenceClosure(m.repoRoot)
		if observation := sourceInventoryObservationFromMutableFields(m.sourceInventoryObservation, m.turnAArtifacts); observation.IsActive() {
			m.evidenceClosure.IngestEvidenceReducerInput(EvidenceReducerInput{
				Class:                      EvidenceReducerInputSourceInventoryObservation,
				SourceInventoryObservation: observation,
			}, m.repoRoot)
		}
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
//
// The backslash -> slash conversion is unconditional rather than
// going through filepath.ToSlash because that helper is a no-op on
// Linux (separator is already '/'), so Windows-shaped paths emitted
// by a Windows binary or held in a cross-OS test fixture would
// silently slip through and miss every map lookup keyed on the
// slash form.
func canonicalizeRepoPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.HasPrefix(p, "./") {
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
		out = append(out, c.canonicalize(f))
	}
	sort.Strings(out)
	return out
}
