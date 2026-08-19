package normalizer

import (
	"reflect"
	"testing"
)

func TestCanonicalizeSlashPairEntities(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		entities []string
		want     []string
	}{
		{
			name:     "joined typed entity becomes two identities",
			raw:      "draw analyzer and Mutable/BusContext data flow",
			entities: []string{"analyzer", "Mutable/BusContext"},
			want:     []string{"analyzer", "Mutable", "BusContext"},
		},
		{
			name:     "one emitted half completes the pair",
			raw:      "draw Mutable/BusContext data flow",
			entities: []string{"Mutable"},
			want:     []string{"Mutable", "BusContext"},
		},
		{
			name:     "repository path remains one entity",
			raw:      "show internal/agent flow",
			entities: []string{"internal/agent"},
			want:     []string{"internal/agent"},
		},
		{
			name:     "lowercase path-like pair stays untouched",
			raw:      "show client/server flow",
			entities: []string{"client/server"},
			want:     []string{"client/server"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalizeSlashPairEntities(tt.raw, tt.entities); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CanonicalizeSlashPairEntities()=%v, want %v", got, tt.want)
			}
		})
	}
}
