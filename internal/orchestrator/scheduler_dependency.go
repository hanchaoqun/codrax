package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func (s *graphState) hardDependencyBlockersForNode(nodeID string) []string {
	if s == nil || strings.TrimSpace(nodeID) == "" {
		return nil
	}
	var blockers []string
	seen := map[string]bool{}
	for _, e := range s.graph.Edges {
		if e.EdgeType != types.EdgeHardDependency || e.To != nodeID || strings.TrimSpace(e.From) == "" {
			continue
		}
		if s.nodeStatus(e.From) == nodeDone {
			continue
		}
		if seen[e.From] {
			continue
		}
		seen[e.From] = true
		blockers = append(blockers, e.From)
	}
	return blockers
}

func renderNodeBlocks(ctx context.Context, b *strings.Builder, blocks []nodeBlock) error {
	if b == nil {
		return nil
	}
	wroteEntryHeader := false
	wroteDependencyHeader := false
	for _, bk := range blocks {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(bk.FailedCriteria) > 0 {
			if !wroteEntryHeader {
				b.WriteString("\nEntry-condition gate: the following nodes are waiting on unmet criteria:\n")
				wroteEntryHeader = true
			}
			for _, fc := range bk.FailedCriteria {
				fmt.Fprintf(b, "  - %s: %s (%s) — %s\n", bk.NodeID, fc.Kind, fc.Expr, fc.Detail)
			}
		}
		if len(bk.DependencyBlockers) > 0 {
			if !wroteDependencyHeader {
				b.WriteString("\nDependency gate: the following nodes are waiting on hard_dependency predecessors:\n")
				wroteDependencyHeader = true
			}
			fmt.Fprintf(b, "  - %s: waiting for %s\n", bk.NodeID, strings.Join(bk.DependencyBlockers, ", "))
		}
	}
	return nil
}
