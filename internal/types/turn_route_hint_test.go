package types

import "testing"

func TestTurnRouteHintRequiresCurrentSourceEvidence(t *testing.T) {
	tests := []struct {
		name string
		hint TurnRouteHint
		want bool
	}{
		{
			name: "required is independent of repository execution access",
			hint: TurnRouteHint{
				CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceRequired,
			},
			want: true,
		},
		{
			name: "artifact pipeline can keep current checkout optional",
			hint: TurnRouteHint{
				NeedsRepoAccess:           true,
				CurrentSourceEvidenceMode: TurnRouteCurrentSourceEvidenceOptional,
			},
			want: false,
		},
		{
			name: "legacy hint retains repository access fallback",
			hint: TurnRouteHint{
				NeedsRepoAccess: true,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hint.RequiresCurrentSourceEvidence(); got != tt.want {
				t.Fatalf("RequiresCurrentSourceEvidence()=%v, want %v for %+v", got, tt.want, tt.hint)
			}
		})
	}
}
