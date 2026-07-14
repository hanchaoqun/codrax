package tracequery

// run_cancel.go — SUPP-CANCEL (§29.71 立案 / 批4 P3-5, 2026-07-14): in-view
// cooperative cancellation for the indexed view engine.
//
// Disease: the duration fuses around the engine were BETWEEN-view only (the
// supplement's 20s between-view deadline, the tool call boundary) — inside a
// single Run the rank/stats aggregation sweeps and the composite chain build
// were uninterruptible, so a pathological window (scoped MaxEvents raises the
// index into the millions of events) could stall report assembly unboundedly.
//
// Design (carrier choice, blast-radius comparison):
//   - Run(ctx, idx, q) signature change — ~130 call sites (115 in-package
//     test calls + 4 production + 17 external test calls) plus every directly
//     tested builder (ComputeWindowStats / BuildWakeupChain / …) would need
//     the same treatment. Rejected on churn.
//   - ctx on *Index — the index is CACHED and shared across concurrent
//     queries and lanes (parse cache singleflight), so a context stored there
//     races across queries and cancels the wrong lane. Rejected on
//     correctness.
//   - ctx on Query (chosen) — Query travels by value through every builder
//     already (unexported-plumbing precedent: chainAnchorWindowsByPID), so an
//     unexported *runCancelState pointer field reaches every scan loop with
//     ZERO existing call-site churn, and a nil carrier (every caller that
//     never opted in: tracediag, direct test calls) is structurally
//     byte-identical behavior.
//
// Lane contract (single value source — all three lanes share Run):
//   - model lane: the tool layer threads its BusContext context in via
//     Query.WithRunContext (timeout semantics = whatever the existing tool
//     context chain carries);
//   - supplement lane: same entry, but the context carries the remaining
//     trace_supplement_max_duration_ms budget as a deadline;
//   - tracediag lane: never calls WithRunContext — nil carrier, never
//     cancels, byte-identical by construction.
//
// Cancellation semantics (禁裸丢 / 禁半账):
//   - sampling is cooperative and event-count-based: one counter increment
//     per scan unit, the context is consulted every 64Ki units (zero hot-path
//     cost otherwise);
//   - once fired, the in-flight builder aborts early and Run's attach gates
//     DISCARD every face whose construction had not completed before the
//     fire — a partial aggregate is never published;
//   - faces attached before the fire are complete builder outputs and stay
//     published;
//   - the Result carries a typed ViewCancellation record plus one honest
//     caveat naming the discarded faces — never a silent drop.
//
// Concurrency: Run and its builders execute on one goroutine per query (each
// tool Execute constructs its own Query), so the state deliberately uses no
// atomics; the carrier must not be shared across concurrently running
// queries.

import (
	"context"
	"fmt"
	"strings"
)

// runCancelSampleMask makes tick consult the context every 64Ki scan units:
// one increment + one mask test per unit on the hot path.
const runCancelSampleMask = (1 << 16) - 1

// runCancelState is the per-Run cooperative-cancellation carrier. A nil
// *runCancelState is fully functional (never fires) so builders can call the
// methods unconditionally.
type runCancelState struct {
	ctx    context.Context
	units  int64
	fireAt int64
	isDone bool
	reason string
	// discarded collects the face names Run's attach gates refused to
	// publish after the fire (already deduplicated by the append helper).
	discarded []string
}

// newRunCancelState attaches a carrier only for contexts that can actually
// be canceled: nil contexts and never-canceled contexts (context.Background,
// context.TODO, plain value contexts — Done() == nil) allocate nothing, so
// those lanes stay structurally byte-identical.
func newRunCancelState(ctx context.Context) *runCancelState {
	if ctx == nil || ctx.Done() == nil {
		return nil
	}
	return &runCancelState{ctx: ctx}
}

// tick registers one scan unit and reports whether cooperative cancellation
// has fired. The context is sampled every 64Ki units; after the first fire
// every call returns true immediately so enclosing loops exit fast.
func (c *runCancelState) tick() bool {
	if c == nil {
		return false
	}
	if c.isDone {
		return true
	}
	c.units++
	if c.units&runCancelSampleMask != 0 {
		return false
	}
	if err := c.ctx.Err(); err != nil {
		c.isDone = true
		c.fireAt = c.units
		c.reason = runCancelReason(err)
	}
	return c.isDone
}

// sample consults the context immediately — a loop/phase-BOUNDARY sampling
// point (not modulo-gated). Placed between the heavy sub-passes of a builder
// so a cancellation that lands inside an uninstrumented pass is observed at
// the next boundary instead of after the whole builder.
func (c *runCancelState) sample() bool {
	if c == nil {
		return false
	}
	if c.isDone {
		return true
	}
	if err := c.ctx.Err(); err != nil {
		c.isDone = true
		c.fireAt = c.units
		c.reason = runCancelReason(err)
	}
	return c.isDone
}

// fired reports whether a sampling point has already observed the
// cancellation. It never consults the context itself: a builder that ran to
// completion without any sampling point firing produced a COMPLETE value even
// if the context expired meanwhile, and the attach gates use exactly this
// property.
func (c *runCancelState) fired() bool {
	return c != nil && c.isDone
}

// discardFace records one face name that an attach gate refused to publish.
func (c *runCancelState) discardFace(face string) {
	if c == nil || strings.TrimSpace(face) == "" {
		return
	}
	for _, existing := range c.discarded {
		if existing == face {
			return
		}
	}
	c.discarded = append(c.discarded, face)
}

// record mints the typed wire record for the Result.
func (c *runCancelState) record(view string) *ViewCancellation {
	if !c.fired() {
		return nil
	}
	return &ViewCancellation{
		View:           view,
		Reason:         c.reason,
		ScannedUnits:   c.fireAt,
		DiscardedFaces: append([]string(nil), c.discarded...),
	}
}

// caveat renders the honest disclosure line for the Result caveat lane. The
// wording is answer/model-facing: it states exactly what was discarded, that
// published faces are complete, and the recovery move.
func (c *runCancelState) caveat(view string) string {
	faces := "none"
	if len(c.discarded) > 0 {
		faces = strings.Join(c.discarded, ",")
	}
	return fmt.Sprintf("in_view_cancellation=true; reason=%s; the %s view stopped at a cooperative cancellation point after %d scan step(s); unfinished result sections were discarded whole instead of being published as partial aggregates (discarded: %s); every section present in this result is complete — narrow the time window or reduce the scope and re-run to complete the view", c.reason, view, c.fireAt, faces)
}

// runCancelReason maps the context error to the typed reason enum.
func runCancelReason(err error) string {
	if err == context.DeadlineExceeded {
		return "deadline_exceeded"
	}
	return "canceled"
}

// WithRunContext returns a copy of q that carries ctx for cooperative
// in-view cancellation sampling inside Run and the indexed view builders.
// nil and never-cancelable contexts (Done() == nil, e.g. context.Background)
// attach nothing — behavior stays byte-identical to a query without a
// context. The three production lanes share this single entry: the model
// tool lane (existing tool context chain), the supplement lane (deadline =
// remaining duration budget), and tracediag (which deliberately never calls
// it — nil carrier, never cancels).
func (q Query) WithRunContext(ctx context.Context) Query {
	q.runCancel = newRunCancelState(ctx)
	return q
}
