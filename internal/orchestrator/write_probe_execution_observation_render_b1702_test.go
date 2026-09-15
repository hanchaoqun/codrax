package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1702ProbeExecutionObservationSystemCardsPreserveVerdictAndOwnership(t *testing.T) {
	var report types.ChangeReport
	raw := `{"plan_id":"probe-plan","channel":"post_apply_verify","passed":true,"verification_status":"passed","test_results":[{"assertion_id":"project-suite","passed":true}],"verification_diagnostics":[{"category":"probe_unavailable","reason_code":"verification_probe_javascript_syntax_error","runner":"verification_probe","outcome":"parser_error","probe_execution_observations":[{"plan_id":"probe-plan","probe_id":"native-check","execution_id":"exec-observed","definition_sha256":"definition-observed","invocation_sha256":"invocation-observed","output_excerpt":"SyntaxError: binding already declared","output_ref":"/outputs/retained-native-check.txt"}]}]}`
	const receipt = `"executed_commands":[{"runner":"verification_probe","framework":"javascript","source":"pre_suite_verification_probe","outcome":"parser_error","exit_code":1,"probe_execution":{"version":1,"definition_sha256":"definition-observed","invocation_sha256":"invocation-observed","execution_id":"exec-observed","started_at":"2026-09-15T10:00:00Z","finished_at":"2026-09-15T10:00:01Z","repository_root":"/repo","executable":"/bin/node","args":["-e","native program"],"working_dir":"/repo"}}],`
	raw = strings.Replace(raw, `"verification_diagnostics":`, receipt+`"verification_diagnostics":`, 1)
	raw = strings.ReplaceAll(raw, "definition-observed", strings.Repeat("a", 64))
	raw = strings.ReplaceAll(raw, "invocation-observed", strings.Repeat("b", 64))
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		for _, renderer := range []struct {
			name string
			run  func(*types.ChangeReport, string) string
		}{
			{"passed", renderVerifySuccess},
			{"unverified", renderVerifyUnverified},
			{"failed", func(report *types.ChangeReport, lang string) string { return renderVerifyFailure(report, "", lang) }},
		} {
			t.Run(renderer.name+"/"+lang, func(t *testing.T) {
				before := b1673JSON(t, report)
				base := report
				base.VerificationDiagnostics = nil
				prefix := renderer.run(&base, lang)
				got := renderer.run(&report, lang)
				if !strings.HasPrefix(got, prefix) {
					t.Fatal("supplement rewrote the pre-existing verdict or final report")
				}
				note := strings.TrimPrefix(got, prefix)
				for _, want := range []string{"SyntaxError: binding already declared", "/outputs/retained-native-check.txt"} {
					if strings.Count(note, want) != 1 {
						t.Errorf("system note lost or duplicated %q: %s", want, note)
					}
				}
				if lang == "zh" {
					for _, forbidden := range []string{"probe_execution_unavailable", "parser_error", "probe_execution_observation", "output_excerpt=", "output_ref=", "plan_id="} {
						if strings.Contains(note, forbidden) {
							t.Errorf("Chinese explanatory chrome leaked enum/field %q", forbidden)
						}
					}
				}
				if string(before) != string(b1673JSON(t, report)) {
					t.Fatal("system-owned supplementary card mutated original report")
				}
			})
		}
	}
}
