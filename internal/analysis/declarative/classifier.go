// Package declarative identifies "declarative" source files — those
// whose primary content is a map literal, a const block, a registry
// table, or a routes manifest rather than function bodies. These
// files carry structural semantics (stage→agent→skill mappings,
// handler→route wiring, default value tables) that the keyword/IDF
// ranker traditionally misses because the file rarely contains the
// surrounding words like "default" or "configuration" — the meaning
// is in the structure of the map, not in lexical keywords.
//
// Session 11 consumers (2026-04-18):
//
//   - C0' ClassificationGrep (analyzer Round 2) uses Classify() to
//     gate which files the analyzer is allowed to peek at with
//     line-level grep when classification is ambiguous.
//
//   - R1 DeclarativeBoost (G6) uses BoostFor() to scale keyword_search
//     rank so a 22-line topology.go can beat a 6000-line explorer.go
//     for "which skill does the explorer agent use by default" style
//     questions.
//
// Both consumers share this package so the filename pattern list and
// small-file threshold stay in one place. TestDeclarativeFileSet_
// CoversKnownSurfaces pins the current codrax declarative surfaces
// so drift here is caught early.
package declarative

import (
	"path/filepath"
	"strings"
)

// Kind enumerates the declarative flavour of a file. Values are
// sorted by the boost weight BoostFor returns — higher-kind files
// are more likely to carry the answer to registration / mapping /
// routing questions, so the ranker and analyzer prioritise them.
type Kind int

const (
	KindNone     Kind = iota
	KindManifest      // package manifests, build descriptors
	KindWire          // DI wiring, RegisterDefaults, init glue
	KindEnum          // enum / constant tables
	KindSchema        // YAML/JSON/TOML config schemas
	KindRoutes        // HTTP / MCP / RPC route tables
	KindDefaults      // default value tables (e.g. skill defaults)
	KindRegistry      // registry wiring (tool/skill/subagent Register)
	KindTopology      // top-level stage/agent/skill topology maps
)

// String returns a short stable label for logs and the coverage test.
func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case KindManifest:
		return "manifest"
	case KindWire:
		return "wire"
	case KindEnum:
		return "enum"
	case KindSchema:
		return "schema"
	case KindRoutes:
		return "routes"
	case KindDefaults:
		return "defaults"
	case KindRegistry:
		return "registry"
	case KindTopology:
		return "topology"
	}
	return "unknown"
}

// Config controls Classifier behaviour. Callers pass either a
// config populated from codrax.yaml or the package default via
// DefaultConfig().
type Config struct {
	// FilenamePatterns is the case-insensitive substring list
	// matched against the base filename (without directory). Empty
	// → use DefaultFilenamePatterns(). The order here also seeds
	// the Kind preference when multiple patterns match; see
	// kindByFilename.
	FilenamePatterns []string

	// MaxLinesForSmall is the line-count ceiling below which a
	// file is eligible for the "small declarative" branch even
	// without a filename-pattern match. Default: 60.
	MaxLinesForSmall int

	// LiteralBlockRatio is the minimum fraction of top-level
	// declarations that must be composite literals (map/slice/struct
	// with `= {...}` bodies) for the small-file branch to classify
	// the file as declarative. Computed heuristically from the raw
	// source bytes — we do NOT parse a full AST here because we
	// want the check to be cheap enough to run in the explorer's
	// scoring hot path (one keyword_search can score hundreds of
	// files). Default: 0.60.
	LiteralBlockRatio float64
}

// DefaultFilenamePatterns is the canonical declarative-filename
// word list. Shared with codrax.yaml's `declarative_filename_patterns`
// so operators can override without forking.
func DefaultFilenamePatterns() []string {
	return []string{
		"topology",
		"registry",
		"defaults",
		"routes",
		"wire",
		"init",
		"manifest",
		"schema",
		"enum",
	}
}

// DefaultConfig returns the package default Classifier config. Safe
// to use when no codrax.yaml is loaded.
func DefaultConfig() Config {
	return Config{
		FilenamePatterns:  DefaultFilenamePatterns(),
		MaxLinesForSmall:  60,
		LiteralBlockRatio: 0.60,
	}
}

// Classifier decides whether a given file is declarative. It is
// stateless and safe to reuse across goroutines; all configuration
// is captured at construction time.
type Classifier struct {
	cfg Config
}

// New constructs a Classifier from cfg. Missing fields fall back to
// DefaultConfig values so partial overrides are safe.
func New(cfg Config) *Classifier {
	if len(cfg.FilenamePatterns) == 0 {
		cfg.FilenamePatterns = DefaultFilenamePatterns()
	}
	if cfg.MaxLinesForSmall <= 0 {
		cfg.MaxLinesForSmall = 60
	}
	if cfg.LiteralBlockRatio <= 0 {
		cfg.LiteralBlockRatio = 0.60
	}
	return &Classifier{cfg: cfg}
}

// ClassifyPath runs the cheap filename-only classification path.
// Callers without file contents (e.g. the keyword_search ranker
// working off a repo_map scored list) use this to decide whether
// to try to read the file. Returns KindNone when no filename
// pattern matches.
func (c *Classifier) ClassifyPath(path string) (Kind, float64) {
	if c == nil || path == "" {
		return KindNone, 0
	}
	base := strings.ToLower(filepath.Base(path))
	// Strip extension for matching (topology.go → "topology").
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if kind := kindByFilename(stem, c.cfg.FilenamePatterns); kind != KindNone {
		return kind, 1.0
	}
	// YAML / JSON / TOML are always declarative by extension.
	switch ext {
	case ".yaml", ".yml":
		return KindSchema, 0.9
	case ".json", ".toml":
		return KindSchema, 0.8
	}
	return KindNone, 0
}

// Classify runs the full classification path, using both the
// filename and the file content. The content check kicks in only
// when ClassifyPath returns KindNone — for paths that already
// matched, the filename classification is authoritative.
//
// The content heuristic computes the ratio of composite-literal
// bytes (Go `= {...}` blocks) to total non-blank bytes; when the
// ratio exceeds LiteralBlockRatio and total lines ≤ MaxLinesForSmall,
// the file is classified as KindTopology with medium confidence
// (0.70). The threshold is intentionally conservative — a false
// positive boost on a regular file is worse than a false negative
// on a rarely-named wiring file.
func (c *Classifier) Classify(path string, content []byte) (Kind, float64) {
	if kind, conf := c.ClassifyPath(path); kind != KindNone {
		return kind, conf
	}
	if len(content) == 0 {
		return KindNone, 0
	}
	lines := countLines(content)
	if lines == 0 || lines > c.cfg.MaxLinesForSmall {
		return KindNone, 0
	}
	ratio := literalBlockRatio(content)
	if ratio >= c.cfg.LiteralBlockRatio {
		return KindTopology, 0.70
	}
	return KindNone, 0
}

// BoostFor returns the additive ranker bonus for kind. Pure lookup,
// no state. Used by R1 DeclarativeBoost (G6) to weight keyword_search
// scores; the numbers are calibrated against the base score range
// (0..100) so a top-kind file can move ~20 points up — enough to
// break ties but not enough to dominate keyword-relevant files.
func (c *Classifier) BoostFor(kind Kind) float64 {
	switch kind {
	case KindTopology:
		return 15
	case KindRegistry:
		return 12
	case KindDefaults:
		return 10
	case KindRoutes:
		return 8
	case KindSchema:
		return 7
	case KindEnum:
		return 7
	case KindWire:
		return 6
	case KindManifest:
		return 6
	}
	return 0
}

// kindByFilename walks the pattern list and returns the strongest
// match. Patterns are tested as case-insensitive substrings of the
// (already-lowercased) stem.
func kindByFilename(stem string, patterns []string) Kind {
	// Longest pattern first so "defaults" beats "default" if both
	// appear in the config. Our default list has no overlap so the
	// ordering is mostly for operator-override safety.
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		p := strings.ToLower(pat)
		if !strings.Contains(stem, p) {
			continue
		}
		switch p {
		case "topology":
			return KindTopology
		case "registry":
			return KindRegistry
		case "defaults":
			return KindDefaults
		case "routes":
			return KindRoutes
		case "wire":
			return KindWire
		case "init":
			return KindWire
		case "manifest":
			return KindManifest
		case "schema":
			return KindSchema
		case "enum":
			return KindEnum
		default:
			// Custom pattern from codrax.yaml — assume registry-class.
			return KindRegistry
		}
	}
	return KindNone
}

// countLines returns the number of \n-delimited lines in b. Files
// without a trailing newline still count correctly (the final
// partial line is included).
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// literalBlockRatio approximates the fraction of source bytes that
// live inside composite-literal blocks. We scan for top-level `{`
// openers that are NOT function bodies — function bodies begin with
// `){` (after a signature's closing paren), so any other opener
// (`= {`, `struct {`, `}{`, `map[K]V{`, `[]T{`) is assumed to start
// a literal block. This is intentionally cheap (no AST) because the
// classifier runs in the file-scoring inner loop; it overestimates
// on blocks that use `{}` for pure control flow but that's fine —
// such files also have few lines and the MaxLinesForSmall ceiling
// drops them long before the ratio check fires.
//
// The "not )" rule correctly excludes func body opening braces in
// all common Go forms:
//
//	func Foo() { ... }        // ) → func body
//	func (x *T) Bar() { ... } // ) → func body
//	if a > b { ... }           // b → NOT a func body (literal for
//	                           //   our purposes — short files with
//	                           //   if/for blocks tend to be declarative
//	                           //   helpers like `init()`)
//
// The false positive on if/for is acceptable because (a) the
// MaxLinesForSmall gate bounds how far the classifier looks, (b)
// an `init()`-heavy 40-line file IS declarative for our purposes
// (register, wire, default), and (c) the only cost of a false
// positive is an extra boost in the ranker — not a content change.
func literalBlockRatio(content []byte) float64 {
	total := 0
	inside := 0
	depth := 0
	for i, c := range content {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			total++
		}
		switch c {
		case '{':
			prev := lookbackMeaningful(content, i)
			if prev == ')' {
				// Function body — do not enter literal mode.
				continue
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		}
		if depth > 0 {
			inside++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(inside) / float64(total)
}

// lookbackMeaningful scans backwards from idx-1 past whitespace and
// newlines until it hits the first non-whitespace rune and returns
// it. Returns 0 when the scan walks off the start of buf.
func lookbackMeaningful(buf []byte, idx int) byte {
	for j := idx - 1; j >= 0; j-- {
		c := buf[j]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c
	}
	return 0
}
