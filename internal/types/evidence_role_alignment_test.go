package types

import "testing"

func TestEvidenceClaimRoleMentionedBySurface_PrecedenceIgnoresBareRoleWords(t *testing.T) {
	ev := EvidenceItem{
		Source:          "codrax.yaml.example",
		LineStart:       489,
		Scope:           ScopeLine,
		GroundingStatus: GroundingGrounded,
		DiagramRole:     EvidenceDiagramRoleConfig,
		SurfaceTerms:    []string{"config", "config_canonical"},
	}

	if EvidenceClaimRoleMentionedBySurface(
		ev,
		[]ClaimForm{ClaimPrecedenceRole},
		"ExploreBudget 是运行时计数器类型，不是独立 config key。",
	) {
		t.Fatal("bare generic role word 'config' must not trigger config precedence citation alignment")
	}
	if !EvidenceClaimRoleMentionedBySurface(
		ev,
		[]ClaimForm{ClaimPrecedenceRole},
		"codrax.yaml.example 配置层列出了 explore_* 配置键。",
	) {
		t.Fatal("specific source surface should trigger config precedence citation alignment")
	}
}
