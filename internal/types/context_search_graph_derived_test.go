package types

import "testing"

func TestSearchGraphDerivedIsClearedWithBaseGraphReplacement(t *testing.T) {
	mutable := NewMutableState("derived graph cache lifecycle")
	firstGraph := &struct{ name string }{"first"}
	mutable.SetSearchGraph(firstGraph)
	derived := &struct{ count int }{3}
	mutable.SetSearchGraphDerived("flow:v1", derived)
	if got := mutable.SearchGraphDerived("flow:v1"); got != derived {
		t.Fatalf("derived index was not retained with its base graph: %#v", got)
	}

	mutable.SetSearchGraph(&struct{ name string }{"second"})
	if got := mutable.SearchGraphDerived("flow:v1"); got != nil {
		t.Fatalf("derived index outlived replacement base graph: %#v", got)
	}
}
