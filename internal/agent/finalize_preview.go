package agent

import (
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalizePreviewHook is the BaseAgent-side wiring that turns the
// LLM adapter's OnToolCallDelta callback into a stream of
// EventLivePreviewChunk events the renderer can show in a live area.
//
// Lifecycle, per dispatch:
//
//  1. Constructed lazily inside BaseAgent.Execute when ctx.Stage ==
//     StageFinalize. Other stages get a nil hook → onToolCallDelta
//     resolves to nil (passing nil into ChatOptions is the no-op
//     contract) so non-finalize streams pay zero overhead.
//
//  2. Each tool-call argument chunk that streams in fires
//     onToolCallDelta. We filter to the `emit_answer_document` tool
//     (the finalizer schema's only emit) and feed the chunk to a
//     SummaryExtractor.
//
//  3. The extractor returns decoded summary text. We accumulate
//     into a single Builder (the renderer needs cumulative — pterm
//     Area replaces the whole content per Update) and emit
//     EventLivePreviewChunk with the running buffer.
//
//  4. NOT responsible for emitting EventLivePreviewClear. That is
//     the orchestrator's job — it knows whether the finalize call
//     was accepted or rejected by the AnswerContract / Tier1 floor
//     gates. The Clear (with PreviewRejected appropriately set) is
//     emitted right before the next round dispatches OR right
//     before the bordered final answer prints.
//
// Concurrency: onToolCallDelta is called from the SSE-reading
// goroutine inside the LLM adapter. Multiple chunks can arrive
// rapidly. The internal mutex guards the buffer + extractor; emit
// is invoked OUTSIDE the lock so a slow renderer cannot back-
// pressure the stream consumer.
//
// Panic isolation: any panic in the extractor or emit is recovered
// silently — the preview path is purely cosmetic and must never
// take down the LLM stream consumer that the byte-identity
// invariant depends on.
type finalizePreviewHook struct {
	emit       render.EventEmitter
	extractor  llm.SummaryExtractor
	mu         sync.Mutex
	buf        strings.Builder
	lastEmitAt time.Time
}

// finalizePreviewThrottle mirrors streamPreviewThrottle in agent.go:
// the minimum gap between consecutive EventLivePreviewChunk
// emissions. 250 ms keeps pterm.Area.Update at ~4 Hz which is
// responsive to the eye but well below the rate at which token
// chunks can arrive on a fast SSE stream. Without throttling, a
// burst of small chunks would back-pressure the LLM stream-reader
// goroutine on the renderer's mutex.
const finalizePreviewThrottle = 250 * time.Millisecond

// newFinalizePreviewHook constructs the hook. emit may be a no-op
// emitter (testing); the extractor is zero-value initialised which
// is the correct starting state.
func newFinalizePreviewHook(emit render.EventEmitter) *finalizePreviewHook {
	return &finalizePreviewHook{emit: emit}
}

// onToolCallDelta is the OnToolCallDelta-shaped method bound to
// this instance. Returns the bound function so callers can pass it
// directly to llm.ChatOptions.OnToolCallDelta. nil-safe: a nil
// receiver yields a nil function, which ChatOptions accepts as
// "no callback".
func (h *finalizePreviewHook) onToolCallDelta(index int, name string, argsChunk string) {
	if h == nil {
		return
	}
	defer func() {
		// Cosmetic preview path: any panic here is swallowed so the
		// SSE consumer can keep accumulating for the canonical parse.
		_ = recover()
	}()
	if name != "" && name != "emit_answer_document" {
		// Tool name has been observed AND it is not the finalizer's
		// emit. Nothing to preview.
		return
	}
	if argsChunk == "" {
		return
	}

	h.mu.Lock()
	decoded, _ := h.extractor.Feed(argsChunk)
	if decoded == "" {
		h.mu.Unlock()
		return
	}
	h.buf.WriteString(decoded)
	now := time.Now()
	// Throttle: skip the emit when the last one was less than
	// finalizePreviewThrottle ago. The buffer has already absorbed
	// the new bytes, so the NEXT chunk's emission carries them.
	// Worst case (stream ends within the throttle window) the
	// orchestrator's EventLivePreviewClear still fires and the
	// renderer's area is torn down — preview just "missed" the
	// last 250 ms of streaming, which is acceptable.
	if now.Sub(h.lastEmitAt) < finalizePreviewThrottle {
		h.mu.Unlock()
		return
	}
	h.lastEmitAt = now
	cumulative := h.buf.String()
	h.mu.Unlock()

	if h.emit == nil {
		return
	}
	h.emit(render.Event{
		Kind:        render.EventLivePreviewChunk,
		Timestamp:   now,
		Stage:       types.StageFinalize,
		PreviewText: cumulative,
	})
}

// flush forces a final emit of the cumulative buffer regardless of
// the throttle window. Called by BaseAgent after Chat returns so
// the renderer paints the LAST 250ms of summary text the throttle
// might have skipped, BEFORE the orchestrator's Clear tears the
// preview area down. Safe to call on a nil hook (non-finalize
// stages) or on a hook that never received any chunks (empty buf).
func (h *finalizePreviewHook) flush() {
	if h == nil || h.emit == nil {
		return
	}
	defer func() { _ = recover() }()
	h.mu.Lock()
	if h.buf.Len() == 0 {
		h.mu.Unlock()
		return
	}
	cumulative := h.buf.String()
	h.lastEmitAt = time.Now()
	h.mu.Unlock()
	h.emit(render.Event{
		Kind:        render.EventLivePreviewChunk,
		Timestamp:   time.Now(),
		Stage:       types.StageFinalize,
		PreviewText: cumulative,
	})
}
