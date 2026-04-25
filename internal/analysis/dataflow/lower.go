package dataflow

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

type genericLowerer struct {
	lang string
}

func (l genericLowerer) Language() string { return l.lang }

func (l genericLowerer) LowerFile(repoRoot string, file *repomap.FileInfo, source []string, opts Options) LoweredFile {
	lowered := LoweredFile{
		File:     file.RelPath,
		Language: file.Language,
		Hash:     file.Hash,
	}

	relsByLine := make(map[int][]repomap.Relation)
	for _, rel := range file.Relations {
		relsByLine[rel.Line] = append(relsByLine[rel.Line], rel)
	}

	for idx := range file.Symbols {
		sym := file.Symbols[idx]
		if sym.Kind != "function" && sym.Kind != "method" {
			continue
		}
		start := sym.Line
		end := sym.EndLine
		if start <= 0 || end <= 0 || start > len(source) {
			continue
		}
		if end > len(source) {
			end = len(source)
		}
		if end-start > opts.MaxNodesPerFunc {
			end = start + opts.MaxNodesPerFunc
			if end > len(source) {
				end = len(source)
			}
		}
		snippet := source[start-1 : end]
		summary := l.lowerSymbol(file, sym, snippet, relsByLine)
		lowered.NodeCount += len(snippet)
		lowered.Summaries = append(lowered.Summaries, summary)
		lowered.Evidence = append(lowered.Evidence, summary.ProducerEvidence...)
	}

	return lowered
}

// prepareBiasTokens lowercases and trims the EntityBias slice once per
// LowerFile call so the hot loop does not re-normalize on every
// comparison. Empty tokens are dropped so callers that pass " " or ""
// don't produce spurious matches against any haystack.
func prepareBiasTokens(bias []string) []string {
	if len(bias) == 0 {
		return nil
	}
	out := make([]string, 0, len(bias))
	for _, b := range bias {
		b = strings.ToLower(strings.TrimSpace(b))
		if b == "" {
			continue
		}
		out = append(out, b)
	}
	return out
}

// fileHitsBias reports whether any prepared bias token appears in the
// lowercased file path. Mirrors the path-side half of sortByEntityBias
// so the symbol-admission gate and the file-ranking gate agree on what
// counts as a path match.
func fileHitsBias(relPath string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	path := strings.ToLower(relPath)
	for _, t := range tokens {
		if strings.Contains(path, t) {
			return true
		}
	}
	return false
}

// fileHitsExplicitBias reports whether the file path matches an
// explicit file-oriented bias token (path separator, extension, or
// dotted locator). This is stricter than fileHitsBias: generic entity
// tokens like "explorer" may overlap a path incidentally, but only an
// explicit file locator should admit every symbol in the file.
func fileHitsExplicitBias(relPath string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	path := strings.ToLower(relPath)
	for _, t := range tokens {
		if !isExplicitFileBiasToken(t) {
			continue
		}
		if strings.Contains(path, t) {
			return true
		}
	}
	return false
}

func isExplicitFileBiasToken(tok string) bool {
	tok = strings.TrimSpace(strings.ToLower(tok))
	if tok == "" {
		return false
	}
	return strings.ContainsAny(tok, `/\`) || strings.Contains(tok, ".")
}

// symbolHitsBias reports whether any prepared bias token appears in
// the lowercased symbol name. Mirrors the symbol-side half of
// sortByEntityBias for the same reason as fileHitsBias.
func symbolHitsBias(symName string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(symName))
	if idx := strings.LastIndex(name, ":"); idx >= 0 && idx+1 < len(name) {
		name = name[idx+1:]
	}
	for _, t := range tokens {
		if strings.Contains(name, t) {
			return true
		}
	}
	return false
}

// applyEvidenceRelevanceGate filters a file's producer evidence by
// subject-aware relevance and enforces the per-file item cap. It is
// applied by Analyze AFTER loadOrLowerFile so the on-disk dataflow
// cache stays pinned to the pristine lowerer output — a second
// question with different EntityBias must not see the first
// question's filtered slate.
//
// Filtering is per-item. Explicit file locators (for example
// "explorer.go" or "config/provider.xml") admit the whole file.
// Generic path overlap is narrower: when the file path matches but at
// least one symbol also matches, only the symbol-matching items are
// kept. That prevents a large file from spilling unrelated helper
// evidence into Primary Evidence just because one token overlaps its
// basename. When no symbol matches and only the file path matches, the
// gate falls back to whole-file admission so file/module-oriented
// questions still get evidence instead of an empty slate.
//
// With empty bias the relevance filter is a no-op; with zero cap the
// truncation is a no-op. Both zero = pass-through (legacy behaviour).
func applyEvidenceRelevanceGate(evidence []types.EvidenceItem, tokens []string, maxItems int, filePathHit, explicitPathHit bool) []types.EvidenceItem {
	if len(tokens) == 0 && maxItems <= 0 {
		return evidence
	}
	allowWholeFile := explicitPathHit
	if len(tokens) > 0 && filePathHit && !allowWholeFile {
		anySubjectHit := false
		for _, ev := range evidence {
			if symbolHitsBias(ev.Subject, tokens) {
				anySubjectHit = true
				break
			}
		}
		allowWholeFile = !anySubjectHit
	}
	out := make([]types.EvidenceItem, 0, len(evidence))
	for _, ev := range evidence {
		if len(tokens) > 0 && !allowWholeFile && !symbolHitsBias(ev.Subject, tokens) {
			continue
		}
		if maxItems > 0 && len(out) >= maxItems {
			break
		}
		out = append(out, ev)
	}
	return out
}

func (l genericLowerer) lowerSymbol(file *repomap.FileInfo, sym repomap.Symbol, snippet []string, relsByLine map[int][]repomap.Relation) FunctionSummary {
	symbolKey := repomap.SymbolKey(&sym)
	summary := FunctionSummary{
		SymbolKey:  symbolKey,
		SymbolName: sym.Name,
		File:       file.RelPath,
		Language:   file.Language,
		LineStart:  sym.Line,
		LineEnd:    sym.EndLine,
		CallSites:  make(map[string]int),
	}

	for i, rawLine := range snippet {
		lineNo := sym.Line + i
		line := strings.TrimSpace(rawLine)
		if line == "" || isCommentLine(file.Language, line) {
			continue
		}

		if guard := detectGuard(file.Language, line); guard != "" {
			summary.Guards = append(summary.Guards, GuardExpr{
				Expr:      guard,
				Source:    file.RelPath,
				LineStart: lineNo,
				LineEnd:   lineNo,
			})
			summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
				types.EvidenceConditional,
				symbolKey,
				"guards",
				guard,
				"",
				file.RelPath,
				"",
				lineNo,
				lineNo,
				0.8,
				"dataflow.lowerer."+file.Language,
				fmt.Sprintf("`%s` line %d guards execution IF %s", symbolKey, lineNo, guard),
				types.AnchorCondition,
				firstIdentifier(guard),
			))
		}

		for _, lit := range detectReturnValues(line) {
			if !contains(summary.Returns, lit) {
				summary.Returns = append(summary.Returns, lit)
			}
			if isConcreteValue(lit) && !contains(summary.Literals, lit) {
				summary.Literals = append(summary.Literals, lit)
				summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
					types.EvidenceConcrete,
					symbolKey,
					"returns",
					lit,
					"",
					file.RelPath,
					"",
					lineNo,
					lineNo,
					0.92,
					"dataflow.lowerer."+file.Language,
					fmt.Sprintf("`%s` line %d returns %s", symbolKey, lineNo, lit),
					types.AnchorReturn,
					firstIdentifier(lit),
				))
			}
		}

		for _, key := range detectConfigKeys(line) {
			if !contains(summary.ReadConfigKeys, key) {
				summary.ReadConfigKeys = append(summary.ReadConfigKeys, key)
				summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
					types.EvidenceMechanism,
					symbolKey,
					"reads_config",
					key,
					"",
					file.RelPath,
					"",
					lineNo,
					lineNo,
					0.85,
					"dataflow.lowerer."+file.Language,
					fmt.Sprintf("`%s` line %d reads config key %q", symbolKey, lineNo, key),
					types.AnchorCall,
					key,
				))
			}
		}

		for _, constructed := range detectConstructedTypes(line) {
			if !contains(summary.ConstructedTypes, constructed) {
				summary.ConstructedTypes = append(summary.ConstructedTypes, constructed)
			}
		}

		for _, field := range detectFieldWrites(line) {
			if !contains(summary.WrittenFields, field) {
				summary.WrittenFields = append(summary.WrittenFields, field)
				summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
					types.EvidenceRelationship,
					symbolKey,
					"writes_field",
					field,
					"",
					file.RelPath,
					"",
					lineNo,
					lineNo,
					0.78,
					"dataflow.lowerer."+file.Language,
					fmt.Sprintf("`%s` line %d writes field `%s`", symbolKey, lineNo, field),
					types.AnchorAssignment,
					field,
				))
			}
		}
		for _, field := range detectFieldReads(line) {
			if !contains(summary.ReadFields, field) {
				summary.ReadFields = append(summary.ReadFields, field)
				summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
					types.EvidenceRelationship,
					symbolKey,
					"reads_field",
					field,
					"",
					file.RelPath,
					"",
					lineNo,
					lineNo,
					0.75,
					"dataflow.lowerer."+file.Language,
					fmt.Sprintf("`%s` line %d reads field `%s`", symbolKey, lineNo, field),
					types.AnchorAssignment,
					field,
				))
			}
		}
		for _, alias := range detectAliases(line) {
			if !contains(summary.AliasTargets, alias) {
				summary.AliasTargets = append(summary.AliasTargets, alias)
			}
		}
		if reason := detectUnknownEffect(file.Language, line); reason != "" {
			summary.HasUnknownEffect = true
			if summary.UnknownReason == "" {
				summary.UnknownReason = reason
			}
		}

		for _, rel := range relsByLine[lineNo] {
			if rel.Kind != "call" {
				continue
			}
			if !contains(summary.Calls, rel.To) {
				summary.Calls = append(summary.Calls, rel.To)
			}
			summary.CallSites[rel.To] = lineNo
			summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
				types.EvidenceRelationship,
				symbolKey,
				"calls",
				rel.To,
				"",
				file.RelPath,
				"",
				lineNo,
				lineNo,
				0.82,
				"dataflow.lowerer."+file.Language,
				fmt.Sprintf("`%s` line %d calls `%s`", symbolKey, lineNo, rel.To),
				types.AnchorCall,
				callerLeafName(rel.To),
			))
		}
	}

	if summary.HasUnknownEffect {
		summary.ProducerEvidence = append(summary.ProducerEvidence, newEvidenceItem(
			types.EvidenceUnresolved,
			symbolKey,
			"unknown_effect",
			summary.UnknownReason,
			"",
			file.RelPath,
			"",
			sym.Line,
			sym.Line,
			0.45,
			"dataflow.lowerer."+file.Language,
			fmt.Sprintf("`%s` has an unresolved effect: %s", symbolKey, summary.UnknownReason),
			types.AnchorDefinition,
			sym.Name,
		))
	}

	return summary
}

func newLowererRegistry() map[string]Lowerer {
	registry := map[string]Lowerer{}
	for _, lang := range []string{
		repomap.LangGo,
		repomap.LangPython,
		repomap.LangJavaScript,
		repomap.LangTypeScript,
		repomap.LangArkTS,
		repomap.LangCangjie,
		repomap.LangJava,
		repomap.LangKotlin,
		repomap.LangRust,
		repomap.LangRuby,
		repomap.LangSwift,
		repomap.LangLua,
		repomap.LangProto,
		repomap.LangC,
		repomap.LangCpp,
	} {
		registry[lang] = genericLowerer{lang: lang}
	}
	return registry
}

func isCommentLine(lang, line string) bool {
	switch lang {
	case repomap.LangPython, repomap.LangRuby:
		return strings.HasPrefix(line, "#")
	case repomap.LangLua:
		return strings.HasPrefix(line, "--")
	default:
		return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
	}
}

func detectGuard(lang, line string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{
		"if ", "if(",
		"else if ", "else if(",
		"elseif ", "elseif(",
		"elif ",
		"unless ", "unless(",
		"guard ",
		"switch ", "switch(",
		"case ",
		"when ", "when(",
		"match ",
		"until ", "until(",
	} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(trimmed), "{"))
		}
	}
	if strings.Contains(trimmed, " ? ") && strings.Contains(trimmed, " : ") {
		return trimmed
	}
	return ""
}

func detectReturnValues(line string) []string {
	var values []string
	if idx := strings.Index(line, "return "); idx >= 0 {
		value := strings.TrimSpace(line[idx+len("return "):])
		value = strings.TrimSuffix(value, ";")
		value = strings.TrimSuffix(value, "}")
		if value != "" {
			values = append(values, value)
		}
	}
	if strings.Contains(line, "=>") {
		parts := strings.SplitN(line, "=>", 2)
		value := strings.TrimSpace(parts[1])
		value = strings.TrimSuffix(value, ";")
		value = strings.TrimSuffix(value, "{")
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func detectConfigKeys(line string) []string {
	var keys []string
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:getenv|lookupenv|env::var)\(\s*["']([^"']+)["']\s*\)`),
		regexp.MustCompile(`(?i)(?:config(?:\.get\w*)?|viper\.get(?:string|bool|int)?|settings(?:\.get\w*)?)\(\s*["']([^"']+)["']\s*\)`),
		regexp.MustCompile(`(?i)(?:\[\s*["']([^"']+)["']\s*\])`),
	}
	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 && !contains(keys, match[1]) {
				keys = append(keys, match[1])
			}
		}
	}
	return keys
}

func detectConstructedTypes(line string) []string {
	var out []string
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\bNew([A-Z][A-Za-z0-9_]*)\s*\(`),
		regexp.MustCompile(`\bnew\s+([A-Z][A-Za-z0-9_]*)\s*\(`),
		regexp.MustCompile(`&([A-Z][A-Za-z0-9_]*)\s*\{`),
		regexp.MustCompile(`\b([A-Z][A-Za-z0-9_]*)\s*\{`),
	}
	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 && !contains(out, match[1]) {
				out = append(out, match[1])
			}
		}
	}
	return out
}

func detectFieldWrites(line string) []string {
	var out []string
	re := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`)
	matches := re.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) > 2 {
			field := match[1] + "." + match[2]
			if !contains(out, field) {
				out = append(out, field)
			}
		}
	}
	return out
}

func detectFieldReads(line string) []string {
	var out []string
	re := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)`)
	matches := re.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		if len(match) > 2 {
			field := match[1] + "." + match[2]
			if !contains(out, field) {
				out = append(out, field)
			}
		}
	}
	return out
}

func detectAliases(line string) []string {
	var out []string
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*:=\s*([A-Za-z_][A-Za-z0-9_\.]*)`),
		regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*=\s*([A-Za-z_][A-Za-z0-9_\.]*)`),
	}
	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 2 {
				rhs := match[2]
				if rhs != "" && !strings.Contains(rhs, "(") && !contains(out, rhs) {
					out = append(out, rhs)
				}
			}
		}
	}
	return out
}

func detectUnknownEffect(lang, line string) string {
	trimmed := strings.TrimSpace(line)
	switch lang {
	case repomap.LangGo:
		if strings.Contains(trimmed, "reflect.") || strings.Contains(trimmed, "unsafe.") || strings.HasPrefix(trimmed, "defer ") {
			return "dynamic reflection/unsafe or deferred side effects"
		}
	case repomap.LangPython:
		if strings.Contains(trimmed, "getattr(") || strings.Contains(trimmed, "setattr(") || strings.Contains(trimmed, "importlib") {
			return "dynamic attribute or import usage"
		}
	case repomap.LangJavaScript, repomap.LangTypeScript, repomap.LangArkTS:
		if strings.Contains(trimmed, "require(") && strings.Contains(trimmed, "+") {
			return "dynamic require/import expression"
		}
		if strings.Contains(trimmed, "Reflect.") || strings.Contains(trimmed, "[") && strings.Contains(trimmed, "](") {
			return "dynamic property dispatch"
		}
	case repomap.LangKotlin, repomap.LangCangjie:
		if strings.Contains(trimmed, "Class.forName(") || strings.Contains(trimmed, "java.lang.reflect") || strings.Contains(trimmed, "::class") {
			return "dynamic reflection or class loading"
		}
	case repomap.LangRust:
		if strings.Contains(trimmed, "macro_rules!") || strings.Contains(trimmed, "unsafe ") {
			return "macro or unsafe block"
		}
	case repomap.LangRuby:
		if strings.Contains(trimmed, "send(") || strings.Contains(trimmed, "public_send(") || strings.Contains(trimmed, "const_get(") || strings.Contains(trimmed, "method_missing") {
			return "dynamic method or constant dispatch"
		}
	case repomap.LangSwift:
		if strings.Contains(trimmed, "Mirror(") || strings.Contains(trimmed, "NSClassFromString(") || strings.Contains(trimmed, "unsafeBitCast(") {
			return "runtime reflection or unsafe cast"
		}
	case repomap.LangLua:
		if strings.Contains(trimmed, "load(") || strings.Contains(trimmed, "loadfile(") || strings.Contains(trimmed, "loadstring(") || strings.Contains(trimmed, "_G[") {
			return "dynamic chunk loading or global dispatch"
		}
	case repomap.LangC, repomap.LangCpp:
		if strings.Contains(trimmed, "->") || strings.Contains(trimmed, "*") {
			return "pointer-heavy effect or aliasing"
		}
	}
	return ""
}

func isConcreteValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if _, err := strconv.ParseFloat(strings.Trim(value, "\"'`"), 64); err == nil {
		return true
	}
	switch strings.ToLower(strings.Trim(value, "\"'`")) {
	case "true", "false", "nil", "null", "none":
		return true
	}
	return strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") || strings.HasPrefix(value, "`")
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// firstIdentifier returns the leading identifier-like token of an
// expression (stops at the first non-identifier character). Used as
// the best-effort anchor symbol for guard / return expressions where
// the full span is available but the grounder needs a single
// identifier token to verify against line text.
func firstIdentifier(expr string) string {
	expr = strings.TrimSpace(expr)
	for i, r := range expr {
		isAlpha := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
		isDigit := r >= '0' && r <= '9'
		if isAlpha || (i > 0 && isDigit) {
			continue
		}
		if i == 0 {
			return ""
		}
		return expr[:i]
	}
	return expr
}

// callerLeafName returns the last dot-separated segment of a callee
// reference (e.g. "pkg.Func" → "Func", "Type.Method" → "Method") so
// the grounder's line-text identifier search hits the token most
// likely to appear verbatim at the call site.
func callerLeafName(ref string) string {
	ref = strings.TrimSpace(ref)
	if idx := strings.LastIndex(ref, "."); idx >= 0 && idx+1 < len(ref) {
		return ref[idx+1:]
	}
	return ref
}

// newEvidenceItem constructs a deterministic evidence item. anchorKind
// and anchorSymbol mirror the emit_evidence tool's required fields so
// downstream renderers and the grounder treat deterministic items and
// LLM-emitted items on equal footing: the line_start semantic (is it
// a call site, a definition, a guard, etc.) is explicit, and the
// identifier the grounder looks for at line_start is named. Without
// these, "X calls Y — source:line" rendered unlabeled and downstream
// consumers could mistake the call-site line for X's own definition.
func newEvidenceItem(kind types.EvidenceKind, subject, predicate, object, condition, source, ref string, lineStart, lineEnd int, confidence float64, producer, summary string, anchorKind types.AnchorKind, anchorSymbol string) types.EvidenceItem {
	item := types.EvidenceItem{
		Kind:         kind,
		Subject:      subject,
		Predicate:    predicate,
		Object:       object,
		Condition:    condition,
		Source:       filepath.ToSlash(source),
		EvidenceRef:  ref,
		LineStart:    lineStart,
		LineEnd:      lineEnd,
		Confidence:   confidence,
		Producer:     producer,
		Summary:      summary,
		AnchorKind:   anchorKind,
		AnchorSymbol: anchorSymbol,
	}
	item.ID = types.StableEvidenceID(kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
	return item
}
