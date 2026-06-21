package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

func readyNodeOrderLess(g types.TaskGraph, left, right string) bool {
	leftRank := criticalPathRank(g, left)
	rightRank := criticalPathRank(g, right)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	return declOrder(g, left) < declOrder(g, right)
}

func criticalPathRank(g types.TaskGraph, id string) int {
	for i, pathID := range g.ExecutionPolicy.CriticalPath {
		if pathID == id {
			return i
		}
	}
	return 1 << 30
}
