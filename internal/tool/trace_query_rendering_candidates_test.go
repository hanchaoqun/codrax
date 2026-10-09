package tool

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderingCandidatesSQLiteDefaultPreparation(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 required for native preparation")
	}
	path := filepath.Join(t.TempDir(), "capture.data")
	cmd := exec.Command(sqlite, path)
	cmd.Stdin = strings.NewReader(`
CREATE TABLE trace_range (start_ts INT, end_ts INT);
INSERT INTO trace_range VALUES (1000000000,1100000000);
CREATE TABLE process (ipid INT,pid INT,name TEXT);
INSERT INTO process VALUES (1,600,'mixed-app'),(2,900,'outside');
CREATE TABLE thread (itid INT,tid INT,ipid INT,name TEXT,start_ts INT,is_main_thread INT,switch_count INT);
INSERT INTO thread VALUES (1,601,1,'mixed-ui',1000000000,0,1),(2,602,1,'VizCompositorTh',1000000000,0,1),(3,901,2,'outside-ui',1000000000,0,1);
CREATE TABLE thread_state (itid INT,ts INT,dur INT,cpu INT,state TEXT);
INSERT INTO thread_state VALUES (1,1000000000,100000000,0,'Running'),(2,1000000000,100000000,1,'Running'),(3,1000000000,100000000,2,'Running');
CREATE TABLE sched_slice (ts,dur,cpu,itid,end_state,priority);
CREATE TABLE callstack (id,ts,dur,itid,callid,name,flag,cookie,chainId,depth);
INSERT INTO callstack VALUES (1,1001000000,1000000,1,NULL,'flutter::PlatformConfiguration::BeginFrame','',NULL,NULL,0),(2,1003000000,1000000,2,NULL,'Graphics.Pipeline::IssueBeginFrame','',NULL,NULL,0),(3,1050000000,1000000,3,NULL,'RNViewBase::OnLayout','',NULL,NULL,0);
CREATE TABLE instant (ts,name,ref,wakeup_from,ref_type);
CREATE TABLE syscall (ts,dur,syscall_number,itid);
CREATE TABLE frame_slice (id,type,ts,itid);
CREATE TABLE native_hook (id,start_ts,end_ts,event_type,all_heap_size,itid,ipid);
`)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create native fixture: %v %s", err, out)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "rendering_candidates", "time_start": 1, "time_end": 1.05})
	if !r.Success {
		t.Fatalf("native preparation: %+v", r)
	}
	frameworks := map[string]bool{}
	for _, record := range r.Observations {
		p, ok := DecodeTraceRenderingCandidates(record)
		if !ok {
			continue
		}
		for _, c := range p.Candidates {
			if c.OwnerScope != "process" || c.OwnerID != 600 {
				t.Fatalf("lost native owner or included right boundary: %+v", c)
			}
			frameworks[c.Framework] = true
			for _, signal := range c.Signals {
				for _, e := range signal.Examples {
					if e.TGID != 600 || e.TID != 601 && e.TID != 602 || e.SourceLine <= 0 {
						t.Fatalf("lost native identity/reference: %+v", e)
					}
				}
			}
		}
	}
	if !frameworks["HARMONY_FLUTTER"] || !frameworks["HARMONY_WEB_PIPELINE"] || len(frameworks) != 2 {
		t.Fatalf("native rendering candidates missing: %v\n%s", frameworks, r.Summary)
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("source database mutated")
	}
}

func renderingCandidateQuery(t *testing.T) (types.ToolResult, types.ObservationRecord, tracequery.RenderingCandidatesResult) {
	t.Helper()
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "rendering_candidates", "time_start": 1, "time_end": 1.05})
	r, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir()}, params)
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %+v", err, r)
	}
	for _, record := range r.Observations {
		if record.Predicate == TraceRenderingCandidatesPredicate {
			p, ok := DecodeTraceRenderingCandidates(record)
			if !ok {
				t.Fatalf("query emitted invalid handoff: %+v", record)
			}
			return r, record, p
		}
	}
	t.Fatal("missing advisory handoff")
	return r, types.ObservationRecord{}, tracequery.RenderingCandidatesResult{}
}

func TestRenderingCandidatesPublicToolHandoff(t *testing.T) {
	_, r, p := renderingCandidateQuery(t)
	if r.GroundingPolicy != types.ClaimGroundingSoft || r.Role != types.AnswerAggregateRoleSupportingCoverage {
		t.Fatal("advisory signatures acquired hard authority")
	}
	found := map[int]map[string]bool{}
	for _, c := range p.Candidates {
		if c.OwnerScope != "process" {
			t.Fatalf("lost native TGID: %+v", c)
		}
		if found[c.OwnerID] == nil {
			found[c.OwnerID] = map[string]bool{}
		}
		found[c.OwnerID][c.Framework] = true
		if c.OwnerID == 700 && c.DefinitionStatus != "unsupported_definition" {
			t.Fatal("index-only game definition presented as supported pipeline")
		}
	}
	for owner, framework := range map[int]string{100: "HARMONY_ARKUI", 200: "HARMONY_FLUTTER", 300: "HARMONY_WEB_PIPELINE", 400: "HARMONY_RN", 500: "HARMONY_KMP", 700: "HARMONY_GAME_ENGINE"} {
		if !found[owner][framework] {
			t.Errorf("missing %d/%s: %+v", owner, framework, p)
		}
	}
	if !found[600]["HARMONY_FLUTTER"] || !found[600]["HARMONY_WEB_PIPELINE"] || len(found[800]) != 0 || len(found[900]) != 0 {
		t.Fatalf("mixed, generic or boundary leak: %+v", found)
	}
	text := TraceRenderingCandidatesText(p, 32)
	for _, want := range []string{"多个候选可共存", "不证明未使用", "不表示卡顿严重程度或根因排名"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing guidance %q", want)
		}
	}
	for name, mutate := range map[string]func(*types.ObservationRecord){
		"hard":   func(r *types.ObservationRecord) { r.GroundingPolicy = types.ClaimGroundingHard },
		"source": func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		"window": func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 2 },
		"target": func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 100 },
		"duplicate": func(r *types.ObservationRecord) {
			r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := r
			mutate(&bad)
			if _, ok := DecodeTraceRenderingCandidates(bad); ok {
				t.Fatal("invalid display binding accepted")
			}
		})
	}
}

func TestRenderingCandidatesPublicAliasHandoff(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_rendering_candidates/events.systrace")
	for _, selector := range []map[string]any{{"thread": "1.ui-201"}, {"pid": 200, "target_scope": "process"}, {"thread": "1.ui"}} {
		params := map[string]any{"source": "path", "path": path, "view": "rendering_candidates", "time_start": 1, "time_end": 1.05}
		for k, v := range selector {
			params[k] = v
		}
		raw, _ := json.Marshal(params)
		r, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir()}, raw)
		if err != nil || !r.Success {
			t.Fatalf("query %v: %v %+v", selector, err, r)
		}
		found := false
		for _, record := range r.Observations {
			if record.Predicate == TraceRenderingCandidatesPredicate {
				p, ok := DecodeTraceRenderingCandidates(record)
				if !ok || p.TotalCandidates == 0 {
					t.Fatalf("alias %v lost typed handoff: %+v", selector, record)
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("alias %v did not publish typed candidates", selector)
		}
	}
}
