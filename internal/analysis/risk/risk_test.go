package risk

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func termGraph(ids ...string) types.TermGraph {
	tg := types.TermGraph{}
	for _, id := range ids {
		tg.Canonical = append(tg.Canonical, types.CanonicalTerm{ID: id, Surface: id})
	}
	return tg
}

func TestEvaluate_SecurityBump(t *testing.T) {
	rm := types.RequestModel{TermGraph: termGraph("en:password", "en:token")}
	got := Evaluate(rm, types.RiskMatrix{})
	if got.Security.Level < 4 {
		t.Fatalf("password should bump security to ≥4; got %d", got.Security.Level)
	}
	if got.Compliance.Level < 3 {
		t.Fatalf("password should also bump compliance; got %d", got.Compliance.Level)
	}
}

func TestEvaluate_Monotonic_NeverDropsLLMLevels(t *testing.T) {
	rm := types.RequestModel{TermGraph: termGraph("en:audit")} // audit bumps compliance to 2
	start := types.RiskMatrix{
		Security: types.RiskLevel{Level: 5, Evidence: []string{"llm said so"}},
	}
	got := Evaluate(rm, start)
	if got.Security.Level != 5 {
		t.Fatalf("Evaluate must not drop existing levels; got %d", got.Security.Level)
	}
	if len(got.Security.Evidence) == 0 {
		t.Fatalf("existing evidence must be preserved")
	}
}

func TestEvaluate_Compliance_GDPR(t *testing.T) {
	rm := types.RequestModel{TermGraph: termGraph("en:gdpr")}
	got := Evaluate(rm, types.RiskMatrix{})
	if got.Compliance.Level != 5 {
		t.Fatalf("gdpr → compliance=5, got %d", got.Compliance.Level)
	}
	if got.Security.Level < 3 {
		t.Fatalf("gdpr should also push security ≥3; got %d", got.Security.Level)
	}
}

func TestEvaluate_ChineseTerms(t *testing.T) {
	// After B4c review: bare 迁移/migration is a weak signal because
	// this refactor itself uses "migration batches" — real schema
	// migration still trips data_integrity via the schema term.
	rm := types.RequestModel{TermGraph: termGraph("zh:迁移", "en:schema")}
	got := Evaluate(rm, types.RiskMatrix{})
	if got.DataIntegrity.Level < 3 {
		t.Fatalf("zh:迁移 + en:schema → data_integrity≥3; got %d", got.DataIntegrity.Level)
	}

	// Bare 迁移 alone must only yield a weak bump (≤2).
	bare := Evaluate(types.RequestModel{TermGraph: termGraph("zh:迁移")}, types.RiskMatrix{})
	if bare.DataIntegrity.Level > 2 {
		t.Fatalf("bare zh:迁移 must not exceed weak signal; got %d", bare.DataIntegrity.Level)
	}
}

func TestEvaluate_EmptyTermGraph_NoChange(t *testing.T) {
	got := Evaluate(types.RequestModel{}, types.RiskMatrix{})
	if !reflect.DeepEqual(got, types.RiskMatrix{}) {
		t.Fatalf("empty input should produce empty matrix, got %+v", got)
	}
}

func TestDerivePolicy_NonWriting_NoReviews(t *testing.T) {
	rm := types.RiskMatrix{
		Security:      types.RiskLevel{Level: 5},
		Compliance:    types.RiskLevel{Level: 5},
		DataIntegrity: types.RiskLevel{Level: 5},
	}
	p := DerivePolicy(rm, false)
	if p.Writing || p.RequireDesignReview || p.RequireCodeReview || p.RequireVerify {
		t.Fatalf("read-only runs must not require any review stages; got %+v", p)
	}
}

func TestDerivePolicy_SecurityTriggersReviews(t *testing.T) {
	p := DerivePolicy(types.RiskMatrix{Security: types.RiskLevel{Level: 3}}, true)
	if !p.RequireDesignReview || !p.RequireCodeReview {
		t.Fatalf("security≥3 must require both reviews; got %+v", p)
	}
}

func TestDerivePolicy_ComplianceTriggersReviews(t *testing.T) {
	p := DerivePolicy(types.RiskMatrix{Compliance: types.RiskLevel{Level: 4}}, true)
	if !p.RequireDesignReview || !p.RequireCodeReview {
		t.Fatalf("compliance≥3 must require both reviews; got %+v", p)
	}
}

func TestDerivePolicy_DataIntegrityTriggersVerify(t *testing.T) {
	p := DerivePolicy(types.RiskMatrix{DataIntegrity: types.RiskLevel{Level: 3}}, true)
	if !p.RequireVerify {
		t.Fatalf("data_integrity≥3 must require verify; got %+v", p)
	}
}

func TestDerivePolicy_CompatibilityForbidsSkipDesign(t *testing.T) {
	p := DerivePolicy(types.RiskMatrix{Compatibility: types.RiskLevel{Level: 3}}, true)
	found := false
	for _, s := range p.ForbidSkipStages {
		if s == "design_review" {
			found = true
		}
	}
	if !found {
		t.Fatalf("compatibility≥3 must forbid skipping design_review; got %+v", p)
	}
}

func TestDerivePolicy_PerformanceTriggersVerify(t *testing.T) {
	p := DerivePolicy(types.RiskMatrix{Performance: types.RiskLevel{Level: 4}}, true)
	if !p.RequireVerify {
		t.Fatalf("performance≥4 must require verify; got %+v", p)
	}
	// performance=3 must NOT trigger verify.
	p = DerivePolicy(types.RiskMatrix{Performance: types.RiskLevel{Level: 3}}, true)
	if p.RequireVerify {
		t.Fatalf("performance=3 should not require verify; got %+v", p)
	}
}

func TestDerivePolicy_AdditiveNoDrops(t *testing.T) {
	rm := types.RiskMatrix{
		Security:      types.RiskLevel{Level: 5},
		DataIntegrity: types.RiskLevel{Level: 5},
		Compatibility: types.RiskLevel{Level: 5},
		Performance:   types.RiskLevel{Level: 5},
		Compliance:    types.RiskLevel{Level: 5},
	}
	p := DerivePolicy(rm, true)
	if !(p.RequireDesignReview && p.RequireCodeReview && p.RequireVerify) {
		t.Fatalf("all-max matrix must require every stage; got %+v", p)
	}
	if len(p.ForbidSkipStages) == 0 {
		t.Fatalf("must forbid skipping at least one stage")
	}
}
