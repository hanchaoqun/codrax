package tracediag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type v2DiscoveryOutcome struct {
	spec   *WindowDiscovery
	result *tracequery.WindowDiscoveryResult
	err    error
}

type v2StepInstance struct {
	step            Step
	logicalOrdinal  int
	logicalLabel    string
	instanceOrdinal int
	instanceCount   int
	blockedErr      error
}

type v2StepStatus struct {
	instance  v2StepInstance
	err       error
	bodyTotal int
	bodyShown int
}

// Test-only seam used to prove the run-level source lock. Production leaves
// it nil; the hook cannot affect discovery or execution semantics.
var traceDiagV2AfterDiscoveriesHook func()

func runV2(ctx context.Context, opts Options, w io.Writer, script *Script, tracePath string, info os.FileInfo, flavorHint tracequery.TraceFlavor, at time.Time) (failed int, runErr error) {
	if len(script.Discoveries) > 0 && tracequery.TracePathRequiresCompositeIndex(tracePath) {
		return 0, fmt.Errorf("tracediag v2: window discovery requires one explicit physical trace artifact; composite/bundle sources must use a physical child")
	}
	totalCap := script.v2Limits.MaxReportLines
	if opts.TotalMaxLines > 0 {
		if opts.TotalMaxLines < script.v2WorstReportLines {
			return 0, fmt.Errorf("tracediag v2: requested report cap %d is below the validated worst-case %d", opts.TotalMaxLines, script.v2WorstReportLines)
		}
		if opts.TotalMaxLines < totalCap {
			totalCap = opts.TotalMaxLines
		}
	}

	sourceVersion, err := tracequery.CaptureTraceSourceVersion(tracePath)
	if err != nil {
		return 0, err
	}
	lockedInfo, err := os.Stat(tracePath)
	if err != nil {
		return 0, err
	}
	if err := sourceVersion.Validate(tracePath); err != nil {
		return 0, fmt.Errorf("tracediag v2 initial source lock: %w", err)
	}
	// The source-universe fingerprint below locks path/size/mtime/inode/ctime
	// across every discovery and execution stage, but it is intentionally not a
	// content digest. Carry a byte-exact digest of the primary artifact as a
	// separate reconciliation authority, matching the v1 report contract.
	primarySHA256 := traceFileSHA256(tracePath)
	if err := sourceVersion.Validate(tracePath); err != nil {
		return 0, fmt.Errorf("tracediag v2 source lock after primary digest: %w", err)
	}
	info = lockedInfo
	discoveries, err := runV2Discoveries(ctx, script, tracePath, flavorHint, sourceVersion)
	if err != nil {
		return 0, err
	}
	if traceDiagV2AfterDiscoveriesHook != nil {
		traceDiagV2AfterDiscoveriesHook()
	}
	if err := sourceVersion.Validate(tracePath); err != nil {
		return 0, fmt.Errorf("tracediag v2 source lock after discovery: %w", err)
	}
	instances, err := resolveV2Plan(script, discoveries)
	if err != nil {
		return 0, err
	}

	var report bytes.Buffer
	rw := newReportWriter(&report, totalCap)
	writeV2ProvenanceHeader(rw, opts, script, tracePath, primarySHA256, info, flavorHint, at, sourceVersion, len(instances))
	for i := range discoveries {
		writeV2DiscoverySection(rw, i+1, len(discoveries), discoveries[i])
	}
	writeV2ExecutionPlan(rw, instances)

	statuses := make([]v2StepStatus, 0, len(instances))
	for i := range instances {
		instance := instances[i]
		status := v2StepStatus{instance: instance}
		writeV2InstanceHeader(rw, i+1, len(instances), instance)
		if instance.blockedErr != nil {
			status.err = instance.blockedErr
			rw.line(fmt.Sprintf("[步骤失败] typed dependency error: %v", instance.blockedErr))
			rw.line("[步骤失败] 未回退到父窗口；继续执行其它独立实例。")
			statuses = append(statuses, status)
			continue
		}
		if err := sourceVersion.Validate(tracePath); err != nil {
			return 0, fmt.Errorf("tracediag v2 source lock before %s: %w", instance.logicalLabel, err)
		}
		outcome := runStep(ctx, tracePath, flavorHint, &instance.step)
		if err := sourceVersion.Validate(tracePath); err != nil {
			return 0, fmt.Errorf("tracediag v2 source lock after %s: %w", instance.logicalLabel, err)
		}
		if outcome.err != nil {
			status.err = outcome.err
			rw.line(fmt.Sprintf("[步骤失败] engine error (verbatim): %v", outcome.err))
			rw.line("[步骤失败] 该实例无结果输出;继续执行后续实例。")
			statuses = append(statuses, status)
			continue
		}
		body := renderStepBody(&instance.step, outcome)
		status.bodyTotal = body.total
		status.bodyShown = len(body.lines)
		for _, line := range body.lines {
			rw.line(line)
		}
		if body.total > len(body.lines) {
			rw.line(fmt.Sprintf("…共 %d 行,按帽截断至 %d,余 %d 行未列", body.total, len(body.lines), body.total-len(body.lines)))
		}
		if completenessErr := generatedCollectionCompletenessError(&instance.step, outcome, body); completenessErr != nil {
			status.err = completenessErr
			rw.line(fmt.Sprintf("[完整性失败] %v", completenessErr))
			rw.line("[完整性失败] 已返回的原始行均可见，但该自动窗仍有匹配行未发布；不得把本实例当作 N/N 完整 witness。")
		}
		statuses = append(statuses, status)
	}
	if err := sourceVersion.Validate(tracePath); err != nil {
		return 0, fmt.Errorf("tracediag v2 final source lock: %w", err)
	}
	writeV2StatusSummary(rw, discoveries, statuses)
	if rw.flushErr() != nil {
		return 0, fmt.Errorf("tracediag: write buffered v2 report: %w", rw.flushErr())
	}
	if rw.capHit {
		return 0, fmt.Errorf("tracediag v2 internal budget error: validated worst-case=%d but report hit cap=%d; no partial report published", script.v2WorstReportLines, totalCap)
	}
	if err := sourceVersion.Validate(tracePath); err != nil {
		return 0, fmt.Errorf("tracediag v2 publish source lock: %w", err)
	}
	if _, err := io.Copy(w, bytes.NewReader(report.Bytes())); err != nil {
		return 0, fmt.Errorf("tracediag: write report: %w", err)
	}
	for _, discovery := range discoveries {
		if discovery.err != nil {
			failed++
		}
	}
	for _, status := range statuses {
		if status.err != nil {
			failed++
		}
	}
	return failed, nil
}

func generatedCollectionCompletenessError(step *Step, outcome stepOutcome, body stepBody) error {
	if step == nil || step.windowOrigin == nil || step.View != "event_search" || outcome.result == nil {
		return nil
	}
	if accounting := body.eventSearch; accounting != nil && accounting.compacted {
		return fmt.Errorf("generated_window_compacted discovery=%s candidate_rank=%d matched=%d emitted=%d; reduce event families or add a denser partition strategy",
			step.windowOrigin.DiscoveryLabel, step.windowOrigin.CandidateRank, accounting.matched, accounting.emitted)
	}
	for _, compaction := range outcome.result.Compactions {
		if compaction.Dimension == tracequery.CompactionDimensionEvents && compaction.Total > compaction.Emitted {
			return fmt.Errorf("generated_window_compacted discovery=%s candidate_rank=%d matched=%d emitted=%d; reduce event families or add a denser partition strategy",
				step.windowOrigin.DiscoveryLabel, step.windowOrigin.CandidateRank, compaction.Total, compaction.Emitted)
		}
	}
	return nil
}

func runV2Discoveries(ctx context.Context, script *Script, tracePath string, flavorHint tracequery.TraceFlavor, sourceVersion tracequery.TraceSourceVersion) ([]v2DiscoveryOutcome, error) {
	out := make([]v2DiscoveryOutcome, 0, len(script.Discoveries))
	for i := range script.Discoveries {
		spec := &script.Discoveries[i]
		if err := sourceVersion.Validate(tracePath); err != nil {
			return nil, fmt.Errorf("tracediag v2 source lock before discovery %s: %w", spec.Label, err)
		}
		request := tracequery.WindowDiscoveryRequest{
			Strategy:         tracequery.WindowDiscoveryStrategy(spec.Strategy),
			TimeStart:        spec.windowStart,
			TimeEnd:          spec.windowEnd,
			TimeStartSet:     spec.windowSet,
			TimeEndSet:       spec.windowSet,
			LineStart:        spec.LineStart,
			LineEnd:          spec.LineEnd,
			MaxWindows:       spec.MaxWindows,
			MaxWindowMs:      spec.MaxWindowMS,
			PaddingMs:        spec.PaddingMS,
			EndpointLimit:    spec.EndpointLimit,
			ActiveLaneLimit:  spec.ActiveLaneLimit,
			CohortEventLimit: spec.CohortEventLimit,
		}
		for _, family := range spec.Families {
			request.Families = append(request.Families, tracequery.WindowDiscoveryFamily(family))
		}
		result, err := tracequery.DiscoverWindows(ctx, tracePath, flavorHint, request)
		if lockErr := sourceVersion.Validate(tracePath); lockErr != nil {
			return nil, fmt.Errorf("tracediag v2 source lock after discovery %s: %w", spec.Label, lockErr)
		}
		outcome := v2DiscoveryOutcome{spec: spec, err: err}
		if err == nil {
			outcome.result = &result
		}
		out = append(out, outcome)
	}
	return out, nil
}

func resolveV2Plan(script *Script, discoveries []v2DiscoveryOutcome) ([]v2StepInstance, error) {
	byLabel := map[string]v2DiscoveryOutcome{}
	generatedWindows := 0
	for _, outcome := range discoveries {
		byLabel[outcome.spec.Label] = outcome
		if outcome.result != nil {
			generatedWindows += len(outcome.result.Windows)
		}
	}
	if generatedWindows > script.v2Limits.MaxGeneratedWindows {
		return nil, fmt.Errorf("tracediag v2 internal plan error: generated windows=%d exceeds validated cap=%d", generatedWindows, script.v2Limits.MaxGeneratedWindows)
	}
	var instances []v2StepInstance
	for i := range script.Steps {
		logical := script.Steps[i]
		if logical.WindowsFrom == nil {
			instances = append(instances, v2StepInstance{step: logical, logicalOrdinal: i + 1, logicalLabel: logical.Label, instanceOrdinal: 1, instanceCount: 1})
			continue
		}
		outcome, ok := byLabel[logical.WindowsFrom.Discovery]
		if !ok || outcome.spec == nil {
			return nil, fmt.Errorf("tracediag v2 internal plan error: validated discovery %q is missing", logical.WindowsFrom.Discovery)
		}
		if outcome.err != nil {
			instances = append(instances, v2StepInstance{step: logical, logicalOrdinal: i + 1, logicalLabel: logical.Label, instanceOrdinal: 1, instanceCount: 1, blockedErr: fmt.Errorf("dependency_failed discovery=%s: %w", outcome.spec.Label, outcome.err)})
			continue
		}
		if outcome.result == nil || len(outcome.result.Windows) == 0 {
			instances = append(instances, v2StepInstance{step: logical, logicalOrdinal: i + 1, logicalLabel: logical.Label, instanceOrdinal: 1, instanceCount: 1, blockedErr: fmt.Errorf("dependency_empty discovery=%s generated no collectible window", outcome.spec.Label)})
			continue
		}
		count := len(outcome.result.Windows)
		for j, window := range outcome.result.Windows {
			resolved := logical
			resolved.WindowsFrom = nil
			resolved.Window = formatSecondsToken(window.StartTs) + ".." + formatSecondsToken(window.EndTs)
			resolved.windowStart = window.StartTs
			resolved.windowEnd = window.EndTs
			resolved.windowSet = true
			resolved.windowOrigin = &WindowProvenance{
				DiscoveryLabel:      outcome.spec.Label,
				WindowOrdinal:       window.Ordinal,
				CandidateRank:       window.CandidateRank,
				CandidateWindow:     window.CandidateWindow,
				Family:              string(window.Family),
				Kind:                window.Kind,
				CoreStartTs:         window.CoreStartTs,
				CoreEndTs:           window.CoreEndTs,
				CoreLineStart:       window.CoreLineStart,
				CoreLineEnd:         window.CoreLineEnd,
				RankBasis:           window.RankBasis,
				IdentityFingerprint: window.IdentityFingerprint,
			}
			instances = append(instances, v2StepInstance{step: resolved, logicalOrdinal: i + 1, logicalLabel: logical.Label, instanceOrdinal: j + 1, instanceCount: count})
		}
	}
	if len(instances) > script.v2Limits.MaxExpandedSteps {
		return nil, fmt.Errorf("tracediag v2 internal plan error: expanded instances=%d exceeds validated cap=%d", len(instances), script.v2Limits.MaxExpandedSteps)
	}
	return instances, nil
}
