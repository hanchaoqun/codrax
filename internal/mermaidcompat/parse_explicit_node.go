package mermaidcompat

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeExplicitNodeLabel encodes authored display text, not node identity.
// Real line endings stay inside one quoted declaration as Mermaid line breaks.
// Existing entities and break tags remain intact, including on an exact replay.
// Empty text is returned unchanged so callers can distinguish an omitted label.
func NormalizeExplicitNodeLabel(label string) (string, error) {
	if !utf8.ValidString(label) {
		return "", fmt.Errorf("invalid visible label: invalid UTF-8")
	}
	for _, r := range label {
		if unicode.IsControl(r) && r != '\r' && r != '\n' && r != '\t' {
			return "", fmt.Errorf("invalid visible label: unsupported control character U+%04X", r)
		}
	}
	label = strings.TrimSpace(label)
	label = strings.NewReplacer("\r\n", "<br/>", "\r", "<br/>", "\n", "<br/>").Replace(label)
	// A model-authored backslash is display data, not a Mermaid escape. Entity
	// encoding prevents trailing slashes from escaping the declaration's quote
	// and preserves literal backslash-n and repeated slashes across all carriers.
	return strings.NewReplacer(`\`, `&#92;`, `"`, `&quot;`).Replace(label), nil
}

// AddExplicitNodeDeclarationChecked is the diagnostic form of
// AddExplicitNodeDeclaration. It changes syntax only; labels cannot introduce
// another declaration or edge, and existing declarations remain immutable.
func AddExplicitNodeDeclarationChecked(body, ident, visibleLabel string) (string, error) {
	ident = strings.TrimSpace(ident)
	family := mermaidBodyFamily(body)
	if family != "flow" && family != "sequence" && family != "class" {
		return body, fmt.Errorf("unsupported Mermaid family for an explicit node declaration")
	}
	safeID := safeStandaloneNodeIdentifier(ident)
	if family == "sequence" {
		safeID = safeExplicitSequenceNodeIdentifier(ident)
	}
	if !safeID {
		return body, fmt.Errorf("invalid endpoint identifier %q for %s declaration", ident, family)
	}
	label, err := NormalizeExplicitNodeLabel(visibleLabel)
	if err != nil {
		return body, err
	}
	if label == "" {
		return body, fmt.Errorf("invalid visible label: empty display text")
	}
	lines := strings.Split(body, "\n")
	header := -1
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if header < 0 && line != "" && !strings.HasPrefix(line, "%%") {
			header = i
		}
		var declarations []NodeDecl
		switch family {
		case "sequence":
			declarations = SequenceParticipantDeclarations(line)
		case "flow":
			declarations = NodeDeclarationsAll(line)
		case "class":
			declarations = classNodeDeclarations(line)
		}
		for _, declaration := range declarations {
			if strings.TrimSpace(declaration.Ident) == ident {
				return body, fmt.Errorf("endpoint %q already has an explicit declaration; it cannot be replaced by an addition", ident)
			}
		}
	}
	var declaration string
	switch family {
	case "sequence":
		declaration = `    participant ` + ident + ` as "` + label + `"`
	case "flow":
		declaration = `    ` + ident + `["` + label + `"]`
	case "class":
		declaration = `    class ` + ident + `["` + label + `"]`
	}
	lines = append(lines, "")
	copy(lines[header+2:], lines[header+1:])
	lines[header+1] = declaration
	return strings.Join(lines, "\n"), nil
}
