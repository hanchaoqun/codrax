package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// V9-1 (§40.15) census tripwire: the action registry (actionCapabilities)
// and the derivation-topology table (dataquery.dataActionTopologies) are a
// bijection — a new action kind must declare its topology (1:1 / 1:N / N:1 /
// none) in the same change that registers it, and the topology table cannot
// name a kind the registry does not admit.
func TestActionTopologyDeclaredForEveryActionKind(t *testing.T) {
	for _, kind := range SupportedActionKinds() {
		topology, ok := ActionTopology(dataquery.DataActionKind(kind))
		if !ok {
			t.Errorf("action kind %q is registered in actionCapabilities but declares no derivation topology", kind)
			continue
		}
		if topology == "" {
			t.Errorf("action kind %q resolves an empty topology", kind)
		}
	}
	for _, kind := range dataquery.DeclaredTopologyActionKinds() {
		if _, ok := Capability(kind); !ok {
			t.Errorf("topology table names %q, which actionCapabilities does not register", kind)
		}
	}
	if topology, _ := ActionTopology(dataquery.DataActionExpandRecords); topology != dataquery.ActionTopologyOneToMany {
		t.Fatalf("expand_records topology=%q, want 1:N", topology)
	}
	if topology, _ := ActionTopology(dataquery.DataActionGroupRecords); topology != dataquery.ActionTopologyManyToOne {
		t.Fatalf("group_records topology=%q, want N:1", topology)
	}
	if topology, _ := ActionTopology(dataquery.DataActionFilterRecords); topology != dataquery.ActionTopologyOneToOne {
		t.Fatalf("filter_records topology=%q, want 1:1", topology)
	}
}
