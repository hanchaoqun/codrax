package index

import (
	"context"
	"os"
	"runtime"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// ParseFiles parses all entries in parallel and returns types.FileInfo results.
// Unparseable files (unsupported language, read errors) are included with
// basic metadata but no symbols.
func ParseFiles(entries []FileEntry, repoRoot string) []*types.FileInfo {
	return ParseFilesWithProgress(entries, repoRoot, nil)
}

// ParseFilesWithProgress is ParseFiles plus a best-effort completion
// callback. The callback receives parsed/total counts on the collector
// goroutine, so callers can throttle UI notifications without adding
// locks to the worker hot path.
func ParseFilesWithProgress(entries []FileEntry, repoRoot string, progress func(done, total int)) []*types.FileInfo {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 4
	}

	type result struct {
		idx  int
		info *types.FileInfo
	}

	jobs := make(chan int, len(entries))
	results := make(chan result, len(entries))

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				fi := parseOneFile(entries[idx])
				results <- result{idx: idx, info: fi}
			}
		}()
	}

	for i := range entries {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	infos := make([]*types.FileInfo, len(entries))
	done := 0
	for r := range results {
		infos[r.idx] = r.info
		done++
		if progress != nil {
			progress(done, len(entries))
		}
	}

	// filter nil (shouldn't happen but be safe)
	out := make([]*types.FileInfo, 0, len(infos))
	for _, fi := range infos {
		if fi != nil {
			out = append(out, fi)
		}
	}
	return out
}

// BasicFileInfo returns metadata for files that repomap deliberately
// indexes but does not parse into symbols. It still records the content
// hash so cache invalidation does not treat every unsupported file as
// changed forever.
func BasicFileInfo(entry FileEntry) *types.FileInfo {
	hash, _ := hashFile(entry.AbsPath)
	fi := &types.FileInfo{
		RelPath:  entry.RelPath,
		Language: entry.Language,
		Size:     entry.Size,
		Hash:     hash,
	}
	if ok, stype := IsSpecialFile(entry.RelPath); ok {
		fi.IsSpecial = true
		fi.SpecialType = stype
	}
	return fi
}

func parseOneFile(entry FileEntry) *types.FileInfo {
	source, err := os.ReadFile(entry.AbsPath)
	if err != nil {
		return &types.FileInfo{
			RelPath:  entry.RelPath,
			Language: entry.Language,
			Size:     entry.Size,
			Hash:     "",
		}
	}

	fi := &types.FileInfo{
		RelPath:  entry.RelPath,
		Language: entry.Language,
		Size:     entry.Size,
		Hash:     contentHash(source),
	}

	// Check for special files
	if ok, stype := IsSpecialFile(entry.RelPath); ok {
		fi.IsSpecial = true
		fi.SpecialType = stype
	}

	// HarmonyOS dual-language dispatch. Both paths own their own
	// tree-sitter / regex pipelines + tier assignment, so we
	// short-circuit before the generic tree-sitter block.
	//
	//   - ArkTS (LangArkTS): tree-sitter-typescript + ArkTS
	//     post-pass (see extract_arkts.go). Tier 1 = TS grammar
	//     parsed successfully; Tier 2 = regex-only salvage;
	//     Tier 3 = path-only.
	//   - Cangjie (LangCangjie): Go-native scanner (no tree-sitter
	//     grammar exists). Tier 1 = state-aware scanner; Tier 2 =
	//     raw regex salvage; Tier 3 = path-only.
	//
	// Tier + reason are recorded on FileInfo via recordFallback so
	// the retrieve.rank layer can discount lower-confidence parses
	// and the build log surfaces degradations (red line L-Fallback-1).
	switch entry.Language {
	case types.LangArkTS:
		pkg, syms, imps, rels, tier := extractArkTS(source, entry.RelPath)
		fi.Package = pkg
		fi.Symbols = syms
		fi.Imports = imps
		fi.Relations = rels
		if tier > 1 {
			recordFallback(fi, 1, tier, "arkts extractor downgraded")
		} else {
			fi.ParseTier = 1
		}
		return fi
	case types.LangCangjie:
		pkg, syms, imps, rels, tier := extractCangjie(source, entry.RelPath)
		fi.Package = pkg
		fi.Symbols = syms
		fi.Imports = imps
		fi.Relations = rels
		if tier > 1 {
			recordFallback(fi, 1, tier, "cangjie extractor downgraded")
		} else {
			fi.ParseTier = 1
		}
		return fi
	}

	// If language is unsupported, return basic info
	lang := types.GetSitterLanguage(entry.Language)
	if lang == nil {
		return fi
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil || tree == nil {
		return fi
	}
	root := tree.RootNode()
	if root == nil {
		return fi
	}

	switch entry.Language {
	case types.LangGo:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractGo(root, source, entry.RelPath)
	case types.LangPython:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractPython(root, source, entry.RelPath)
	case types.LangJavaScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, false)
	case types.LangTypeScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, true)
	case types.LangJava:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJava(root, source, entry.RelPath)
	case types.LangKotlin:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractKotlin(root, source, entry.RelPath)
	case types.LangRust:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractRust(root, source, entry.RelPath)
	case types.LangC, types.LangCpp:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractCCpp(root, source, entry.RelPath, entry.Language)
	case types.LangRuby:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractRuby(root, source, entry.RelPath)
	case types.LangSwift:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractSwift(root, source, entry.RelPath)
	case types.LangLua:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractLua(root, source, entry.RelPath)
	case types.LangProto:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractProto(root, source, entry.RelPath)
	}

	// Phase 6 stage 18 (2026-05-03) — populate per-line typed AST
	// node-shape features. Generic across languages: the walker
	// reads tree-sitter node Type() names and maps the closed set
	// (return_statement / call_expression / new_expression /
	// arrow_function / composite_literal / etc.) to typed
	// LineFeature enum values. Languages whose grammar uses
	// different node names skip silently — callers treat absence
	// as "no signal" rather than guessing via byte tokens.
	fi.LineFeatures = extractLineFeatures(root, source)

	// Phase 6 stage 21 (2026-05-03) — populate Symbol.ReturnTypeNames
	// for every function-like declaration across all languages.
	// Single post-pass walks the AST, finds every
	// function/method/lambda node via isFunctionNodeKind, extracts
	// its return-type names via extractReturnTypeNames, and matches
	// to the corresponding Symbol by line range. Languages whose
	// grammar uses different return-type field names degrade to
	// empty ReturnTypeNames (typed-only contract). Existing inline
	// wiring (goExtractFunc / goExtractMethod) is preserved — the
	// post-pass only sets the field when currently empty.
	backfillReturnTypeNames(root, source, fi.Symbols)

	return fi
}

// --- helpers ---

func nodeText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return node.Content(source)
}

func nodeLine(node *sitter.Node) int {
	if node == nil {
		return 0
	}
	return int(node.StartPoint().Row) + 1 // 1-based
}

func nodeEndLine(node *sitter.Node) int {
	if node == nil {
		return 0
	}
	return int(node.EndPoint().Row) + 1
}

// childByType finds the first direct child with the given type.
func childByType(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == nodeType {
			return ch
		}
	}
	return nil
}

// childrenByType returns all direct children with the given type.
func childrenByType(node *sitter.Node, nodeType string) []*sitter.Node {
	var out []*sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		if ch.Type() == nodeType {
			out = append(out, ch)
		}
	}
	return out
}

// prevSiblingComment returns the text of an immediately preceding comment.
func prevSiblingComment(node *sitter.Node, source []byte) string {
	prev := node.PrevSibling()
	if prev == nil {
		return ""
	}
	t := prev.Type()
	if t == "comment" || t == "line_comment" || t == "block_comment" {
		text := nodeText(prev, source)
		return firstLine(text)
	}
	return ""
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

// walkNamedChildren calls fn for every named child, recursively if recurse is true.
func walkNamedChildren(node *sitter.Node, recurse bool, fn func(*sitter.Node)) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		ch := node.NamedChild(i)
		fn(ch)
		if recurse {
			walkNamedChildren(ch, true, fn)
		}
	}
}
