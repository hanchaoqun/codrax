package types

// ReadRangesSnapshot returns a defensive copy of all merged read ranges keyed
// by canonical repo-relative path. It is an audit/resume substrate; callers
// must not rebuild it from rendered tool prose.
func (c *EvidenceClosure) ReadRangesSnapshot() map[string][]LineRange {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneLineRangeMap(c.readRanges)
}

// FileTotalLinesSnapshot returns a defensive copy of every banner-derived file
// line count currently known to the closure.
func (c *EvidenceClosure) FileTotalLinesSnapshot() map[string]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneIntMap(c.fileTotalLines)
}
