package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// executed_command_outcome_consumer_census_test.go — fold-in round four,
// finding K (colleague_merge_audit §40.36): the ExecutedCommand.Outcome set
// is closed AND total on the consumer side. Every `switch` over the field —
// in this package and in the tool / orchestrator / writeflow consumers —
// enumerates every declared ExecutedCommandOutcome* member explicitly
// through the typed constants; a `default` arm may exist only for labels
// outside the set, which the enumeration makes structurally unreachable for
// members. The census is bound to the real declaration (the constant block
// of test_surface.go, cross-checked against AllExecutedCommandOutcomes) and
// to the real switch statements (go/ast over the non-test files), so:
//   - a new member that a consumer switch does not name is red;
//   - a member spelled as a string literal in any consumer switch is red;
//   - a switch over Outcome that is not registered below is red (a new
//     consumer must join the census), and a stale registration is red.
//
// Every switch that LOOKS like an Outcome switch (its tag reads a field
// named Outcome or a local named outcome, or a case names a member constant)
// must be registered with its domain, so the reader sees which field each
// one reads:
//   - command: ExecutedCommand.Outcome — total enumeration required;
//   - diagnostic: VerificationDiagnostic.Outcome, which carries the command
//     label for command-derived diagnostics and probe status words
//     otherwise — member literals banned, totality not required;
//   - other: an unrelated field or local that happens to share the name
//     (the probe status envelope's Outcome, the controller apply-transition
//     outcome) — registration only, so the census never mistakes it for a
//     command consumer and a new command consumer cannot hide among them.

type outcomeSwitchDomain int

const (
	outcomeDomainCommand outcomeSwitchDomain = iota
	outcomeDomainDiagnostic
	outcomeDomainOther
)

type outcomeSwitchRegistration struct {
	dir, file, fn string
	domain        outcomeSwitchDomain
}

var registeredOutcomeSwitches = []outcomeSwitchRegistration{
	{dir: ".", file: "change_plan.go", fn: "executedCommandUnavailableReasonCode"},
	{dir: ".", file: "change_plan.go", fn: "executedCommandFailed"},
	{dir: ".", file: "verification_proof_profile.go", fn: "verificationProofCommandUnavailableReasonCode"},
	{dir: ".", file: "verification_proof_profile.go", fn: "verificationProofCommandClass"},
	{dir: "../tool", file: "run_tests.go", fn: "failureKindFromExecutedCommand"},
	{dir: "../tool", file: "run_tests.go", fn: "verificationDiagnosticClass"},
	{dir: "../tool", file: "run_tests.go", fn: "verificationProbeBaselineCommandCounts"},
	{dir: "../tool", file: "run_tests.go", fn: "verificationConfidenceFromCommand"},
	{dir: "../tool", file: "run_tests.go", fn: "makeResourceExhaustionReport"},
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "runPlanVerificationProbes"},
	{dir: "../orchestrator", file: "write_controller_scheduler.go", fn: "verifyCoverageCommandCoversPath"},
	{dir: "../orchestrator", file: "write_verify_render.go", fn: "reportUntriedRunnableCandidate"},
	{dir: "../writeflow", file: "no_change_replan.go", fn: "verifyFailureDiagnosticUnavailableReasonCode", domain: outcomeDomainDiagnostic},
	{dir: "../writeflow", file: "no_change_replan.go", fn: "verifyFailureDiagnosticLooksFailed", domain: outcomeDomainDiagnostic},
	// Probe status envelope words (passed / assertion_failed / import_error …).
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "runPythonVerificationProbe", domain: outcomeDomainOther},
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "runExternalVerificationProbe", domain: outcomeDomainOther},
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "inlineVerificationProbeReasonCode", domain: outcomeDomainOther},
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "pythonVerificationProbeReasonCode", domain: outcomeDomainOther},
	{dir: "../tool", file: "run_tests_verification_probe.go", fn: "pythonVerificationProbeImportDiagnostics", domain: outcomeDomainOther},
	// Controller apply-transition outcome (writeControllerApplyTransition*).
	{dir: "../orchestrator", file: "write_controller_scheduler.go", fn: "runWriteControllerWorkflow", domain: outcomeDomainOther},
}

var outcomeConsumerDirs = []string{".", "../tool", "../orchestrator", "../writeflow"}

const outcomeConsumerConstPrefix = "ExecutedCommandOutcome"

// declaredOutcomeMembers reads the closed set from the constant block of
// test_surface.go (name → value).
func declaredOutcomeMembers(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test_surface.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, outcomeConsumerConstPrefix) || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s must be a string literal", name.Name)
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[name.Name] = value
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no ExecutedCommandOutcome* constants declared in test_surface.go")
	}
	return out
}

// outcomeMemberName resolves a case expression to a member constant name.
func outcomeMemberName(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := v.X.(*ast.Ident); ok && pkg.Name == "types" && strings.HasPrefix(v.Sel.Name, outcomeConsumerConstPrefix) {
			return v.Sel.Name, true
		}
	case *ast.Ident:
		if strings.HasPrefix(v.Name, outcomeConsumerConstPrefix) {
			return v.Name, true
		}
	case *ast.ParenExpr:
		return outcomeMemberName(v.X)
	}
	return "", false
}

// outcomeSwitchTagMentionsOutcome reports whether a switch tag reads the
// Outcome field (directly, through strings.TrimSpace, or through a local
// named outcome / kind).
func outcomeSwitchTagMentionsOutcome(tag ast.Expr) bool {
	switch v := tag.(type) {
	case nil:
		return false
	case *ast.SelectorExpr:
		return v.Sel.Name == "Outcome"
	case *ast.Ident:
		return v.Name == "outcome"
	case *ast.CallExpr:
		for _, arg := range v.Args {
			if outcomeSwitchTagMentionsOutcome(arg) {
				return true
			}
		}
	case *ast.ParenExpr:
		return outcomeSwitchTagMentionsOutcome(v.X)
	}
	return false
}

type outcomeSwitchFinding struct {
	dir, file, fn string
	pos           string
	members       map[string]bool
	violations    []string
}

// outcomeSwitchesIn scans parsed files for switches over Outcome and returns
// one finding per switch (attributed to the enclosing declaration).
func outcomeSwitchesIn(fset *token.FileSet, dir string, files map[string]*ast.File, members map[string]string) []outcomeSwitchFinding {
	byValue := map[string]string{}
	for name, value := range members {
		byValue[value] = name
	}
	var out []outcomeSwitchFinding
	for fileName, file := range files {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sw, ok := n.(*ast.SwitchStmt)
				if !ok {
					return true
				}
				finding := outcomeSwitchFinding{dir: dir, file: fileName, fn: fd.Name.Name, pos: fset.Position(sw.Pos()).String(), members: map[string]bool{}}
				isOutcome := outcomeSwitchTagMentionsOutcome(sw.Tag)
				var literals []string
				for _, stmt := range sw.Body.List {
					clause, ok := stmt.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, expr := range clause.List {
						if name, ok := outcomeMemberName(expr); ok {
							isOutcome = true
							finding.members[name] = true
							continue
						}
						if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							value, err := strconv.Unquote(lit.Value)
							if err != nil {
								continue
							}
							if name, member := byValue[value]; member {
								literals = append(literals, "member "+name+" spelled as the literal "+lit.Value+" at "+fset.Position(lit.Pos()).String())
							}
						}
					}
				}
				if !isOutcome {
					return true
				}
				finding.violations = append(finding.violations, literals...)
				out = append(out, finding)
				return true
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pos < out[j].pos })
	return out
}

func parseOutcomeConsumerDir(t *testing.T, dir string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = file
	}
	if len(files) == 0 {
		t.Fatalf("no non-test files in %s", dir)
	}
	return fset, files
}

// outcomeCensusCheck applies the registry rules to a scan; it is shared by
// the live census and the self-red probes.
func outcomeCensusCheck(findings []outcomeSwitchFinding, registry []outcomeSwitchRegistration, members map[string]string) []string {
	var problems []string
	matched := map[int]bool{}
	for _, f := range findings {
		key := f.dir + "/" + f.file + ":" + f.fn
		idx := -1
		for i, reg := range registry {
			if reg.dir == f.dir && reg.file == f.file && reg.fn == f.fn {
				idx = i
			}
		}
		if idx < 0 {
			problems = append(problems, f.pos+": switch over ExecutedCommand.Outcome in "+key+" is not registered in the consumer census")
			continue
		}
		matched[idx] = true
		if registry[idx].domain == outcomeDomainOther {
			continue
		}
		for _, v := range f.violations {
			problems = append(problems, f.pos+": "+v)
		}
		if registry[idx].domain == outcomeDomainDiagnostic {
			continue
		}
		var missing []string
		for name := range members {
			if !f.members[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			problems = append(problems, f.pos+": switch in "+key+" does not enumerate the members "+strings.Join(missing, ", ")+" (a default arm must be unreachable for members)")
		}
	}
	for i, reg := range registry {
		if !matched[i] {
			problems = append(problems, "stale registration: no switch over ExecutedCommand.Outcome found in "+reg.dir+"/"+reg.file+":"+reg.fn)
		}
	}
	return problems
}

func TestEveryExecutedCommandOutcomeConsumerSwitchEnumeratesTheClosedSet(t *testing.T) {
	members := declaredOutcomeMembers(t)
	// The runtime member list is exactly the declaration.
	all := AllExecutedCommandOutcomes()
	if len(all) != len(members) {
		t.Fatalf("AllExecutedCommandOutcomes has %d members, %d constants declared", len(all), len(members))
	}
	seenValue := map[string]bool{}
	for _, value := range all {
		if seenValue[value] {
			t.Fatalf("AllExecutedCommandOutcomes repeats %q", value)
		}
		seenValue[value] = true
	}
	for name, value := range members {
		if !seenValue[value] {
			t.Fatalf("AllExecutedCommandOutcomes lacks %s (%q)", name, value)
		}
	}
	var findings []outcomeSwitchFinding
	for _, dir := range outcomeConsumerDirs {
		fset, files := parseOutcomeConsumerDir(t, dir)
		findings = append(findings, outcomeSwitchesIn(fset, dir, files, members)...)
	}
	if len(findings) < len(registeredOutcomeSwitches) {
		t.Fatalf("found %d Outcome switches, fewer than the %d registered", len(findings), len(registeredOutcomeSwitches))
	}
	for _, problem := range outcomeCensusCheck(findings, registeredOutcomeSwitches, members) {
		t.Error(problem)
	}
}

// The decision table of the typed classifiers over every member (the
// default arms are structurally unreachable for members — pinned above —
// and the per-member decisions are pinned here, including the three
// main-snapshot baseline evidence rows that used to fall into the
// conservative default of executedCommandFailed).
func TestExecutedCommandOutcomeClassifiersDecideEveryMember(t *testing.T) {
	type decision struct {
		failedExit0, failedExit1 bool
		unavailable              string
		proofUnavailable         string
	}
	byExit := func(unavailable string) decision {
		return decision{failedExit0: false, failedExit1: true, unavailable: unavailable, proofUnavailable: unavailable}
	}
	never := func() decision { return decision{} }
	always := func(unavailable string) decision {
		return decision{failedExit0: true, failedExit1: true, unavailable: unavailable, proofUnavailable: unavailable}
	}
	want := map[string]decision{
		"":                                    byExit(""),
		ExecutedCommandOutcomeExecuted:        byExit(""),
		ExecutedCommandOutcomeSyntaxPreflight: byExit(""),
		ExecutedCommandOutcomeSyntaxCheckFallback: byExit(""),
		ExecutedCommandOutcomeSuiteContinued:      byExit(""),
		ExecutedCommandOutcomeSuiteSkipped:        byExit(""),
		ExecutedCommandOutcomeSyntheticNoTests:    never(),
		ExecutedCommandOutcomeZeroTests:           never(),
		// Baseline evidence rows: expected_failure_observed is the probe's
		// desired baseline result; not_observed and unavailable weaken proof
		// through the probe_baseline confidence lane — none is a failed
		// command and none is a verification-unavailable reason.
		ExecutedCommandOutcomeExpectedFailureObserved:    never(),
		ExecutedCommandOutcomeExpectedFailureNotObserved: never(),
		ExecutedCommandOutcomeBaselineUnavailable:        never(),
		ExecutedCommandOutcomeRunnerMissing:              always(string(FailureKindRunnerMissing)),
		ExecutedCommandOutcomeNotConfigured:              always(string(FailureKindRunnerMissing)),
		ExecutedCommandOutcomeTimeout:                    always(""),
		ExecutedCommandOutcomeOOM:                        always(""),
		ExecutedCommandOutcomeCPULimit:                   always(""),
		ExecutedCommandOutcomeParserError:                {failedExit0: true, failedExit1: true, unavailable: "", proofUnavailable: string(FailureKindParserError)},
		ExecutedCommandOutcomeProbeConfigError:           always(""),
		ExecutedCommandOutcomeExpectedStdoutMissing:      always(""),
		"future_label_outside_the_set":                   always(""),
	}
	want[ExecutedCommandOutcomeSyntheticNoTests] = decision{unavailable: string(FailureKindNoTests), proofUnavailable: string(FailureKindNoTests)}
	want[ExecutedCommandOutcomeZeroTests] = want[ExecutedCommandOutcomeSyntheticNoTests]
	for _, member := range AllExecutedCommandOutcomes() {
		if _, ok := want[member]; !ok {
			t.Fatalf("decision table lacks member %q", member)
		}
	}
	for outcome, d := range want {
		exit0 := ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: outcome, ExitCode: 0}
		exit1 := ExecutedCommand{Runner: "rust", WorkingDir: ".", Outcome: outcome, ExitCode: 101}
		if got := executedCommandFailed(exit0); got != d.failedExit0 {
			t.Errorf("%q exit 0: failed=%v want %v", outcome, got, d.failedExit0)
		}
		if got := executedCommandFailed(exit1); got != d.failedExit1 {
			t.Errorf("%q exit 101: failed=%v want %v", outcome, got, d.failedExit1)
		}
		if got := executedCommandUnavailableReasonCode(exit1); got != d.unavailable {
			t.Errorf("%q: unavailable reason=%q want %q", outcome, got, d.unavailable)
		}
		if got := verificationProofCommandUnavailableReasonCode(exit1, VerificationProofRunnerProject); got != d.proofUnavailable {
			t.Errorf("%q: proof unavailable reason=%q want %q", outcome, got, d.proofUnavailable)
		}
	}
	// The proof ledger therefore records a baseline evidence row as covered,
	// never as a failed capability.
	baseline := ExecutedCommand{Runner: "verification_probe", Source: "verification_probe_main_snapshot_baseline",
		Command: "verification_probe_baseline:p", Outcome: ExecutedCommandOutcomeExpectedFailureObserved, ExitCode: 1,
		ReasonCode: "verification_probe_baseline_expected_failure_observed"}
	report := &ChangeReport{PlanID: "plan-k", Passed: true, VerificationStatus: VerificationStatusPassed, ExecutedCommands: []ExecutedCommand{baseline}}
	ledger := &VerificationProofLedger{}
	ledger.addVerificationReportLedgerItems(report)
	for _, item := range ledger.Capabilities {
		if item.Kind == "executed_command" && item.Status != VerificationProofLedgerItemCovered {
			t.Fatalf("baseline evidence row must not be a failed/unavailable capability: %+v", item)
		}
	}
}

// Self-red: the checker flags a total switch missing a member, a member
// spelled as a literal, an unregistered switch, and a stale registration.
func TestExecutedCommandOutcomeConsumerCensusSelfRed(t *testing.T) {
	members := map[string]string{"ExecutedCommandOutcomeExecuted": "executed", "ExecutedCommandOutcomeTimeout": "timeout"}
	parse := func(src string) (*token.FileSet, map[string]*ast.File) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "probe.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return fset, map[string]*ast.File{"probe.go": file}
	}
	reg := []outcomeSwitchRegistration{{dir: "x", file: "probe.go", fn: "total"}, {dir: "x", file: "probe.go", fn: "mixed", domain: outcomeDomainDiagnostic}}
	t.Run("missing_member_in_total_switch", func(t *testing.T) {
		fset, files := parse(`package x
func total(cmd types.ExecutedCommand) bool {
	switch strings.TrimSpace(cmd.Outcome) {
	case types.ExecutedCommandOutcomeExecuted:
		return true
	default:
		return false
	}
}
func mixed(diag types.VerificationDiagnostic) bool { switch diag.Outcome { case types.ExecutedCommandOutcomeTimeout: return true }; return false }
`)
		problems := outcomeCensusCheck(outcomeSwitchesIn(fset, "x", files, members), reg, members)
		if len(problems) != 1 || !strings.Contains(problems[0], "does not enumerate the members ExecutedCommandOutcomeTimeout") {
			t.Fatalf("problems = %v", problems)
		}
	})
	t.Run("member_spelled_as_literal_even_in_mixed_switch", func(t *testing.T) {
		fset, files := parse(`package x
func total(cmd types.ExecutedCommand) bool {
	switch cmd.Outcome {
	case types.ExecutedCommandOutcomeExecuted, "timeout":
		return true
	}
	return false
}
func mixed(diag types.VerificationDiagnostic) bool { switch diag.Outcome { case "passed", "executed": return true }; return false }
`)
		problems := outcomeCensusCheck(outcomeSwitchesIn(fset, "x", files, members), reg, members)
		joined := strings.Join(problems, "\n")
		if !strings.Contains(joined, `member ExecutedCommandOutcomeTimeout spelled as the literal "timeout"`) ||
			!strings.Contains(joined, `member ExecutedCommandOutcomeExecuted spelled as the literal "executed"`) {
			t.Fatalf("problems = %v", problems)
		}
	})
	t.Run("unregistered_switch_and_stale_registration", func(t *testing.T) {
		fset, files := parse(`package x
func total(cmd types.ExecutedCommand) bool {
	switch cmd.Outcome {
	case types.ExecutedCommandOutcomeExecuted, types.ExecutedCommandOutcomeTimeout:
		return true
	}
	return false
}
func stranger(cmd types.ExecutedCommand) bool {
	switch cmd.Outcome {
	case types.ExecutedCommandOutcomeExecuted, types.ExecutedCommandOutcomeTimeout:
		return true
	}
	return false
}
`)
		problems := outcomeCensusCheck(outcomeSwitchesIn(fset, "x", files, members), reg, members)
		joined := strings.Join(problems, "\n")
		if !strings.Contains(joined, "x/probe.go:stranger is not registered") || !strings.Contains(joined, "stale registration: no switch over ExecutedCommand.Outcome found in x/probe.go:mixed") {
			t.Fatalf("problems = %v", problems)
		}
	})
	t.Run("total_switch_naming_every_member_is_green", func(t *testing.T) {
		fset, files := parse(`package x
func total(cmd types.ExecutedCommand) bool {
	switch cmd.Outcome {
	case "", types.ExecutedCommandOutcomeExecuted:
		return true
	case types.ExecutedCommandOutcomeTimeout:
		return false
	default:
		return false
	}
}
func mixed(diag types.VerificationDiagnostic) bool { switch diag.Outcome { case "passed", types.ExecutedCommandOutcomeExecuted: return true }; return false }
`)
		if problems := outcomeCensusCheck(outcomeSwitchesIn(fset, "x", files, members), reg, members); len(problems) != 0 {
			t.Fatalf("problems = %v", problems)
		}
	})
}
