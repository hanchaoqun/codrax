package types

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// ScopedReadCoverage identifies the source window behind a demoted read
// suggestion. RepositoryRoot and Path are producer-resolved identities, not
// a model's display label. Empty LineRanges means a whole-file suggestion.
type ScopedReadCoverage struct {
	RepositoryRoot string
	Path           string
	LineRanges     []LineRange
}

type scopedReadCoverageIdentity struct {
	repositoryRoot string
	path           string
}

type scopedReadCoverageDemand struct {
	wholeFile bool
	ranges    []LineRange
}

type scopedReadCoverageObservation struct {
	ranges     []LineRange
	totalLines int
}

// This run-local display state is deliberately separate from readSet and
// readRanges. Those historical carriers support relative aliases and do not
// bind each observation to a repository; neither behavior can clear a scoped
// display warning. A source-less legacy caveat remains independently unknown.
type scopedReadCoverageState struct {
	unknown bool
	demands map[scopedReadCoverageIdentity]scopedReadCoverageDemand
	reads   map[scopedReadCoverageIdentity]scopedReadCoverageObservation
}

// AppendScopedReadCoverageCaveat records display-only read debt without
// changing the historical completion boundary used by scheduling gates.
func (c *EvidenceClosure) AppendScopedReadCoverageCaveat(caveat CompletionCaveat, scopes []ScopedReadCoverage) {
	if c == nil || caveat.Lane != DowngradeLaneForcedReadCoverage {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// A source-less pre-upgrade in-memory record must not acquire a fabricated
	// scope when a newer producer later reports a different, known file.
	c.scopedReadCoverage.unknown = c.forcedReadCoverageSourceUnknownLocked()
	c.appendCompletionCaveatLocked(caveat)
	if len(scopes) == 0 {
		c.scopedReadCoverage.unknown = true
	}
	for _, scope := range scopes {
		identity, ok := scopedReadCoverageSourceIdentity(scope.RepositoryRoot, scope.Path)
		if !ok || !validScopedReadCoverageRanges(scope.LineRanges) {
			c.scopedReadCoverage.unknown = true
			continue
		}
		c.scopedReadCoverage.addDemand(identity, scopedReadCoverageDemand{
			wholeFile: len(scope.LineRanges) == 0,
			ranges:    scope.LineRanges,
		})
	}
}

// RecordScopedReadCoverage is called only at a successful current-source
// read_file exit. It does not grant evidence, source-role, or gate authority.
func (c *EvidenceClosure) RecordScopedReadCoverage(repositoryRoot string, coverage ToolReadCoverage) {
	if c == nil || strings.TrimSpace(coverage.RawRef) == "" || coverage.LineStart <= 0 ||
		coverage.LineEnd < coverage.LineStart || coverage.TotalLines < 0 ||
		(coverage.TotalLines > 0 && coverage.LineEnd > coverage.TotalLines) {
		return
	}
	identity, ok := scopedReadCoverageSourceIdentity(repositoryRoot, coverage.Path)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scopedReadCoverage.addObservation(identity, scopedReadCoverageObservation{
		ranges:     []LineRange{{Start: coverage.LineStart, End: coverage.LineEnd}},
		totalLines: coverage.TotalLines,
	})
}

// CurrentCompletionCaveats is the current display/hint view. Historical
// CompletionCaveats remains the monotonic completion-boundary authority.
func (c *EvidenceClosure) CurrentCompletionCaveats() []CompletionCaveat {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []CompletionCaveat
	for _, caveat := range c.completionCaveats {
		if caveat.Lane == DowngradeLaneForcedReadCoverage && c.scopedReadCoverage.resolved() {
			continue
		}
		out = append(out, caveat)
	}
	return out
}

func scopedReadCoverageSourceIdentity(repositoryRoot, source string) (scopedReadCoverageIdentity, bool) {
	if strings.ContainsAny(repositoryRoot+source, "\x00\r\n") {
		return scopedReadCoverageIdentity{}, false
	}
	// Do not resolve a missing/relative repository against process cwd. The
	// source producer must supply its actual absolute repository identity.
	root := canonpath.CanonicalRepoRelative(strings.TrimSpace(repositoryRoot), "")
	if !evidenceClosureLooksAbsolutePath(root) {
		return scopedReadCoverageIdentity{}, false
	}
	// Relative spelling normalization is safe; directory/traversal forms and
	// unrelated absolute paths are not source identities. Literal glob-looking
	// filename bytes are allowed because no pattern matching is performed.
	raw := strings.TrimSpace(strings.ReplaceAll(source, `\`, "/"))
	if strings.HasSuffix(raw, "/") {
		return scopedReadCoverageIdentity{}, false
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return scopedReadCoverageIdentity{}, false
		}
	}
	path, ok := canonpath.CanonicalRepoRelativeIdentity(canonpath.CanonicalRepoRelative(raw, root))
	if !ok {
		return scopedReadCoverageIdentity{}, false
	}
	return scopedReadCoverageIdentity{repositoryRoot: root, path: path}, true
}

func validScopedReadCoverageRanges(ranges []LineRange) bool {
	for _, r := range ranges {
		if r.Start <= 0 || r.End < r.Start {
			return false
		}
	}
	return true
}

func (s *scopedReadCoverageState) addDemand(identity scopedReadCoverageIdentity, demand scopedReadCoverageDemand) {
	if s.demands == nil {
		s.demands = make(map[scopedReadCoverageIdentity]scopedReadCoverageDemand)
	}
	current := s.demands[identity]
	current.wholeFile = current.wholeFile || demand.wholeFile
	current.ranges = mergeLineRanges(append(cloneLineRanges(current.ranges), demand.ranges...))
	s.demands[identity] = current
}

func (s *scopedReadCoverageState) addObservation(identity scopedReadCoverageIdentity, observed scopedReadCoverageObservation) {
	if s.reads == nil {
		s.reads = make(map[scopedReadCoverageIdentity]scopedReadCoverageObservation)
	}
	current := s.reads[identity]
	current.ranges = mergeLineRanges(append(cloneLineRanges(current.ranges), observed.ranges...))
	if observed.totalLines > current.totalLines {
		current.totalLines = observed.totalLines
	}
	s.reads[identity] = current
}

func (s scopedReadCoverageState) resolved() bool {
	if s.unknown || len(s.demands) == 0 {
		return false
	}
	for identity, demand := range s.demands {
		observed, ok := s.reads[identity]
		if !ok {
			return false
		}
		if demand.wholeFile && (observed.totalLines <= 0 ||
			!scopedReadCoverageRangeCovered(observed, LineRange{Start: 1, End: observed.totalLines})) {
			return false
		}
		for _, r := range demand.ranges {
			if !scopedReadCoverageRangeCovered(observed, r) {
				return false
			}
		}
	}
	return true
}

func scopedReadCoverageRangeCovered(observed scopedReadCoverageObservation, want LineRange) bool {
	if want.Start <= 0 || want.End < want.Start {
		return false
	}
	if observed.totalLines > 0 && want.End > observed.totalLines {
		want.End = observed.totalLines
	}
	if want.End < want.Start {
		return false // an entirely out-of-file requested region is not proof
	}
	next := want.Start
	for _, r := range observed.ranges {
		if r.End < next {
			continue
		}
		if r.Start > next {
			return false
		}
		if r.End >= want.End {
			return true
		}
		next = r.End + 1
	}
	return false
}

func (s scopedReadCoverageState) clone() scopedReadCoverageState {
	var out scopedReadCoverageState
	out.merge(s)
	return out
}

func (s *scopedReadCoverageState) merge(other scopedReadCoverageState) {
	s.unknown = s.unknown || other.unknown
	for identity, demand := range other.demands {
		s.addDemand(identity, demand)
	}
	for identity, observed := range other.reads {
		s.addObservation(identity, observed)
	}
}

func (c *EvidenceClosure) cloneForcedReadCoverageLocked(out *EvidenceClosure) {
	out.scopedReadCoverage = c.scopedReadCoverage.clone()
	out.scopedReadCoverage.unknown = c.forcedReadCoverageSourceUnknownLocked()
	for _, caveat := range c.completionCaveats {
		if caveat.Lane == DowngradeLaneForcedReadCoverage {
			out.appendCompletionCaveatLocked(caveat)
		}
	}
}

func (c *EvidenceClosure) mergeForcedReadCoverageLocked(other *EvidenceClosure) {
	c.scopedReadCoverage.unknown = c.forcedReadCoverageSourceUnknownLocked()
	c.scopedReadCoverage.merge(other.scopedReadCoverage)
	for _, caveat := range other.completionCaveats {
		if caveat.Lane == DowngradeLaneForcedReadCoverage {
			c.appendCompletionCaveatLocked(caveat)
		}
	}
}

// Identify a historical source-less record before any new demand is merged.
// Both the parent and the fork can predate scoped producers; neither may borrow
// the other's known source identity to resolve its independent unknown.
func (c *EvidenceClosure) forcedReadCoverageSourceUnknownLocked() bool {
	if c.scopedReadCoverage.unknown {
		return true
	}
	if len(c.scopedReadCoverage.demands) == 0 {
		for _, caveat := range c.completionCaveats {
			if caveat.Lane == DowngradeLaneForcedReadCoverage {
				return true
			}
		}
	}
	return false
}
