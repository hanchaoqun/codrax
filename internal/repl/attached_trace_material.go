package repl

import (
	"context"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

type attachedTraceMaterialGetter interface {
	AttachedTraceMaterial() *attachment.TraceMaterial
}

type attachedTraceMaterialSetter interface {
	SetAttachedTraceMaterial(*attachment.TraceMaterial)
}

func (r *REPL) seedAttachedTraceFromRunner() {
	if getter, ok := r.runner.(attachedHitraceGetter); ok {
		r.attachedHitrace = getter.AttachedHitrace()
	}
	if getter, ok := r.runner.(attachedHitraceSourceGetter); ok {
		r.attachedHitraceSource = getter.AttachedHitraceSource()
	}
	if getter, ok := r.runner.(attachedTraceMaterialGetter); ok {
		r.attachedTraceMaterial = getter.AttachedTraceMaterial()
	}
}

// Preserve the complete-file receipt when dispatching a CLI-seeded preview.
// The plain trace setter intentionally clears a prior receipt, so the typed
// setter must be last. A runner that cannot carry it must not get only preview.
func (r *REPL) propagateAttachedTrace() error {
	materialSetter, canCarry := r.runner.(attachedTraceMaterialSetter)
	if r.attachedTraceMaterial != nil {
		if !canCarry {
			return fmt.Errorf("runner cannot retain complete prepared trace material; reattach the source")
		}
		if err := r.attachedTraceMaterial.Validate(context.Background(), r.attachedHitrace); err != nil {
			return err
		}
	}
	if setter, ok := r.runner.(attachedHitraceSetter); ok {
		setter.SetAttachedHitrace(r.attachedHitrace)
	}
	if setter, ok := r.runner.(attachedHitraceSourceSetter); ok {
		setter.SetAttachedHitraceSource(r.attachedHitraceSource)
	}
	if canCarry {
		materialSetter.SetAttachedTraceMaterial(r.attachedTraceMaterial)
	}
	return nil
}

func (r *REPL) replaceAttachedTraceText(body, source string) {
	r.attachedHitrace, r.attachedHitraceSource = body, source
	r.attachedHitraceAutoRestored = false
	r.attachedTraceMaterial = nil
	// Slash-command runs such as /read-runs resume bypass ordinary dispatch.
	// Publish the replacement now so they cannot observe the old preview
	// after its complete-material receipt has been withdrawn.
	if setter, ok := r.runner.(attachedHitraceSetter); ok {
		setter.SetAttachedHitrace(body)
	}
	if setter, ok := r.runner.(attachedHitraceSourceSetter); ok {
		setter.SetAttachedHitraceSource(source)
	}
	if setter, ok := r.runner.(attachedTraceMaterialSetter); ok {
		setter.SetAttachedTraceMaterial(nil)
	}
}

func (r *REPL) clearAttachedTrace() {
	r.replaceAttachedTraceText("", "")
}

func (r *REPL) persistTraceRuntimeArtifact(body, source string) (RuntimeArtifactRef, error) {
	if r.attachedTraceMaterial != nil && !r.attachedTraceMaterial.SelfContainedText() {
		return r.runtimeArtifactStore.put("trace", body, source, true, r.attachedTraceMaterial.SourcePath())
	}
	return r.runtimeArtifactStore.Put("trace", body, source)
}

func preparedTraceReattachMessage(lang, originalPath string) string {
	if isZh(lang) {
		return fmt.Sprintf("已保存的 trace 仅为预览，不能恢复为完整附件。请重新附加原始文件 %q；二进制文件目前请通过 CLI --htrace 接入。", originalPath)
	}
	return fmt.Sprintf("The saved trace is a preview, not a complete attachment. Reattach original file %q; use CLI --htrace for binary files until REPL preparation is available.", originalPath)
}
