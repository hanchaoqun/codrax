package tool

import "strings"

const maxAnswerBlockArrayObjectOpenerRepairs = 8

// insertMissingArrayObjectOpeners repairs a bounded structural slip where an
// object-valued array element starts directly with `"key":` instead of
// `{"key":`. It uses the JSON container stack rather than field names: prose
// and enum values cannot trigger it, and object keys inside an existing object
// are left untouched. The caller accepts the candidate only when the complete
// blocks array decodes into block-shaped objects; normal schema, authority and
// evidence validation still run afterwards.
func insertMissingArrayObjectOpeners(s string) (string, bool) {
	if strings.TrimSpace(s) == "" {
		return s, false
	}
	var b strings.Builder
	b.Grow(len(s) + maxAnswerBlockArrayObjectOpenerRepairs)
	stack := make([]byte, 0, 8)
	inString := false
	escape := false
	repairs := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString {
			b.WriteByte(ch)
			if escape {
				escape = false
				continue
			}
			switch ch {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		if ch == '"' {
			if len(stack) != 0 && stack[len(stack)-1] == '[' && jsonArrayElementStartsAt(s, i) {
				if _, key := quotedObjectKeyAt(s, i); key {
					repairs++
					if repairs > maxAnswerBlockArrayObjectOpenerRepairs {
						return s, false
					}
					b.WriteByte('{')
					stack = append(stack, '{')
				}
			}
			inString = true
			b.WriteByte(ch)
			continue
		}
		switch ch {
		case '[', '{':
			stack = append(stack, ch)
		case ']':
			if len(stack) != 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		case '}':
			if len(stack) != 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		}
		b.WriteByte(ch)
	}
	if repairs == 0 {
		return s, false
	}
	return b.String(), true
}

func jsonArrayElementStartsAt(s string, at int) bool {
	for i := at - 1; i >= 0; i-- {
		if isJSONSpaceByte(s[i]) {
			continue
		}
		return s[i] == '[' || s[i] == ','
	}
	return false
}
