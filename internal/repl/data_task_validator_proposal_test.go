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
// candidate, the EXISTING candidate lane admits and executes it, the
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

// TestValidatorProposalNoDirectExecution_SourcePin structurally pins "zero
// bypass to execution": the proposal composition point has exactly one
// production caller — the repair-degradation recovery, where the model's
// continuation attempt runs FIRST — the synthesis function is reachable only
// through the composition point, and neither function contains an execution
// or publication call. Deleting the lane, re-routing it around the candidate
// machinery, or reordering it ahead of the model all turn this red.
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

	for _, fn := range []string{"dataTaskValidatorHintFallbackPlanCandidate", "dataTaskSynthesizeValidatorProposalPlan", "dataTaskValidatorAssembleAnswerRepairHint"} {
		body := pinReplFunctionSource(t, "data_task_proposal.go", fn)
		for _, forbidden := range []string{".Run(", "finalDataTaskAnswerForCLI", "dataTaskAnswerMarkdown", "AppendRecord"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains %q — the proposal lane must only return a candidate, never execute or publish", fn, forbidden)
			}
		}
	}
	synthBody := pinReplFunctionSource(t, "data_task_proposal.go", "dataTaskSynthesizeValidatorProposalPlan")
	gateIdx := strings.Index(synthBody, "dataTaskReferencePathIsWorkflowMaterial(")
	buildIdx := strings.Index(synthBody, "BuildRequiredOutputProjectionPlan(")
	if gateIdx < 0 || buildIdx < 0 || gateIdx > buildIdx {
		t.Fatalf("synthesis gateIdx=%d buildIdx=%d: the C1/C3 material credential must run BEFORE any plan is built", gateIdx, buildIdx)
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
