package tool

import (
	"fmt"
	"sort"
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// source_inventory_language_census.go — repo-wide language census surfaced as
// SOFT navigation guidance on a source-inventory lens result.
//
// Problem it solves (RNE-C53 corner): the source-class universe attached by the
// reconciler is partitioned by SourcePathRole and limited to the lens query
// scopes. When a model derives its scopes from grep candidates, and the request
// keywords collide with the analyzer's OWN implementation (e.g. Cangjie's
// `extend` / `foreign func` / `public class` collide with codrax's Go parser),
// the model never scopes to the real source files, the scoped universe reports
// only the in-scope language, and the model concludes language-wide absence
// ("no .cj source files") while git-tracked sources of that language sit in the
// repo. The census walks every tracked file (scope-independent) so the model
// sees those languages and the exact files to open. Precise repomap signal,
// soft guidance: it never gates an answer.

// sourceInventoryRepoLanguageSampleLimit bounds how many representative file
// paths the repo-wide language census carries per language. It keeps the
// navigation hint useful without turning the census into a long file dump.
const sourceInventoryRepoLanguageSampleLimit = 4

// sourceInventoryRepoLanguageHintSampleLimit bounds how many sample paths the
// out-of-scope language hint shows per language so the navigation note stays
// compact even when the repo carries many auxiliary-language corpora.
const sourceInventoryRepoLanguageHintSampleLimit = 2

// sourceInventoryRepoLanguageHintLimit bounds how many out-of-scope languages
// the navigation hint lists, so a polyglot repo cannot turn the hint into a
// wall of text. Specialty (low-count) languages are listed first, since a
// narrow navigation scope is most likely to miss those entirely.
const sourceInventoryRepoLanguageHintLimit = 8

// sourceInventoryRepoLanguageCensus builds the repo-wide language census for a
// source-inventory observation. The COUNT is computed over every tracked source
// file (scope-independent), so the model sees that source files of a language
// exist even when its current navigation scope — typically a grep-derived
// candidate list — surfaced none. Each language also records InScope: whether
// AT LEAST ONE of its files is under the lens scopes, evaluated over the full
// file walk so a repo-dominant language whose sample paths fall outside a narrow
// scope is not misreported as scope-absent. Samples prefer out-of-scope paths so
// the hint hands the model openable targets. The result is soft guidance only —
// it is rendered as a navigation hint and never feeds an answer gate.
func sourceInventoryRepoLanguageCensus(ctx *types.BusContext, scopes []string) []types.SourceInventoryLanguageCount {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return nil
	}
	files, _ := sourceInventoryTrackedRepoFiles(ctx.RepoRoot)
	if len(files) == 0 {
		return nil
	}
	type langAcc struct {
		count   int
		inScope bool
		samples []string
	}
	byLang := map[string]*langAcc{}
	var order []string
	for _, rel := range files {
		rel = strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")
		if rel == "" {
			continue
		}
		lang := repotypes.DetectLanguage(rel)
		if lang == "" || !repotypes.IsSupportedReadLanguage(lang) {
			continue
		}
		acc := byLang[lang]
		if acc == nil {
			acc = &langAcc{}
			byLang[lang] = acc
			order = append(order, lang)
		}
		acc.count++
		if sourceInventoryFileInScopes(rel, scopes) {
			acc.inScope = true
		} else if len(acc.samples) < sourceInventoryRepoLanguageSampleLimit {
			// Prefer out-of-scope paths so the hint lists files the model
			// has not navigated to yet.
			acc.samples = append(acc.samples, rel)
		}
	}
	if len(byLang) == 0 {
		return nil
	}
	out := make([]types.SourceInventoryLanguageCount, 0, len(order))
	for _, lang := range order {
		acc := byLang[lang]
		out = append(out, types.SourceInventoryLanguageCount{
			Language: lang,
			Count:    acc.count,
			InScope:  acc.inScope,
			Samples:  acc.samples,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Language < out[j].Language
	})
	return out
}

// renderSourceInventoryRepoLanguageCensus renders the repo-wide language census
// as a navigation hint. The first line is the whole-repo language composition.
// It then flags any language with NO file under the current lens scopes (InScope
// == false) — the exact blind spot that lets a grep-derived, scope-limited pass
// conclude language-wide absence (e.g. "no .cj source files") while git-tracked
// sources of that language sit in the repo. Specialty (low-count) languages are
// listed first and the list is capped, since a repo-dominant language is almost
// always at least partially in scope and not the enumeration target. It is soft
// guidance: the model decides relevance and still owns the answer.
func renderSourceInventoryRepoLanguageCensus(observation types.SourceInventoryObservation) string {
	if len(observation.RepoLanguages) == 0 {
		return ""
	}
	counts := make([]string, 0, len(observation.RepoLanguages))
	var flagged []types.SourceInventoryLanguageCount
	for _, lc := range observation.RepoLanguages {
		if lc.Count <= 0 || strings.TrimSpace(lc.Language) == "" {
			continue
		}
		counts = append(counts, fmt.Sprintf("%s:%d", lc.Language, lc.Count))
		if !lc.InScope && len(lc.Samples) > 0 {
			flagged = append(flagged, lc)
		}
	}
	if len(counts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- repo_languages (whole-repo census, independent of the scopes above): %s\n", strings.Join(counts, ", "))

	if len(flagged) == 0 {
		return strings.TrimSpace(b.String())
	}
	// Specialty (low-count) languages first: a narrow scope is most likely to
	// miss those entirely, and they are the usual enumeration target.
	sort.SliceStable(flagged, func(i, j int) bool {
		if flagged[i].Count != flagged[j].Count {
			return flagged[i].Count < flagged[j].Count
		}
		return flagged[i].Language < flagged[j].Language
	})
	if len(flagged) > sourceInventoryRepoLanguageHintLimit {
		flagged = flagged[:sourceInventoryRepoLanguageHintLimit]
	}
	hints := make([]string, 0, len(flagged))
	for _, lc := range flagged {
		samples := lc.Samples
		if len(samples) > sourceInventoryRepoLanguageHintSampleLimit {
			samples = samples[:sourceInventoryRepoLanguageHintSampleLimit]
		}
		hints = append(hints, fmt.Sprintf("  - %s (%d file(s)): %s", lc.Language, lc.Count, strings.Join(samples, ", ")))
	}
	b.WriteString("- these languages have source files in the repo but NONE under the scopes above; if the request enumerates source constructs of one of them, open these files before concluding absence:\n")
	b.WriteString(strings.Join(hints, "\n"))
	b.WriteByte('\n')
	return strings.TrimSpace(b.String())
}
