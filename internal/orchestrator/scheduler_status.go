package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func (s *graphState) attachEvidenceClosure(closure *types.EvidenceClosure) {
	if s == nil {
		return
	}
	s.closure = closure
	if closure == nil {
		return
	}
	for id, status := range s.status {
		if status == nodePending && closure.NodeExecStatus(id) != types.NodeExecPending {
			continue
		}
		closure.SetNodeExecStatus(id, nodeStatusToExecStatus(status))
	}
	s.status = nil
	if len(s.attempts) > 0 {
		merged := closure.NodeExecAttempts()
		if merged == nil {
			merged = make(map[string]int, len(s.attempts))
		}
		for id, attempts := range s.attempts {
			if attempts > merged[id] {
				merged[id] = attempts
			}
		}
		closure.SetNodeExecAttempts(merged)
	}
	s.attempts = nil
}

// markRunning / markDone / markFailed / requeue are state transitions on the
// typed node execution status authority.
func (s *graphState) markRunning(id string) {
	s.incrementNodeAttempt(id)
	s.setStatus(id, nodeRunning)
}
func (s *graphState) markDone(id string)   { s.setStatus(id, nodeDone) }
func (s *graphState) markFailed(id string) { s.setStatus(id, nodeFailed) }
func (s *graphState) requeue(id string)    { s.setStatus(id, nodeRequeued) }

func (s *graphState) setStatus(id string, status nodeStatus) {
	if s == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.closure != nil {
		s.closure.SetNodeExecStatus(id, nodeStatusToExecStatus(status))
		return
	}
	if s.status == nil {
		s.status = make(map[string]nodeStatus)
	}
	s.status[id] = status
}

func nodeStatusToExecStatus(status nodeStatus) types.NodeExecStatus {
	switch status {
	case nodeRunning:
		return types.NodeExecRunning
	case nodeDone:
		return types.NodeExecDone
	case nodeFailed:
		return types.NodeExecFailed
	case nodeRequeued:
		return types.NodeExecRequeued
	default:
		return types.NodeExecPending
	}
}

func execStatusToNodeStatus(status types.NodeExecStatus) nodeStatus {
	switch types.NormalizeNodeExecStatus(status) {
	case types.NodeExecRunning:
		return nodeRunning
	case types.NodeExecDone:
		return nodeDone
	case types.NodeExecFailed:
		return nodeFailed
	case types.NodeExecRequeued:
		return nodeRequeued
	default:
		return nodePending
	}
}

func (s *graphState) incrementNodeAttempt(id string) int {
	if s == nil {
		return 0
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0
	}
	if s.closure != nil {
		return s.closure.IncrementNodeExecAttempt(id)
	}
	if s.attempts == nil {
		s.attempts = make(map[string]int)
	}
	s.attempts[id]++
	return s.attempts[id]
}

func (s *graphState) nodeAttempt(id string) int {
	if s == nil {
		return 0
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return 0
	}
	if s.closure != nil {
		return s.closure.NodeExecAttempt(id)
	}
	if s.attempts == nil {
		return 0
	}
	attempts := s.attempts[id]
	if attempts < 0 {
		return 0
	}
	return attempts
}

func (s *graphState) nodeStatus(id string) nodeStatus {
	if s == nil {
		return nodePending
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nodePending
	}
	if s.closure != nil {
		return execStatusToNodeStatus(s.closure.NodeExecStatus(id))
	}
	if s.status == nil {
		return nodePending
	}
	st, ok := s.status[id]
	if !ok {
		return nodePending
	}
	switch st {
	case nodeRunning, nodeDone, nodeFailed, nodeRequeued:
		return st
	default:
		return nodePending
	}
}
