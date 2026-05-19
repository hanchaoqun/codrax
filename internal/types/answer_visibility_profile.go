package types

// AnswerSymbolVisibility is the analyzer-emitted symbol visibility scope for
// source-code inventory questions. It is separate from SourceScopeProfile:
// SourceScope answers which paths are principal evidence; this enum answers
// whether private/internal symbols may be principal answer members.
type AnswerSymbolVisibility string

const (
	AnswerSymbolVisibilityUnknown        AnswerSymbolVisibility = "unknown"
	AnswerSymbolVisibilityPublicExported AnswerSymbolVisibility = "public_exported"
	AnswerSymbolVisibilityAll            AnswerSymbolVisibility = "all"
	AnswerSymbolVisibilityPrivateOnly    AnswerSymbolVisibility = "private_only"
)

func AllAnswerSymbolVisibilities() []AnswerSymbolVisibility {
	return []AnswerSymbolVisibility{
		AnswerSymbolVisibilityUnknown,
		AnswerSymbolVisibilityPublicExported,
		AnswerSymbolVisibilityAll,
		AnswerSymbolVisibilityPrivateOnly,
	}
}

func (v AnswerSymbolVisibility) IsValid() bool {
	for _, declared := range AllAnswerSymbolVisibilities() {
		if v == declared {
			return true
		}
	}
	return false
}

func (v AnswerSymbolVisibility) ExcludesPrivateSymbols() bool {
	return v == AnswerSymbolVisibilityPublicExported
}

// AnswerVisibilityProfile is the typed lane for answer-member symbol
// visibility. Downstream uses this enum plus the repo graph's Exported bit; it
// must not infer visibility scope from raw request text or rendered prose.
type AnswerVisibilityProfile struct {
	SymbolVisibility AnswerSymbolVisibility `json:"symbol_visibility"`
	SourceQuotes     []string               `json:"source_quotes,omitempty"`
	Confidence       float64                `json:"confidence,omitempty"`
	Rationale        string                 `json:"rationale,omitempty"`
}

func (p *AnswerVisibilityProfile) Active() bool {
	return p != nil &&
		p.SymbolVisibility != "" &&
		p.SymbolVisibility != AnswerSymbolVisibilityUnknown
}

func (p *AnswerVisibilityProfile) ExcludesPrivateSymbols() bool {
	return p != nil && p.SymbolVisibility.ExcludesPrivateSymbols()
}
