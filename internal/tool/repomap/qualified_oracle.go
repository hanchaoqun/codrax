package repomap

// Qualified-name oracle resolution (QNO batch, 2026-07-05).
//
// Closes the A1 anchor-obligation boundary recorded in
// docs/design/arch_stability_batch_plan_20260702.md §A1 遗留记录: the
// flat symbol index keys are FlattenIdentifier(bareName), so a
// package- / receiver-qualified LLM or user spelling ("gate.Run",
// "Gate.Run", "(*Gate).Run") always missed → EntityProvenance
// Resolved=false → the R3b named-subject pin and the anchor
// obligation lane never fired for the exact form users write most.
//
// Red line — 禁 fuzzy: resolution is a DETERMINISTIC decomposition
// (separator grammar only) followed by EXACT hierarchical matching
// against the graph's typed scope levels (Symbol.Receiver,
// Symbol.Parent, FileInfo.Package, defining-directory basename).
// Equality is FlattenIdentifier equality — the same case-form rule
// SymbolExistsFlat already commits to — never similarity, prefix,
// substring, or edit distance. Undecomposable or unverifiable names
// are honest misses.
//
// Precision posture vs the agent-side symbolMatchesQualifier
// (internal/agent/qualified_name.go): that helper is deliberately
// recall-loose (ANY prefix segment may match) because its consumer is
// an evidence filter with its own fallback. This oracle answers a
// bare existence question that provenance / anchor lanes treat as
// PROOF, so it requires EVERY qualifier segment to verify.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Compile-time: the graph oracle implements the optional qualified
// extension alongside the base SymbolOracle contract.
var (
	_ types.SymbolOracle          = (*graphOracle)(nil)
	_ types.QualifiedSymbolOracle = (*graphOracle)(nil)
)

// SplitQualifiedSegments decomposes a possibly-qualified identifier
// into its scope segments by language separator grammar. Purely
// syntactic and deterministic:
//
//	"gate.Run"            → ["gate", "Run"]        (Go / Java / Python / ArkTS / Cangjie …)
//	"mod::Type::method"   → ["mod", "Type", "method"]  (C++ / Rust / Ruby)
//	"Foo::Bar#baz"        → ["Foo", "Bar", "baz"]      (Ruby instance method)
//	"(*Gate).Run"         → ["Gate", "Run"]        (Go method-expression receiver)
//	"(g *Gate).Run"       → ["Gate", "Run"]        (Go receiver declaration)
//	"RunWith"             → ["RunWith"]            (unqualified)
//	""                    → nil
//
// `->` is intentionally NOT a separator: in the C++ pointer-member
// form `obj->method` the graph symbol is the bare method name and the
// left side is a VALUE, not a scope (same ruling as
// expandEntityNameAliases). Case is preserved — callers pick their
// own equality rule.
func SplitQualifiedSegments(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	canon := strings.ReplaceAll(name, "::", ".")
	canon = strings.ReplaceAll(canon, "#", ".")
	raw := strings.Split(canon, ".")
	segments := make([]string, 0, len(raw))
	for _, s := range raw {
		s = normalizeReceiverSegment(strings.TrimSpace(s))
		if s != "" {
			segments = append(segments, s)
		}
	}
	if len(segments) == 0 {
		return nil
	}
	return segments
}

// normalizeReceiverSegment reduces a Go receiver-shaped segment to its
// bare type name by grammar, not guesswork:
//
//	"(*Gate)"   → "Gate"   (pointer method expression)
//	"(Gate)"    → "Gate"   (value method expression)
//	"(g *Gate)" → "Gate"   (receiver declaration: last field is the type)
//	"*Gate"     → "Gate"   (unparenthesised pointer spelling)
//	"Gate"      → "Gate"   (already bare)
//
// Segments that are not receiver-shaped pass through unchanged; a
// segment that reduces to nothing returns "" and is dropped by the
// caller (honest decomposition failure, never a guess).
func normalizeReceiverSegment(seg string) string {
	if strings.HasPrefix(seg, "(") && strings.HasSuffix(seg, ")") && len(seg) >= 2 {
		seg = strings.TrimSpace(seg[1 : len(seg)-1])
		// Receiver declaration form "(g *Gate)": the TYPE is the last
		// whitespace-separated field per Go grammar.
		if fields := strings.Fields(seg); len(fields) > 1 {
			seg = fields[len(fields)-1]
		}
	}
	seg = strings.TrimLeft(seg, "*&")
	return strings.TrimSpace(seg)
}

// QualifiedSymbolExists implements types.QualifiedSymbolOracle. See
// the interface doc for the full contract; short form: trailing
// segment must exist under flat (case-form-aware) equality AND every
// qualifier segment must exactly match one of that definition's typed
// scope levels. Single-segment names are (false, 0) by contract —
// the exact / flat lanes own unqualified spellings.
func (o *graphOracle) QualifiedSymbolExists(name string) (bool, int) {
	if o == nil || o.graph == nil {
		return false, 0
	}
	segments := SplitQualifiedSegments(name)
	if len(segments) < 2 {
		return false, 0
	}
	bare := segments[len(segments)-1]
	flatBare := contract.FlattenIdentifier(bare)
	if flatBare == "" {
		return false, 0
	}
	// Cheap precise short-circuit: if no symbol anywhere flattens to
	// the trailing segment, the qualified form cannot resolve. Reuses
	// the graph-level memoized flat index (already built by the flat
	// lookup every consumer performs first).
	if idx := o.graph.FlatSymbolIndex(o.buildFlatIndex); idx != nil {
		if _, ok := idx[flatBare]; !ok {
			return false, 0
		}
	}
	prefix := segments[:len(segments)-1]
	found := false
	minTier := 0
	for symName, defs := range o.graph.SymbolDefs {
		if contract.FlattenIdentifier(symName) != flatBare {
			continue
		}
		for _, def := range defs {
			if def == nil || !symbolScopeSatisfiesAllQualifiers(o.graph, def, prefix) {
				continue
			}
			tier := defParseTier(o.graph, def)
			found = true
			if minTier == 0 || tier < minTier {
				minTier = tier
			}
		}
	}
	return found, minTier
}

// symbolScopeSatisfiesAllQualifiers reports whether EVERY qualifier
// segment flat-equals one of the definition's typed scope candidates:
//
//   - Symbol.Receiver — Go method receiver type ("Gate.Run").
//   - Symbol.Parent — containing type/class (Java / Kotlin / Python /
//     ArkTS class members; Cangjie type members).
//   - FileInfo.Package — whole package string AND its `.` / `/`
//     segments, so "com.foo.Bar.method" verifies against Package
//     "com.foo" segment-by-segment. (Cangjie red line: Package comes
//     from package_clause upstream — this consumes, never infers.)
//   - Defining-directory basename — deterministic fallback ONLY when
//     the file carries no Package (C / Rust / Swift, or a stale
//     FileIndex row). When a package clause IS recorded it is the
//     authoritative scope; the directory name must not widen it (F3,
//     2026-07-05: Go `pkg/v2/` layouts would otherwise let "v2.Bar"
//     hit a package-`bar` symbol — the honest answer is a miss).
//
// All-segments-must-match is the precision posture: this feeds
// existence PROOF lanes, so one unverifiable qualifier = miss.
func symbolScopeSatisfiesAllQualifiers(g *rmtypes.Graph, def *rmtypes.Symbol, prefix []string) bool {
	candidates := make(map[string]bool, 8)
	addCandidate := func(s string) {
		if flat := contract.FlattenIdentifier(strings.TrimSpace(s)); flat != "" {
			candidates[flat] = true
		}
	}
	addCandidate(def.Receiver)
	addCandidate(def.Parent)
	if g != nil && def.File != "" {
		pkg := ""
		if fi, ok := g.FileIndex[def.File]; ok && fi != nil {
			pkg = strings.TrimSpace(fi.Package)
		}
		if pkg != "" {
			addCandidate(pkg)
			for _, seg := range strings.FieldsFunc(pkg, func(r rune) bool {
				return r == '.' || r == '/'
			}) {
				addCandidate(seg)
			}
		} else {
			addCandidate(DirBasename(def.File))
		}
	}
	if len(candidates) == 0 {
		return false
	}
	for _, seg := range prefix {
		flat := contract.FlattenIdentifier(seg)
		if flat == "" || !candidates[flat] {
			return false
		}
	}
	return true
}

// defParseTier mirrors buildFlatIndex tier semantics: the defining
// file's ParseTier with a floor of 1; a missing FileIndex row is a
// stale index and degrades to the least-reliable tier 4 so tier-
// filtering callers behave conservatively (same as Graph.SymbolExists).
func defParseTier(g *rmtypes.Graph, def *rmtypes.Symbol) int {
	if g != nil {
		if fi, ok := g.FileIndex[def.File]; ok && fi != nil {
			if fi.ParseTier > 0 {
				return fi.ParseTier
			}
			return 1
		}
	}
	return 4
}

// DirBasename returns the last directory segment of a forward-slash
// relative path ("internal/analysis/gate/gate.go" → "gate"); "" for
// bare filenames. Windows separators normalised defensively.
// Exported as the SINGLE implementation (QNO 备注b, 2026-07-05):
// agent-side symbolMatchesQualifier consumes this instead of keeping
// its own copy.
func DirBasename(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	slash := strings.LastIndexByte(p, '/')
	if slash <= 0 {
		return ""
	}
	dir := p[:slash]
	if last := strings.LastIndexByte(dir, '/'); last >= 0 {
		return dir[last+1:]
	}
	return dir
}
