package types

// These are display-only role/coordinate explanations, not read permissions or
// artifact classification. Callers still use the existing producer context,
// published-ref registry or current navigation ticket before showing them.
const TraceQueryInputLineScopeGuidance = "trace_query line_start/line_end select original trace/index lines, not result pagination. Composite traces use index-global virtual lines; trace_artifacts/source_spans identify the physical artifact and local line."

const TraceQueryResultReadRoleGuidance = "Inspect published query results with grep or read_file (line_offset/limit); they are not raw captures or current repository source. For new analysis, use trace_query on the original capture. " + TraceQueryInputLineScopeGuidance
