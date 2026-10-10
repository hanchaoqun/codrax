package types

import (
	"math"
	"reflect"
	"testing"
)

func TestAllocatePresentationRowsCompleteSmallCollections(t *testing.T) {
	groups := []PresentationRowGroup{{Key: "a", Rows: 9}, {Key: "b", Rows: 3}, {Key: "c", Rows: 6}, {Key: "d", Rows: 1}}
	for _, budget := range []int{19, 128} {
		if got := AllocatePresentationRows(groups, budget); !reflect.DeepEqual(got, []int{9, 3, 6, 1}) {
			t.Fatalf("complete small collection capped by independent row limits: %v", got)
		}
	}
}

func TestAllocatePresentationRowsParentThenGroupCoverage(t *testing.T) {
	groups := []PresentationRowGroup{
		{Key: "a1", ParentKey: "a", Rows: 20}, {Key: "a2", ParentKey: "a", Rows: 20}, {Key: "a3", ParentKey: "a", Rows: 20},
		{Key: "b1", ParentKey: "b", Rows: 20},
	}
	for _, test := range []struct {
		budget int
		want   []int
	}{{2, []int{1, 0, 0, 1}}, {4, []int{1, 1, 1, 1}}, {8, []int{2, 2, 2, 2}}} {
		if got := AllocatePresentationRows(groups, test.budget); !reflect.DeepEqual(got, test.want) {
			t.Errorf("budget %d: got %v want %v", test.budget, got, test.want)
		}
	}
}

func TestAllocatePresentationRowsFillByPriorityWithoutDroppingSmallGroups(t *testing.T) {
	groups := []PresentationRowGroup{{Key: "large", Rows: 100}, {Key: "small", Rows: 3}, {Key: "preferred", Rows: 4, Priority: -1}}
	if got := AllocatePresentationRows(groups, 8); !reflect.DeepEqual(got, []int{1, 3, 4}) {
		t.Fatal(got)
	}
	// Key is caller metadata, not a deduplication key; neither repeated keys
	// nor absent parent identities are inferred to be the same source/table.
	groups = []PresentationRowGroup{{Key: "same", Rows: 8}, {Key: "same", Rows: 3}}
	if got := AllocatePresentationRows(groups, 6); !reflect.DeepEqual(got, []int{3, 3}) {
		t.Fatal(got)
	}
}

func TestAllocatePresentationRowsBoundsAndImmutability(t *testing.T) {
	groups := []PresentationRowGroup{{Rows: -1}, {Rows: 0}, {Rows: math.MaxInt}, {Rows: math.MaxInt}}
	before := append([]PresentationRowGroup(nil), groups...)
	for _, budget := range []int{-1, 0, 1, 2, 3, 128, math.MaxInt} {
		got, remaining := AllocatePresentationRows(groups, budget), max(0, budget)
		for i, count := range got {
			if count < 0 || count > max(0, groups[i].Rows) || count > remaining {
				t.Fatalf("invalid allocation: %v budget %d", got, budget)
			}
			remaining -= count
		}
		if remaining != 0 {
			t.Fatalf("capacity was left unused: %d", remaining)
		}
	}
	if !reflect.DeepEqual(before, groups) {
		t.Fatal("allocator mutated producer data")
	}
}
