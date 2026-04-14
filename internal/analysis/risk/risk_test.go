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

