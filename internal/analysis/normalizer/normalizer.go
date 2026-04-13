package normalizer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

type extractedTerm struct {
	Surface string
	Kind    string
	Lang    string
	POS     string
	Domain  string
}

var (
	reCodeSymbol = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)
	reConfigKey  = regexp.MustCompile(`\b[a-z][a-z0-9_]*(?:\.[a-z0-9_]+)+\b`)
	reCommand    = regexp.MustCompile("`([^`]+)`|\\b(?:go|make|npm|yarn|python|bash|sh)\\s+[\\w./:=-]+(?:\\s+[\\w./:=-]+)*")
)

func BuildTermGraph(request string) types.TermGraph {
	request = stripConversationPrefix(strings.TrimSpace(request))
	raw := extractTerms(request)
	return canonicalizeAndLink(raw)
}

func stripConversationPrefix(s string) string {
	const marker = "## Current request\n"
	if i := strings.Index(s, marker); i >= 0 {
		return strings.TrimSpace(s[i+len(marker):])
	}
	return s
}

func extractTerms(s string) []extractedTerm {
	terms := make([]extractedTerm, 0, 32)
	seen := make(map[string]bool)
	add := func(t extractedTerm) {
		key := t.Surface + "|" + t.Kind
		if seen[key] || strings.TrimSpace(t.Surface) == "" {
			return
		}
		seen[key] = true
		terms = append(terms, t)
	}

	for _, m := range reConfigKey.FindAllString(s, -1) {
		add(extractedTerm{Surface: m, Kind: "config_key", Lang: "en", POS: "noun", Domain: "config"})
	}
	for _, m := range reCommand.FindAllStringSubmatch(s, -1) {
		candidate := strings.TrimSpace(m[0])
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			candidate = strings.TrimSpace(m[1])
		}
		add(extractedTerm{Surface: candidate, Kind: "command_fragment", Lang: "en", POS: "verb", Domain: "cli"})
	}
	for _, m := range reCodeSymbol.FindAllString(s, -1) {
		if isStopToken(m) {
			continue
		}
		add(extractedTerm{Surface: m, Kind: "code_symbol", Lang: "en", POS: "noun", Domain: "code"})
	}
	for _, token := range splitCJKPhrases(s) {
		lang := detectLanguage(token)
		if lang == "zh" || lang == "ja" {
			add(extractedTerm{Surface: token, Kind: "nl_term", Lang: lang, POS: "noun", Domain: "request"})
		}
	}
	for _, token := range strings.Fields(s) {
		if detectLanguage(token) == "en" && len(token) > 2 && isAlphaLike(token) {
			add(extractedTerm{Surface: token, Kind: "nl_term", Lang: "en", POS: "noun", Domain: "request"})
		}
	}
	return terms
}

func canonicalizeAndLink(raw []extractedTerm) types.TermGraph {
	nodes := make([]types.TermNode, 0, len(raw))
	aliases := make([]types.TermAliasEdge, 0, len(raw))
	indexByCanonical := make(map[string]string)
	for i, t := range raw {
		canonical := canonicalToken(t.Surface, t.Kind)
		id := fmt.Sprintf("term-%d", i+1)
		nodes = append(nodes, types.TermNode{
			ID:        id,
			Surface:   t.Surface,
			Canonical: canonical,
			Language:  t.Lang,
			POS:       t.POS,
			Domain:    t.Domain,
			Kind:      t.Kind,
		})
		if root, ok := indexByCanonical[canonical]; ok {
			aliases = append(aliases, types.TermAliasEdge{From: id, To: root, Relation: "alias_of", Confidence: aliasConfidence(t)})
		} else {
			indexByCanonical[canonical] = id
		}
	}
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			if synonym(nodes[i].Canonical, nodes[j].Canonical) {
				aliases = append(aliases, types.TermAliasEdge{From: nodes[i].ID, To: nodes[j].ID, Relation: "related", Confidence: 0.62})
			}
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Canonical < nodes[j].Canonical })
	return types.TermGraph{Nodes: nodes, Aliases: aliases}
}

func canonicalToken(surface, kind string) string {
	s := strings.TrimSpace(surface)
	s = strings.Trim(s, "`\"'“”‘’")
	if kind == "command_fragment" {
		s = strings.Join(strings.Fields(s), " ")
		return strings.ToLower(s)
	}
	if kind == "config_key" || kind == "code_symbol" {
		return s
	}
	return strings.ToLower(s)
}

func aliasConfidence(t extractedTerm) float64 {
	switch t.Kind {
	case "code_symbol", "config_key":
		return 0.96
	case "command_fragment":
		return 0.88
	default:
		return 0.72
	}
}

func isStopToken(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "the", "and", "for", "with", "from", "into", "user", "request", "how", "what":
		return true
	}
	return false
}

func splitCJKPhrases(s string) []string {
	f := func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",.;:!?，。；：！？()[]{}<>\"'`", r)
	}
	parts := strings.FieldsFunc(s, f)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if utf8.RuneCountInString(p) < 2 {
			continue
		}
		if hasCJK(p) {
			out = append(out, p)
		}
	}
	return out
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func detectLanguage(s string) string {
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			return "ja"
		case unicode.In(r, unicode.Han):
			return "zh"
		case unicode.In(r, unicode.Latin):
			return "en"
		}
	}
	return "unknown"
}

func isAlphaLike(s string) bool {
	for _, r := range s {
		if !(unicode.IsLetter(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func synonym(a, b string) bool {
	pairs := [][2]string{{"注册", "register"}, {"設定", "config"}, {"配置", "config"}}
	for _, p := range pairs {
		if (strings.Contains(a, p[0]) && strings.Contains(b, p[1])) || (strings.Contains(a, p[1]) && strings.Contains(b, p[0])) {
			return true
		}
	}
	return false
}

func RetrievalTokens(g types.TermGraph) []string {
	set := map[string]bool{}
	for _, n := range g.Nodes {
		switch n.Kind {
		case "code_symbol", "config_key", "command_fragment":
			set[n.Canonical] = true
		case "nl_term":
			if n.Language != "unknown" && utf8.RuneCountInString(n.Canonical) >= 2 {
				set[n.Canonical] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
