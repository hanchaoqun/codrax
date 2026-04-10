package repomap

import "time"

// Symbol represents an extracted code symbol.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // package, function, method, type, interface, class, struct, enum, const, var, field, trait
	File     string `json:"file"` // relative path
	Line     int    `json:"line"` // 1-based
	EndLine  int    `json:"end_line"`
	Exported bool   `json:"exported"`
	Receiver string `json:"receiver,omitempty"` // for methods
	Signature string `json:"signature,omitempty"`
	Doc      string `json:"doc,omitempty"` // first line of doc comment
	Parent   string `json:"parent,omitempty"` // containing type/class
}

// Import represents an import/include/require statement.
type Import struct {
	Raw   string `json:"raw"`   // raw import string as written
	Path  string `json:"path"`  // cleaned path
	Alias string `json:"alias,omitempty"`
	File  string `json:"file"`  // which file contains this import
	Line  int    `json:"line"`
}

// Relation represents a relationship between code entities.
type Relation struct {
	Kind string `json:"kind"` // import, call, reference, type_usage, inheritance, embedding
	From string `json:"from"` // file or file:symbol
	To   string `json:"to"`   // file or file:symbol
	File string `json:"file"` // file where the relation is observed
	Line int    `json:"line"`
}

// FileInfo holds all extracted data for a single source file.
type FileInfo struct {
	RelPath     string     `json:"rel_path"`
	Language    string     `json:"language"`
	Package     string     `json:"package,omitempty"`
	Size        int64      `json:"size"`
	Hash        string     `json:"hash"` // content hash for cache invalidation
	Symbols     []Symbol   `json:"symbols,omitempty"`
	Imports     []Import   `json:"imports,omitempty"`
	Relations   []Relation `json:"relations,omitempty"`
	IsSpecial   bool       `json:"is_special,omitempty"`
	SpecialType string     `json:"special_type,omitempty"` // build_config, dockerfile, ci, etc.
}

// Graph is the complete repository index.
type Graph struct {
	Root        string                `json:"root"`
	Files       []*FileInfo           `json:"-"`
	FileIndex   map[string]*FileInfo  `json:"-"` // rel path → FileInfo
	SymbolDefs  map[string][]*Symbol  `json:"-"` // symbol name → all definitions
	ImportGraph map[string][]string   `json:"-"` // file → imported file paths
	ReverseImports map[string][]string `json:"-"` // file → files that import it
	Scores      map[string]float64    `json:"-"` // key → importance score
	Metadata    Metadata              `json:"metadata"`
}

// Metadata holds scan-level statistics.
type Metadata struct {
	ScanTime     time.Time      `json:"scan_time"`
	FileCount    int            `json:"file_count"`
	SymbolCount  int            `json:"symbol_count"`
	RelationCount int           `json:"relation_count"`
	Languages    map[string]int `json:"languages"`    // language → file count
	SpecialFiles []string       `json:"special_files"` // notable files (go.mod, etc.)
}

// ViewParams controls what a view generator produces.
type ViewParams struct {
	Query      string // search query for task_map
	TargetFile string // file path for edit_impact
	EntryPoint string // symbol or file for call_path
	TopN       int    // max items to show (0 = default)
}
