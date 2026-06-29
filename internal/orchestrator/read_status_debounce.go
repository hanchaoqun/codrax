package orchestrator

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

type orchestratorStatusDebouncer struct {
	mu             sync.Mutex
	seen           map[string]string
	lifecycleEnd   map[string]string
	lifecycleError map[string]string
}

// SetEmitter attaches an event emitter for real-time CLI rendering.
// Must be called before Run(). Passing nil restores the no-op default.
func (o *Orchestrator) SetEmitter(emit render.EventEmitter) {
	if emit == nil {
		emit = render.NopEmitter
	}
	o.readStatusDebouncer.Reset()
	o.emit = func(ev render.Event) {
		if o.suppressReadStatusEvent(ev) {
			return
		}
		emit(ev)
	}
}

func (d *orchestratorStatusDebouncer) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = nil
	d.lifecycleEnd = nil
	d.lifecycleError = nil
}

func (d *orchestratorStatusDebouncer) Suppress(key, cursor string) bool {
	if d == nil || key == "" || cursor == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]string)
	}
	if d.seen[key] == cursor {
		return true
	}
	d.seen[key] = cursor
	return false
}

func (o *Orchestrator) suppressReadStatusEvent(ev render.Event) bool {
	if o == nil || o.busCtx == nil {
		return false
	}
	if o.busCtx.Mode.Normalize() != types.ModeRead {
		return false
	}
	key, ok := readStatusDebounceKey(ev)
	if !ok {
		return false
	}
	if readStatusShouldSuppressSkippedLifecycle(ev) {
		return true
	}
	cursor := o.readStatusDebounceCursor(ev)
	switch o.readStatusDebouncer.lifecycleDecision(ev, o.readStatusLifecycleCursor(ev)) {
	case readStatusLifecycleSuppress:
		return true
	case readStatusLifecycleBypassGeneric:
		return false
	}
	return o.readStatusDebouncer.Suppress(key, cursor)
}

func readStatusShouldSuppressSkippedLifecycle(ev render.Event) bool {
	switch ev.Kind {
	case render.EventTaskNodeStart, render.EventTaskNodeEnd:
	default:
		return false
	}
	if !ev.NodeSkipped {
		return false
	}
	switch readStatusEventNodeKind(ev) {
	case types.NodeEvidence, types.NodeProbe:
		return true
	default:
		return false
	}
}

type readStatusLifecycleDecision int

const (
	readStatusLifecycleNone readStatusLifecycleDecision = iota
	readStatusLifecycleSuppress
	readStatusLifecycleBypassGeneric
)

func (d *orchestratorStatusDebouncer) lifecycleDecision(ev render.Event, cursor string) readStatusLifecycleDecision {
	if d == nil || cursor == "" {
		return readStatusLifecycleNone
	}
	family, ok := readStatusLifecycleFamily(ev)
	if !ok {
		return readStatusLifecycleNone
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lifecycleEnd == nil {
		d.lifecycleEnd = make(map[string]string)
	}
	if d.lifecycleError == nil {
		d.lifecycleError = make(map[string]string)
	}
	switch ev.Kind {
	case render.EventTaskNodeEnd:
		if strings.TrimSpace(ev.Error) != "" || ev.NodeSkipped {
			delete(d.lifecycleEnd, family)
			d.lifecycleError[family] = cursor
			return readStatusLifecycleBypassGeneric
		}
		d.lifecycleEnd[family] = cursor
		delete(d.lifecycleError, family)
	case render.EventTaskNodeStart:
		if d.lifecycleError[family] == cursor {
			delete(d.lifecycleError, family)
			return readStatusLifecycleBypassGeneric
		}
		if d.lifecycleEnd[family] == cursor {
			return readStatusLifecycleSuppress
		}
	}
	return readStatusLifecycleNone
}

func readStatusLifecycleFamily(ev render.Event) (string, bool) {
	switch ev.Kind {
	case render.EventTaskNodeStart, render.EventTaskNodeEnd:
	default:
		return "", false
	}
	kind := readStatusEventNodeKind(ev)
	switch kind {
	case types.NodeEvidence, types.NodeProbe, types.NodeReconcile:
		return fmt.Sprintf("%s:%s", kind, strings.TrimSpace(ev.DispatchKind)), true
	default:
		return "", false
	}
}

func readStatusDebounceKey(ev render.Event) (string, bool) {
	switch ev.Kind {
	case render.EventOrchestratorNotice:
		switch ev.NoticeKind {
		case render.NoticeRetry,
			render.NoticeInvestigationReady,
			render.NoticeInvestigationSupplement,
			render.NoticeConvergenceStall:
			return fmt.Sprintf("notice:%d:%s:%s", ev.NoticeKind, ev.Stage, ev.Agent), true
		default:
			return "", false
		}
	case render.EventTaskNodeStart, render.EventTaskNodeEnd:
		kind := readStatusEventNodeKind(ev)
		switch kind {
		case types.NodeEvidence, types.NodeProbe, types.NodeReconcile:
			return fmt.Sprintf("node:%d:%s:%s", ev.Kind, kind, ev.DispatchKind), true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func readStatusEventNodeKind(ev render.Event) types.TaskNodeType {
	kind := types.TaskNodeType(strings.TrimSpace(ev.NodeKind))
	if kind == "" {
		kind = types.TaskNodeType(strings.TrimSpace(ev.DispatchKind))
	}
	return kind
}

func (o *Orchestrator) readStatusDebounceCursor(ev render.Event) string {
	return o.readStatusDebounceCursorWithEvent(ev, true)
}

func (o *Orchestrator) readStatusLifecycleCursor(ev render.Event) string {
	return o.readStatusDebounceCursorWithEvent(ev, false)
}

func (o *Orchestrator) readStatusDebounceCursorWithEvent(ev render.Event, includeEvent bool) string {
	if o == nil || o.busCtx == nil {
		return ""
	}
	ctx := o.busCtx
	mut := ctx.Mutable
	evidenceCount := len(ctx.EvidenceItems)
	emittedEvidenceCount := 0
	aggregateFactCount := 0
	dispatchToolCount := 0
	pendingReadCount := 0
	investigationComplete := false
	stableCompleteReason := false
	progressReason := ""
	sourceInventoryShape := "inactive"
	if mut != nil {
		emittedEvidenceCount = len(mut.EmittedEvidence())
		aggregateFactCount = len(mut.StableInvestigationAggregateFacts())
		dispatchToolCount = len(mut.DispatchToolResults())
		investigationComplete = mut.IsInvestigationComplete()
		stableCompleteReason = strings.TrimSpace(mut.StableInvestigationCompleteReason()) != ""
		if closure := mut.EvidenceClosure(); closure != nil {
			pendingReadCount = len(closure.PendingReads())
			progress := closure.LatestProgressDecision()
			progressReason = strings.TrimSpace(progress.ReasonCode)
		}
		sourceInventoryShape = readStatusSourceInventoryShapeForContext(ctx, mut)
	}
	eventPart := ""
	if includeEvent {
		eventPart = fmt.Sprintf("event=%d|", ev.Kind)
	}
	return fmt.Sprintf(
		"stage=%s|%sev=%d|emit_ev=%d|flow=%d|chain=%d|sym=%d|fact=%d|tool=%d|dispatch_tool=%d|mcp=%d|pending=%d|complete=%t|stable_complete=%t|progress=%s|source_inventory=%s",
		ctx.PipelineStage,
		eventPart,
		evidenceCount,
		emittedEvidenceCount,
		len(ctx.FlowFindings),
		len(ctx.AnswerChains),
		len(ctx.AnswerSymbols),
		aggregateFactCount,
		len(ctx.ToolResults),
		dispatchToolCount,
		len(ctx.MCPResponses),
		pendingReadCount,
		investigationComplete,
		stableCompleteReason,
		progressReason,
		sourceInventoryShape,
	)
}

func readStatusSourceInventoryShapeForContext(ctx *types.BusContext, mut *types.MutableState) string {
	obs := types.SourceInventoryObservationFromMutable(mut)
	if ctx == nil || ctx.AnalysisIR == nil || mut == nil {
		return readStatusSourceInventoryShape(obs)
	}
	snapshot := sourceInventoryAuthoritySnapshotForReadScheduler(ctx, ctx.AnalysisIR, obs)
	return readStatusSourceInventorySnapshotShape(obs, snapshot)
}

func readStatusSourceInventoryShape(obs types.SourceInventoryObservation) string {
	if !obs.IsActive() {
		return "inactive"
	}
	memberCount := 0
	completeSetCount := 0
	for _, set := range obs.Sets {
		memberCount += len(set.Members)
		if set.Complete {
			completeSetCount++
		}
	}
	page := ""
	if obs.Page != nil {
		page = fmt.Sprintf(":%d:%d:%t:%s", obs.Page.Offset, obs.Page.Emitted, obs.Page.Complete, strings.TrimSpace(obs.Page.NextCursor))
	}
	exec := ""
	if obs.Execution != nil {
		exec = fmt.Sprintf(":%t:%t:%t", obs.Execution.Budgeted, obs.Execution.CandidateBudgetTruncated, obs.Execution.AttributesDeferred)
	}
	return fmt.Sprintf("active:%t:sets=%d:complete_sets=%d:members=%d:classes=%d:langs=%d%s%s",
		obs.Complete,
		len(obs.Sets),
		completeSetCount,
		memberCount,
		len(obs.SourceClasses),
		len(obs.RepoLanguages),
		page,
		exec,
	)
}

func readStatusSourceInventorySnapshotShape(obs types.SourceInventoryObservation, snapshot types.SourceInventoryAuthoritySnapshot) string {
	base := readStatusSourceInventoryShape(obs)
	if !snapshot.Active {
		return base
	}
	reasons := "none"
	if len(snapshot.ReasonCodes) > 0 {
		reasons = strings.Join(snapshot.ReasonCodes, ",")
	}
	return fmt.Sprintf("%s:authority=%t:need=%t:landing=%t:reasons=%s",
		base,
		snapshot.PrincipalAuthority,
		snapshot.NeedsFollowup,
		snapshot.CanEnterMechanicalLanding,
		reasons,
	)
}
