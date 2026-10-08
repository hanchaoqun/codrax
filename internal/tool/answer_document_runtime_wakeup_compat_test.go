package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeWakeupSharedAuthorityWireCompatibility(t *testing.T) {
	for _, comm := range []string{"", "loader", "line\n\r\t\x00end", strings.Repeat("线程", 60), strings.Repeat("a", 201), string([]byte{0xff, 'a'})} {
		for _, pid := range []int{0, 42} {
			thread := tracequery.ThreadRef{Comm: comm, PID: pid}
			old := sanitizeForBanner(comm)
			switch {
			case old != "" && pid > 0:
				old = fmt.Sprintf("%s-%d", old, pid)
			case old == "" && pid > 0:
				old = fmt.Sprintf("pid=%d", pid)
			case old == "":
				old = "unknown-thread"
			}
			if got := traceThreadLabel(thread); got != old {
				t.Fatalf("label bytes changed: got %q want %q", got, old)
			}
		}
	}
	ctx, _, _ := runtimeWakeFixture(t)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}})
	ledger := types.ObservationLedger{Records: result.Observations}
	rows := RuntimeDiagramRelations(ledger, &ctx.AnalysisIR.RequestModel)
	events := types.RuntimeWakeupDiagramEvents(ledger, &ctx.AnalysisIR.RequestModel)
	if len(rows) != 4 || len(events) != 4 {
		t.Fatalf("want four original events, got %d/%d", len(rows), len(events))
	}
	for i, event := range events {
		// This is the pre-move carrier. Its field order, tags, and ThreadRef
		// encoding are part of the existing event credential byte identity.
		var old struct {
			IndexPath string                           `json:"index_path"`
			From      string                           `json:"from"`
			To        string                           `json:"to"`
			Waker     tracequery.ThreadRef             `json:"waker"`
			Wakee     tracequery.ThreadRef             `json:"wakee"`
			Timestamp float64                          `json:"timestamp"`
			Line      int                              `json:"line"`
			Branch    int                              `json:"branch"`
			Segment   int                              `json:"segment"`
			ScanScope *types.TraceEventSearchScanScope `json:"scan_scope,omitempty"`
		}
		if err := json.Unmarshal(event.Fingerprint, &old); err != nil {
			t.Fatal(err)
		}
		legacyData, _ := json.Marshal(old)
		if string(legacyData) != string(event.Fingerprint) {
			t.Fatal("event carrier bytes changed")
		}
		r := event.Record.SourceRef
		coordinates, _ := json.Marshal([]any{r.QueryScopeID, r.PayloadRef, old.IndexPath, r.QueryWindowStartTs, r.QueryWindowEndTs,
			r.QueryTargetPID, r.QueryTargetThread, r.QueryTargetScope, r.QueryLineRangeKnown, r.QueryLineStart, r.QueryLineEnd, old.Line, old.Branch, old.Segment})
		for _, endpoint := range []struct{ role, got string }{{"from", rows[i].FromIdentity}, {"to", rows[i].ToIdentity}} {
			want := fmt.Sprintf("runtime_instance_%x", sha256.Sum256(append(append([]byte(nil), coordinates...), []byte("\x00"+endpoint.role+"\x00"+string(legacyData))...)))
			if endpoint.got != want {
				t.Fatalf("event %d %s credential changed", i, endpoint.role)
			}
		}
	}
}
