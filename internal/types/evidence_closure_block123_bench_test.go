package types

import "testing"

// BenchmarkAppendViolation_PerStageBookkeeping pins the
// performance overhead Block 1 introduces by auto-bumping
// PerStage[Stage].Violations on every AppendViolation call. The
// hot-path footprint must stay O(1) — a single map write to a
// small (≤7 stages) map plus the per-Run scalar bump.
func BenchmarkAppendViolation_PerStageBookkeeping(b *testing.B) {
	c := NewEvidenceClosure("")
	v := Violation{Kind: ViolShape, Stage: "finalize"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.AppendViolation(v)
	}
}

// BenchmarkStageHealthSnapshot pins the cost of the read API used
// by emitCGECSummary at end-of-Run and by Block 3's selective
// fallback before each contract-failure decision. Should remain
// O(stages × violations) where stages ≤ 7 and violations is the
// per-Run total.
func BenchmarkStageHealthSnapshot(b *testing.B) {
	c := NewEvidenceClosure("")
	// Seed a realistic violation set: 100 entries spread across 4
	// stages, including 10 contradictions.
	for i := 0; i < 90; i++ {
		c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize"})
	}
	for i := 0; i < 10; i++ {
		c.AppendViolation(Violation{Kind: ViolSelfContradiction, Stage: "finalize"})
	}
	for i := 0; i < 5; i++ {
		c.AppendViolation(Violation{Kind: ViolPlanCritic, Stage: "plan"})
	}
	for i := 0; i < 5; i++ {
		c.AppendViolation(Violation{Kind: ViolReflectorObservation, Stage: "verify"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.StageHealthSnapshot()
	}
}

// BenchmarkAppendViolation_Baseline measures the cost without
// per-stage bookkeeping (Stage="" path) so we can quantify the
// Block 1 overhead vs the legacy path. A single AppendViolation
// with empty Stage skips the PerStage map write entirely.
func BenchmarkAppendViolation_Baseline(b *testing.B) {
	c := NewEvidenceClosure("")
	v := Violation{Kind: ViolShape} // no Stage
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.AppendViolation(v)
	}
}
