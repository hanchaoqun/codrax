package tool

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These tests enter default SQLite preparation, not a fabricated query index.
// Deliberately absent Running rows exercise the converter's CPU-unknown lane.
func transactionSQLitePublicFixture(t *testing.T, changes string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "capture.data")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE trace_range (start_ts INT, end_ts INT);
INSERT INTO trace_range VALUES (0,2000000000);
CREATE TABLE process (ipid INT,pid INT,name TEXT);
INSERT INTO process VALUES (1,100,'app'),(2,200,'render_service');
CREATE TABLE thread (itid INT,tid INT,ipid INT,name TEXT,start_ts INT,is_main_thread INT,switch_count INT);
INSERT INTO thread VALUES (1,101,1,'app-main',0,0,1),(2,201,2,'rs-main',0,0,1);
CREATE TABLE thread_state (itid INT,ts INT,dur INT,cpu INT,state TEXT);
CREATE TABLE sched_slice (ts,dur,cpu,itid,end_state,priority);
CREATE TABLE callstack (id,ts,dur,itid,callid,name,flag,cookie,chainId,depth);
INSERT INTO callstack VALUES
 (1,1001000000,1000000,1,NULL,'H:MarshRSTransactionData cmdCount: 1, transactionFlag:[101,7]','',NULL,NULL,0),
 (2,1020000000,1000000,2,NULL,'H:RSMainThread::ProcessCommandUni [101,7]','',NULL,NULL,0);
CREATE TABLE instant (ts,name,ref,wakeup_from,ref_type);
CREATE TABLE syscall (ts,dur,syscall_number,itid);
CREATE TABLE frame_slice (id,type,ts,itid);
CREATE TABLE native_hook (id,start_ts,end_ts,event_type,all_heap_size,itid,ipid);
` + changes)
	if err != nil {
		t.Fatalf("create native fixture: %v", err)
	}
	return path
}

func transactionSQLitePublicQuery(t *testing.T, path string, start, end float64) (*types.BusContext, types.ToolResult, types.ObservationRecord, tracequery.TransactionHandoffsResult) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	if !r.Success {
		t.Fatalf("default SQLite transaction query failed: %s", r.Summary)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("query mutated original SQLite bytes")
	}
	for _, record := range r.Observations {
		if p, ok := DecodeTraceTransactionHandoffs(record); ok {
			if p.Status != "available" || p.Window.StartTs != start || p.Window.EndTs != end || p.Window.EndInclusive {
				t.Fatalf("wrong native availability/window: %+v", p)
			}
			return ctx, r, record, p
		}
	}
	t.Fatalf("native transaction handoff missing: %s", r.Summary)
	return nil, types.ToolResult{}, types.ObservationRecord{}, tracequery.TransactionHandoffsResult{}
}

func transactionSQLiteOnlyHandoff(t *testing.T, p tracequery.TransactionHandoffsResult) tracequery.TransactionHandoff {
	t.Helper()
	if p.TotalKeys != 1 || len(p.Handoffs) != 1 || p.OmittedKeys != 0 || p.Handoffs[0].TID != 101 || p.Handoffs[0].Sequence != "7" {
		t.Fatalf("unexpected transaction inventory: %+v", p)
	}
	return p.Handoffs[0]
}

func TestTransactionHandoffsSQLitePublicUnknownCPUAndSourceBinding(t *testing.T) {
	path := transactionSQLitePublicFixture(t, "")
	ctx, result, record, p := transactionSQLitePublicQuery(t, path, 1, 1.05)
	payload := hmc17NamedPayload(t, result)
	h := transactionSQLiteOnlyHandoff(t, p)
	if h.Status != "observed_unique_protocol_match" || h.SubmissionCount != 1 || h.ConsumptionCount != 1 || p.WindowSubmissionEvents != 1 || p.WindowConsumptionEvents != 1 {
		t.Fatalf("CPU absence erased valid protocol observations: %+v", p)
	}
	for _, e := range []tracequery.TransactionEndpoint{h.Submissions[0], h.Consumptions[0]} {
		// Native preparation publishes a one-child tracebundle. The query
		// identity is the bundle; endpoint coordinates belong to its physical
		// child, never to the manifest's JSON line numbers.
		mapped := false
		for _, source := range payload.TraceArtifacts {
			mapped = mapped || source.SourcePath == e.SourcePath
		}
		if !mapped || e.SourceLine <= 0 || e.Line <= 0 || !e.InWindow || e.TGID <= 0 {
			t.Fatalf("lost physical source/owner coordinate: %+v sources=%+v", e, payload.TraceArtifacts)
		}
	}
	raw := hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "H:", "time_start": 1, "time_end": 1.05}))
	seen := 0
	for _, e := range raw.Events {
		if e.SpanAction != "B" || !strings.HasPrefix(e.SpanName, "H:") {
			continue
		}
		seen++
		co := tracequery.ProjectTraceEventInventoryCoordinates(e.Event)
		if co.CPUKnown || co.CPU != -1 || !co.EmitterTIDKnown || !co.EmitterTGIDKnown {
			t.Fatalf("CPU-unknown native observation gained execution location: %+v %+v", e, co)
		}
	}
	if seen != 2 {
		t.Fatalf("missing native CPU-unknown control endpoints: %+v", raw.Events)
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"other source": func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"other window": func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 2 },
		"other owner":  func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 999 },
		"model copy":   func(r *types.ObservationRecord) { r.Producer = "model" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := record
			mutate(&bad)
			if _, ok := DecodeTraceTransactionHandoffs(bad); ok {
				t.Fatal("accepted rebound display receipt")
			}
		})
	}
}

func TestTransactionHandoffsSQLitePublicWindowDoesNotDecideUniqueness(t *testing.T) {
	for name, ts := range map[string]string{"before window": "900000000", "after window": "1100000000"} {
		t.Run(name, func(t *testing.T) {
			path := transactionSQLitePublicFixture(t, `INSERT INTO callstack VALUES (3,`+ts+`,1000000,1,NULL,'H:MarshRSTransactionData transactionFlag:[101,7]','',NULL,NULL,0);`)
			_, _, _, p := transactionSQLitePublicQuery(t, path, 1, 1.05)
			h := transactionSQLiteOnlyHandoff(t, p)
			if h.Status != "ambiguous" || h.SubmissionCount != 2 || h.ConsumptionCount != 1 || h.WindowSubmissions != 1 || h.WindowConsumptions != 1 || p.WindowSubmissionEvents != 1 {
				t.Fatalf("out-of-window competing submission erased before matching: %+v", p)
			}
			outside := 0
			for _, e := range h.Submissions {
				if !e.InWindow {
					outside++
				}
			}
			if outside != 1 {
				t.Fatalf("outside duplicate lost: %+v", h)
			}
		})
	}
}

func TestTransactionHandoffsSQLitePublicRightBoundaryAndOutsidePeer(t *testing.T) {
	path := transactionSQLitePublicFixture(t, `UPDATE callstack SET ts=1050000000 WHERE id=2;`)
	_, _, _, p := transactionSQLitePublicQuery(t, path, 1, 1.05)
	h := transactionSQLiteOnlyHandoff(t, p)
	if h.Status != "observed_unique_protocol_match" || h.WindowSubmissions != 1 || h.WindowConsumptions != 0 || p.WindowConsumptionEvents != 0 || h.Consumptions[0].InWindow || h.Consumptions[0].Ts != 1.05 {
		t.Fatalf("right-boundary peer counted inside selected window: %+v", p)
	}
	_, _, _, later := transactionSQLitePublicQuery(t, path, 1.05, 1.06)
	peer := transactionSQLiteOnlyHandoff(t, later)
	if peer.WindowSubmissions != 0 || peer.WindowConsumptions != 1 || peer.Submissions[0].InWindow || !peer.Consumptions[0].InWindow {
		t.Fatalf("window-local event accounting lost outside predecessor: %+v", later)
	}
}

func TestTransactionHandoffsSQLitePublicRejectedRowsLimitUniverse(t *testing.T) {
	path := transactionSQLitePublicFixture(t, `INSERT INTO callstack VALUES (3,1010000000,1000000,99,NULL,'H:MarshRSTransactionData transactionFlag:[101,7]','',NULL,NULL,0);`)
	_, r, _, p := transactionSQLitePublicQuery(t, path, 1, 1.05)
	h := transactionSQLiteOnlyHandoff(t, p)
	if h.Status != "observed_unique_protocol_match" || h.SubmissionCount != 1 {
		t.Fatalf("fixture must exercise upstream rejection, not fabricated duplicate: %+v", p)
	}
	// Exact fixture fact: the original database has two matching submissions;
	// only one valid-owner endpoint survives preparation. The matching universe
	// must be stated as published observations, never as original-DB uniqueness.
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM callstack WHERE name LIKE 'H:MarshRSTransactionData%'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("invalid native rejection control count=%d: %v", count, err)
	}
	if !strings.Contains(strings.Join(p.Caveats, " "), "Unpublished/rejected source rows remain outside this observation universe") || !strings.Contains(TraceTransactionHandoffsText(p), "转换时被拒绝或未发布的行不在此总体内") || !strings.Contains(r.Summary, "协议") {
		t.Fatalf("published-source qualifier missing: %+v\n%s", p, r.Summary)
	}
	encoded, err := json.Marshal(p)
	if err != nil || bytes.Contains(encoded, []byte(`"status":"unique"`)) || bytes.Contains(encoded, []byte(`"causality":"proven"`)) {
		t.Fatalf("observation widened into original-source identity/causal proof: %s %v", encoded, err)
	}
}

func TestTransactionHandoffsSQLitePublicNamespacePreservesHostOwner(t *testing.T) {
	path := transactionSQLitePublicFixture(t, `
INSERT INTO process VALUES (3,1000,'host-app');
INSERT INTO thread VALUES (3,101,3,'host-app-main',0,0,1);
INSERT INTO thread_state VALUES (3,1000000000,100000000,0,'Running');
`)
	ctx, _, _, p := transactionSQLitePublicQuery(t, path, 1, 1.05)
	h := transactionSQLiteOnlyHandoff(t, p)
	if h.Status != "observed_unique_protocol_match" || h.Submissions[0].TID != 101 || h.Submissions[0].TGID != 1000 {
		t.Fatalf("namespace alias lost exact host owner or was rejected: %+v", p)
	}
	raw := hmc17NamedPayload(t, hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "H:MarshRSTransactionData", "time_start": 1, "time_end": 1.05}))
	if len(raw.Events) != 1 || raw.Events[0].TGID != 1000 || raw.Events[0].SpanPID != 100 || raw.Events[0].PID != 101 {
		t.Fatalf("native host/payload namespace coordinates were collapsed: %+v", raw.Events)
	}
}
