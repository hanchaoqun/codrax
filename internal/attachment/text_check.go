package attachment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Kind string

const (
	KindLog   Kind = "log"
	KindTrace Kind = "trace"
)

type Surface string

const (
	SurfaceCLI  Surface = "cli"
	SurfaceREPL Surface = "repl"
)

const (
	// TextProbeBytes is the fixed admission prefix for held physical files and
	// stdin. Publishing a smaller attachment payload must never reduce this
	// safety probe.
	TextProbeBytes = 64 * 1024
	textProbeBytes = TextProbeBytes
	// TracePhysicalLineMaxBytes is the single pre-parser ceiling for physical
	// trace rows. Ftrace-family rows are normally tiny; 16 MiB preserves broad
	// compatibility while bounding every legacy ReadString allocation.
	TracePhysicalLineMaxBytes = 16 << 20
)

// BinaryTraceFormat is a closed, content-only classification for exact binary
// container magics which must never be admitted as trace text. It deliberately
// does not consult path suffixes: a textual file named *.sys remains text, and
// an unrelated Windows driver named *.sys does not become a trace artifact.
type BinaryTraceFormat string

const (
	BinaryTraceFormatUnknown     BinaryTraceFormat = ""
	BinaryTraceFormatHarmonyRMQ  BinaryTraceFormat = "harmony_rmq"
	BinaryTraceFormatOHOSProfile BinaryTraceFormat = "openharmony_profiler"
	BinaryTraceFormatLinuxPerf   BinaryTraceFormat = "linux_perf_data"
	BinaryTraceFormatGZIP        BinaryTraceFormat = "gzip"
	BinaryTraceFormatZIP         BinaryTraceFormat = "zip"
	BinaryTraceFormatSQLite      BinaryTraceFormat = "sqlite"
)

type TextIssue struct {
	Kind   Kind
	Path   string
	Reason string
}

func (e TextIssue) Error() string {
	return e.Message("en", SurfaceCLI)
}

func ValidateText(kind Kind, path string, data []byte, truncated bool) error {
	if issue := CheckText(kind, path, data, truncated); issue != nil {
		return *issue
	}
	return nil
}

// ValidateSourceLabel rejects path spellings that could inject a second
// # codrax-source header line into the attachment envelope. The source label
// is provenance metadata, not trace/log body text, so control characters have
// no valid representation in this line-oriented carrier.
func ValidateSourceLabel(source string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("attachment source label is empty")
	}
	for _, r := range source {
		if unicode.IsControl(r) {
			return fmt.Errorf("attachment source label contains control character U+%04X", r)
		}
	}
	return nil
}

// ValidateSingleTraceAttachmentProvenance prevents multiple physical capture
// files from being flattened into one synthetic trace clock/causal universe.
// Multi-trace comparison remains available through separate named paths or a
// provenance-carrying tracebundle; one attached blob may name at most one
// reserved source header.
func ValidateSingleTraceAttachmentProvenance(body string) error {
	const marker = "# codrax-source: "
	count := 0
	for offset := 0; offset < len(body); {
		end := strings.IndexByte(body[offset:], '\n')
		if end < 0 {
			end = len(body) - offset
		}
		line := strings.TrimSuffix(body[offset:offset+end], "\r")
		if strings.HasPrefix(line, marker) {
			count++
			if count > 1 {
				return fmt.Errorf("attached trace contains multiple physical source headers; name each trace path in the question or use a provenance-carrying .tracebundle.json instead of merging captures")
			}
		}
		offset += end
		if offset < len(body) && body[offset] == '\n' {
			offset++
		}
	}
	return nil
}

// ValidatePublishableText applies the shared text policy and, only when the
// caller proves that a byte cap truncated the source, removes an accepted
// incomplete UTF-8 suffix. Returned bytes are therefore always complete UTF-8
// and can be revalidated later without carrying ambient truncation state.
func ValidatePublishableText(kind Kind, path string, data []byte, truncated bool) ([]byte, error) {
	if err := ValidateText(kind, path, data, truncated); err != nil {
		return nil, err
	}
	if truncated && len(data) > 0 && !utf8.Valid(data) {
		start := len(data) - 1
		for start > 0 && !utf8.RuneStart(data[start]) {
			start--
		}
		if !utf8.FullRune(data[start:]) {
			data = data[:start]
		}
	}
	if len(data) == 0 {
		return nil, TextIssue{Kind: kind, Path: path, Reason: "empty"}
	}
	return data, nil
}

// ValidateTextString is the zero-copy companion to ValidateText for runtime
// attachment payloads already owned as strings. It applies the same policy to
// the complete payload without materializing a second body-sized []byte.
func ValidateTextString(kind Kind, path, data string, truncated bool) error {
	if issue := CheckTextString(kind, path, data, truncated); issue != nil {
		return *issue
	}
	return nil
}

// ValidateTextReaderAt performs the shared text admission check against a
// bounded prefix without advancing any caller-owned stream offset. The caller
// must supply the size from the same held file generation as reader. Full
// in-memory attachments use ValidateText instead and are checked in full.
func ValidateTextReaderAt(kind Kind, path string, reader io.ReaderAt, size int64) error {
	if reader == nil {
		return fmt.Errorf("validate attached %s %s: reader is nil", kind, quotePath(path))
	}
	if size < 0 {
		return fmt.Errorf("validate attached %s %s: size is negative", kind, quotePath(path))
	}
	probeSize := size
	if probeSize > textProbeBytes {
		probeSize = textProbeBytes
	}
	probe := make([]byte, int(probeSize))
	if probeSize > 0 {
		n, err := reader.ReadAt(probe, 0)
		if n != len(probe) {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return fmt.Errorf("validate attached %s %s: read text probe: %w", kind, quotePath(path), err)
		}
		if err != nil && err != io.EOF {
			return fmt.Errorf("validate attached %s %s: read text probe: %w", kind, quotePath(path), err)
		}
	}
	return ValidateText(kind, path, probe, size > probeSize)
}

// ValidateTextReaderAtFull validates every byte of one held physical source
// with fixed memory. It also enforces a caller-selected physical-line ceiling
// before any downstream parser can allocate an unbounded line. ReaderAt keeps
// the descriptor offset unchanged; callers bind the verdict to a before/after
// file-generation identity.
func ValidateTextReaderAtFull(ctx context.Context, kind Kind, path string, reader io.ReaderAt, size int64, maxLineBytes int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if reader == nil {
		return fmt.Errorf("validate attached %s %s: reader is nil", kind, quotePath(path))
	}
	if size < 0 {
		return fmt.Errorf("validate attached %s %s: size is negative", kind, quotePath(path))
	}
	if maxLineBytes < 0 {
		return fmt.Errorf("validate attached %s %s: physical line limit is negative", kind, quotePath(path))
	}
	if size == 0 {
		return TextIssue{Kind: kind, Path: path, Reason: "empty"}
	}

	headSize := size
	if headSize > 32 {
		headSize = 32
	}
	head := make([]byte, int(headSize))
	if err := readTextReaderAtExact(reader, head, 0); err != nil {
		return fmt.Errorf("validate attached %s %s: read format prefix: %w", kind, quotePath(path), err)
	}
	if kind == KindTrace {
		if format := KnownBinaryTraceFormat(head); format != BinaryTraceFormatUnknown {
			return TextIssue{Kind: kind, Path: path, Reason: "known binary trace format: " + string(format)}
		}
	}

	buf := make([]byte, textProbeBytes)
	var pending [utf8.UTFMax]byte
	pendingLen := 0
	var seenRunes int64
	var controlRunes int64
	var lineBytes int64
	lineLimit := int64(maxLineBytes)
	observeRune := func(r rune) {
		seenRunes++
		switch r {
		case '\n', '\r', '\t', '\f', '\b', 0x1b:
			return
		}
		if unicode.IsControl(r) {
			controlRunes++
		}
	}
	lineTooLong := func() error {
		return TextIssue{Kind: kind, Path: path, Reason: fmt.Sprintf("physical line exceeds %d bytes", maxLineBytes)}
	}

	for offset := int64(0); offset < size; {
		if err := ctx.Err(); err != nil {
			return err
		}
		want := int64(len(buf))
		if remaining := size - offset; remaining < want {
			want = remaining
		}
		chunk := buf[:int(want)]
		if err := readTextReaderAtExact(reader, chunk, offset); err != nil {
			return fmt.Errorf("validate attached %s %s: read full text preflight at offset %d: %w", kind, quotePath(path), offset, err)
		}
		asciiOnly := pendingLen == 0
		var asciiControls int64
		for _, value := range chunk {
			if value == 0 {
				return TextIssue{Kind: kind, Path: path, Reason: "contains NUL bytes"}
			}
			if value >= utf8.RuneSelf {
				asciiOnly = false
				continue
			}
			if value >= 0x20 && value != 0x7f {
				continue
			}
			switch value {
			case '\n', '\r', '\t', '\f', '\b', 0x1b:
			default:
				asciiControls++
			}
		}

		for start := 0; start < len(chunk); {
			rel := bytes.IndexByte(chunk[start:], '\n')
			if rel < 0 {
				lineBytes += int64(len(chunk) - start)
				if lineLimit > 0 && lineBytes > lineLimit {
					return lineTooLong()
				}
				break
			}
			lineBytes += int64(rel)
			if lineLimit > 0 && lineBytes > lineLimit {
				return lineTooLong()
			}
			lineBytes = 0
			start += rel + 1
		}
		if asciiOnly {
			seenRunes += int64(len(chunk))
			controlRunes += asciiControls
			offset += want
			continue
		}

		index := 0
		if pendingLen > 0 {
			for index < len(chunk) && pendingLen < len(pending) && !utf8.FullRune(pending[:pendingLen]) {
				pending[pendingLen] = chunk[index]
				pendingLen++
				index++
			}
			if utf8.FullRune(pending[:pendingLen]) {
				r, runeSize := utf8.DecodeRune(pending[:pendingLen])
				if r == utf8.RuneError && runeSize == 1 {
					return TextIssue{Kind: kind, Path: path, Reason: "not valid UTF-8 text"}
				}
				observeRune(r)
				pendingLen = 0
			}
		}
		for index < len(chunk) {
			if chunk[index] < utf8.RuneSelf {
				observeRune(rune(chunk[index]))
				index++
				continue
			}
			if !utf8.FullRune(chunk[index:]) {
				pendingLen = copy(pending[:], chunk[index:])
				break
			}
			r, runeSize := utf8.DecodeRune(chunk[index:])
			if r == utf8.RuneError && runeSize == 1 {
				return TextIssue{Kind: kind, Path: path, Reason: "not valid UTF-8 text"}
			}
			observeRune(r)
			index += runeSize
		}
		offset += want
	}
	if pendingLen != 0 {
		return TextIssue{Kind: kind, Path: path, Reason: "not valid UTF-8 text"}
	}
	if seenRunes >= 32 && controlRunes > seenRunes/10 {
		return TextIssue{Kind: kind, Path: path, Reason: "too many non-text control bytes"}
	}
	return ctx.Err()
}

func readTextReaderAtExact(reader io.ReaderAt, data []byte, offset int64) error {
	if len(data) == 0 {
		return nil
	}
	n, err := reader.ReadAt(data, offset)
	if n != len(data) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

func CheckText(kind Kind, path string, data []byte, truncated bool) *TextIssue {
	return checkTextSignals(kind, path, len(data), truncated,
		KnownBinaryTraceFormat(data),
		bytes.IndexByte(data, 0) >= 0,
		utf8.Valid(data),
		func(offset int) (rune, int) { return utf8.DecodeRune(data[offset:]) },
		func(offset int) byte { return data[offset] },
	)
}

// CheckTextString shares CheckText's single policy authority while keeping the
// caller's immutable payload in place. The callbacks below are stack-local and
// every scan advances monotonically, so the extra memory remains constant.
func CheckTextString(kind Kind, path, data string, truncated bool) *TextIssue {
	return checkTextSignals(kind, path, len(data), truncated,
		knownBinaryTraceFormatString(data),
		strings.IndexByte(data, 0) >= 0,
		utf8.ValidString(data),
		func(offset int) (rune, int) { return utf8.DecodeRuneInString(data[offset:]) },
		func(offset int) byte { return data[offset] },
	)
}

func checkTextSignals(
	kind Kind,
	path string,
	length int,
	truncated bool,
	format BinaryTraceFormat,
	hasNUL bool,
	validUTF8 bool,
	decodeRune func(int) (rune, int),
	byteAt func(int) byte,
) *TextIssue {
	if length == 0 {
		if truncated {
			return nil
		}
		return &TextIssue{Kind: kind, Path: path, Reason: "empty"}
	}
	if kind == KindTrace && format != BinaryTraceFormatUnknown {
		return &TextIssue{Kind: kind, Path: path, Reason: "known binary trace format: " + string(format)}
	}
	if hasNUL {
		return &TextIssue{Kind: kind, Path: path, Reason: "contains NUL bytes"}
	}
	if kind == KindTrace && physicalLineExceedsInputLimit(length, TracePhysicalLineMaxBytes, byteAt) {
		return &TextIssue{Kind: kind, Path: path, Reason: fmt.Sprintf("physical line exceeds %d bytes", TracePhysicalLineMaxBytes)}
	}
	if !validUTF8ForPossiblyTruncatedInput(length, truncated, validUTF8, decodeRune, byteAt) {
		return &TextIssue{Kind: kind, Path: path, Reason: "not valid UTF-8 text"}
	}
	if mostlyControlRunesInput(length, decodeRune) {
		return &TextIssue{Kind: kind, Path: path, Reason: "too many non-text control bytes"}
	}
	return nil
}

func physicalLineExceedsInputLimit(length, maxLineBytes int, byteAt func(int) byte) bool {
	if length <= 0 || maxLineBytes <= 0 || byteAt == nil {
		return false
	}
	lineBytes := 0
	for offset := 0; offset < length; offset++ {
		if byteAt(offset) == '\n' {
			lineBytes = 0
			continue
		}
		lineBytes++
		if lineBytes > maxLineBytes {
			return true
		}
	}
	return false
}

// KnownBinaryTraceFormat recognizes only byte-exact container magics. The
// result is an admission label, not a claim that the current converter can
// successfully decode every member of that container family.
func KnownBinaryTraceFormat(data []byte) BinaryTraceFormat {
	return knownBinaryTraceFormatInput(len(data), func(offset int) byte { return data[offset] })
}

func knownBinaryTraceFormatString(data string) BinaryTraceFormat {
	return knownBinaryTraceFormatInput(len(data), func(offset int) byte { return data[offset] })
}

func knownBinaryTraceFormatInput(length int, byteAt func(int) byte) BinaryTraceFormat {
	switch {
	case inputHasPrefix(length, byteAt, "\xce\x0a"):
		return BinaryTraceFormatHarmonyRMQ
	case inputHasPrefix(length, byteAt, "OHOSPROF"):
		return BinaryTraceFormatOHOSProfile
	case inputHasPrefix(length, byteAt, "PERFILE2"), inputHasPrefix(length, byteAt, "2ELIFREP"):
		return BinaryTraceFormatLinuxPerf
	case inputHasPrefix(length, byteAt, "\x1f\x8b"):
		return BinaryTraceFormatGZIP
	case inputHasPrefix(length, byteAt, "PK\x03\x04"),
		inputHasPrefix(length, byteAt, "PK\x05\x06"),
		inputHasPrefix(length, byteAt, "PK\x07\x08"):
		return BinaryTraceFormatZIP
	case inputHasPrefix(length, byteAt, "SQLite format 3\x00"):
		return BinaryTraceFormatSQLite
	default:
		return BinaryTraceFormatUnknown
	}
}

func inputHasPrefix(length int, byteAt func(int) byte, prefix string) bool {
	if length < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if byteAt(i) != prefix[i] {
			return false
		}
	}
	return true
}

func validUTF8ForPossiblyTruncatedInput(
	length int,
	truncated bool,
	validUTF8 bool,
	decodeRune func(int) (rune, int),
	byteAt func(int) byte,
) bool {
	if validUTF8 {
		return true
	}
	if !truncated {
		return false
	}
	for offset := 0; offset < length; {
		r, size := decodeRune(offset)
		if r == utf8.RuneError && size == 1 {
			return validIncompleteUTF8SuffixInput(length-offset, func(index int) byte {
				return byteAt(offset + index)
			})
		}
		offset += size
	}
	return true
}

func validIncompleteUTF8SuffixInput(length int, byteAt func(int) byte) bool {
	if length == 0 {
		return false
	}
	want := 0
	secondMin, secondMax := byte(0x80), byte(0xbf)
	switch first := byteAt(0); {
	case first >= 0xc2 && first <= 0xdf:
		want = 2
	case first == 0xe0:
		want, secondMin = 3, 0xa0
	case first >= 0xe1 && first <= 0xec:
		want = 3
	case first == 0xed:
		want, secondMax = 3, 0x9f
	case first >= 0xee && first <= 0xef:
		want = 3
	case first == 0xf0:
		want, secondMin = 4, 0x90
	case first >= 0xf1 && first <= 0xf3:
		want = 4
	case first == 0xf4:
		want, secondMax = 4, 0x8f
	default:
		return false
	}
	if length >= want {
		return false
	}
	if length >= 2 && (byteAt(1) < secondMin || byteAt(1) > secondMax) {
		return false
	}
	if length > 2 {
		for index := 2; index < length; index++ {
			continuation := byteAt(index)
			if continuation < 0x80 || continuation > 0xbf {
				return false
			}
		}
	}
	return true
}

func mostlyControlRunesInput(length int, decodeRune func(int) (rune, int)) bool {
	if length == 0 {
		return false
	}
	control := 0
	seen := 0
	for offset := 0; offset < length; {
		r, size := decodeRune(offset)
		offset += size
		seen++
		switch r {
		case '\n', '\r', '\t', '\f', '\b', 0x1b:
			continue
		}
		if unicode.IsControl(r) {
			control++
		}
	}
	return seen >= 32 && control*10 > seen
}

func (e TextIssue) Message(lang string, surface Surface) string {
	if isZh(lang) {
		return e.messageZh(surface)
	}
	return e.messageEn(surface)
}

func (e TextIssue) messageZh(surface Surface) string {
	path := quotePath(e.Path)
	reason := e.reasonZh()
	switch e.Kind {
	case KindTrace:
		if e.Reason == "empty" {
			return fmt.Sprintf("附加 trace %s 没有可读取内容。请确认文件不是空文件,或改用文本 systrace/perfetto/atrace 输出。", path)
		}
		if e.lineTooLong() {
			return fmt.Sprintf("附加 trace %s 超出单行解析安全上限(%s)。请重新导出或采集单行长度受限的 UTF-8 trace 文本后再附加；这不是二进制格式判定，系统未自动转换。", path, reason)
		}
		if e.needsTextExport() {
			return fmt.Sprintf("附加 trace %s 看起来不是可解析文本(%s)。该输入是压缩包/归档/数据库容器，不能据此承诺 `trace convert` 可直接处理；请先解包或导出为 UTF-8 `.systrace`/`.perftrace` 后再附加。系统未自动转换。", path, reason)
		}
		if surface == SurfaceREPL {
			return fmt.Sprintf("附加 trace %s 看起来不是可解析文本(%s)。如果这是二进制 Harmony/OpenHarmony HiTrace,请先运行 `/htrace convert <binary-trace-path> [out.systrace]`,再用 `/htrace <out.systrace>` 附加。CLI 可用 `codrax trace convert --input <binary-trace-path>`；请用当前 shell 的路径补全/安全参数引用把路径作为单个参数传入，诊断中的引号不是 shell 语法。", path, reason)
		}
		return fmt.Sprintf("附加 trace %s 看起来不是可解析文本(%s)。如果这是二进制 Harmony/OpenHarmony HiTrace,请先运行 `codrax trace convert --input <binary-trace-path>`,用当前 shell 的路径补全/安全参数引用把路径作为单个参数传入（诊断引号不是 shell 语法）,再用 `--htrace <out.systrace>` 附加转换后的文本。", path, reason)
	default:
		if e.Reason == "empty" {
			return fmt.Sprintf("附加 log %s 没有可读取内容。请确认文件不是空文件。", path)
		}
		return fmt.Sprintf("附加 log %s 看起来不是可解析文本(%s)。请先转换/解码为 UTF-8 文本,再用 `/log` 或 `--log` 附加。", path, reason)
	}
}

func (e TextIssue) messageEn(surface Surface) string {
	path := quotePath(e.Path)
	switch e.Kind {
	case KindTrace:
		if e.Reason == "empty" {
			return fmt.Sprintf("attached trace %s has no readable content. Attach a text systrace/perfetto/atrace file instead.", path)
		}
		if e.lineTooLong() {
			return fmt.Sprintf("attached trace %s exceeds the physical-line parser safety limit (%s). Export or recapture line-bounded UTF-8 trace text before attaching it again; this is not a binary-format verdict, and no automatic conversion was run.", path, e.Reason)
		}
		if e.needsTextExport() {
			return fmt.Sprintf("attached trace %s is not readable text (%s). This is a compressed/archive/database container, so direct `trace convert` support is not promised; unpack or export UTF-8 `.systrace`/`.perftrace` first. No automatic conversion was run.", path, e.Reason)
		}
		if surface == SurfaceREPL {
			return fmt.Sprintf("attached trace %s is not readable text (%s). If this is a binary Harmony/OpenHarmony HiTrace, run `/htrace convert <binary-trace-path> [out.systrace]`, then attach the converted file with `/htrace <out.systrace>`. CLI: `codrax trace convert --input <binary-trace-path>`; pass the path as one argument using shell-native completion/quoting. Diagnostic quotes are not shell syntax.", path, e.Reason)
		}
		return fmt.Sprintf("attached trace %s is not readable text (%s). If this is a binary Harmony/OpenHarmony HiTrace, run `codrax trace convert --input <binary-trace-path>` and pass the path as one argument using shell-native completion/quoting (diagnostic quotes are not shell syntax), then attach the generated .systrace with `--htrace`.", path, e.Reason)
	default:
		if e.Reason == "empty" {
			return fmt.Sprintf("attached log %s has no readable content. Check that the file is not empty.", path)
		}
		return fmt.Sprintf("attached log %s is not readable text (%s). Convert/decode it to UTF-8 text before attaching it with `/log` or `--log`.", path, e.Reason)
	}
}

func (e TextIssue) needsTextExport() bool {
	for _, format := range []BinaryTraceFormat{BinaryTraceFormatGZIP, BinaryTraceFormatZIP, BinaryTraceFormatSQLite} {
		if e.Reason == "known binary trace format: "+string(format) {
			return true
		}
	}
	return false
}

func (e TextIssue) lineTooLong() bool {
	return strings.HasPrefix(strings.TrimSpace(e.Reason), "physical line exceeds ")
}

func (e TextIssue) reasonZh() string {
	switch e.Reason {
	case "contains NUL bytes":
		return "包含 NUL 字节"
	case "not valid UTF-8 text":
		return "不是有效的 UTF-8 文本"
	case "too many non-text control bytes":
		return "控制字符比例过高"
	default:
		if e.lineTooLong() {
			return strings.Replace(strings.TrimSpace(e.Reason), "physical line exceeds ", "物理单行超过 ", 1)
		}
		return e.Reason
	}
}

func quotePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "<inline>"
	}
	return strconv.Quote(path)
}

func isZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}
