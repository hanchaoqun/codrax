package hitraceconv

import (
	"os"
	"path"
	"strings"
)

func normalizeResultCollections(result *Result) {
	if result == nil {
		return
	}
	result.Artifacts = dedupeArtifacts(result.Artifacts)
	result.Caveats = dedupeStrings(result.Caveats)
}

func dedupeArtifacts(items []Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]Artifact, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Path) == "" {
			continue
		}
		key := artifactDedupeKey(item)
		if idx, ok := seen[key]; ok {
			out[idx] = mergeArtifact(out[idx], item)
			continue
		}
		seen[key] = len(out)
		item.Caveats = dedupeStrings(item.Caveats)
		out = append(out, item)
	}
	return out
}

func mergeArtifact(base, extra Artifact) Artifact {
	if base.Path == "" {
		base.Path = extra.Path
	}
	if base.Bytes == 0 {
		base.Bytes = extra.Bytes
	}
	if base.DataType == 0 {
		base.DataType = extra.DataType
	}
	if base.PluginName == "" {
		base.PluginName = extra.PluginName
	}
	if base.PluginVersion == "" {
		base.PluginVersion = extra.PluginVersion
	}
	if base.SourceOffset == 0 {
		base.SourceOffset = extra.SourceOffset
	}
	if base.SourceBytes == 0 {
		base.SourceBytes = extra.SourceBytes
	}
	if base.Converter == "" {
		base.Converter = extra.Converter
	}
	if base.Perf == nil {
		base.Perf = extra.Perf
	}
	base.Caveats = dedupeStrings(append(base.Caveats, extra.Caveats...))
	return base
}

func artifactDedupeKey(item Artifact) string {
	return strings.TrimSpace(item.Type) + "\x00" + artifactDedupePath(item.Path) + "\x00" + strings.TrimSpace(item.Converter)
}

func artifactDedupePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	return path.Clean(raw)
}

func artifactPathExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func dedupeStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}
