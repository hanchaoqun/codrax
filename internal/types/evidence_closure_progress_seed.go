package types

// SetLatestProgressDecision hydrates the typed progress authority from a
// durable read-run snapshot. It is intentionally separate from
// RecordDowngradeProgressDelta: resume/replay must not mutate the downgrade
// fingerprint window just to restore the last structured decision.
func (c *EvidenceClosure) SetLatestProgressDecision(decision ProgressDecision) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.latestProgressDecision = decision
	c.mu.Unlock()
}
