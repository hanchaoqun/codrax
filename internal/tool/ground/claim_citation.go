package ground

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ClaimCitationCandidate is a concrete same-file anchor that better
// matches a claim's load-bearing code identifier than the cited line
// did. It is intentionally small and renderer-neutral so tool layers
// can surface it in retry hints without importing repomap structs.
type ClaimCitationCandidate struct {
	File string
	Line int
	Name string
}

// ClaimCitationReport validates whether a file:line citation is both
// structurally grounded and semantically aligned with a short claim.
//
// Valid mirrors GroundCitation.Valid: false means the file:line itself
// is not grounded and callers should reject/drop it. Corroborated is
// true when either the claim contains no code-shaped identifiers, the
// cited line/window contains one of them, the cited line falls inside
// a matching symbol region, or no better same-file anchor is known.
// Corroborated=false is reserved for the actionable drift case: the
// cited line does not bear the claim identifiers and this context can
// name better candidate anchors.
type ClaimCitationReport struct {
	Valid        bool
	Corroborated bool
	Tier         types.GroundingTier
	Reason       string
	Candidates   []ClaimCitationCandidate
}

// GroundClaimCitation is the reusable claim-level guard for tools that
// receive a free-form rationale plus a file:line citation. It builds on
// GroundCitation for path/line existence, then uses the existing read_file
// line index and repomap graph to catch line drift where the claim names
// `Foo` but the cited window is around unrelated `Bar` while `Foo` is
// visible elsewhere in the same file.
//
// Claim-level citations are stricter than render-only citations: when
// read_file history exists, the cited source and complete line/range
// must be visible in that gutter. GroundCitation's graph fallback remains
// useful for final rendering, but it is too permissive for a tool that is
// asking the model to justify a fresh judgement from what it actually saw.
//
// The rule is deliberately conservative: absence of a better same-file
// candidate does not make the citation fail. That keeps body-line cites
// and prose-heavy rationales working while still rejecting repairable
// off-by-symbol anchors with concrete retry hints.
func GroundClaimCitation(c types.Citation, claim string, gc *Context) ClaimCitationReport {
	file := strings.TrimSpace(c.File)
	if gc != nil {
		file = canonicalContextPath(gc, file)
	}
	start, end := c.Line, c.LineEnd
	if end < start {
		end = start
	}
	if gc != nil && len(gc.LineIndex) > 0 {
		fileLines, ok := gc.LineIndex[file]
		if !ok {
			return ClaimCitationReport{
				Valid:        false,
				Corroborated: false,
				Reason:       fmt.Sprintf("source %q is not present in read_file history; cite a visible read_file gutter line", file),
			}
		}
		if !claimCitationRangeFullyRead(fileLines, start, end) {
			return ClaimCitationReport{
				Valid:        false,
				Corroborated: false,
				Reason: fmt.Sprintf("citation range %s:%d-%d is not present in read_file history; cite a visible read_file gutter line%s",
					file, start, end, formatClaimReadRangesHint(fileLines)),
			}
		}
	}
	base := GroundCitation(c, gc)
	if !base.Valid {
		return ClaimCitationReport{
			Valid:        false,
			Corroborated: false,
			Tier:         base.Tier,
			Reason:       base.Reason,
		}
	}
	if gc == nil || (len(gc.LineIndex) == 0 && gc.Graph == nil) {
		return ClaimCitationReport{Valid: true, Corroborated: true, Tier: base.Tier}
	}
	ids := claimCitationIdentifiers(claim, gc.Graph)
	if len(ids) == 0 {
		return ClaimCitationReport{Valid: true, Corroborated: true, Tier: base.Tier}
	}
	if claimCitationWindowContainsIdentifier(gc, file, start, end, ids) ||
		claimCitationGraphSymbolMatches(gc, file, start, end, ids) {
		return ClaimCitationReport{Valid: true, Corroborated: true, Tier: base.Tier}
	}
	candidates := claimCitationCandidates(gc, file, start, end, ids)
	if len(candidates) == 0 {
		return ClaimCitationReport{Valid: true, Corroborated: true, Tier: base.Tier}
	}
	return ClaimCitationReport{
		Valid:        true,
		Corroborated: false,
		Tier:         base.Tier,
		Reason:       "cited line/window does not contain the claim's code identifiers",
		Candidates:   candidates,
	}
}

// FormatClaimCitationCandidates renders up to the already-ranked
// candidates as `file:line (name)` entries for tool retry summaries.
func FormatClaimCitationCandidates(candidates []ClaimCitationCandidate) string {
	if len(candidates) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(candidates))
	for _, c := range candidates {
		label := fmt.Sprintf("%s:%d", c.File, c.Line)
		if strings.TrimSpace(c.Name) != "" {
			label += " (" + c.Name + ")"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, ", ")
}

func claimCitationIdentifiers(claim string, graph *repomap.Graph) []string {
	raw := extractCodeIdentifiers(claim, graph)
	if len(raw) == 0 {
		return nil
	}
	var out []string
	for _, tok := range raw {
		if !isStrongClaimCitationIdentifier(tok, graph) {
			continue
		}
		out = append(out, tok)
	}
	return out
}

func isStrongClaimCitationIdentifier(tok string, graph *repomap.Graph) bool {
	if len(tok) < 3 {
		return false
	}
	lower := strings.ToLower(tok)
	switch lower {
	case "the", "and", "for", "with", "from", "into", "that", "this", "then", "than",
		"true", "false", "nil", "null", "none", "return", "returns", "returned",
		"contains", "contain", "includes", "include", "including", "line", "lines",
		"file", "files", "function", "method", "type", "struct", "const", "agent", "agents":
		return false
	}
	hasUpper, hasLower, hasDigit, hasUnderscore, interiorUpper := false, false, false, false, false
	for i, r := range tok {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
			if i > 0 {
				interiorUpper = true
			}
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '_':
			hasUnderscore = true
		}
	}
	if graph != nil {
		if defs, ok := graph.SymbolDefs[tok]; ok && len(defs) > 0 {
			return true
		}
	}
	return hasUnderscore || hasDigit || interiorUpper || (hasUpper && !hasLower)
}

func claimCitationWindowContainsIdentifier(gc *Context, file string, start, end int, ids []string) bool {
	if gc == nil || len(ids) == 0 {
		return false
	}
	for line := start; line <= end; line++ {
		for _, id := range ids {
			if _, ok := VerifyLineAnchor(gc, file, line, id, 2); ok {
				return true
			}
		}
	}
	return false
}

func claimCitationGraphSymbolMatches(gc *Context, file string, start, end int, ids []string) bool {
	if gc == nil || gc.Graph == nil || len(ids) == 0 {
		return false
	}
	for _, sym := range gc.Graph.SymbolsInFile(file) {
		if !claimIdentifierMatches(ids, sym.Name) {
			continue
		}
		symEnd := sym.EndLine
		if symEnd < sym.Line {
			symEnd = sym.Line
		}
		if claimLineRangesOverlap(start, end, sym.Line, symEnd) {
			return true
		}
	}
	return false
}

func claimCitationCandidates(gc *Context, file string, citedStart, citedEnd int, ids []string) []ClaimCitationCandidate {
	if gc == nil || file == "" || len(ids) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []ClaimCitationCandidate
	add := func(line int, name string) {
		if line <= 0 || (line >= citedStart-2 && line <= citedEnd+2) {
			return
		}
		key := fmt.Sprintf("%s:%d:%s", file, line, name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, ClaimCitationCandidate{File: file, Line: line, Name: name})
	}
	if gc.Graph != nil {
		for _, sym := range gc.Graph.SymbolsInFile(file) {
			if claimIdentifierMatches(ids, sym.Name) {
				add(sym.Line, sym.Name)
			}
		}
	}
	if fileLines, ok := gc.LineIndex[file]; ok {
		lines := make([]int, 0, len(fileLines))
		for line := range fileLines {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		for _, line := range lines {
			for _, id := range ids {
				if matched, ok := findNonCommentAnchorLine(fileLines, line, 0, id, file); ok && matched == line {
					add(line, id)
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		di := claimDistanceToRange(out[i].Line, citedStart, citedEnd)
		dj := claimDistanceToRange(out[j].Line, citedStart, citedEnd)
		if di != dj {
			return di < dj
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func claimIdentifierMatches(ids []string, name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	for _, id := range ids {
		if id == name || strings.EqualFold(id, name) {
			return true
		}
	}
	return false
}

func claimCitationRangeFullyRead(fileLines map[int]string, start, end int) bool {
	if len(fileLines) == 0 || start <= 0 || end <= 0 {
		return false
	}
	for line := start; line <= end; line++ {
		if _, ok := fileLines[line]; !ok {
			return false
		}
	}
	return true
}

func formatClaimReadRangesHint(fileLines map[int]string) string {
	if len(fileLines) == 0 {
		return ""
	}
	lines := make([]int, 0, len(fileLines))
	for line := range fileLines {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	var ranges []string
	start, prev := lines[0], lines[0]
	flush := func() {
		if start == prev {
			ranges = append(ranges, strconv.Itoa(start))
		} else {
			ranges = append(ranges, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for _, line := range lines[1:] {
		if line == prev+1 {
			prev = line
			continue
		}
		flush()
		start, prev = line, line
	}
	flush()
	if len(ranges) > 4 {
		ranges = append(ranges[:4], "...")
	}
	return " (visible ranges: " + strings.Join(ranges, ", ") + ")"
}

func claimLineRangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	if aEnd < aStart {
		aEnd = aStart
	}
	if bEnd < bStart {
		bEnd = bStart
	}
	return aStart <= bEnd && bStart <= aEnd
}

func claimDistanceToRange(line, start, end int) int {
	if line < start {
		return start - line
	}
	if line > end {
		return line - end
	}
	return 0
}
