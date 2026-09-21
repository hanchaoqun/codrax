package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These resolver tests use the real producer's qualifier. Public tool/child
// process coverage lives in project_test_observation_nested_scope_test.go.
func TestProjectTestResultScopeRunnerMatrix(t *testing.T) {
	for _, tc := range []struct{ runner, framework, file, suite, id string }{
		{"python", "unittest", "tests/test_value.py", "tests.test_value.ValueTest", "test_value"},
		{"python", "pytest", "tests/test_value.py", "tests/test_value.py::ValueTest", "test_value[one::two]"},
		{"python", "django", "app/tests/test_value.py", "app.tests.test_value.ValueTest", "test_value"},
		{"go", "", "pkg/value_test.go", "example.test/widget/pkg", "TestValue/subcase"},
		{"java", "maven", "src/test/java/example/ValueTest.java", "example.ValueTest", "example.ValueTest#checks"},
		{"node", "jest", "value.test.ts", "/repo/packages/widget/value.test.ts", "value > checks"},
		{"ruby", "", "spec/value_spec.rb", "./spec/value_spec.rb", "value returns incremented value"},
		{"rust", "", "tests/value.rs", "cargo", "value::checks"},
		{"swift", "", "Tests/ValueTests.swift", "ValueTests", "testValue"},
	} {
		for _, passed := range []bool{true, false} {
			verdict := "pass"
			if !passed {
				verdict = "fail"
			}
			t.Run(tc.runner+"/"+tc.framework+"/"+verdict, func(t *testing.T) {
				candidate := types.TestSurfaceCandidate{Runner: tc.runner, Framework: tc.framework, WorkingDir: "packages/widget", HasTestSignal: true}
				observation, report := scopeResolverFixture(t, candidate, tc.file, tc.suite, tc.id, passed)
				before, _ := json.Marshal(report)
				matches := projectTestObservationExecutionMatches(observation, report, passed)
				if len(matches) != 1 {
					t.Fatalf("producer-qualified identity must match its exact execution/path: matches=%+v report=%+v", matches, report)
				}
				if len(projectTestObservationExecutionMatches(observation, report, !passed)) != 0 {
					t.Fatal("opposite verdict borrowed this result")
				}
				if !passed {
					plan := &types.ChangePlan{ID: report.PlanID, ProjectTestObservations: []types.ProjectTestObservation{observation}}
					relevance := BuildVerifyFailureContractRelevance(report, plan)
					if len(relevance.Hits) != 1 || relevance.Hits[0].ContractID != "value-contract" {
						t.Fatalf("single exact failed execution must retain failure relevance: %+v", relevance)
					}
					// Scope recovery must not retire a contract by guessing which
					// of multiple failed invocations produced the row.
					other := report.ExecutedCommands[0]
					other.Suite += "-other"
					report.ExecutedCommands = append(report.ExecutedCommands, other)
					if got := BuildVerifyFailureContractRelevance(report, plan); len(got.Hits) != 0 {
						t.Fatalf("ambiguous failed executions gained relevance: %+v", got)
					}
					report.ExecutedCommands = report.ExecutedCommands[:1]
				}
				after, _ := json.Marshal(report)
				if string(before) != string(after) {
					t.Fatal("scope matching must not rewrite published results")
				}
			})
		}
	}
}

func scopeResolverFixture(t *testing.T, candidate types.TestSurfaceCandidate, file, suite, id string, passed bool) (types.ProjectTestObservation, *types.ChangeReport) {
	t.Helper()
	const repoRoot = "/repo"
	report := qualifyChangeReport(&types.ChangeReport{PlanID: "scope-plan", Passed: passed, TestResults: []types.TestResult{{
		Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion,
		Suite: suite, AssertionID: id, Passed: passed,
	}}}, runnerPlan{Runner: candidate.Runner, Framework: candidate.Framework, Root: filepath.Join(repoRoot, candidate.WorkingDir)}, repoRoot)
	testPath := filepath.ToSlash(filepath.Join(candidate.WorkingDir, file))
	executedSuite, ok := projectTestObservationCandidateSuite(candidate, testPath)
	if !ok {
		t.Fatalf("fixture has no supported exact selector: candidate=%+v path=%s", candidate, testPath)
	}
	exit := 0
	if !passed {
		exit = 1
	}
	report.TestSurface = &types.TestSurface{Candidates: []types.TestSurfaceCandidate{candidate}}
	report.ExecutedCommands = []types.ExecutedCommand{{Runner: candidate.Runner, Framework: candidate.Framework, WorkingDir: candidate.WorkingDir,
		Suite: executedSuite, Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: exit}}
	row := report.TestResults[0]
	return types.ProjectTestObservation{ID: "value", TestPath: testPath, AssertionSuite: row.Suite, AssertionID: row.AssertionID, ContractRefs: []string{"value-contract"}}, report
}

func TestProjectTestResultScopeRejectsCrossScopeAndMalformedIdentity(t *testing.T) {
	for _, passed := range []bool{true, false} {
		for _, mutation := range []string{"sibling", "suite_only", "id_only", "mixed_fields", "double_prefix", "wrong_framework", "unexecuted", "opposite_command", "skipped", "aggregate", "label_collision", "prefix_collision", "lying_candidate_id"} {
			name := "pass/"
			if !passed {
				name = "fail/"
			}
			t.Run(name+mutation, func(t *testing.T) {
				candidate := types.TestSurfaceCandidate{Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", HasTestSignal: true}
				observation, report := scopeResolverFixture(t, candidate, "tests/test_value.py", "tests.test_value.ValueTest", "test_value", passed)
				prefix := "python/unittest@packages/widget::"
				row := &report.TestResults[0]
				switch mutation {
				case "sibling", "mixed_fields", "lying_candidate_id":
					sibling := candidate
					sibling.WorkingDir = "packages/other"
					report.TestSurface.Candidates = append(report.TestSurface.Candidates, sibling)
					row.AssertionID = strings.Replace(row.AssertionID, prefix, "python/unittest@packages/other::", 1)
					if mutation != "mixed_fields" {
						row.Suite = strings.Replace(row.Suite, prefix, "python/unittest@packages/other::", 1)
					}
					if mutation == "lying_candidate_id" {
						report.TestSurface.Candidates[0].ID = "python/unittest@packages/other"
					}
				case "suite_only":
					row.AssertionID = strings.TrimPrefix(row.AssertionID, prefix)
				case "id_only":
					row.Suite = strings.TrimPrefix(row.Suite, prefix)
				case "double_prefix":
					row.Suite, row.AssertionID = prefix+row.Suite, prefix+row.AssertionID
				case "wrong_framework":
					report.TestSurface.Candidates[0].Framework = "pytest"
					report.ExecutedCommands[0].Framework = "pytest"
				case "unexecuted":
					report.ExecutedCommands[0].Outcome = types.ExecutedCommandOutcomeRunnerMissing
				case "opposite_command":
					report.ExecutedCommands[0].ExitCode = 1 - report.ExecutedCommands[0].ExitCode
				case "skipped":
					row.ObservationScope = types.TestObservationScopeNonAsserting
				case "aggregate":
					row.ObservationScope = types.TestObservationScopeAggregate
				case "label_collision":
					// Non-Python frameworks do not appear in producer labels.
					candidate = types.TestSurfaceCandidate{Runner: "node", Framework: "jest", WorkingDir: "packages/widget", HasTestSignal: true}
					observation, report = scopeResolverFixture(t, candidate, "value.test.js", "value.test.js", "checks", passed)
					other := candidate
					other.Framework = "vitest"
					report.TestSurface.Candidates = append(report.TestSurface.Candidates, other)
					row = &report.TestResults[0]
				case "prefix_collision":
					other := candidate
					other.WorkingDir += "::nested"
					report.TestSurface.Candidates = append(report.TestSurface.Candidates, other)
					row.Suite, row.AssertionID = prefix+"nested::"+"ValueTest", prefix+"nested::"+"test_value"
				}
				// Even an exact model declaration of the wrong published row
				// cannot make it evidence for this candidate/path.
				observation.AssertionSuite, observation.AssertionID = row.Suite, row.AssertionID
				if got := projectTestObservationExecutionMatches(observation, report, passed); len(got) != 0 {
					t.Fatalf("%s borrowed or forged scope: %+v", mutation, got)
				}
			})
		}
	}
}

func TestProjectTestResultScopeRootCannotBorrowNestedCommandIdentity(t *testing.T) {
	for _, catalogFromCommandOnly := range []bool{false, true} {
		candidate := types.TestSurfaceCandidate{Runner: "go", WorkingDir: ".", HasTestSignal: true}
		observation, report := scopeResolverFixture(t, candidate, "value_test.go", "example.test/widget", "TestValue", true)
		if !projectTestObservationExecuted(observation, report) {
			t.Fatal("root control must pass")
		}
		nested := types.TestSurfaceCandidate{Runner: "go", WorkingDir: "child", HasTestSignal: true}
		if !catalogFromCommandOnly {
			report.TestSurface.Candidates = append(report.TestSurface.Candidates, nested)
		}
		report.ExecutedCommands = append(report.ExecutedCommands, types.ExecutedCommand{Runner: "go", WorkingDir: "child", Suite: ".", Outcome: types.ExecutedCommandOutcomeExecuted})
		row := &report.TestResults[0]
		row.Suite, row.AssertionID = "go@child::"+row.Suite, "go@child::"+row.AssertionID
		observation.AssertionSuite, observation.AssertionID = row.Suite, row.AssertionID
		if projectTestObservationExecuted(observation, report) {
			t.Fatal("root '.' selector borrowed nested evidence")
		}
	}
}

func TestProjectTestResultScopeNativeDelimitersAreNotQualifiers(t *testing.T) {
	candidate := types.TestSurfaceCandidate{Runner: "python", Framework: "pytest", WorkingDir: ".", HasTestSignal: true}
	observation, report := scopeResolverFixture(t, candidate, "tests/test_value.py", "tests/test_value.py::ValueTest", "test_value[one::two]", true)
	report.TestSurface.Candidates = append(report.TestSurface.Candidates, types.TestSurfaceCandidate{Runner: "python", Framework: "pytest", WorkingDir: "nested"})
	if !projectTestObservationExecuted(observation, report) {
		t.Fatal("native pytest class and parameter delimiters must remain exact identities")
	}
}

func TestRunnerResultScopeLabelPreservesProducerDialect(t *testing.T) {
	for _, tc := range []struct{ runner, framework, want string }{
		{"python", " unittest ", "python/unittest@modules/a"},
		{"java", "gradle", "java/gradle@modules/a"},
		{"go", "unused", "go@modules/a"},
		{"node", "jest", "node@modules/a"},
		{"hvigor", "arkts", "hvigor@modules/a"},
		{"cjpm", "cangjie", "cjpm@modules/a"},
	} {
		// A qualifier test is not a claim that every runner has a native
		// assertion/PTO execution lane.
		if got := runnerResultScopeLabel(tc.runner, tc.framework, "modules/a"); got != tc.want {
			t.Fatalf("%s published qualifier changed: got=%s want=%s", tc.runner, got, tc.want)
		}
	}
}
