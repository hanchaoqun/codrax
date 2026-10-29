package outputdump

import (
	"fmt"
	"runtime"
	"strings"
)

// DisplaySize is shared by terminal, Markdown and HTML presentation. The
// legacy Bytes field keeps its transport meaning; it is never enough to
// identify a bounded attachment preview as an original file.
func (a RuntimeArtifact) DisplaySize(lang string) string {
	if m := a.preparedMaterial; m != nil {
		return exactArtifactBytes(m.SourceBytes)
	}
	if a.previewOnly {
		if artifactDisplayEnglish(lang) {
			return fmt.Sprintf("original size unknown; preview %d B", a.Bytes)
		}
		return fmt.Sprintf("原件大小未知；预览 %d B", a.Bytes)
	}
	if a.fileBytesKnown || a.Bytes > 0 {
		return HumanBytes(a.Bytes)
	}
	if artifactDisplayEnglish(lang) {
		return "unknown"
	}
	return "未知"
}

func (a RuntimeArtifact) DisplayKind() string {
	if m := a.preparedMaterial; m != nil {
		return artifactFormatLabel(m.SourceFormat)
	}
	return firstNonEmpty(a.Kind, "artifact")
}

func (a RuntimeArtifact) DisplayDetail(lang string) string {
	m := a.preparedMaterial
	if m == nil {
		return a.Detail
	}
	query := m.QueryPath
	if m.QueryPath == m.SourcePath {
		query = "同原件"
		if artifactDisplayEnglish(lang) {
			query = "same as original"
		}
	}
	if artifactDisplayEnglish(lang) {
		detail := fmt.Sprintf("input channel: %s; original format: %s; query material: %s (%s, %s); preview envelope: %d B (not file size)",
			a.inputChannel, artifactFormatLabel(m.SourceFormat), query, exactArtifactBytes(m.QueryBytes), artifactFormatLabel(m.QueryFormat), m.PreviewBytes)
		if m.ArchiveMember != "" {
			detail += "; archive member: " + m.ArchiveMember
		}
		return appendPreparedArtifactDetail(detail, a.Detail)
	}
	detail := fmt.Sprintf("输入通道：%s；原件格式：%s；查询材料：%s（%s，%s）；预览封装：%d B（不是文件大小）",
		a.inputChannel, artifactFormatLabel(m.SourceFormat), query, exactArtifactBytes(m.QueryBytes), artifactFormatLabel(m.QueryFormat), m.PreviewBytes)
	if m.ArchiveMember != "" {
		detail += "；归档成员：" + m.ArchiveMember
	}
	return appendPreparedArtifactDetail(detail, a.Detail)
}

// Only a successful same-generation original observation can join a request
// card to a prepared card. Child carriers and guessed path/name identities do
// not participate; Kind and Carrier retain their pre-existing meanings.
func samePreparedOriginalDisplayRoot(a, b RuntimeArtifact) bool {
	if a.preparedMaterial == nil {
		a, b = b, a
	}
	return a.preparedMaterial != nil && b.preparedMaterial == nil &&
		b.Carrier == RuntimeArtifactCarrierDirectRequestPath &&
		a.filePath == b.filePath && a.fileGeneration.SameVersion(b.fileGeneration)
}

func sameRuntimeArtifactDisplayKey(a, b RuntimeArtifact) bool {
	if !strings.EqualFold(strings.TrimSpace(a.Kind), strings.TrimSpace(b.Kind)) {
		return false
	}
	sourceA, sourceB := strings.TrimSpace(a.Source), strings.TrimSpace(b.Source)
	if runtime.GOOS == "windows" {
		sourceA, sourceB = strings.ToLower(sourceA), strings.ToLower(sourceB)
	}
	if sourceA != sourceB {
		return false
	}
	if a.preparedMaterial == nil && b.preparedMaterial == nil {
		return true
	}
	if a.preparedMaterial == nil || b.preparedMaterial == nil {
		return false
	}
	queryA, knownA := a.preparedMaterial.FileGeneration(a.preparedMaterial.QueryPath)
	queryB, knownB := b.preparedMaterial.FileGeneration(b.preparedMaterial.QueryPath)
	return knownA && knownB && queryA.SameVersion(queryB) &&
		a.fileGeneration.SameVersion(b.fileGeneration) &&
		a.preparedMaterial.QueryPath == b.preparedMaterial.QueryPath &&
		a.preparedMaterial.ArchiveMember == b.preparedMaterial.ArchiveMember
}

func appendPreparedArtifactDetail(detail, original string) string {
	if original == "runtime trace" || original == "runtime log" {
		return detail
	}
	return appendDetail(detail, original)
}

func exactArtifactBytes(n int64) string { return fmt.Sprintf("%d B", n) }
func artifactDisplayEnglish(lang string) bool {
	return strings.EqualFold(strings.TrimSpace(lang), "en")
}

func artifactFormatLabel(format string) string {
	switch format {
	case "pcap":
		return "PCAP"
	case "gzip":
		return "gzip"
	case "zip":
		return "ZIP"
	case "sqlite":
		return "SQLite"
	case "text":
		return "text"
	case "tracebundle":
		return "Trace bundle"
	case "harmony_rmq":
		return "Harmony RMQ"
	case "openharmony_profiler":
		return "OpenHarmony profiler"
	case "linux_perf_data":
		return "Linux perf data"
	case "simpleperf_report_sample_proto":
		return "Simpleperf report protobuf"
	case "openharmony_raw":
		return "OpenHarmony raw"
	default:
		return "unknown"
	}
}
