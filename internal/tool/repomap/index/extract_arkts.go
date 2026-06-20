package index

import (
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// extractArkTS produces a parsed FileInfo for a HarmonyOS ArkTS file.
//
// Strategy (red line: do not reinvent the TS grammar wheel):
//
//  1. Try the tree-sitter-typescript parser. ArkTS strict-mode is a
//     near-superset of TS surface for what tree-sitter can recognise
//     — imports, regular classes, functions, modules — so calling
//     extractJS gives us all the shared ground for free.
//  2. Run an ArkTS-aware post-pass (arkTSPostPass) directly on the
//     source bytes to recover the ArkTS-specific surface that is NOT
//     valid TypeScript and will produce ERROR nodes:
//     - `@Component struct Foo { ... build() { ... } ... }`
//     - `@State message: string = '...'`
//     - `@Builder` / `@Styles` / `@Extend(Type)` decorators
//     - the build() UI entry method
//  3. Merge: the ArkTS post-pass owns ArkTS-specific symbol kinds
//     (component, ui-entry, state-field, prop-field, …); the TS
//     extractor owns plain TS surface (function, class, interface,
//     …). When both produce a symbol with the same name+line, the
//     ArkTS post-pass wins because it has richer kind information.
//
// Tier semantics:
//
//   - Tier 1: TS grammar parsed AND post-pass found ≥1 ArkTS-specific
//     symbol (the file is genuinely ArkTS).
//   - Tier 1: TS grammar parsed but no ArkTS-specific symbols (the
//     file is plain .ts in an ArkTS project — also full-fidelity).
//   - Tier 2: TS grammar failed (file contains struct keyword early
//     or strict-mode-violating syntax that TS reject); fall back to
//     post-pass-only output.
//   - Tier 3: TS grammar failed AND post-pass found nothing useful;
//     return path-only (no symbols).
//
// extractArkTS is called from parser.go::parseOneFile when
// FileEntry.Language == LangArkTS.
func extractArkTS(src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation, tier int) {
	// Phase 1: try TS grammar.
	tsSyms, tsImps, tsRels, tsOK := tryTSGrammarForArkTS(src, file)

	// Phase 2: ArkTS regex post-pass — ALWAYS runs (Tier 1 augments,
	// Tier 2 stands alone). The post-pass is responsible for the
	// 21-decorator ArkTS surface.
	akSyms, akImps := arkTSPostPass(src, file)

	if tsOK {
		// Merge: dedup by (name, line) keeping ArkTS-specific kinds.
		syms = mergeArkTSSymbols(tsSyms, akSyms)
		imps = mergeArkTSImports(tsImps, akImps)
		rels = tsRels
		tier = 1
		return
	}

	// Phase 3: TS grammar failed entirely → tier 2 with only
	// post-pass output. If the post-pass also produced nothing,
	// drop to tier 3 (path-only).
	if len(akSyms) == 0 && len(akImps) == 0 {
		tier = 3
		return
	}
	syms = akSyms
	imps = akImps
	tier = 2
	return
}

// tryTSGrammarForArkTS calls extractJS with the TS parser. Returns
// (symbols, imports, relations, ok). ok=false when the TS grammar
// fails to produce a usable root, signalling fallback to Tier 2.
func tryTSGrammarForArkTS(src []byte, file string) ([]types.Symbol, []types.Import, []types.Relation, bool) {
	root, ok := parseTreeSitterIfPossible(types.LangTypeScript, src)
	if !ok {
		return nil, nil, nil, false
	}
	// extractJS owns its own walk; we throw away `pkg` (ArkTS has
	// no package keyword) and pass isTS=true.
	_, syms, imps, rels := extractJS(root, src, file, true)
	return syms, imps, rels, true
}

// ArkTS strict-mode decorator whitelist. 21 decorators total —
// kept here as the single source of truth so that the post-pass
// regex, IsExported, and any future skill prompts agree.
//
// Categorised for readability; concatenation order does not matter.
var arkTSDecoratorWhitelist = []string{
	// ArkUI structural
	"Component", "Entry", "Preview", "CustomDialog",
	"Observed", "Reusable",
	// Function-level UI
	"Builder", "BuilderParam", "Styles", "Extend",
	// State management
	"State", "Prop", "Link", "Provide", "Consume", "Watch",
	"ObjectLink",
	"StorageLink", "StorageProp",
	"LocalStorageLink", "LocalStorageProp",
}

// arkTSDecoratorRegex matches a single decorator with optional
// argument list, capturing the decorator name in group 1.
var arkTSDecoratorRegex = regexp.MustCompile(`@(\w+)(?:\s*\([^)]*\))?`)

// componentStructRegex matches `struct Name`. Two anchoring shapes
// are accepted because both occur in real ArkTS source:
//
//  1. `struct` at start-of-line (decorators stacked on preceding
//     lines, the canonical multi-line form). The leading whitespace
//     plus optional `export` prefix covers indented helpers.
//  2. `struct` after one or more decorators on the SAME line, e.g.
//     `@Component struct Card { ... }`. Walks back to the first `@`
//     so the post-pass picks up the inline decorator stack.
//
// `(?m)` is required for the `^` anchor to match at every line
// boundary inside the source string.
var componentStructRegex = regexp.MustCompile(
	`(?m)^[ \t]*(?:export\s+)?(?:@\w+(?:\s*\([^)]*\))?\s+)*struct[ \t]+(\w+)\b`)

// builderFunctionRegex matches `@Builder function Foo(...)` or
// `@Builder Bar(...)` (the latter is the in-struct method form).
var builderFunctionRegex = regexp.MustCompile(
	`(?m)^[ \t]*@Builder(?:\s*\([^)]*\))?\s+(?:function\s+)?(\w+)\s*\(`)

// stylesFunctionRegex matches `@Styles function foo()`.
var stylesFunctionRegex = regexp.MustCompile(
	`(?m)^[ \t]*@Styles\s+(?:function\s+)?(\w+)\s*\(`)

// extendFunctionRegex matches `@Extend(Type) function foo(...)`.
// Captures both the target type (group 1) and the function name
// (group 2) so the relation graph can record an "extends" edge.
var extendFunctionRegex = regexp.MustCompile(
	`(?m)^[ \t]*@Extend\s*\(\s*(\w+)\s*\)\s+(?:function\s+)?(\w+)\s*\(`)

// stateFieldRegex matches `@State|@Prop|@Link|@Provide|@Consume|
// @ObjectLink|@StorageLink|@StorageProp|@LocalStorageLink|
// @LocalStorageProp|@Watch (...)? <name>: <type>`.
//
// Group 1 = decorator name (e.g. "State"), group 2 = field name.
var stateFieldRegex = regexp.MustCompile(
	`(?m)^[ \t]*@(State|Prop|Link|Provide|Consume|ObjectLink|StorageLink|StorageProp|LocalStorageLink|LocalStorageProp|Watch)(?:\s*\([^)]*\))?\s+(\w+)\s*:`)

// buildMethodRegex matches `build() {` — the ArkUI render entry.
// Anchored to start-of-line indentation to avoid matching e.g.
// `console.build()` calls.
var buildMethodRegex = regexp.MustCompile(`(?m)^[ \t]+build\s*\(\s*\)\s*\{`)

// importRegex provides a Tier 2 salvage for `import` statements
// when the TS grammar bails out. Captures the path in group 2.
//
// Form 1: `import { A, B } from 'path'`
// Form 2: `import Foo from 'path'`
// Form 3: `import 'path'`
var importRegex = regexp.MustCompile(
	`(?m)^[ \t]*import\s+(?:(?:\{[^}]*\}|[\w*]+(?:\s*,\s*\{[^}]*\})?)\s+from\s+)?['"]([^'"]+)['"]`)

// arkTSPostPass scans `src` for ArkTS-specific surface and emits
// symbols + imports. Returns the discovered set; the caller is
// responsible for merging with any TS-grammar-produced output.
//
// Implementation note: we deliberately use line-based regex rather
// than a tree-sitter query because:
//   - the `struct` keyword is the very thing that breaks TS parse,
//     so a working parse tree may not exist on the salvage path
//   - ArkTS decorators are simple enough to capture without an AST
//   - reusing the same regex set for Tier 1 (post-pass) and Tier 2
//     (salvage-only) keeps the tier semantics symmetric
func arkTSPostPass(src []byte, file string) ([]types.Symbol, []types.Import) {
	srcStr := string(src)
	lines := strings.Split(srcStr, "\n")

	var syms []types.Symbol

	// 1) Components: scan struct declarations, walk back to find
	//    the decorator stack, attach each decorator name to Doc.
	//    Decorators may live on lines BEFORE the struct or be
	//    stacked INLINE on the same line — both forms are covered
	//    by combining walkBackForDecorators with an inline scan.
	for _, m := range componentStructRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		name := srcStr[m[2]:m[3]]
		line := byteOffsetToLine(srcStr, m[0])
		// Inline decorators: scan the matched range (m[0]..m[1])
		// for `@Word` tokens before reaching `struct`.
		inline := arkTSDecoratorRegex.FindAllStringSubmatch(srcStr[m[0]:m[1]], -1)
		var inlineDecs []string
		for _, dm := range inline {
			inlineDecs = append(inlineDecs, dm[1])
		}
		// Multi-line stack from preceding lines.
		stacked := walkBackForDecorators(lines, line)
		decorators := append(inlineDecs, stacked...)
		exported := containsAny(decorators, "Component", "Entry", "Preview", "CustomDialog")
		kind := "component"
		if !containsAny(decorators, arkTSDecoratorWhitelist...) {
			// `struct` outside ArkUI — uncommon but possible in
			// helpers; keep as plain struct kind.
			kind = "struct"
		}
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     kind,
			File:     file,
			Line:     line,
			EndLine:  findBlockEndLine(lines, line),
			Exported: exported,
			Doc:      decoratorsAsDoc(decorators),
		})
	}

	// 2) build() UI entry methods.
	for _, m := range buildMethodRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		line := byteOffsetToLine(srcStr, m[0])
		parent := findEnclosingStruct(lines, line, syms)
		syms = append(syms, types.Symbol{
			Name:     "build",
			Kind:     "ui-entry",
			File:     file,
			Line:     line,
			EndLine:  findBlockEndLine(lines, line),
			Exported: true, // build() is always module-visible to runtime
			Parent:   parent,
		})
	}

	// 3) @Builder / @Styles / @Extend free functions.
	for _, m := range builderFunctionRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		name := srcStr[m[2]:m[3]]
		line := byteOffsetToLine(srcStr, m[0])
		syms = append(syms, types.Symbol{
			Name: name, Kind: "builder", File: file, Line: line,
			EndLine: findBlockEndLine(lines, line), Exported: true, Doc: "@Builder",
		})
	}
	for _, m := range stylesFunctionRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		name := srcStr[m[2]:m[3]]
		line := byteOffsetToLine(srcStr, m[0])
		syms = append(syms, types.Symbol{
			Name: name, Kind: "styles", File: file, Line: line,
			EndLine: findBlockEndLine(lines, line), Exported: true, Doc: "@Styles",
		})
	}
	for _, m := range extendFunctionRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		target := srcStr[m[2]:m[3]]
		name := srcStr[m[4]:m[5]]
		line := byteOffsetToLine(srcStr, m[0])
		syms = append(syms, types.Symbol{
			Name:     name,
			Kind:     "extend",
			File:     file,
			Line:     line,
			EndLine:  findBlockEndLine(lines, line),
			Parent:   target,
			Exported: true,
			Doc:      "@Extend(" + target + ")",
		})
	}

	// 4) @State / @Prop / @Link / @Provide / @Consume / @ObjectLink /
	//    @StorageLink / @StorageProp / @LocalStorageLink /
	//    @LocalStorageProp / @Watch fields.
	for _, m := range stateFieldRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		decoName := srcStr[m[2]:m[3]]
		fieldName := srcStr[m[4]:m[5]]
		line := byteOffsetToLine(srcStr, m[0])
		parent := findEnclosingStruct(lines, line, syms)
		syms = append(syms, types.Symbol{
			Name:     fieldName,
			Kind:     decoratorToFieldKind(decoName),
			File:     file,
			Line:     line,
			Parent:   parent,
			Exported: false,
			Doc:      "@" + decoName,
		})
	}

	// 5) Imports — Tier 2 salvage only effectively (Tier 1 already
	//    has TS-grammar imports). Cheap to run unconditionally; the
	//    merge step de-dupes.
	var imps []types.Import
	for _, m := range importRegex.FindAllStringSubmatchIndex(srcStr, -1) {
		path := srcStr[m[2]:m[3]]
		line := byteOffsetToLine(srcStr, m[0])
		imps = append(imps, types.Import{
			Raw:  srcStr[m[0]:m[1]],
			Path: path,
			File: file,
			Line: line,
		})
	}

	return syms, imps
}

// decoratorToFieldKind maps a state-management decorator to a
// canonical Symbol.Kind. Centralised so other consumers (skill
// prompt, log_triage entity hint) can reuse the same labels.
func decoratorToFieldKind(decorator string) string {
	switch decorator {
	case "State":
		return "state-field"
	case "Prop":
		return "prop-field"
	case "Link":
		return "link-field"
	case "Provide":
		return "provide-field"
	case "Consume":
		return "consume-field"
	case "ObjectLink":
		return "object-link-field"
	case "StorageLink", "StorageProp":
		return "storage-field"
	case "LocalStorageLink", "LocalStorageProp":
		return "local-storage-field"
	case "Watch":
		return "watch-binding"
	default:
		return "field"
	}
}

// walkBackForDecorators returns the decorator names attached to a
// struct or method at `targetLine`. Walks up consecutive non-blank
// lines that start with `@` until a blank or non-decorator line.
func walkBackForDecorators(lines []string, targetLine int) []string {
	if targetLine <= 0 || targetLine > len(lines) {
		return nil
	}
	var out []string
	// targetLine is 1-based; lines is 0-based.
	for i := targetLine - 2; i >= 0; i-- {
		stripped := strings.TrimSpace(lines[i])
		if stripped == "" {
			break
		}
		if !strings.HasPrefix(stripped, "@") {
			break
		}
		// Extract decorator name(s) from this line — typically one,
		// occasionally many on a stacked line `@Entry @Component`.
		for _, m := range arkTSDecoratorRegex.FindAllStringSubmatch(stripped, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// findBlockEndLine scans forward from `startLine` until brace balance
// returns to 0. It works for component structs and decorator-driven
// function-like blocks because all of them use the same `{...}` surface
// in ArkTS source.
func findBlockEndLine(lines []string, startLine int) int {
	if startLine <= 0 || startLine > len(lines) {
		return startLine
	}
	depth := 0
	seenOpen := false
	for i := startLine - 1; i < len(lines); i++ {
		for _, ch := range lines[i] {
			switch ch {
			case '{':
				depth++
				seenOpen = true
			case '}':
				depth--
				if seenOpen && depth == 0 {
					return i + 1
				}
			}
		}
	}
	return startLine
}

// findEnclosingStruct returns the name of the nearest preceding
// struct or component on or before `line`. Used to attach build()
// and state fields to their owning struct as the Symbol.Parent.
func findEnclosingStruct(lines []string, line int, candidates []types.Symbol) string {
	best := ""
	bestLine := 0
	for _, s := range candidates {
		if (s.Kind == "component" || s.Kind == "struct") && s.Line <= line && s.Line > bestLine {
			best = s.Name
			bestLine = s.Line
		}
	}
	return best
}

// byteOffsetToLine converts a byte offset within `s` to a 1-based
// line number. Used because the regex APIs return byte indices but
// Symbol.Line is 1-based line counts.
func byteOffsetToLine(s string, off int) int {
	if off < 0 || off >= len(s) {
		return 1
	}
	return strings.Count(s[:off], "\n") + 1
}

// containsAny reports whether `set` contains any of `needles`.
// Helper for decorator whitelist lookups.
func containsAny(set []string, needles ...string) bool {
	for _, s := range set {
		for _, n := range needles {
			if s == n {
				return true
			}
		}
	}
	return false
}

// decoratorsAsDoc renders a decorator stack as a single-line Doc:
//
//	@Entry @Component
//
// so consumers (rank, evidence display) can read it without
// needing a separate field on Symbol.
func decoratorsAsDoc(decorators []string) string {
	if len(decorators) == 0 {
		return ""
	}
	uniq := map[string]bool{}
	var parts []string
	for _, d := range decorators {
		if !uniq[d] {
			uniq[d] = true
			parts = append(parts, "@"+d)
		}
	}
	sort.Strings(parts) // deterministic order for cache stability
	return strings.Join(parts, " ")
}

// mergeArkTSSymbols combines TS-grammar symbols with ArkTS
// post-pass symbols, de-duping by (Name, Line). The ArkTS post-pass
// wins on tie because its kinds are richer (component vs class,
// state-field vs property).
func mergeArkTSSymbols(ts, ak []types.Symbol) []types.Symbol {
	seen := make(map[string]int) // key → index in result
	out := make([]types.Symbol, 0, len(ts)+len(ak))
	add := func(s types.Symbol, prefer bool) {
		key := s.Name + ":" + intToStr(s.Line)
		if idx, ok := seen[key]; ok {
			if prefer {
				out[idx] = s
			}
			return
		}
		seen[key] = len(out)
		out = append(out, s)
	}
	for _, s := range ts {
		add(s, false)
	}
	for _, s := range ak {
		add(s, true)
	}
	return out
}

// mergeArkTSImports combines TS-grammar imports with regex-found
// imports, de-duping by Path. TS grammar wins on tie (it has
// alias and raw context).
func mergeArkTSImports(ts, ak []types.Import) []types.Import {
	seen := map[string]bool{}
	out := make([]types.Import, 0, len(ts)+len(ak))
	for _, imp := range ts {
		if seen[imp.Path] {
			continue
		}
		seen[imp.Path] = true
		out = append(out, imp)
	}
	for _, imp := range ak {
		if seen[imp.Path] {
			continue
		}
		seen[imp.Path] = true
		out = append(out, imp)
	}
	return out
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
