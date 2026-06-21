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
		closure.SetNodeExecStatus(id, nodeStatusToExecStatus(status))
	}
	s.status = nil
}

// markRunning / markDone / markFailed / requeue are state transitions on the
// typed node execution status authority.
func (s *graphState) markRunning(id string) {
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
