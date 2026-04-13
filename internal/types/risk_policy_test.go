package types

import "testing"

func TestResolveRiskPolicy(t *testing.T) {
	t.Run("critical security requires review and verify", func(t *testing.T) {
		p := ResolveRiskPolicy(RiskMatrix{Security: RiskDimension{Level: 4}})
		if !p.RequireReview || !p.RequireVerification {
			t.Fatalf("unexpected policy: %#v", p)
		}
	})

	t.Run("moderate risk enforces verify only", func(t *testing.T) {
		p := ResolveRiskPolicy(RiskMatrix{Performance: RiskDimension{Level: 3}})
		if p.RequireReview || !p.RequireVerification {
			t.Fatalf("unexpected policy: %#v", p)
		}
	})

	t.Run("low risk keeps default", func(t *testing.T) {
		p := ResolveRiskPolicy(RiskMatrix{})
		if p.RequireReview || p.RequireVerification || len(p.MandatoryStages) != 0 {
			t.Fatalf("unexpected policy: %#v", p)
		}
	})
}

func TestNormalizeRiskMatrix(t *testing.T) {
	m := NormalizeRiskMatrix(RiskMatrix{
		Security: RiskDimension{Level: -3},
		Ops:      RiskDimension{Level: 9},
	})
	if m.Security.Level != 0 || m.Ops.Level != 5 {
		t.Fatalf("normalize failed: %#v", m)
	}
}
