package types

import "strings"

func (c *EvidenceClosure) SetNodeExecStatus(nodeID string, status NodeExecStatus) {
	if c == nil {
		return
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodeExecStatus == nil {
		c.nodeExecStatus = make(map[string]NodeExecStatus)
	}
	c.nodeExecStatus[nodeID] = NormalizeNodeExecStatus(status)
}

func (c *EvidenceClosure) NodeExecStatus(nodeID string) NodeExecStatus {
	if c == nil {
		return NodeExecPending
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return NodeExecPending
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return NormalizeNodeExecStatus(c.nodeExecStatus[nodeID])
}

func (c *EvidenceClosure) NodeExecStatuses() map[string]NodeExecStatus {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneNodeExecStatusMap(c.nodeExecStatus)
}

func (c *EvidenceClosure) IncrementNodeExecAttempt(nodeID string) int {
	if c == nil {
		return 0
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nodeExecAttempts == nil {
		c.nodeExecAttempts = make(map[string]int)
	}
	c.nodeExecAttempts[nodeID]++
	return c.nodeExecAttempts[nodeID]
}

func (c *EvidenceClosure) SetNodeExecAttempts(attempts map[string]int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodeExecAttempts = cloneNodeExecAttemptMap(attempts)
	if c.nodeExecAttempts == nil {
		c.nodeExecAttempts = make(map[string]int)
	}
}

func (c *EvidenceClosure) NodeExecAttempt(nodeID string) int {
	if c == nil {
		return 0
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	attempts := c.nodeExecAttempts[nodeID]
	if attempts < 0 {
		return 0
	}
	return attempts
}

func (c *EvidenceClosure) NodeExecAttempts() map[string]int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneNodeExecAttemptMap(c.nodeExecAttempts)
}
