package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTypedReconciliationRequestedScopeKeepsActualQueryMeasurements(t *testing.T) {
	start, end := 2.0, 2.020
	row := tool.RuntimeTraceReconciliationRow{
		Subject: "app-100", EvidenceTag: "E1", WindowStartTs: 2, WindowEndTs: 2.021,
		WindowMS: 21, TotalMS: 21, RunnableMS: .020, SleepMS: 20.980,
		WindowScope: types.ResolveTraceQueryWindowScope(&types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2..2.020",
		}, 2, 2.021),
	}
	before, _ := json.Marshal(row)
	finding := renderTargetStateReconciliation(row)
	for lang, got := range map[string]string{"zh": finding.entryZH, "en": finding.entry} {
		if !strings.Contains(got, row.WindowScope.Format(lang)) || !strings.Contains(got, "0.020ms") || !strings.Contains(got, "21.000ms") || !strings.Contains(got, "[E1]") {
			t.Fatalf("%s comparison lost actual values, evidence, or scope: %s", lang, got)
		}
	}
	after, _ := json.Marshal(row)
	if string(before) != string(after) {
		t.Fatal("scope description changed a reconciliation measurement")
	}
	row.WindowScope = types.TraceQueryWindowScope{}
	legacy := renderTargetStateReconciliation(row)
	if strings.Contains(legacy.entry, "requested window") || strings.Contains(legacy.entryZH, "用户指定范围") {
		t.Fatal("legacy reconciliation cannot invent requested scope")
	}
}

func TestTypedReconciliationRequestedScopeRankRowsKeepTheirQueryRuler(t *testing.T) {
	start, end := 2.0, 2.020
	profile := &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2..2.020",
	}
	for _, scope := range []types.TraceQueryWindowScope{
		types.ResolveTraceQueryWindowScope(profile, 2, 2.021),
		types.ResolveTraceQueryWindowScope(profile, 0, 0),
		{},
	} {
		row := tool.RuntimeTraceReconciliationRow{
			Subject: "worker-200", CauseToken: "runnable", FixDirection: "scheduling",
			EffectiveMS: .020, EvidenceTag: "E2", WindowScope: scope,
		}
		before, _ := json.Marshal(row)
		for name, finding := range map[string]proseScalarBindingFinding{
			"rank": renderRankOneReconciliation(row), "direction": renderDirectionReconciliation(row),
		} {
			for lang, got := range map[string]string{"zh": finding.entryZH, "en": finding.entry} {
				if !strings.Contains(got, "0.020ms") || !strings.Contains(got, "[E2]") {
					t.Fatalf("%s/%s lost its original measured value or evidence: %s", name, lang, got)
				}
				if note := scope.Format(lang); note != "" && !strings.Contains(got, note) {
					t.Fatalf("%s/%s did not retain the rank row's exact query scope: %s", name, lang, got)
				} else if note == "" && (strings.Contains(got, "requested window") || strings.Contains(got, "用户指定范围")) {
					t.Fatalf("%s/%s invented a request ruler for a legacy row: %s", name, lang, got)
				}
			}
		}
		after, _ := json.Marshal(row)
		if string(before) != string(after) {
			t.Fatal("rank scope disclosure changed the selected row")
		}
	}
}
