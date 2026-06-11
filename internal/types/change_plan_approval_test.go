package types

import (
	"testing"
	"time"
)

func TestApprovalRecordFingerprint_TamperEvidence(t *testing.T) {
	now := time.Now()
	rec := &WriteApprovalRecord{
		Policy: "manual", RiskLevel: "high", Action: "manual",
		UserDecision: "approved", ReasonCode: "ok", Source: "repl_approve",
		PlanFingerprint: "abc123", DecidedAt: &now, Operator: "alice <a@x>",
	}
	rec.RecordFingerprint = ApprovalRecordFingerprint(rec)
	plan := &ChangePlan{Approval: rec}
	if !plan.ApprovalRecordIntegrityOK() {
		t.Fatal("untouched record must verify")
	}
	rec.UserDecision = "denied" // post-hoc tamper
	if plan.ApprovalRecordIntegrityOK() {
		t.Fatal("edited user_decision must fail integrity")
	}
	rec.UserDecision = "approved"
	tampered := now.Add(48 * time.Hour)
	rec.DecidedAt = &tampered
	if plan.ApprovalRecordIntegrityOK() {
		t.Fatal("edited decided_at must fail integrity")
	}
	// legacy records without a fingerprint stay valid
	legacy := &ChangePlan{Approval: &WriteApprovalRecord{UserDecision: "approved"}}
	if !legacy.ApprovalRecordIntegrityOK() {
		t.Fatal("legacy records are grandfathered")
	}
}
