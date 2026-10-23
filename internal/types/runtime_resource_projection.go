package types

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

// RuntimeResourceDisplayEvent is supplied while the native producer formats
// its fixed tables. Coordinates are never recovered from display strings.
// SummaryRow and frame ranges bind one whole event and its physical frames;
// FrameNote identifies its event-specific quality note in the members table.
type RuntimeResourceDisplayEvent struct {
	TimestampNS          int64
	TID, PID             int
	Thread               string
	SummaryRow           int
	FrameStart, FrameEnd int
	FrameNote            int
}

type runtimeResourceEventDisplayIndex struct {
	matched int
	events  []RuntimeResourceDisplayEvent
}

// WithRuntimeResourceEventDisplayIndex extends an existing native-only seal.
// JSON cannot mint this credential; retaining original population counts does
// not make retained detail rows a complete request or capture population.
func WithRuntimeResourceEventDisplayIndex(r ObservationRecord, matched int, events []RuntimeResourceDisplayEvent) ObservationRecord {
	if !runtimeResourcePublicationValid(r) || matched < len(events) {
		return r
	}
	p, ok := DecodeRuntimeMeasurementPublication(r)
	if !ok || len(p.Tables) < 2 || len(p.Tables[0].Rows) != len(events) {
		return r
	}
	frames, note := 0, -1
	for i, event := range events {
		if event.TimestampNS < 0 || event.TID < 0 || event.PID < 0 || event.SummaryRow != i || event.FrameStart != frames || event.FrameEnd < frames || event.FrameEnd > len(p.Tables[1].Rows) || event.FrameNote <= note || event.FrameNote >= len(p.Tables[1].Notes) {
			return r
		}
		frames, note = event.FrameEnd, event.FrameNote
	}
	if frames != len(p.Tables[1].Rows) {
		return r
	}
	witness := *r.runtimeResourcePublication
	witness.eventDisplay = &runtimeResourceEventDisplayIndex{matched: matched, events: append([]RuntimeResourceDisplayEvent(nil), events...)}
	r.runtimeResourcePublication = &witness
	return r
}

// RuntimeResourceDisplayProjection is a detached display selection. It grants
// no new query, denominator, completion, source-code or causal authority.
type RuntimeResourceDisplayProjection struct {
	SelectedEventIndexes  []int
	OriginalMatchedEvents int
	RetainedEvents        int
	ProjectedEvents       int
	TargetExcludedEvents  int
	WindowExcludedEvents  int
	BoundaryNote          string
}

func runtimeResourceProjectionRequest(r ObservationRecord, rm *RequestModel) bool {
	if !runtimeResourcePublicationValid(r) || r.runtimeResourcePublication.eventDisplay == nil || rm == nil ||
		!rm.RuntimeQuestionProfile.SuppressesRootCauseRankingPrompt() || !rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		switch target.Kind {
		case RuntimeTargetKindThread:
			pid, name, valid := threadidentity.QuerySelector(target.PID, target.Thread)
			if !valid || pid == 0 && name == "" {
				return false
			}
		case RuntimeTargetKindProcess:
			if target.PID <= 0 || target.PID > RuntimeTargetMaxPID {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func runtimeResourceProjectEvents(r ObservationRecord, rm *RequestModel, scope *RuntimeArtifactScopeProfile) (RuntimeResourceDisplayProjection, bool) {
	if !runtimeResourceProjectionRequest(r, rm) {
		return RuntimeResourceDisplayProjection{}, false
	}
	index := r.runtimeResourcePublication.eventDisplay
	out := RuntimeResourceDisplayProjection{OriginalMatchedEvents: index.matched, RetainedEvents: len(index.events)}
	for i, event := range index.events {
		tidKnown, pidKnown := event.TID > 0, event.PID > 0
		row := TraceEventSearchInventoryRow{TraceTimeSeconds: float64(event.TimestampNS) / 1e9,
			EmitterTID: event.TID, EmitterTGID: event.PID, EmitterTIDKnown: &tidKnown, EmitterTGIDKnown: &pidKnown, Comm: event.Thread}
		if !TraceEventRowMatchesEmitterTarget(row, rm) {
			out.TargetExcludedEvents++
			continue
		}
		if !TraceEventRowWithinRequestedWindows(row, scope) {
			out.WindowExcludedEvents++
			continue
		}
		out.SelectedEventIndexes = append(out.SelectedEventIndexes, i)
	}
	out.ProjectedEvents = len(out.SelectedEventIndexes)
	// No retained member is not a native zero-result query. Keep exact empty
	// queries and independently queried supplementary targets on their old
	// paths instead of replacing either with an empty projected population.
	if out.ProjectedEvents == 0 {
		return RuntimeResourceDisplayProjection{}, false
	}
	out.BoundaryNote = fmt.Sprintf("按请求主体及时间点展示资源事件=%d；原查询匹配=%d，原交接保留=%d、省略=%d；主体不匹配排除=%d，匹配主体但在请求窗口外排除=%d。多主体取并集、多窗口取半开区间并集；原查询总体和统计分母未改变。保留预览中无匹配不等于完整查询零结果；事件与栈帧不证明执行时长、等待、完整展开或根因。", out.ProjectedEvents, out.OriginalMatchedEvents, out.RetainedEvents, out.OriginalMatchedEvents-out.RetainedEvents, out.TargetExcludedEvents, out.WindowExcludedEvents)
	return out, true
}

// RuntimeResourceCurrentDisplayProjection validates the exact same-call
// original and current physical input before sharing its event indexes with
// prose, table and fact-diagram consumers. Ledger normalization may change
// ToolCallID navigation and the ledger's exact inferred ClaimAuthority;
// same-ID replacement payloads cannot borrow it.
func RuntimeResourceCurrentDisplayProjection(input ObservationLedgerInput, result ToolResult, record ObservationRecord) (RuntimeResourceDisplayProjection, bool) {
	original, found := runtimeResourceProjectionOriginal(result, record, input.RequestModel)
	if !found {
		return RuntimeResourceDisplayProjection{}, false
	}
	scope, _, current := runtimeMeasurementQueryDisplayScope(input, result, original)
	if !current {
		return RuntimeResourceDisplayProjection{}, false
	}
	start, end, known := TraceObservationContinuousQueryWindow(original.SourceRef)
	if runtimeResourceMeasurementMatchesRequest(original, input.RequestModel) && runtimeResourceMeasurementWithinWindows(original, scope) &&
		(!scope.HasExplicitTimeWindows() || !known || scope.ContainsExplicitTimeWindow(start, end)) {
		return RuntimeResourceDisplayProjection{}, false // Existing exact/native path needs no projection.
	}
	return runtimeResourceProjectEvents(original, input.RequestModel, scope)
}

func runtimeResourceProjectionOriginal(result ToolResult, record ObservationRecord, rm *RequestModel) (ObservationRecord, bool) {
	if !result.Success || result.ToolName != "trace_query" || result.ReusedFromRunMemo || result.TraceViewCancellation != nil {
		return ObservationRecord{}, false
	}
	var original ObservationRecord
	found := false
	for _, candidate := range result.Observations {
		if candidate.ID != record.ID {
			continue
		}
		a, b := candidate, record
		a.SourceRef.ToolCallID, b.SourceRef.ToolCallID = "", ""
		// CompileObservationLedger fills an absent authority deterministically.
		// Accept only that exact derived value, never an arbitrary replacement.
		if !a.ClaimAuthority.IsValid() && b.ClaimAuthority == inferObservationClaimAuthority(a) {
			a.ClaimAuthority = b.ClaimAuthority
		}
		if found || !runtimeResourceProjectionRequest(candidate, rm) || runtimeSamplingDigest(a) != runtimeSamplingDigest(b) {
			return ObservationRecord{}, false
		}
		original, found = candidate, true
	}
	return original, found
}

// Applies the exact private event indexes to fixed native rows. Global notes
// retain the original census; event-specific notes follow their own event.
func projectRuntimeResourceTable(table RuntimeMeasurementTable, r ObservationRecord, projection RuntimeResourceDisplayProjection) RuntimeMeasurementTable {
	out := table.Clone()
	if table.View == RuntimeMeasurementDistribution {
		out.Label = "原查询总体统计（不替代请求成员统计）：" + out.Label
		out.DefaultPresentation = false
		out.Notes = append(out.Notes, "以下统计保持原查询完整扫描总体及分母，未按当前展示成员重算；不能用作请求主体或窗口的合计。")
	} else {
		index := r.runtimeResourcePublication.eventDisplay
		out.Notes = append(out.Notes, "原查询表说明："+table.Label)
		if table.View == RuntimeMeasurementSummary {
			out.Label = "请求范围内已保留的资源事件"
		} else if table.View == RuntimeMeasurementMembers {
			out.Label = "请求范围内已保留的资源事件栈帧"
		}
		out.Rows = nil
		if table.View == RuntimeMeasurementMembers {
			eventNotes := map[int]bool{}
			for _, event := range index.events {
				eventNotes[event.FrameNote] = true
			}
			out.Notes = nil
			for i, note := range table.Notes {
				if !eventNotes[i] {
					out.Notes = append(out.Notes, note)
				}
			}
			out.Notes = append(out.Notes, "原查询表说明："+table.Label)
		}
		for _, selected := range projection.SelectedEventIndexes {
			event := index.events[selected]
			if table.View == RuntimeMeasurementSummary {
				out.Rows = append(out.Rows, append([]string(nil), table.Rows[event.SummaryRow]...))
			} else if table.View == RuntimeMeasurementMembers {
				for _, row := range table.Rows[event.FrameStart:event.FrameEnd] {
					out.Rows = append(out.Rows, append([]string(nil), row...))
				}
				out.Notes = append(out.Notes, table.Notes[event.FrameNote])
			}
		}
	}
	out.Notes = append(out.Notes, projection.BoundaryNote)
	out.coverageScope, out.MemberSet, out.DisplayPopulation = nil, nil, nil
	out.nativeTraceDisplayPopulation = false
	out.nativeResourceFactsDigest = runtimeSamplingDigest(out)
	return out
}
