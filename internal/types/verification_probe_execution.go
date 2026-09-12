package types

import (
	"encoding/hex"
	"time"
)

const VerificationProbeExecutionReceiptVersion = 1

// VerificationProbeExecutionReceipt separates the reproducible definition /
// invocation from the actual execution instance. Only the execution producer
// writes it, after a started process has reached a terminal state. Digests do
// not contain execution timestamps or random producer-owned temporary paths.
// Actual invocation fields remain available for audit, not equality guessing.
type VerificationProbeExecutionReceipt struct {
	Version          int                                      `json:"version"`
	DefinitionSHA256 string                                   `json:"definition_sha256"`
	InvocationSHA256 string                                   `json:"invocation_sha256"`
	ExecutionID      string                                   `json:"execution_id"`
	StartedAt        time.Time                                `json:"started_at"`
	FinishedAt       time.Time                                `json:"finished_at"`
	RepositoryRoot   string                                   `json:"repository_root"`
	Executable       string                                   `json:"executable"`
	Args             []string                                 `json:"args"`
	WorkingDir       string                                   `json:"working_dir"`
	TargetExecution  *VerificationProbeTargetExecutionReceipt `json:"target_execution,omitempty"`
}

func verificationProbeExecutionIdentity(receipt *VerificationProbeExecutionReceipt) string {
	if receipt == nil || receipt.Version != VerificationProbeExecutionReceiptVersion || receipt.ExecutionID == "" ||
		receipt.StartedAt.IsZero() || receipt.FinishedAt.Before(receipt.StartedAt) ||
		receipt.RepositoryRoot == "" || receipt.Executable == "" || len(receipt.Args) == 0 || receipt.WorkingDir == "" {
		return ""
	}
	for _, value := range []string{receipt.DefinitionSHA256, receipt.InvocationSHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return ""
		}
	}
	return verificationProofLedgerStableID("probe_execution_v1", receipt.DefinitionSHA256, receipt.InvocationSHA256)
}
