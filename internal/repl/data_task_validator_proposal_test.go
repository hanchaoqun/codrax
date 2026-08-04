package repl

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// E2PROP-1 pins (§29.150⑥ "开提案车道来"; §29.142 E-2 memo (c)/(d)):
// the system may PROPOSE a validator-parameterized assemble_answer plan as a
// fallback candidate when the planner degrades — it may never answer, never
// execute directly, and never launder a generated-artifact reference. Four
// families:
//
//	① deterministic witness re-attestation (17,0,5 shape): validator hint →
//	  typed params → credential gate → candidate → adoption through the
//	  EXISTING candidate lane → correct answer published;
//	② poisoned negative arm: generated-alias reference_path refuses the
//	  proposal with a named reason (no silent drop);
//	③ structural pin: the proposal lane has exactly one production entry
//	  (the repair-degradation recovery, AFTER the model's continuation
//	  attempt) and no execution/publication path;
//	④ degradation without complete validator params keeps the honest typed
//	  no_tool_call error byte-for-byte (現状 unchanged).

func validatorProposalWitnessRecords() ([]dataTaskWorkflowRecord, dataquery.TaskPlan) {
	contract := referenceGroundingContract()
	wrong := referenceGroundingResult("17,4,5", map[string]string{"GroupA": "17", "GroupB": "4", "GroupC": "5"})
	// Witness-faithful lineage (§29.142 replay#4): the wrong answer WAS
	// projected by an assemble_answer round over a clean reconcile ledger —
	// grounding is the only red lane, exactly the shape whose repair hint
	// carries the complete parameters.
	wrong.Reconcile = &dataquery.ReconcileReport{
		Status:         dataquery.LooseText("pass"),
		ExpectedAnswer: dataquery.LooseText("17,4,5"),
		ActualAnswer:   dataquery.LooseText("17,4,5"),
		Groups: []dataquery.ReconcileGroup{
			{GroupKey: dataquery.LooseText("GroupA"), Metric: dataquery.LooseText("total_value"), Expected: dataquery.LooseText("17"), Actual: dataquery.LooseText("17"), Difference: dataquery.LooseText("0")},
			{GroupKey: dataquery.LooseText("GroupB"), Metric: dataquery.LooseText("total_value"), Expected: dataquery.LooseText("4"), Actual: dataquery.LooseText("4"), Difference: dataquery.LooseText("0")},
			{GroupKey: dataquery.LooseText("GroupC"), Metric: dataquery.LooseText("total_value"), Expected: dataquery.LooseText("5"), Actual: dataquery.LooseText("5"), Difference: dataquery.LooseText("0")},
		},
	}
	wrong.Artifacts = []dataquery.DataArtifact{{
		ID:   "final_answer.json",
		Kind: string(dataquery.DataActionAssembleAnswer),
	}}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Status:           "ready",
			Goal:             "project per-target totals",
			CoverageContract: contract,
			OutputContract:   wrong.OutputContract,
			Actions: []dataquery.DataAction{{
				ID:   "project_targets",
				Kind: dataquery.DataActionAssembleAnswer,
				Params: map[string]string{
					"reference_path":      "targets.csv",
					"reference_key_field": "canonical_label",
				},
			}},
		},
		Result: &wrong,
	}}
	current := dataquery.TaskPlan{
		Status:           "complete",
		Goal:             "project per-target totals",
		CoverageContract: contract,
		OutputContract:   wrong.OutputContract,
	}
	return records, current
}

// TestValidatorProposalWitnessReplayAdoptsGroundedProjection is pin ① — the
// full §29.142 witness chain, deterministically: the stored answer "17,4,5"
// fails the grounding gate (message carries complete assemble_answer
// parameters), the repair planner dies with the typed E-2 double-empty
// error, the continuation planner (the model's own next chance) also
// produces nothing, the system synthesizes the validator-parameterized
// candidate, the EXISTING candidate lane prepares and executes it, the
// validator chain re-verifies, and the grounded truth "17,0,5" is published.
func TestValidatorProposalWitnessReplayAdoptsGroundedProjection(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	checkpoint := writeDataTaskWorkflowCheckpointFile(t.TempDir(), root, records, current, dataquery.TaskPlan{}, 1, 0, "witness replay checkpoint", "test")
	if checkpoint == "" {
		t.Fatal("checkpoint path empty")
	}
	planner := &stubDataTaskPlanner{
		// E-2 double-empty degradation shape: the repair planner's bounded
		// reprompt already happened inside planDataTask; the surviving
		// typed error is what the workflow sees.
		repairErr: newDataTaskPlannerNoToolError("data task planner", nil),
		// The continuation planner (model) is consulted FIRST and returns
		// nothing — only then may the system propose.
		continuePlan: dataquery.TaskPlan{},
		eval:         dataquery.Evaluation{Status: dataquery.EvalComplete, Reason: "grounded projection satisfies the contract", Confidence: "high"},
	}
	var progress bytes.Buffer
	runtimeAnchor := t.TempDir()
	answer, err := RunDataTaskCLI(context.Background(), "sum per target", TurnPolicy{Route: RouteData, NeedsDataAccess: true, Source: "data"}, DataTaskCLIConfig{
		Planner:         planner,
		RepoRoot:        root,
		RuntimeAnchor:   runtimeAnchor,
		Language:        "en",
		MaxRepairRounds: 2,
		MaxDataRounds:   6,
		Progress:        &progress,
		ResumePath:      checkpoint,
	})
	if err != nil {
		t.Fatalf("RunDataTaskCLI witness replay: %v", err)
	}
	if !strings.Contains(answer, "17,0,5") {
		t.Fatalf("answer=%q, want the grounded truth 17,0,5 published through the candidate lane", answer)
	}
	if strings.Contains(answer, "17,4,5") {
		t.Fatalf("answer=%q, must not republish the ungrounded witness value", answer)
	}
	if planner.repairCalls != 1 {
		t.Fatalf("repairCalls=%d, want exactly one degraded repair attempt before the proposal", planner.repairCalls)
	}
	// Model-first ordering: the continuation planner ran during resume AND
	// again inside the repair-degradation recovery BEFORE the proposal was
	// synthesized. A proposal that preempts the model would leave this at 1.
	if planner.continueCalls != 2 {
		t.Fatalf("continueCalls=%d, want 2 (resume + model-first continuation before the proposal)", planner.continueCalls)
	}
	// Typed provenance: the adopted candidate is journaled and audited under
	// the validator_proposal source, not laundered into a generic continue.
	proposalPlans, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-validator_proposal-r*.plan.json"))
	if err != nil || len(proposalPlans) == 0 {
		t.Fatalf("proposal plan audit files=%v err=%v, want the candidate audited under the validator_proposal scope", proposalPlans, err)
	}
	terminalFiles, err := filepath.Glob(filepath.Join(runtimeAnchor, "data-audit", "*-terminal.json"))
	if err != nil || len(terminalFiles) != 1 {
		t.Fatalf("terminal audit files=%v err=%v, want exactly one", terminalFiles, err)
	}
	rawTerminal, err := os.ReadFile(terminalFiles[0])
	if err != nil {
		t.Fatalf("read terminal audit: %v", err)
	}
	terminalText := string(rawTerminal)
	for _, want := range []string{`"validator_proposal"`, `"status": "complete"`, "17,0,5"} {
		if !strings.Contains(terminalText, want) {
			t.Fatalf("terminal audit missing %q:\n%s", want, terminalText)
		}
	}
}

// TestValidatorProposalSynthesizesValidatorParameterizedCandidate is the
// unit arm of pin ①: the candidate's parameters are carried on TYPED
// validator fields (violation.InputAlias/Field), tie verbatim to the repair
// driver, and re-emerge byte-identical on the synthesized assemble_answer
// action.
func TestValidatorProposalSynthesizesValidatorParameterizedCandidate(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	wrong := *records[0].Result

	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, wrong)
	if guard.Empty() || len(guard.Violations) == 0 {
		t.Fatal("witness shape must keep the grounding guard red")
	}
	violation := guard.Violations[0]
	if violation.InputAlias != "targets.csv" || violation.Field != "canonical_label" {
		t.Fatalf("violation typed params=%q/%q, want reference_path/reference_key_field carried on typed fields (not only prose)", violation.InputAlias, violation.Field)
	}

	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: current}
	plan, ok := dataTaskValidatorHintFallbackPlanCandidate(root, view, guard.ErrorText())
	if !ok {
		t.Fatal("live validator hint with complete typed params must synthesize a candidate")
	}
	if len(plan.Actions) != 1 || string(plan.Actions[0].Kind) != string(dataquery.DataActionAssembleAnswer) {
		t.Fatalf("candidate actions=%+v, want exactly one assemble_answer projection", plan.Actions)
	}
	params := plan.Actions[0].Params
	if params["complete_reference"] != "true" || params["reference_path"] != "targets.csv" || params["reference_key_field"] != "canonical_label" {
		t.Fatalf("candidate params=%v, want the validator's complete_reference/reference_path/reference_key_field verbatim", params)
	}
	if !plan.OutputContract.CompleteReference || plan.OutputContract.ReferencePath != "targets.csv" {
		t.Fatalf("candidate output contract=%+v, want the declared complete-reference projection", plan.OutputContract)
	}
}

// TestValidatorProposalRefusesGeneratedAliasReferencePath is pin ② — the
// laundering source of replay#2: a reference_path naming a workflow-generated
// artifact ("contributions.json") must be refused by the C1/C3 material
// credential BEFORE any proposal exists, with a named reason.
func TestValidatorProposalRefusesGeneratedAliasReferencePath(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	wrong := *records[0].Result
	poisoned := wrong
	poisoned.Artifacts = []dataquery.DataArtifact{{
		ID:   "contributions.json",
		Kind: string(dataquery.DataActionComputeContribs),
	}}
	poisonedRecords := append([]dataTaskWorkflowRecord(nil), records...)
	poisonedRecords[0] = dataTaskWorkflowRecord{Plan: records[0].Plan, Result: &poisoned}

	if _, refusal, ok := dataTaskSynthesizeValidatorProposalPlan(root, poisonedRecords, current, poisoned, "contributions.json", "canonical_label"); ok || !strings.Contains(refusal, "material credential") {
		t.Fatalf("ok=%v refusal=%q, want the generated-alias reference refused with the credential named", ok, refusal)
	}

	// Positive control on the same records: the real on-disk source material
	// passes the gate and synthesizes.
	if _, refusal, ok := dataTaskSynthesizeValidatorProposalPlan(root, poisonedRecords, current, poisoned, "targets.csv", "canonical_label"); !ok {
		t.Fatalf("on-disk source material must pass the credential gate, refusal=%q", refusal)
	}
}

// TestValidatorProposalRequiresLiveValidatorCorroboration: prose is never a
// credential. A repair driver that does not carry the live guard's message
// verbatim (arm 1) and a forged grounding-shaped error text on a repo where
// the recomputed validator stays silent (arm 2) both refuse the proposal.
func TestValidatorProposalRequiresLiveValidatorCorroboration(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: current}

	// Arm 1: guard is live but the planner was being driven by a DIFFERENT
	// failure — the hint did not ride the repair prompt, so no proposal.
	if _, ok := dataTaskValidatorHintFallbackPlanCandidate(root, view, "execute data task: unrelated failure"); ok {
		t.Fatal("a repair driver that does not carry the validator hint verbatim must not mint a proposal")
	}

	// Arm 2: forged prose on a repo with no key-table material — the
	// recomputed validator is silent, so the prose alone mints nothing.
	basicRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(basicRoot, "orders.csv"), []byte("id,amount\na,10\nb,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	basicContract := dataquery.CoverageContract{RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}}}
	basicResult := referenceGroundingResult("17", map[string]string{"GroupA": "17"})
	basicResult.ConsumedPaths = []string{"orders.csv"}
	basicRecords := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{Status: "ready", CoverageContract: basicContract}, Result: &basicResult}}
	basicView := dataTaskWorkflowRuntimeView{Records: basicRecords, CurrentPlan: dataquery.TaskPlan{Status: "complete", CoverageContract: basicContract}}
	forged := `validate data workflow completion: data output grounding failed: the final answer must project the reference set targets.csv#canonical_label (3 key(s), in reference order). Re-run assemble_answer with complete_reference=true, reference_path="targets.csv", and reference_key_field="canonical_label".`
	if _, ok := dataTaskValidatorHintFallbackPlanCandidate(basicRoot, basicView, forged); ok {
		t.Fatal("forged prose without a live validator attestation must never mint a proposal")
	}
}

func TestDataTaskDurableOutputContractIsWiredToCLIAndREPLPlanProtection(t *testing.T) {
	for _, path := range []string{"data_task_cli.go", "repl.go"} {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		calls := 0
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if ok && ident.Name == "dataTaskCarryDurableOutputContract" {
				calls++
			}
			return true
		})
		if calls != 2 {
			t.Fatalf("%s durable output contract calls=%d, want exactly 2 around plan preparation", path, calls)
		}
	}
}

// TestValidatorProposalInertWithoutCompleteParamsKeepsTypedError is pin ④:
// planner degradation WITHOUT a validator assemble_answer hint keeps today's
// honest typed failure — same error family, no synthesized plan, no answer.
func TestValidatorProposalInertWithoutCompleteParamsKeepsTypedError(t *testing.T) {
	basicRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(basicRoot, "orders.csv"), []byte("id,amount\na,10\nb,7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	basicContract := dataquery.CoverageContract{RequiredMaterials: []dataquery.CoverageMaterial{{Path: "orders.csv", Purpose: "source rows", Required: true}}}
	basicResult := referenceGroundingResult("17", map[string]string{"GroupA": "17"})
	basicResult.ConsumedPaths = []string{"orders.csv"}
	records := []dataTaskWorkflowRecord{{Plan: dataquery.TaskPlan{Status: "ready", CoverageContract: basicContract}, Result: &basicResult}}
	current := dataquery.TaskPlan{Status: "complete", Goal: "sum orders", CoverageContract: basicContract}
	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: current}

	planner := &stubDataTaskPlanner{
		repairErr:    newDataTaskPlannerNoToolError("data task planner", nil),
		continuePlan: dataquery.TaskPlan{},
	}
	result, _, ok, err := repairDataTaskPlanForCLI(context.Background(), planner, "sum orders", basicRoot, TurnPolicy{}, nil, current, "validate data workflow completion: some non-grounding failure", view, 0, 2, nil)
	if ok {
		t.Fatalf("result=%+v, degradation without validator params must not produce a plan", result)
	}
	if err == nil || !strings.Contains(err.Error(), "returned no tool_call") {
		t.Fatalf("err=%v, want the honest typed no_tool_call error preserved verbatim", err)
	}
	if planner.continueCalls != 1 {
		t.Fatalf("continueCalls=%d, want the model's continuation chance consumed before failing honest", planner.continueCalls)
	}
}

// --- pin ③: structural no-direct-execution census -------------------------

func validatorProposalPackageSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		out[name] = parsed
	}
	return out
}

func validatorProposalCallSites(files map[string]*ast.File, callee string) map[string]bool {
	callers := map[string]bool{}
	for name, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == callee {
					callers[name+":"+fn.Name.Name] = true
				}
				return true
			})
		}
	}
	return callers
}

// validatorProposalFuncDecls indexes every top-level (non-method) function of
// the package by name so the closure walk can resolve Ident-called functions.
func validatorProposalFuncDecls(files map[string]*ast.File) map[string]*ast.FuncDecl {
	out := map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil || fn.Body == nil {
				continue
			}
			out[fn.Name.Name] = fn
		}
	}
	return out
}

// validatorProposalClosureViolations walks the TRANSITIVE package-local call
// closure of the proposal lane and reports every reachable execution or
// publication call — a helper-mediated detour is as red as a direct one.
func validatorProposalClosureViolations(index map[string]*ast.FuncDecl, roots []string, forbiddenIdents, forbiddenSelectors map[string]bool) (map[string]bool, []string) {
	visited := map[string]bool{}
	var violations []string
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if visited[name] {
			continue
		}
		visited[name] = true
		fn, ok := index[name]
		if !ok {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if forbiddenIdents[fun.Name] {
					violations = append(violations, name+" -> "+fun.Name)
					return true
				}
				if _, defined := index[fun.Name]; defined && !visited[fun.Name] {
					queue = append(queue, fun.Name)
				}
			case *ast.SelectorExpr:
				if forbiddenSelectors[fun.Sel.Name] {
					violations = append(violations, name+" -> ."+fun.Sel.Name+"(...)")
				}
			}
			return true
		})
	}
	return visited, violations
}

// TestValidatorProposalNoDirectExecution_SourcePin structurally pins "zero
// bypass to execution": the proposal composition point has exactly one
// production caller — the repair-degradation recovery, where the model's
// continuation attempt runs FIRST — the synthesis function is reachable only
// through the composition point, and the lane's TRANSITIVE package-local call
// closure cannot reach an execution (ActionRunner.Run / Runner.Run), a
// publication (finalDataTaskAnswerForCLI / dataTaskAnswerMarkdown), or a
// record-append. Deleting the lane, re-routing it around the candidate
// machinery, reordering it ahead of the model, or smuggling a call through an
// intermediate helper all turn this red.
func TestValidatorProposalNoDirectExecution_SourcePin(t *testing.T) {
	files := validatorProposalPackageSources(t)

	candidateCallers := validatorProposalCallSites(files, "dataTaskValidatorHintFallbackPlanCandidate")
	if len(candidateCallers) != 1 || !candidateCallers["data_task_workflow.go:dataTaskRepairFailureContinuationWithRuntimeView"] {
		t.Fatalf("dataTaskValidatorHintFallbackPlanCandidate production callers=%v, want exactly the repair-degradation recovery (the only entry into the existing candidate lane)", candidateCallers)
	}
	synthCallers := validatorProposalCallSites(files, "dataTaskSynthesizeValidatorProposalPlan")
	if len(synthCallers) != 1 || !synthCallers["data_task_proposal.go:dataTaskValidatorHintFallbackPlanCandidate"] {
		t.Fatalf("dataTaskSynthesizeValidatorProposalPlan production callers=%v, want only the credential-gated composition point", synthCallers)
	}

	recoveryBody := pinReplFunctionSource(t, "data_task_workflow.go", "dataTaskRepairFailureContinuationWithRuntimeView")
	modelIdx := strings.Index(recoveryBody, "dataTaskRunContinuationPlannerWithRuntimeView(")
	proposalIdx := strings.Index(recoveryBody, "dataTaskValidatorHintFallbackPlanCandidate(")
	if modelIdx < 0 || proposalIdx < 0 || modelIdx > proposalIdx {
		t.Fatalf("recovery ordering modelIdx=%d proposalIdx=%d: the continuation planner (model) must be consulted before the system proposal", modelIdx, proposalIdx)
	}

	index := validatorProposalFuncDecls(files)
	roots := []string{"dataTaskValidatorHintFallbackPlanCandidate", "dataTaskSynthesizeValidatorProposalPlan", "dataTaskValidatorAssembleAnswerRepairHint"}
	visited, violations := validatorProposalClosureViolations(index, roots,
		map[string]bool{"finalDataTaskAnswerForCLI": true, "dataTaskAnswerMarkdown": true},
		map[string]bool{"Run": true, "AppendRecord": true},
	)
	if len(violations) != 0 {
		t.Fatalf("proposal lane closure reaches execution/publication calls: %v — the lane must only return a candidate", violations)
	}
	// Sanity: the closure walk actually traversed the load-bearing helpers
	// (an accidentally-empty walk would vacuously pass).
	for _, want := range []string{"dataTaskCompletionAnswerSelection", "dataTaskReferencePathIsWorkflowMaterial", "dataTaskExplicitReferenceProjectionCandidate"} {
		if !visited[want] {
			t.Fatalf("closure census did not visit %s — walker broken or lane rewired, update the pin together with the code", want)
		}
	}

	synthBody := pinReplFunctionSource(t, "data_task_proposal.go", "dataTaskSynthesizeValidatorProposalPlan")
	gateIdx := strings.Index(synthBody, "dataTaskReferencePathIsWorkflowMaterial(")
	buildIdx := strings.Index(synthBody, "BuildRequiredOutputProjectionPlan(")
	if gateIdx < 0 || buildIdx < 0 || gateIdx > buildIdx {
		t.Fatalf("synthesis gateIdx=%d buildIdx=%d: the C1/C3 material credential must run BEFORE any plan is built", gateIdx, buildIdx)
	}
}

// TestCompletionAnswerSelectionSingleAuthority_SourcePin (P3-b): the DL-C
// terminal-answer selection consult exists exactly once —
// dataTaskCompletionAnswerSelection — and both completion-side consumers (the
// evaluation decision and the validator-proposal extraction) route through
// it instead of hand-copying the selection condition.
func TestCompletionAnswerSelectionSingleAuthority_SourcePin(t *testing.T) {
	evalBody := pinReplFunctionSource(t, "data_task_workflow.go", "dataTaskEvaluationDecisionWithRepo")
	hintBody := pinReplFunctionSource(t, "data_task_proposal.go", "dataTaskValidatorAssembleAnswerRepairHint")
	for name, body := range map[string]string{"dataTaskEvaluationDecisionWithRepo": evalBody, "dataTaskValidatorAssembleAnswerRepairHint": hintBody} {
		if !strings.Contains(body, "dataTaskCompletionAnswerSelection(") {
			t.Errorf("%s must consult the shared dataTaskCompletionAnswerSelection helper (DL-C single authority)", name)
		}
		if strings.Contains(body, "selectDataTaskTerminalAnswerWithRepo(") {
			t.Errorf("%s re-derives the terminal-answer selection inline instead of the shared helper", name)
		}
	}
	helperBody := pinReplFunctionSource(t, "data_task_workflow.go", "dataTaskCompletionAnswerSelection")
	if !strings.Contains(helperBody, "selectDataTaskTerminalAnswerWithRepo(") || !strings.Contains(helperBody, "sel.FromFallback && !sel.Contested") {
		t.Fatalf("dataTaskCompletionAnswerSelection must own the selection condition; body:\n%s", helperBody)
	}
}

// TestValidatorProposalCredentialRefusesAbsoluteMaterializedPath (P2,
// DATAGATE-2 "abs 一律不授信" extension): a MATERIALIZED generated artifact
// registered under its absolute blob path exists on disk outside the repo
// fence — disk existence must not defeat the alias conviction, and the
// proposal lane must refuse it. Relative on-disk true sources keep their
// credential unchanged.
func TestValidatorProposalCredentialRefusesAbsoluteMaterializedPath(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	wrong := *records[0].Result

	blobDir := t.TempDir()
	blobPath := filepath.Join(blobDir, "reference_blob.csv")
	if err := os.WriteFile(blobPath, []byte("target_id,canonical_label\nT1,GroupA\nT2,GroupX\nT3,GroupC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	materialized := wrong
	materialized.Artifacts = append(append([]dataquery.DataArtifact(nil), wrong.Artifacts...), dataquery.DataArtifact{
		ID:     "reference_blob",
		Kind:   string(dataquery.DataActionExtractRecords),
		Fields: map[string]string{"artifact_path": blobPath},
	})
	materializedRecords := append([]dataTaskWorkflowRecord(nil), records...)
	materializedRecords[0] = dataTaskWorkflowRecord{Plan: records[0].Plan, Result: &materialized}

	if dataTaskReferencePathIsWorkflowMaterial(root, materializedRecords, current, materialized, blobPath) {
		t.Fatal("absolute materialized artifact path (on-disk, outside repo) must not gain source-material standing")
	}
	if _, refusal, ok := dataTaskSynthesizeValidatorProposalPlan(root, materializedRecords, current, materialized, blobPath, "canonical_label"); ok || !strings.Contains(refusal, "material credential") {
		t.Fatalf("ok=%v refusal=%q, want the absolute materialized reference refused with the credential named", ok, refusal)
	}
	// Relative on-disk true source: credential unchanged.
	if !dataTaskReferencePathIsWorkflowMaterial(root, materializedRecords, current, materialized, "targets.csv") {
		t.Fatal("relative on-disk source material must keep its credential")
	}
}

// TestValidatorProposalNotOfferedWhenModelNeverConsulted (P3-a): a pre-flight
// continuation failure (typed no_plan_shape minted BEFORE the continuation
// planner ran) means the model was never consulted — the system must not
// propose in its place, even with a live validator hint.
func TestValidatorProposalNotOfferedWhenModelNeverConsulted(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	wrong := *records[0].Result
	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, wrong)
	if guard.Empty() {
		t.Fatal("witness shape must keep the grounding guard red")
	}
	planner := &stubDataTaskPlanner{continuePlan: dataquery.TaskPlan{}}
	// CurrentPlan without runtime shape => the continuation lane pre-flights
	// with the typed no_plan_shape error before consulting the model.
	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: dataquery.TaskPlan{}}
	result, handled, ok, err := dataTaskRepairFailureContinuationWithRuntimeView(context.Background(), planner, "sum per target", root, TurnPolicy{}, nil, view, newDataTaskPlannerNoToolError("data task planner", nil), guard.ErrorText())
	if ok {
		t.Fatalf("result=%+v, proposal must not fire when the model was never consulted", result)
	}
	if !handled || err == nil || !dataTaskPlannerErrorHasCode(err, dataTaskPlannerErrorNoPlanShape) {
		t.Fatalf("handled=%v err=%v, want the pre-flight typed no_plan_shape failure preserved", handled, err)
	}
	if planner.continueCalls != 0 {
		t.Fatalf("continueCalls=%d, want 0 (pre-flight failure means the model never ran)", planner.continueCalls)
	}
}

// TestValidatorProposalSourceSurvivesREPLContinuationFallback (P3-d): the
// typed validator_proposal provenance survives the REPL return tuple — the
// REPL lane shares the recovery choke point and must not launder the source
// back into a generic continue.
func TestValidatorProposalSourceSurvivesREPLContinuationFallback(t *testing.T) {
	root := referenceGroundingFixtureRepo(t)
	records, current := validatorProposalWitnessRecords()
	wrong := *records[0].Result
	guard := dataTaskOutputReferenceGroundingGuardResult(root, records, current, wrong)
	if guard.Empty() {
		t.Fatal("witness shape must keep the grounding guard red")
	}
	planner := &stubDataTaskPlanner{continuePlan: dataquery.TaskPlan{}}
	r := &REPL{repoRoot: root, language: "en", dataTaskPlanner: planner}
	view := dataTaskWorkflowRuntimeView{Records: records, CurrentPlan: current}
	plan, reason, source, ok := r.dataTaskRepairFailureContinuationFallback("sum per target", TurnPolicy{}, nil, view, newDataTaskPlannerNoToolError("data task planner", nil), guard.ErrorText())
	if !ok {
		t.Fatal("REPL continuation fallback must adopt the validator proposal in the witness shape")
	}
	if source != dataTaskValidatorProposalSource {
		t.Fatalf("source=%q, want %q carried through the REPL tuple", source, dataTaskValidatorProposalSource)
	}
	if strings.TrimSpace(reason) == "" {
		t.Fatal("fallback reason must disclose the proposal lineage")
	}
	if len(plan.Actions) != 1 || string(plan.Actions[0].Kind) != string(dataquery.DataActionAssembleAnswer) {
		t.Fatalf("plan actions=%+v, want the validator-parameterized assemble_answer candidate", plan.Actions)
	}
	if planner.continueCalls != 1 {
		t.Fatalf("continueCalls=%d, want the model consulted exactly once before the proposal", planner.continueCalls)
	}
}

func pinReplFunctionSource(t *testing.T, file, function string) string {
	t.Helper()
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatalf("read %s: %v", file, readErr)
	}
	src := string(raw)
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != function {
			continue
		}
		start := fset.Position(fn.Pos()).Offset
		end := fset.Position(fn.End()).Offset
		return src[start:end]
	}
	t.Fatalf("function %s not found in %s — the source pin census is stale, update it together with the code", function, file)
	return ""
}
