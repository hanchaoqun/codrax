package types

import "strings"

// AssignmentEvidenceEndpoints returns the exact assignment receiver and the
// primary assigned value surface from the grounded source-line snippet.
//
// Grounding an assignment-shaped line is not enough to authorize a directed
// relation: model-authored Subject/Object may still name the enclosing
// function, a nearby participant, or another token on the line. This parser is
// deliberately conservative and language-neutral. It accepts only one simple
// assignment/initializer surface and refuses destructuring, chained
// assignments, binary/ternary RHS expressions, and literals. Unsupported
// syntax remains valid ordinary evidence, but cannot become hard relation
// authority.
func AssignmentEvidenceEndpoints(item EvidenceItem) (receiver, value string, ok bool) {
	if item.AnchorKind != AnchorAssignment && item.AnchorKind != AnchorInitializer {
		return "", "", false
	}
	line := firstAssignmentEvidenceLine(item.Snippet)
	if line == "" {
		return "", "", false
	}
	line = stripAssignmentLineComment(line)
	if line == "" {
		return "", "", false
	}

	op, width, count := findSimpleAssignmentOperator(line)
	if count != 1 && item.AnchorKind == AnchorInitializer {
		op, width, count = findSimpleInitializerColon(line)
	}
	if count != 1 || op <= 0 || op+width >= len(line) {
		return "", "", false
	}
	lhs := strings.TrimSpace(line[:op])
	rhs := strings.TrimSpace(line[op+width:])
	receiver, ok = assignmentReceiverSurface(lhs)
	if !ok {
		return "", "", false
	}
	value, ok = assignmentPrimaryValueSurface(rhs)
	if !ok {
		return "", "", false
	}
	return receiver, value, true
}

// AssignmentEvidenceEndpointsMatch reports whether the model-authored
// Subject/Object are the assignment's real LHS/RHS identities. A short or
// locally de-qualified spelling may identify the exact line-local endpoint;
// unrelated enclosing functions and nearby participants cannot.
func AssignmentEvidenceEndpointsMatch(item EvidenceItem) bool {
	receiver, value, ok := AssignmentEvidenceEndpoints(item)
	if !ok {
		return false
	}
	return assignmentEvidenceEndpointCompatible(item.Subject, receiver) &&
		assignmentEvidenceEndpointCompatible(item.Object, value)
}

func firstAssignmentEvidenceLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if line, _, found := strings.Cut(raw, "\n"); found {
		return strings.TrimSpace(line)
	}
	return raw
}

func stripAssignmentLineComment(line string) string {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return strings.TrimSpace(line[:i])
			}
		case '#':
			if i == 0 || assignmentASCIIWhitespace(line[i-1]) {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return strings.TrimSpace(line)
}

func findSimpleAssignmentOperator(line string) (index, width, count int) {
	quote := byte(0)
	escaped := false
	depth := 0
	index = -1
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
			continue
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			continue
		}
		if ch != '=' {
			continue
		}
		prev, next := byte(0), byte(0)
		if i > 0 {
			prev = line[i-1]
		}
		if i+1 < len(line) {
			next = line[i+1]
		}
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' || prev == '-' ||
			next == '=' || next == '>' {
			continue
		}
		candidateWidth := 1
		candidateIndex := i
		if prev == ':' {
			candidateWidth = 2
			candidateIndex = i - 1
		} else if prev == '+' || prev == '-' || prev == '*' || prev == '/' || prev == '%' ||
			prev == '&' || prev == '|' || prev == '^' {
			// Compound update is not a simple source-to-receiver transfer.
			return -1, 0, 2
		}
		if depth != 0 {
			continue
		}
		index, width = candidateIndex, candidateWidth
		count++
	}
	return index, width, count
}

func findSimpleInitializerColon(line string) (index, width, count int) {
	quote := byte(0)
	escaped := false
	depth := 0
	index = -1
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ':':
			prev, next := byte(0), byte(0)
			if i > 0 {
				prev = line[i-1]
			}
			if i+1 < len(line) {
				next = line[i+1]
			}
			if depth == 0 && prev != ':' && next != ':' && next != '=' {
				index, width = i, 1
				count++
			}
		}
	}
	return index, width, count
}

func assignmentReceiverSurface(lhs string) (string, bool) {
	lhs = strings.TrimSpace(strings.Trim(lhs, "{}();"))
	if lhs == "" || assignmentHasTopLevelComma(lhs) || strings.ContainsAny(lhs, "[]") {
		return "", false
	}
	// Type annotations live after the declared receiver in TS/ArkTS and
	// similar languages. Namespace separators are preserved.
	for i := 0; i < len(lhs); i++ {
		if lhs[i] != ':' || (i > 0 && lhs[i-1] == ':') || (i+1 < len(lhs) && lhs[i+1] == ':') {
			continue
		}
		lhs = strings.TrimSpace(lhs[:i])
		break
	}
	if lhs == "" {
		return "", false
	}
	if keyword, rest := assignmentLeadingDeclarationKeyword(lhs); keyword != "" {
		if surface := assignmentLeadingIdentity(rest); surface != "" {
			return surface, true
		}
		return "", false
	}
	surface := assignmentTrailingIdentity(lhs)
	return surface, surface != ""
}

func assignmentPrimaryValueSurface(rhs string) (string, bool) {
	rhs = strings.TrimSpace(strings.TrimRight(rhs, ",;}"))
	if rhs == "" || assignmentHasTopLevelComma(rhs) || assignmentHasTopLevelValueOperator(rhs) {
		return "", false
	}
	for {
		trimmed := strings.TrimLeft(rhs, "&*!+-~ ")
		changed := trimmed != rhs
		rhs = trimmed
		for _, keyword := range []string{"await", "new", "try", "return", "move"} {
			prefix := keyword + " "
			if strings.HasPrefix(rhs, prefix) {
				rhs = strings.TrimSpace(strings.TrimPrefix(rhs, prefix))
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	if rhs == "" || rhs[0] >= '0' && rhs[0] <= '9' || strings.ContainsRune("'\"`[{", rune(rhs[0])) {
		// Numeric/string/collection literals are useful scalar facts, not a
		// code-identity source endpoint for a directed data-flow relation.
		return "", false
	}
	surface := assignmentLeadingIdentity(rhs)
	if surface == "" {
		return "", false
	}
	switch strings.ToLower(surface) {
	case "true", "false", "nil", "null", "none", "undefined":
		return "", false
	}
	return surface, true
}

func assignmentLeadingDeclarationKeyword(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	for _, keyword := range []string{"const", "let", "var", "val", "auto", "final"} {
		prefix := keyword + " "
		if strings.HasPrefix(trimmed, prefix) {
			return keyword, strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return "", trimmed
}

func assignmentLeadingIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	start := 0
	for start < len(raw) && !assignmentIdentityByte(raw[start]) {
		start++
	}
	end := start
	for end < len(raw) && assignmentIdentityByte(raw[end]) {
		end++
	}
	if start == end {
		return ""
	}
	candidate := strings.Trim(raw[start:end], ".:-?>")
	if !assignmentParsedIdentityValid(candidate) {
		return ""
	}
	return candidate
}

func assignmentTrailingIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	end := len(raw)
	for end > 0 && !assignmentIdentityByte(raw[end-1]) {
		end--
	}
	start := end
	for start > 0 && assignmentIdentityByte(raw[start-1]) {
		start--
	}
	if start == end {
		return ""
	}
	candidate := strings.Trim(raw[start:end], ".:-?>")
	if !assignmentParsedIdentityValid(candidate) {
		return ""
	}
	return candidate
}

func assignmentIdentityByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' ||
		ch == '_' || ch == '$' || ch == '.' || ch == ':' || ch == '-' || ch == '>' || ch == '#'
}

func assignmentParsedIdentityValid(candidate string) bool {
	normalized := strings.NewReplacer("::", ".", "->", ".", "#", ".").Replace(candidate)
	return IsCodeIdentitySurface(normalized)
}

func assignmentHasTopLevelComma(raw string) bool {
	quote := byte(0)
	escaped := false
	depth := 0
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func assignmentHasTopLevelValueOperator(raw string) bool {
	quote := byte(0)
	escaped := false
	depth := 0
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '?', '|', '&', '+', '*', '/', '%', '<', '>', '=':
			if depth == 0 {
				return true
			}
		case '-':
			// Keep C/C++ member access as one identity surface; every other
			// top-level minus is an expression, not one primary value endpoint.
			if depth == 0 && (i+1 >= len(raw) || raw[i+1] != '>') {
				return true
			}
		}
	}
	return false
}

func assignmentEvidenceEndpointCompatible(model, exact string) bool {
	model = strings.TrimSpace(model)
	exact = strings.TrimSpace(exact)
	if model == "" || exact == "" {
		return false
	}
	if AnswerCodeIdentitySurfacesCompatible(model, exact) {
		return true
	}
	normalize := func(raw string) string {
		raw = strings.ToLower(strings.Trim(strings.TrimSpace(raw), "`'\""))
		raw = strings.NewReplacer("::", ".", "->", ".", "#", ".", "/", ".", `\`, ".").Replace(raw)
		return strings.Trim(raw, ".")
	}
	model = normalize(model)
	exact = normalize(exact)
	return model != "" && exact != "" && strings.HasSuffix(exact, "."+model)
}

func assignmentASCIIWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}
