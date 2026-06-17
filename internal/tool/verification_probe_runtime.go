package tool

import (
	"sort"
	"strings"
)

const defaultVerificationProbeLanguage = "python"

var verificationProbeLanguageAliases = map[string]string{
	"python":     "python",
	"py":         "python",
	"javascript": "javascript",
	"js":         "javascript",
	"node":       "javascript",
	"ruby":       "ruby",
	"rb":         "ruby",
	"go":         "go",
	"golang":     "go",
}

func supportedVerificationProbeLanguages() []string {
	out := []string{"python", "javascript", "ruby", "go"}
	sort.Strings(out)
	return out
}

func supportedVerificationProbeLanguageSet() map[string][]string {
	return map[string][]string{
		"$.verification_probes[].language": supportedVerificationProbeLanguages(),
	}
}

func supportedVerificationProbeLanguageList() string {
	return strings.Join(supportedVerificationProbeLanguages(), ", ")
}

func normalizeVerificationProbeLanguage(raw string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return defaultVerificationProbeLanguage, true
	}
	lang, ok := verificationProbeLanguageAliases[key]
	return lang, ok
}

func verificationProbeHasExecutableFailureSignal(language, code string) bool {
	switch language {
	case "python":
		return pythonVerificationProbeHasExecutableFailureSignal(code)
	case "javascript":
		return javascriptVerificationProbeHasExecutableFailureSignal(code)
	case "ruby":
		return rubyVerificationProbeHasExecutableFailureSignal(code)
	case "go":
		return goVerificationProbeHasExecutableFailureSignal(code)
	default:
		return false
	}
}

func javascriptVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripCLikeProbeStringsAndComments(code))
	for _, signal := range []string{
		"assert(",
		"assert.",
		"console.assert(",
		"throw ",
		"throw(",
		"process.exit(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func rubyVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripRubyProbeStringsAndComments(code))
	for _, signal := range []string{
		"raise ",
		"raise(",
		"fail ",
		"fail(",
		"abort(",
		"exit(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func goVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripCLikeProbeStringsAndComments(code))
	for _, signal := range []string{
		"panic(",
		"os.Exit(",
		"log.Fatal(",
		"log.Fatalf(",
		"t.Fatal(",
		"t.Fatalf(",
	} {
		if strings.Contains(surface, compactProbeSignalSurface(signal)) {
			return true
		}
	}
	return false
}

func compactProbeSignalSurface(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if isPythonProbeASCIILetter(ch) || isPythonProbeASCIIDigit(ch) || ch == '_' || ch == '.' || ch == '(' || ch == ' ' {
			b.WriteByte(ch)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func stripCLikeProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if quote != '`' {
					inString = false
				}
				escaped = false
				continue
			}
			b.WriteByte(' ')
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(src) {
			next := src[i+1]
			if next == '/' {
				b.WriteString("  ")
				i += 2
				for i < len(src) && src[i] != '\n' {
					b.WriteByte(' ')
					i++
				}
				if i < len(src) && src[i] == '\n' {
					b.WriteByte('\n')
				}
				continue
			}
			if next == '*' {
				b.WriteString("  ")
				i += 2
				for i < len(src) {
					if src[i] == '\n' {
						b.WriteByte('\n')
					} else {
						b.WriteByte(' ')
					}
					if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
						b.WriteByte(' ')
						i++
						break
					}
					i++
				}
				continue
			}
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			inString = true
			quote = ch
			escaped = false
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func stripRubyProbeStringsAndComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	inString := false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(src); i++ {
		ch := src[i]
		if inString {
			if ch == '\n' {
				b.WriteByte('\n')
				if quote != '`' {
					inString = false
				}
				escaped = false
				continue
			}
			b.WriteByte(' ')
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '#' {
			for i < len(src) && src[i] != '\n' {
				b.WriteByte(' ')
				i++
			}
			if i < len(src) && src[i] == '\n' {
				b.WriteByte('\n')
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			inString = true
			quote = ch
			escaped = false
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
