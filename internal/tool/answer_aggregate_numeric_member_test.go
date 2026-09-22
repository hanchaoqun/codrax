package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The emit-side compatibility parser must not reintroduce a relation that the
// shared grammar deliberately left literal (including bare source filenames).
func TestPreEmitAggregateMemberRelationUsesSharedGrammar(t *testing.T) {
	for _, member := range []string{
		"34579.451840 (line 118)", "0.125", "12.5ms", "1_000.25", "0x1.2p3",
		"worker.go", "settings.yaml", "compiler.Compile", "Service.run (line 118)",
		"7zip.Entry", "_1.Entry", "$1.Entry", "模块.执行", "pair.0", "123 -> 456",
	} {
		t.Run(member, func(t *testing.T) {
			left, right, ok := preEmitAggregateMemberLabelRelationParts(member)
			wantLeft, wantRight, wantOK := types.AnswerAggregateMemberRelationParts(member)
			if left != wantLeft || right != wantRight || ok != wantOK {
				t.Errorf("emit-side parser resurrected a rejected or different relation: got (%q,%q,%t), shared (%q,%q,%t)", left, right, ok, wantLeft, wantRight, wantOK)
			}
		})
	}
}
