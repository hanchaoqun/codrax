package orchestrator

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/outputdump"
)

// dumpFinalOutputArgs keeps the orchestrator call sites and existing
// tests readable while the implementation lives in the shared
// outputdump package used by both pipeline and REPL-local answers.
type dumpFinalOutputArgs struct {
	dir      string
	max      int
	request  string
	answer   string
	hasLog   bool
	logBytes int
	hasTrace bool
	traceB   int
	now      time.Time
	pid      int
}

func (a dumpFinalOutputArgs) outputDumpArgs() outputdump.Args {
	return outputdump.Args{
		Dir:        a.dir,
		Max:        a.max,
		Request:    a.request,
		Answer:     a.answer,
		HasLog:     a.hasLog,
		LogBytes:   a.logBytes,
		HasTrace:   a.hasTrace,
		TraceBytes: a.traceB,
		Now:        a.now,
		PID:        a.pid,
	}
}

func writeFinalOutputDump(a dumpFinalOutputArgs) string {
	return outputdump.Write(a.outputDumpArgs())
}

func outputDumpFileName(now time.Time, pid int) string {
	return outputdump.FileName(now, pid)
}

func buildOutputDumpBody(a dumpFinalOutputArgs) string {
	return outputdump.BuildBody(a.outputDumpArgs())
}

func pruneOutputDumpDir(dir string, max int) {
	outputdump.PruneDir(dir, max)
}
