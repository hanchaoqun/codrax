package repl

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/loginput"
)

const completeLogPasteMaxBytes = 1 << 30

// A small model preview does not make a complete input line unreadable.
func (r *REPL) logInputLineLimit() int { return max(r.attachedLogMaxBytes+1, (1<<20)+1) }

type attachedLogCatalogSetter interface{ SetAttachedLogCatalog(*loginput.Catalog) }
type attachedLogCatalogGetter interface{ AttachedLogCatalog() *loginput.Catalog }

func (r *REPL) seedAttachedLogCatalogFromRunner() {
	if getter, ok := r.runner.(attachedLogCatalogGetter); ok {
		r.attachedLogCatalog = getter.AttachedLogCatalog()
	}
}

func (r *REPL) propagateAttachedLog() error {
	setter, canCarry := r.runner.(attachedLogCatalogSetter)
	if r.attachedLogCatalog != nil && !canCarry {
		return fmt.Errorf("runner cannot retain complete log sources; reattach with a log-query-capable runner")
	}
	if textSetter, ok := r.runner.(attachedLogSetter); ok {
		textSetter.SetAttachedLog(r.attachedLog)
	}
	if canCarry {
		setter.SetAttachedLogCatalog(r.attachedLogCatalog)
	}
	return nil
}

func (r *REPL) replaceAttachedLogText(body string) {
	r.attachedLog, r.attachedLogCatalog = body, nil
	r.attachedLogAutoRouted, r.attachedLogAutoRestored = false, false
	if setter, ok := r.runner.(attachedLogSetter); ok {
		setter.SetAttachedLog(body)
	}
	if setter, ok := r.runner.(attachedLogCatalogSetter); ok {
		setter.SetAttachedLogCatalog(nil)
	}
}

func (r *REPL) publishLogCatalog(catalog *loginput.Catalog) {
	r.replaceAttachedLogText(catalog.Preview(r.attachedLogMaxBytes))
	r.attachedLogCatalog = catalog
	if setter, ok := r.runner.(attachedLogCatalogSetter); ok {
		setter.SetAttachedLogCatalog(catalog)
	}
}

func (r *REPL) prepareAttachedLogFile(path string, appendSource bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	r.prepareAttachedLogFileContext(ctx, path, appendSource)
}

func (r *REPL) prepareAttachedLogFileContext(ctx context.Context, path string, appendSource bool) {
	if err := attachment.ValidateSourceLabel(path); err != nil {
		r.errorf("log: %v\n", err)
		return
	}
	if path == "" {
		r.errorf("log: missing path\n")
		return
	}
	if appendSource && r.attachedLogCatalog == nil && r.attachedLog != "" {
		// A plain text setter, old import or restored preview supplies bytes but
		// no completeness receipt. Only a fresh complete-input preparation may
		// grant that authority; preserve the old preview when refusing to mix.
		r.errorf("log: %s\n", logAppendReattachMessage(r.language))
		return
	}
	inputs := []loginput.Input{{Path: path}}
	var catalog *loginput.Catalog
	var err error
	if appendSource && r.attachedLogCatalog != nil {
		catalog, err = r.attachedLogCatalog.Append(ctx, inputs)
	} else {
		catalog, err = loginput.Prepare(ctx, inputs, loginput.Options{})
	}
	if err != nil {
		r.replaceAttachedLogText("")
		r.errorf("log: %v\n", err)
		return
	}
	r.publishLogCatalog(catalog)
	var fullBytes int64
	for _, source := range catalog.Sources() {
		fullBytes += source.DecodedBytes
	}
	if fullBytes > int64(len(r.attachedLog)) {
		r.info("Log preview is bounded; queries retain complete source bytes.\n")
	}
	r.success(attachedLogLoadedMsg(r.language, path, len(r.attachedLog)))
}

func (r *REPL) prepareAttachedLogText(body, name string) bool {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: name, Data: []byte(body)}}, loginput.Options{})
	if err != nil {
		r.replaceAttachedLogText("")
		r.errorf("log: %v\n", err)
		return false
	}
	r.publishLogCatalog(catalog)
	return true
}

func (r *REPL) persistLogRuntimeArtifact(body string) (RuntimeArtifactRef, error) {
	if r.attachedLogCatalog != nil {
		return r.runtimeArtifactStore.putLogPreview(body)
	}
	return r.runtimeArtifactStore.Put("log", body, runtimeArtifactSourceFromAttachment("log", body))
}

func preparedLogReattachMessage(lang string) string {
	if isZh(lang) {
		return "已保存的日志仅为预览，不能恢复为完整附件。请使用 /log 重新附加原文件或完整文本。"
	}
	return "The saved log is a preview, not a complete attachment. Reattach the original files or complete text with /log."
}

func logAppendReattachMessage(lang string) string {
	if isZh(lang) {
		return "无法确认当前日志是否完整，不能直接追加。请使用 /log 重新附加原文件或完整文本后再追加；当前预览保持不变。"
	}
	return "The current log's completeness is unknown. Reattach the original files or complete text with /log before appending; the current preview is unchanged."
}
