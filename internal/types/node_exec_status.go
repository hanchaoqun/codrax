package types

import "strings"

// NodeExecStatus is the typed run-local execution status for an IR TaskNode.
// It mirrors the scheduler's current graphState status map during the M1b
// shadow phase so later loopkernel work can consume a lane-neutral authority
// without scraping scheduler internals or rendered logs.
type NodeExecStatus string

const (
	NodeExecPending  NodeExecStatus = "pending"
	NodeExecRunning  NodeExecStatus = "running"
	NodeExecDone     NodeExecStatus = "done"
	NodeExecFailed   NodeExecStatus = "failed"
	NodeExecRequeued NodeExecStatus = "requeued"
)

func NormalizeNodeExecStatus(status NodeExecStatus) NodeExecStatus {
	switch NodeExecStatus(strings.TrimSpace(string(status))) {
	case NodeExecRunning:
		return NodeExecRunning
	case NodeExecDone:
		return NodeExecDone
	case NodeExecFailed:
		return NodeExecFailed
	case NodeExecRequeued:
		return NodeExecRequeued
	default:
		return NodeExecPending
	}
}
