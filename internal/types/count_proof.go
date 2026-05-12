package types

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	bareCountLinePattern       = regexp.MustCompile(`^\s*(-?\d+)\s*$`)
	labeledCountLinePattern    = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(?:count|total)\s*[:=]\s*(-?\d+)(?:[^A-Za-z0-9_]|$)`)
	reversedCountLinePattern   = regexp.MustCompile(`(?i)^\s*(-?\d+)\s+(?:total|count)\s*$`)
	wcSingleFileLinePattern    = regexp.MustCompile(`^\s*(-?\d+)\s+\S+\s*$`)
	grepCountSingleLinePattern = regexp.MustCompile(`^[^:\n]+:\s*(-?\d+)\s*$`)
)

// DeterministicCountProofInteger extracts a scalar count only from
// command-output shapes that already represent a final count. It
// deliberately rejects grep / rg match listings such as
// "file.go:63: text", because those contain integers but still
// require visual tallying. Count questions must not close on those
// noisy surfaces.
func DeterministicCountProofInteger(summary string) (int, bool) {
	lines := countProofPayloadLines(summary)
	if len(lines) == 0 {
		return 0, false
	}
	if len(lines) == 1 {
		return deterministicCountFromSingleLine(lines[0])
	}

	var found int
	foundOK := false
	for i, line := range lines {
		if value, ok := deterministicCountFromMultiLine(line, i == len(lines)-1); ok {
			if foundOK {
				return 0, false
			}
			found, foundOK = value, true
		}
	}
	if !foundOK {
		return 0, false
	}
	return found, true
}

func countProofPayloadLines(summary string) []string {
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	rawLines := strings.Split(summary, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "[exec_command:") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func deterministicCountFromSingleLine(line string) (int, bool) {
	if value, ok := parseCountPattern(line, bareCountLinePattern); ok {
		return value, true
	}
	if value, ok := parseCountPattern(line, labeledCountLinePattern); ok {
		return value, true
	}
	if value, ok := parseCountPattern(line, reversedCountLinePattern); ok {
		return value, true
	}
	if value, ok := parseCountPattern(line, grepCountSingleLinePattern); ok {
		return value, true
	}
	if strings.Count(line, ":") == 0 {
		if value, ok := parseCountPattern(line, wcSingleFileLinePattern); ok {
			return value, true
		}
	}
	return 0, false
}

func deterministicCountFromMultiLine(line string, isLast bool) (int, bool) {
	if value, ok := parseCountPattern(line, labeledCountLinePattern); ok {
		return value, true
	}
	if value, ok := parseCountPattern(line, reversedCountLinePattern); ok {
		return value, true
	}
	if isLast {
		if value, ok := parseCountPattern(line, bareCountLinePattern); ok {
			return value, true
		}
	}
	return 0, false
}

func parseCountPattern(line string, pattern *regexp.Regexp) (int, bool) {
	match := pattern.FindStringSubmatch(line)
	if len(match) < 2 {
		return 0, false
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return value, true
}
