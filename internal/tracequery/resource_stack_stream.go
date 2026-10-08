package tracequery

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const (
	resourceStackStreamRecordLimit = 65536
	resourceStackStreamByteLimit   = 32 << 20
)

// StreamResourceStack retains only the native resource carrier, not the main
// trace index. It scans the complete frozen source: even frames physically
// outside the requested window must participate in event/frame integrity.
func StreamResourceStack(ctx context.Context, path string, q Query) (Result, error) {
	return streamResourceStack(ctx, path, q, resourceStackStreamRecordLimit, resourceStackStreamByteLimit)
}

func streamResourceStack(ctx context.Context, path string, q Query, recordLimit, byteLimit int) (Result, error) {
	if CanonicalViewName(q.View) != ViewResourceStack {
		return Result{}, fmt.Errorf("resource stack stream requires view=%s", ViewResourceStack)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	selection, err := resolveTraceIndexSelection(ctx, path)
	if err != nil {
		return Result{}, err
	}
	defer selection.close()
	physicalPath, source, err := resourceStackStreamSource(selection)
	if err != nil {
		return Result{}, err
	}
	var events []Event
	bytes, overflow := 0, false
	observe := func(ev Event) bool {
		if ev.Type == EventResourceStack && !overflow {
			if len(events) >= recordLimit || len(ev.FieldText) > byteLimit-bytes {
				overflow, events = true, nil
			} else {
				events = append(events, ev)
				bytes += len(ev.FieldText)
			}
		}
		// A resource retention cap is not an EOF. Continue the shared parser
		// for complete source quality and generation validation, without
		// retaining a partial event/frame pool or unrelated CPU evidence.
		return true
	}
	idx, err := resourceStackScanSource(ctx, selection, physicalPath, q.TraceFlavorHint, observe)
	if err != nil {
		return Result{}, err
	}
	source.LocalLineCount, source.EventCount = idx.LineCount, idx.ParsedKnown
	source.timestampOrder, source.clockRegressions = idx.TimestampOrder, idx.ClockRegressions
	idx.Path, idx.Size = selection.indexPath, selection.universe.totalBytes
	idx.TraceArtifacts = []TraceArtifactSource{source}
	idx.Events = events
	idx.Caveats = append(idx.Caveats, selection.caveats...)
	if selection.bundleSet {
		idx.Caveats = append(idx.Caveats, traceBundleCaveats(selection.bundle)...)
	}
	q = normalizeQuery(idx, q).WithRunContext(ctx)
	var p *ResourceStackResult
	if overflow {
		p = &ResourceStackResult{SourcePath: idx.Path, Window: ResourceStackWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd, EndInclusive: q.timeEndBackfilled},
			Status: "unavailable", Reason: "resource_stack_complete_scan_retention_limit", TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope,
			Caveats: []string{fmt.Sprintf("The source was scanned to EOF, but complete resource event/frame retention exceeded %d records or %d bytes; no partial resource census is published.", recordLimit, byteLimit)}}
		if p.TargetScope == "" {
			p.TargetScope = TargetScopeThread
		}
	} else {
		p = buildResourceStack(idx, q)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	// Match index publication: validate generations after private decoding
	// and aggregation, not merely after the physical read. A replaced child
	// or manifest during that work must not publish an old-generation fact.
	if err := errors.Join(selection.validateIndex(idx), selection.validate(ctx)); err != nil {
		return Result{}, err
	}
	if err := selection.close(); err != nil {
		return Result{}, err
	}
	return Result{View: ViewResourceStack, SourcePath: idx.Path, TimeUnit: "seconds", TimeStart: q.TimeStart, TimeEnd: q.TimeEnd,
		LineCount: idx.LineCount, ScannedLineCount: idx.ScannedLineCount, UnparsedLineCount: idx.UnparsedLines,
		EventCount: idx.ParsedKnown, ParseLinePanics: idx.ParseLinePanics, ClockRegressions: idx.ClockRegressions,
		TraceArtifacts: idx.TraceArtifacts, ResourceStack: p, Caveats: append(idx.Caveats, p.Caveats...)}, nil
}

func resourceStackStreamSource(selection *traceIndexSelection) (string, TraceArtifactSource, error) {
	physicalPath := selection.indexPath
	var source TraceArtifactSource
	if len(selection.artifactSpecs) != 0 {
		if len(selection.artifactSpecs) != 1 || selection.artifactSpecs[0].source.Kind != "systrace" {
			return "", source, fmt.Errorf("resource_stack requires_complete_identity_mapped_single_source")
		}
		source = selection.artifactSpecs[0].source
		physicalPath = source.SourcePath
	}
	entry, ok := selection.universe.entry(physicalPath)
	if !ok {
		return "", source, fmt.Errorf("resource stack source is absent from the frozen source universe")
	}
	if len(selection.artifactSpecs) == 0 {
		source = singleTraceArtifactSourceWithIdentity(physicalPath, entry.identity, 0, 0)
	} else {
		source.SourceBytes, source.SourceModUnixNano, source.sourceIdentity = entry.identity.Size(), entry.identity.ModUnixNano(), entry.identity
	}
	// Reuse the indexed view's exact source contract. The selection above
	// already attested V2 child bytes and bound all paths to held generations;
	// a single child alone does not establish clock or capture compatibility.
	if !resourceStackSingleSource(&Index{Path: selection.indexPath, TraceArtifacts: []TraceArtifactSource{source}}) {
		return "", source, fmt.Errorf("resource_stack requires_complete_identity_mapped_single_source")
	}
	return physicalPath, source, nil
}

func resourceStackScanSource(ctx context.Context, selection *traceIndexSelection, path string, flavor TraceFlavor, observe func(Event) bool) (*Index, error) {
	entry, ok := selection.universe.entry(path)
	if !ok {
		return nil, fmt.Errorf("resource stack source is absent from the frozen source universe")
	}
	f, identity, err := openTraceSourceRegularContext(ctx, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if !entry.identity.SameVersion(identity) {
		return nil, fmt.Errorf("resource stack source changed before stream scan opened the artifact")
	}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	idx, err := streamScanReader(ctx, path, info, io.NewSectionReader(f, 0, identity.Size()), flavor, observe, nil, false, 0, false)
	if err != nil {
		return nil, traceReadErrorAfterIdentity(f, identity, "resource stack stream physical read", err)
	}
	if err := validateTraceFileIdentityAfterRead(f, identity, "resource stack stream"); err != nil {
		return nil, err
	}
	return idx, nil
}
