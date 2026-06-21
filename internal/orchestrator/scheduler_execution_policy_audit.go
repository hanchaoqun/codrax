package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func deriveExecutionPolicyAuditWarnings(g types.TaskGraph) []types.ExecutionPolicyAuditWarning {
	warnings := types.ExecutionPolicyAuditWarnings(g)
	for _, warning := range warnings {
		logging.Warning("[scheduler] execution policy audit warning: %s", formatExecutionPolicyAuditWarning(warning))
	}
	return warnings
}

func (s *graphState) executionPolicyWarnings() []types.ExecutionPolicyAuditWarning {
	if s == nil {
		return nil
	}
	return types.CloneExecutionPolicyAuditWarnings(s.executionPolicyAuditWarnings)
}

func formatExecutionPolicyAuditWarning(w types.ExecutionPolicyAuditWarning) string {
	switch w.Code {
	case types.ExecutionPolicyAuditBlankCriticalPathID:
		return fmt.Sprintf("code=%s path_index=%d", w.Code, w.PathIndex)
	case types.ExecutionPolicyAuditDuplicateCriticalPathID:
		return fmt.Sprintf("code=%s node_id=%s path_index=%d first_index=%d", w.Code, w.NodeID, w.PathIndex, w.FirstIndex)
	case types.ExecutionPolicyAuditMissingCriticalPathNode:
		return fmt.Sprintf("code=%s node_id=%s path_index=%d", w.Code, w.NodeID, w.PathIndex)
	default:
		return fmt.Sprintf("code=%s node_id=%s path_index=%d first_index=%d", w.Code, w.NodeID, w.PathIndex, w.FirstIndex)
	}
}
