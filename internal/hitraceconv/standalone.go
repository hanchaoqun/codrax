package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	profilerTraceHeaderSize       = 1024
	profilerTraceHeaderMagic      = uint64(0x464F5250534F484F)
	profilerDataTypeHiperf        = uint32(1)
	profilerPluginNameOffset      = 108
	profilerPluginNameSize        = 128
	profilerPluginVersionOffset   = 236
	profilerPluginVersionSize     = 8
	profilerStandalonePayloadBase = profilerTraceHeaderSize
)

var profilerTraceHeaderMagicLE = []byte{
	byte(profilerTraceHeaderMagic & 0xff),
	byte((profilerTraceHeaderMagic >> 8) & 0xff),
	byte((profilerTraceHeaderMagic >> 16) & 0xff),
	byte((profilerTraceHeaderMagic >> 24) & 0xff),
	byte((profilerTraceHeaderMagic >> 32) & 0xff),
	byte((profilerTraceHeaderMagic >> 40) & 0xff),
	byte((profilerTraceHeaderMagic >> 48) & 0xff),
	byte((profilerTraceHeaderMagic >> 56) & 0xff),
}

type standaloneSegment struct {
	Offset        int64
	Length        int64
	DataType      uint32
	PluginName    string
	PluginVersion string
}

type traceBundleMetadata struct {
	Version   string     `json:"version"`
	InputPath string     `json:"input_path"`
	Systrace  string     `json:"systrace,omitempty"`
	Artifacts []Artifact `json:"artifacts,omitempty"`
	Caveats   []string   `json:"caveats,omitempty"`
}

func extractStandaloneArtifacts(ctx context.Context, input string, inputSize int64, outputPath string) ([]Artifact, []string, error) {
	segments, err := findStandaloneSegments(ctx, input, inputSize)
	if err != nil {
		return nil, nil, err
	}
	if len(segments) == 0 {
		return nil, nil, nil
	}
	base := traceSidecarBase(input, outputPath)
	in, err := os.Open(input)
	if err != nil {
		return nil, nil, err
	}
	defer in.Close()
	var artifacts []Artifact
	var caveats []string
	perfOrdinal := 0
	for _, seg := range segments {
		if seg.DataType != profilerDataTypeHiperf {
			continue
		}
		perfOrdinal++
		outPath := numberedSidecarPath(base, perfOrdinal, ".perf.data")
		if err := ensureOutputDoesNotExist(outPath); err != nil {
			return artifacts, caveats, err
		}
		n, err := copyRangeToFile(in, seg.Offset+profilerStandalonePayloadBase, seg.Length-profilerStandalonePayloadBase, outPath)
		if err != nil {
			return artifacts, caveats, err
		}
		artifacts = append(artifacts, Artifact{
			Type:          ArtifactPerfData,
			Path:          outPath,
			Bytes:         n,
			DataType:      seg.DataType,
			PluginName:    seg.PluginName,
			PluginVersion: seg.PluginVersion,
			SourceOffset:  seg.Offset,
			SourceBytes:   seg.Length,
			Caveats:       []string{"raw perf.data sidecar extracted; run an official hiperf/simpleperf adapter to produce .perftrace before trace_query can aggregate CPU samples"},
		})
	}
	if len(artifacts) > 0 {
		caveats = append(caveats, fmt.Sprintf("extracted %d HIPERF_DATA standalone perf.data artifact(s); raw perf.data is preserved as sidecar and still needs official parser conversion to .perftrace for trace_query sample aggregation", len(artifacts)))
	}
	return artifacts, caveats, nil
}

func findStandaloneSegments(ctx context.Context, path string, size int64) ([]standaloneSegment, error) {
	if size < profilerTraceHeaderSize {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const chunkSize = 1024 * 1024
	overlap := len(profilerTraceHeaderMagicLE) - 1
	buf := make([]byte, chunkSize+overlap)
	var segments []standaloneSegment
	seen := map[int64]bool{}
	var base int64
	var carry []byte
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		copy(buf, carry)
		n, readErr := f.Read(buf[len(carry):chunkSize])
		window := buf[:len(carry)+n]
		search := window
		searchBase := base - int64(len(carry))
		for {
			idx := bytes.Index(search, profilerTraceHeaderMagicLE)
			if idx < 0 {
				break
			}
			candidate := searchBase + int64(idx)
			if candidate >= 0 && !seen[candidate] {
				seen[candidate] = true
				if seg, ok := readStandaloneSegmentAt(f, candidate, size); ok {
					segments = append(segments, seg)
				}
			}
			search = search[idx+1:]
			searchBase = candidate + 1
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		if len(window) > overlap {
			carry = append(carry[:0], window[len(window)-overlap:]...)
		} else {
			carry = append(carry[:0], window...)
		}
		base += int64(n)
	}
	return segments, nil
}

func isProfilerTraceHeaderPrefix(first, second uint32) bool {
	return first == uint32(profilerTraceHeaderMagic&0xffffffff) && second == uint32((profilerTraceHeaderMagic>>32)&0xffffffff)
}

func readStandaloneSegmentAt(f *os.File, off int64, fileSize int64) (standaloneSegment, bool) {
	if off < 0 || off+profilerTraceHeaderSize > fileSize {
		return standaloneSegment{}, false
	}
	header := make([]byte, profilerTraceHeaderSize)
	if _, err := f.ReadAt(header, off); err != nil {
		return standaloneSegment{}, false
	}
	if binary.LittleEndian.Uint64(header[0:8]) != profilerTraceHeaderMagic {
		return standaloneSegment{}, false
	}
	length := int64(binary.LittleEndian.Uint64(header[8:16]))
	if length < profilerTraceHeaderSize || off+length > fileSize {
		return standaloneSegment{}, false
	}
	dataType := binary.LittleEndian.Uint32(header[56:60])
	return standaloneSegment{
		Offset:        off,
		Length:        length,
		DataType:      dataType,
		PluginName:    cString(header[profilerPluginNameOffset : profilerPluginNameOffset+profilerPluginNameSize]),
		PluginVersion: cString(header[profilerPluginVersionOffset : profilerPluginVersionOffset+profilerPluginVersionSize]),
	}, true
}

func writeTraceBundle(input, outputPath string, artifacts []Artifact, caveats []string) (Artifact, error) {
	if len(artifacts) == 0 {
		return Artifact{}, nil
	}
	base := traceSidecarBase(input, outputPath)
	path := base + ".tracebundle.json"
	if err := ensureOutputDoesNotExist(path); err != nil {
		return Artifact{}, err
	}
	meta := traceBundleMetadata{
		Version:   converterVersion,
		InputPath: input,
		Systrace:  outputPath,
		Artifacts: artifacts,
		Caveats:   caveats,
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Artifact{}, err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return Artifact{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Type: ArtifactTraceBundle, Path: path, Bytes: info.Size(), Converter: converterVersion}, nil
}

func traceSidecarBase(input, outputPath string) string {
	base := strings.TrimSpace(outputPath)
	if base == "" {
		base = input
	}
	if strings.HasSuffix(base, defaultOutputSuffix) {
		base = strings.TrimSuffix(base, defaultOutputSuffix)
	}
	return base
}

func numberedSidecarPath(base string, ordinal int, suffix string) string {
	if ordinal <= 1 {
		return base + suffix
	}
	ext := filepath.Ext(suffix)
	stem := strings.TrimSuffix(suffix, ext)
	if ext == "" {
		return fmt.Sprintf("%s%s_%d", base, suffix, ordinal)
	}
	return fmt.Sprintf("%s%s_%d%s", base, stem, ordinal, ext)
}

func ensureOutputDoesNotExist(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("output file already exists: %s (delete it first or specify a different output path)", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check output path %s: %w", path, err)
	}
	return nil
}

func copyRangeToFile(in *os.File, off, length int64, outPath string) (int64, error) {
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(out, io.NewSectionReader(in, off, length))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(outPath)
		return written, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(outPath)
		return written, closeErr
	}
	return written, nil
}

func cString(data []byte) string {
	if idx := bytes.IndexByte(data, 0); idx >= 0 {
		data = data[:idx]
	}
	return strings.TrimSpace(string(data))
}
