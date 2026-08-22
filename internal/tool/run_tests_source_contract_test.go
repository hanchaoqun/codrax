package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPostApplySourceContractConfidenceRecordsObservesPlanOwnedExactLine(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.c"), []byte("void greet(void) {\n    char *buf = 0;\n    return buf;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := sourceContractTestPlan("src/main.c", types.WriteBehaviorContract{
		ID: "return-line", Kind: types.WriteBehaviorObservable,
		Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals,
		Expected: "return buf;", EvidenceRef: "src/main.c:3", Required: true,
	})

	records := postApplySourceContractConfidenceRecords(root, plan)
	if len(records) != 1 || records[0].Category != "source_contract_refs" ||
		records[0].Status != "satisfied" || records[0].ReasonCode != "post_apply_source_contract_observed" ||
		len(records[0].ContractRefs) != 1 || records[0].ContractRefs[0] != "return-line" {
		t.Fatalf("exact plan-owned source contract was not observed: %+v", records)
	}
}

func TestPostApplySourceContractConfidenceRecordsReportsExactLineMismatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.c"), []byte("retrun buf;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := sourceContractTestPlan("main.c", types.WriteBehaviorContract{
		ID: "return-line", Kind: types.WriteBehaviorInvariant,
		Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals,
		Expected: "return buf;", EvidenceRef: "main.c:1", Required: true,
	})

	records := postApplySourceContractConfidenceRecords(root, plan)
	if len(records) != 1 || records[0].Status != "missing" ||
		records[0].ReasonCode != "post_apply_source_contract_value_mismatch" {
		t.Fatalf("exact source mismatch was not retained as typed debt: %+v", records)
	}
}

func TestPostApplySourceContractConfidenceRecordsRejectsRuntimeRangeAndUnownedContracts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.c"), []byte("return buf;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := &types.ChangePlan{
		ID: "plan-source-boundaries", TargetPaths: []string{"main.c"},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "runtime-return", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpReturns, Expected: "buf", EvidenceRef: "main.c:1", Required: true},
			{ID: "line-range", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpContains, Expected: "return", EvidenceRef: "main.c:1-2", Required: true},
			{ID: "unowned", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
				Operator: types.WriteBehaviorOpEquals, Expected: "return buf;", EvidenceRef: "other.c:1", Required: true},
		},
	}
	if records := postApplySourceContractConfidenceRecords(root, plan); len(records) != 0 {
		t.Fatalf("runtime, range, or unowned contracts gained source authority: %+v", records)
	}
}

func TestPostApplySourceContractConfidenceRecordsRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.c")
	if err := os.WriteFile(outside, []byte("return buf;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "main.c")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	plan := sourceContractTestPlan("main.c", types.WriteBehaviorContract{
		ID: "escaped", Kind: types.WriteBehaviorObservable,
		Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals,
		Expected: "return buf;", EvidenceRef: "main.c:1", Required: true,
	})
	if records := postApplySourceContractConfidenceRecords(root, plan); len(records) != 0 {
		t.Fatalf("symlink escape gained source authority: %+v", records)
	}
}

func sourceContractTestPlan(path string, contract types.WriteBehaviorContract) *types.ChangePlan {
	return &types.ChangePlan{
		ID:                "plan-source-contract",
		TargetPaths:       []string{path},
		BehaviorContracts: []types.WriteBehaviorContract{contract},
	}
}
