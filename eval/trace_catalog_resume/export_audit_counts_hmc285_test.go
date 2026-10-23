package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

// Literal expected counts deliberately do not use the producer's summarizer.
// This tests the independent offline audit, not physical package acceptance;
// existing public export tests cover references, copied bytes and final links.
func hmc285AuditCounts() tracecatalog.ExportManifest {
	attempt := func(id string, outcome tracecatalog.Outcome) tracecatalog.Attempt {
		return tracecatalog.Attempt{ID: id, Completion: tracecatalog.Completion{Outcome: outcome}}
	}
	return tracecatalog.ExportManifest{
		Catalog: tracecatalog.Snapshot{
			Queries: []tracecatalog.QueryRecord{
				{ID: "declared", Outcome: tracecatalog.OutcomeSuccess, Attempts: []tracecatalog.Attempt{
					attempt("failed-first", tracecatalog.OutcomeFailure), attempt("successful-retry", tracecatalog.OutcomeSuccess),
				}},
				{ID: "resumed", Outcome: tracecatalog.OutcomeNotExecuted},
				{ID: "observed", Outcome: tracecatalog.OutcomeEmpty, Attempts: []tracecatalog.Attempt{attempt("empty-query", tracecatalog.OutcomeEmpty)}},
			},
			History: []tracecatalog.HistoricalRun{{ID: "old-run", Queries: []tracecatalog.QueryRecord{
				{ID: "old-query", Outcome: tracecatalog.OutcomeSuccess, Attempts: []tracecatalog.Attempt{attempt("old-attempt", tracecatalog.OutcomeSuccess)}},
			}}},
		},
		QueryOrigins: map[string]tracecatalog.QueryOrigin{
			"declared": tracecatalog.QueryOriginDeclared, "resumed": tracecatalog.QueryOriginResumed, "observed": tracecatalog.QueryOriginObserved,
		},
		ExecutionCoverage: tracecatalog.ExecutionCoverage{
			Basis: "registered_current_plans", DeclaredPlans: 1, ResumedPlans: 1, ObservedQueries: 1,
			Attempts: 3, QueriesWithAttempts: 2, QueriesWithoutAttempts: 1,
			AttemptOutcomes:  map[tracecatalog.Outcome]int{tracecatalog.OutcomeFailure: 1, tracecatalog.OutcomeSuccess: 1, tracecatalog.OutcomeEmpty: 1},
			PlannedOutcomes:  map[tracecatalog.Outcome]int{tracecatalog.OutcomeSuccess: 1, tracecatalog.OutcomeNotExecuted: 1},
			ObservedOutcomes: map[tracecatalog.Outcome]int{tracecatalog.OutcomeEmpty: 1},
		},
		MaterialComplete: false,
	}
}

func hmc285AuditFiles() map[string]tracecatalog.ExportFile {
	return map[string]tracecatalog.ExportFile{"report.md": {Path: "report.md"}}
}

func TestHMC285ExportAuditSeparatesRecordsAttemptsAndLatestOutcomes(t *testing.T) {
	if err := inspectExportReferences(hmc285AuditCounts(), hmc285AuditFiles()); err != nil {
		t.Fatalf("independent three-record/three-attempt audit: %v", err)
	}
}

func TestHMC285ExportAuditRejectsChangedCoverage(t *testing.T) {
	tests := []struct {
		name string
		edit func(*tracecatalog.ExecutionCoverage)
	}{
		{"basis", func(c *tracecatalog.ExecutionCoverage) { c.Basis = "observed_queries_only" }},
		{"declared", func(c *tracecatalog.ExecutionCoverage) { c.DeclaredPlans++ }},
		{"resumed", func(c *tracecatalog.ExecutionCoverage) { c.ResumedPlans++ }},
		{"observed_origin_is_not_all_executed", func(c *tracecatalog.ExecutionCoverage) { c.ObservedQueries = 2 }},
		{"attempts", func(c *tracecatalog.ExecutionCoverage) { c.Attempts++ }},
		{"records_with_attempts", func(c *tracecatalog.ExecutionCoverage) { c.QueriesWithAttempts++ }},
		{"records_without_attempts", func(c *tracecatalog.ExecutionCoverage) { c.QueriesWithoutAttempts++ }},
		{"retry_lost", func(c *tracecatalog.ExecutionCoverage) { delete(c.AttemptOutcomes, tracecatalog.OutcomeFailure) }},
		{"attempt_result_count", func(c *tracecatalog.ExecutionCoverage) { c.AttemptOutcomes[tracecatalog.OutcomeSuccess]++ }},
		{"same_total_wrong_attempt_distribution", func(c *tracecatalog.ExecutionCoverage) {
			delete(c.AttemptOutcomes, tracecatalog.OutcomeFailure)
			c.AttemptOutcomes[tracecatalog.OutcomeSuccess]++
		}},
		{"latest_plan_includes_past_failure", func(c *tracecatalog.ExecutionCoverage) { c.PlannedOutcomes[tracecatalog.OutcomeFailure]++ }},
		{"observed_latest_changed", func(c *tracecatalog.ExecutionCoverage) {
			delete(c.ObservedOutcomes, tracecatalog.OutcomeEmpty)
			c.ObservedOutcomes[tracecatalog.OutcomeSuccess] = 1
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := hmc285AuditCounts()
			tt.edit(&m.ExecutionCoverage)
			if err := inspectExportReferences(m, hmc285AuditFiles()); err == nil || !strings.Contains(err.Error(), "execution coverage differs") {
				t.Fatalf("tampered coverage must fail exact reconstruction: %v", err)
			}
		})
	}
}

func TestHMC285ExportAuditRequiresOutcomeMaps(t *testing.T) {
	for _, field := range []string{"attempt_outcomes", "planned_outcomes", "observed_outcomes"} {
		for _, mode := range []string{"null", "omitted"} {
			t.Run(field+"/"+mode, func(t *testing.T) {
				raw, err := json.Marshal(hmc285AuditCounts())
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]json.RawMessage
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				var coverage map[string]json.RawMessage
				if err := json.Unmarshal(wire["execution_coverage"], &coverage); err != nil {
					t.Fatal(err)
				}
				delete(coverage, field)
				if mode == "null" {
					coverage[field] = json.RawMessage("null")
				}
				wire["execution_coverage"], err = json.Marshal(coverage)
				if err != nil {
					t.Fatal(err)
				}
				raw, err = json.Marshal(wire)
				if err != nil {
					t.Fatal(err)
				}
				var changed tracecatalog.ExportManifest
				if err := json.Unmarshal(raw, &changed); err != nil {
					t.Fatal(err)
				}
				if err := inspectExportReferences(changed, hmc285AuditFiles()); err == nil {
					t.Fatal("missing/null result distribution passed audit")
				}
			})
		}
	}
}

func TestHMC285ExportAuditEmptyRosterHasExplicitEmptyDistribution(t *testing.T) {
	m := tracecatalog.ExportManifest{
		QueryOrigins: map[string]tracecatalog.QueryOrigin{},
		ExecutionCoverage: tracecatalog.ExecutionCoverage{
			Basis: "observed_queries_only", AttemptOutcomes: map[tracecatalog.Outcome]int{},
			PlannedOutcomes: map[tracecatalog.Outcome]int{}, ObservedOutcomes: map[tracecatalog.Outcome]int{},
		},
		MaterialComplete: true,
	}
	if err := inspectExportReferences(m, hmc285AuditFiles()); err != nil {
		t.Fatalf("explicit empty current roster: %v", err)
	}
	m.ExecutionCoverage.AttemptOutcomes = nil
	if err := inspectExportReferences(m, hmc285AuditFiles()); err == nil {
		t.Fatal("null outcomes are not a verified empty distribution")
	}
}
