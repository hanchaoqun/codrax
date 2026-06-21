package types

import "strings"

// NodeExecStatus is the typed run-local execution status for an IR TaskNode.
// Read/write loop components consume it as the lane-neutral execution authority
// instead of scraping scheduler internals, rendered logs, or model prose.
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
