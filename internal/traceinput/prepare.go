// Package traceinput prepares complete physical trace attachments at product
// boundaries. Parsers continue to accept only query-ready text or bundles.
package traceinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const defaultPreviewBytes = 2 << 20

type Options struct {
	InputPath             string
	RuntimeAnchor         string
	RuntimeAnchorFallback string
	PreviewBytes          int
	Progress              hitraceconv.ProgressFunc
}

// Error describes a preparation boundary failure, not a decoder capability.
// Converter errors are preserved as causes rather than reclassified as text.
type Error struct {
	Code string
	Path string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("prepare trace %q: %s: %v", e.Path, e.Code, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

type converter func(context.Context, hitraceconv.Options) (hitraceconv.Result, error)

// Prepare never truncates conversion or query material. PreviewBytes is a cap
// on the entire model envelope, including its source and truncation headers.
func Prepare(ctx context.Context, opts Options) (*attachment.TraceMaterial, error) {
	return prepare(ctx, opts, hitraceconv.ConvertFile)
}

func prepare(ctx context.Context, opts Options, convert converter) (material *attachment.TraceMaterial, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.PreviewBytes < 0 {
		return nil, fmt.Errorf("trace preview limit is negative")
	}
	if opts.PreviewBytes == 0 {
		opts.PreviewBytes = defaultPreviewBytes
	}
	if err := attachment.ValidateSourceLabel(opts.InputPath); err != nil {
		return nil, err
	}
	source, err := filepath.Abs(opts.InputPath)
	if err != nil {
		return nil, err
	}
	source = filepath.Clean(source)
	held, original, err := filegeneration.OpenRegularReadOnly(source)
	if err != nil {
		return nil, err
	}
	var owned *managedDirectory
	defer func() {
		// A source replacement dominates any provisional format verdict.
		if identityErr := validateHeld(source, held, original); identityErr != nil {
			err = identityErr
		}
		err = errors.Join(err, ctx.Err(), held.Close())
		if err == nil && owned != nil {
			// Successful publications outlive preparation, but their directory
			// handles do not. A failed validation retains cleanup authority;
			// a failed handle release is terminal and cannot delete by path.
			err = owned.close()
		}
		if err != nil {
			material = nil
			if owned != nil {
				err = errors.Join(err, owned.cleanup())
			}
		}
	}()
	probeSize := original.Size()
	if probeSize > attachment.TextProbeBytes {
		probeSize = attachment.TextProbeBytes
	}
	probe := make([]byte, int(probeSize))
	if _, err := io.ReadFull(io.NewSectionReader(held, 0, probeSize), probe); err != nil {
		return nil, err
	}
	kind, binary := binaryCandidate(probe)
	if kind == string(attachment.BinaryTraceFormatSQLite) {
		return nil, &Error{Code: "sqlite_export_required", Path: source, Err: fmt.Errorf("existing SQLite databases require an explicit text export; automatic database intake is not supported")}
	}
	bindings := map[string]filegeneration.Identity{source: original}
	if !binary {
		preview, complete, err := previewFromHeld(ctx, source, source, held, original, opts.PreviewBytes, "")
		if err != nil {
			return nil, err
		}
		// Existing bundles remain text inputs, but their entire admitted child
		// universe must be pinned, not just the visible JSON prefix.
		if strings.HasSuffix(strings.ToLower(source), ".tracebundle.json") {
			complete = false
			if err := bindBundle(ctx, source, bindings); err != nil {
				return nil, err
			}
		}
		return bind(ctx, source, source, preview, bindings, complete)
	}
	_, sourceSHA, measured, err := tracebundle.MeasureFile(ctx, held)
	if err != nil {
		return nil, err
	}
	if !original.SameVersion(measured) {
		return nil, fmt.Errorf("trace source changed before conversion: %q", source)
	}
	if err := validateHeld(source, held, original); err != nil {
		return nil, err
	}
	owned, err = newManagedDirectory(opts.RuntimeAnchor, opts.RuntimeAnchorFallback)
	if err != nil {
		return nil, err
	}
	directPerf := kind == string(attachment.BinaryTraceFormatLinuxPerf) ||
		kind == string(attachment.BinaryTraceFormatGZIP) || kind == "simpleperf_report_sample_proto"
	if opts.Progress != nil {
		opts.Progress(hitraceconv.ProgressEvent{
			Stage: "trace_prepare", Status: hitraceconv.ProgressStatusStarted,
			Message: "converting complete trace capture", Path: source,
			OutputPath: filepath.Join(owned.path, "capture.systrace"), BytesTotal: original.Size(),
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, err := convert(ctx, hitraceconv.Options{
		InputPath: source, OutputPath: filepath.Join(owned.path, "capture.systrace"),
		TraceEngine: "auto",
		// DB retention is a trace-only option; direct perf families have no
		// scheduling DB and the converter correctly refuses that combination.
		KeepTraceDB:   !directPerf,
		RuntimeAnchor: opts.RuntimeAnchor, RuntimeAnchorFallback: opts.RuntimeAnchorFallback,
		Progress: opts.Progress,
	})
	if err != nil {
		return nil, &Error{Code: "conversion_failed", Path: source, Err: err}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := owned.validate(); err != nil {
		return nil, err
	}
	previewPath := hitraceconv.QueryReadySystracePath(result)
	if previewPath == "" {
		previewPath = hitraceconv.QueryReadyPerfTracePath(result.Artifacts)
	}
	if previewPath == "" {
		return nil, &Error{Code: "no_query_ready_material", Path: source, Err: fmt.Errorf("conversion produced inventory only, without a receipt-approved queryable trace or sample artifact")}
	}
	queryPath := previewPath
	if result.BundlePath != "" {
		queryPath = result.BundlePath
	}
	for _, artifact := range result.Artifacts {
		if err := bindArtifact(ctx, artifact, bindings); err != nil {
			return nil, err
		}
	}
	if result.BundlePath != "" {
		if err := bindBundle(ctx, result.BundlePath, bindings); err != nil {
			return nil, err
		}
	} else if err := tracequery.ValidateTraceInputPath(ctx, queryPath); err != nil {
		return nil, err
	}
	if _, ok := bindings[queryPath]; !ok {
		return nil, fmt.Errorf("query material has no converter artifact binding: %q", queryPath)
	}
	receiptPath := filepath.Join(owned.path, "preparation.json")
	if err := writeReceipt(ctx, receiptPath, preparationReceipt{
		Version: "traceinput-v1", SourcePath: source, SourceKind: kind,
		SourceBytes: original.Size(), SourceSHA256: sourceSHA, SourceGeneration: original.CacheToken(),
		QueryPath: queryPath, PreviewPath: previewPath, Conversion: result,
	}, bindings); err != nil {
		return nil, err
	}
	previewFile, previewID, err := filegeneration.OpenRegularReadOnly(previewPath)
	if err != nil {
		return nil, err
	}
	if prior, ok := bindings[previewPath]; !ok || !prior.SameVersion(previewID) {
		return nil, errors.Join(fmt.Errorf("converted preview generation changed"), previewFile.Close())
	}
	preview, _, previewErr := previewFromHeld(ctx, previewPath, queryPath, previewFile, previewID, opts.PreviewBytes, receiptPath)
	previewErr = errors.Join(previewErr, validateHeld(previewPath, previewFile, previewID), previewFile.Close())
	if previewErr != nil {
		return nil, previewErr
	}
	if err := owned.validate(); err != nil {
		return nil, err
	}
	return bind(ctx, source, queryPath, preview, bindings, false)
}

// These are candidate families only. Version, member, payload, endian and
// provider support are determined by the existing converter's strict intake.
func binaryCandidate(probe []byte) (string, bool) {
	if bytes.HasPrefix(probe, []byte("SIMPLEPERF")) {
		return "simpleperf_report_sample_proto", true
	}
	if bytes.HasPrefix(probe, []byte{0x49, 0xdf}) {
		return "openharmony_raw", true
	}
	format := attachment.KnownBinaryTraceFormat(probe)
	return string(format), format != attachment.BinaryTraceFormatUnknown
}

func bind(ctx context.Context, source, query, preview string, bindings map[string]filegeneration.Identity, completeText bool) (*attachment.TraceMaterial, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var material *attachment.TraceMaterial
	var err error
	if completeText && source == query {
		material, err = attachment.BindCompleteTextTraceMaterial(source, preview, bindings)
	} else {
		material, err = attachment.BindTraceMaterial(source, query, preview, bindings)
	}
	if err != nil {
		return nil, err
	}
	if err := material.Validate(ctx, preview); err != nil {
		return nil, err
	}
	return material, nil
}

func previewFromHeld(ctx context.Context, path, query string, held *os.File, id filegeneration.Identity, limit int, receiptPath string) (string, bool, error) {
	if err := attachment.ValidateTextReaderAtFull(ctx, attachment.KindTrace, path, held, id.Size(), attachment.TracePhysicalLineMaxBytes); err != nil {
		return "", false, err
	}
	if err := attachment.ValidateSourceLabel(query); err != nil {
		return "", false, err
	}
	header := "# codrax-source: " + query + "\n"
	if receiptPath != "" {
		header += "# codrax-conversion-receipt: " + receiptPath + "\n"
	}
	truncated := id.Size() > int64(limit-len(header))
	if truncated {
		header += "# codrax-preview: truncated; full query material retained\n"
	}
	budget := limit - len(header)
	if budget <= 0 {
		return "", false, fmt.Errorf("trace preview limit %d cannot fit provenance headers and trace content", limit)
	}
	want := id.Size()
	if want > int64(budget) {
		want = int64(budget)
	}
	body := make([]byte, int(want))
	if _, err := io.ReadFull(io.NewSectionReader(held, 0, want), body); err != nil {
		return "", false, err
	}
	body, err := attachment.ValidatePublishableText(attachment.KindTrace, path, body, truncated)
	if err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return header + string(body), !truncated && int64(len(body)) == id.Size(), nil
}

func validateHeld(path string, held *os.File, original filegeneration.Identity) error {
	current, err := filegeneration.FromFile(held)
	if err != nil {
		return err
	}
	bound, err := filegeneration.FromPath(path)
	if err != nil {
		return err
	}
	if !original.SameVersion(current) || !original.SameVersion(bound) {
		return fmt.Errorf("trace source generation changed during preparation: %q", path)
	}
	return nil
}
