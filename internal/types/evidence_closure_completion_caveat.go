package types

// ClearRepairsByDowngradeLane drops only repair directives owned by lane.
// A lane that has already converged is an accepted typed boundary; a later,
// unrelated completion gate must not resurrect that lane merely because its
// repair directive was queued again before the convergence check. Read debt
// and repairs owned by other lanes remain untouched.
func (c *EvidenceClosure) ClearRepairsByDowngradeLane(lane DowngradeLane) int {
	if c == nil || lane == "" {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	kept := c.repairs[:0]
	cleared := 0
	for _, repair := range c.repairs {
		if repair.DowngradeLane == lane {
			cleared++
			continue
		}
		kept = append(kept, repair)
	}
	c.repairs = kept
	return cleared
}

// HasCompletionCaveat reports whether lane already reached its typed
// disclose-and-proceed boundary. It is the monotonic authority used by later
// completion attempts; retry prose and incidental blocker churn are not.
func (c *EvidenceClosure) HasCompletionCaveat(lane DowngradeLane) bool {
	if c == nil || lane == "" {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, caveat := range c.completionCaveats {
		if caveat.Lane == lane {
			return true
		}
	}
	return false
}

// ClearCompletionCaveat removes a lane boundary after precise current-state
// evidence proves that lane's obligation. This is an upgrade, not a retry:
// all evidence remains in the closure and no other caveat is changed.
func (c *EvidenceClosure) ClearCompletionCaveat(lane DowngradeLane) bool {
	if c == nil || lane == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, caveat := range c.completionCaveats {
		if caveat.Lane != lane {
			continue
		}
		c.completionCaveats = append(c.completionCaveats[:i], c.completionCaveats[i+1:]...)
		return true
	}
	return false
}
