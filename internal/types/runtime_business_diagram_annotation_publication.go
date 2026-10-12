package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
)

// RuntimeBusinessDiagramAnnotationPublication is public transport only. A
// decoded value has no diagram authority without the contract's independently
// current, source-qualified native business summary.
type RuntimeBusinessDiagramAnnotationPublication struct {
	Version       int                  `json:"version"`
	ObservationID string               `json:"observation_id"`
	Source        ObservationSourceRef `json:"source"`
	Subject       string               `json:"subject"`
	Name          string               `json:"name"`
	Span          ObservationSpan      `json:"span"`
	Owner         RuntimeWakeupThread  `json:"owner"`
	StartRef      string               `json:"start_ref"`
	EndRef        string               `json:"end_ref"`
}

func decodeRuntimeBusinessDiagramAnnotation(r ObservationRecord) (RuntimeBusinessDiagramAnnotationPublication, bool) {
	var p RuntimeBusinessDiagramAnnotationPublication
	if r.Negative || r.Predicate != TraceBusinessSpanPredicate || r.ClaimKey != TraceBusinessSpanPredicate+":"+r.Object ||
		r.Origin != AnswerEvidenceOriginRuntimeArtifact || r.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Role != AnswerAggregateRoleSupportingCoverage ||
		r.GroundingPolicy != ClaimGroundingHard || r.ProvenanceLane != ObservationProvenanceArtifactSpan ||
		r.ID == "" || r.Subject == "" || r.Object == "" || r.SourceRef.Path == "" || r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" {
		return p, false
	}
	var raw string
	count := 0
	for _, note := range r.RichNotes {
		if value, ok := strings.CutPrefix(note, TraceNoteKeyBusinessDiagramAnnotation+"="); ok {
			raw = value
			count++
		}
	}
	if count != 1 || len(raw) > 32<<10 || !runtimeMeasurementUniqueKeys(raw) {
		return p, false
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || p.Version != 1 || p.ObservationID != r.ID ||
		!reflect.DeepEqual(p.Source, r.SourceRef) || p.Subject != r.Subject || p.Name != r.Object || p.Span != r.Span ||
		p.Owner.PID <= 0 || p.Owner.TGID < 0 {
		return p, false
	}
	start, end, known := TraceObservationContinuousQueryWindow(r.SourceRef)
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if !known || !finite(r.Span.StartTs) || !finite(r.Span.EndTs) || r.Span.EndTs <= r.Span.StartTs ||
		r.Span.StartTs < start || r.Span.EndTs >= end || r.Span.LineStart <= 0 || r.Span.LineEnd <= r.Span.LineStart ||
		p.StartRef != fmt.Sprintf("%s:%d", r.SourceRef.Path, r.Span.LineStart) ||
		p.EndRef != fmt.Sprintf("%s:%d", r.SourceRef.Path, r.Span.LineEnd) ||
		len(r.SupportRefs) != 1 || r.SupportRefs[0] != fmt.Sprintf("%s:%d-%d", r.SourceRef.Path, r.Span.LineStart, r.Span.LineEnd) ||
		(r.SourceRef.QueryLineRangeKnown && ((r.SourceRef.QueryLineStart > 0 && r.Span.LineStart < r.SourceRef.QueryLineStart) ||
			(r.SourceRef.QueryLineEnd > 0 && r.Span.LineEnd > r.SourceRef.QueryLineEnd))) {
		return p, false
	}
	if !runtimeBusinessAnnotationMatchesNativeOwner(r, p.Owner) {
		return p, false
	}
	return p, true
}

// The existing scheduler account preserves the entire native ThreadRef even
// when its reader label omits TGID or truncates Comm. Compare that typed tuple,
// not another copy of the display label. The remaining scheduler quantities
// stay with their existing decoder and are not converted into note authority.
func runtimeBusinessAnnotationMatchesNativeOwner(r ObservationRecord, owner RuntimeWakeupThread) bool {
	var raw string
	count := 0
	for _, note := range r.RichNotes {
		if value, ok := strings.CutPrefix(note, TraceNoteKeyBusinessSpanSchedulerStates+"="); ok {
			raw = value
			count++
		}
	}
	if count != 1 || len(raw) > 64<<10 || !runtimeMeasurementUniqueKeys(raw) {
		return false
	}
	var account struct {
		SourcePath string              `json:"source_path"`
		Thread     RuntimeWakeupThread `json:"thread"`
		Window     struct {
			StartTs float64 `json:"start_ts"`
			EndTs   float64 `json:"end_ts"`
		} `json:"window"`
	}
	return json.Unmarshal([]byte(raw), &account) == nil && account.SourcePath == r.SourceRef.Path &&
		account.Thread == owner && account.Window.StartTs == r.Span.StartTs && account.Window.EndTs == r.Span.EndTs
}

func buildNativeBusinessDiagramAnnotations(input ObservationLedgerInput, contract *RuntimeMeasurementContract) []NativeBusinessDiagramAnnotation {
	qualified := map[string]string{}
	for _, table := range contract.Choices() {
		if table.View == RuntimeMeasurementSummary && strings.HasPrefix(table.nativeFactIdentity, "business:") {
			qualified[table.ObservationID] = table.nativeFactIdentity
		}
	}
	seen := map[string]string{}
	conflict := map[string]bool{}
	var candidates []NativeBusinessDiagramAnnotation
	for _, results := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo {
				continue
			}
			for _, r := range result.Observations {
				if qualified[r.ID] == "" {
					continue
				}
				present := false
				for _, note := range r.RichNotes {
					present = present || strings.HasPrefix(note, TraceNoteKeyBusinessDiagramAnnotation+"=")
				}
				if !present {
					continue
				}
				p, ok := decodeRuntimeBusinessDiagramAnnotation(r)
				measurement, measured := DecodeRuntimeMeasurementPublication(r)
				if !ok || !measured || len(measurement.Tables) != 1 {
					conflict[r.ID] = true
					continue
				}
				// Revalidate this result itself. A table from another view/target
				// or a current sibling cannot lend this carrier its private source.
				original := runtimeMeasurementOriginalSource(input, result, r.SourceRef)
				qualifyRuntimeBusinessMeasurementIdentity(&measurement, r, original, result.TraceQueryWindowReplay)
				if measurement.Tables[0].nativeFactIdentity != qualified[r.ID] {
					conflict[r.ID] = true
					continue
				}
				raw, err := json.Marshal(p)
				if err != nil {
					conflict[r.ID] = true
					continue
				}
				if previous, exists := seen[r.ID]; exists {
					conflict[r.ID] = conflict[r.ID] || previous != string(raw)
					continue
				}
				seen[r.ID] = string(raw)
				fingerprint := sha256.Sum256(append(append([]byte(qualified[r.ID]), 0), raw...))
				candidates = append(candidates, NativeBusinessDiagramAnnotation{
					Selector: "native-business:" + hex.EncodeToString(fingerprint[:]), Name: p.Name, Record: r, Owner: p.Owner,
					StartTs: p.Span.StartTs, EndTs: p.Span.EndTs, StartLine: p.Span.LineStart, EndLine: p.Span.LineEnd,
					StartRef: p.StartRef, EndRef: p.EndRef,
				})
			}
		}
	}
	var out []NativeBusinessDiagramAnnotation
	for _, candidate := range candidates {
		if !conflict[candidate.Record.ID] {
			out = append(out, candidate)
		}
	}
	return cloneNativeBusinessDiagramAnnotations(out)
}
