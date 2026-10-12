package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only an observed complete B/E pair can annotate its own executing thread.
// This public transport never grants authority without the current-source
// private business summary qualifier in RuntimeMeasurementContract.
func traceQueryBusinessDiagramAnnotationNote(r types.ObservationRecord, span tracequery.TraceSpanSummary) string {
	start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
	if !known || span.Kind != "sync" || span.Thread.PID <= 0 ||
		span.ActualStartTs != 0 || span.ActualEndTs != 0 || span.ActualDurationMs != 0 ||
		span.StartTs < start || span.EndTs >= end || span.EndTs <= span.StartTs ||
		span.StartLine <= 0 || span.EndLine <= span.StartLine || span.SourcePath != r.SourceRef.Path ||
		span.Name != r.Object || traceThreadLabel(span.Thread) != r.Subject ||
		span.StartTs != r.Span.StartTs || span.EndTs != r.Span.EndTs || span.StartLine != r.Span.LineStart || span.EndLine != r.Span.LineEnd ||
		len(r.SupportRefs) != 1 || r.SupportRefs[0] != fmt.Sprintf("%s:%d-%d", span.SourcePath, span.StartLine, span.EndLine) {
		return ""
	}
	p := types.RuntimeBusinessDiagramAnnotationPublication{Version: 1, ObservationID: r.ID, Source: r.SourceRef,
		Subject: r.Subject, Name: r.Object, Span: r.Span, Owner: types.RuntimeWakeupThread(span.Thread),
		StartRef: fmt.Sprintf("%s:%d", span.SourcePath, span.StartLine), EndRef: fmt.Sprintf("%s:%d", span.SourcePath, span.EndLine)}
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > 32<<10 {
		return ""
	}
	return types.TraceNoteKeyBusinessDiagramAnnotation + "=" + string(raw)
}
