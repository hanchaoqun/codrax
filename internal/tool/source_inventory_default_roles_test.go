package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSourceInventoryDefaultQueryEnumerationRolesCoreDeclarationsOnly(t *testing.T) {
	got := sourceInventoryDefaultQueryEnumerationRoles()
	want := []types.AnswerCandidateRole{
		types.AnswerCandidateRoleFunction,
		types.AnswerCandidateRoleMethod,
		types.AnswerCandidateRoleType,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default synthesized source-inventory roles\ngot:  %#v\nwant: %#v", got, want)
	}
}
