package operation

import (
	"fmt"
	"strings"
)

// OutputSummary is a deterministic compact view of command output for handoff
// and operation memory. It is intentionally heuristic and advisory.
type OutputSummary struct {
	Kind    string
	Summary string
	Lines   int
	Bytes   int
}

func SummarizeStepOutput(step CommandStepResult) OutputSummary {
	text := strings.TrimSpace(step.OutputPreview)
	lines := nonEmptyLines(text)
	kind := classifyOutputKind(step, lines)
	summary := summarizeLines(kind, lines, step)
	return OutputSummary{
		Kind:    kind,
		Summary: summary,
		Lines:   len(lines),
		Bytes:   len(step.OutputPreview),
	}
}

func classifyOutputKind(step CommandStepResult, lines []string) string {
	lower := strings.ToLower(strings.Join(lines, "\n"))
	switch {
	case step.Status == StatusFailed:
		return "error_output"
	case strings.Contains(lower, "usage:") || strings.Contains(lower, "options:") || strings.Contains(lower, "--help"):
		return "help_text"
	case looksLikeSearchHits(lines):
		return "search_hits"
	case step.PayloadRef != "" || len(step.OutputPreview) > 2048 || len(lines) > 40:
		return "large_output_summary"
	case strings.TrimSpace(step.OutputPreview) == "":
		return "empty_output"
	default:
		return "command_output"
	}
}

func summarizeLines(kind string, lines []string, step CommandStepResult) string {
	if len(lines) == 0 {
		if step.Error != "" {
			return oneLine(step.Error, 500)
		}
		return "command produced no output"
	}
	first := oneLine(lines[0], 220)
	switch kind {
	case "help_text":
		return fmt.Sprintf("help text preview; first line: %s", first)
	case "search_hits":
		return fmt.Sprintf("search output with %d preview hit line(s); first hit: %s", len(lines), first)
	case "large_output_summary":
		return fmt.Sprintf("large output preview with %d non-empty line(s); first line: %s", len(lines), first)
	case "error_output":
		if step.FailureClass != "" {
			return fmt.Sprintf("%s failure; first line: %s", step.FailureClass, first)
		}
		return "command failed; first line: " + first
	default:
		if len(lines) == 1 {
			return first
		}
		return fmt.Sprintf("%d output line(s); first line: %s", len(lines), first)
	}
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= 80 {
			break
		}
	}
	return out
}

func looksLikeSearchHits(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	hits := 0
	for _, line := range lines {
		if strings.Count(line, ":") >= 2 || strings.Count(line, "-") >= 2 && strings.Contains(line, ":") {
			hits++
		}
	}
	return hits >= 2 || hits == len(lines) && hits > 0
}
