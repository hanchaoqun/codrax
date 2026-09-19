package repl

import (
	"path/filepath"
	"testing"
)

type traceRuntimeAnchorRunner struct {
	materialAwareRunner
	anchor string
}

func (r *traceRuntimeAnchorRunner) SetTraceRuntimeAnchor(anchor string) { r.anchor = anchor }

func TestNewREPLPropagatesStableTraceRuntimeAnchor(t *testing.T) {
	runner := &traceRuntimeAnchorRunner{}
	anchor := filepath.Join(t.TempDir(), "runtime")
	New(Config{Runner: runner, RuntimeAnchor: anchor, RepoRoot: t.TempDir(), Render: renderNothing})
	if runner.anchor != anchor {
		t.Fatalf("trace material would use another root: got %q want %q", runner.anchor, anchor)
	}
}
