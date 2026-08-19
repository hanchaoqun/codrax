package types

import "strings"

// RemoveAcceptedEvidenceIDs applies an explicit typed supersession to the
// closure carrier. It removes only exact stable IDs, including historical
// repair snapshots that would otherwise reintroduce the stale identity during
// handoff. Production callers route this through EvidenceReducerInput.
func (c *EvidenceClosure) RemoveAcceptedEvidenceIDs(ids []string) {
	if c == nil || len(ids) == 0 {
		return
	}
	remove := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			remove[id] = true
		}
	}
	if len(remove) == 0 {
		return
	}
	filter := func(refs []AcceptedEvidenceRef) []AcceptedEvidenceRef {
		out := make([]AcceptedEvidenceRef, 0, len(refs))
		for _, ref := range refs {
			if remove[strings.TrimSpace(ref.ID)] {
				continue
			}
			out = append(out, ref)
		}
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptedEvidence = filter(c.acceptedEvidence)
	for i := range c.repairs {
		c.repairs[i].AcceptedEvidence = filter(c.repairs[i].AcceptedEvidence)
	}
}
