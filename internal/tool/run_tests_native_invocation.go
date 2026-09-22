package tool

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

var nativeTestInvocationSequence atomic.Uint64

// This is a tool-produced correlation identity, not a secret, a successful
// verdict, report-file freshness, or proof that a particular source file ran.
// The sequence separates concurrent commands even on a coarse clock; process
// and time separate independent executions and persisted reports.
func newNativeTestInvocationID() string {
	return fmt.Sprintf("native:%x:%x:%x", os.Getpid(), time.Now().UnixNano(), nativeTestInvocationSequence.Add(1))
}

// Call only on one native invocation's parsed leaf, before any merge. Never
// relabel a report aggregate, a probe, a locked reverify, or prior observations.
func bindNativeTestResultInvocation(report *types.ChangeReport, invocationID string) {
	if report == nil || invocationID == "" {
		return
	}
	for i := range report.TestResults {
		if report.TestResults[i].InvocationID == "" {
			report.TestResults[i].InvocationID = invocationID
		}
	}
}
