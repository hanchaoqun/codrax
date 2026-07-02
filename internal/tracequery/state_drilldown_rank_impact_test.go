package tracequery

import "testing"

// TestStateDrilldownChurnImpactStaysPhysical pins §7.30 S1 (berlin customer
// report): the fragmented state-churn ranking composite (dominant + half the
// remaining churn) must never be published as the row's physical duration.
// With the customer's numbers (running=2029.609ms of total=3325.663ms) the
// composite is 2677.636ms, which pushed the four-state sum to 119% of the
// window and contradicted the churn totals in the same report.
func TestStateDrilldownChurnImpactStaysPhysical(t *testing.T) {
	churn := ThreadStateChurnSummary{
		Thread:           ThreadRef{Comm: "oney.hmn.berlin", PID: 42591},
		DominantState:    string(StateRunning),
		DominantImpactMs: 2029.609,
		TotalMs:          3325.663,
		RunningMs:        2029.609,
		SleepMs:          1280.602,
		RunnableMs:       15.100,
		DStateMs:         0.352,
		LineStart:        797101,
		LineEnd:          1718491,
	}
	stats := WindowStats{
		Window:     TimeWindow{StartTs: 6793222.031397627, EndTs: 6793225.369801793},
		StateChurn: []ThreadStateChurnSummary{churn},
	}
	plan := buildStateDrilldownPlan(stats, 12)
	step := findStateDrilldownStepForTest(plan, 42591, "state_churn", string(StateRunning))
	if step == nil {
		t.Fatalf("churn-derived drilldown step missing: %+v", plan)
	}
	if !near(step.ImpactMs, 2029.609, 0.001) {
		t.Fatalf("published impact must be the physical dominant duration, got %.3f", step.ImpactMs)
	}
	if !near(step.RankImpactMs, 2677.636, 0.001) {
		t.Fatalf("ranking composite must live in RankImpactMs only, got %.3f", step.RankImpactMs)
	}
	if step.TotalMs != churn.TotalMs {
		t.Fatalf("total must stay the churn total, got %.3f", step.TotalMs)
	}

	// Ordering: fragmentation must still outrank a plain duration between the
	// physical impact and the composite (that is the composite's whole job).
	plain := WindowStats{
		Window:     stats.Window,
		StateChurn: []ThreadStateChurnSummary{churn},
		TopRunning: []ThreadDuration{{
			Thread: ThreadRef{Comm: "plain-runner", PID: 777}, DurationMs: 2300,
		}},
	}
	ordered := buildStateDrilldownPlan(plain, 12)
	churnStep := findStateDrilldownStepForTest(ordered, 42591, "state_churn", string(StateRunning))
	plainStep := findStateDrilldownStepForTest(ordered, 777, "top_running", string(StateRunning))
	if churnStep == nil || plainStep == nil {
		t.Fatalf("both steps must survive: %+v", ordered)
	}
	if churnStep.Rank > plainStep.Rank {
		t.Fatalf("fragmented churn (rank composite %.3f) must outrank plain 2300ms running: churn rank=%d plain rank=%d",
			churnStep.RankImpactMs, churnStep.Rank, plainStep.Rank)
	}
}

// TestNormalizeRootCauseSubjectKindMarksAggregates pins §7.30 complaints
// 1/6/8: aggregate-metric rows with a structurally empty ThreadRef are typed
// as aggregate_metric so renderers stop presenting them as an unresolved
// thread; rows with a representative or real thread keep the thread shape.
func TestNormalizeRootCauseSubjectKindMarksAggregates(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "io_pressure"},
		{Type: "io_pressure", Thread: ThreadRef{Comm: "reader", PID: 12}},
		{Type: "cpu_pressure"},
		{Type: "irq_burst"},
		{Type: "fragmented_running", Thread: ThreadRef{Comm: "ui", PID: 7}},
	}
	normalizeRootCauseSubjectKind(items)
	if items[0].SubjectKind != RootCauseSubjectKindAggregateMetric ||
		items[2].SubjectKind != RootCauseSubjectKindAggregateMetric ||
		items[3].SubjectKind != RootCauseSubjectKindAggregateMetric {
		t.Fatalf("empty-thread aggregate rows must be typed aggregate_metric: %+v", items)
	}
	if items[1].SubjectKind != "" || items[4].SubjectKind != "" {
		t.Fatalf("threaded rows must keep the thread shape: %+v", items)
	}
}
