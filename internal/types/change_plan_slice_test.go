package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestDeriveChangePlanSlicesTinyPlan(t *testing.T) {
	plan := &ChangePlan{Changes: []FileChange{
		{Path: "src/a.py", Kind: "patch"},
		{Path: "tests/test_a.py", Kind: "patch"},
	}}
	got := DeriveChangePlanSlices(plan, ChangePlanSliceOptions{MaxChangesPerSlice: 4})
	if len(got) != 1 {
		t.Fatalf("expected one slice, got %+v", got)
	}
	if got[0].ID != "slice-001" || got[0].Status != ChangePlanSlicePending {
		t.Fatalf("unexpected slice identity/status: %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].ChangeIndexes, []int{0, 1}) {
		t.Fatalf("unexpected change indexes: %+v", got[0].ChangeIndexes)
	}
	if !reflect.DeepEqual(got[0].Paths, []string{"src/a.py", "tests/test_a.py"}) {
		t.Fatalf("unexpected paths: %+v", got[0].Paths)
	}
	if violations := ValidateChangePlanSlices(plan, got); len(violations) != 0 {
		t.Fatalf("derived slices should validate: %v", violations)
	}
}

func TestDeriveChangePlanSlicesSplitsLargePlan(t *testing.T) {
	plan := &ChangePlan{Changes: []FileChange{
		{Path: "a.py"}, {Path: "b.py"}, {Path: "c.py"}, {Path: "d.py"}, {Path: "e.py"},
	}}
	got := DeriveChangePlanSlices(plan, ChangePlanSliceOptions{MaxChangesPerSlice: 2})
	if len(got) != 3 {
		t.Fatalf("expected three slices, got %+v", got)
	}
	if !reflect.DeepEqual(got[0].ChangeIndexes, []int{0, 1}) ||
		!reflect.DeepEqual(got[1].ChangeIndexes, []int{2, 3}) ||
		!reflect.DeepEqual(got[2].ChangeIndexes, []int{4}) {
		t.Fatalf("unexpected slice grouping: %+v", got)
	}
}

func TestDeriveChangePlanSlicesRecordsDependencySlices(t *testing.T) {
	plan := &ChangePlan{Changes: []FileChange{
		{Path: "pkg/new_api.py"},
		{Path: "pkg/caller.py", DependsOn: []string{"pkg/new_api.py"}},
		{Path: "tests/test_caller.py", DependsOn: []string{"pkg/caller.py"}},
	}}
	got := DeriveChangePlanSlices(plan, ChangePlanSliceOptions{MaxChangesPerSlice: 1})
	if len(got) != 3 {
		t.Fatalf("expected three slices, got %+v", got)
	}
	if !reflect.DeepEqual(got[1].DependsOnSlices, []string{"slice-001"}) {
		t.Fatalf("slice 2 dependency not recorded: %+v", got[1].DependsOnSlices)
	}
	if !reflect.DeepEqual(got[2].DependsOnSlices, []string{"slice-002"}) {
		t.Fatalf("slice 3 dependency not recorded: %+v", got[2].DependsOnSlices)
	}
	if violations := ValidateChangePlanSlices(plan, got); len(violations) != 0 {
		t.Fatalf("derived dependency slices should validate: %v", violations)
	}
}

func TestDeriveChangePlanSlicesIsolatesPolicyPath(t *testing.T) {
	plan := &ChangePlan{Changes: []FileChange{
		{Path: "src/a.py"},
		{Path: ".github/workflows/ci.yml"},
		{Path: "src/b.py"},
	}}
	got := DeriveChangePlanSlices(plan, ChangePlanSliceOptions{
		MaxChangesPerSlice: 4,
		IsolatedPaths:      []string{".github/workflows/ci.yml"},
	})
	if len(got) != 3 {
		t.Fatalf("expected isolated path to force three slices, got %+v", got)
	}
	if !reflect.DeepEqual(got[1].Paths, []string{".github/workflows/ci.yml"}) {
		t.Fatalf("isolated path should be alone: %+v", got[1].Paths)
	}
}

func TestNormalizeChangePlanSlicesDerivesPathsAndDeps(t *testing.T) {
	plan := &ChangePlan{
		Changes: []FileChange{
			{Path: "src/a.py"},
			{Path: "src/b.py", DependsOn: []string{"src/a.py"}},
		},
		Slices: []ChangePlanSlice{{
			ID:            " first ",
			Status:        ChangePlanSliceVerified,
			ChangeIndexes: []int{0, 0, -1, 99},
			Paths:         []string{"ignored-by-normalizer"},
		}, {
			ID:            " second ",
			ChangeIndexes: []int{1},
		}, {
			ID: " empty ",
		}},
	}
	got := NormalizeChangePlanSlices(plan, ChangePlanSliceOptions{})
	if len(got) != 2 {
		t.Fatalf("expected two normalized slices, got %+v", got)
	}
	if got[0].ID != "first" || got[0].Status != ChangePlanSliceVerified ||
		!reflect.DeepEqual(got[0].ChangeIndexes, []int{0}) ||
		!reflect.DeepEqual(got[0].Paths, []string{"src/a.py"}) {
		t.Fatalf("first slice not normalized: %+v", got[0])
	}
	if got[1].Status != ChangePlanSlicePending ||
		!reflect.DeepEqual(got[1].DependsOnSlices, []string{"first"}) {
		t.Fatalf("second slice not normalized with dependency: %+v", got[1])
	}
}

func TestValidateChangePlanSlicesRejectsLaterDependency(t *testing.T) {
	plan := &ChangePlan{Changes: []FileChange{
		{Path: "dependent.py", DependsOn: []string{"prereq.py"}},
		{Path: "prereq.py"},
	}}
	slices := []ChangePlanSlice{
		{ID: "slice-001", Status: ChangePlanSlicePending, ChangeIndexes: []int{0}},
		{ID: "slice-002", Status: ChangePlanSlicePending, ChangeIndexes: []int{1}},
	}
	violations := ValidateChangePlanSlices(plan, slices)
	if len(violations) == 0 {
		t.Fatal("expected later dependency violation")
	}
	if !strings.Contains(strings.Join(violations, "\n"), "depends on later slice") {
		t.Fatalf("unexpected violations: %v", violations)
	}
}
