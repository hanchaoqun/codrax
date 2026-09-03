package normalizer

import (
	"reflect"
	"testing"
)

// EVOLUTION RECORD (V4-3, colleague_merge_audit §40.21): this file used to
// pin CanonicalizeSlashPairEntities, which REPLACED a model-emitted joined
// `X/Y` entity by X, Y whenever the request wrote the pair. That split was
// retired: a normalizer that runs before a hard gate may complete a roster
// but must never change the model's entity count. The retirement pin below
// asserts that a joined entity survives verbatim; RequestSlashPairHalves is
// the exported pure predicate the emit-side gate now consults instead.

func TestCompleteSlashPairEntities_NeverSplitsJoinedEntity(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		entities []string
		want     []string
	}{
		{
			name:     "joined typed entity stays one model claim",
			raw:      "draw analyzer and Mutable/BusContext data flow",
			entities: []string{"analyzer", "Mutable/BusContext"},
			want:     []string{"analyzer", "Mutable/BusContext"},
		},
		{
			name:     "one emitted half still completes the pair",
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
			if got := CompleteSlashPairEntities(tt.raw, tt.entities); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CompleteSlashPairEntities()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestSlashPairs(t *testing.T) {
	got := RequestSlashPairs("画出 analyzer、Mutable/BusContext 之间的数据流，读 internal/agent 与 client/server，比较 retry_budget/cap_limit")
	want := [][2]string{{"Mutable", "BusContext"}, {"retry_budget", "cap_limit"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RequestSlashPairs()=%v want %v", got, want)
	}
	if got := RequestSlashPairs("plain question"); got != nil {
		t.Fatalf("no slash → nil, got %v", got)
	}
}

func TestRequestSlashPairHalves(t *testing.T) {
	tests := []struct {
		name        string
		raw, entity string
		left, right string
		ok          bool
	}{
		{name: "exact request pair", raw: "画出 analyzer、Mutable/BusContext 之间的数据流", entity: "Mutable/BusContext", left: "Mutable", right: "BusContext", ok: true},
		{name: "case-insensitive verbatim match", raw: "Mutable/BusContext", entity: " mutable/buscontext ", left: "Mutable", right: "BusContext", ok: true},
		{name: "snake_case halves", raw: "check retry_budget/cap_limit pair", entity: "retry_budget/cap_limit", left: "retry_budget", right: "cap_limit", ok: true},
		{name: "one half is not the pair", raw: "Mutable/BusContext", entity: "Mutable", ok: false},
		{name: "repository path never qualifies", raw: "look under internal/agent", entity: "internal/agent", ok: false},
		{name: "lowercase word pair never qualifies", raw: "client/server flow", entity: "client/server", ok: false},
		{name: "pair absent from request", raw: "plain question", entity: "Mutable/BusContext", ok: false},
		{name: "multi-segment path out of scope", raw: "Read internal/agent/Analyzer", entity: "agent/Analyzer", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, right, ok := RequestSlashPairHalves(tt.raw, tt.entity)
			if ok != tt.ok || left != tt.left || right != tt.right {
				t.Fatalf("RequestSlashPairHalves(%q,%q)=(%q,%q,%v) want (%q,%q,%v)", tt.raw, tt.entity, left, right, ok, tt.left, tt.right, tt.ok)
			}
		})
	}
}
