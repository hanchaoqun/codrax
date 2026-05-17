package types

import (
	"path/filepath"
	"strings"
)

// RankIndexBuilder incrementally collects the query-independent
// ranking facts while BuildGraph is already walking files. It keeps
// transient symbol-to-file maps only until Finish, so the retained
// RankIndex is smaller than the temporary structures RankGraph used
// to allocate for every query.
type RankIndexBuilder struct {
	idx            *RankIndex
	symbolIDFile   map[SymbolID]string
	symbolNameFile map[string]string
	referrerPairs  map[rankReferrerPair]struct{}
	totalFiles     int
}

type rankReferrerPair struct {
	defFile string
	srcFile string
}

// NewRankIndexBuilder starts a two-stage rank index build. Call
// AddFileSymbols after symbol IDs have been derived, then
// AddFileRelations after import edges and resolver indexes are ready.
func NewRankIndexBuilder(totalFiles int) *RankIndexBuilder {
	if totalFiles < 0 {
		totalFiles = 0
	}
	return &RankIndexBuilder{
		idx: &RankIndex{
			SymbolDefCount: make(map[string]int),
			CallCountByID:  make(map[SymbolID]int),
			RefCountByID:   make(map[SymbolID]int),
			CallCount:      make(map[string]int),
			RefCount:       make(map[string]int),
			FileFanout:     make(map[string]float64),
			Entrypoints:    make(map[string]bool),
		},
		symbolIDFile:   make(map[SymbolID]string),
		symbolNameFile: make(map[string]string),
		referrerPairs:  make(map[rankReferrerPair]struct{}),
		totalFiles:     totalFiles,
	}
}

// AddFileSymbols records symbol-definition ambiguity and temporary
// symbol→file ownership maps for later relation fan-out accounting.
func (b *RankIndexBuilder) AddFileSymbols(fi *FileInfo) {
	if b == nil || b.idx == nil || fi == nil {
		return
	}
	localNames := make(map[string]bool, len(fi.Symbols))
	for idx := range fi.Symbols {
		sym := &fi.Symbols[idx]
		if sym.Name == "" {
			continue
		}
		if !localNames[sym.Name] {
			b.idx.SymbolDefCount[sym.Name]++
			localNames[sym.Name] = true
		}
		if sym.ID != "" {
			b.symbolIDFile[sym.ID] = fi.RelPath
		}
		// Preserve the legacy unresolved-edge behaviour: the last
		// same-named definition in scan order owns name-keyed fan-out.
		b.symbolNameFile[sym.Name] = fi.RelPath
	}
}

// AddFileRelations records call/reference fan-in and distinct
// source-file fan-out after BuildGraph has populated MethodIndex,
// SymbolByID, and import edges.
func (b *RankIndexBuilder) AddFileRelations(g *Graph, fi *FileInfo) {
	if b == nil || b.idx == nil || g == nil || fi == nil {
		return
	}
	for _, rel := range fi.Relations {
		var resolved *Symbol
		if rel.Kind == "call" {
			resolved = g.ResolveCallTarget(fi, rel)
		}
		switch rel.Kind {
		case "call":
			if resolved != nil && resolved.ID != "" {
				b.idx.CallCountByID[resolved.ID]++
			} else {
				b.idx.CallCount[rel.To]++
			}
		case "type_usage", "reference":
			b.idx.RefCount[rel.To]++
		}

		defFile := ""
		if resolved != nil && resolved.ID != "" {
			defFile = b.symbolIDFile[resolved.ID]
		} else if f, ok := b.symbolNameFile[rel.To]; ok {
			defFile = f
		}
		if defFile == "" || defFile == fi.RelPath {
			continue
		}
		pair := rankReferrerPair{defFile: defFile, srcFile: fi.RelPath}
		if _, exists := b.referrerPairs[pair]; exists {
			continue
		}
		b.referrerPairs[pair] = struct{}{}
		b.idx.FileFanout[defFile]++
	}
}

// Finish converts transient counters into their final representation
// and drops the build-only maps held by the builder.
func (b *RankIndexBuilder) Finish(g *Graph) *RankIndex {
	if b == nil || b.idx == nil {
		return &RankIndex{}
	}
	if b.totalFiles > 0 {
		total := float64(b.totalFiles)
		for path, count := range b.idx.FileFanout {
			b.idx.FileFanout[path] = count / total
		}
	}
	b.idx.Entrypoints = DetectEntrypoints(g)
	idx := b.idx
	b.idx = nil
	b.symbolIDFile = nil
	b.symbolNameFile = nil
	b.referrerPairs = nil
	return idx
}

// BuildRankIndex rebuilds the derived rank index from an already
// constructed graph. This is the compatibility path for tests and
// callers that assemble Graph values directly instead of going
// through index.BuildGraph.
func BuildRankIndex(g *Graph) *RankIndex {
	if g == nil {
		return (&RankIndexBuilder{}).Finish(nil)
	}
	b := NewRankIndexBuilder(len(g.Files))
	for _, fi := range g.Files {
		b.AddFileSymbols(fi)
	}
	for _, fi := range g.Files {
		b.AddFileRelations(g, fi)
	}
	return b.Finish(g)
}

// EnsureRankIndex returns g.RankIndex, rebuilding it when absent.
func EnsureRankIndex(g *Graph) *RankIndex {
	if g == nil {
		return BuildRankIndex(nil)
	}
	if g.RankIndex == nil {
		g.RankIndex = BuildRankIndex(g)
	}
	return g.RankIndex
}

// DetectEntrypoints returns query-independent entrypoint candidates
// from structural graph data and resolved reverse imports.
func DetectEntrypoints(g *Graph) map[string]bool {
	ep := make(map[string]bool)
	if g == nil {
		return ep
	}
	for _, fi := range g.Files {
		if fi == nil {
			continue
		}
		base := filepath.Base(fi.RelPath)
		switch {
		case fi.Language == LangGo && base == "main.go":
			ep[fi.RelPath] = true
		case fi.Language == LangGo && fi.Package == "main":
			ep[fi.RelPath] = true
		case fi.Language == LangPython && (base == "__main__.py" || base == "manage.py" || base == "app.py"):
			ep[fi.RelPath] = true
		case fi.Language == LangRust && base == "main.rs":
			ep[fi.RelPath] = true
		case fi.Language == LangJava && hasMainLikeSymbol(fi):
			ep[fi.RelPath] = true
		case fi.Language == LangKotlin &&
			(strings.EqualFold(base, "main.kt") || strings.EqualFold(base, "main.kts") || hasMainLikeSymbol(fi)):
			ep[fi.RelPath] = true
		case fi.Language == LangSwift &&
			(strings.EqualFold(base, "main.swift") || hasMainLikeSymbol(fi)):
			ep[fi.RelPath] = true
		case fi.Language == LangRuby &&
			(base == "main.rb" || strings.HasPrefix(filepath.ToSlash(fi.RelPath), "bin/") || strings.HasPrefix(filepath.ToSlash(fi.RelPath), "exe/")):
			ep[fi.RelPath] = true
		case fi.Language == LangLua &&
			(strings.EqualFold(base, "main.lua") || strings.EqualFold(base, "init.lua")):
			ep[fi.RelPath] = true
		case base == "index.js" || base == "index.ts" || base == "index.tsx":
			ep[fi.RelPath] = true
		case fi.Language == LangArkTS &&
			(base == "Index.ets" || base == "EntryAbility.ts" || base == "EntryAbility.ets"):
			ep[fi.RelPath] = true
		case fi.Language == LangCangjie && base == "main.cj":
			ep[fi.RelPath] = true
		case fi.IsSpecial && fi.SpecialType == "dockerfile":
			ep[fi.RelPath] = true
		}

		if len(g.ReverseImports[fi.RelPath]) == 0 && len(fi.Symbols) > 0 {
			for _, sym := range fi.Symbols {
				if sym.Exported {
					ep[fi.RelPath] = true
					break
				}
			}
		}
	}
	return ep
}

func hasMainLikeSymbol(fi *FileInfo) bool {
	if fi == nil {
		return false
	}
	for _, sym := range fi.Symbols {
		if sym.Name != "main" {
			continue
		}
		switch strings.ToLower(sym.Kind) {
		case "function", "method":
			return true
		}
	}
	return false
}
