package types

import (
	"strings"
	"unicode"
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
	case "attached_log.txt":
		return "log"
	case "attached_trace.txt", "attached_hitrace.txt", "attached_atrace.txt":
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
		if RuntimeArtifactPathKind(token) == "" {
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
