package tracediag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func scriptUsesPacketDomain(s *Script) bool {
	for _, step := range s.Steps {
		if tracequery.ViewCapacityFor(strings.TrimSpace(step.View)).Execution == tracequery.ExecutionStreamingPacket {
			return true
		}
	}
	return false
}

// One script has one physical source. A packet execution domain has no Trace
// thread, line or relative-clock selectors, including inherited defaults.
// V2 static steps remain valid; Trace discovery/fan-out needs a separate run.
func validatePacketScriptDomain(s *Script) error {
	if !scriptUsesPacketDomain(s) {
		return nil
	}
	bad := s.Defaults.PID != 0 || strings.TrimSpace(s.Defaults.Thread) != "" || strings.TrimSpace(s.Defaults.Window) != "" || s.Inputs != nil || len(s.Discoveries) != 0 || s.tidOverrideSet
	for _, step := range s.Steps {
		if tracequery.ViewCapacityFor(strings.TrimSpace(step.View)).Execution != tracequery.ExecutionStreamingPacket {
			return fmt.Errorf("tracediag: packet execution cannot mix Trace or unknown views in one source-domain script")
		}
		bad = bad || step.PID != 0 || strings.TrimSpace(step.PIDFrom) != "" || strings.TrimSpace(step.Thread) != "" || strings.TrimSpace(step.Window) != "" || step.LineStart != 0 || step.LineEnd != 0 || strings.TrimSpace(step.Pattern) != "" || len(step.Patterns) != 0 || len(step.EventTypes) != 0 || len(step.EventNames) != 0 || len(step.TraceMarkActions) != 0 || len(step.EventFieldFilters) != 0 || step.WindowsFrom != nil
	}
	if bad {
		return fmt.Errorf("tracediag: packet execution does not accept Trace coordinates, filters, inputs or discoveries; only label/view/max_lines and report budgets apply")
	}
	return nil
}

func runPacketScript(ctx context.Context, opts Options, w io.Writer, script *Script, source string, flavor tracequery.TraceFlavor, at time.Time) (int, error) {
	if flavor != tracequery.TraceFlavorAuto {
		return 0, fmt.Errorf("tracediag: packet execution cannot use a Trace flavor override")
	}
	if opts.TraceInputPreparer == nil {
		return 0, fmt.Errorf("tracediag: packet execution requires the host readonly input preparer")
	}
	// Reject unsupported formats before calling a preparer that can also
	// convert Trace inputs. This path never creates converted material.
	physical, err := filepath.EvalSymlinks(source)
	if err != nil {
		return 0, err
	}
	version, err := tracequery.CapturePacketSourceVersion(ctx, physical)
	if err != nil {
		return 0, fmt.Errorf("tracediag: packet input: %w", err)
	}
	material, err := opts.TraceInputPreparer.Prepare(ctx, physical)
	if err != nil {
		return 0, fmt.Errorf("tracediag: packet preparation: %w", err)
	}
	if material == nil || !material.SinglePhysicalSource() || material.SourcePath() != physical || material.QueryPath() != physical {
		return 0, fmt.Errorf("tracediag: packet preparation must retain the original physical capture")
	}
	validate := func() error {
		if err := version.Validate(ctx); err != nil {
			return err
		}
		return material.Validate(ctx, material.Preview())
	}
	if err := validate(); err != nil {
		return 0, err
	}
	digest, err := version.ContentDigest(ctx)
	if err != nil {
		return 0, fmt.Errorf("tracediag: packet source digest: %w", err)
	}
	if err := validate(); err != nil {
		return 0, err
	}
	totalCap := opts.TotalMaxLines
	if totalCap <= 0 {
		totalCap = DefaultTotalMaxLines
	}
	if script.Version == ScriptVersionV2 {
		totalCap = script.v2Limits.MaxReportLines
		if opts.TotalMaxLines > 0 {
			if opts.TotalMaxLines < script.v2WorstReportLines {
				return 0, fmt.Errorf("tracediag v2: requested report cap %d is below the validated worst-case %d", opts.TotalMaxLines, script.v2WorstReportLines)
			}
			if opts.TotalMaxLines < totalCap {
				totalCap = opts.TotalMaxLines
			}
		}
	}
	var report bytes.Buffer
	rw := newReportWriter(&report, totalCap)
	rw.line("# codrax tracediag 采集报告")
	rw.line(fmt.Sprintf("codrax_version=%s build_time=%s generated_at=%s", opts.Version, opts.BuildTime, at.Format(time.RFC3339)))
	rw.line(fmt.Sprintf("capture=%s size_bytes=%d sha256=%s source_version=%s", filepath.Base(physical), version.SourceBytes(), digest, version.Fingerprint()))
	rw.line(fmt.Sprintf("script=%s version=%d steps=%d execution=streaming_packet", filepath.Base(opts.ScriptPath), script.Version, len(script.Steps)))
	rw.line("说明: 原件只读;时间为 PCAP 原生 epoch 秒,不对齐 Trace 时钟;帧长不是应用载荷。全部记录聚合与最多 64 个可见记录/流分开;max_lines 仅限制展示。")
	statuses := make([]StepStatus, 0, len(script.Steps))
	for i := range script.Steps {
		if err := validate(); err != nil {
			return 0, err
		}
		step := &script.Steps[i]
		status := StepStatus{Label: step.Label, View: step.View}
		writeStepHeader(rw, i+1, len(script.Steps), step)
		query, queryErr := tracequery.StreamPacketQuery(ctx, material.QueryPath(), step.View, 64)
		if err := validate(); err != nil {
			return 0, err
		}
		if queryErr != nil {
			status.Err = queryErr
			rw.line(fmt.Sprintf("[步骤失败] engine error (verbatim): %v", queryErr))
			rw.line("[步骤失败] 该步骤无结果输出;继续执行后续步骤。")
		} else {
			if err := query.ValidateCurrentSource(); err != nil {
				return 0, err
			}
			body := renderStepBody(step, stepOutcome{result: &query.Payload})
			status.BodyTotal, status.BodyShown = body.total, len(body.lines)
			for _, line := range body.lines {
				rw.line(line)
			}
			if body.total > len(body.lines) {
				rw.line(fmt.Sprintf("…共 %d 行,按帽截断至 %d,余 %d 行未列", body.total, len(body.lines), body.total-len(body.lines)))
			}
			complete := query.Payload.PacketInventory != nil && query.Payload.PacketInventory.Status == "complete" || query.Payload.PacketFlows != nil && query.Payload.PacketFlows.Status == "complete"
			if !complete {
				status.Err = fmt.Errorf("packet query incomplete; retained partial observations do not establish a complete capture")
				rw.line("[步骤失败] " + status.Err.Error())
			}
			if err := query.ValidateCurrentSource(); err != nil {
				return 0, err
			}
		}
		statuses = append(statuses, status)
	}
	writeStatusSummary(rw, statuses)
	if err := rw.flushErr(); err != nil {
		return 0, err
	}
	if script.Version == ScriptVersionV2 && rw.capHit {
		return 0, fmt.Errorf("tracediag v2 packet report exceeded validated line budget; no partial report published")
	}
	if err := validate(); err != nil {
		return 0, err
	}
	failed := 0
	for _, status := range statuses {
		if status.Err != nil {
			failed++
		}
	}
	if _, err := io.Copy(w, &report); err != nil {
		return 0, fmt.Errorf("tracediag: write packet report: %w", err)
	}
	return failed, nil
}
