package hitraceconv

import (
	"encoding/json"
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
		if strings.TrimSpace(item.Type) == "" && strings.TrimSpace(item.Path) == "" &&
			item.Standalone == nil && item.PerfTransform == nil && item.standaloneReceipt == nil {
			continue
		}
		key := artifactDedupeKey(item)
		if idx, ok := seen[key]; ok {
			out[idx] = mergeArtifact(out[idx], item)
			continue
		}
		seen[key] = len(out)
		item = cloneArtifact(item)
		item.Caveats = dedupeStrings(item.Caveats)
		out = append(out, item)
	}
	return out
}

func cloneArtifact(item Artifact) Artifact {
	cloned := item
	cloned.Caveats = append([]string(nil), item.Caveats...)
	if item.Standalone != nil {
		provenance := *item.Standalone
		cloned.Standalone = &provenance
	}
	if item.PerfTransform != nil {
		transform := *item.PerfTransform
		cloned.PerfTransform = &transform
	}
	if item.standaloneReceipt != nil {
		receipt := *item.standaloneReceipt
		cloned.standaloneReceipt = &receipt
	}
	if item.Trace != nil {
		capability := *item.Trace
		cloned.Trace = &capability
	}
	if item.Perf != nil {
		capability := *item.Perf
		capability.Caveats = append([]string(nil), item.Perf.Caveats...)
		if item.Perf.RawCaptureCompleteness != nil {
			capability.RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(*item.Perf.RawCaptureCompleteness)
		}
		if item.Perf.RawCaptureResidual != nil {
			capability.RawCaptureResidual = cloneRawPerfCaptureResidual(*item.Perf.RawCaptureResidual)
		}
		if item.Perf.RawSampleAdmission != nil {
			capability.RawSampleAdmission = cloneRawPerfSampleAdmission(*item.Perf.RawSampleAdmission)
		}
		cloned.Perf = &capability
	}
	return cloned
}

func cloneArtifactList(items []Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]Artifact, len(items))
	for index := range items {
		cloned[index] = cloneArtifact(items[index])
	}
	return cloned
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
	if base.Standalone == nil && extra.Standalone != nil {
		provenance := *extra.Standalone
		base.Standalone = &provenance
	}
	if base.PerfTransform == nil && extra.PerfTransform != nil {
		base.PerfTransform = clonePerfInputTransform(extra.PerfTransform)
	}
	base.Caveats = dedupeStrings(append(base.Caveats, extra.Caveats...))
	return base
}

func artifactDedupeKey(item Artifact) string {
	key := strings.TrimSpace(item.Type) + "\x00" + artifactDedupePath(item.Path) + "\x00" + strings.TrimSpace(item.Converter)
	if item.Standalone == nil {
		key += "\x00standalone_provenance:nil"
	} else if encoded, err := json.Marshal(item.Standalone); err == nil {
		key += "\x00standalone_provenance:" + string(encoded)
	} else {
		key += "\x00standalone_provenance:invalid"
	}
	if item.standaloneReceipt == nil {
		key += "\x00standalone_receipt:nil"
	} else {
		receipt := item.standaloneReceipt
		key += "\x00standalone_receipt:" + strconv.FormatInt(receipt.Offset, 10) +
			"\x00" + strconv.FormatInt(receipt.Length, 10) + "\x00" +
			strconv.FormatUint(uint64(receipt.DataType), 10) + "\x00" + receipt.PluginName +
			"\x00" + receipt.PluginVersion + "\x00" + receipt.Integrity + "\x00" + receipt.Layout +
			"\x00" + string(receipt.PayloadSHA256[:]) + "\x00" + strconv.FormatBool(receipt.PerfEligible) +
			"\x00" + string(receipt.PerfInputFormat) + "\x00" + receipt.ArtifactPath +
			"\x00" + receipt.BindingPath
	}
	if item.Standalone != nil || item.standaloneReceipt != nil {
		// A receipt-backed raw child may dedupe only when its complete public
		// claim is byte-identical. Conflicting duplicates must survive
		// normalization so the publication gate rejects both input orders.
		key += "\x00standalone_public_claim\x00" + item.Type + "\x00" + item.Path +
			"\x00" + item.Converter + "\x00" + item.SHA256 + "\x00" +
			strconv.FormatInt(item.Bytes, 10) + "\x00" +
			strconv.FormatUint(uint64(item.DataType), 10) + "\x00" + item.PluginName + "\x00" +
			item.PluginVersion + "\x00" + strconv.FormatInt(item.SourceOffset, 10) + "\x00" +
			strconv.FormatInt(item.SourceBytes, 10)
		if item.Trace != nil {
			key += "\x00unexpected_trace_capability"
		}
		if item.Perf == nil {
			key += "\x00nil_perf_capability"
		} else if encoded, err := json.Marshal(item.Perf); err == nil {
			key += "\x00perf_capability:" + string(encoded)
		} else {
			key += "\x00perf_capability:invalid"
		}
	}
	if item.Type == ArtifactPerfTrace {
		key += "\x00perf_receipt\x00" + item.SHA256 + "\x00" + strconv.FormatInt(item.Bytes, 10)
		if item.PerfTransform == nil {
			key += "\x00perf_input_transform:nil"
		} else if encoded, err := json.Marshal(item.PerfTransform); err == nil {
			key += "\x00perf_input_transform:" + string(encoded)
		} else {
			key += "\x00perf_input_transform:invalid"
		}
		if item.Perf == nil {
			return key + "\x00nil_capability"
		}
		capability, err := json.Marshal(item.Perf)
		if err != nil {
			return key + "\x00invalid_capability"
		}
		return key + "\x00" + string(capability)
	}
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
