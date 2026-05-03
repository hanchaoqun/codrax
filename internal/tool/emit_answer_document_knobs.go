package tool

import "strings"

// Runtime configuration knobs preserved across the B8 V1-carrier
// retirement (block_only_carrier.md §5.8). The Mermaid renderability
// gate + diagram identifier whitelist are orthogonal to V1/V2
// carrier choice; they govern Mermaid block validation regardless
// of which carrier the answer used to live in. Lives here so
// cmd/root.go's yaml resolution call sites stay stable.

// MermaidGateMode is the runtime mode for the mermaid renderability
// gate.
type MermaidGateMode int

const (
	// MermaidGateOff disables the gate entirely. Diagnostics still
	// flow into stats counters but no submission is ever rejected
	// for renderability reasons.
	MermaidGateOff MermaidGateMode = iota
	// MermaidGateSoft is the default: diagnostics counted, gate
	// never rejects. Operators get visibility without retry overhead.
	MermaidGateSoft
	// MermaidGateStrict rejects emit_answer_document submissions
	// when ANY mermaid block has Outcome == OutcomeLibraryRejected.
	// Unsupported-kind / fallback-rune outcomes are NEVER rejected
	// (library-subset gaps, not LLM mistakes).
	MermaidGateStrict
)

var mermaidGateMode = MermaidGateSoft

// SetMermaidGateMode is the runtime accessor used by cmd/root.go
// at startup to apply the yaml override. Defaults to soft.
func SetMermaidGateMode(mode MermaidGateMode) { mermaidGateMode = mode }

// GetMermaidGateMode returns the current mode.
func GetMermaidGateMode() MermaidGateMode { return mermaidGateMode }

// diagramIdentifierWhitelist is the set of identifier-shaped tokens
// the mermaid bare-identifier oracle should NOT flag even when
// SymbolOracle reports they're missing. Operator-tunable via yaml
// `diagram_identifier_whitelist`.
var diagramIdentifierWhitelist = map[string]bool{}

// SetDiagramIdentifierWhitelist replaces the active whitelist.
// Called once from cmd/root.go at startup.
func SetDiagramIdentifierWhitelist(words []string) {
	out := map[string]bool{}
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w != "" {
			out[w] = true
		}
	}
	diagramIdentifierWhitelist = out
}
