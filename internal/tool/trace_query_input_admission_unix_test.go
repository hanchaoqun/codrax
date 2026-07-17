//go:build !windows

package tool

import (
	"encoding/json"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryFIFOIsActionRequiredSourceAdmissionFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "customer.trace")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	assertTraceQuerySourceUnavailableForTest(t, path)
}

func TestTraceQueryDeviceIsActionRequiredSourceAdmissionFailure(t *testing.T) {
	assertTraceQuerySourceUnavailableForTest(t, "/dev/null")
}

func assertTraceQuerySourceUnavailableForTest(t *testing.T, path string) {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search"})
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: t.TempDir()}, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.Success || result.Repair == nil || result.Repair.Code != tracequery.TraceInputAdmissionCodeSourceUnavailable {
		t.Fatalf("non-regular source did not produce typed source admission repair: %+v", result)
	}
	if result.Repair.Metadata["stage"] != types.ToolRepairStageTraceInputAdmission ||
		result.Repair.Metadata["status"] != types.ToolRepairStatusActionRequired {
		t.Fatalf("non-regular source repair lacks terminal family metadata: %+v", result.Repair.Metadata)
	}
}
