package index

import (
	"regexp"
	"strconv"
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
	pkg, syms, imps, rels, _, tier = extractCangjieWithLineFeatures(src, file)
	return
}

// extractCangjieWithLineFeatures is the FileInfo-producing entry. Cangjie has
// no tree-sitter grammar, so its lexer and AST-grade call extractor own the
// equivalent typed line features. The compatibility wrapper above preserves
// the existing focused extractor API.
func extractCangjieWithLineFeatures(src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation, lineFeatures map[int][]types.LineFeature, tier int) {
	pkg, syms, imps, rels, lineFeatures, _, tier = extractCangjieWithStructuralFeatures(src, file)
	return
}

func extractCangjieWithStructuralFeatures(src []byte, file string) (pkg string, syms []types.Symbol, imps []types.Import, rels []types.Relation, lineFeatures map[int][]types.LineFeature, branches []types.ControlFlowBranch, tier int) {
	// Phase 1: tokeniser + recursive-descent parser (D5/M1 upgrade).
	pkg, syms, imps, rels = parseCangjie(src, file)
	rels = append(rels, cangjieExtractCalls(src, file)...)
	lineFeatures = cangjieExtractLineFeatures(src, rels)
	branches = cangjieExtractControlFlowBranches(src, rels)
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
	rels = append(rels, cangjieExtractCalls(src, file)...)
	lineFeatures = cangjieExtractLineFeatures(src, rels)
	branches = cangjieExtractControlFlowBranches(src, rels)

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

// cangjieExtractLineFeatures maps the hand-written parser's exact tokens and
// call relations onto the shared LineFeature vocabulary. `return` is an exact
// lexer token outside comments and strings; calls are accepted only from the
// Cangjie parser provenance. This mirrors the closed tree-sitter node switch
// instead of guessing from arbitrary source substrings.
func cangjieExtractLineFeatures(src []byte, rels []types.Relation) map[int][]types.LineFeature {
	out := make(map[int][]types.LineFeature)
	add := func(line int, feature types.LineFeature) {
		if line <= 0 {
			return
		}
		for _, existing := range out[line] {
			if existing == feature {
				return
			}
		}
		out[line] = append(out[line], feature)
	}
	for _, token := range lexCangjie(src) {
		if token.Kind != cjTokIdent && token.Kind != cjTokKeyword {
			continue
		}
		switch token.Text {
		case "return":
			add(token.Line, types.LineFeatureReturnStmt)
		case "if", "else", "match", "case":
			add(token.Line, types.LineFeatureGuard)
		}
	}
	for _, rel := range rels {
		if rel.Kind == "call" && rel.Provenance == types.ProvenanceCangjieParser {
			add(rel.Line, types.LineFeatureCallExpression)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cangjieExtractControlFlowBranches is the hand-written-parser counterpart of
// extractControlFlowBranches. It accepts only exact lexer tokens and balanced
// braces, then attaches parser-authored call/return effects by bounded source
// positions. It never infers an arm from line adjacency.
func cangjieExtractControlFlowBranches(src []byte, rels []types.Relation) []types.ControlFlowBranch {
	tokens := lexCangjie(src)
	var out []types.ControlFlowBranch
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Text != "if" || (tokens[i].Kind != cjTokIdent && tokens[i].Kind != cjTokKeyword) {
			continue
		}
		open := cangjieNextControlBrace(tokens, i+1)
		if open < 0 {
			continue
		}
		close := cangjieMatchingControlBrace(tokens, open)
		if close < 0 {
			continue
		}
		conditionStart := tokens[i].Offset + len(tokens[i].Text)
		conditionEnd := tokens[open].Offset
		condition := ""
		if conditionStart >= 0 && conditionStart <= conditionEnd && conditionEnd <= len(src) {
			condition = compactControlFlowText(string(src[conditionStart:conditionEnd]))
		}
		out = append(out, cangjieMaterializeControlBranch(
			src, tokens, rels, condition, tokens[i].Line,
			types.ControlFlowArmConsequence, open, close,
		))

		next := close + 1
		if next >= len(tokens) || tokens[next].Text != "else" ||
			(tokens[next].Kind != cjTokIdent && tokens[next].Kind != cjTokKeyword) {
			continue
		}
		altOpen := cangjieNextControlBrace(tokens, next+1)
		if altOpen < 0 {
			continue
		}
		altClose := cangjieMatchingControlBrace(tokens, altOpen)
		if altClose < 0 {
			continue
		}
		out = append(out, cangjieMaterializeControlBranch(
			src, tokens, rels, condition, tokens[next].Line,
			types.ControlFlowArmAlternative, altOpen, altClose,
		))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cangjieNextControlBrace(tokens []cangjieToken, start int) int {
	paren, bracket := 0, 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case cjTokLParen:
			paren++
		case cjTokRParen:
			if paren > 0 {
				paren--
			}
		case cjTokLBracket:
			bracket++
		case cjTokRBracket:
			if bracket > 0 {
				bracket--
			}
		case cjTokLBrace:
			if paren == 0 && bracket == 0 {
				return i
			}
		case cjTokSemicolon, cjTokEOF:
			if paren == 0 && bracket == 0 {
				return -1
			}
		}
	}
	return -1
}

func cangjieMatchingControlBrace(tokens []cangjieToken, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case cjTokLBrace:
			depth++
		case cjTokRBrace:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func cangjieMaterializeControlBranch(src []byte, tokens []cangjieToken, rels []types.Relation, condition string, guardLine int, arm types.ControlFlowBranchArm, open, close int) types.ControlFlowBranch {
	startLine := tokens[open].Line
	endLine := tokens[close].Line
	var effects []types.ControlFlowEffect
	seen := make(map[string]bool)
	add := func(effect types.ControlFlowEffect) {
		key := string(effect.Kind) + "\x00" + effect.Expression + "\x00" + strconv.Itoa(effect.LineStart)
		if effect.Kind.IsValid() && effect.Expression != "" && effect.LineStart > 0 && !seen[key] {
			seen[key] = true
			effects = append(effects, effect)
		}
	}
	for _, rel := range rels {
		if rel.Kind != "call" || rel.Provenance != types.ProvenanceCangjieParser || rel.Line < startLine || rel.Line > endLine {
			continue
		}
		expr := strings.TrimSpace(rel.ToEP.Name)
		if rel.ToEP.Receiver != "" {
			expr = strings.TrimSpace(rel.ToEP.Receiver) + "." + expr
		}
		add(types.ControlFlowEffect{Kind: types.ControlFlowEffectCall, Expression: expr, LineStart: rel.Line, LineEnd: rel.Line})
	}
	for i := open + 1; i < close; i++ {
		if tokens[i].Text == "return" && (tokens[i].Kind == cjTokIdent || tokens[i].Kind == cjTokKeyword) {
			add(types.ControlFlowEffect{Kind: types.ControlFlowEffectReturn, Expression: "return", LineStart: tokens[i].Line, LineEnd: tokens[i].Line})
		}
	}
	return types.ControlFlowBranch{
		Condition:     condition,
		GuardLine:     guardLine,
		Arm:           arm,
		BodyLineStart: startLine,
		BodyLineEnd:   endLine,
		Effects:       effects,
		Provenance:    types.ProvenanceCangjieParser,
		ResolvedBy:    "cangjie_balanced_control_branch",
	}
}

// cangjieExtractCalls performs an expression-level pass over the same
// comment/string-aware token stream used by the declaration parser. The
// declaration parser intentionally treats bodies as opaque; without this
// second pass Cangjie was the only statically typed executable language whose
// repomap could never carry a source call edge. The pass emits only explicit
// name(...) and receiver.name(...) token shapes and excludes declaration and
// control heads. It does not infer receiver types.
func cangjieExtractCalls(src []byte, file string) []types.Relation {
	tokens := lexCangjie(src)
	parameterAuthorities := cangjieParameterReceiverAuthorities(tokens)
	scopes := []cangjieReceiverScope{{parent: -1, authorities: make(map[string]lexicalReceiverAuthority)}}
	var rels []types.Relation
	for i := 0; i+1 < len(tokens); i++ {
		switch tokens[i].Kind {
		case cjTokLBrace:
			authorities := make(map[string]lexicalReceiverAuthority)
			for name, authority := range parameterAuthorities[i] {
				authorities[name] = authority
			}
			scopes = append(scopes, cangjieReceiverScope{parent: len(scopes) - 1, authorities: authorities})
			continue
		case cjTokRBrace:
			if len(scopes) > 1 {
				scopes = scopes[:len(scopes)-1]
			}
			continue
		}
		if cangjieLocalReceiverDeclarationAt(tokens, i) {
			typeName := ""
			if i+2 < len(tokens) && tokens[i+1].Kind == cjTokColon &&
				(tokens[i+2].Kind == cjTokIdent || tokens[i+2].Kind == cjTokKeyword) {
				typeName = tokens[i+2].Text
			}
			addLexicalReceiverAuthority(scopes[len(scopes)-1].authorities, tokens[i].Text, typeName)
		}
		nameTok := tokens[i]
		if nameTok.Kind != cjTokIdent || tokens[i+1].Kind != cjTokLParen {
			continue
		}
		name := strings.TrimSpace(nameTok.Text)
		if name == "" || cangjieNonCallHead(name) || cangjieDeclarationCallHead(tokens, i) {
			continue
		}
		receiver := ""
		if i >= 2 && tokens[i-1].Kind == cjTokDot && cangjieReceiverToken(tokens[i-2]) {
			start := i - 2
			for start >= 2 && tokens[start-1].Kind == cjTokDot && cangjieReceiverToken(tokens[start-2]) {
				start -= 2
			}
			var b strings.Builder
			for j := start; j <= i-2; j++ {
				b.WriteString(tokens[j].Text)
			}
			receiver = strings.TrimSpace(b.String())
		}
		if binding := cangjieReceiverBinding(receiver); binding != "" {
			if receiverType, declared := cangjieReceiverTypeAt(scopes, binding); declared && receiverType != "" {
				receiver = receiverType
			}
		}
		rels = append(rels, types.Relation{
			Kind:       "call",
			FromEP:     types.RelationEndpoint{File: file, Line: nameTok.Line},
			ToEP:       types.RelationEndpoint{Name: name, Receiver: receiver, File: file, Line: nameTok.Line},
			File:       file,
			Line:       nameTok.Line,
			Confidence: types.ConfidenceAST,
			Provenance: types.ProvenanceCangjieParser,
			ResolvedBy: "cangjie_token_call",
		})
	}
	return rels
}

type cangjieReceiverScope struct {
	parent      int
	authorities map[string]lexicalReceiverAuthority
}

func cangjieParameterReceiverAuthorities(tokens []cangjieToken) map[int]map[string]lexicalReceiverAuthority {
	byBody := make(map[int]map[string]lexicalReceiverAuthority)
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Kind != cjTokIdent || tokens[i+1].Kind != cjTokColon ||
			(tokens[i+2].Kind != cjTokIdent && tokens[i+2].Kind != cjTokKeyword) ||
			!cangjieBindingDeclarationAt(tokens, i) || cangjieLocalReceiverDeclarationAt(tokens, i) {
			continue
		}
		body := cangjieDeclarationBodyBrace(tokens, i)
		if body < 0 {
			continue
		}
		if byBody[body] == nil {
			byBody[body] = make(map[string]lexicalReceiverAuthority)
		}
		addLexicalReceiverAuthority(byBody[body], tokens[i].Text, tokens[i+2].Text)
	}
	return byBody
}

func cangjieReceiverTypeAt(scopes []cangjieReceiverScope, binding string) (string, bool) {
	for scope := len(scopes) - 1; scope >= 0; scope = scopes[scope].parent {
		if authority, declared := scopes[scope].authorities[binding]; declared {
			if authority.conflicted {
				return "", true
			}
			return authority.typeName, true
		}
	}
	return "", false
}

func cangjieLocalReceiverDeclarationAt(tokens []cangjieToken, nameIndex int) bool {
	if nameIndex <= 0 || nameIndex >= len(tokens) || tokens[nameIndex].Kind != cjTokIdent {
		return false
	}
	prev := tokens[nameIndex-1]
	return prev.Kind == cjTokKeyword && (prev.Text == "let" || prev.Text == "var" || prev.Text == "const")
}

func cangjieDeclarationBodyBrace(tokens []cangjieToken, nameIndex int) int {
	depth := 0
	open := -1
	for i := nameIndex - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case cjTokRParen:
			depth++
		case cjTokLParen:
			if depth == 0 {
				open = i
				i = -1
				continue
			}
			depth--
		}
	}
	if open < 0 {
		return -1
	}
	depth = 0
	close := -1
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case cjTokLParen:
			depth++
		case cjTokRParen:
			depth--
			if depth == 0 {
				close = i
				i = len(tokens)
			}
		}
	}
	if close < 0 {
		return -1
	}
	for i := close + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case cjTokLBrace:
			return i
		case cjTokSemicolon, cjTokEq:
			return -1
		}
	}
	return -1
}

func cangjieBindingDeclarationAt(tokens []cangjieToken, nameIndex int) bool {
	if nameIndex <= 0 || nameIndex >= len(tokens) {
		return false
	}
	prev := tokens[nameIndex-1]
	if prev.Kind == cjTokKeyword && (prev.Text == "let" || prev.Text == "var" || prev.Text == "const") {
		return true
	}
	depth := 0
	open := -1
	for i := nameIndex - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case cjTokRParen:
			depth++
		case cjTokLParen:
			if depth == 0 {
				open = i
				i = -1
				continue
			}
			depth--
		}
	}
	if open < 1 {
		return false
	}
	// Function/init/main/type declaration headers are the only parenthesized
	// binding domains accepted here. A call such as submit(width: payload)
	// therefore cannot contribute receiver type authority.
	for i := open - 1; i >= 0 && i >= open-3; i-- {
		if tokens[i].Kind != cjTokKeyword {
			continue
		}
		switch tokens[i].Text {
		case "func", "init", "main", "operator", "foreign", "class", "struct":
			return true
		}
	}
	return false
}

func cangjieReceiverBinding(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if idx := strings.LastIndex(receiver, "."); idx >= 0 {
		receiver = receiver[idx+1:]
	}
	return strings.TrimSpace(receiver)
}

func cangjieReceiverToken(tok cangjieToken) bool {
	return tok.Kind == cjTokIdent || tok.Kind == cjTokKeyword
}

func cangjieNonCallHead(name string) bool {
	switch name {
	case "if", "else", "for", "while", "match", "catch", "try", "synchronized",
		"return", "throw", "spawn", "quote", "macro":
		return true
	default:
		return false
	}
}

func cangjieDeclarationCallHead(tokens []cangjieToken, nameIndex int) bool {
	if nameIndex <= 0 {
		return false
	}
	prev := tokens[nameIndex-1]
	if prev.Kind != cjTokKeyword {
		return false
	}
	switch prev.Text {
	case "func", "class", "struct", "interface", "enum", "extend", "operator", "foreign":
		return true
	default:
		return false
	}
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
