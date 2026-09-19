package orchestrator

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SetTraceRuntimeAnchor binds derived trace material to the stable runtime
// directory. It must never use BusContext.WorkDir: that directory may be a
// per-Run temporary blob directory removed before citations are consumed.
func (o *Orchestrator) SetTraceRuntimeAnchor(anchor string) {
	if o != nil {
		o.traceRuntimeAnchor = absoluteTraceRuntimeAnchor(anchor)
	}
}

func absoluteTraceRuntimeAnchor(anchor string) string {
	if strings.TrimSpace(anchor) == "" {
		anchor = ".codrax"
	}
	if absolute, err := filepath.Abs(anchor); err == nil {
		return absolute
	}
	// The preparer rejects unusable anchors before conversion. Do not borrow
	// the source repository or a temporary working directory as a fallback.
	return anchor
}

func (o *Orchestrator) newTraceInputPreparer() types.TraceInputPreparer {
	return traceinput.NewCoordinator(traceinput.Options{
		RuntimeAnchor:         o.traceRuntimeAnchor,
		RuntimeAnchorFallback: hitraceconv.RuntimeAnchorFallback(o.traceRuntimeAnchor),
	})
}
