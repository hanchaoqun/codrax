package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// newTestBusCtx returns a minimal BusContext with a writable
// MutableState. No repo, no logger, no work dir — exactly what
// emit_change_plan.Execute needs and nothing more.
func newTestBusCtx() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("test request"),
	}
}

// TestEmitChangePlan_HappyPath locks the canonical success path:
// valid params in → ChangePlan installed on Mutable + PendingApply
// entries enqueued on WriteClosure + ToolResult.Success == true.
func TestEmitChangePlan_HappyPath(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a comment to main.go",
		"summary": "Add a one-line header comment to main.go explaining the binary's role. Trivial change; no behavior impact.",
		"changes": [
			{"path": "main.go", "kind": "modify", "new_content": "// codrax entry point\npackage main\n", "rationale": "add a brief header comment"}
		],
		"acceptance_tests": ["go build ./... passes"]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got false with summary: %s", res.Summary)
	}

	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if plan.Summary == "" {
		t.Error("plan Summary should not be empty")
	}
	if len(plan.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(plan.Changes))
	}
	if plan.Changes[0].Path != "main.go" {
		t.Errorf("change path = %q, want main.go", plan.Changes[0].Path)
	}
	if plan.Status != "pending_approval" {
		t.Errorf("status = %q, want pending_approval", plan.Status)
	}
	if !strings.HasPrefix(plan.ID, "plan-") {
		t.Errorf("plan ID = %q, want prefix 'plan-'", plan.ID)
	}
	if len(plan.TargetPaths) != 1 {
		t.Errorf("target_paths = %v, want 1 entry", plan.TargetPaths)
	}
	if len(plan.AcceptanceTests) != 1 {
		t.Errorf("acceptance_tests = %v, want 1 entry", plan.AcceptanceTests)
	}

	// WriteClosure should hold PendingApplies matching the changes.
	wc := ctx.Mutable.WriteClosure()
	pending := wc.PendingApplies()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending apply, got %d", len(pending))
	}
	if len(pending) > 0 && pending[0].Path != "main.go" {
		t.Errorf("pending apply path = %q, want main.go", pending[0].Path)
	}
}

func TestEmitChangePlan_StructuredPayloadCompatRepairsStringArrays(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a comment to main.go",
		"summary": "Add a one-line header comment to main.go explaining the binary's role. Trivial change; no behavior impact.",
		"changes": "[{\"path\":\"main.go\",\"kind\":\"modify\",\"new_content\":\"// codrax entry point\\npackage main\\n\",\"rationale\":\"add a brief header comment\",\"depends_on\":\"[]\"}]",
		"acceptance_tests": "[\"go build ./... passes\"]"
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 1 {
		t.Fatalf("ChangePlan not installed from repaired payload: %+v", plan)
	}
	if plan.Changes[0].Path != "main.go" || !strings.Contains(plan.Changes[0].NewContent, "package main") {
		t.Fatalf("change row was not preserved after compat repair: %+v", plan.Changes[0])
	}
	if len(plan.AcceptanceTests) != 1 || plan.AcceptanceTests[0] != "go build ./... passes" {
		t.Fatalf("acceptance tests were not repaired: %+v", plan.AcceptanceTests)
	}
}

func TestEmitChangePlan_PersistsVerificationProbesInFingerprint(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a small behavioural probe so verify can still check the contract when project pytest infrastructure is unavailable.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "timeout_seconds": 3}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.VerificationProbes) != 1 {
		t.Fatalf("verification probes not persisted: %+v", plan.VerificationProbes)
	}
	fingerprintWithProbe := types.PlanFingerprint(plan)
	plan.VerificationProbes = nil
	fingerprintWithoutProbe := types.PlanFingerprint(plan)
	if fingerprintWithProbe == "" || fingerprintWithProbe == fingerprintWithoutProbe {
		t.Fatalf("plan fingerprint must include verification probes, with=%q without=%q", fingerprintWithProbe, fingerprintWithoutProbe)
	}
}

func TestEmitChangePlan_MergesChangeLocalVerificationProbes(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and keep the bounded behavioural probe next to the change that it exercises.",
		"changes": [
			{
				"path": "widget.py",
				"kind": "modify",
				"new_content": "VALUE = 42\n",
				"rationale": "set the corrected value",
				"verification_probes": [
					{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "changed_symbol_refs": ["widget.VALUE"]}
				]
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.VerificationProbes) != 1 {
		t.Fatalf("change-local verification probes were not merged: %+v", plan.VerificationProbes)
	}
	probe := plan.VerificationProbes[0]
	if probe.ID != "value_contract" || len(probe.ChangedSymbolRefs) != 1 || probe.ChangedSymbolRefs[0] != "widget.VALUE" {
		t.Fatalf("unexpected merged probe: %+v", probe)
	}
}

func TestEmitChangePlan_RepairsStringWrappedChangesArray(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	changes := `[{"path":"main.py","kind":"modify","new_content":"VALUE = 1\n","rationale":"set the corrected value"}]`
	params := json.RawMessage(`{
		"request": "fix a python value",
		"summary": "Modify main.py with a small deterministic value update.",
		"changes": ` + jsonString(changes) + `
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected string-wrapped changes[] to be repaired, got summary: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.Changes) != 1 || plan.Changes[0].Path != "main.py" {
		t.Fatalf("repaired changes[] did not install expected plan: %+v", plan)
	}
}

func TestEmitChangePlan_PersistsBehaviorContractsAndProbeRefs(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "fix widget value"},
			BehaviorContracts: []types.WriteBehaviorContract{{
				ID:       "widget-value",
				Kind:     types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpEquals,
				Subject:  "widget.VALUE",
				Expected: "42",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	})
	params := json.RawMessage(`{
		"request": "fix widget value",
		"summary": "Modify widget.py and attach a bounded probe that imports the changed module and verifies the typed contract.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "contract_refs": ["widget-value"], "changed_symbol_refs": ["widget.VALUE"], "expects_baseline_failure": true}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.BehaviorContracts) != 1 || plan.BehaviorContracts[0].ID != "widget-value" {
		t.Fatalf("behavior contract snapshot missing: %+v", plan.BehaviorContracts)
	}
	if len(plan.VerificationProbes) != 1 {
		t.Fatalf("verification probe missing: %+v", plan.VerificationProbes)
	}
	probe := plan.VerificationProbes[0]
	if len(probe.ContractRefs) != 1 || probe.ContractRefs[0] != "widget-value" {
		t.Fatalf("contract refs not preserved: %+v", probe)
	}
	if len(probe.ChangedSymbolRefs) != 1 || probe.ChangedSymbolRefs[0] != "widget.VALUE" || !probe.ExpectsBaselineFailure {
		t.Fatalf("changed-symbol/baseline metadata not preserved: %+v", probe)
	}
}

func TestEmitChangePlan_RejectsUnknownBehaviorContractRef(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "fix widget value"},
			BehaviorContracts: []types.WriteBehaviorContract{{
				ID:       "widget-value",
				Kind:     types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpEquals,
				Expected: "42",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	})
	params := json.RawMessage(`{
		"request": "fix widget value",
		"summary": "Modify widget.py and attach a bounded probe with a stale contract reference.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "contract_refs": ["missing-contract"]}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected unknown behavior contract ref to be rejected")
	}
	if !strings.Contains(res.Summary, "unknown behavior_contract id") {
		t.Fatalf("summary should name unknown contract ref, got: %s", res.Summary)
	}
}

func TestEmitChangePlan_AcceptsProbeWithoutExplicitRequiredContractRefs(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "fix widget value"},
			BehaviorContracts: []types.WriteBehaviorContract{{
				ID:       "widget-value",
				Kind:     types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpEquals,
				Expected: "42",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	})
	params := json.RawMessage(`{
		"request": "fix widget value",
		"summary": "Modify widget.py and attach a bounded probe that forgot to name which contract it covers.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected plan emit to accept structurally valid probe metadata, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.BehaviorContracts) != 1 || plan.BehaviorContracts[0].ID != "widget-value" {
		t.Fatalf("behavior contract snapshot missing: %+v", plan.BehaviorContracts)
	}
	if len(plan.VerificationProbes) != 1 || len(plan.VerificationProbes[0].ContractRefs) != 0 {
		t.Fatalf("probe contract refs should remain exactly as emitted for verify confidence accounting: %+v", plan.VerificationProbes)
	}
}

func TestEmitChangePlan_AcceptsPartialRequiredContractProbeCoverage(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopePackage, Summary: "fix widget value and label"},
			BehaviorContracts: []types.WriteBehaviorContract{{
				ID:       "widget-value",
				Kind:     types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpEquals,
				Expected: "42",
				Required: true,
				Source:   "write_analyzer",
			}, {
				ID:       "outcome-1",
				Kind:     types.WriteBehaviorObservable,
				Operator: types.WriteBehaviorOpSatisfies,
				Expected: "label remains visible",
				Required: true,
				Source:   "expected_outcome_fallback",
			}},
		},
	})
	params := json.RawMessage(`{
		"request": "fix widget value and label",
		"summary": "Modify widget.py and attach a bounded probe that only covers one required contract.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "code": "import widget\nassert widget.VALUE == 42\n", "contract_refs": ["widget-value"]}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected plan emit to accept partial probe coverage and defer confidence to verify, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed on Mutable")
	}
	if len(plan.BehaviorContracts) != 2 {
		t.Fatalf("behavior contract snapshot missing: %+v", plan.BehaviorContracts)
	}
	if len(plan.VerificationProbes) != 1 || len(plan.VerificationProbes[0].ContractRefs) != 1 || plan.VerificationProbes[0].ContractRefs[0] != "widget-value" {
		t.Fatalf("partial contract refs should be preserved for verify confidence accounting: %+v", plan.VerificationProbes)
	}
}

func TestEmitChangePlan_RejectsEscapingVerificationProbeWorkingDir(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a small behavioural probe so verify can still check the contract when project pytest infrastructure is unavailable.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "value_contract", "language": "python", "working_dir": "../outside", "code": "import widget\nassert widget.VALUE == 42\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected escaping verification probe working_dir to be rejected")
	}
	if !strings.Contains(res.Summary, "escapes the repository") {
		t.Fatalf("summary should mention repository escape, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected probe must not install a plan: %+v", plan)
	}
}

func TestEmitChangePlan_RejectsPythonProbeWithoutExecutableFailureSignal(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a behavioural probe that only prints failure text.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "prints_fail", "language": "python", "code": "import widget\nif widget.VALUE != 42:\n    print('FAIL: wrong value')\nelse:\n    print('PASS')\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected print-only verification probe to be rejected")
	}
	if !strings.Contains(res.Summary, "executable failure signal") {
		t.Fatalf("summary should explain missing executable failure signal, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected probe must not install a plan: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeWithExpectedStdoutWithoutAssert(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a stdout-contract probe.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "stdout_contract", "language": "python", "code": "import widget\nprint('VALUE=%s' % widget.VALUE)\n", "expected_stdout": ["VALUE=42"]}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected stdout-contract probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one probe, got: %+v", plan)
	}
	if got := plan.VerificationProbes[0].ExpectedStdout; len(got) != 1 || got[0] != "VALUE=42" {
		t.Fatalf("expected_stdout not preserved: %+v", got)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeWithInlineRaiseFailureSignal(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix a python behaviour",
		"summary": "Modify widget.py and attach a probe that raises on failed behavior.",
		"changes": [
			{"path": "widget.py", "kind": "modify", "new_content": "VALUE = 42\n", "rationale": "set the corrected value"}
		],
		"verification_probes": [
			{"id": "inline_raise", "language": "python", "code": "import widget\nif widget.VALUE != 42: raise AssertionError('wrong value')\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected inline-raise probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one probe, got: %+v", plan)
	}
}

func TestEmitChangePlan_RejectsPythonProbeThatCopiesImplementationInsteadOfImportingTarget(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix option directive parsing",
		"summary": "Modify the Sphinx option parser and attach a probe. The probe must exercise the changed production module instead of copying the proposed regular expression into the probe.",
		"changes": [
			{"path": "sphinx/domains/std.py", "kind": "modify", "new_content": "option_desc_re = None\n", "rationale": "update option parsing"}
		],
		"verification_probes": [
			{"id": "copied_regex", "language": "python", "code": "import re\nfixed_re = re.compile(r'(\\[[^\\s=]+=\\])([^\\s=[]+)(=?\\s*.*)')\nassert fixed_re.match('[enable=]PATTERN')\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected copied implementation probe to be rejected")
	}
	if !strings.Contains(res.Summary, "do not import any changed Python production module") {
		t.Fatalf("summary should explain changed-module probe coupling, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected probe must not install a plan: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeImportingChangedTargetModule(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix option directive parsing",
		"summary": "Modify the Sphinx option parser and attach a probe that imports the changed production module before asserting behavior.",
		"changes": [
			{"path": "sphinx/domains/std.py", "kind": "modify", "new_content": "option_desc_re = None\n", "rationale": "update option parsing"}
		],
		"verification_probes": [
			{"id": "module_behavior", "language": "python", "code": "from sphinx.domains import std\nassert std.option_desc_re.match('[enable=]PATTERN')\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected changed-module probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one probe, got: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsExtraPythonProbeWhenAnotherProbeImportsChangedTarget(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix option directive parsing",
		"summary": "Modify the Sphinx option parser with one coupled behavior probe plus an extra smoke probe.",
		"changes": [
			{"path": "sphinx/domains/std.py", "kind": "modify", "new_content": "option_desc_re = None\n", "rationale": "update option parsing"}
		],
		"verification_probes": [
			{"id": "module_behavior", "language": "python", "code": "from sphinx.domains import std\nassert std.option_desc_re is not None\n"},
			{"id": "local_regex_smoke", "language": "python", "code": "import re\nassert re.match(r'.+', 'x')\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected plan with at least one changed-module probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 2 {
		t.Fatalf("expected accepted plan with two probes, got: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeImportingPublicPackageForChangedSubmodule(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix xarray public API roundtrip",
		"summary": "Modify DataArray.to_unstacked_dataset and verify the fix through the public xarray API.",
		"changes": [
			{"path": "xarray/core/dataarray.py", "kind": "modify", "new_content": "class DataArray:\n    pass\n", "rationale": "update DataArray behavior"}
		],
		"verification_probes": [
			{"id": "public_api_roundtrip", "language": "python", "code": "import xarray as xr\narr = xr.DataArray([1])\nassert arr is not None\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected public package import to cover changed submodule, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one public-API probe, got: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeImportingSiblingPublicAPIForChangedSubmodule(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "fix seaborn objects boolean color handling",
		"summary": "Modify the core scale code and verify it through the public objects API.",
		"changes": [
			{"path": "seaborn/_core/scales.py", "kind": "modify", "new_content": "class ContinuousBase:\n    pass\n", "rationale": "update scale handling"}
		],
		"verification_probes": [
			{"id": "objects_boolean_color", "language": "python", "code": "import seaborn.objects as so\nassert so is not None\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected sibling public API import to cover changed submodule, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one public-API probe, got: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonProbeImportingRepoLocalSiblingPublicPackage(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	for _, path := range []string{
		"lib/matplotlib",
		"lib/mpl_toolkits",
		"lib/mpl_toolkits/mplot3d",
	} {
		if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path), "__init__.py"), []byte(""), 0o644); err != nil {
			t.Fatalf("write %s __init__: %v", path, err)
		}
	}
	params := json.RawMessage(`{
		"request": "fix 3D axes visibility",
		"summary": "Modify Axes3D.draw and verify the behavior through Matplotlib's public pyplot entrypoint, which lives in a sibling top-level package under the same lib source root.",
		"changes": [
			{"path": "lib/mpl_toolkits/mplot3d/axes3d.py", "kind": "modify", "new_content": "class Axes3D:\n    def draw(self, renderer):\n        if not self.get_visible():\n            return\n", "rationale": "add the visibility guard"}
		],
		"verification_probes": [
			{"id": "public_pyplot_entrypoint", "language": "python", "code": "import matplotlib.pyplot as plt\nfig = plt.figure()\nassert fig is not None\n"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected repo-local sibling public package probe to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil || len(plan.VerificationProbes) != 1 {
		t.Fatalf("expected accepted plan with one public-entrypoint probe, got: %+v", plan)
	}
}

func TestEmitChangePlan_RejectsPythonDiscardBindingNameThatIsNeverRead(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("python interpreter not available; skip Python discard-binding validation test")
	}
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "src/_pytest/_code/code.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "src/_pytest/_code"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "class TracebackEntry:\n" +
		"    def getsource(self):\n" +
		"        start = self.getfirstlinesource()\n" +
		"        try:\n" +
		"            astnode, _, end = getstatementrange_ast(\n" +
		"                self.lineno, source, astnode=astnode\n" +
		"            )\n" +
		"        except SyntaxError:\n" +
		"            end = self.lineno + 1\n" +
		"        return source[start:end]\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "fix pytest assertion source range",
		"summary": "Capture the start returned by getstatementrange_ast so assertion output can use the exact statement range.",
		"changes": [
			{
				"path": "src/_pytest/_code/code.py",
				"kind": "patch",
				"patch": "--- a/src/_pytest/_code/code.py\n+++ b/src/_pytest/_code/code.py\n@@ -2,7 +2,7 @@ class TracebackEntry:\n     def getsource(self):\n         start = self.getfirstlinesource()\n         try:\n-            astnode, _, end = getstatementrange_ast(\n+            astnode, ast_start, end = getstatementrange_ast(\n                 self.lineno, source, astnode=astnode\n             )\n         except SyntaxError:\n",
				"rationale": "capture ast_start from getstatementrange_ast"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected dead discard-binding replacement to be rejected")
	}
	for _, want := range []string{"discard binding", "ast_start", "never reads"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain %q, got: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected dead edit plan must not install ChangePlan: %+v", plan)
	}
}

func TestEmitChangePlan_AcceptsPythonDiscardBindingNameWhenRead(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("python interpreter not available; skip Python discard-binding validation test")
	}
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "src/_pytest/_code/code.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "src/_pytest/_code"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "class TracebackEntry:\n" +
		"    def getsource(self):\n" +
		"        start = self.getfirstlinesource()\n" +
		"        try:\n" +
		"            astnode, _, end = getstatementrange_ast(\n" +
		"                self.lineno, source, astnode=astnode\n" +
		"            )\n" +
		"        except SyntaxError:\n" +
		"            end = self.lineno + 1\n" +
		"        return source[start:end]\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "fix pytest assertion source range",
		"summary": "Use the start returned by getstatementrange_ast when slicing traceback source.",
		"changes": [
			{
				"path": "src/_pytest/_code/code.py",
				"kind": "patch",
				"patch": "--- a/src/_pytest/_code/code.py\n+++ b/src/_pytest/_code/code.py\n@@ -2,12 +2,11 @@ class TracebackEntry:\n     def getsource(self):\n-        start = self.getfirstlinesource()\n         try:\n-            astnode, _, end = getstatementrange_ast(\n+            astnode, ast_start, end = getstatementrange_ast(\n                 self.lineno, source, astnode=astnode\n             )\n         except SyntaxError:\n             end = self.lineno + 1\n-        return source[start:end]\n+        return source[ast_start:end]\n",
				"rationale": "use ast_start from getstatementrange_ast for the returned source slice"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected used discard-binding replacement to be accepted, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan == nil {
		t.Fatal("accepted plan should install ChangePlan")
	}
}

func TestEmitChangePlan_RejectsPythonAddedStatementAfterReturn(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "sphinx/ext/viewcode.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "sphinx/ext"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "def should_generate_module_page(app, modname):\n" +
		"    return True\n" +
		"\n" +
		"\n" +
		"def collect_pages(app):\n" +
		"    env = app.builder.env\n" +
		"    return []\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "prevent epub viewcode pages",
		"summary": "Add the epub guard to collect_pages.",
		"changes": [
			{
				"path": "sphinx/ext/viewcode.py",
				"kind": "patch",
				"patch": "diff --git a/sphinx/ext/viewcode.py b/sphinx/ext/viewcode.py\n--- a/sphinx/ext/viewcode.py\n+++ b/sphinx/ext/viewcode.py\n@@ -1,6 +1,8 @@\n def should_generate_module_page(app, modname):\n     return True\n \n+    if app.builder.name.startswith(\"epub\"):\n+        return []\n \n def collect_pages(app):\n     env = app.builder.env\n",
				"rationale": "guard epub generation"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected unreachable Python insertion to be rejected")
	}
	for _, want := range []string{"unreachable", "after a terminal statement", "wrong function"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain %q, got: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected unreachable edit plan must not install ChangePlan: %+v", plan)
	}
}

func TestEmitChangePlan_AllowsPythonAddedStatementBeforeReturn(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "sphinx/ext/viewcode.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "sphinx/ext"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "def collect_pages(app):\n" +
		"    env = app.builder.env\n" +
		"    return []\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "prevent epub viewcode pages",
		"summary": "Add the epub guard to collect_pages before returning pages.",
		"changes": [
			{
				"path": "sphinx/ext/viewcode.py",
				"kind": "patch",
				"patch": "diff --git a/sphinx/ext/viewcode.py b/sphinx/ext/viewcode.py\n--- a/sphinx/ext/viewcode.py\n+++ b/sphinx/ext/viewcode.py\n@@ -1,3 +1,5 @@\n def collect_pages(app):\n     env = app.builder.env\n+    if app.builder.name.startswith(\"epub\"):\n+        return []\n     return []\n",
				"rationale": "guard epub generation before returning collected pages"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected reachable Python insertion to be accepted, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan == nil {
		t.Fatal("accepted reachable edit should install ChangePlan")
	}
}

func TestEmitChangePlan_RejectsStructuredEditDuplicatePythonDefinitionStutter(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "lib/mpl_toolkits/mplot3d/axes3d.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "lib/mpl_toolkits/mplot3d"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte("class Axes3D:\n    @allow_rasterization\n    def draw(self, renderer):\n        self._unstale_viewLim()\n        self.patch.draw(renderer)\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "fix 3D axes visibility",
		"summary": "Modify Axes3D.draw by adding a visibility guard.",
		"changes": [
			{
				"path": "lib/mpl_toolkits/mplot3d/axes3d.py",
				"kind": "patch",
				"edits": [
					{
						"kind": "insert_before",
						"start_line": 3,
						"old_text": "    def draw(self, renderer):\n",
						"content": "    def draw(self, renderer):\n        if not self.get_visible():\n            return\n        self._unstale_viewLim()\n"
					}
				],
				"rationale": "add guard before drawing"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected duplicate Python definition stutter to be rejected")
	}
	for _, want := range []string{"duplicate Python function", "draw", "replace the existing definition"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain %q, got: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected duplicate definition plan must not install ChangePlan: %+v", plan)
	}
}

func TestValidatePythonDuplicateDefinitionStutter_AllowsUnrelatedEditWithPreexistingDuplicate(t *testing.T) {
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "pkg/module.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldContent := "class Existing:\n" +
		"    def stable(self):\n" +
		"        return 1\n" +
		"\n" +
		"    def stable(self):\n" +
		"        return 2\n" +
		"\n" +
		"def target():\n" +
		"    return 'old'\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(oldContent), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	newContent := strings.Replace(oldContent, "return 'old'", "return 'new'", 1)
	change := types.FileChange{
		Path:       path,
		Kind:       "modify",
		NewContent: newContent,
	}
	if rej := validatePythonDuplicateDefinitionStutter(ctx, []types.FileChange{change}); rej != "" {
		t.Fatalf("preexisting unrelated duplicate should not reject this change: %s", rej)
	}
}

func TestEmitChangePlan_RejectsStructuredInsertContainingAnchorBlock(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.RepoRoot = t.TempDir()
	path := "pkg/module.py"
	if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oldBlock := "from os import path\nfrom sphinx.util import logging\nfrom sphinx.util.osutil import make_filename_from_project\n"
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, filepath.FromSlash(path)), []byte(oldBlock+"\nVALUE = 1\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	params := json.RawMessage(`{
		"request": "add ensuredir import",
		"summary": "Add the missing ensuredir import for the module.",
		"changes": [
			{
				"path": "pkg/module.py",
				"kind": "patch",
				"edits": [
					{
						"kind": "insert_before",
						"start_line": 1,
						"old_text": "from os import path\nfrom sphinx.util import logging\nfrom sphinx.util.osutil import make_filename_from_project\n",
						"content": "from os import path\nfrom sphinx.util import logging\nfrom sphinx.util.osutil import ensuredir\nfrom sphinx.util.osutil import make_filename_from_project\n"
					}
				],
				"rationale": "import ensuredir next to the existing osutil import"
			}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected anchor-copying structured insertion to be rejected")
	}
	for _, want := range []string{"old_text anchor", "new bytes"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should contain %q, got: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected duplicate-anchor insertion must not install ChangePlan: %+v", plan)
	}
}

func TestPythonDuplicateDefinitionStutter_AllowsConditionalCompatibilityDefinitions(t *testing.T) {
	content := `class Compat:
    if FEATURE:
        def render(self):
            return "new"
    else:
        def render(self):
            return "old"
`
	if dup, ok := findPythonDuplicateDefinitionStutter(content); ok {
		t.Fatalf("conditional compatibility definitions should not be stutter rejected: %+v", dup)
	}
}

func TestPythonDuplicateDefinitionStutter_AllowsPropertyAccessorDefinitions(t *testing.T) {
	content := `class Ticker:
    @property
    def locator(self):
        return self._locator

    @locator.setter
    def locator(self, locator):
        self._locator = locator

    @locator.deleter
    def locator(self):
        del self._locator
`
	if dup, ok := findPythonDuplicateDefinitionStutter(content); ok {
		t.Fatalf("property accessor definitions should not be stutter rejected: %+v", dup)
	}
}

func TestPythonDuplicateDefinitionStutter_AllowsTypingOverloads(t *testing.T) {
	content := `from typing import Union, overload

class Source:
    @overload
    def __getitem__(self, key: int) -> str:
        ...

    @overload
    def __getitem__(self, key: slice) -> "Source":
        ...

    def __getitem__(self, key: Union[int, slice]) -> Union[str, "Source"]:
        return self
`
	if dup, ok := findPythonDuplicateDefinitionStutter(content); ok {
		t.Fatalf("typing overload definitions should not be stutter rejected: %+v", dup)
	}
}

func TestPythonDuplicateDefinitionStutter_RejectsAdjacentDuplicateDefinitions(t *testing.T) {
	content := `class Ticker:
    def locator(self):
        return self._locator

    def locator(self):
        return self._other_locator
`
	if dup, ok := findPythonDuplicateDefinitionStutter(content); !ok {
		t.Fatal("expected adjacent duplicate definitions to be stutter rejected")
	} else if dup.Name != "locator" || dup.FirstLine != 2 || dup.SecondLine != 5 {
		t.Fatalf("unexpected duplicate report: %+v", dup)
	}
}

// TestEmitChangePlan_EmptyChangesRejected locks the hard cross-
// field check: a plan with zero changes is meaningless and must
// fail with a clear diagnostic.
func TestEmitChangePlan_EmptyChangesRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "trivial",
		"summary": "nothing to change",
		"changes": []
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty changes[]")
	}
	if !strings.Contains(res.Summary, "empty") {
		t.Errorf("summary should mention 'empty', got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Error("ChangePlan should not be installed on failed emit")
	}
}

func TestEmitChangePlan_EmptyChangesAllowedOnlyForTypedNoChangeReplan(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	ctx.Mode = types.ModeApply
	ctx.PipelineStage = types.StagePlan
	ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:  "plan-applied",
		BatchID: "batch-1",
		Attempt: 1,
	})
	ctx.Mutable.AppendPlanStageProbeReport(&types.ChangeReport{
		PlanID:  "plan-applied",
		Channel: types.ChangeReportChannelPlannerProbe,
		Passed:  true,
		TestResults: []types.TestResult{{
			AssertionID: "tests/test_cli.py::TestRoutes::test_route_registration",
			Kind:        types.TestResultKindUnit,
			Passed:      true,
		}},
	})
	params := json.RawMessage(`{
		"request": "replan after verify failure",
		"summary": "The scoped typed probe now passes against the already-applied worktree, so no additional code change should be applied.",
		"changes": []
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected typed no-change replan sentinel to be accepted, got: %s", res.Summary)
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected no-change sentinel plan installed on Mutable")
	}
	if plan.Status != types.PlanStatusNoChangeRequired {
		t.Fatalf("status = %q, want %q", plan.Status, types.PlanStatusNoChangeRequired)
	}
	if len(plan.Changes) != 0 || len(plan.TargetPaths) != 0 {
		t.Fatalf("no-change sentinel must not enqueue file changes, got changes=%d target_paths=%d", len(plan.Changes), len(plan.TargetPaths))
	}
	if pending := ctx.Mutable.WriteClosure().PendingApplies(); len(pending) != 0 {
		t.Fatalf("no-change sentinel must not enqueue pending applies: %+v", pending)
	}
}

func TestEmitChangePlan_ReplanNoOpStructuredEditPointsToTypedProbeSentinel(t *testing.T) {
	tool := &EmitChangePlan{}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := newTestBusCtx()
	ctx.Mode = types.ModeApply
	ctx.PipelineStage = types.StagePlan
	ctx.RepoRoot = root
	ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:  "plan-applied",
		BatchID: "batch-1",
		Attempt: 1,
	})
	params := json.RawMessage(`{
		"request": "replan after verify failure",
		"summary": "Try to re-emit the already-applied value change.",
		"changes": [
			{
				"path": "widget.py",
				"kind": "patch",
				"rationale": "the worktree already has this value",
				"edits": [{"kind": "replace", "start_line": 1, "end_line": 1, "old_text": "VALUE = 42\n", "content": "VALUE = 42\n"}]
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected no-op structured edit to be rejected")
	}
	if !strings.Contains(res.Summary, "run_tests(dry_run=true, verification_probe={...})") || !strings.Contains(res.Summary, types.PlanStatusNoChangeRequired) {
		t.Fatalf("replan no-op rejection should point at typed probe sentinel, got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected no-op edit must not install a plan: %+v", plan)
	}
}

func TestEmitChangePlan_ReplanOldTextMismatchWithPassedProbePointsToTypedProbeSentinel(t *testing.T) {
	tool := &EmitChangePlan{}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "widget.py"), []byte("VALUE = 42\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := newTestBusCtx()
	ctx.Mode = types.ModeApply
	ctx.PipelineStage = types.StagePlan
	ctx.RepoRoot = root
	ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:  "plan-applied",
		BatchID: "batch-1",
		Attempt: 1,
	})
	ctx.Mutable.AppendPlanStageProbeReport(&types.ChangeReport{
		Channel:            types.ChangeReportChannelPlannerProbe,
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{
			AssertionID: "tests/test_widget.py::test_value",
			Kind:        types.TestResultKindUnit,
			Passed:      true,
		}},
	})
	params := json.RawMessage(`{
		"request": "replan after verify failure",
		"summary": "Try to re-emit a value change that is already present in the worktree.",
		"changes": [
			{
				"path": "widget.py",
				"kind": "patch",
				"rationale": "the worktree already has this value",
				"edits": [{"kind": "replace", "start_line": 1, "end_line": 1, "old_text": "VALUE = 41\n", "content": "VALUE = 42\n"}]
			}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("expected stale structured edit to be rejected")
	}
	for _, want := range []string{"old_text mismatch", "run_tests(dry_run=true, verification_probe={...})", types.PlanStatusNoChangeRequired} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("replan stale-anchor rejection should mention %q, got: %s", want, res.Summary)
		}
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Fatalf("rejected stale structured edit must not install a plan: %+v", plan)
	}
}

// TestEmitChangePlan_InvalidKindRejected locks the per-change
// kind validation: kind must be one of create|modify|delete|patch.
func TestEmitChangePlan_InvalidKindRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "foo.go", "kind": "frobnicate", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for illegal kind")
	}
	if !strings.Contains(res.Summary, "frobnicate") {
		t.Errorf("summary should mention invalid kind, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_EmptyPathRejected locks the per-change path
// validation: path must be non-empty.
func TestEmitChangePlan_EmptyPathRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "", "kind": "modify", "new_content": "x", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty path")
	}
	if !strings.Contains(res.Summary, "empty path") {
		t.Errorf("summary should mention empty path, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_EmptySummaryRejected locks the required-field
// validation at the structure level (JSON schema + runtime).
func TestEmitChangePlan_EmptySummaryRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "",
		"changes": [
			{"path": "foo.go", "kind": "modify", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for empty summary")
	}
}

// TestEmitChangePlan_UnknownFieldsRejected locks the strict-decode
// guarantee: any schema drift (extra JSON fields the params struct
// does not declare) fails loudly rather than silently losing data.
func TestEmitChangePlan_UnknownFieldsRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "test",
		"summary": "test summary here with sufficient length",
		"changes": [
			{"path": "foo.go", "kind": "modify", "rationale": "test"}
		],
		"unknown_future_field": "this should break decode"
	}`)
	res, err := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for unknown field")
	}
	if err == nil {
		t.Error("expected non-nil error for unknown field")
	}
}

// TestEmitChangePlan_NilMutableGracefullyFails ensures the tool
// degrades cleanly when caller passes an incomplete BusContext —
// no panic, clean error message.
func TestEmitChangePlan_NilMutableGracefullyFails(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := &types.BusContext{} // no Mutable
	params := json.RawMessage(`{"request":"x","summary":"y","changes":[{"path":"a","kind":"modify","rationale":"b"}]}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for nil Mutable")
	}
	if !strings.Contains(res.Summary, "writable context") {
		t.Errorf("summary should name the missing writable context, got: %s", res.Summary)
	}
}

// TestEmitChangePlan_MetadataBasics pins tool surface invariants:
// Name, Description, Parameters schema are all non-empty and the
// schema parses as valid JSON.
func TestEmitChangePlan_MetadataBasics(t *testing.T) {
	tool := &EmitChangePlan{}
	if tool.Name() != "emit_change_plan" {
		t.Errorf("Name = %q, want emit_change_plan", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	schema := tool.Parameters()
	if len(schema) == 0 {
		t.Fatal("Parameters should not be empty")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Errorf("Parameters schema is not valid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema root type = %v, want 'object'", parsed["type"])
	}
}

// TestEmitChangePlan_IsWriteMarker verifies the tool has the
// expected marker tag. emit_change_plan is classified ReadOnly
// because it mutates BusContext, not the filesystem (disk writes
// happen in cmd/root.go's writePlanFile).
func TestEmitChangePlan_IsWriteMarker(t *testing.T) {
	tool := &EmitChangePlan{}
	if tool.IsWrite() {
		t.Error("emit_change_plan should be classified ReadOnly (mutates BusContext, not repo)")
	}
}

// TestEmitChangePlan_DuplicatePathRejected locks the B1-Q1
// decision: one change per file per plan. A planner that emits
// two entries for the same path is buggy; the tool rejects rather
// than silently losing one (or worse, applying both in undefined
// order).
func TestEmitChangePlan_DuplicatePathRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "touch foo.go twice",
		"summary": "illegal plan shape: two changes for one file.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "A", "rationale": "first"},
			{"path": "foo.go", "kind": "patch", "patch": "B", "rationale": "second"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for duplicate path")
	}
	if !strings.Contains(res.Summary, "duplicate") {
		t.Errorf("summary should mention 'duplicate', got: %s", res.Summary)
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		t.Error("ChangePlan must not be installed when validation fails")
	}
}

// TestEmitChangePlan_UnknownDependsOnRejected locks the B1-Q1
// rule: every depends_on entry must resolve to another change in
// the plan. A dangling reference is silently non-functional (the
// apply-stage W1 check would reject later, but earlier is better).
func TestEmitChangePlan_UnknownDependsOnRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "add a test",
		"summary": "foo.go depends on a path that isn't in this plan.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "X", "rationale": "uses bar",
			 "depends_on": ["bar.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for unknown depends_on")
	}
	if !strings.Contains(res.Summary, "bar.go") {
		t.Errorf("summary should name the missing target; got: %s", res.Summary)
	}
}

// TestEmitChangePlan_SelfDependRejected locks the degenerate
// self-edge case. A change depending on itself is always a bug
// (the planner model confused this-edit with this-file) and the
// cycle detector would catch it anyway, but this test documents
// the intent separately from the multi-node cycle test below.
func TestEmitChangePlan_SelfDependRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "self-loop",
		"summary": "illegal: a change depending on its own path.",
		"changes": [
			{"path": "foo.go", "kind": "modify", "new_content": "X",
			 "rationale": "self", "depends_on": ["foo.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for self-dependency")
	}
	if !strings.Contains(res.Summary, "itself") {
		t.Errorf("summary should mention 'itself'; got: %s", res.Summary)
	}
}

// TestEmitChangePlan_CycleRejected verifies multi-node depends_on
// cycles are rejected with the specific cycle path in the error
// message. The planner's retry hint can then identify which edges
// to break instead of guessing.
func TestEmitChangePlan_CycleRejected(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	// a -> b -> c -> a cycle
	params := json.RawMessage(`{
		"request": "3-node cycle",
		"summary": "illegal: depends_on forms a cycle a → b → c → a.",
		"changes": [
			{"path": "a.go", "kind": "modify", "new_content": "A",
			 "rationale": "a", "depends_on": ["c.go"]},
			{"path": "b.go", "kind": "modify", "new_content": "B",
			 "rationale": "b", "depends_on": ["a.go"]},
			{"path": "c.go", "kind": "modify", "new_content": "C",
			 "rationale": "c", "depends_on": ["b.go"]}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("expected Success=false for depends_on cycle")
	}
	if !strings.Contains(res.Summary, "cycle") {
		t.Errorf("summary should mention 'cycle'; got: %s", res.Summary)
	}
	// Cycle path should name all three files.
	for _, p := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(res.Summary, p) {
			t.Errorf("cycle description should mention %q; got: %s", p, res.Summary)
		}
	}
}

// TestEmitChangePlan_ValidDependsOnHappyPath locks that a legal
// DAG of depends_on edges passes validation and the DependsOn
// field survives to the installed ChangePlan (so the apply stage
// can read it for topological sorting).
func TestEmitChangePlan_ValidDependsOnHappyPath(t *testing.T) {
	tool := &EmitChangePlan{}
	ctx := newTestBusCtx()
	params := json.RawMessage(`{
		"request": "create helper + modify caller",
		"summary": "Create helper.go first, then modify main.go to import it. Declares the ordering via depends_on.",
		"changes": [
			{"path": "helper.go", "kind": "create",
			 "new_content": "package main\nfunc Helper() {}\n",
			 "rationale": "new helper function"},
			{"path": "main.go", "kind": "modify",
			 "new_content": "package main\nfunc main() { Helper() }\n",
			 "rationale": "call the helper",
			 "depends_on": ["helper.go"]}
		]
	}`)
	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true, got summary: %s", res.Summary)
	}

	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("expected ChangePlan installed")
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(plan.Changes))
	}
	// Find the main.go change and check DependsOn.
	var mainCh *types.FileChange
	for i := range plan.Changes {
		if plan.Changes[i].Path == "main.go" {
			mainCh = &plan.Changes[i]
			break
		}
	}
	if mainCh == nil {
		t.Fatal("main.go change missing from plan")
	}
	if len(mainCh.DependsOn) != 1 || mainCh.DependsOn[0] != "helper.go" {
		t.Errorf("main.go depends_on = %v, want [helper.go]", mainCh.DependsOn)
	}
	// helper.go should have no depends_on (the "leaf" of the graph).
	var helperCh *types.FileChange
	for i := range plan.Changes {
		if plan.Changes[i].Path == "helper.go" {
			helperCh = &plan.Changes[i]
			break
		}
	}
	if helperCh == nil {
		t.Fatal("helper.go change missing from plan")
	}
	if len(helperCh.DependsOn) != 0 {
		t.Errorf("helper.go depends_on should be empty, got %v", helperCh.DependsOn)
	}
}

// TestDetectDepsCycle_UnitTable exercises the cycle helper
// directly on a handful of graph shapes so the surface-level
// tests above stay focused on tool-Execute integration, not
// graph-theory edge cases.
func TestDetectDepsCycle_UnitTable(t *testing.T) {
	cases := []struct {
		name    string
		changes []emitChangePlanChange
		wantCyc bool // true → any non-empty cycle string is acceptable
	}{
		{
			name:    "empty graph",
			changes: []emitChangePlanChange{},
			wantCyc: false,
		},
		{
			name: "single node no deps",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
			},
			wantCyc: false,
		},
		{
			name: "two-node chain",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
				{Path: "b.go", DependsOn: []string{"a.go"}},
			},
			wantCyc: false,
		},
		{
			name: "two-node cycle",
			changes: []emitChangePlanChange{
				{Path: "a.go", DependsOn: []string{"b.go"}},
				{Path: "b.go", DependsOn: []string{"a.go"}},
			},
			wantCyc: true,
		},
		{
			name: "diamond (legal DAG)",
			changes: []emitChangePlanChange{
				{Path: "a.go"},
				{Path: "b.go", DependsOn: []string{"a.go"}},
				{Path: "c.go", DependsOn: []string{"a.go"}},
				{Path: "d.go", DependsOn: []string{"b.go", "c.go"}},
			},
			wantCyc: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectDepsCycle(emitChangesToFileChanges(c.changes))
			if (got != "") != c.wantCyc {
				t.Errorf("detectDepsCycle = %q, wantCyc=%v", got, c.wantCyc)
			}
		})
	}
}

// TestEmitChangePlan_PatchPreflight_RejectsCorruptDiff pins the
// session-35 fix: a kind=patch change whose unified diff fails
// `git apply --check` is rejected at emit time. The session-35 e2e
// run saw 2/3 planner dispatches produce diffs with mismatched @@
// hunk counts — those landed in plan.json, were persisted + approved,
// and blew up only at apply time inside the worktree, with no retry
// path (the coder's retries re-read the same corrupt diff from Mutable).
//
// With the pre-flight gate, the same malformed diff fails emit_change_plan
// → planner's ShouldStop doesn't fire (no plan installed) → planner
// retries within the same dispatch, seeing git's error verbatim. The
// failure is caught at the stage that can actually fix it.
func TestEmitChangePlan_PatchPreflight_RejectsCorruptDiff(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	// Seed a real git repo with known content so the pre-flight check
	// has a baseline to validate against.
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	// Clear the seeded plan — gitWorktreeFixture installs one, but we
	// want emit_change_plan.Execute to install its own from our params.
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Corrupt diff: @@ header claims 6 lines but body has 7 content
	// lines — the same malformed shape the session-35 planner produced.
	corruptDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,6 +1,6 @@
 line1
-line2
+lineTWO
 line3
 extra1
 extra2
 extra3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Pre-flight check should reject a malformed hunk header.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(corruptDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("corrupt diff should be rejected at emit time; got Success=true")
	}
	for _, want := range []string{"git apply", "kind=patch", "file.txt"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("rejection summary should carry %q; got %q", want, res.Summary)
		}
	}
	// No ChangePlan should have been installed — rejection is
	// pre-mutation so state stays clean.
	if ctx.Mutable.ChangePlan() != nil {
		t.Error("rejected emit must not install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_AcceptsValidDiff confirms the
// gate is one-sided: a well-formed diff with correct hunk counts
// passes emit cleanly.
func TestEmitChangePlan_PatchPreflight_AcceptsValidDiff(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	validDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-line2
+lineTWO
 line3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Well-formed diff, pre-flight should accept.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(validDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("valid diff should pass pre-flight; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

func TestEmitChangePlan_RejectsAdjacentDuplicateInsertedBlock(t *testing.T) {
	ctx := &types.BusContext{Mutable: types.NewMutableState("duplicate insert")}
	tool := &EmitChangePlan{}
	duplicateDiff := `--- a/a.py
+++ b/a.py
@@ -1,3 +1,11 @@
 x = data["x"]
 y = data["y"]
+mask = ~(x.isna() | y.isna())
+x = x[mask]
+y = y[mask]
+mask = ~(x.isna() | y.isna())
+x = x[mask]
+y = y[mask]
 if x.nunique() <= order:
     pass
`
	params := json.RawMessage(`{
		"request": "drop missing rows",
		"summary": "Filter missing rows before fitting. The duplicate-insertion gate should reject planner stutter.",
		"changes": [
			{"path": "a.py", "kind": "patch", "patch": ` + jsonString(duplicateDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatalf("adjacent duplicate insertion block should be rejected; got Success=true")
	}
	if !strings.Contains(res.Summary, "repeats the same added block twice") {
		t.Fatalf("rejection should name adjacent duplicate block, got %q", res.Summary)
	}
	if ctx.Mutable.ChangePlan() != nil {
		t.Fatal("rejected duplicate insertion must not install a ChangePlan")
	}
}

func TestValidatePlanPatchDuplicateInsertions_Carveouts(t *testing.T) {
	docPatch := `--- a/notes.md
+++ b/notes.md
@@ -0,0 +1,6 @@
+same
+same
+same
+same
+same
+same
`
	if rej := validatePlanPatchDuplicateInsertions([]types.FileChange{{Path: "notes.md", Kind: "patch", Patch: docPatch}}); rej != "" {
		t.Fatalf("non-source documentation patch should bypass duplicate block gate, got %q", rej)
	}
	shortSourcePatch := `--- a/a.py
+++ b/a.py
@@ -0,0 +1,4 @@
+call()
+call()
+call()
+call()
`
	if rej := validatePlanPatchDuplicateInsertions([]types.FileChange{{Path: "a.py", Kind: "patch", Patch: shortSourcePatch}}); rej != "" {
		t.Fatalf("short repeated single-line insertions should not trip the block-stutter gate, got %q", rej)
	}
}

func TestEmitChangePlan_PatchPreflight_TolerantToMissingContextMarkers(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "def greet(name):\n    if not name:\n        name = \"world\"\n    retrun f\"Hello, {name}!\"\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// The unchanged context lines below carry the file indentation
	// only. A strict unified diff needs one extra leading space marker
	// before each context line. This is a generic LLM serialization
	// error: the model authored the correct source lines but omitted
	// the diff marker, so the tool may normalize the patch envelope
	// without inventing content.
	missingMarkers := `--- a/file.txt
+++ b/file.txt
@@ -2,3 +2,3 @@
    if not name:
        name = "world"
-    retrun f"Hello, {name}!"
+    return f"Hello, {name}!"
`
	params := json.RawMessage(`{
		"request": "fix typo",
		"summary": "Fix a one-line typo. The context marker normalizer should accept the model-authored patch body.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(missingMarkers) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing-context-marker diff should pass pre-flight via structural normalization; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_TolerantToMiscountedHeader pins
// the --recount tolerance: a diff with a structurally-correct BODY
// but a WRONG @@ hunk-header line count must pass pre-flight. The
// session-35 eval repeatedly saw LLMs produce 5-line hunks declared
// as ",6" (or 7-line declared as ",6") — the body was byte-correct,
// the metadata wasn't. Failing those would burn planner retries on
// a mechanical counting error the system can trivially recover from.
// --recount is git's own blessed path for this scenario.
func TestEmitChangePlan_PatchPreflight_TolerantToMiscountedHeader(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Body is correct (3 old lines, 3 new lines — matches the file
	// exactly), but header declares ",6" for each side. Pre --recount
	// this would fail; post --recount it's accepted.
	miscountedDiff := `--- a/file.txt
+++ b/file.txt
@@ -1,6 +1,6 @@
 line1
-line2
+lineTWO
 line3
`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Header line count is wrong but body matches — --recount should fix.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(miscountedDiff) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("miscounted-header diff should pass pre-flight via --recount; got FAIL: %s", res.Summary)
	}
	if ctx.Mutable.ChangePlan() == nil {
		t.Error("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_TolerantToMissingTrailingNewline
// pins the newline-normalisation tolerance: a diff whose BODY is
// correct but whose FINAL LINE is missing its `\n` terminator must
// pass pre-flight. Session-35 eval saw 1/3 LLM dispatches drop the
// final newline, causing git to report "corrupt patch at line N"
// where N is the body length. This is a failure of mechanical
// serialisation, not semantics.
func TestEmitChangePlan_PatchPreflight_TolerantToMissingTrailingNewline(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "line1\nline2\nline3\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	// Body is correct; note the deliberate missing \n at end.
	noTrailingNewline := `--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-line2
+lineTWO
 line3`
	params := json.RawMessage(`{
		"request": "rename line2",
		"summary": "Fix line2 naming. Body complete but final line missing its terminating newline.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": ` + jsonString(noTrailingNewline) + `, "rationale": "test"}
		]
	}`)

	res, err := tool.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !res.Success {
		t.Fatalf("missing-trailing-newline diff should pass pre-flight; got FAIL: %s", res.Summary)
	}
	// Confirm the plan stored the ORIGINAL (not normalised) patch so
	// re-applying it via apply_patch lands bytes-correct too. The
	// tool layer normalises at the edge; Mutable state stays as-is.
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		t.Fatal("accepted emit must install a ChangePlan on Mutable")
	}
}

// TestEmitChangePlan_PatchPreflight_EmptyPatchRejected confirms the
// emit-time check catches a missing Patch field for kind=patch
// (previously only the apply_patch tool would reject this — too late).
func TestEmitChangePlan_PatchPreflight_EmptyPatchRejected(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available; pre-flight check skipped")
	}
	ctx := gitWorktreeFixture(t, "x\n")
	ctx.Mutable = types.NewMutableState("test")

	tool := &EmitChangePlan{}
	params := json.RawMessage(`{
		"request": "noop",
		"summary": "Empty patch must be rejected at emit time.",
		"changes": [
			{"path": "file.txt", "kind": "patch", "patch": "", "rationale": "test"}
		]
	}`)
	res, _ := tool.Execute(ctx, params)
	if res.Success {
		t.Fatal("kind=patch with empty Patch must be rejected")
	}
	if !strings.Contains(res.Summary, "empty") || !strings.Contains(res.Summary, "patch") {
		t.Errorf("rejection should name empty patch; got %q", res.Summary)
	}
}

func TestEmitChangePlan_OldTextMismatchExposesPlanRepairPack(t *testing.T) {
	root := t.TempDir()
	writeSurfaceFile(t, root, "widget.py", "VALUE = 1\n")
	ctx := &types.BusContext{
		Mutable:  types.NewMutableState("repair old_text mismatch"),
		RepoRoot: root,
	}
	params := json.RawMessage(`{
		"request": "fix widget value",
		"summary": "Change widget.py with a bounded structured edit.",
		"changes": [{
			"path": "widget.py",
			"kind": "patch",
			"rationale": "update the value",
			"edits": [{
				"kind": "replace",
				"start_line": 1,
				"old_text": "VALUE = 0\n",
				"content": "VALUE = 2\n"
			}]
		}]
	}`)
	res, err := (&EmitChangePlan{}).Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if res.Success {
		t.Fatal("old_text mismatch should reject the plan")
	}
	pack := mustPlanRepairPack(t, res)
	if pack.ReasonCode != "old_text_mismatch" {
		t.Fatalf("reason_code=%q, want old_text_mismatch; pack=%+v", pack.ReasonCode, pack)
	}
	if len(pack.CurrentBytes) != 1 || pack.CurrentBytes[0].ExpectedOldText != "VALUE = 1\n" {
		t.Fatalf("repair pack should carry exact expected old_text, got %+v", pack.CurrentBytes)
	}
	if !strings.Contains(res.Summary, planRepairSummaryTag) {
		t.Fatalf("summary should retain transparent repair pack line, got %q", res.Summary)
	}
}

func mustPlanRepairPack(t *testing.T, res types.ToolResult) types.PlanRepairPack {
	t.Helper()
	if res.Repair == nil {
		t.Fatalf("expected ToolRepair, got nil; summary=%s", res.Summary)
	}
	if res.Repair.Code != planRepairToolCode {
		t.Fatalf("repair code=%q, want %q; repair=%+v", res.Repair.Code, planRepairToolCode, res.Repair)
	}
	pack, ok := planRepairPackFromMetadataJSON(res.Repair.Metadata[planRepairMetadataKey])
	if !ok {
		t.Fatalf("missing or invalid plan_repair_pack metadata: %+v", res.Repair.Metadata)
	}
	return pack
}

// jsonString produces a valid JSON-encoded string literal (with
// escaping) for injection into a raw JSON template. The test data
// includes newlines + tabs that need JSON escaping; building the
// params map via json.Marshal and re-encoding would lose readability.
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// Unreachable for normal strings.
		panic(err)
	}
	return string(b)
}
