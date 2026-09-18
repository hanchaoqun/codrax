package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
)

// loadAttachedLog returns the merged --log / --log-text payload
// (size-capped at log_attach_max_bytes). Trace attachments are
// loaded separately by loadAttachedTrace and bounded by the
// independent trace_attach_max_bytes cap.
//
// Rules:
//   - --log (slice) and --log-text are mutually exclusive when both
//     are non-empty
//   - exactly one `-` is allowed across BOTH --log and the trace
//     entries combined (only one stdin consumer per Run); the
//     stdin gate is enforced in this function before any read
//   - multiple --log entries are concatenated in CLI order, each
//     prefixed by a `# codrax-source: <path>\n` header so the LLM
//     can distinguish file boundaries (e.g. multi-process panics)
//   - the combined byte length is capped at maxAttachedLogBytes;
//     excess bytes are tail-truncated with a WARN
func loadAttachedLog() (string, error) {
	if len(flagAttachLog) > 0 && flagAttachLogText != "" {
		return "", fmt.Errorf("--log and --log-text are mutually exclusive")
	}
	if err := enforceStdinExclusivity(); err != nil {
		return "", err
	}
	body, err := loadMultiPathSlice("log", flagAttachLog, flagAttachLogText, maxAttachedLogBytes)
	if err != nil {
		return "", err
	}
	if body == "" {
		return "", nil
	}
	return truncateAttachedToCap(body, maxAttachedLogBytes, "log"), nil
}

// loadAttachedTrace retains the historical preview-only helper signature.
// Product callers use loadPreparedAttachedTrace and transport its complete
// material receipt alongside the bounded preview.
func loadAttachedTrace() (string, string, error) {
	loaded, err := loadPreparedAttachedTrace(context.Background(), nil)
	return loaded.body, loaded.source, err
}

type cliPreparedTrace struct {
	body     string
	source   string
	material *attachment.TraceMaterial
}

var prepareAttachedTraceInput = traceinput.Prepare

// loadPreparedAttachedTrace returns one physical trace and its query authority.
// Accepts any combination of --htrace / --atrace (slices) plus
// --htrace-text / --atrace-text (single inline strings). Within-
// flavour (file vs inline) and cross-flavour (htrace vs atrace) are
// mutually exclusive when both are non-empty. Repeating --htrace or
// --atrace with multiple physical captures is rejected: flattening
// their independent clocks and provenance would create a synthetic
// causal timeline. Multi-trace work uses independently named paths or
// one provenance-carrying tracebundle.
func loadPreparedAttachedTrace(ctx context.Context, progress hitraceconv.ProgressFunc) (cliPreparedTrace, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return cliPreparedTrace{}, err
	}
	if len(flagAttachHitrace) > 0 && flagAttachHitraceText != "" {
		return cliPreparedTrace{}, fmt.Errorf("--htrace and --htrace-text are mutually exclusive")
	}
	if len(flagAttachAtrace) > 0 && flagAttachAtraceText != "" {
		return cliPreparedTrace{}, fmt.Errorf("--atrace and --atrace-text are mutually exclusive")
	}
	if (len(flagAttachHitrace) > 0 || flagAttachHitraceText != "") &&
		(len(flagAttachAtrace) > 0 || flagAttachAtraceText != "") {
		return cliPreparedTrace{}, fmt.Errorf("--htrace and --atrace are aliases — set only one")
	}
	// Promote --atrace* into --htrace* so the rest of the loader only
	// sees one shape.
	paths := flagAttachHitrace
	text := flagAttachHitraceText
	source := "harmony_hitrace"
	if len(flagAttachAtrace) > 0 {
		paths = flagAttachAtrace
		source = "android_atrace"
	}
	if flagAttachAtraceText != "" {
		text = flagAttachAtraceText
		source = "android_atrace"
	}
	if err := enforceStdinExclusivity(); err != nil {
		return cliPreparedTrace{}, err
	}
	if len(paths) > 1 {
		return cliPreparedTrace{}, fmt.Errorf("multiple physical trace attachments cannot be flattened into one causal timeline; name each path in the question or use a provenance-carrying .tracebundle.json")
	}
	if len(paths) == 1 && paths[0] != "-" {
		anchor, err := filepath.Abs(runtimeAnchorDir)
		if err != nil {
			return cliPreparedTrace{}, fmt.Errorf("resolve trace attachment runtime anchor: %w", err)
		}
		material, err := prepareCLITraceFile(ctx, traceinput.Options{
			InputPath: paths[0], RuntimeAnchor: anchor,
			RuntimeAnchorFallback: traceConvertWSLRuntimeAnchorFallback(anchor),
			PreviewBytes:          maxAttachedTraceBytes, Progress: progress,
		})
		if err != nil {
			return cliPreparedTrace{}, fmt.Errorf("prepare attached trace %q: %w", paths[0], err)
		}
		if material == nil {
			return cliPreparedTrace{}, fmt.Errorf("prepare attached trace %q returned no query material", paths[0])
		}
		if err := material.Validate(ctx, material.Preview()); err != nil {
			return cliPreparedTrace{}, err
		}
		return cliPreparedTrace{body: material.Preview(), source: source, material: material}, nil
	}
	// Inline and stdin remain bounded text protocols. In particular, a
	// truncated stdin prefix must never be passed to a binary converter.
	body, err := loadMultiPathSlice("trace", paths, text, maxAttachedTraceBytes)
	if err != nil {
		return cliPreparedTrace{}, err
	}
	if err := ctx.Err(); err != nil {
		return cliPreparedTrace{}, err
	}
	if body == "" {
		return cliPreparedTrace{}, nil
	}
	return cliPreparedTrace{body: truncateAttachedToCap(body, maxAttachedTraceBytes, "trace"), source: source}, nil
}

// The process-wide worktree handler otherwise exits without running the
// conversion transaction's defers. Give preparation sole signal ownership
// until it has returned and completed rollback; do not add a wall-clock limit.
func prepareCLITraceFile(parent context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
	worktree.SetSignalHandlerSuppressed(true)
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer func() {
		stop()
		worktree.SetSignalHandlerSuppressed(false)
	}()
	material, err := prepareAttachedTraceInput(ctx, opts)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, errors.Join(err, ctxErr)
	}
	return material, err
}

func cliPreparedTraceLoadedLines(lang string, material *attachment.TraceMaterial) []string {
	if material == nil {
		return nil
	}
	converted := material.SourcePath() != material.QueryPath()
	if cliStatusUseChinese(lang) {
		line := fmt.Sprintf("✓ trace 文件已就绪：%s", material.QueryPath())
		if converted {
			line = fmt.Sprintf("✓ trace 已自动转换：%s → %s", material.SourcePath(), material.QueryPath())
		}
		return []string{line, fmt.Sprintf("· 模型预览有界（%d 字节）；trace 查询使用完整材料，不受预览上限截断。", len(material.Preview()))}
	}
	line := fmt.Sprintf("✓ trace file ready: %s", material.QueryPath())
	if converted {
		line = fmt.Sprintf("✓ trace automatically converted: %s → %s", material.SourcePath(), material.QueryPath())
	}
	return []string{line, fmt.Sprintf("· Bounded model preview (%d bytes); trace queries use the complete material, unaffected by the preview limit.", len(material.Preview()))}
}

// enforceStdinExclusivity checks that at most one `-` appears across
// every repeated --log / --htrace / --atrace entry. Returns an error
// when two or more would consume stdin (impossible on a single
// process). Called from both loaders before any actual read so the
// failure is reported once at flag-validation time rather than on
// the second read where stdin is already drained.
func enforceStdinExclusivity() error {
	stdinCount := 0
	for _, p := range flagAttachLog {
		if p == "-" {
			stdinCount++
		}
	}
	for _, p := range flagAttachHitrace {
		if p == "-" {
			stdinCount++
		}
	}
	for _, p := range flagAttachAtrace {
		if p == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		return fmt.Errorf("only one of --log/--htrace/--atrace can consume stdin (`-`) per run; got %d", stdinCount)
	}
	return nil
}

// loadMultiPathSlice reads every entry in `paths` (or returns
// `inlineText` when the slice is empty), prefixing each file body
// with `# codrax-source: <path>\n`. Per-file stdin read is bounded
// by `cap` (channel-specific) so a single oversized file cannot OOM
// the process; the final aggregate cap applies in
// truncateAttachedToCap. `kind` is "log" / "trace" for error context.
func loadMultiPathSlice(kind string, paths []string, inlineText string, cap int) (string, error) {
	if inlineText != "" {
		if err := attachment.ValidateText(attachment.Kind(kind), "--"+kind+"-text", []byte(inlineText), false); err != nil {
			return "", err
		}
		return inlineText, nil
	}
	if len(paths) == 0 {
		return "", nil
	}
	if kind == "trace" && len(paths) > 1 {
		return "", fmt.Errorf("multiple physical trace attachments cannot be flattened into one causal timeline; name each path in the question or use a provenance-carrying .tracebundle.json")
	}
	if cap <= 0 {
		cap = defaultAttachedLogMaxBytes
	}
	var b strings.Builder
	for _, p := range paths {
		if err := attachment.ValidateSourceLabel(p); err != nil {
			return "", fmt.Errorf("load attached %s source label %q: %w", kind, p, err)
		}
		// Header keeps file boundaries visible to the LLM. Single-
		// path attachments still get a header for symmetry; the
		// log-triage / perf-triage skill prompts reference the
		// `# codrax-source:` token by literal so this is part of
		// the contract, not just decoration.
		separatorBytes := 0
		if b.Len() > 0 {
			separatorBytes = 1
		}
		header := fmt.Sprintf("# codrax-source: %s\n", p)
		visibleRemaining := cap - b.Len()
		if visibleRemaining < separatorBytes || visibleRemaining-separatorBytes <= len(header) {
			return "", fmt.Errorf("attached %s cap %d cannot fit source header plus at least 1 content byte for %q", kind, cap, p)
		}
		if separatorBytes != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(header)
		remaining := cap - b.Len() + 1
		data, _, err := readAttachedSourceLimited(attachment.Kind(kind), p, remaining)
		if err != nil {
			return "", fmt.Errorf("load attached %s %q: %w", kind, p, err)
		}
		b.Write(data)
	}
	return b.String(), nil
}

func readAttachedSourceLimited(kind attachment.Kind, path string, limit int) ([]byte, bool, error) {
	if limit <= 0 {
		return nil, false, nil
	}
	if path != "-" {
		return attachment.ReadTextFileLimited(kind, path, limit)
	}
	readBudget := limit
	if readBudget < attachment.TextProbeBytes {
		readBudget = attachment.TextProbeBytes
	}
	if readBudget == int(^uint(0)>>1) {
		return nil, false, fmt.Errorf("attached %s stdin read budget overflows", kind)
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, int64(readBudget+1)))
	if err != nil {
		return nil, false, err
	}
	probeLen := len(raw)
	if probeLen > attachment.TextProbeBytes {
		probeLen = attachment.TextProbeBytes
	}
	if err := attachment.ValidateText(kind, "-", raw[:probeLen], len(raw) > probeLen); err != nil {
		return nil, false, err
	}
	payloadLen := len(raw)
	if payloadLen > limit {
		payloadLen = limit
	}
	data := raw[:payloadLen]
	truncated := len(raw) > payloadLen
	data, err = attachment.ValidatePublishableText(kind, "-", data, truncated)
	if err != nil {
		return nil, false, err
	}
	return data, truncated, nil
}

func truncateAttachedLog(s string) string {
	return truncateAttachedToCap(s, maxAttachedLogBytes, "log")
}

// truncateAttachedToCap is the per-channel truncation entry point.
// Mirrors truncateAttachedLog but parameterised by cap + a label so
// the WARN message names the right channel ("log" vs "trace") when
// the caps differ.
func truncateAttachedToCap(s string, cap int, kind string) string {
	if len(s) <= cap {
		return s
	}
	logging.Warning("[cmd] attached %s truncated: %d → %d bytes", kind, len(s), cap)
	return types.CutPrefixRuneSafe(s, cap)
}
