package repomap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
)

// ParseFiles parses all entries in parallel and returns FileInfo results.
// Unparseable files (unsupported language, read errors) are included with
// basic metadata but no symbols.
func ParseFiles(entries []FileEntry, repoRoot string) []*FileInfo {
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 4
	}

	type result struct {
		idx  int
		info *FileInfo
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

	infos := make([]*FileInfo, len(entries))
	for r := range results {
		infos[r.idx] = r.info
	}

	// filter nil (shouldn't happen but be safe)
	out := make([]*FileInfo, 0, len(infos))
	for _, fi := range infos {
		if fi != nil {
			out = append(out, fi)
		}
	}
	return out
}

func parseOneFile(entry FileEntry) *FileInfo {
	source, err := os.ReadFile(entry.AbsPath)
	if err != nil {
		return &FileInfo{
			RelPath:  entry.RelPath,
			Language: entry.Language,
			Size:     entry.Size,
			Hash:     "",
		}
	}

	h := sha256.Sum256(source)
	hash := hex.EncodeToString(h[:8])

	fi := &FileInfo{
		RelPath:  entry.RelPath,
		Language: entry.Language,
		Size:     entry.Size,
		Hash:     hash,
	}

	// Check for special files
	if ok, stype := IsSpecialFile(entry.RelPath); ok {
		fi.IsSpecial = true
		fi.SpecialType = stype
	}

	// If language is unsupported, return basic info
	lang := GetSitterLanguage(entry.Language)
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
	case LangGo:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractGo(root, source, entry.RelPath)
	case LangPython:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractPython(root, source, entry.RelPath)
	case LangJavaScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, false)
	case LangTypeScript:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJS(root, source, entry.RelPath, true)
	case LangJava:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractJava(root, source, entry.RelPath)
	case LangRust:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractRust(root, source, entry.RelPath)
	case LangC, LangCpp:
		fi.Package, fi.Symbols, fi.Imports, fi.Relations = extractCCpp(root, source, entry.RelPath, entry.Language)
	}

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
