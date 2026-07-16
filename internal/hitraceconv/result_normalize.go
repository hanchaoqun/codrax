package hitraceconv

import (
	"os"
	"path"
	"strconv"
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
	key := strings.TrimSpace(item.Type) + "\x00" + artifactDedupePath(item.Path) + "\x00" + strings.TrimSpace(item.Converter)
	if item.Trace == nil && item.traceReceiptBindingPath == "" && item.traceReceiptArtifactPath == "" {
		return key
	}
	// Receipt-backed systrace and type-only inventory must never coalesce.
	// Include the complete in-memory receipt projection so a forged/drifted
	// duplicate survives normalization for strict consumer rejection instead
	// of being field-merged into a seemingly valid claim.
	key += "\x00trace_receipt\x00" + item.traceReceiptBindingPath + "\x00" + item.traceReceiptArtifactPath +
		"\x00" + item.SHA256 + "\x00" + strconv.FormatInt(item.Bytes, 10) +
		"\x00" + strconv.FormatUint(uint64(item.DataType), 10) + "\x00" + item.PluginName +
		"\x00" + item.PluginVersion + "\x00" + strconv.FormatInt(item.SourceOffset, 10) +
		"\x00" + strconv.FormatInt(item.SourceBytes, 10)
	if item.Perf != nil {
		key += "\x00unexpected_perf_capability"
	}
	if item.Trace == nil {
		return key + "\x00nil_capability"
	}
	capability := item.Trace
	return key + "\x00" + capability.ProviderKind + "\x00" + capability.ProviderName +
		"\x00" + capability.OutputFormat + "\x00" + capability.ValidationProfile +
		"\x00" + strconv.Itoa(capability.Rows) + "\x00" + strconv.Itoa(capability.Known) +
		"\x00" + strconv.Itoa(capability.AuthoritativeKnown) +
		"\x00" + strconv.Itoa(capability.AdvisoryRows) +
		"\x00" + strconv.Itoa(capability.IntentionalUnknown) +
		"\x00" + strconv.Itoa(capability.IntentionalHeaderOnly) +
		"\x00" + strconv.FormatBool(capability.TraceQueryReady)
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
