package index

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestB1692PublicCangjieReturnExpressionRequiresCompleteShape(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		want             bool
	}{
		{"competing_arrow", `x => "y"`, false},
		{"adjacent_atoms", `1 garbage`, false},
		{"assignment", `x = "y"`, false},
		{"unparsed_operator", `x + y`, false},
		{"unparsed_index", `x[0]`, false},
		{"adjacent_literals", `"one" "two"`, false},
		{"bad_call_arguments", `combine(first second)`, false},
		{"member_missing_name", `x.`, false},
		{"separated_decimal_digits", `1 2`, false},
		{"depth_limit", strings.Repeat("(", 33) + "value" + strings.Repeat(")", 33), false},
		{"token_limit", "value" + strings.Repeat(".member", 128), false},
		{"string_atom", `"value"`, true},
		{"rune_atom", `'x'`, true},
		{"decimal_atom", `123`, true},
		{"identifier", `selected`, true},
		{"boolean_atom", `false`, true},
		{"member", `settings.value`, true},
		{"zero_argument_call", `makeValue()`, true},
		{"nested_call", `normalize(config.value, build("value", 123))`, true},
		{"grouped_member", `(settings.value)`, true},
		{"call_member", `factory().value`, true},
		{"depth_within_limit", strings.Repeat("(", 32) + "value" + strings.Repeat(")", 32), true},
		{"tokens_within_limit", "value" + strings.Repeat(".member", 127), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "package p\nfunc work(): String { return " + tc.expression + "; }"
			fi, rows := b1692ParsePublic(t, types.LangCangjie, source)
			if len(fi.Symbols) != 1 || !fi.Symbols[0].HasParserOwnedBody() {
				t.Fatal("actual parser callable/body prerequisite missing")
			}
			got := types.NewCallableReturnReader(fi, []byte(source)).For(fi.Symbols[0])
			if !tc.want {
				if len(rows) != 0 || len(got) != 0 {
					t.Fatalf("unparsed expression received confirmed callable return: rows=%+v reader=%+v", rows, got)
				}
				return
			}
			if len(rows) != 1 || len(got) != 1 || rows[0].Kind != "explicit_return" || rows[0].Expression != tc.expression || !reflect.DeepEqual(got, fi.CallableReturnExpressions) {
				t.Fatalf("supported exact return expression lost: rows=%+v reader=%+v", rows, got)
			}
		})
	}
}
