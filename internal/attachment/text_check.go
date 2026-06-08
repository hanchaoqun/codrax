package attachment

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
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

const textProbeBytes = 64 * 1024

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

func CheckText(kind Kind, path string, data []byte, truncated bool) *TextIssue {
	if len(data) == 0 {
		if truncated {
			return nil
		}
		return &TextIssue{Kind: kind, Path: path, Reason: "empty"}
	}
	sample := data
	if len(sample) > textProbeBytes {
		sample = sample[:textProbeBytes]
		truncated = true
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return &TextIssue{Kind: kind, Path: path, Reason: "contains NUL bytes"}
	}
	if !validUTF8ForPossiblyTruncated(sample, truncated) {
		return &TextIssue{Kind: kind, Path: path, Reason: "not valid UTF-8 text"}
	}
	if mostlyControlBytes(sample) {
		return &TextIssue{Kind: kind, Path: path, Reason: "too many non-text control bytes"}
	}
	return nil
}

func validUTF8ForPossiblyTruncated(data []byte, truncated bool) bool {
	if utf8.Valid(data) {
		return true
	}
	if !truncated {
		return false
	}
	for trim := 1; trim <= 3 && trim < len(data); trim++ {
		if utf8.Valid(data[:len(data)-trim]) {
			return true
		}
	}
	return false
}

func mostlyControlBytes(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	control := 0
	seen := 0
	for _, b := range data {
		if b >= 0x80 {
			continue
		}
		seen++
		switch b {
		case '\n', '\r', '\t', '\f', '\b', 0x1b:
			continue
		}
		if b < 0x20 || b == 0x7f {
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
		if surface == SurfaceREPL {
			return fmt.Sprintf("附加 trace %s 看起来不是可解析文本(%s)。如果这是二进制 Harmony/OpenHarmony HiTrace,请先运行 `/htrace convert %s [out.systrace]`,再用 `/htrace <out.systrace>` 附加。CLI 可用 `codrax trace convert --input %s`。", path, reason, path, path)
		}
		return fmt.Sprintf("附加 trace %s 看起来不是可解析文本(%s)。如果这是二进制 Harmony/OpenHarmony HiTrace,请先运行 `codrax trace convert --input %s`,再用 `--htrace <out.systrace>` 附加转换后的文本。", path, reason, path)
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
		if surface == SurfaceREPL {
			return fmt.Sprintf("attached trace %s is not readable text (%s). If this is a binary Harmony/OpenHarmony HiTrace, run `/htrace convert %s [out.systrace]`, then attach the converted file with `/htrace <out.systrace>`. CLI: `codrax trace convert --input %s`.", path, e.Reason, path, path)
		}
		return fmt.Sprintf("attached trace %s is not readable text (%s). If this is a binary Harmony/OpenHarmony HiTrace, run `codrax trace convert --input %s`, then attach the generated .systrace with `--htrace`.", path, e.Reason, path)
	default:
		if e.Reason == "empty" {
			return fmt.Sprintf("attached log %s has no readable content. Check that the file is not empty.", path)
		}
		return fmt.Sprintf("attached log %s is not readable text (%s). Convert/decode it to UTF-8 text before attaching it with `/log` or `--log`.", path, e.Reason)
	}
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
