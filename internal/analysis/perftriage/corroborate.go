package perftriage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/logtriage"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ClearedStall records a PerfStall whose File the corroborate gate
// cleared. Callers (emit_perf_trace) consume this list to stamp
// ctx.TypedDenials so the same negative-knowledge channel that
// log_triage uses also covers perf attachments.
//
// Mirrors logtriage.ClearedFrame for architectural symmetry.
type ClearedStall struct {
	OriginalFile string
	Symbol       string
	Kind         string // "io" / "lock" / "sync-rpc" / "native-call"
}

// CorroborateStallFiles is the perf_triage parallel of
// logtriage.frameFileCorroboratesFunc. Walks bundle.Stalls and
// clears any stall.File that does not contain the named symbol —
// preventing the LLM from grounding into a real-but-unrelated repo
// file just because the trace tag happened to name a path.
//
// Returns the list of cleared stalls (OriginalFile + Symbol + Kind)
// so callers can stamp them as TypedDenials.
//
// Reuses logtriage.ResolveFrameFile (path normalisation) +
// logtriage.loadFileIdentifiers (cache layer for "what identifiers
// live in file X?") so the gate is byte-identical to the log gate.
//
// Empty repoRoot disables the gate entirely (single-shot tests
// without a real repo): all stall.File survive untouched (callers
// rely on this for hand-crafted unit fixtures).
func CorroborateStallFiles(bundle *types.PerfBundle, repoRoot string) []ClearedStall {
	if bundle == nil || repoRoot == "" || len(bundle.Stalls) == 0 {
		return nil
	}
	repoBase := filepath.Base(filepath.Clean(repoRoot))
	fileIdentifiers := make(map[string]map[string]bool)

	var cleared []ClearedStall
	for i := range bundle.Stalls {
		s := &bundle.Stalls[i]
		if s.File == "" {
			continue
		}
		original := s.File
		// Normalise via the same StripBuildPathPrefix + ResolveFrameFile
		// the log gate uses.
		stripped := logtriage.StripBuildPathPrefix(s.File, repoBase, nil)
		resolved := logtriage.ResolveFrameFile(repoRoot, stripped)
		if resolved == "" {
			cleared = append(cleared, ClearedStall{OriginalFile: original, Symbol: s.Symbol, Kind: s.Kind})
			s.File = ""
			continue
		}
		s.File = resolved
		// Symbol corroboration: if stall has a Symbol, verify it
		// appears as an identifier in the resolved file. Empty Symbol
		// passes (the trace tag only carried a file path; we trust
		// the path resolution).
		if s.Symbol == "" {
			continue
		}
		// Stat-load identifier set (cached across calls within this
		// CorroborateStallFiles invocation).
		idents, ok := loadFileIdentifiersWrap(repoRoot, s.File, fileIdentifiers)
		if !ok || len(idents) == 0 {
			// Read failure → conservative pass (don't over-reject).
			continue
		}
		// idents map is lowercased by IdentifiersFromContent (matches
		// log gate's case-fold convention for cross-language tolerance)
		// — fold the symbol the same way before lookup.
		sym := strings.ToLower(s.Symbol)
		short := strings.ToLower(lastDotSegment(s.Symbol))
		if !idents[sym] && !idents[short] {
			cleared = append(cleared, ClearedStall{OriginalFile: original, Symbol: s.Symbol, Kind: s.Kind})
			s.File = "" // corroborate failed — same as log gate
		}
	}
	return cleared
}

// loadFileIdentifiersWrap reads `file` (resolved or repo-relative)
// and extracts identifier-shaped tokens via
// logtriage.IdentifiersFromContent (single shared regex / case-fold
// rule). Per-call cache so repeated stalls on the same file pay the
// I/O once.
func loadFileIdentifiersWrap(repoRoot, file string, cache map[string]map[string]bool) (map[string]bool, bool) {
	if existing, ok := cache[file]; ok {
		return existing, true
	}
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, file)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, false
	}
	idents := logtriage.IdentifiersFromContent(string(data))
	cache[file] = idents
	return idents, true
}

// lastDotSegment returns the suffix after the last dot, or s if no
// dot. Handles fully-qualified symbols like "com.example.Foo.bar"
// → "bar" (Java/Kotlin), or "module.Foo" → "Foo" (Python/JS).
func lastDotSegment(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return s
}
