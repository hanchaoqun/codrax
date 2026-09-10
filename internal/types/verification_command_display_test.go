package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSuiteSkippedWriteContextUsesNonExecutionMeaning(t *testing.T) {
	for _, policy := range []bool{false, true} {
		t.Run(fmt.Sprint(policy), func(t *testing.T) {
			cmd := ExecutedCommand{Runner: "python", Framework: "unittest", WorkingDir: ".", Command: "python3 -m unittest discover -v", Outcome: ExecutedCommandOutcomeSuiteSkipped, Source: "unrecognized", ExitCode: 0}
			if policy {
				cmd.Source = "probe_primary_suite_skipped"
			}
			report := &ChangeReport{Passed: true, ExecutedCommands: []ExecutedCommand{cmd}}
			before, _ := json.Marshal(report)
			pack := WriteContextPackFromChangeReport(report)
			var text string
			for _, item := range pack.Items {
				if item.Kind == "executed_command" {
					text = item.Text
				}
			}
			if !strings.Contains(text, "execution=not_run") || strings.Contains(text, "exit=0") || !strings.Contains(text, cmd.Command) {
				t.Fatalf("shared context misrepresented skipped execution: %s", text)
			}
			if strings.Contains(text, "bounded probes passed") != policy {
				t.Fatalf("policy explanation must require exact source: %s", text)
			}
			if policy && (!strings.Contains(text, "not evidence of a missing environment") || !strings.Contains(text, "not customer-source search targets")) {
				t.Fatalf("policy skip invites an environment/source search: %s", text)
			}
			after, _ := json.Marshal(report)
			if string(before) != string(after) {
				t.Fatal("shared display mutated durable report")
			}
		})
	}
}

func TestSuiteSkippedDisplayPreservesOtherOutcomeBytes(t *testing.T) {
	for _, outcome := range append(AllExecutedCommandOutcomes(), "", "future_outcome") {
		if outcome == ExecutedCommandOutcomeSuiteSkipped {
			continue
		}
		for _, code := range []int{0, 7} {
			cmd := ExecutedCommand{Runner: "python", WorkingDir: ".", Command: "python3 -m unittest", Outcome: outcome, Source: "probe_primary_suite_skipped", ExitCode: code}
			want := fmt.Sprintf("command=%s cwd=. runner=python exit=%d", cmd.Command, code)
			if outcome != "" {
				want += " outcome=" + outcome
			}
			if got := renderExecutedCommandContext(cmd); got != want {
				t.Errorf("non-skip outcome %q display changed: got %q want %q", outcome, got, want)
			}
		}
	}
	if got := renderExecutedCommandContext(ExecutedCommand{}); got != "" {
		t.Fatalf("empty legacy row acquired display: %q", got)
	}
}

func TestSuiteSkippedWriteContextKeepsMeaningBeforeCommandCap(t *testing.T) {
	cmd := ExecutedCommand{Runner: "python", Command: strings.Repeat("long-command-", 100),
		Outcome: ExecutedCommandOutcomeSuiteSkipped, Source: "probe_primary_suite_skipped"}
	text := renderExecutedCommandContext(cmd)
	for _, want := range []string{"execution=not_run", "bounded probes passed", "not evidence of a missing environment", "not customer-source search targets", "command=long-command-"} {
		if !strings.Contains(text, want) {
			t.Errorf("bounded shared display lost %q: %s", want, text)
		}
	}
	if len([]rune(text)) > writeContextPackTextLen+3 || !strings.HasSuffix(text, "...") {
		t.Fatalf("existing display budget changed: %d runes", len([]rune(text)))
	}
}
