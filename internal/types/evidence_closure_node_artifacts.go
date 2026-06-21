package types

// AppendNodeArtifactRecords merges node-scoped artifact refs into the closure's
// run-local ledger. It is a typed reducer target; callers must not use it to
// store payloads or rendered prose.
func (c *EvidenceClosure) AppendNodeArtifactRecords(records []NodeArtifactRecord) {
	if c == nil || len(records) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeArtifactLedger = MergeNodeArtifactLedgers(c.nodeArtifactLedger, NodeArtifactLedger{Records: records})
}

func (c *EvidenceClosure) SetNodeArtifactLedger(ledger NodeArtifactLedger) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeArtifactLedger = NormalizeNodeArtifactLedger(ledger)
}

func (c *EvidenceClosure) NodeArtifactLedger() NodeArtifactLedger {
	if c == nil {
		return NodeArtifactLedger{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CloneNodeArtifactLedger(c.nodeArtifactLedger)
}

func (c *EvidenceClosure) NodeArtifactRecords() []NodeArtifactRecord {
	if c == nil {
		return nil
	}
	return c.NodeArtifactLedger().Records
}
