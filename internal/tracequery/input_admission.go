package tracequery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

const (
	// TraceInputAdmissionCodeConversionRequired is the stable, typed verdict
	// for a binary/non-text physical trace. Callers may render recovery from
	// this code; they must not recover it from Error text.
	TraceInputAdmissionCodeConversionRequired = "trace_conversion_required"
	// TraceInputAdmissionCodeTextExportRequired covers generic compressed,
	// archive, and database containers. Their magic proves non-text bytes, not
	// that codrax trace convert supports the container as a capture input.
	TraceInputAdmissionCodeTextExportRequired = "trace_text_export_required"
	// TraceInputAdmissionCodeEmpty is deliberately separate: converting an
	// empty capture cannot restore missing bytes, so its recovery is recapture,
	// not trace convert.
	TraceInputAdmissionCodeEmpty = "trace_input_empty"
	// TraceInputAdmissionCodeLineTooLong rejects otherwise-text input whose
	// physical line exceeds the bounded parser safety contract.
	TraceInputAdmissionCodeLineTooLong = "trace_physical_line_too_long"
	// TraceInputAdmissionCodeSourceUnavailable covers a physical source that
	// cannot be safely frozen as the regular, generation-bound input universe
	// required by trace_query (FIFO/device/named pipe, invalid explicit
	// manifest, or generation/identity failure). It is intentionally distinct
	// from conversion_required: conversion cannot repair an unsafe pathname or
	// provenance control document.
	TraceInputAdmissionCodeSourceUnavailable = "trace_input_source_unavailable"
)

// TraceInputAdmissionError is the fail-closed trace-text admission verdict.
// It is minted only from attachment's shared held-reader authority and always
// names the physical member that failed (including a tracebundle child).
type TraceInputAdmissionError struct {
	Code   string
	Path   string
	Reason string
}

func (e *TraceInputAdmissionError) Error() string {
	if e == nil {
		return "trace input admission failed"
	}
	path := strings.TrimSpace(e.Path)
	quoted := strconv.Quote(path)
	if path == "" {
		quoted = "<binary-trace-path>"
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "non-text trace input"
	}
	if e.Code == TraceInputAdmissionCodeEmpty {
		return fmt.Sprintf("trace input %s was rejected before trace investigation: code=%s reason=%s; the capture is empty, so collect a non-empty text trace and retry trace_query", quoted, TraceInputAdmissionCodeEmpty, reason)
	}
	if e.Code == TraceInputAdmissionCodeTextExportRequired {
		return fmt.Sprintf("trace input %s was rejected before trace investigation: code=%s reason=%s; its content identifies a compressed/archive/database container, not queryable trace text; unpack or export a UTF-8 .systrace/.perftrace first (automatic conversion was not run)", quoted, TraceInputAdmissionCodeTextExportRequired, reason)
	}
	if e.Code == TraceInputAdmissionCodeLineTooLong {
		return fmt.Sprintf("trace input %s was rejected before trace investigation: code=%s reason=%s; export or recapture line-bounded UTF-8 trace text before retrying (automatic conversion was not run)", quoted, TraceInputAdmissionCodeLineTooLong, reason)
	}
	if e.Code == TraceInputAdmissionCodeSourceUnavailable {
		return fmt.Sprintf("trace input %s was rejected before trace investigation: code=%s reason=%s; provide an existing regular, stable trace file (or repair the explicit tracebundle manifest and its bound children) before retrying; conversion was not attempted", quoted, TraceInputAdmissionCodeSourceUnavailable, reason)
	}
	return fmt.Sprintf("trace input %s was rejected before trace investigation: code=%s reason=%s; its held content was classified as binary/non-text and this rejected input was not parsed; try `codrax trace convert --input <binary-trace-path>`, passing the path as one argv value via shell-native completion/quoting (diagnostic quotes are not shell syntax), then query a generated query-ready .tracebundle.json or .systrace", quoted, TraceInputAdmissionCodeConversionRequired, reason)
}

// TraceInputAdmissionCodeForReason is the single recovery classifier shared by
// held physical inputs and direct in-memory tool attachments.
func TraceInputAdmissionCodeForReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "empty" {
		return TraceInputAdmissionCodeEmpty
	}
	if strings.HasPrefix(reason, "physical line exceeds ") {
		return TraceInputAdmissionCodeLineTooLong
	}
	for _, format := range []string{
		string(attachment.BinaryTraceFormatGZIP),
		string(attachment.BinaryTraceFormatZIP),
		string(attachment.BinaryTraceFormatSQLite),
	} {
		if reason == "known binary trace format: "+format {
			return TraceInputAdmissionCodeTextExportRequired
		}
	}
	return TraceInputAdmissionCodeConversionRequired
}

// validateHeldTraceInput is the single tracequery-side adapter around the
// shared attachment text authority. It probes the already-open descriptor via
// ReaderAt (so the parser offset stays unchanged) and publishes a content
// verdict only after proving that the same held generation survived the read.
func validateHeldTraceInput(ctx context.Context, file *os.File, identity traceFileIdentity, displayPath string, allowColdRead bool) error {
	if file == nil {
		return fmt.Errorf("trace input admission: held file is nil")
	}
	probeErr := traceInputAdmissions.validate(ctx, file, identity, displayPath, allowColdRead)
	if probeErr == nil {
		return nil
	}
	var issue attachment.TextIssue
	if !errors.As(probeErr, &issue) {
		return fmt.Errorf("trace input admission probe failed for %s: %w", displayPath, probeErr)
	}
	return &TraceInputAdmissionError{
		Code:   TraceInputAdmissionCodeForReason(issue.Reason),
		Path:   firstNonEmptyInputAdmissionString(issue.Path, displayPath),
		Reason: issue.Reason,
	}
}

// validateHeldTraceFileIdentityAfterRead deliberately compares only the live
// descriptor generation. Held converter validation owns a sealed descriptor,
// not file.Name(); reopening that advisory pathname would reintroduce the very
// authority the held API excludes. Path-based readers add their separate
// binding check in openTraceSourceRegular.
func validateHeldTraceFileIdentityAfterRead(file *os.File, opened traceFileIdentity, operation string) error {
	if file == nil || !opened.Initialized() {
		return fmt.Errorf("trace held source identity unavailable after %s", operation)
	}
	finalIdentity, err := traceFileIdentityFromFile(file)
	if err != nil {
		return fmt.Errorf("trace held source identity check after %s: %w", operation, err)
	}
	if !opened.SameVersion(finalIdentity) {
		return fmt.Errorf("trace held source identity changed during %s; discard mixed-version streaming results and retry", operation)
	}
	return nil
}

// ValidateTraceInputPath freezes the same source universe BuildIndex would
// consume and validates every physical member before any query/index parser is
// entered. V2 child digest work uses the existing generation-keyed cache, so a
// subsequent BuildIndex reuses the attestation instead of hashing twice.
// Optional sibling-bundle failures retain the existing resolver semantics:
// the explicitly requested text artifact remains usable; explicit bundles
// fail closed on an invalid child.
func ValidateTraceInputPath(ctx context.Context, path string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("trace input admission: path is empty")
	}
	selection, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return normalizeTraceInputAdmissionError(path, err)
	}
	if err := selection.validate(ctx); err != nil {
		return normalizeTraceInputAdmissionError(path, selection.closeAfter(err))
	}
	return normalizeTraceInputAdmissionError(path, selection.close())
}

func normalizeTraceInputAdmissionError(path string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var admission *TraceInputAdmissionError
	if errors.As(err, &admission) {
		return err
	}
	return sourceUnavailableTraceInputError(path, err)
}

func sourceUnavailableTraceInputError(path string, err error) error {
	reason := "trace source could not be safely admitted"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		reason = err.Error()
	}
	return &TraceInputAdmissionError{
		Code:   TraceInputAdmissionCodeSourceUnavailable,
		Path:   strings.TrimSpace(path),
		Reason: reason,
	}
}

func firstNonEmptyInputAdmissionString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
