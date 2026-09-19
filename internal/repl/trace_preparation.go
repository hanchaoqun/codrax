package repl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Local file preparation is not a model turn. Cancellation captures this
// generation, never a runner's current/next Run. Commit and tuple publication
// share the lock with cancellation; input draining must happen OUTSIDE it.
type tracePreparationOperation struct {
	mu                sync.Mutex
	ctx               context.Context
	cancel            context.CancelFunc
	terminal          bool
	finished          bool
	shutdownRequested bool
	done              chan struct{}
	shutdownExit      chan struct{}
}

func newTracePreparationOperation() *tracePreparationOperation {
	ctx, cancel := context.WithCancel(context.Background())
	return &tracePreparationOperation{
		ctx: ctx, cancel: cancel, done: make(chan struct{}), shutdownExit: make(chan struct{}),
	}
}

func (op *tracePreparationOperation) Cancel(reason string) {
	op.cancelActive(reason)
}

// cancelActive reports whether this operation still accepted cancellation.
// Published/failed operations remain registered while input and owned-file
// cleanup finish; a late signal must not misreport their result as canceled.
func (op *tracePreparationOperation) cancelActive(string) bool {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.terminal {
		return false
	}
	op.cancel()
	return true
}

func (op *tracePreparationOperation) finish() {
	op.mu.Lock()
	if !op.finished {
		op.terminal = true
		op.finished = true
		op.cancel()
		close(op.done)
	}
	shutdown := op.shutdownRequested
	op.mu.Unlock()
	if shutdown {
		// Signal ownership includes the final process exit. Do not let the
		// REPL replay a queued question or /exit while its signal handler is
		// still cleaning up. Production never closes this channel: os.Exit
		// ends both goroutines; tests may close it to simulate that exit.
		<-op.shutdownExit
	}
}

// requestShutdown and finish linearize under the same lock. A request accepted
// before finish holds the REPL after its owned rollback/input drain completes.
// A finished operation is outside this local shutdown scope. Publication may
// already be terminal while draining, so it does not by itself reject shutdown.
func (op *tracePreparationOperation) requestShutdown() bool {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.finished {
		return false
	}
	op.shutdownRequested = true
	if !op.terminal {
		op.cancel()
	}
	return true
}

func (op *tracePreparationOperation) commit(pending *traceinput.Preparation, publish func(*attachment.TraceMaterial)) error {
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.terminal {
		return errors.New("trace preparation operation already finished")
	}
	material, err := pending.Commit(op.ctx)
	op.terminal = true
	if err != nil {
		return err
	}
	publish(material)
	return nil
}

func (r *REPL) prepareAttachedTrace(path string) {
	err := r.loadPreparedTrace(path)
	if err != nil {
		r.traceAttachmentFailed = true
		r.reportAttachmentTextIssue(err)
		r.warn("%s\n", traceAttachmentRecoveryMsg(r.language))
		return
	}
	r.resolveTraceAttachmentFailure()
	r.success(attachedHitraceLoadedMsg(r.language, path, len(r.attachedHitrace)))
	if r.attachedTraceMaterial != nil && !r.attachedTraceMaterial.SelfContainedText() {
		if isZh(r.language) {
			r.info("Trace 已就绪：模型预览有长度上限，查询仍使用完整文件；原始文件未修改。")
		} else {
			r.info("Trace ready: the model preview is bounded; queries use the complete file. The original is unchanged.")
		}
	}
}

func (r *REPL) loadPreparedTrace(path string) (err error) {
	// Preflight BEFORE any conversion. A preview-only runner must not silently
	// discard the complete material. nil is allowed for local command-only use.
	if r.runner != nil {
		_, bodyOK := r.runner.(attachedHitraceSetter)
		_, sourceOK := r.runner.(attachedHitraceSourceSetter)
		_, materialOK := r.runner.(attachedTraceMaterialSetter)
		if !bodyOK || !sourceOK || !materialOK {
			return errors.New("runner cannot retain complete prepared trace material")
		}
	}
	op := newTracePreparationOperation()
	if !r.tracePreparation.CompareAndSwap(nil, op) {
		op.finish()
		return errors.New("trace preparation is already running")
	}
	defer func() {
		op.finish() // after Discard; shutdown can now safely exit
		r.tracePreparation.CompareAndSwap(op, nil)
	}()
	r.installCancelSignalHandler()
	var lease *scriptInputLease
	if !r.interactive() {
		lease, err = r.scriptedInputOwner().beginRuntime(op.Cancel, nil)
		if err != nil {
			return err
		}
	}
	window := r.armInputWindow(r.tracePreparationCallbacks(op))
	defer func() {
		// Neither owner mutex may be acquired under op.mu (callbacks take it).
		receipt := lease.stopAndDrain()
		for _, line := range receipt.lines {
			r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: line})
		}
		if receipt.dropped > 0 {
			r.warn("[repl] preparation input queue full (%d lines / %d bytes): dropped %d line(s); /cancel remained available\n", cancelListenerQueueCap, receipt.byteCap, receipt.dropped)
		}
		r.drainTracePreparationWindow(window)
	}()
	anchor, err := filepath.Abs(firstNonEmptyString(r.runtimeAnchor, ".codrax"))
	if err != nil {
		return err
	}
	pending, err := traceinput.Begin(op.ctx, traceinput.Options{
		InputPath: path, RuntimeAnchor: anchor,
		RuntimeAnchorFallback: hitraceconv.RuntimeAnchorFallback(anchor),
		PreviewBytes:          r.attachedTraceMaxBytes,
		Progress: func(event hitraceconv.ProgressEvent) {
			r.info(htraceConvertProgressMsg(r.language, event))
		},
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, pending.Discard()) }()
	return op.commit(pending, func(material *attachment.TraceMaterial) {
		r.replaceAttachedTraceText(material.Preview(), mergeTraceSourceHints("", r.currentTraceSourceHint(path)))
		r.attachedTraceMaterial = material
		if setter, ok := r.runner.(attachedTraceMaterialSetter); ok {
			setter.SetAttachedTraceMaterial(material) // raw setter withdraws the old receipt
		}
	})
}

func (r *REPL) tracePreparationCallbacks(op *tracePreparationOperation) runInputWindowCallbacks {
	return runInputWindowCallbacks{
		consumeCommand: func(line string) bool {
			if !isScriptCancelCommand(line) {
				return false
			}
			op.Cancel("/cancel")
			return true
		},
		onCtrlC: func() { op.Cancel("Ctrl+C") },
		onEsc:   func() { op.Cancel("esc") },
		onLine: func(line string) {
			if r.renderer != nil {
				r.renderer.CommitUserInputLine(line)
			}
		},
		onPaste: func(n int) {
			if r.renderer != nil {
				r.renderer.CommitUserInputLine(runInputPasteQueuedLabel(r.language, n))
			}
		},
	}
}

func (r *REPL) drainTracePreparationWindow(w *runInputWindow) {
	if w == nil {
		return
	}
	fmt.Fprint(r.out, ansiDisableBracketed)
	queued, partial, _, dropped := w.drain()
	for _, line := range queued {
		r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: line})
	}
	for _, blob := range w.takePastes() {
		r.pendingFollowUps = append(r.pendingFollowUps, pendingFollowUp{Text: blob, Verbatim: true})
	}
	if partial != "" {
		r.pendingInputPrefill = partial
	}
	if dropped {
		r.warn("%s\n", runInputDroppedMsg(r.language))
	}
}

// This is an explicit failed-command state, not a classifier over questions.
// Both queued and not-yet-read script input go through it before any LLM call.
func (r *REPL) holdInputAfterTraceFailure(entry pendingFollowUp) bool {
	if !r.traceAttachmentFailed {
		return false
	}
	if !entry.Verbatim {
		command := strings.Fields(types.NormalizeREPLCommandAlias(entry.Text))
		if len(command) > 0 {
			switch command[0] {
			case "/htrace", "/help", "/exit", "/cancel":
				return false
			}
		}
	}
	if len(r.heldTraceInputs) >= cancelListenerQueueCap || len(entry.Text) > scriptInputQueueBytes-r.heldTraceInputBytes {
		r.warn("%s\n", runInputDroppedMsg(r.language))
		return true
	}
	r.heldTraceInputs = append(r.heldTraceInputs, entry)
	r.heldTraceInputBytes += len(entry.Text)
	r.warn("%s\n", traceAttachmentRecoveryMsg(r.language))
	return true
}

func (r *REPL) resolveTraceAttachmentFailure() {
	if !r.traceAttachmentFailed {
		return
	}
	r.traceAttachmentFailed = false
	r.pendingFollowUps = append(r.heldTraceInputs, r.pendingFollowUps...)
	r.heldTraceInputs, r.heldTraceInputBytes = nil, 0
	if isZh(r.language) {
		r.info("已确认附件状态，暂存输入将继续处理。")
	} else {
		r.info("Attachment choice confirmed; held input will resume.")
	}
}

func traceAttachmentRecoveryMsg(lang string) string {
	if isZh(lang) {
		return "新 Trace 未加载成功，旧附件保持不变，后续输入已暂存，不会自动用旧附件作答。请重新 /htrace <path>，或 /htrace keep 明确沿用当前附件，或 /htrace clear 清除附件后继续；/htrace show 可查看，/exit 可退出。"
	}
	return "The new trace was not loaded; the previous attachment is unchanged. Following input is held, never answered against the old trace automatically. Retry /htrace <path>, confirm /htrace keep, or /htrace clear to continue without it; /htrace show inspects it and /exit quits."
}
