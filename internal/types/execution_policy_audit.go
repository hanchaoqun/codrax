package types

import "strings"

const (
	ExecutionPolicyAuditBlankCriticalPathID     = "blank_critical_path_id"
	ExecutionPolicyAuditMissingCriticalPathNode = "missing_critical_path_node"
	ExecutionPolicyAuditDuplicateCriticalPathID = "duplicate_critical_path_id"
)

// ExecutionPolicyAuditWarning is a closed typed warning for structural drift in
// analyzer/compiler-authored execution policy. It is advisory: schedulers may
// keep using the valid subset of the policy, but invalid IDs must not disappear
// as silent priority corruption.
type ExecutionPolicyAuditWarning struct {
	Code       string `json:"code"`
	NodeID     string `json:"node_id,omitempty"`
	PathIndex  int    `json:"path_index,omitempty"`
	FirstIndex int    `json:"first_index,omitempty"`
}

func ExecutionPolicyAuditWarnings(g TaskGraph) []ExecutionPolicyAuditWarning {
	if len(g.ExecutionPolicy.CriticalPath) == 0 {
		return nil
	}
	nodeIDs := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			nodeIDs[id] = struct{}{}
		}
	}
	seen := make(map[string]int, len(g.ExecutionPolicy.CriticalPath))
	var out []ExecutionPolicyAuditWarning
	for i, raw := range g.ExecutionPolicy.CriticalPath {
		id := strings.TrimSpace(raw)
		if id == "" {
			out = append(out, ExecutionPolicyAuditWarning{
				Code:      ExecutionPolicyAuditBlankCriticalPathID,
				PathIndex: i,
			})
			continue
		}
		if first, ok := seen[id]; ok {
			out = append(out, ExecutionPolicyAuditWarning{
				Code:       ExecutionPolicyAuditDuplicateCriticalPathID,
				NodeID:     id,
				PathIndex:  i,
				FirstIndex: first,
			})
		} else {
			seen[id] = i
		}
		if _, ok := nodeIDs[id]; !ok {
			out = append(out, ExecutionPolicyAuditWarning{
				Code:      ExecutionPolicyAuditMissingCriticalPathNode,
				NodeID:    id,
				PathIndex: i,
			})
		}
	}
	return out
}

func CloneExecutionPolicyAuditWarnings(in []ExecutionPolicyAuditWarning) []ExecutionPolicyAuditWarning {
	if len(in) == 0 {
		return nil
	}
	out := make([]ExecutionPolicyAuditWarning, len(in))
	copy(out, in)
	return out
}
