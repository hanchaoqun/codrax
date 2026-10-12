package hitraceconv

import (
	"bytes"
	"encoding/binary"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// SourceMetadata reads only private producer payloads; Result/bundle JSON,
// generic parser keys and prose caveats cannot mint capture identity fields.
func (r Result) SourceMetadata() attachment.TraceSourceMetadata {
	var fields []attachment.TraceSourceMetadataField
	for _, artifact := range r.Artifacts {
		fields = append(fields, artifact.SourceMetadata.Fields()...)
	}
	for _, coverage := range r.TraceDBCoverage {
		fields = append(fields, coverage.SourceMetadata.Fields()...)
	}
	return attachment.NewTraceSourceMetadata(fields)
}

// SourceMetadataForQueryPath binds context only to the selected query member.
// Mixed bundles retain diagnostics in their conversion receipt but cannot
// attribute another member's headers to the queried stream.
func (r Result) SourceMetadataForQueryPath(path string) attachment.TraceSourceMetadata {
	if path == r.BundlePath && path != "" {
		member := ""
		for _, artifact := range r.Artifacts {
			if artifact.Type != ArtifactSystrace && artifact.Type != ArtifactPerfTrace {
				continue
			}
			if member != "" {
				return attachment.NewTraceSourceMetadata([]attachment.TraceSourceMetadataField{{Origin: "capture_members", Kind: "parser", Name: "device_workload_attribution", State: "invalid", Detail: "multiple query members: device/workload headers are not uniquely attributable; per-member diagnostics remain in conversion receipt"}})
			}
			member = artifact.Path
		}
		path = member
	}
	for _, artifact := range r.Artifacts {
		if artifact.Path != path {
			continue
		}
		if artifact.Type == ArtifactPerfTrace {
			return artifact.SourceMetadata
		}
		if artifact.Type == ArtifactSystrace {
			var fields []attachment.TraceSourceMetadataField
			for _, coverage := range r.TraceDBCoverage {
				if coverage.ArtifactPath == "" || coverage.ArtifactPath == path {
					fields = append(fields, coverage.SourceMetadata.Fields()...)
				}
			}
			return attachment.NewTraceSourceMetadata(fields)
		}
	}
	return attachment.TraceSourceMetadata{}
}

func rawPerfSourceMetadata(ids []int) []attachment.TraceSourceMetadataField {
	fields := []attachment.TraceSourceMetadataField{
		{Origin: "perf_feature", Kind: "device", Name: "Hostname", State: "missing"},
		{Origin: "perf_feature", Kind: "device", Name: "CPUDesc", State: "missing"},
		{Origin: "perf_feature", Kind: "device", Name: "CPUID", State: "missing"},
		{Origin: "perf_feature", Kind: "workload", Name: "WorkloadCmd", State: "missing"},
	}
	for _, id := range ids {
		if index := rawPerfMetadataIndex(id); index >= 0 {
			fields[index].State = "invalid"
			fields[index].Detail = "feature declared but not decoded"
		}
	}
	return fields
}

func rawPerfMetadataIndex(id int) int {
	for i, fieldID := range []int{perfFeatureHostname, perfFeatureCPUDesc, perfFeatureCPUID, perfFeatureHiperfWorkloadCmd} {
		if id == fieldID {
			return i
		}
	}
	return -1
}

func decodeRawPerfMetadataField(field attachment.TraceSourceMetadataField, buf []byte) attachment.TraceSourceMetadataField {
	field.State, field.Value, field.Detail = "invalid", "", "invalid feature string length or UTF-8"
	if len(buf) < 4 {
		return field
	}
	n := uint64(binary.LittleEndian.Uint32(buf[:4]))
	if n == 0 || n > uint64(len(buf)-4) {
		return field
	}
	payload := buf[4 : 4+int(n)]
	if end := bytes.IndexByte(payload, 0); end >= 0 {
		payload = payload[:end]
	}
	value := string(payload)
	if !utf8.ValidString(value) {
		return field
	}
	if len(value) > maxTraceDBParserMetadataValueBytes {
		field.State, field.Detail = "truncated", "value exceeds 1024-byte diagnostic budget; no partial value presented"
		return field
	}
	field.State, field.Value, field.Detail = "observed", value, ""
	return field
}
