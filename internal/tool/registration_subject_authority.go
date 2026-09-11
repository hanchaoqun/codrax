package tool

import (
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// registrationDefinitionParameterReference withdraws one precise overclaim:
// a pure callable declaration whose only reference to the claimed registry is
// in an AST-owned parameter type. It does not validate other registrations.
// Missing/ambiguous parser or read coverage, attached-source references and
// inline bodies remain unknown and follow the existing evidence path.
func registrationDefinitionParameterReference(it types.EvidenceItem, gc *ground.Context) string {
	if it.AnchorKind != types.AnchorDefinition || it.Scope != types.ScopeLine {
		return ""
	}
	fi := emitEvidenceGraphFileInfo(gc, it.Source)
	if fi == nil || fi.ParseTier > 1 {
		return ""
	}
	decl := selectedDefinitionCallable(fi, it)
	if decl == nil || it.LineStart != decl.Line ||
		decl.BodyPresence != repomap.CallableBodyPresent || decl.BodyStartLine != decl.Line ||
		decl.BodyEndLine <= decl.Line {
		return ""
	}
	subject := registrationIdentifierPathTail(it.Subject)
	if subject == "" || registrationIdentifierPathTail(it.Object) != decl.Name ||
		registrationIdentifierPathTail(it.AnchorSymbol) != decl.Name {
		return ""
	}
	for _, identity := range []string{decl.Name, decl.Parent, decl.Receiver} {
		if registrationIdentifierPathTail(identity) == subject {
			return ""
		}
	}
	for _, parameter := range decl.ParameterBindings {
		if registrationIdentifierPathTail(parameter.Binding) == subject {
			return ""
		}
	}
	// Use the complete already-read line, never the possibly truncated
	// Symbol.Signature. A single terminal opening brace excludes inline body
	// calls, default/const block expressions and multiline signatures.
	line := strings.TrimSpace(evidenceVisibleLineText(gc, it.Source, decl.Line))
	if !strings.HasSuffix(line, "{") || strings.Count(line, "{") != 1 || strings.Contains(line, "}") ||
		!registrationClosedDeclarationDelimiters(line) {
		return ""
	}
	// The parser does not provide a cross-language annotation carrier. Scan
	// the already-read prefix to an actual declaration boundary, not a fixed
	// number of lines or a blank/semicolon heuristic. Stacked annotations may
	// extend arbitrarily far. The budget is only a reason to remain unknown.
	boundary := 0
	for _, previous := range fi.Symbols {
		if (previous.Kind == "function" || previous.Kind == "method") && previous.Parent == decl.Parent && previous.Receiver == decl.Receiver &&
			previous.BodyPresence == repomap.CallableBodyPresent && previous.Line > 0 &&
			previous.BodyEndLine == previous.EndLine && previous.EndLine >= previous.Line && previous.EndLine < decl.Line && previous.EndLine > boundary {
			boundary = previous.EndLine
		}
	}
	if decl.Line-boundary > 32 {
		return ""
	}
	if boundary > 0 {
		end, ok := registrationAlreadyReadLine(gc, it.Source, boundary)
		// Line metadata has no closing column; a declaration followed on the
		// same line by another annotation is not a usable prefix boundary.
		if !ok || (strings.TrimSpace(end) != "}" && strings.TrimSpace(end) != "};") {
			return ""
		}
	}
	for n := boundary + 1; n < decl.Line; n++ {
		prefix, ok := registrationAlreadyReadLine(gc, it.Source, n)
		if !ok || indexIdentifierToken(prefix, subject) >= 0 || !registrationClosedDeclarationDelimiters(prefix) {
			return ""
		}
	}
	for _, parameter := range decl.ParameterBindings {
		typeText := strings.TrimSpace(parameter.Type)
		if !registrationSimpleTypeReference(typeText, subject) || strings.Count(line, typeText) != 1 {
			continue
		}
		withoutType := strings.Replace(line, typeText, "", 1)
		if indexIdentifierToken(withoutType, subject) < 0 {
			return parameter.Binding
		}
	}
	return ""
}

// This is only a conservative unknown check, not an annotation parser. A
// closing continuation whose opener is farther back must not be mistaken for
// a complete, unrelated prefix just because the subject is farther away.
func registrationClosedDeclarationDelimiters(line string) bool {
	var stack []rune
	for _, r := range line {
		switch r {
		case '(', '[':
			stack = append(stack, r)
		case ')', ']':
			if len(stack) == 0 || (r == ')' && stack[len(stack)-1] != '(') ||
				(r == ']' && stack[len(stack)-1] != '[') {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0
}

func registrationAlreadyReadLine(gc *ground.Context, source string, n int) (string, bool) {
	if gc == nil {
		return "", false
	}
	canonical := ground.CanonicalContextPath(gc, source)
	if line, ok := gc.LineIndex[canonical][n]; ok {
		return line, true
	}
	line, ok := gc.LineIndex[source][n]
	return line, ok
}

// A complete identifier path only; annotations, expressions and prose labels
// cannot be reduced to their last word to trigger this withdrawal.
func registrationIdentifierPathTail(raw string) string {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(raw), "::", "."), ".")
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for i, r := range part {
			if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
				return ""
			}
		}
	}
	return parts[len(parts)-1]
}

// Parameter metadata is a type expression, not a bag of type identities.
// Only simple paths/wrappers/generics are considered here; skip lifetime
// tokens, and leave const expressions, array lengths, function expressions,
// quoted names and unfamiliar syntax unknown. A match proves only a
// parameter-declaration reference, never a binding or a call.
func registrationSimpleTypeReference(raw, subject string) bool {
	matched := false
	runes := []rune(raw)
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '[' {
			// Empty array/slice suffixes are type wrappers. A nonempty
			// bracket may instead contain a const-length value (Go/C).
			if i+1 >= len(runes) || runes[i+1] != ']' {
				return false
			}
			i += 2
			continue
		}
		if unicode.IsSpace(r) || strings.ContainsRune("&*?<>,", r) {
			i++
			continue
		}
		lifetime := r == '\''
		if lifetime {
			i++
			if i == len(runes) {
				return false
			}
		}
		start := i
		for i < len(runes) && (runes[i] == '_' || unicode.IsLetter(runes[i]) || (i > start && unicode.IsDigit(runes[i])) || (!lifetime && (runes[i] == ':' || runes[i] == '.'))) {
			i++
		}
		if start == i {
			return false
		}
		tail := registrationIdentifierPathTail(string(runes[start:i]))
		if tail == "" {
			return false
		}
		matched = matched || (!lifetime && tail == subject)
	}
	return matched
}
