package skill

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSharedGuidanceOwnership_BindsExactBodyAndDoesNotSerialize(t *testing.T) {
	plain := TierBItem{Body: "original contract", AppliesTo: AppliesToFilter{RequiresTrace: true}}
	owned := plain.withSharedGuidance(TraceQueryViewMatrixGuidance)
	if plain.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) || !owned.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) {
		t.Fatal("only internally registered original body may own the shared contract")
	}
	if owned.ProvidesSharedGuidance(SharedGuidanceID("another_section")) {
		t.Fatal("section ownership must not apply to a different contract")
	}
	changed := owned
	changed.Body += " changed"
	if changed.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) || !owned.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) {
		t.Fatal("copy-and-replace must revoke only the changed body's ownership")
	}
	for _, tc := range []struct {
		name    string
		marshal func(any) ([]byte, error)
	}{
		{"json", json.Marshal}, {"yaml", yaml.Marshal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := tc.marshal(plain)
			if err != nil {
				t.Fatal(err)
			}
			after, err := tc.marshal(owned)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("private ownership changed serialized skill contract: %s / %s", before, after)
			}
		})
	}
	var loaded TierBItem
	if err := yaml.Unmarshal([]byte("body: custom\nsharedGuidance: trace_query_view_matrix\nshared_guidance: trace_query_view_matrix\n"), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) {
		t.Fatal("custom YAML must not be able to claim internal ownership")
	}
}

func TestExploreDefault_TraceMatrixOwnerContainsActualContract(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)
	sk, err := registry.Get("explore-skill")
	if err != nil {
		t.Fatal(err)
	}
	owners := 0
	for _, item := range sk.WorkflowTierB {
		if item.ProvidesSharedGuidance(TraceQueryViewMatrixGuidance) {
			owners++
			if strings.Count(item.Body, RenderTraceQueryViewMatrix()) != 1 {
				t.Fatal("registered owner must actually contain the complete generated matrix exactly once")
			}
		}
	}
	if owners != 1 {
		t.Fatalf("got %d owners, want one", owners)
	}
}
