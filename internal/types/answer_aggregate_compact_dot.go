package types

import (
	"errors"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// A compact dot is an owner/member shorthand, not a decimal point. Only this
// implicit syntax needs a named owner: explicit arrows and numeric selectors
// such as pair.0 retain their existing grammar. Keep Unicode, $, _, hyphens and
// digit-prefixed names (7zip.Entry) rather than imposing one language's lexer.
func aggregateCompactDotOwnerOK(part string) bool {
	if !aggregateRelationAtomOK(part) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(part)
	if !unicode.IsDigit(first) && first != '-' && first != '+' {
		return true
	}
	onlyNumeric := true
	for _, r := range part {
		if !unicode.IsDigit(r) && r != '_' && r != '-' && r != '+' {
			onlyNumeric = false
			break
		}
	}
	if onlyNumeric {
		return false
	}
	if _, err := strconv.ParseInt(part, 0, 64); err == nil || errors.Is(err, strconv.ErrRange) {
		return false
	}
	_, err := strconv.ParseFloat(part, 64)
	return err != nil && !errors.Is(err, strconv.ErrRange)
}
