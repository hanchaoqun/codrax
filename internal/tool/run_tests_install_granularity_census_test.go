package tool

import "testing"

// Granularity and failure context are explicit addends, never replacements for
// the audit sentence or another report installation. Mutating any one of
// these structural obligations must remain red independently of runtime tests.
func TestRunTestsInstallChokePointGranularityCompositionMutation(t *testing.T) {
	build := func(body string) string {
		return chokePointPrelude + `
func (t *RunTests) Execute(ctx *types.BusContext, dryRunProbe bool, report *types.ChangeReport, base string) types.ToolResult {
    installFinishedReport := func(report *types.ChangeReport, base string) string {
` + body + `
    }
    return types.ToolResult{Summary: installFinishedReport(report, base)}
}`
	}
	const install = "installRunTestsReport(ctx, report, dryRunProbe)\n"
	const failureAddend = " + renderRunTestsFailureContextSummary(report)"
	const expression = "base + renderRunTestsWorktreeAuditSummary(report) + renderRunTestsProbeGranularitySummary(report)" + failureAddend
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"all disclosures", install + "return " + expression, true},
		{"grouped additions", install + "return (base + (renderRunTestsWorktreeAuditSummary(report) + renderRunTestsProbeGranularitySummary(report))" + failureAddend + ")", true},
		{"missing audit", install + "return base + renderRunTestsProbeGranularitySummary(report)" + failureAddend, false},
		{"missing granularity", install + "return base + renderRunTestsWorktreeAuditSummary(report)" + failureAddend, false},
		{"missing failure context", install + "return base + renderRunTestsWorktreeAuditSummary(report) + renderRunTestsProbeGranularitySummary(report)", false},
		{"wrong audit report", install + "return base + renderRunTestsWorktreeAuditSummary(other) + renderRunTestsProbeGranularitySummary(report)" + failureAddend, false},
		{"wrong granularity report", install + "return base + renderRunTestsWorktreeAuditSummary(report) + renderRunTestsProbeGranularitySummary(other)" + failureAddend, false},
		{"different installed report", "installRunTestsReport(ctx, other, dryRunProbe)\nreturn " + expression, false},
		{"duplicate granularity", install + "return " + expression + " + renderRunTestsProbeGranularitySummary(report)", false},
		{"unknown addend", install + "return " + expression + " + unrelated(report)", false},
		{"double installation", install + install + "return " + expression, false},
		{"no installation", "return " + expression, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violations := chokePointSelfRed(t, build(tc.body))
			if (len(violations) == 0) != tc.valid {
				t.Fatalf("valid=%v violations=%v", tc.valid, violations)
			}
		})
	}
}
