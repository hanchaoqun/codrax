package hitraceconv

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	codraxtypes "github.com/hanchaoqun/codrax/internal/types"
)

const embeddedTraceStreamerRuntimeProbeTimeout = 5 * time.Second

// Test seam. Production always parses the verified embedded child and invokes
// only its declared system loader with --list; it never executes trace work.
var embeddedTraceStreamerRuntimeProbe = probeEmbeddedTraceStreamerRuntime

func probeEmbeddedTraceStreamerRuntime(childPath string, platform embeddedTraceStreamerPlatform) error {
	if runtime.GOOS != "linux" || strings.TrimSpace(platform.GOOS) != "linux" {
		return nil
	}
	interpreter, err := embeddedTraceStreamerELFInterpreter(childPath)
	if err != nil {
		return fmt.Errorf("elf_invalid: %w", err)
	}
	if !filepath.IsAbs(interpreter) {
		return fmt.Errorf("loader_invalid: PT_INTERP is not absolute: %q", interpreter)
	}
	info, err := os.Stat(interpreter)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("loader_missing: %s", interpreter)
		}
		return fmt.Errorf("loader_stat_failed: %s: %w", interpreter, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("loader_invalid: %s is not a regular file", interpreter)
	}

	ctx, cancel := context.WithTimeout(context.Background(), embeddedTraceStreamerRuntimeProbeTimeout)
	defer cancel()
	command, err := newExternalToolCommand(ctx, interpreter, "--list", childPath)
	if err != nil {
		return fmt.Errorf("loader_supervisor_failed: %w", err)
	}
	// Status/preflight and the real embedded-child execution share this clean
	// loader environment. A static-capable parent must not map caller-supplied
	// dynamic objects merely because status was requested.
	command.setEnvironment(embeddedTraceStreamerRuntimeEnvironment(os.Environ()))
	output, runErr, _, _ := runCommandWithProgressUntilExit(Options{}, command, "", "")
	text := strings.TrimSpace(string(output))
	return embeddedTraceStreamerRuntimeProbeResult(ctx.Err(), runErr, text, interpreter)
}

func embeddedTraceStreamerRuntimeEnvironment(environment []string) []string {
	out := make([]string, 0, len(environment))
	for _, item := range environment {
		name, _, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if strings.HasPrefix(name, "LD_") || name == "GLIBC_TUNABLES" ||
			name == "LANG" || name == "LANGUAGE" || name == "LC_ALL" || strings.HasPrefix(name, "LC_") {
			continue
		}
		out = append(out, item)
	}
	out = append(out, "LANG=C", "LC_ALL=C")
	return out
}

func embeddedTraceStreamerRuntimeProbeResult(contextErr, runErr error, output, interpreter string) error {
	if errors.Is(contextErr, context.DeadlineExceeded) {
		return fmt.Errorf("probe_timeout: loader=%s timeout=%s", interpreter, embeddedTraceStreamerRuntimeProbeTimeout)
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "=> not found") || strings.Contains(lower, "cannot open shared object file") {
		return fmt.Errorf("shared_library_missing: %s", boundedEmbeddedRuntimeReason(output))
	}
	if strings.Contains(lower, "version `glibc_") && strings.Contains(lower, "not found") {
		return fmt.Errorf("glibc_too_old_or_symbol_missing: %s", boundedEmbeddedRuntimeReason(output))
	}
	if runErr != nil {
		return fmt.Errorf("loader_probe_failed: %v output=%s", runErr, boundedEmbeddedRuntimeReason(output))
	}
	return nil
}

func embeddedTraceStreamerELFInterpreter(filePath string) (string, error) {
	file, err := elf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open ELF: %w", err)
	}
	defer file.Close()
	for _, program := range file.Progs {
		if program.Type != elf.PT_INTERP {
			continue
		}
		if program.Filesz == 0 || program.Filesz > 4096 {
			return "", fmt.Errorf("PT_INTERP size=%d is invalid", program.Filesz)
		}
		body, err := io.ReadAll(io.LimitReader(program.Open(), int64(program.Filesz)))
		if err != nil {
			return "", fmt.Errorf("read PT_INTERP: %w", err)
		}
		interpreter := strings.TrimSpace(strings.TrimRight(string(body), "\x00"))
		if interpreter == "" {
			return "", fmt.Errorf("PT_INTERP is empty")
		}
		return interpreter, nil
	}
	return "", fmt.Errorf("PT_INTERP is missing")
}

func boundedEmbeddedRuntimeReason(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	var safe strings.Builder
	for _, character := range value {
		switch character {
		case '\n':
			safe.WriteString(`\n`)
		case '\r':
			safe.WriteString(`\r`)
		case '\t':
			safe.WriteString(`\t`)
		default:
			if unicode.IsControl(character) {
				fmt.Fprintf(&safe, `\u{%x}`, character)
				continue
			}
			safe.WriteRune(character)
		}
	}
	value = safe.String()
	const max = 1024
	if len(value) <= max {
		return value
	}
	const marker = "…"
	return codraxtypes.CutPrefixRuneSafe(value, max-len(marker)) + marker
}
