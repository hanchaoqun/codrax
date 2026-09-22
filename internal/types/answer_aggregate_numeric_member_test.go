package types

import (
	"strings"
	"testing"
)

func TestAggregateCompactDotKeepsNumericMembersLiteral(t *testing.T) {
	for _, member := range []string{
		"34579.451840 (line 118)", "0.125", "-12.5", "+12.5",
		"12.5ms", "1.2e-3", "1_000.25", "1_000.25 (L8)",
		"0x1.2p3", "1e999.25", "１２.５", "１２_３.５",
		"worker.go", "settings.yaml", "internal/worker.go",
	} {
		t.Run(member, func(t *testing.T) {
			if left, right, ok := AnswerAggregateMemberRelationParts(member); ok {
				t.Errorf("literal member became a relation: %q -> %q", left, right)
			}
			if got := AnswerAggregateMemberSurfaceKey(member); got != "literal:"+strings.ToLower(member) {
				t.Errorf("literal member acquired a relation identity: %q", got)
			}
			for _, display := range AnswerAggregateMemberDisplayCandidates(member) {
				if strings.ContainsAny(display, "→") || strings.Contains(display, " -> ") {
					t.Errorf("literal member acquired an arrow display: %q", display)
				}
			}
			fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer, Value: "1", Members: []string{member}}
			if AnswerAggregateFactHasRelationMembers(fact) {
				t.Error("literal-only fact was classified as a relation set")
			}
		})
	}
}

func TestAggregateCompactDotPreservesNamedOwnersAndExplicitEdges(t *testing.T) {
	for _, pair := range [][2]string{
		{"compiler.Compile", "compiler → Compile"},
		{"Service.run (line 118)", "Service → run (line 118)"},
		{"7zip.Entry", "7zip → Entry"}, {"_1.Entry", "_1 → Entry"},
		{"$1.Entry", "$1 → Entry"}, {"模块.执行", "模块 → 执行"},
		{"foo-bar.Entry", "foo-bar → Entry"}, {"pair.0", "pair → 0"},
		{"123 -> 456", "123 → 456"}, {"crate::run", "crate → run"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			left, right, ok := AnswerAggregateMemberRelationParts(pair[0])
			if !ok || left == "" || right == "" {
				t.Fatalf("existing symbolic/explicit relation lost: %q", pair[0])
			}
			key := "relation:" + strings.ToLower(left) + "\x00" + strings.ToLower(right)
			if got := AnswerAggregateMemberSurfaceKey(pair[0]); got != AnswerAggregateMemberSurfaceKey(pair[1]) {
				t.Errorf("existing relation spelling identity changed: %q vs %q", got, pair[1])
			}
			// Non-decorated spellings keep the same canonical identity bytes.
			if !strings.Contains(pair[0], "(") && AnswerAggregateMemberSurfaceKey(pair[0]) != key {
				t.Errorf("named relation identity changed: %q", AnswerAggregateMemberSurfaceKey(pair[0]))
			}
		})
	}
}
