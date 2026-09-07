package types

// CallableBodyPresence separates a concrete implementation from a declaration
// and from parsers that cannot establish either. Only syntax extractors mint
// this field; names, model prose, evidence kinds and line counts cannot do so.
type CallableBodyPresence string

const (
	CallableBodyUnknown CallableBodyPresence = ""
	CallableBodyPresent CallableBodyPresence = "present"
	CallableBodyAbsent  CallableBodyPresence = "absent"
)

// HasParserOwnedBody is only a body-existence prerequisite. It does not grant
// read coverage, a call edge, an interface-to-implementation binding, or any
// execution/behavior authority. Unknown future values fail open to inspection,
// not to a hard obligation to produce a supposedly known implementation.
func (s *Symbol) HasParserOwnedBody() bool {
	return s != nil && s.BodyPresence == CallableBodyPresent &&
		s.Line > 0 && s.EndLine >= s.Line && s.BodyStartLine >= s.Line &&
		s.BodyEndLine >= s.BodyStartLine && s.BodyEndLine <= s.EndLine
}
