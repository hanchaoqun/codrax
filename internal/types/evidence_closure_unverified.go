package types

import (
	"github.com/hanchaoqun/codrax/internal/logging"
)

// evidence_closure_unverified.go carries the UnverifiedFinding lifecycle
// (E6): record, deterministic clear-on-read, and the typed demotion escape
// lane used by the C2 completion-denial breaker. Split out of
// evidence_closure.go per the IR delivery hot-file ratchet; the closure
// fields and the path canonicaliser / readAliasLocked helpers stay in
// evidence_closure.go.
//
// Deliberately NO round-based age demotion: both producers mark findings at
// analyze time (round 0), so any "rounds since marked" clock reduces to
// "rounds into exploration" and would expire the anti-hallucination gate
// unconditionally — the adversarial review of the first E6 cut confirmed
// that failure. Degradation is owned by the C2 denial breaker alone, which
// demotes only after repeated identical no-progress denials and attaches a
// completion gate note.

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
	// Advisory marks a finding demoted by the C2 completion-denial
	// breaker: once true it stops hard-blocking completion /
	// exact-resolution surfaces and renders only as soft prompt
	// guidance. Monotonic — never un-set. Demotion-by-flag instead of
	// REMOVAL is deliberate: removal would shrink
	// ComputeDowngradeBlockerKey without real progress and reset the
	// contract_chain converger mid-stall, while the one-way flag
	// changes the key exactly once.
	Advisory bool `json:"advisory,omitempty"`
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

// dropVerifiedUnverifiedFindsLocked removes Kind=="path" findings whose
// canonicalized Token resolves into readSet via readAliasLocked — the
// file provably exists because a read landed on it, so the "file does
// not exist" mark is disproven by a precise boolean signal. One-way
// removal: a later SetReadSet snapshot without the file must NOT
// resurrect the finding. Symbol-kind entries are untouched — a file
// read proves path existence, not symbol presence. Caller MUST hold
// c.mu (write).
//
// Clearing the mark cannot weaken the exact-resolution floor: the
// exact_resolved_defining_proof gate still independently requires a
// grounded defining proof via evidenceHasAnyDefiningExactTargetProof;
// the clear only stops a stale "file does not exist" assertion from
// blocking evidence that provably reads the file.
func (c *EvidenceClosure) dropVerifiedUnverifiedFindsLocked() int {
	if len(c.unverifiedFinds) == 0 || len(c.readSet) == 0 {
		return 0
	}
	kept := c.unverifiedFinds[:0]
	dropped := 0
	for _, u := range c.unverifiedFinds {
		if u.Kind == "path" {
			if canon := c.canonicalize(u.Token); canon != "" {
				if _, ok := c.readAliasLocked(canon); ok {
					dropped++
					logging.Info("[CGEC] I1 unverified finding cleared by read verification: kind=path token=%s", u.Token)
					continue
				}
			}
		}
		kept = append(kept, u)
	}
	c.unverifiedFinds = kept
	return dropped
}

// dropGroundedSymbolUnverifiedFindsLocked removes Kind=="symbol"
// findings whose Token verbatim-equals the AnchorSymbol of a grounded
// accepted-evidence ref — the symbol provably resolved, disproving
// "symbol not found in graph" with a typed signal. Same one-way
// semantics as the path clear. Caller MUST hold c.mu (write).
func (c *EvidenceClosure) dropGroundedSymbolUnverifiedFindsLocked() int {
	if len(c.unverifiedFinds) == 0 || len(c.acceptedEvidence) == 0 {
		return 0
	}
	hasSymbol := false
	for _, u := range c.unverifiedFinds {
		if u.Kind == "symbol" {
			hasSymbol = true
			break
		}
	}
	if !hasSymbol {
		return 0
	}
	grounded := make(map[string]bool)
	for _, ref := range c.acceptedEvidence {
		if ref.GroundingStatus == GroundingGrounded && ref.AnchorSymbol != "" {
			grounded[ref.AnchorSymbol] = true
		}
	}
	if len(grounded) == 0 {
		return 0
	}
	kept := c.unverifiedFinds[:0]
	dropped := 0
	for _, u := range c.unverifiedFinds {
		if u.Kind == "symbol" && grounded[u.Token] {
			dropped++
			logging.Info("[CGEC] I1 unverified finding cleared by grounded evidence anchor: kind=symbol token=%s", u.Token)
			continue
		}
		kept = append(kept, u)
	}
	c.unverifiedFinds = kept
	return dropped
}

// DrainVerifiedUnverifiedFindings removes every path-kind unverified
// finding the closure can prove exists via read coverage. Mirrors
// DrainSatisfiedPendingReads for gate-site calls so the C2 pre-complete
// gate and the downgrade blocker key evaluate post-drain state within
// the same dispatch. Returns the count of cleared findings.
func (c *EvidenceClosure) DrainVerifiedUnverifiedFindings() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropVerifiedUnverifiedFindsLocked()
}

// DemoteUnverifiedFindingsToAdvisory flips Advisory on the findings
// matching (kind, token) verbatim. It is the typed escape lane the C2
// unverified-path denial breaker uses so subsequent completion attempts
// and the exact-resolution consumers stop hard-blocking on them.
// Monotonic — the flag is never un-set. Returns the number of findings
// demoted by THIS call.
func (c *EvidenceClosure) DemoteUnverifiedFindingsToAdvisory(kind string, tokens ...string) int {
	if c == nil || len(tokens) == 0 {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	demoted := 0
	for _, token := range tokens {
		for i := range c.unverifiedFinds {
			if c.unverifiedFinds[i].Advisory {
				continue
			}
			if c.unverifiedFinds[i].Kind == kind && c.unverifiedFinds[i].Token == token {
				c.unverifiedFinds[i].Advisory = true
				demoted++
			}
		}
	}
	return demoted
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
