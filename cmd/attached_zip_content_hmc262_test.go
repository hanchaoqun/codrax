package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC262CLIZIPNamedMemberUsesCompleteAttachment(t *testing.T) {
	for _, alias := range []string{"htrace", "atrace"} {
		t.Run(alias, func(t *testing.T) {
			resetPreparedTraceFlags(t)
			dir := t.TempDir()
			source := filepath.Join(dir, "采集 archive.capture")
			original := hmc262CLIZIP(t, map[string][]byte{"采集/capture.data": cliBinaryTraceTailFixture()})
			if err := os.WriteFile(source, original, 0400); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CODRAX_TRACE_STREAMER", filepath.Join(dir, "missing-converter"))
			calls := 0
			prepareAttachedTraceInput = func(ctx context.Context, opts traceinput.Options) (*attachment.TraceMaterial, error) {
				calls++
				opts.RuntimeAnchor, opts.RuntimeAnchorFallback = filepath.Join(dir, "runtime"), ""
				return traceinput.Prepare(ctx, opts)
			}
			maxAttachedTraceBytes = 1024
			if alias == "htrace" {
				flagAttachHitrace = []string{source}
			} else {
				flagAttachAtrace = []string{source}
			}
			loaded, err := loadPreparedAttachedTrace(t.Context(), nil)
			if err != nil || loaded.material == nil || calls != 1 {
				t.Fatalf("ZIP did not enter shared preparation: calls=%d err=%v", calls, err)
			}
			if loaded.material.SourcePath() != source || loaded.material.QueryPath() == source || len(loaded.body) > 1024 || strings.Contains(loaded.body, "tail-target") {
				t.Fatal("source/complete material/preview boundary lost")
			}
			bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, AttachedHitrace: loaded.body, AttachedTraceMaterial: loaded.material, Mutable: types.NewMutableState("query attachment")}
			projected := types.ToolBusContext(promptctx.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore), types.AgentExplorer)
			result, err := (&tool.TraceQuery{}).Execute(projected, json.RawMessage(`{"source":"attached_trace","view":"event_search","pattern":"tail-target","time_start":1.0105,"time_end":1.012,"limit":10}`))
			if err != nil || !result.Success || !strings.Contains(result.Summary, "tail-target") {
				t.Fatalf("full ZIP member tail unavailable: %+v %v", result, err)
			}
			if after, err := os.ReadFile(source); err != nil || !bytes.Equal(original, after) {
				t.Fatal("original ZIP changed")
			}
			// A second normal-name member is ambiguity, not a license to pick
			// one or reuse the earlier attachment. A failed loader publishes none.
			ambiguous := filepath.Join(dir, "ambiguous.zip")
			if err := os.WriteFile(ambiguous, hmc262CLIZIP(t, map[string][]byte{"a.data": cliBinaryTraceTailFixture(), "b.data": cliBinaryTraceTailFixture()}), 0400); err != nil {
				t.Fatal(err)
			}
			flagAttachHitrace, flagAttachAtrace = []string{ambiguous}, nil
			failed, err := loadPreparedAttachedTrace(t.Context(), nil)
			if err == nil || !strings.Contains(err.Error(), "multiple_trace_members") || failed.material != nil || failed.body != "" {
				t.Fatalf("ambiguity published an attachment: %+v %v", failed, err)
			}
		})
	}
}

func hmc262CLIZIP(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range members {
		member, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
