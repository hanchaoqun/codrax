package orchestrator

import (
	"os"
	"time"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

// dumpFinalOutputArgs keeps the orchestrator call sites and existing
// tests readable while the implementation lives in the shared
// outputdump package used by both pipeline and REPL-local answers.
type dumpFinalOutputArgs struct {
	dir                        string
	max                        int
	language                   string
	request                    string
	answer                     string
	hasLog                     bool
	logBytes                   int
	hasTrace                   bool
	traceB                     int
	artifacts                  []outputdump.RuntimeArtifact
	rootCauses                 *types.TraceRootCauseReportV2
	requireRootCauseJSON       bool
	rootCauseUnavailableReason string
	now                        time.Time
	pid                        int
}

func (a dumpFinalOutputArgs) outputDumpArgs() outputdump.Args {
	return outputdump.Args{
		Dir:                        a.dir,
		Max:                        a.max,
		Language:                   a.language,
		Request:                    a.request,
		Answer:                     a.answer,
		HasLog:                     a.hasLog,
		LogBytes:                   a.logBytes,
		HasTrace:                   a.hasTrace,
		TraceBytes:                 a.traceB,
		RuntimeArtifacts:           append([]outputdump.RuntimeArtifact(nil), a.artifacts...),
		RootCauseReport:            a.rootCauses,
		RequireRootCauseJSON:       a.requireRootCauseJSON,
		RootCauseUnavailableReason: a.rootCauseUnavailableReason,
		Now:                        a.now,
		PID:                        a.pid,
	}
}

func writeFinalOutputDump(a dumpFinalOutputArgs) string {
	return outputdump.Write(a.outputDumpArgs())
}

func writeFinalOutputDumpResult(a dumpFinalOutputArgs) outputdump.Result {
	return outputdump.WriteResult(a.outputDumpArgs())
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

// SetOutputDump configures the per-Run output directory (normally
// <CWD>/.codrax/output/). Empty explicitly disables default artifacts.
// max bounds retained runs, including standalone root-cause failure files;
// non-positive max disables pruning.
func (o *Orchestrator) SetOutputDump(dir string, max int) {
	o.outputDumpDir = dir
	o.outputDumpMax = max
}

// hasTraceRootCauseOutputContext consumes attachments and typed runtime
// contracts/policies only. Merely mentioning "trace" or a converter fixture
// in source-analysis prose must not turn that task into a Trace diagnosis.
func (o *Orchestrator) hasTraceRootCauseOutputContext(bus *types.BusContext) bool {
	if o.attachedHitrace != "" {
		return true
	}
	if bus == nil {
		return false
	}
	if bus.Mutable != nil && (bus.Mutable.TraceFindingContract() != nil || bus.Mutable.TraceRootCauseReport() != nil || bus.Mutable.TraceQueryRuntimeObservationCount() > 0) {
		return true
	}
	if bus.AnalysisIR == nil {
		return false
	}
	policy := bus.AnalysisIR.RequestModel.ExternalObservationPolicy
	if policy == nil || (!policy.ExcludesCurrentSource() && !policy.ArtifactCitationsExternalOnly()) {
		return false
	}
	for _, artifact := range bus.RuntimeArtifactPreflight.Artifacts {
		if artifact.RuntimeArtifactKind() == "trace" {
			return true
		}
	}
	return false
}

func (o *Orchestrator) ensureDefaultTraceRootCauseOutput(bus *types.BusContext) error {
	if o.rootCauseOutputErr != nil {
		return o.rootCauseOutputErr
	}
	if o.outputDumpDir == "" || !o.hasTraceRootCauseOutputContext(bus) {
		return nil
	}
	var mutable *types.MutableState
	if bus != nil {
		mutable = bus.Mutable
	}
	if mutable != nil && mutable.FinalAnswerRootCauseJSONPath() != "" {
		return nil
	}
	// No successful final-output hook: publish no unfinalized draft selection.
	result := outputdump.WriteRootCauseOnly(outputdump.Args{
		Dir: o.outputDumpDir, Max: o.outputDumpMax, HasTrace: true,
		RootCauseUnavailableReason: outputdump.RootCauseReasonTranscriptNotAvailable,
		Now:                        time.Now(), PID: os.Getpid(),
	})
	o.rootCauseOutputErr = result.RootCauseJSONError
	if mutable != nil && result.RootCauseJSONPath != "" {
		mutable.SetFinalAnswerArtifactPaths(mutable.FinalAnswerMarkdownPath(), mutable.FinalAnswerHTMLPath(), result.RootCauseJSONPath)
	}
	return o.rootCauseOutputErr
}

// RootCauseOutputError reports file delivery separately from analysis. A
// frontend can show the intact model answer before returning a file error.
// Run itself must not fail on this error: existing callers discard answers
// when Run fails. The CLI checks this at its post-answer output boundary.
func (o *Orchestrator) RootCauseOutputError() error {
	return o.rootCauseOutputErr
}
