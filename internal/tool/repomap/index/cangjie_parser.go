package index

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// cangjie_parser.go — recursive-descent parser over the token
// stream produced by cangjie_lexer.go. Extracts:
//
//   - package clause            → FileInfo.Package
//   - import declarations       → []types.Import
//   - type declarations         → Symbol{Kind:class|struct|interface|enum}
//   - extend blocks             → Symbol{Kind:extend} + inheritance relation
//   - function / method         → Symbol{Kind:function|method|ctor|operator|foreign-func}
//     (method-ness tracked via enclosing-type stack — red line P1/M1 fix)
//   - modifiers (public/open/…) → Symbol.Exported + Doc
//   - generics + where clauses  → skipped over without mis-matching braces
//
// The parser is deliberately permissive: it commits to "grab top-
// level decls + walk into class/struct bodies". Expression bodies,
// match branches, and property accessors are skipped as opaque
// brace-balanced blocks. This keeps the scope tight enough to hand-
// maintain while still producing parent-aware symbols that the
// graph and rank consumers can trust.

// parseCangjie runs the full pipeline: lex → parse → emit. Returns
// (pkg, symbols, imports, relations). Mirrors the signature the
// top-level extractCangjie wrapper expects.
func parseCangjie(src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation) {
	toks := lexCangjie(src)
	p := &cangjieParser{toks: toks, file: file}
	p.run()
	return p.pkg, p.syms, p.imps, p.rels
}

type cangjieParser struct {
	toks []cangjieToken
	pos  int
	file string

	pkg  string
	syms []types.Symbol
	imps []types.Import
	rels []types.Relation
}

// peek returns the token at offset `ahead` from pos without
// advancing. Returns the EOF token when out of range.
func (p *cangjieParser) peek(ahead int) cangjieToken {
	idx := p.pos + ahead
	if idx >= len(p.toks) {
		return p.toks[len(p.toks)-1] // EOF guaranteed by lexer
	}
	return p.toks[idx]
}

func (p *cangjieParser) cur() cangjieToken { return p.peek(0) }

func (p *cangjieParser) advance() cangjieToken {
	t := p.cur()
	if t.Kind != cjTokEOF {
		p.pos++
	}
	return t
}

// run is the top-level parse loop. Walks tokens, dispatching on
// keyword recognition for declarations we care about.
func (p *cangjieParser) run() {
	for p.cur().Kind != cjTokEOF {
		// Optional decorator stack before a declaration.
		decorators := p.consumeDecorators()

		// Collect modifiers like `public static` etc.
		modifiers := p.consumeModifiers()

		t := p.cur()
		switch {
		case t.Kind == cjTokKeyword && t.Text == "package":
			p.parsePackage()
		case t.Kind == cjTokKeyword && t.Text == "import":
			p.parseImport()
		case t.Kind == cjTokKeyword && (t.Text == "class" || t.Text == "struct" ||
			t.Text == "interface" || t.Text == "enum"):
			p.parseTypeDecl(t.Text, modifiers, decorators, "")
		case t.Kind == cjTokKeyword && t.Text == "extend":
			p.parseExtendDecl(modifiers, decorators)
		case t.Kind == cjTokKeyword && t.Text == "func":
			p.parseFuncDecl(modifiers, decorators, "")
		case t.Kind == cjTokKeyword && t.Text == "foreign":
			p.parseForeignFunc(modifiers, decorators)
		case t.Kind == cjTokKeyword && t.Text == "operator":
			p.parseOperatorFunc(modifiers, decorators, "")
		case t.Kind == cjTokKeyword && (t.Text == "let" || t.Text == "var" || t.Text == "const"):
			p.parsePropertyDecl(t.Text, modifiers, decorators, "")
		case t.Kind == cjTokKeyword && t.Text == "init":
			// Top-level init — unusual but legal inside a class
			// body. At file scope just consume.
			p.parseInit(modifiers, decorators, "")
		case t.Kind == cjTokKeyword && t.Text == "main":
			p.parseMainEntry()
		default:
			// Not a decl-starter we care about. Advance one token
			// so the loop makes progress; opaque expressions and
			// top-level `let`/`var` decls fall through here today.
			p.advance()
		}
	}
}

// consumeDecorators gobbles a sequence of `@Name(args)?` decorators
// at the current position. Returns the decorator names (without @).
func (p *cangjieParser) consumeDecorators() []string {
	var out []string
	for p.cur().Kind == cjTokAt {
		p.advance() // @
		if p.cur().Kind == cjTokIdent {
			out = append(out, p.cur().Text)
			p.advance()
		}
		// Optional argument list in parens.
		if p.cur().Kind == cjTokLParen {
			p.skipBalanced(cjTokLParen, cjTokRParen)
		}
		// Optional `[` bracket form: `@CallingThread["main"]`.
		if p.cur().Kind == cjTokLBracket {
			p.skipBalanced(cjTokLBracket, cjTokRBracket)
		}
	}
	return out
}

// consumeModifiers collects a run of modifier keywords (public,
// static, open, …). Returns them as an ordered list so the caller
// can reconstruct the Doc string in source order.
func (p *cangjieParser) consumeModifiers() []string {
	var mods []string
	for {
		t := p.cur()
		if t.Kind != cjTokKeyword {
			break
		}
		switch t.Text {
		case "public", "private", "protected", "internal",
			"open", "static", "sealed", "abstract",
			"override", "redef", "mut", "const", "unsafe":
			mods = append(mods, t.Text)
			p.advance()
		default:
			return mods
		}
	}
	return mods
}

// hasExportModifier reports whether any modifier is `public` / `open`.
func hasExportModifier(mods []string) bool {
	for _, m := range mods {
		if m == "public" || m == "open" {
			return true
		}
	}
	return false
}

func modifierDoc(mods []string) string {
	return strings.TrimSpace(strings.Join(mods, " "))
}

// parsePackage — `package a.b.c`.
func (p *cangjieParser) parsePackage() {
	p.advance() // 'package'
	var parts []string
	for {
		t := p.cur()
		if t.Kind == cjTokIdent {
			parts = append(parts, t.Text)
			p.advance()
		} else {
			break
		}
		if p.cur().Kind == cjTokDot {
			p.advance()
			continue
		}
		break
	}
	if len(parts) > 0 && p.pkg == "" {
		p.pkg = strings.Join(parts, ".")
	}
	p.consumeOptionalSemi()
}

// parseImport — `import a.b.c` / `import a.b as z`.
func (p *cangjieParser) parseImport() {
	start := p.cur()
	p.advance() // 'import'
	var parts []string
	for p.cur().Kind == cjTokIdent {
		parts = append(parts, p.cur().Text)
		p.advance()
		if p.cur().Kind == cjTokDot {
			p.advance()
			continue
		}
		break
	}
	alias := ""
	if p.cur().Kind == cjTokKeyword && p.cur().Text == "as" {
		p.advance()
		if p.cur().Kind == cjTokIdent {
			alias = p.cur().Text
			p.advance()
		}
	}
	if len(parts) > 0 {
		path := strings.Join(parts, ".")
		p.imps = append(p.imps, types.Import{
			Raw:   "import " + path,
			Path:  path,
			Alias: alias,
			File:  p.file,
			Line:  start.Line,
		})
	}
	p.consumeOptionalSemi()
}

// parseTypeDecl — class / struct / interface / enum.
func (p *cangjieParser) parseTypeDecl(keyword string, mods, decorators []string, parent string) {
	start := p.cur()
	p.advance() // keyword
	if p.cur().Kind != cjTokIdent {
		// Malformed; bail out to the next token.
		return
	}
	name := p.cur().Text
	p.advance()

	// Optional generics <T, U>
	p.skipGenerics()

	// Optional superclass / interfaces: `<:` Type1 & Type2
	parents := p.collectInheritanceParents()
	for _, sup := range parents {
		p.rels = append(p.rels, types.Relation{
			Kind: "inheritance",
			FromEP: types.RelationEndpoint{
				Name: name, File: p.file, Line: start.Line,
			},
			ToEP: types.RelationEndpoint{
				Name: sup, File: p.file, Line: start.Line,
			},
			File:       p.file,
			Line:       start.Line,
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceCangjieParser,
			ResolvedBy: "cangjie_inheritance_clause",
		})
	}

	// Optional where clause
	p.skipWhereClause()

	exported := hasExportModifier(mods)
	if keyword == "interface" {
		// Interface decls are exported by default in Cangjie.
		exported = true
	}
	sym := types.Symbol{
		Name:     name,
		Kind:     keyword,
		File:     p.file,
		Line:     start.Line,
		EndLine:  start.Line,
		Exported: exported,
		Parent:   parent,
		Doc:      modifierDoc(mods),
	}
	idx := len(p.syms)
	p.syms = append(p.syms, sym)

	// Descend into body. Body is optional for interface method
	// declarations without bodies; when present we record the
	// closing-brace line so read-mode scanners can slice the body.
	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.parseBody(name)
	}
}

// parseExtendDecl — `extend TargetType [<: Trait]* { ... }`.
func (p *cangjieParser) parseExtendDecl(mods, decorators []string) {
	start := p.cur()
	p.advance() // 'extend'
	// Target type — take the first identifier.
	if p.cur().Kind != cjTokIdent {
		return
	}
	target := p.cur().Text
	p.advance()
	// Skip type path tail `.Foo<T>` etc.
	for p.cur().Kind == cjTokDot {
		p.advance()
		if p.cur().Kind == cjTokIdent {
			target += "." + p.cur().Text
			p.advance()
		}
	}
	p.skipGenerics()
	// Trait list `<: A & B`.
	p.collectInheritanceParents()
	p.skipWhereClause()

	p.syms = append(p.syms, types.Symbol{
		Name:     target,
		Kind:     "extend",
		File:     p.file,
		Line:     start.Line,
		EndLine:  start.Line,
		Exported: true,
		Parent:   p.pkg,
		Doc:      modifierDoc(mods),
	})
	idx := len(p.syms) - 1
	p.rels = append(p.rels, types.Relation{
		Kind: "inheritance",
		FromEP: types.RelationEndpoint{
			Name: "extend", File: p.file, Line: start.Line,
		},
		ToEP: types.RelationEndpoint{
			Name: target, File: p.file, Line: start.Line,
		},
		File:       p.file,
		Line:       start.Line,
		Confidence: types.ConfidenceAST,
		Provenance: types.ProvenanceCangjieParser,
		ResolvedBy: "cangjie_extend_decl",
	})

	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.parseBody(target)
	}
}

// parseFuncDecl — `func Name<T>(params) ReturnType? whereClause? body?`.
func (p *cangjieParser) parseFuncDecl(mods, decorators []string, parent string) {
	start := p.cur()
	p.advance() // 'func'
	if p.cur().Kind != cjTokIdent {
		return
	}
	name := p.cur().Text
	p.advance()
	p.skipGenerics()
	arity := p.countAndSkipParams()
	// Return type `: T` — opaque, skip.
	cjReturnTypes := p.skipReturnType()
	p.skipWhereClause()

	kind := "function"
	if parent != "" {
		kind = "method"
	}
	p.syms = append(p.syms, types.Symbol{
		Name:            name,
		Kind:            kind,
		File:            p.file,
		Line:            start.Line,
		EndLine:         start.Line,
		Exported:        hasExportModifier(mods) || parent == "" && len(decorators) > 0,
		Parent:          parent,
		Arity:           arity,
		Doc:             modifierDoc(mods),
		ReturnTypeNames: cjReturnTypes,
	})
	idx := len(p.syms) - 1

	// Body may be absent (interface method). If present, skip it.
	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.skipBalancedEndLine(cjTokLBrace, cjTokRBrace)
	}
}

// parseForeignFunc — `foreign func Name(params) Ret`.
func (p *cangjieParser) parseForeignFunc(mods, decorators []string) {
	start := p.cur()
	p.advance() // 'foreign'
	if p.cur().Kind == cjTokKeyword && p.cur().Text == "func" {
		p.advance()
	}
	if p.cur().Kind != cjTokIdent {
		return
	}
	name := p.cur().Text
	p.advance()
	p.skipGenerics()
	p.countAndSkipParams()
	cjReturnTypes := p.skipReturnType()
	p.consumeOptionalSemi()
	p.syms = append(p.syms, types.Symbol{
		Name:            name,
		Kind:            "foreign-func",
		File:            p.file,
		Line:            start.Line,
		EndLine:         start.Line,
		Exported:        true,
		Doc:             strings.TrimSpace("foreign " + modifierDoc(mods)),
		ReturnTypeNames: cjReturnTypes,
	})
}

// parseOperatorFunc — `operator func +(rhs: T): T`.
func (p *cangjieParser) parseOperatorFunc(mods, decorators []string, parent string) {
	start := p.cur()
	p.advance() // 'operator'
	if p.cur().Kind == cjTokKeyword && p.cur().Text == "func" {
		p.advance()
	}
	// Operator token — may be `+`, `>>`, `==`, `[]` (subscript), or
	// an identifier like `call`. The `[]` form is special: the
	// signature is `operator func [](i: Int64): T` where `[` and
	// `]` together form the operator NAME, then the `(` opens the
	// parameter list. Accept any non-paren punctuation tokens
	// (including the bracket pair) as operator characters.
	opText := ""
	for {
		t := p.cur()
		if t.Kind == cjTokLParen || t.Kind == cjTokEOF {
			break
		}
		if t.Kind == cjTokIdent {
			// For identifier operators like `call`.
			opText = t.Text
			p.advance()
			break
		}
		// Collect punctuation chars as operator. Include brackets
		// (cjTokLBracket / cjTokRBracket) so `[]` subscript reads
		// correctly. Cap at 3 chars to avoid runaway.
		switch t.Kind {
		case cjTokOther, cjTokLAngle, cjTokRAngle, cjTokEq,
			cjTokLBracket, cjTokRBracket, cjTokColon, cjTokDot:
			opText += t.Text
			p.advance()
			if len(opText) >= 3 {
				break
			}
			continue
		}
		// Unknown token shape — advance defensively to avoid stall.
		p.advance()
	}
	p.countAndSkipParams()
	cjReturnTypes := p.skipReturnType()
	p.skipWhereClause()
	p.syms = append(p.syms, types.Symbol{
		Name:            "operator " + opText,
		Kind:            "operator",
		File:            p.file,
		Line:            start.Line,
		EndLine:         start.Line,
		Exported:        hasExportModifier(mods),
		Parent:          parent,
		Doc:             strings.TrimSpace("operator " + modifierDoc(mods)),
		ReturnTypeNames: cjReturnTypes,
	})
	idx := len(p.syms) - 1
	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.skipBalancedEndLine(cjTokLBrace, cjTokRBrace)
	}
}

// parseInit — `init(params) { ... }` (no func keyword).
func (p *cangjieParser) parseInit(mods, decorators []string, parent string) {
	start := p.cur()
	p.advance() // 'init'
	p.countAndSkipParams()
	p.syms = append(p.syms, types.Symbol{
		Name:     "init",
		Kind:     "ctor",
		File:     p.file,
		Line:     start.Line,
		EndLine:  start.Line,
		Exported: hasExportModifier(mods) || parent != "",
		Parent:   parent,
		Doc:      strings.TrimSpace(modifierDoc(mods) + " init"),
	})
	idx := len(p.syms) - 1
	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.skipBalancedEndLine(cjTokLBrace, cjTokRBrace)
	}
}

// parseMainEntry — `main(): Int64 { ... }`.
func (p *cangjieParser) parseMainEntry() {
	start := p.cur()
	p.advance() // 'main'
	if p.cur().Kind != cjTokLParen {
		return // not a main-entry shorthand
	}
	p.countAndSkipParams()
	cjReturnTypes := p.skipReturnType()
	p.syms = append(p.syms, types.Symbol{
		Name:            "main",
		Kind:            "function",
		File:            p.file,
		Line:            start.Line,
		EndLine:         start.Line,
		Exported:        true,
		Doc:             "main entry",
		ReturnTypeNames: cjReturnTypes,
	})
	idx := len(p.syms) - 1
	if p.cur().Kind == cjTokLBrace {
		p.syms[idx].EndLine = p.skipBalancedEndLine(cjTokLBrace, cjTokRBrace)
	}
}

// parseBody walks a `{ ... }` body, recognising nested decls and
// attaching them to `parent`. The body's own members may be
// methods, nested types, or property declarations; we handle the
// first two and opaquely skip the third.
func (p *cangjieParser) parseBody(parent string) int {
	if p.cur().Kind != cjTokLBrace {
		return 0
	}
	endLine := p.cur().Line
	p.advance() // `{`

	for p.cur().Kind != cjTokRBrace && p.cur().Kind != cjTokEOF {
		decorators := p.consumeDecorators()
		mods := p.consumeModifiers()
		t := p.cur()
		switch {
		case t.Kind == cjTokKeyword && (t.Text == "class" || t.Text == "struct" ||
			t.Text == "interface" || t.Text == "enum"):
			p.parseTypeDecl(t.Text, mods, decorators, parent)
		case t.Kind == cjTokKeyword && t.Text == "func":
			p.parseFuncDecl(mods, decorators, parent)
		case t.Kind == cjTokKeyword && t.Text == "init":
			p.parseInit(mods, decorators, parent)
		case t.Kind == cjTokKeyword && t.Text == "operator":
			p.parseOperatorFunc(mods, decorators, parent)
		case t.Kind == cjTokKeyword && t.Text == "foreign":
			p.parseForeignFunc(mods, decorators)
		case t.Kind == cjTokKeyword && (t.Text == "let" || t.Text == "var" || t.Text == "const"):
			p.parsePropertyDecl(t.Text, mods, decorators, parent)
		default:
			// Skip one token; a property decl / expression body
			// will eventually hit the closing brace.
			p.advance()
		}
		endLine = p.cur().Line
	}
	if p.cur().Kind == cjTokRBrace {
		endLine = p.cur().Line
		p.advance()
	}
	return endLine
}

// parsePropertyDecl captures Cangjie `let` / `var` / `const` declarations.
// Inside class / struct / interface bodies these are answer-grade fields;
// at top level they are module variables / constants. The parser consumes
// only the declaration header on the current source line and skips any
// initializer without interpreting expression tokens.
func (p *cangjieParser) parsePropertyDecl(keyword string, mods, decorators []string, parent string) {
	start := p.cur()
	p.advance() // let / var / const
	if p.cur().Kind != cjTokIdent {
		p.skipToPropertyBoundary(start.Line)
		return
	}
	name := p.cur().Text
	p.advance()
	signature := ""
	if p.cur().Kind == cjTokColon {
		p.advance()
		var parts []string
		for p.cur().Kind != cjTokEOF && p.cur().Kind != cjTokEq &&
			p.cur().Kind != cjTokSemicolon && p.cur().Kind != cjTokComma &&
			p.cur().Kind != cjTokRBrace && p.cur().Line == start.Line {
			if p.cur().Text != "" {
				parts = append(parts, p.cur().Text)
			}
			p.advance()
		}
		signature = strings.TrimSpace(strings.Join(parts, ""))
	}
	kind := "field"
	if parent == "" {
		kind = "var"
		if keyword == "let" || keyword == "const" {
			kind = "const"
		}
	}
	p.syms = append(p.syms, types.Symbol{
		Name:      name,
		Kind:      kind,
		File:      p.file,
		Line:      start.Line,
		EndLine:   start.Line,
		Exported:  hasExportModifier(mods),
		Parent:    parent,
		Signature: signature,
		Doc:       strings.TrimSpace(strings.Join(append([]string{}, append(mods, decorators...)...), " ")),
	})
	p.skipToPropertyBoundary(start.Line)
}

func (p *cangjieParser) skipToPropertyBoundary(startLine int) {
	for p.cur().Kind != cjTokEOF {
		switch p.cur().Kind {
		case cjTokSemicolon:
			p.advance()
			return
		case cjTokRBrace:
			return
		}
		if p.cur().Line > startLine {
			return
		}
		p.advance()
	}
}

// collectInheritanceParents consumes the optional `<:` list after a
// type declaration header. Cangjie shape: `<: A & B<T> & C.D`. We
// advance past `<:` (one token pair in the lexer stream), then
// pick identifiers separated by `&`.
//
// Returns the parent type names (bare, without generics). Called
// after `parseTypeDecl` reads the declared name and optional
// generics.
func (p *cangjieParser) collectInheritanceParents() []string {
	// Look for `<:` combo — the lexer emits `<` `:` or may emit
	// `:` directly when Cangjie writes `: A & B` (both forms exist
	// across the 1.0.0 spec).
	if p.cur().Kind == cjTokLAngle && p.peek(1).Kind == cjTokColon {
		p.advance()
		p.advance()
	} else if p.cur().Kind == cjTokColon {
		p.advance()
	} else {
		return nil
	}
	var out []string
	for {
		if p.cur().Kind != cjTokIdent {
			break
		}
		name := p.cur().Text
		p.advance()
		for p.cur().Kind == cjTokDot {
			p.advance()
			if p.cur().Kind == cjTokIdent {
				name += "." + p.cur().Text
				p.advance()
			}
		}
		p.skipGenerics()
		out = append(out, name)
		// Separator `&` (emitted as cjTokOther) or `,` — continue.
		if p.cur().Kind == cjTokOther && p.cur().Text == "&" {
			p.advance()
			continue
		}
		if p.cur().Kind == cjTokComma {
			p.advance()
			continue
		}
		break
	}
	return out
}

// skipGenerics skips `<T, U <: Bound>` if present. No-op if the
// current token isn't `<`, or if `<` is immediately followed by
// `:` (that is the `<:` inheritance operator, not a generic
// parameter list opener — must be left for collectInheritanceParents).
func (p *cangjieParser) skipGenerics() {
	if p.cur().Kind != cjTokLAngle {
		return
	}
	if p.peek(1).Kind == cjTokColon {
		return // `<:` inheritance token pair, not a generic
	}
	// Need to balance angles because bounds may contain `<` / `>`.
	depth := 0
	for p.cur().Kind != cjTokEOF {
		switch p.cur().Kind {
		case cjTokLAngle:
			depth++
		case cjTokRAngle:
			depth--
			if depth == 0 {
				p.advance()
				return
			}
		}
		p.advance()
	}
}

// skipWhereClause — Cangjie generic where clause (`where T <: Foo`).
// Terminated by `{` (body) or `=` (function expression body) or EOF.
func (p *cangjieParser) skipWhereClause() {
	if !(p.cur().Kind == cjTokKeyword && p.cur().Text == "where") {
		return
	}
	for p.cur().Kind != cjTokEOF {
		k := p.cur().Kind
		if k == cjTokLBrace || k == cjTokEq {
			return
		}
		p.advance()
	}
}

// skipReturnType skips `: ReturnType` after a parameter list. The
// return type ends at the first top-level (depth-0) brace / assign /
// where / statement-terminator / enclosing-closer token. Anything
// before that is part of the type expression (including nested
// generics, tuple types, function types, etc.).
//
// Phase 6 stage 21 (2026-05-03): now also collects bare identifier
// tokens encountered inside the return-type expression (across all
// depths) into the deduplicated set returned to the caller. The
// caller assigns the result to Symbol.ReturnTypeNames so the
// downstream typed factory-detection (containsFactoryReference)
// can resolve Cangjie factories the same way it does for tree-
// sitter languages. Returns nil when no return-type clause is
// present (procedures / void functions).
func (p *cangjieParser) skipReturnType() []string {
	if p.cur().Kind != cjTokColon {
		return nil
	}
	p.advance() // :
	depth := 0
	seen := map[string]bool{}
	var out []string
	for p.cur().Kind != cjTokEOF {
		t := p.cur()
		if depth == 0 {
			switch t.Kind {
			case cjTokLBrace, cjTokEq, cjTokSemicolon, cjTokRBrace:
				return out
			}
			if t.Kind == cjTokKeyword && t.Text == "where" {
				return out
			}
		}
		// Collect identifier-shaped tokens at any depth — generic
		// parameters, tuple component types, and bare type names
		// all qualify as "type names that contributed to the return
		// type". Generic-wrapper tokens (Option, Result, Box, etc.)
		// are intentionally included alongside the inner T.
		if t.Kind == cjTokIdent && !seen[t.Text] {
			seen[t.Text] = true
			out = append(out, t.Text)
		}
		switch t.Kind {
		case cjTokLAngle, cjTokLParen, cjTokLBracket:
			depth++
		case cjTokRAngle, cjTokRParen, cjTokRBracket:
			if depth == 0 {
				return out
			}
			depth--
		}
		p.advance()
	}
	return out
}

// countAndSkipParams consumes a parenthesised parameter list and
// returns the top-level comma count + 1 (the parameter arity).
// Nested parens (default values / lambda types) are balance-
// tracked so their commas do not inflate the count.
func (p *cangjieParser) countAndSkipParams() int {
	if p.cur().Kind != cjTokLParen {
		return 0
	}
	p.advance()
	depth := 0
	arity := 0
	hasAny := false
	for p.cur().Kind != cjTokEOF {
		t := p.cur()
		if depth == 0 {
			switch t.Kind {
			case cjTokRParen:
				p.advance()
				if hasAny {
					return arity + 1
				}
				return 0
			case cjTokComma:
				arity++
				p.advance()
				continue
			}
		}
		switch t.Kind {
		case cjTokLParen, cjTokLBracket, cjTokLAngle, cjTokLBrace:
			depth++
		case cjTokRParen, cjTokRBracket, cjTokRAngle, cjTokRBrace:
			if depth == 0 {
				p.advance()
				if hasAny {
					return arity + 1
				}
				return 0
			}
			depth--
		}
		hasAny = true
		p.advance()
	}
	return 0
}

// skipBalanced skips a matching pair of `open`/`close` tokens.
// Used for bodies we don't care to descend into (expression blocks,
// decorator arguments, etc.).
func (p *cangjieParser) skipBalanced(open, close cangjieTokKind) {
	_ = p.skipBalancedEndLine(open, close)
}

func (p *cangjieParser) skipBalancedEndLine(open, close cangjieTokKind) int {
	if p.cur().Kind != open {
		return p.cur().Line
	}
	endLine := p.cur().Line
	p.advance()
	depth := 1
	for p.cur().Kind != cjTokEOF && depth > 0 {
		endLine = p.cur().Line
		switch p.cur().Kind {
		case open:
			depth++
		case close:
			depth--
		}
		p.advance()
	}
	return endLine
}

// consumeOptionalSemi skips a trailing `;` (rare in Cangjie but legal).
func (p *cangjieParser) consumeOptionalSemi() {
	if p.cur().Kind == cjTokSemicolon {
		p.advance()
	}
}
