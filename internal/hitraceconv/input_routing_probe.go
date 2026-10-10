package hitraceconv

import (
	"context"
	"fmt"
	"io"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// InputRoutingCapability is navigation-only content evidence. Schema candidates
// do not establish readable rows, timestamps, ownership, or trace admission.
// No material receipt or query authority is published by this probe.
type InputRoutingCapability struct {
	Path           string   `json:"path"`
	Container      string   `json:"container"`
	Status         string   `json:"status"`
	NativeReader   string   `json:"native_reader,omitempty"`
	CandidateViews []string `json:"candidate_views,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

const InputRoutingSQLiteMaxBytes int64 = 32 << 20

type routingSchemaFamily struct {
	view   string
	tables map[string][]string
}

func nativeRoutingSchemaFamilies() []routingSchemaFamily {
	// Kept in agreement with the corresponding exporters by a syntax-aware
	// contract test; adding a family never grants semantic row authority.
	return []routingSchemaFamily{
		{"measurements", map[string][]string{"measure": {"ts", "value", "filter_id"}, "measure_filter": {"id"}}},
		{"process_measurements", map[string][]string{"process_measure": {"ts", "value", "filter_id"}, "process_measure_filter": {"id", "name", "ipid"}}},
		{"cpu_state_frequency", map[string][]string{"measure": {"ts", "value", "filter_id"}, "cpu_measure_filter": {"id", "name", "cpu"}}},
	}
}

// ProbeInputRoutingCapability performs a bounded read of one held generation.
// SQLite is inspected only through the existing sealed, read-only VFS. A live
// WAL, excess size, cancellation or source change remains unknown, not denied.
func ProbeInputRoutingCapability(ctx context.Context, path, runtimeAnchor string) (out InputRoutingCapability) {
	out = InputRoutingCapability{Path: path, Container: "unknown", Status: "unknown"}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		out.Reason = "probe_budget_or_cancelled"
		return
	}
	source, err := openConversionInputAuthority(path)
	if err != nil {
		out.Reason = "input_unavailable"
		return
	}
	defer func() {
		if source.Validate(conversionInputStageProbe) != nil || ctx.Err() != nil {
			out.NativeReader, out.CandidateViews, out.Status, out.Reason = "", nil, "unknown", "input_changed_or_probe_cancelled"
		}
		if source.Close() != nil {
			out.NativeReader, out.CandidateViews, out.Status, out.Reason = "", nil, "unknown", "input_close_failed"
		}
	}()
	n := source.Size()
	if n > 64 {
		n = 64
	}
	prefix := make([]byte, n)
	if _, err := io.ReadFull(io.NewSectionReader(source, 0, n), prefix); err != nil {
		out.Reason = "input_read_failed"
		return
	}
	kind := attachment.KnownBinaryTraceFormat(prefix)
	switch kind {
	case attachment.BinaryTraceFormatHarmonyRMQ, attachment.BinaryTraceFormatOHOSProfile, attachment.BinaryTraceFormatLinuxPerf:
		out.Container, out.Status, out.NativeReader = string(kind), "format_candidate", "trace_query"
		out.Reason = "native_trace_preparation_required"
	case attachment.BinaryTraceFormatSQLite:
		out.Container = "sqlite"
		if source.Size() > InputRoutingSQLiteMaxBytes {
			out.Reason = "schema_probe_size_budget"
			return
		}
		views, err := probeSQLiteNativeViews(ctx, source, runtimeAnchor)
		if err != nil {
			out.Reason = "schema_probe_unavailable"
			return
		}
		out.Status, out.CandidateViews = "schema_inspected", views
		if len(views) > 0 {
			out.NativeReader = "trace_query"
		}
	case attachment.BinaryTraceFormatGZIP, attachment.BinaryTraceFormatZIP:
		out.Container, out.Reason = string(kind), "container_only_domain_unknown"
	default:
		out.Reason = "no_supported_content_signature"
	}
	return
}

func probeSQLiteNativeViews(ctx context.Context, source *conversionInputAuthority, anchor string) (views []string, resultErr error) {
	if anchor == "" {
		return nil, fmt.Errorf("routing probe runtime anchor missing")
	}
	if err := validateExistingTraceDBBoundary(ctx, source); err != nil {
		return nil, err
	}
	staging, err := newRuntimePrivateConversionDir(anchor, "routing-input-*")
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, staging.FinalizeCleanup()) }()
	lease, err := newExternalToolInputLeaseWithProgress(ctx, source, staging, sealedTraceDBVirtualName, externalToolInputSnapshotOnly, nil)
	if err != nil {
		return nil, err
	}
	sealed, err := sealExternalToolInputSnapshot(ctx, lease, staging)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, sealed.Close()) }()
	tdb, err := openTraceDBFromSealed(ctx, sealed, source.DisplayPath())
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = traceDBJoinPreservingSingle(resultErr, tdb.close()) }()
	// Candidate families mirror native exporters' minimum schemas. Matching is
	// deliberately weaker than authoritative preparation: no rows are queried.
	for _, family := range nativeRoutingSchemaFamilies() {
		matched := true
		for table, required := range family.tables {
			exists, err := tdb.tableExists(ctx, table)
			if err != nil {
				return nil, err
			}
			if !exists {
				matched = false
				break
			}
			columns, err := tdb.columnNames(ctx, table)
			if err != nil {
				return nil, err
			}
			present := map[string]bool{}
			for _, col := range columns {
				present[sqliteASCIIIdentifierFold(col)] = true
			}
			for _, col := range required {
				if !present[col] {
					matched = false
				}
			}
		}
		if matched {
			views = append(views, family.view)
		}
	}
	if err := sealed.Validate(); err != nil {
		return nil, err
	}
	if err := validateExistingTraceDBBoundary(ctx, source); err != nil {
		return nil, err
	}
	return views, nil
}
