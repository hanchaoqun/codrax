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
			}
		}
		for _, field := range detectFieldReads(line) {
			if !contains(summary.ReadFields, field) {
				summary.ReadFields = append(summary.ReadFields, field)
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
		repomap.LangJava,
		repomap.LangRust,
		repomap.LangC,
		repomap.LangCpp,
	} {
		registry[lang] = genericLowerer{lang: lang}
	}
	return registry
}

func isCommentLine(lang, line string) bool {
	switch lang {
	case repomap.LangPython:
		return strings.HasPrefix(line, "#")
	default:
		return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*")
	}
}

func detectGuard(lang, line string) string {
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []string{"if ", "if(", "else if ", "else if(", "switch ", "switch(", "case ", "elif ", "match "} {
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSuffix(strings.TrimSpace(trimmed), "{")
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
		regexp.MustCompile(`(?i)(?:config(?:\.get)?|viper\.get(?:string|bool|int)?|settings(?:\.get)?)\(\s*["']([^"']+)["']\s*\)`),
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
	case repomap.LangJavaScript, repomap.LangTypeScript:
		if strings.Contains(trimmed, "require(") && strings.Contains(trimmed, "+") {
			return "dynamic require/import expression"
		}
		if strings.Contains(trimmed, "Reflect.") || strings.Contains(trimmed, "[") && strings.Contains(trimmed, "](") {
			return "dynamic property dispatch"
		}
	case repomap.LangRust:
		if strings.Contains(trimmed, "macro_rules!") || strings.Contains(trimmed, "unsafe ") {
			return "macro or unsafe block"
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

func newEvidenceItem(kind types.EvidenceKind, subject, predicate, object, condition, source, ref string, lineStart, lineEnd int, confidence float64, producer, summary string) types.EvidenceItem {
	item := types.EvidenceItem{
		Kind:        kind,
		Subject:     subject,
		Predicate:   predicate,
		Object:      object,
		Condition:   condition,
		Source:      filepath.ToSlash(source),
		EvidenceRef: ref,
		LineStart:   lineStart,
		LineEnd:     lineEnd,
		Confidence:  confidence,
		Producer:    producer,
		Summary:     summary,
	}
	item.ID = types.StableEvidenceID(kind, item.Subject, item.Predicate, item.Object, item.Condition, item.Source, item.LineStart, item.LineEnd)
	return item
}
