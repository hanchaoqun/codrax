package repomap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cacheBaseDir is the root directory for repo map caches. When empty
// (the default), CacheDir falls back to os.UserCacheDir()/codrax/repomap.
// main.go can override it at startup via SetCacheDir so the operator
// can point all caches at a single tree (e.g. alongside logs/memory).
var cacheBaseDir string

// SetCacheDir overrides the base directory for repo map caches.
// An empty value restores the default (os.UserCacheDir). Called once
// from main.go before any tool runs.
func SetCacheDir(dir string) {
	cacheBaseDir = dir
}

// CacheDir returns the cache directory for a given repo root.
// Default: ~/.cache/codrax/repomap/<repo-slug>/
// Configured: <cache_dir>/repomap/<repo-slug>/
func CacheDir(repoRoot string) string {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		abs = repoRoot
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		// keep original
	}

	h := sha256.Sum256([]byte(abs))
	slug := filepath.Base(abs) + "-" + hex.EncodeToString(h[:4])

	base := cacheBaseDir
	if base == "" {
		base, err = os.UserCacheDir()
		if err != nil {
			base = filepath.Join(os.TempDir(), "codrax-cache")
		}
		base = filepath.Join(base, "codrax")
	}
	return filepath.Join(base, "repomap", slug)
}

const (
	cacheSymbolsFile   = "symbols.md"
	cacheRelationsFile = "relations.md"
	cacheMetaFile      = "meta.md"
	cacheHashesFile    = "hashes.md"
)

// SaveCache writes the graph index to markdown files in the cache directory.
func SaveCache(dir string, g *Graph) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Save file hashes for incremental update
	if err := saveHashes(dir, g); err != nil {
		return err
	}

	// Save symbols
	if err := saveSymbols(dir, g); err != nil {
		return err
	}

	// Save relations
	if err := saveRelations(dir, g); err != nil {
		return err
	}

	// Save metadata
	return saveMeta(dir, g)
}

func saveHashes(dir string, g *Graph) error {
	var b strings.Builder
	b.WriteString("# File Hashes\n\n")
	for _, fi := range g.Files {
		b.WriteString(fmt.Sprintf("%s\t%s\t%s\t%d\n", fi.RelPath, fi.Hash, fi.Language, fi.Size))
	}
	return os.WriteFile(filepath.Join(dir, cacheHashesFile), []byte(b.String()), 0o644)
}

func saveSymbols(dir string, g *Graph) error {
	var b strings.Builder
	b.WriteString("# Symbols Index\n\n")

	for _, fi := range g.Files {
		if len(fi.Symbols) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s", fi.RelPath))
		if fi.Package != "" {
			b.WriteString(fmt.Sprintf(" [pkg:%s]", fi.Package))
		}
		b.WriteString(fmt.Sprintf(" [hash:%s]\n\n", fi.Hash))

		for _, sym := range fi.Symbols {
			exported := ""
			if sym.Exported {
				exported = " [exported]"
			}
			recv := ""
			if sym.Receiver != "" {
				recv = fmt.Sprintf("(%s) ", sym.Receiver)
			} else if sym.Parent != "" {
				recv = fmt.Sprintf("%s.", sym.Parent)
			}
			sig := ""
			if sym.Signature != "" {
				sig = " `" + sym.Signature + "`"
			}
			doc := ""
			if sym.Doc != "" {
				doc = " — " + sym.Doc
			}
			b.WriteString(fmt.Sprintf("- `%s%s` %s :%d-%d%s%s%s\n",
				recv, sym.Name, sym.Kind, sym.Line, sym.EndLine, exported, sig, doc))
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dir, cacheSymbolsFile), []byte(b.String()), 0o644)
}

func saveRelations(dir string, g *Graph) error {
	var b strings.Builder
	b.WriteString("# Relations Index\n\n")

	// Group by kind
	groups := make(map[string][]Relation)
	for _, fi := range g.Files {
		for _, rel := range fi.Relations {
			groups[rel.Kind] = append(groups[rel.Kind], rel)
		}
		// import relations
		for _, imp := range fi.Imports {
			groups["import"] = append(groups["import"], Relation{
				Kind: "import",
				From: fi.RelPath,
				To:   imp.Path,
				File: fi.RelPath,
				Line: imp.Line,
			})
		}
	}

	kindOrder := []string{"import", "call", "type_usage", "reference", "inheritance", "embedding"}
	for _, kind := range kindOrder {
		rels, ok := groups[kind]
		if !ok || len(rels) == 0 {
			continue
		}
		heading := strings.ReplaceAll(kind, "_", " ")
		if len(heading) > 0 {
			heading = strings.ToUpper(heading[:1]) + heading[1:]
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", heading))

		// deduplicate for display
		seen := make(map[string]bool)
		for _, rel := range rels {
			key := rel.From + "→" + rel.To
			if seen[key] {
				continue
			}
			seen[key] = true
			b.WriteString(fmt.Sprintf("- %s → %s :%d\n", rel.From, rel.To, rel.Line))
		}
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(dir, cacheRelationsFile), []byte(b.String()), 0o644)
}

func saveMeta(dir string, g *Graph) error {
	var b strings.Builder
	b.WriteString("# Repo Map Metadata\n\n")
	b.WriteString(fmt.Sprintf("- **Scan time**: %s\n", g.Metadata.ScanTime.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Files**: %d\n", g.Metadata.FileCount))
	b.WriteString(fmt.Sprintf("- **Symbols**: %d\n", g.Metadata.SymbolCount))
	b.WriteString(fmt.Sprintf("- **Relations**: %d\n", g.Metadata.RelationCount))
	b.WriteString("\n## Languages\n\n")
	for lang, count := range g.Metadata.Languages {
		b.WriteString(fmt.Sprintf("- %s: %d\n", lang, count))
	}
	if len(g.Metadata.SpecialFiles) > 0 {
		b.WriteString("\n## Special Files\n\n")
		for _, f := range g.Metadata.SpecialFiles {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	return os.WriteFile(filepath.Join(dir, cacheMetaFile), []byte(b.String()), 0o644)
}

// LoadCachedHashes reads the file hashes from a previous scan.
// Returns a map of relPath → hash.
func LoadCachedHashes(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, cacheHashesFile))
	if err != nil {
		return nil
	}
	hashes := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) >= 2 {
			hashes[parts[0]] = parts[1]
		}
	}
	return hashes
}

// ChangedFiles returns the list of files that changed since the cache was written.
// Uses git diff if possible, otherwise compares hashes.
func ChangedFiles(repoRoot, cacheDir string, entries []FileEntry) []string {
	cachedHashes := LoadCachedHashes(cacheDir)
	if cachedHashes == nil {
		// no cache → everything is new
		all := make([]string, len(entries))
		for i, e := range entries {
			all[i] = e.RelPath
		}
		return all
	}

	// Try git-based detection first (fast)
	gitChanged := gitChangedSince(repoRoot, cacheDir)
	if gitChanged != nil {
		return gitChanged
	}

	// Fallback: compare hashes
	var changed []string
	currentFiles := make(map[string]bool)
	for _, entry := range entries {
		currentFiles[entry.RelPath] = true
		oldHash, exists := cachedHashes[entry.RelPath]
		if !exists {
			changed = append(changed, entry.RelPath) // new file
			continue
		}
		// read and hash to compare
		data, err := os.ReadFile(entry.AbsPath)
		if err != nil {
			changed = append(changed, entry.RelPath)
			continue
		}
		h := sha256.Sum256(data)
		newHash := hex.EncodeToString(h[:8])
		if newHash != oldHash {
			changed = append(changed, entry.RelPath)
		}
	}
	// deleted files count as changed for graph invalidation
	for path := range cachedHashes {
		if !currentFiles[path] {
			changed = append(changed, path)
		}
	}
	return changed
}

func gitChangedSince(repoRoot, cacheDir string) []string {
	metaPath := filepath.Join(cacheDir, cacheMetaFile)
	info, err := os.Stat(metaPath)
	if err != nil {
		return nil
	}

	// Use modification time of cache as reference
	since := info.ModTime().Format("2006-01-02T15:04:05")
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=ACMRD", "--since="+since)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var changed []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			changed = append(changed, line)
		}
	}
	return changed
}

// NeedsFullRescan returns true if the cache is missing or too stale.
func NeedsFullRescan(cacheDir string) bool {
	_, err := os.Stat(filepath.Join(cacheDir, cacheHashesFile))
	return err != nil
}
