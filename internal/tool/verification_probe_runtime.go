package tool

import (
	"encoding/json"
	"strings"
)

const defaultVerificationProbeLanguage = "python"

type verificationProbeRuntimeSpec struct {
	Language    string
	Aliases     []string
	Description string
}

var verificationProbeRuntimeSpecs = []verificationProbeRuntimeSpec{
	{Language: "python", Aliases: []string{"py"}, Description: "python"},
	{Language: "javascript", Aliases: []string{"js", "node"}, Description: "javascript (Node.js)"},
	{Language: "ruby", Aliases: []string{"rb"}, Description: "ruby"},
	{Language: "java", Aliases: []string{"javac"}, Description: "java (JDK javac/java)"},
	{Language: "go", Aliases: []string{"golang"}, Description: "go"},
}

func supportedVerificationProbeLanguages() []string {
	out := make([]string, 0, len(verificationProbeRuntimeSpecs))
	for _, spec := range verificationProbeRuntimeSpecs {
		out = append(out, spec.Language)
	}
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

func supportedVerificationProbeRuntimeDescription() string {
	parts := make([]string, 0, len(verificationProbeRuntimeSpecs))
	for _, spec := range verificationProbeRuntimeSpecs {
		parts = append(parts, spec.Description)
	}
	return strings.Join(parts, ", ")
}

func verificationProbeLanguageEnumJSON() string {
	data, err := json.Marshal(supportedVerificationProbeLanguages())
	if err != nil {
		return `["python","javascript","ruby","go"]`
	}
	return string(data)
}

func injectVerificationProbeLanguageSchema(schema string) json.RawMessage {
	schema = strings.ReplaceAll(schema, "__VERIFICATION_PROBE_LANGUAGE_ENUM__", verificationProbeLanguageEnumJSON())
	schema = strings.ReplaceAll(schema, "__VERIFICATION_PROBE_LANGUAGE_DESCRIPTION__", supportedVerificationProbeRuntimeDescription())
	return json.RawMessage(schema)
}

func normalizeVerificationProbeLanguage(raw string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return defaultVerificationProbeLanguage, true
	}
	for _, spec := range verificationProbeRuntimeSpecs {
		if key == spec.Language {
			return spec.Language, true
		}
		for _, alias := range spec.Aliases {
			if key == alias {
				return spec.Language, true
			}
		}
	}
	return "", false
}

func verificationProbeHasExecutableFailureSignal(language, code string) bool {
	switch language {
	case "python":
		return pythonVerificationProbeHasExecutableFailureSignal(code)
	case "javascript":
		return javascriptVerificationProbeHasExecutableFailureSignal(code)
	case "ruby":
		return rubyVerificationProbeHasExecutableFailureSignal(code)
	case "java":
		return javaVerificationProbeHasExecutableFailureSignal(code)
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

func javaVerificationProbeHasExecutableFailureSignal(code string) bool {
	surface := compactProbeSignalSurface(stripCLikeProbeStringsAndComments(code))
	for _, signal := range []string{
		"assert ",
		"assert(",
		"throw ",
		"throw new",
		"System.exit(",
		"Assertions.",
		"Assert.",
		"assertEquals(",
		"assertTrue(",
		"assertFalse(",
		"fail(",
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
