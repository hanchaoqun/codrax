package index

import (
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractCangjie produces a parsed FileInfo for a Cangjie 1.0.0 LTS
// (cjnative) source file. Tier 1 uses a hand-written tokeniser +
// recursive-descent parser (cangjie_lexer.go + cangjie_parser.go)
// that tracks brace depth and enclosing-type stack, so class / struct
// methods are emitted with Kind=method and Parent set to the
// enclosing type. Tier 2 falls back to the comment-aware regex scan
// kept in this file as a safety net for malformed sources.
//
// Red line L-Cangjie-2: FileInfo.Package MUST come from the
// package_clause (first non-comment / non-blank statement). Path
// inference is explicitly disallowed because Cangjie packages have
// their own namespace divorced from filesystem layout.
//
// Red line L-Cangjie-1: callers must not feed .cjo compiled
// artefacts here — scanner.go excludes them at discovery time.
func extractCangjie(src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation, tier int) {
	// Phase 1: tokeniser + recursive-descent parser (D5/M1 upgrade).
	pkg, syms, imps, rels = parseCangjie(src, file)
	if len(syms) > 0 || pkg != "" {
		tier = 1
		return
	}

	// Phase 2: legacy regex salvage — covers pathological sources
	// where the tokeniser bailed out (nested decorator sequences
	// that confuse the state machine, or truncated / partial files).
	cleaned := stripCangjieCommentsAndStrings(string(src))
	pkg = findCangjiePackage(cleaned)
	syms = scanCangjieDecls(cleaned, src, file, pkg)
	imps = scanCangjieImports(cleaned, file)
	rels = scanCangjieExtendRelations(cleaned, file)

	if len(syms) > 0 || pkg != "" {
		tier = 2
		return
	}

	// Phase 3: raw-regex last-ditch salvage (no comment stripping).
	salvageSyms, salvageImps, salvagePkg := cangjieRegexSalvage(src, file)
	if len(salvageSyms) == 0 && len(salvageImps) == 0 && salvagePkg == "" {
		tier = 3
		return
	}
	pkg = salvagePkg
	syms = salvageSyms
	imps = salvageImps
	tier = 2
	return
}

// stripCangjieCommentsAndStrings replaces comments and string
// literal contents with spaces of equal length so byte offsets,
// line numbers, and brace depth remain accurate while keywords
// inside comments/strings cannot false-match downstream regex.
//
// Handles:
//   - `//` to end-of-line
//   - `/* ... */` possibly multi-line
//   - `"..."` double-quoted strings with `\` escapes
//   - `'\”` single-char strings (Cangjie Rune literals)
//   - interpolated `"${...}"` — we blank the contents including
//     any nested braces because tracking interpolation is expensive
//     and the post-pass only cares about brace depth at top level
func stripCangjieCommentsAndStrings(s string) string {
	b := []byte(s)
	out := make([]byte, len(b))
	copy(out, b)

	i := 0
	n := len(b)
	for i < n {
		c := b[i]

		// Line comment
		if c == '/' && i+1 < n && b[i+1] == '/' {
			j := i
			for j < n && b[j] != '\n' {
				if b[j] != '\n' {
					out[j] = ' '
				}
				j++
			}
			i = j
			continue
		}
		// Block comment
		if c == '/' && i+1 < n && b[i+1] == '*' {
			out[i] = ' '
			out[i+1] = ' '
			j := i + 2
			for j < n {
				if b[j] == '*' && j+1 < n && b[j+1] == '/' {
					out[j] = ' '
					out[j+1] = ' '
					j += 2
					break
				}
				if b[j] != '\n' {
					out[j] = ' '
				}
				j++
			}
			i = j
			continue
		}
		// String literal
		if c == '"' {
			j := i + 1
			for j < n && b[j] != '"' {
				if b[j] == '\\' && j+1 < n {
					out[j] = ' '
					out[j+1] = ' '
					j += 2
					continue
				}
				if b[j] != '\n' {
					out[j] = ' '
				}
				j++
			}
			if j < n {
				j++
			}
			i = j
			continue
		}
		// Rune literal (Cangjie uses single-quote for Rune)
		if c == '\'' {
			j := i + 1
			for j < n && b[j] != '\'' {
				if b[j] == '\\' && j+1 < n {
					out[j] = ' '
					out[j+1] = ' '
					j += 2
					continue
				}
				if b[j] != '\n' {
					out[j] = ' '
				}
				j++
			}
			if j < n {
				j++
			}
			i = j
			continue
		}

		i++
	}
	return string(out)
}

// cangjiePackageRegex captures the package name from
// `package x.y.z` at the top of a Cangjie source file. Must appear
// on its own line to be recognised — this matches the Cangjie spec
// which makes package declarations a top-level statement.
var cangjiePackageRegex = regexp.MustCompile(
	`(?m)^[ \t]*package\s+([\w.]+)\s*$`)

// findCangjiePackage returns the package name declared at the top
// of the file, or "" if no package_clause is present. The result
// is written into FileInfo.Package.
func findCangjiePackage(cleaned string) string {
	m := cangjiePackageRegex.FindStringSubmatch(cleaned)
	if m == nil {
		return ""
	}
	return m[1]
}

// cangjieImportRegex captures `import x.y.z` or `import x.y.{a, b}`
// or `import x.y as z`. Group 1 is the first token after `import`.
var cangjieImportRegex = regexp.MustCompile(
	`(?m)^[ \t]*import\s+([\w.]+)(?:\s*\.\s*\{[^}]*\})?(?:\s+as\s+(\w+))?`)

// scanCangjieImports extracts import declarations from the cleaned
// (comment-stripped) source. Imports are always top-level so we do
// not need brace-depth tracking.
func scanCangjieImports(cleaned, file string) []types.Import {
	var out []types.Import
	for _, m := range cangjieImportRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		path := cleaned[m[2]:m[3]]
		var alias string
		if m[4] >= 0 {
			alias = cleaned[m[4]:m[5]]
		}
		line := byteOffsetToLine(cleaned, m[0])
		out = append(out, types.Import{
			Raw:   strings.TrimSpace(cleaned[m[0]:m[1]]),
			Path:  path,
			Alias: alias,
			File:  file,
			Line:  line,
		})
	}
	return out
}

// Cangjie modifier token set. Order matters for the declaration
// regex — we anchor with `(?:<modifier>\s+)*` so any subset in any
// order matches. Duplication is fine (Cangjie rejects duplicates at
// semantic layer; the scanner does not).
var cangjieModifierTokens = []string{
	"public", "private", "protected", "internal",
	"open", "static", "operator", "sealed", "abstract",
	"foreign", "override", "redef", "mut", "const", "unsafe",
}

// cangjieModifierGroup is a regex sub-expression matching any
// modifier token optionally repeated. Built once at init time
// from cangjieModifierTokens.
var cangjieModifierGroup = "(?:(?:" + strings.Join(cangjieModifierTokens, "|") + ")\\s+)*"

// cangjieFuncDeclRegex matches `[modifiers] func <name>(`.
//
// Groups: 1 = combined modifier prefix, 2 = function name.
var cangjieFuncDeclRegex = regexp.MustCompile(
	`(?m)^[ \t]*(` + cangjieModifierGroup + `)func\s+(\w+)\s*(?:<[^>]*>)?\s*\(`)

// cangjieMainEntryRegex matches the Cangjie entrypoint shorthand
// where the `func` keyword is omitted: `main(): Int64 {` or
// `main() {`. Cangjie 1.0.0 LTS treats `main` specially so that the
// typical program entry reads without modifier or keyword noise.
// Capture: group 1 = (empty placeholder), group 2 = literal "main".
var cangjieMainEntryRegex = regexp.MustCompile(
	`(?m)^[ \t]*()(main)\s*\(\s*\)\s*(?::\s*\w+)?\s*\{`)

// cangjieInitRegex matches a Cangjie `init` constructor inside a
// class or struct body. Syntax: `[public|private|...] init(params)`
// with no `func` keyword. Captured: group 1 = modifier prefix,
// group 2 = literal "init".
//
// We only match lines that START with the modifier set (or with
// `init` directly after optional whitespace) so that a helper
// function named `initialise` or a variable named `initialized`
// doesn't false-match.
var cangjieInitRegex = regexp.MustCompile(
	`(?m)^[ \t]*(` + cangjieModifierGroup + `)(init)\s*\(`)

// cangjieOperatorFuncRegex matches `[modifiers] operator func <op>(`.
//
// Group 1 = modifiers prefix; group 2 = operator token (e.g. `+`,
// `>>`, `==`). The operator token may be multi-character so we use
// a non-greedy `[^ \t()]+` capture.
var cangjieOperatorFuncRegex = regexp.MustCompile(
	`(?m)^[ \t]*(` + cangjieModifierGroup + `)operator\s+func\s+([^\s()]+)\s*\(`)

// cangjieForeignFuncRegex matches `foreign func <name>(`. Unlike
// plain func, `foreign` is not interchangeable with other modifier
// positions — it comes first and marks FFI declarations.
var cangjieForeignFuncRegex = regexp.MustCompile(
	`(?m)^[ \t]*foreign\s+func\s+(\w+)\s*(?:<[^>]*>)?\s*\(`)

// cangjieTypeDeclRegex matches class / struct / interface / enum.
//
// Groups: 1 = modifier prefix, 2 = keyword, 3 = type name.
var cangjieTypeDeclRegex = regexp.MustCompile(
	`(?m)^[ \t]*(` + cangjieModifierGroup + `)(class|struct|interface|enum)\s+(\w+)\b`)

// cangjieExtendRegex matches `extend <type>` (with optional
// generic params). Captures the target type name; the extend block
// body is not parsed in detail — just anchored.
var cangjieExtendRegex = regexp.MustCompile(
	`(?m)^[ \t]*extend\s+(\w+)(?:<[^>]*>)?`)

// scanCangjieDecls walks the cleaned source and extracts
// top-level declarations into Symbol records. `cleaned` is the
// comment/string-blanked source; `orig` is the raw bytes for doc
// extraction (not currently used but kept for future doc-capture).
func scanCangjieDecls(cleaned string, _ []byte, file, pkg string) []types.Symbol {
	var syms []types.Symbol

	// 1) foreign func
	for _, m := range cangjieForeignFuncRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		name := cleaned[m[2]:m[3]]
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     "foreign-func",
			File:     file,
			Line:     byteOffsetToLine(cleaned, m[0]),
			Exported: true, // foreign decls are effectively public
			Doc:      "foreign",
		})
	}

	// 2) operator func
	for _, m := range cangjieOperatorFuncRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		modPrefix := cleaned[m[2]:m[3]]
		op := cleaned[m[4]:m[5]]
		syms = append(syms, types.Symbol{
			Name:     "operator " + op,
			Kind:     "operator",
			File:     file,
			Line:     byteOffsetToLine(cleaned, m[0]),
			Exported: hasCangjieExportModifier(modPrefix),
			Doc:      strings.TrimSpace(modPrefix) + "operator func",
		})
	}

	// 3) regular func (skip duplicates of operator/foreign covered above)
	seenLines := map[int]bool{}
	for _, s := range syms {
		seenLines[s.Line] = true
	}
	for _, m := range cangjieFuncDeclRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		line := byteOffsetToLine(cleaned, m[0])
		if seenLines[line] {
			continue
		}
		modPrefix := cleaned[m[2]:m[3]]
		name := cleaned[m[4]:m[5]]
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     "function",
			File:     file,
			Line:     line,
			Exported: hasCangjieExportModifier(modPrefix),
			Doc:      cangjieModifierList(modPrefix),
		})
		seenLines[line] = true
	}

	// 3b) Cangjie `main` entry shorthand (no `func` keyword).
	for _, m := range cangjieMainEntryRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		line := byteOffsetToLine(cleaned, m[0])
		if seenLines[line] {
			continue
		}
		syms = append(syms, types.Symbol{
			Name:     "main",
			Kind:     "function",
			File:     file,
			Line:     line,
			Exported: true,
			Doc:      "main entry",
		})
		seenLines[line] = true
	}

	// 3c) Cangjie `init` constructor (no `func` keyword). Common
	//     forms: `public init(...)`, `init(...)`. Marked Kind=ctor
	//     so downstream callers can distinguish constructors from
	//     regular methods — useful for e.g. "show me the constructors"
	//     queries.
	for _, m := range cangjieInitRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		line := byteOffsetToLine(cleaned, m[0])
		if seenLines[line] {
			continue
		}
		modPrefix := cleaned[m[2]:m[3]]
		syms = append(syms, types.Symbol{
			Name:     "init",
			Kind:     "ctor",
			File:     file,
			Line:     line,
			Exported: hasCangjieExportModifier(modPrefix),
			Doc:      cangjieModifierList(modPrefix) + " init",
		})
		seenLines[line] = true
	}

	// 4) class / struct / interface / enum
	for _, m := range cangjieTypeDeclRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		line := byteOffsetToLine(cleaned, m[0])
		if seenLines[line] {
			continue
		}
		modPrefix := cleaned[m[2]:m[3]]
		kw := cleaned[m[4]:m[5]]
		name := cleaned[m[6]:m[7]]
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     kw,
			File:     file,
			Line:     line,
			Exported: hasCangjieExportModifier(modPrefix),
			Doc:      cangjieModifierList(modPrefix),
		})
		seenLines[line] = true
	}

	// 5) extend blocks — record as Symbol so consumers can list
	//    "types extended in this package". The Symbol.Name is the
	//    extended type; Kind=extend; Parent is the Cangjie package
	//    (for cross-package visibility).
	for _, m := range cangjieExtendRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		line := byteOffsetToLine(cleaned, m[0])
		if seenLines[line] {
			continue
		}
		target := cleaned[m[2]:m[3]]
		syms = append(syms, types.Symbol{
			Name:     target,
			Kind:     "extend",
			File:     file,
			Line:     line,
			Parent:   pkg,
			Exported: true,
		})
		seenLines[line] = true
	}

	return syms
}

// scanCangjieExtendRelations emits one Relation per `extend` block
// so the graph records the type-extension edge. Kind is "inheritance"
// to match the existing Go/Java/Rust convention.
func scanCangjieExtendRelations(cleaned, file string) []types.Relation {
	var rels []types.Relation
	for _, m := range cangjieExtendRegex.FindAllStringSubmatchIndex(cleaned, -1) {
		target := cleaned[m[2]:m[3]]
		line := byteOffsetToLine(cleaned, m[0])
		rels = append(rels, types.Relation{
			Kind: "inheritance",
			FromEP: types.RelationEndpoint{
				Name: "extend",
				File: file,
				Line: line,
			},
			ToEP: types.RelationEndpoint{
				Name: target,
				File: file,
				Line: line,
			},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceRegexSalvage,
			Provenance: types.ProvenanceRegexFallback,
			ResolvedBy: "cangjie_regex_extend",
		})
	}
	return rels
}

// hasCangjieExportModifier returns true if `modPrefix` contains
// a `public` or `open` modifier — the two Cangjie modifiers that
// make a symbol visible outside its defining package.
func hasCangjieExportModifier(modPrefix string) bool {
	fields := strings.Fields(modPrefix)
	for _, f := range fields {
		if f == "public" || f == "open" {
			return true
		}
	}
	return false
}

// cangjieModifierList returns the modifier prefix as a single-line
// Doc string (trimmed, collapsed). Used so evidence display shows
// the modifier stack without needing a separate Symbol field.
func cangjieModifierList(modPrefix string) string {
	trimmed := strings.TrimSpace(modPrefix)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

// cangjieRegexSalvage is the Tier 2 fallback path. It runs the
// same regexes as Tier 1 but on the RAW source (no comment/string
// blanking) so keyword matches inside comments/strings may produce
// spurious symbols. Callers get the list as-is with ParseTier=2.
//
// The salvage exists so that pathological sources where comment
// detection itself fails (mismatched `/*` etc.) still surface some
// signal. In practice this is reachable only when the Tier 1 scan
// returned no symbols AND no package — a very narrow window.
func cangjieRegexSalvage(src []byte, file string) ([]types.Symbol, []types.Import, string) {
	s := string(src)
	pkg := findCangjiePackage(s)
	syms := scanCangjieDecls(s, src, file, pkg)
	imps := scanCangjieImports(s, file)
	return syms, imps, pkg
}
