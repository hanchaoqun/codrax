package context

import (
	"context"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPreparedLogContextCarriesSourceNavigationNotPreviewAuthority(t *testing.T) {
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "first", Data: []byte(strings.Repeat("head\n", 100) + "10-09 12:34:56.123 41 42 E Tag: tail\n")}, {Path: "/nonexistent/prepared-log-input"}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{AttachedLog: catalog.Preview(32), AttachedLogCatalog: catalog}
	withTool := formatAttachedLogContext(ac, map[string]bool{"log_query": true})
	for _, want := range []string{"log_query", "bounded preview", "physical evidence coordinates", "\"complete\":false", "clock", "Source metadata shown: 2/2"} {
		if !strings.Contains(withTool, want) {
			t.Errorf("missing %q in %s", want, withTool)
		}
	}
	if strings.Contains(withTool, "attached_log.txt") || strings.Contains(withTool, "tail") {
		t.Fatal("preview advertised as full original")
	}
	withoutTool := formatAttachedLogContext(ac, map[string]bool{})
	if !strings.Contains(withoutTool, "no log query tool") || strings.Contains(withoutTool, "Use log_query") {
		t.Fatal("unavailable tool instruction")
	}
}
