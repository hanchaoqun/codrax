package types

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

type CallableReturnKind string

const (
	CallableReturnExplicit       CallableReturnKind = "explicit_return"
	CallableReturnExpressionBody CallableReturnKind = "expression_body"
	CallableReturnImplicitTail   CallableReturnKind = "implicit_tail"
)

// CallableReturnExpression is a lexical source fact, not a claim that this
// expression executes, has a constant value, or is the only possible return.
// Byte extents are half-open UTF-8 source offsets; line extents are inclusive.
// The enclosing FileInfo owns file identity and source-generation hash.
type CallableReturnExpression struct {
	CallableName     string             `json:"callable_name"`
	CallableReceiver string             `json:"callable_receiver,omitempty"`
	CallableParent   string             `json:"callable_parent,omitempty"`
	CallableLine     int                `json:"callable_line"`
	CallableEndLine  int                `json:"callable_end_line"`
	Kind             CallableReturnKind `json:"kind"`
	Expression       string             `json:"expression"`
	LineStart        int                `json:"line_start"`
	LineEnd          int                `json:"line_end"`
	StartByte        uint32             `json:"start_byte"`
	EndByte          uint32             `json:"end_byte"`
	Provenance       string             `json:"provenance"`
	ResolvedBy       string             `json:"resolved_by"`
}

func (r CallableReturnExpression) IsValid() bool {
	switch r.Kind {
	case CallableReturnExplicit, CallableReturnExpressionBody, CallableReturnImplicitTail:
	default:
		return false
	}
	provenance := (r.Provenance == ProvenanceTreeSitter && r.ResolvedBy == "tree_sitter_callable_return") ||
		(r.Provenance == ProvenanceCangjieParser && r.ResolvedBy == "cangjie_callable_return")
	return provenance && r.CallableName != "" && r.CallableLine > 0 && r.CallableEndLine >= r.CallableLine &&
		r.Expression != "" && r.LineStart >= r.CallableLine && r.LineEnd >= r.LineStart && r.LineEnd <= r.CallableEndLine &&
		r.EndByte > r.StartByte && uint64(r.EndByte-r.StartByte) == uint64(len(r.Expression))
}

func (r CallableReturnExpression) MatchesCallable(s Symbol) bool {
	return r.IsValid() && s.HasParserOwnedBody() && r.CallableName == s.Name && r.CallableReceiver == s.Receiver &&
		r.CallableParent == s.Parent && r.CallableLine == s.Line && r.CallableEndLine == s.EndLine &&
		r.LineStart >= s.BodyStartLine && r.LineEnd <= s.BodyEndLine
}

// CallableReturnReader is an immutable per-file validated snapshot. Build it
// once for a source generation; For only copies the selected owner's receipts.
type CallableReturnReader struct {
	byOwner map[callableReturnOwner][]CallableReturnExpression
}

type callableReturnOwner struct {
	file, name, receiver, parent   string
	start, end, bodyStart, bodyEnd int
}

type callableReturnDeclaration struct {
	name, receiver, parent string
	start, end             int
}

func callableReturnOwnerOf(s Symbol) callableReturnOwner {
	return callableReturnOwner{s.File, s.Name, s.Receiver, s.Parent, s.Line, s.EndLine, s.BodyStartLine, s.BodyEndLine}
}

// NewCallableReturnReader is the shared source-generation and owner check. An
// expression on the same line as a closure or neighboring declaration cannot
// acquire another callable's identity. Consumers still own read-scope and
// relevance policies; receiving this slice never means the source was read.
// Validation is one source hash and one source-line index per file, not per
// callable; there is no process-wide cache or alias of the mutable FileInfo.
func NewCallableReturnReader(f *FileInfo, source []byte) *CallableReturnReader {
	if f == nil || f.Hash == "" || len(source) == 0 {
		return nil
	}
	hash := sha256.Sum256(source)
	if f.Hash != hex.EncodeToString(hash[:8]) {
		return nil
	}
	owners := make(map[callableReturnDeclaration][]Symbol)
	for _, s := range f.Symbols {
		if s.File == f.RelPath && s.HasParserOwnedBody() {
			key := callableReturnDeclaration{s.Name, s.Receiver, s.Parent, s.Line, s.EndLine}
			owners[key] = append(owners[key], s)
		}
	}
	newlines := make([]int, 0)
	for i, b := range source {
		if b == '\n' {
			newlines = append(newlines, i)
		}
	}
	reader := &CallableReturnReader{byOwner: make(map[callableReturnOwner][]CallableReturnExpression)}
	for _, r := range f.CallableReturnExpressions {
		if !r.IsValid() || uint64(r.EndByte) > uint64(len(source)) || string(source[r.StartByte:r.EndByte]) != r.Expression {
			continue
		}
		if sort.SearchInts(newlines, int(r.StartByte))+1 != r.LineStart || sort.SearchInts(newlines, int(r.EndByte)-1)+1 != r.LineEnd {
			continue
		}
		// Body bounds are taken from the exact same declaration, not inferred
		// from the expression's proximity to a name.
		matches := owners[callableReturnDeclaration{r.CallableName, r.CallableReceiver, r.CallableParent, r.CallableLine, r.CallableEndLine}]
		if len(matches) == 1 && r.MatchesCallable(matches[0]) {
			key := callableReturnOwnerOf(matches[0])
			reader.byOwner[key] = append(reader.byOwner[key], r)
		}
	}
	return reader
}

func (r *CallableReturnReader) For(s Symbol) []CallableReturnExpression {
	if r == nil || !s.HasParserOwnedBody() {
		return nil
	}
	return append([]CallableReturnExpression(nil), r.byOwner[callableReturnOwnerOf(s)]...)
}

// CallableReturnsFor is a convenience for a single lookup. Multi-callable
// consumers should construct one NewCallableReturnReader per file instead.
func (f *FileInfo) CallableReturnsFor(s Symbol, source []byte) []CallableReturnExpression {
	return NewCallableReturnReader(f, source).For(s)
}
