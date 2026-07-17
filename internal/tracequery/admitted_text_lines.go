package tracequery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// AdmittedTraceTextLine is one physical line from a trace generation that
// passed the shared full-file text admission policy. Text excludes trailing
// CR/LF bytes; Number is one-based and refers to the physical source file.
type AdmittedTraceTextLine struct {
	Number int
	Text   string
}

// AdmittedTraceTextScan describes how far StreamAdmittedTraceTextLines read.
// Stopped is true only when the visitor deliberately returned false.
type AdmittedTraceTextScan struct {
	ScannedLines int
	Stopped      bool
}

// StreamAdmittedTraceTextLines safely streams raw physical trace text without
// entering the event parser. It is the path-based authority for lightweight
// marker/census consumers that still need all trace-input correctness gates:
//
//   - platform-safe nonblocking open and regular-file proof;
//   - full-generation binary/UTF-8/control/physical-line admission;
//   - a SectionReader frozen to the admitted byte size;
//   - the shared 16 MiB physical-line ceiling; and
//   - final held-generation plus pathname-binding validation, including when
//     the visitor stops early or the scan itself fails.
//
// The visitor is synchronous and must not retain unbounded source text.
func StreamAdmittedTraceTextLines(
	ctx context.Context,
	path string,
	visit func(AdmittedTraceTextLine) bool,
) (AdmittedTraceTextScan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if visit == nil {
		return AdmittedTraceTextScan{}, fmt.Errorf("admitted trace text scan: visitor is nil")
	}
	if strings.TrimSpace(path) == "" {
		return AdmittedTraceTextScan{}, fmt.Errorf("admitted trace text scan: path is empty")
	}
	if err := ctx.Err(); err != nil {
		return AdmittedTraceTextScan{}, err
	}

	file, openedIdentity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return AdmittedTraceTextScan{}, err
	}
	reader := bufio.NewReaderSize(io.NewSectionReader(file, 0, openedIdentity.Size()), 256*1024)
	var result AdmittedTraceTextScan
	var scanErr error
	for lineNo := 1; ; lineNo++ {
		if err := ctx.Err(); err != nil {
			scanErr = err
			break
		}
		line, readErr := readStreamScanPhysicalLine(reader, attachment.TracePhysicalLineMaxBytes)
		if len(line) > 0 {
			result.ScannedLines = lineNo
			if !visit(AdmittedTraceTextLine{
				Number: lineNo,
				Text:   strings.TrimRight(line, "\r\n"),
			}) {
				result.Stopped = true
				break
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				scanErr = readErr
			}
			break
		}
	}

	identityErr := validateTraceFileIdentityAfterRead(file, openedIdentity, "admitted trace text scan")
	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close admitted trace text source %s: %w", path, closeErr)
	}
	return result, errors.Join(scanErr, identityErr, closeErr)
}
