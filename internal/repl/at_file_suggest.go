package repl

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// @-file completion for the native editor (TTY-3b polish item, design
// ledger docs/design/tty_single_stdin_owner_20260729.md §5): typing
// `@par` in the prompt suggests repo files whose name matches, so
// PIB-5c pins are discoverable instead of memorized. Scoring mirrors
// pi's ladder (basename exact > basename prefix > basename contains >
// path contains). The file index builds lazily ONCE per REPL session
// (bounded walk, hidden/vendor dirs skipped) — the per-keystroke cost
// is a filter over an in-memory slice, never a filesystem walk.

const (
	atFileIndexMaxEntries = 5000
	atFileSuggestMax      = 8
)

var atFileSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".codrax": true,
}

// buildAtFileIndex walks repoRoot collecting repo-relative file paths.
// Bounded: stops at atFileIndexMaxEntries (silent cap is fine here —
// completion is a convenience surface, the pin extractor still accepts
// any typed path).
func buildAtFileIndex(repoRoot string) []string {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	var files []string
	_ = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != repoRoot && (atFileSkipDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= atFileIndexMaxEntries {
			return filepath.SkipAll
		}
		return nil
	})
	return files
}

// atFileIndexProvider returns the lazily-built session file index.
func (r *REPL) atFileIndexProvider() []string {
	r.atFileIndexOnce.Do(func() {
		r.atFileIndex = buildAtFileIndex(r.repoRoot)
	})
	return r.atFileIndex
}

// atFileSuggestions completes the @token the cursor is finishing.
// Returns whole-value replacements (the native accept path swaps the
// entire editor value), capped and score-ordered.
func atFileSuggestions(value string, index []string) []slashSuggestion {
	if len(index) == 0 || !strings.Contains(value, "@") {
		return nil
	}
	start := strings.LastIndexAny(value, " \t") + 1
	token := value[start:]
	if !strings.HasPrefix(token, "@") || len(token) < 2 || strings.Contains(token, "@@") {
		return nil
	}
	query := strings.ToLower(token[1:])
	type scored struct {
		path  string
		score int
	}
	var hits []scored
	for _, path := range index {
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(path)
		score := 0
		switch {
		case base == query:
			score = 100
		case strings.HasPrefix(base, query):
			score = 80
		case strings.Contains(base, query):
			score = 50
		case strings.Contains(lower, query):
			score = 30
		default:
			continue
		}
		hits = append(hits, scored{path: path, score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		// Same rung: the shorter basename is the closer match
		// (query "dock" prefers dock.go over dock-notes.md).
		bi, bj := filepath.Base(hits[i].path), filepath.Base(hits[j].path)
		if len(bi) != len(bj) {
			return len(bi) < len(bj)
		}
		return hits[i].path < hits[j].path
	})
	if len(hits) > atFileSuggestMax {
		hits = hits[:atFileSuggestMax]
	}
	out := make([]slashSuggestion, 0, len(hits))
	for _, h := range hits {
		out = append(out, slashSuggestion{
			display: "@" + h.path,
			insert:  value[:start] + "@" + h.path,
			help:    "",
		})
	}
	return out
}

