package normalizer

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// buildAliasGraph emits three kinds of edges:
//
//  1. surface-variant: two different source texts with the same
//     canonical ID → the non-canonical one becomes an alias (relation
//     "synonym"). Example: "TaskList" survives as canonical, "task_list"
//     becomes an alias to "code:tasklist".
//
//  2. translation: a Chinese concept whose rule-table translation
//     exists in the canonical set → alias (relation "translation")
//     from the zh surface to the en canonical ID.
//
//  3. english-plural: an English concept ending in "s" whose singular
//     form also appears in the canonical set → alias (relation
//     "synonym"). Keeps the longer form canonical so the term that
//     actually appeared in the question drives surface substitution.
//
// Edges are emitted in a deterministic order (canonical-ID sorted,
// then source-text sorted) so tests and diff-based eval stay stable.
func buildAliasGraph(surfaces []surface, index map[string]string) []types.TermAlias {
	if len(index) == 0 {
		return nil
	}
	canonicalSurfaces := make(map[string]string, len(index)) // ID → first surface
	for _, s := range surfaces {
		id, ok := index[s.text]
		if !ok {
			continue
		}
		if _, exists := canonicalSurfaces[id]; !exists {
			canonicalSurfaces[id] = s.text
		}
	}

	seen := make(map[string]bool) // "source→target" key dedup
	var out []types.TermAlias
	emit := func(src, target, relation string, conf float32) {
		if src == "" || target == "" {
			return
		}
		key := src + "→" + target + ":" + relation
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, types.TermAlias{
			Source:     src,
			Target:     target,
			Relation:   relation,
			Confidence: conf,
		})
	}

	// (1) surface variants
	for _, s := range surfaces {
		id, ok := index[s.text]
		if !ok {
			continue
		}
		if canonicalSurfaces[id] != s.text {
			emit(s.text, id, "synonym", 0.9)
		}
	}

	// (2) zh→en translations
	for _, s := range surfaces {
		if s.kind != kindHan {
			continue
		}
		en, ok := ChineseTranslation(s.text)
		if !ok {
			continue
		}
		targetID := "en:" + en
		if _, exists := canonicalSurfaces[targetID]; exists {
			emit(s.text, targetID, "translation", 0.85)
			continue
		}
		// Also try upgrading to a code symbol if the English surface
		// happens to land on one (e.g. "探索器" → "explorer" which
		// might exist as code:explorer from a CamelCase match).
		codeID := "code:" + en
		if _, exists := canonicalSurfaces[codeID]; exists {
			emit(s.text, codeID, "translation", 0.85)
		}
	}

	// (3) english plural collapse
	for _, s := range surfaces {
		if s.kind != kindEnWord {
			continue
		}
		lower := strings.ToLower(s.text)
		if !strings.HasSuffix(lower, "s") || len(lower) < 5 {
			continue
		}
		singular := lower[:len(lower)-1]
		singularID := "en:" + singular
		if _, exists := canonicalSurfaces[singularID]; exists {
			emit(s.text, singularID, "synonym", 0.75)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		return out[i].Source < out[j].Source
	})
	return out
}
