package types

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// codeOrConfigSourcePathExtensions is the canonical set of file
// extensions that flag a string as a likely source / declarative-
// config file reference. The union is:
//
//   - code source extensions (every key currently present in
//     internal/tool/repomap/types/lang.go :: extToLang) — kept in sync
//     manually because internal/types is the foundation layer and
//     cannot import the repomap subtree without inverting the package
//     graph;
//   - declarative config / data formats (yaml / yml / json / toml /
//     ini / xml / md) used as anchor surfaces in config-precedence and
//     architecture answers.
//
// Three former hardcoded copies in
// internal/types/evidence_surface_terms.go,
// internal/types/answer_support_plan.go, and
// internal/tool/emit_investigation_complete.go all redirected through
// IsCodeOrConfigPathExtension / HasCodeOrConfigPathSuffix so a new
// language ext only needs one update here.
var codeOrConfigSourcePathExtensions = map[string]bool{
	// code source extensions (mirrors repomap extToLang)
	".go":    true,
	".c":     true,
	".cc":    true,
	".cpp":   true,
	".cxx":   true,
	".h":     true,
	".hh":    true,
	".hpp":   true,
	".hxx":   true,
	".cj":    true,
	".cjo":   true,
	".ets":   true,
	".ts":    true,
	".tsx":   true,
	".js":    true,
	".jsx":   true,
	".mjs":   true,
	".java":  true,
	".kt":    true,
	".kts":   true,
	".py":    true,
	".pyi":   true,
	".rs":    true,
	".rb":    true,
	".php":   true,
	".swift": true,
	".lua":   true,
	".proto": true,
	".m":     true,
	".mm":    true,
	".cu":    true,
	".cuh":   true,
	// declarative config / data formats
	".yaml": true,
	".yml":  true,
	".json": true,
	".toml": true,
	".ini":  true,
	".xml":  true,
	".md":   true,
}

var runtimeArtifactPathExtensions = map[string]bool{
	".log":       true,
	".trace":     true,
	".systrace":  true,
	".htrace":    true,
	".atrace":    true,
	".ftrace":    true,
	".perfetto":  true,
	".perftrace": true,
}

// IsCodeOrConfigPathExtension reports whether ext (with leading dot,
// case-insensitive) is a known code source or declarative config file
// extension. Used by surface-term filters that already separated ext
// from the rest of the path.
func IsCodeOrConfigPathExtension(ext string) bool {
	if ext == "" {
		return false
	}
	return codeOrConfigSourcePathExtensions[strings.ToLower(ext)]
}

// HasCodeOrConfigPathSuffix reports whether s ends with any known
// code or config extension. Case-insensitive. Used by hint scanners
// that work on raw tokens without first parsing them as paths.
func HasCodeOrConfigPathSuffix(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	for ext := range codeOrConfigSourcePathExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// IsRuntimeArtifactPathExtension reports whether ext is a runtime observation
// artifact suffix. This helper intentionally lives in internal/types so source
// lane gates, prompt planners, and tool validators can make the same typed
// origin distinction without importing tool-layer grep helpers.
func IsRuntimeArtifactPathExtension(ext string) bool {
	if ext == "" {
		return false
	}
	return runtimeArtifactPathExtensions[strings.ToLower(ext)]
}

// Reserved blob-materialization basenames Codrax itself writes for attached
// runtime artifacts (context.AttachedLogBlobName / AttachedTraceBlobName and
// the trace-lane companions). They are SYSTEM-RESERVED spellings, not user
// prose: a citation/path naming one of them is a runtime artifact by
// construction. Single authority for every consumer — the path-kind switch
// below, the carve suffix list, and the typed citation spelling set in
// internal/tool (CPD #58: the tool-side set used to miss these, so blob-path
// pseudo-citations evaded the typed artifact lane).
const (
	AttachedLogBlobBasename     = "attached_log.txt"
	AttachedTraceBlobBasename   = "attached_trace.txt"
	AttachedHitraceBlobBasename = "attached_hitrace.txt"
	AttachedAtraceBlobBasename  = "attached_atrace.txt"
)

// ReservedRuntimeArtifactBlobBasenames returns the reserved blob basenames
// above. Callers must treat the result as read-only.
func ReservedRuntimeArtifactBlobBasenames() []string {
	return []string{
		AttachedLogBlobBasename,
		AttachedTraceBlobBasename,
		AttachedHitraceBlobBasename,
		AttachedAtraceBlobBasename,
	}
}

// RuntimeArtifactPathKind reports the coarse artifact family for a log/trace
// runtime artifact path. It is path-shape only; it must not be used to infer
// user intent from prose.
func RuntimeArtifactPathKind(s string) string {
	if s == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(s))
	base := lower
	if idx := strings.LastIndexAny(base, `/\`); idx >= 0 {
		base = base[idx+1:]
	}
	switch base {
	case AttachedLogBlobBasename:
		return "log"
	case AttachedTraceBlobBasename, AttachedHitraceBlobBasename, AttachedAtraceBlobBasename:
		return "trace"
	}
	if base == "perf.data" || strings.HasSuffix(lower, ".perf.data") {
		return "trace"
	}
	if strings.HasSuffix(lower, ".tracebundle.json") {
		return "trace"
	}
	if strings.HasSuffix(lower, ".log") && len(base) > len(".log") {
		return "log"
	}
	for ext := range runtimeArtifactPathExtensions {
		if strings.HasSuffix(lower, ext) && len(base) > len(ext) {
			return "trace"
		}
	}
	return ""
}

// LooksLikeRuntimeArtifactPath reports whether s names a log/trace/perfetto
// runtime artifact. It is path-shape only; it must not be used to infer user
// intent from prose.
func LooksLikeRuntimeArtifactPath(s string) bool {
	return RuntimeArtifactPathKind(s) != ""
}

// RuntimeArtifactPathInToken carves an embedded runtime-artifact path out of a
// single whitespace/punctuation-delimited token that may have CJK prose glued
// directly to it with no separator, e.g.
//
//	"分析/tmp/frame.systrace的卡顿" -> "/tmp/frame.systrace"
//
// It returns "" when the token does not contain a recognized log/trace
// artifact path. This is the gap behind the "by-name in the question" display
// regression: the request tokenizer treats CJK punctuation as a separator but
// not bare CJK ideographs, so a path written flush against Chinese prose stays
// fused to it and the suffix match in RuntimeArtifactPathKind fails.
//
// Legitimate paths that themselves contain CJK directory/file names are
// preserved: trimming only removes a LEADING or TRAILING run of CJK prose and
// only when the trimmed result is still a recognized artifact path, so a middle
// component like /tmp/中文/x.systrace is never split. When the glued tail is
// MIXED prose (CJK immediately after the extension, then more ASCII prose with
// no separator, e.g. "x.sys.ftrace里面这一帧Choreographer#doFrame"), the cut is
// anchored on the extension→CJK adjacency instead — see
// carveArtifactPathBeforeGluedProse. Still path-shape only; it must not be
// used to infer user intent from prose.
func RuntimeArtifactPathInToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if labeled := dropLeadingArtifactLabel(token); labeled != "" {
		return labeled
	}
	if RuntimeArtifactPathKind(token) != "" {
		if lead := dropLeadingArtifactProse(token); lead != token && RuntimeArtifactPathKind(lead) != "" {
			return lead
		}
		return token
	}
	trimmed := dropTrailingArtifactProse(token)
	if trimmed == token || RuntimeArtifactPathKind(trimmed) == "" {
		// The trailing walk strips a PURE CJK prose run only; it stops at the
		// first ASCII rune. A real question can glue MIXED prose after the
		// path — CJK first, then an ASCII term with no separator, e.g.
		// "record_trace.sys.ftrace里面这一帧Choreographer#doFrame" — and the
		// walk never reaches the extension. Fall back to cutting at the
		// precise extension→CJK-prose boundary inside the token.
		trimmed = carveArtifactPathBeforeGluedProse(token)
		if trimmed == "" {
			return ""
		}
	}
	if lead := dropLeadingArtifactProse(trimmed); lead != trimmed && RuntimeArtifactPathKind(lead) != "" {
		return lead
	}
	return trimmed
}

// carveArtifactPathBeforeGluedProse cuts a token right after a recognized
// runtime-artifact extension when the rune immediately following the extension
// is a CJK prose rune. This is the mixed-tail complement to
// dropTrailingArtifactProse: the boundary is anchored on the precise
// "<artifact extension><CJK rune>" adjacency, never on prose content, so it
// cannot fire on tokens without a recognized artifact suffix. Boundaries are
// tried rightmost-first (keeping CJK path components that sit between two
// artifact suffixes intact, mirroring the leading/trailing trim guarantees),
// falling back leftward when a candidate fails re-verification. A boundary is
// rejected when the remainder after it still looks like a path continuation
// (contains a path separator, or ends in a dot-extension) — carving
// "handler.trace" out of "handler.trace包/foo.go" would mint a phantom
// artifact from a source path. Returns "" when no boundary verifies.
func carveArtifactPathBeforeGluedProse(s string) string {
	// ASCII-only lowering keeps byte offsets aligned with s: full
	// strings.ToLower changes the byte length of runes like U+0130 İ or
	// U+212A K, which would skew every index after them. All recognized
	// artifact suffixes are pure ASCII, so ASCII folding is sufficient.
	lower := asciiLowerPreservingLength(s)
	var boundaries []int
	seen := map[int]bool{}
	for _, ext := range runtimeArtifactCarveSuffixes() {
		from := 0
		for {
			idx := strings.Index(lower[from:], ext)
			if idx < 0 {
				break
			}
			end := from + idx + len(ext)
			if end < len(s) && !seen[end] {
				if r, _ := utf8.DecodeRuneInString(s[end:]); isArtifactPathProseRune(r) {
					seen[end] = true
					boundaries = append(boundaries, end)
				}
			}
			from += idx + 1
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(boundaries)))
	for _, end := range boundaries {
		if end <= 0 {
			continue
		}
		if carvedRemainderLooksLikePathContinuation(s[end:]) {
			continue
		}
		candidate := s[:end]
		if RuntimeArtifactPathKind(candidate) != "" {
			return candidate
		}
		// Basename special cases (perf.data, attached_log.txt, …) only
		// verify once leading prose is stripped: "抓到perf.data" fails the
		// exact-basename check while "perf.data" passes it.
		if lead := dropLeadingArtifactProse(candidate); lead != candidate && RuntimeArtifactPathKind(lead) != "" {
			return lead
		}
	}
	return ""
}

// carvedRemainderLooksLikePathContinuation reports whether the text after a
// candidate carve boundary still reads as part of a path rather than glued
// prose: a path separator anywhere, or a trailing dot-extension (1-6 ASCII
// alphanumerics), means the artifact suffix sat on a directory or infix
// component of a longer non-artifact path ("handler.trace包/foo.go",
// "崩溃日志.log分析报告.md"). Prose glued after a real artifact path — CJK runs
// optionally mixed with ASCII terms like "里面这一帧Choreographer#doFrame" —
// contains neither.
func carvedRemainderLooksLikePathContinuation(remainder string) bool {
	if strings.ContainsAny(remainder, `/\`) {
		return true
	}
	idx := strings.LastIndex(remainder, ".")
	if idx < 0 || idx == len(remainder)-1 {
		return false
	}
	tail := remainder[idx+1:]
	if len(tail) > 6 {
		return false
	}
	for i := 0; i < len(tail); i++ {
		c := tail[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func asciiLowerPreservingLength(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

// runtimeArtifactCarveSuffixes lists the extension literals used to locate a
// glued prose boundary inside a token. It mirrors the suffix set accepted by
// RuntimeArtifactPathKind (including its basename special cases); every carve
// candidate is re-verified through RuntimeArtifactPathKind, so drift here can
// only cause a missed carve, never a false positive.
func runtimeArtifactCarveSuffixes() []string {
	out := []string{
		".log", ".perf.data", ".tracebundle.json",
		"perf.data",
	}
	out = append(out, ReservedRuntimeArtifactBlobBasenames()...)
	for ext := range runtimeArtifactPathExtensions {
		out = append(out, ext)
	}
	return out
}

func dropLeadingArtifactLabel(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r != ':' && r != '：' {
			continue
		}
		prefix := strings.TrimSpace(s[:i])
		suffix := strings.TrimSpace(s[i+utf8.RuneLen(r):])
		if prefix == "" || suffix == "" {
			continue
		}
		if isWindowsDriveArtifactPath(prefix, suffix) {
			continue
		}
		if strings.ContainsAny(prefix, `/\`) {
			continue
		}
		if RuntimeArtifactPathKind(suffix) != "" {
			return suffix
		}
		if carved := RuntimeArtifactPathInToken(suffix); carved != "" {
			return carved
		}
	}
	return ""
}

func isWindowsDriveArtifactPath(prefix, suffix string) bool {
	if len(prefix) != 1 || suffix == "" {
		return false
	}
	r := rune(prefix[0])
	if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
		return false
	}
	return strings.HasPrefix(suffix, `\`) || strings.HasPrefix(suffix, `/`)
}

func dropLeadingArtifactProse(s string) string {
	for i, r := range s {
		if !isArtifactPathProseRune(r) {
			return s[i:]
		}
	}
	return ""
}

func dropTrailingArtifactProse(s string) string {
	end := len(s)
	for end > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:end])
		if !isArtifactPathProseRune(r) {
			break
		}
		end -= size
	}
	return s[:end]
}

// isArtifactPathProseRune reports whether r is a CJK prose rune that would be
// glued directly to an ASCII-ish artifact path in Chinese/Japanese/Korean
// questions. ASCII letters are deliberately excluded — English prose is
// whitespace-delimited, so the existing tokenizer already splits it.
func isArtifactPathProseRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// RuntimeArtifactPathKindInText returns the runtime artifact family when a
// structured analyzer/source-policy field contains a path-shaped runtime
// artifact token embedded in a longer quote. It is still path-shape only: the
// caller must already be consuming a typed path/source-quote field, not raw user
// intent prose.
func RuntimeArtifactPathKindInText(s string) string {
	if tokens := RuntimeArtifactPathTokensInText(s); len(tokens) > 0 {
		return RuntimeArtifactPathKind(tokens[0])
	}
	return ""
}

// RuntimeArtifactPathTokensInText extracts path-shaped runtime artifact tokens
// from a typed source/path carrier. Callers must use the result as an artifact
// identity hint only; this helper is not an intent classifier.
func RuntimeArtifactPathTokensInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		token := strings.Trim(raw, "`\"'()[]{}<>，。；；,;：:")
		// Carve out a runtime-artifact path even when CJK prose is glued
		// directly to it (no separator), e.g. "分析/tmp/x.systrace的卡顿".
		token = RuntimeArtifactPathInToken(token)
		if token == "" {
			return
		}
		key := strings.ToLower(token)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, token)
	}
	trimmed := strings.TrimSpace(s)
	if trimmed != "" && !strings.ContainsFunc(trimmed, runtimeArtifactPathTokenSeparator) {
		add(trimmed)
	}
	for _, token := range strings.FieldsFunc(s, runtimeArtifactPathTokenSeparator) {
		add(token)
	}
	return out
}

func runtimeArtifactPathTokenSeparator(r rune) bool {
	switch r {
	case '/', '\\', '.', '-', '_', '~', ':':
		return false
	default:
		return unicode.IsSpace(r) || strings.ContainsRune("`\"'()[]{}<>，。；；,;|", r)
	}
}
