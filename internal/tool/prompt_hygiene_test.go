package tool

import (
	"strings"
	"testing"
)

// The former TestToolSchemasDoNotExposeInternalMechanismTerms (a second,
// hand-written tool roster with a bare substring matcher) was merged
// into glossary_lint_test.go :: TestNoInternalTermsInToolSchemas over
// llmFacingToolRoster (§40.52); TestToolRosterCensus keeps that roster
// total against the source.

func TestGrepToolPromptDocumentsRuntimeArtifactControls(t *testing.T) {
	grep := &GrepTool{}
	description := grep.Description()
	for _, want := range []string{
		"fixed_string=true",
		"line_start/line_end",
		"trace/systrace/htrace/perf artifacts",
		"large log files",
		"do not grep for E|pid|name",
		`trace_query(view="span_window"`,
		"Do NOT use the result to count matches by eye",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("grep description missing %q:\n%s", want, description)
		}
	}
	parameters := string(grep.Parameters())
	for _, want := range []string{
		`"fixed_string"`,
		`"line_start"`,
		`"line_end"`,
		`"context_lines"`,
	} {
		if !strings.Contains(parameters, want) {
			t.Fatalf("grep parameters missing %q:\n%s", want, parameters)
		}
	}
}

func TestReadFilePromptDocumentsLineOffsetCoordinates(t *testing.T) {
	readFile := &ReadFile{}
	description := readFile.Description()
	for _, want := range []string{
		"line_offset/limit",
		"line_offset=100 starts at source line 101",
		"not a byte or character offset",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("read_file description missing %q:\n%s", want, description)
		}
	}
	parameters := string(readFile.Parameters())
	for _, want := range []string{
		`"line_offset"`,
		"zero-based LINE offset",
		"not a byte or character offset",
	} {
		if !strings.Contains(parameters, want) {
			t.Fatalf("read_file parameters missing %q:\n%s", want, parameters)
		}
	}
	if strings.Contains(parameters, `"offset"`) {
		t.Fatalf("read_file model-facing schema must not expose legacy offset:\n%s", parameters)
	}
}
