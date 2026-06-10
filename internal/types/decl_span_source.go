package types

// DeclSpanSource exposes exported-declaration line positions for one
// repo-relative file, backed by the typed repository graph. It is the
// precise corroboration channel for write-approval API-impact decisions:
// a plan hunk that modifies or deletes a declaration line of an exported
// symbol is typed evidence of public-surface change, while body-only edits
// inside an exported symbol are not.
//
// ok=false means the file is not reliably indexed (unknown to the graph, or
// parsed at a low-confidence fallback tier). Consumers MUST then degrade to
// softer signals — never guess spans, never substitute heuristics into the
// hard gate.
type DeclSpanSource interface {
	ExportedDeclLines(relPath string) (lines []int, ok bool)
}
