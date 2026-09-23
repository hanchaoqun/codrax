package types

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
)

// TraceResourceObservationClaimKey identifies a typed resource aggregate, not
// its display label. Capture/query authority remains in ObservationSourceRef.
// Keep the family prefix for existing coverage consumers; only the suffix is
// opaque. Values are neither trimmed nor interpreted as paths or instructions.
func TraceResourceObservationClaimKey(kind, operation, path, dev, address, thread string) string {
	return kind + "_resource:" + traceResourceObservationIdentityDigest(
		"typed_resource_v1", kind, operation, path, dev, address, thread,
	)
}

// TracePluginObservationClaimKey preserves every published grouping dimension,
// including an absent domain and different categories of the same metric value.
func TracePluginObservationClaimKey(kind, domain, event, metric, value, category, thread string) string {
	return "plugin_event:" + traceResourceObservationIdentityDigest(
		"typed_plugin_v1", kind, domain, event, metric, value, category, thread,
	)
}

// Length-prefixed bytes avoid delimiter and invalid-UTF-8 normalization aliases.
// The fixed digest also keeps large source values out of the unbounded prompt
// claim-label slot; actual fields remain independently available as observations.
func traceResourceObservationIdentityDigest(fields ...string) string {
	h := sha256.New()
	var size [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(size[:], uint64(len(field)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(field))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// A legacy banner can have already lost quoting, precision or field content.
// Bind its own row rather than reconstructing a typed aggregate identity from
// that text. This never changes the legacy display-only authority ceiling.
func traceResourceLegacyObservationClaimKey(family string, index, ordinal int, line, continuation string) string {
	return family + ":legacy:" + traceResourceObservationIdentityDigest(
		"legacy_summary_row_v1", family, strconv.Itoa(index), strconv.Itoa(ordinal), line, continuation,
	)
}

// TraceResourceObservationSubject is a display label, never an identity or a
// fallback field value. Preserve the coordinate kind when no path was observed.
func TraceResourceObservationSubject(kind, path, dev, address string) string {
	if path != "" {
		return path
	}
	if address != "" {
		return "address=" + address
	}
	if dev != "" {
		return "dev=" + dev
	}
	return kind + "_resource"
}
