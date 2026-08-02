package types

import "testing"

func TestImpactObligationSetFromChangePlanDerivesPlanSignals(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-impact",
		Changes: []FileChange{{
			Path:      "caller.py",
			Kind:      "modify",
			DependsOn: []string{"helper.py"},
		}, {
			Path:    "old.py",
			NewPath: "new.py",
			Kind:    "rename",
		}},
		VerificationProbes: []VerificationProbe{{
			ID:                "probe-contract",
			ContractRefs:      []string{"contract-1"},
			ChangedSymbolRefs: []string{"pkg.symbol"},
		}},
		Slices: []ChangePlanSlice{{
			ID:                "slice-001",
			ContractRefs:      []string{"slice-contract"},
			ChangedSymbolRefs: []string{"slice.symbol"},
		}},
	}
	set := ImpactObligationSetFromChangePlan(plan)
	checks := []ImpactObligation{
		{Kind: "changed_file", Relation: "change", Obligation: "verify_changed_file", SubjectPath: "caller.py"},
		{Kind: "dependency", Relation: "depends_on", Obligation: "verify_dependency_order", SubjectPath: "caller.py", RelatedPath: "helper.py"},
		{Kind: "changed_file", Relation: "rename_target", Obligation: "verify_changed_file", SubjectPath: "new.py", RelatedPath: "old.py"},
		{Kind: "behavior_contract", Relation: "contract_ref", Obligation: "verify_contract_ref", ContractRef: "contract-1", VerificationProbe: "probe-contract"},
		{Kind: "changed_symbol", Relation: "symbol_ref", Obligation: "verify_changed_symbol", SubjectSymbol: "pkg.symbol", VerificationProbe: "probe-contract"},
		{Kind: "behavior_contract", Relation: "slice_contract_ref", Obligation: "verify_contract_ref", ContractRef: "slice-contract", SliceID: "slice-001"},
	}
	for _, want := range checks {
		if !impactObligationSetHas(set, want) {
			t.Fatalf("missing obligation %+v in %+v", want, set.Obligations)
		}
	}
}

func TestImpactObligationSetFromChangePlanKeepsPathRefsOutOfSymbolLane(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-path-ref",
		Changes: []FileChange{{
			Path: "pkg/widget.ts",
			Kind: "patch",
		}},
		VerificationProbes: []VerificationProbe{{
			ID:                "probe-widget",
			ChangedSymbolRefs: []string{"path:pkg/widget.ts", "Widget.render"},
		}},
	}
	set := ImpactObligationSetFromChangePlan(plan)
	if !impactObligationSetHas(set, ImpactObligation{
		Kind:              "changed_file",
		Relation:          "path_ref",
		Obligation:        "verify_changed_file",
		SubjectPath:       "pkg/widget.ts",
		VerificationProbe: "probe-widget",
	}) {
		t.Fatalf("missing probe path obligation: %+v", set.Obligations)
	}
	if !impactObligationSetHas(set, ImpactObligation{
		Kind:              "changed_symbol",
		Relation:          "symbol_ref",
		Obligation:        "verify_changed_symbol",
		SubjectSymbol:     "Widget.render",
		VerificationProbe: "probe-widget",
	}) {
		t.Fatalf("missing symbol obligation: %+v", set.Obligations)
	}
	for _, ob := range set.Obligations {
		if ob.SubjectSymbol == "path:pkg/widget.ts" {
			t.Fatalf("path identity leaked into symbol lane: %+v", ob)
		}
	}
}

func impactObligationSetHas(set ImpactObligationSet, want ImpactObligation) bool {
	want = impactObligation(want)
	for _, got := range set.Obligations {
		if got.Kind == want.Kind &&
			got.Relation == want.Relation &&
			got.Obligation == want.Obligation &&
			got.SubjectPath == want.SubjectPath &&
			got.RelatedPath == want.RelatedPath &&
			got.SubjectSymbol == want.SubjectSymbol &&
			got.ContractRef == want.ContractRef &&
			got.VerificationProbe == want.VerificationProbe &&
			got.SliceID == want.SliceID {
			return true
		}
	}
	return false
}
