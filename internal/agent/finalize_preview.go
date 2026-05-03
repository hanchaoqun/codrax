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
	// rawArgs accumulates the full streamed tool-call arguments
	// (NOT just the extracted "summary" field). Populated alongside
	// buf so V2 partial salvage can scan the raw JSON for V2 block
	// shapes when the V1 SummaryExtractor produced nothing. B3 落地
	// — see Partial() doc.
	rawArgs    strings.Builder
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
	// rawArgs always captures the chunk so V2 partial salvage can
	// scan the full streamed JSON later, even when the V1 extractor
	// produced no `summary` decode.
	h.rawArgs.WriteString(argsChunk)
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

// Partial returns the cumulative decoded `summary` text the hook
// has accumulated from in-flight emit_answer_document tool-call
// argument chunks. Returns "" when no chunks streamed (e.g. EOF at
// iter=0 with the connection closed before any response data).
//
// The orchestrator's force-finalize escape path consults this when
// a Chat error fires before the tool call closes: when partial
// summary text exists, BaseAgent synthesises an assistant message
// containing the partial prose and feeds it through the finalizer's
// existing "missing emit_answer_document" fallback (which surfaces
// the last assistant content as the answer with a warning banner).
// Without this, a connection drop mid-summary would discard 5-30 KB
// of streamed prose and force the user to retry from scratch.
//
// V2 carrier note (B3, block_only_carrier.md §5.3 B3-T6):
// The V1 SummaryExtractor recognises the `summary` JSON field at
// the document's top level. V2 emits express the answer's lead-in
// prose through a BlockSummary block whose `text` field carries the
// equivalent prose; that text reaches this hook through the same
// argsChunk stream but the V1 extractor does not key on it. As a
// transitional measure, when the buffered text already contains
// enough V2-shape signal, the V2 partial salvage helper below
// drains BlockSummary.text instead. This lets a connection drop
// mid-V2-emit still rescue the prose. B5 will replace this with a
// proper V2-aware streaming extractor; B3's helper is the minimum
// viable path so V2 emits don't silently lose preview / partial-
// salvage parity with V1.
//
// Nil-safe (mirrors flush()): non-finalize stages pass nil hooks.
func (h *finalizePreviewHook) Partial() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	primary := h.buf.String()
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	// V1 extractor produced nothing (no top-level "summary" field
	// in the streamed JSON). Try the V2 fall-back: extract the first
	// BlockSummary.text we can find in the raw stream-collected
	// arguments.
	return v2PartialFromArgsChunk(h.rawArgs.String())
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

// v2PartialFromArgsChunk extracts the first BlockSummary text from a
// streamed V2 emit_answer_document tool-call argument string. Used
// as a fallback when the V1 SummaryExtractor produced nothing — a
// V2 emit has no top-level "summary" field so the V1 extractor
// returns "" and the partial-salvage path needs another route.
//
// The implementation is a deliberately simple character-level
// state machine: it scans for the first "blocks" array, then within
// it looks for the first object whose "kind" value is "summary",
// then captures that object's "text" field. Truncated streams are
// fine — we return whatever text characters we have collected.
//
// B3 落地 — minimal viable; B5 will replace this with a proper
// V2-aware streaming extractor once the V2 carrier becomes default.
func v2PartialFromArgsChunk(rawArgs string) string {
	if !strings.Contains(rawArgs, `"v2"`) {
		return ""
	}
	idx := strings.Index(rawArgs, `"kind":"summary"`)
	if idx < 0 {
		idx = strings.Index(rawArgs, `"kind": "summary"`)
		if idx < 0 {
			return ""
		}
	}
	tail := rawArgs[idx:]
	textKey := strings.Index(tail, `"text"`)
	if textKey < 0 {
		return ""
	}
	tail = tail[textKey:]
	colon := strings.Index(tail, ":")
	if colon < 0 {
		return ""
	}
	tail = strings.TrimLeft(tail[colon+1:], " \t\r\n")
	if !strings.HasPrefix(tail, `"`) {
		return ""
	}
	tail = tail[1:]
	var b strings.Builder
	for i := 0; i < len(tail); i++ {
		c := tail[i]
		if c == '\\' && i+1 < len(tail) {
			next := tail[i+1]
			switch next {
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			case '/':
				b.WriteByte('/')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			i++
			continue
		}
		if c == '"' {
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}
