package types

import "testing"

func TestResolveLogOperationalRelationAuthority(t *testing.T) {
	local := LogOperationalSemantic{TransitionAuthority: LogOperationalTransitionEventLocalOnly}
	witness := LogOperationalSemantic{TransitionAuthority: "producer_transition_id"}
	tests := []struct {
		name string
		rows []LogOperationalSemantic
		want LogOperationalRelationAuthority
	}{
		{name: "none", want: LogOperationalRelationNotApplicable},
		{name: "single", rows: []LogOperationalSemantic{local}, want: LogOperationalRelationNotApplicable},
		{name: "line order only", rows: []LogOperationalSemantic{local, local}, want: LogOperationalRelationObservedLineOrderOnly},
		{name: "typed transition", rows: []LogOperationalSemantic{local, witness}, want: LogOperationalRelationTypedTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveLogOperationalRelationAuthority(tt.rows); got != tt.want {
				t.Fatalf("authority=%q, want %q", got, tt.want)
			}
		})
	}
}
