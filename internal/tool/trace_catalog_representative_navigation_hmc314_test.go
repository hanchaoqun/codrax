package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC314PublicRepresentativePagesKeepExactExecution(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	body, err := os.ReadFile(filepath.Join(root, "capture.data"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"three.data", "four.data", "five.data", "six.data", "seven.data"} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	discovery, err := (&TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"resource_stack","pid":101,"time_start":10,"time_end":10.05},{"view":"resource_stack","pid":999,"time_start":10,"time_end":10.05}],"representatives":{"per_cluster":1}}`))
	if err != nil || !discovery.Success {
		t.Fatalf("discover: %v %s", err, discovery.Summary)
	}
	c := bus.Mutable.TraceCatalogs()[0]
	for i := 0; i < 3; i++ {
		if !catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101).Success {
			t.Fatal("native success prerequisite")
		}
	}
	if !catalogPublicQuery(t, bus, filepath.Join(root, "nested/capture.data"), 999).Success {
		t.Fatal("native empty prerequisite")
	}
	for i := 0; i < 2; i++ {
		if catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101).Success {
			t.Fatal("native failure prerequisite")
		}
	}
	before, _ := json.Marshal(c.CurrentExecution())
	request, _ := json.Marshal(map[string]any{"action": "representative_status", "catalog_id": c.ID()})
	seen, pages := map[string]bool{}, 0
	for len(request) > 0 {
		if err := toolparam.Validate(request, (&TraceCatalog{}).Parameters()); err != nil {
			t.Fatal(err)
		}
		out, err := (&TraceCatalog{}).Execute(bus, request)
		if err != nil || !out.Success || len(out.Summary) > 64<<10 {
			t.Fatalf("page: %v %s", err, out.Summary)
		}
		if len(out.Observations) != 0 || out.ReadCoverage != nil || out.TraceQuerySourceRead.Path() != "" {
			t.Fatal("navigation minted measurement/source permission")
		}
		var page struct {
			Index   string `json:"saved_index"`
			Records []struct {
				Kind   string                            `json:"record_kind"`
				Member tracecatalog.RepresentativeMember `json:"member"`
				tracecatalog.RepresentativeQueryNavigation
			} `json:"records"`
			Next      json.RawMessage            `json:"next_call"`
			Calls     map[string]json.RawMessage `json:"query_calls"`
			Displayed int                        `json:"displayed_records"`
			Omitted   int                        `json:"omitted_records"`
			Offset    int                        `json:"offset"`
			Total     int                        `json:"total_records"`
		}
		if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Records) > 16 || page.Displayed != len(page.Records) || page.Offset+page.Displayed+page.Omitted != page.Total {
			t.Fatal("whole-record pagination census changed")
		}
		index, err := tracecatalog.Load(context.Background(), page.Index)
		if err != nil {
			t.Fatal(err)
		}
		queries := map[string]tracecatalog.QueryRecord{}
		for _, q := range index.Queries {
			queries[q.ID] = q
		}
		for _, r := range page.Records {
			if r.Kind != "cluster_member" {
				continue
			}
			m := r.Member
			q, ok := queries[m.QueryID]
			if !ok || seen[m.QueryID] || m.ArtifactID != q.ArtifactID || m.ArtifactRevision != q.ArtifactRevision || m.Outcome != q.Outcome || m.Attempts != len(q.Attempts) {
				t.Fatal("member differs from same-return index")
			}
			seen[m.QueryID] = true
			distribution := map[tracecatalog.Outcome]int{}
			for _, a := range q.Attempts {
				distribution[a.Outcome]++
			}
			if !reflect.DeepEqual(distribution, r.AttemptOutcomes) || !reflect.DeepEqual(r.Specification.Window, q.Window) || r.Specification.Object != q.Object || r.Specification.View != q.View || r.Origin != tracecatalog.QueryOriginDeclared {
				t.Fatalf("query/attempt provenance lost: %+v", r)
			}
		}
		for id, call := range page.Calls {
			if !seen[id] {
				t.Fatal("hidden/foreign query call")
			}
			if err := toolparam.Validate(call, (&TraceQuery{}).Parameters()); err != nil {
				t.Fatal(err)
			}
			foreign := bus.ShallowClone()
			foreign.Mutable = types.NewMutableState("new run")
			if rejected, _ := (&TraceQuery{}).Execute(foreign, call); rejected.Success {
				t.Fatal("JSON navigation restored live execution permission")
			}
		}
		pages++
		request = page.Next
	}
	if pages != 2 || len(seen) != 16 {
		t.Fatalf("expected whole membership across pages: pages=%d members=%d", pages, len(seen))
	}
	after, _ := json.Marshal(c.CurrentExecution())
	if string(before) != string(after) {
		t.Fatal("status reselected or rewrote original query/attempt history")
	}
}

func TestHMC314RepresentativePageByteBudget(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	pattern := strings.Repeat("literal", 2000)
	request, _ := json.Marshal(map[string]any{"queries": []map[string]any{{"view": "event_search", "pid": 101, "pattern": pattern}, {"view": "event_search", "pid": 202, "pattern": pattern}, {"view": "event_search", "pid": 303, "pattern": pattern}}, "representatives": map[string]any{"per_cluster": 1}})
	seen, pages := map[string]bool{}, 0
	for len(request) > 0 {
		if err := toolparam.Validate(request, (&TraceCatalog{}).Parameters()); err != nil {
			t.Fatal(err)
		}
		out, err := (&TraceCatalog{}).Execute(bus, request)
		if err != nil || !out.Success || len(out.Summary) > 64<<10 {
			t.Fatalf("original byte budget: %v %s", err, out.Summary)
		}
		var page struct {
			Records []struct {
				Member tracecatalog.RepresentativeMember `json:"member"`
				tracecatalog.RepresentativeQueryNavigation
			} `json:"records"`
			Next      json.RawMessage `json:"next_call"`
			Displayed int             `json:"displayed_records"`
			Omitted   int             `json:"omitted_records"`
			Offset    int             `json:"offset"`
			Total     int             `json:"total_records"`
		}
		if err := json.Unmarshal([]byte(out.Summary), &page); err != nil {
			t.Fatal(err)
		}
		if page.Total != 12 || page.Offset+page.Displayed+page.Omitted != 12 || page.Displayed != len(page.Records) {
			t.Fatal("partial page misreported whole record census")
		}
		if pages == 0 && (page.Omitted == 0 || page.Displayed >= 16) {
			t.Fatal("byte budget did not trigger independently of record cap")
		}
		for _, row := range page.Records {
			if row.Member.QueryID == "" {
				continue
			}
			var args map[string]any
			if json.Unmarshal(row.Specification.Parameters, &args) != nil || args["pattern"] != pattern || seen[row.Member.QueryID] || row.Member.Attempts != 0 || len(row.AttemptOutcomes) != 0 {
				t.Fatal("member specification was clipped, duplicated or promoted to executed")
			}
			seen[row.Member.QueryID] = true
		}
		pages++
		request = page.Next
	}
	if pages < 3 || len(seen) != 9 {
		t.Fatalf("full exact membership did not survive paging: %d/%d", pages, len(seen))
	}
}
