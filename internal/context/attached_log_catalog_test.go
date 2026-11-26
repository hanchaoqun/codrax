package context

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedLogContextCarriesSourceNavigationNotPreviewAuthority(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "first", Data: []byte(strings.Repeat("head\n", 100) + "10-09 12:34:56.123 41 42 E Tag: tail\n")}, {Path: "/nonexistent/prepared-log-input"}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{AttachedLog: catalog.Preview(32), AttachedLogCatalog: catalog}
	withTool := formatAttachedLogContext(ac, map[string]bool{"log_query": true}, false)
	for _, want := range []string{"log_query", "bounded preview", "physical evidence coordinates", "\"complete\":false", "clock", "Source metadata shown: 2/2"} {
		if !strings.Contains(withTool, want) {
			t.Errorf("missing %q in %s", want, withTool)
		}
	}
	if strings.Contains(withTool, "attached_log.txt") || strings.Contains(withTool, "tail") {
		t.Fatal("preview advertised as full original")
	}
	withoutTool := formatAttachedLogContext(ac, map[string]bool{}, false)
	if !strings.Contains(withoutTool, "no log query tool") || strings.Contains(withoutTool, "Use log_query") {
		t.Fatal("unavailable tool instruction")
	}
}

func TestPreparedFrameFactIsVisibleWithItsOriginalRecordReference(t *testing.T) {
	const row = `{"record_kind":"frame","side":"target","metric_kind":"jank","count":1,"thread_id":42,"start_ns":0,"end_ns":1000,"observation_id":"o","source_id":"s","frame_metrics":{"duration_ns":1000}}`
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "saved fact", Data: []byte(row)}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{
		InputPath: "../../eval/fixtures/hmosperf_rendering_candidates/events.systrace", RuntimeAnchor: t.TempDir(), PreviewBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := catalog.PreparedFrameFact()
	if !ok {
		t.Fatal("missing prepared frame fact")
	}
	prompt := formatAttachedLogContext(&types.AgentContext{AttachedLog: catalog.Preview(12), AttachedLogCatalog: catalog, AttachedTraceMaterial: material}, nil, false)
	if !strings.Contains(prompt, row) || !strings.Contains(prompt, fact.RecordRef) {
		t.Fatal("complete attached fact and physical record reference must survive a clipped log preview")
	}
}
