package tool

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC265CatalogScopeRefusalIsNotFileAccessFailure(t *testing.T) {
	bus, _ := catalogPublicFixture(t)
	outside := t.TempDir()
	for _, tc := range []struct {
		name, root   string
		scopeRefused bool
	}{
		{"outside", outside, true},
		{"missing", filepath.Join(bus.RepoRoot, "missing"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args, _ := json.Marshal(map[string]any{"root": tc.root})
			out, err := (&TraceCatalog{}).Execute(bus, args)
			if err != nil || out.Success {
				t.Fatalf("expected diagnostic: %v %+v", err, out)
			}
			got := out.Repair != nil && out.Repair.Code == "capture_directory_scope_refused"
			if got != tc.scopeRefused {
				t.Fatalf("scope disposition got=%v want=%v: %+v", got, tc.scopeRefused, out)
			}
			if got {
				carrier, ok := types.ToolHandoffCarrierFromToolResult(out)
				if !ok || carrier.RepairCode != "capture_directory_scope_refused" {
					t.Fatal("precise disposition lost across handoff")
				}
			}
			if len(bus.Mutable.TraceCatalogs()) != 0 || len(out.Observations) != 0 {
				t.Fatal("refusal minted discovery or observations")
			}
		})
	}
	bus.UserPinnedFiles = []string{outside}
	args, _ := json.Marshal(map[string]any{"root": outside})
	out, err := (&TraceCatalog{}).Execute(bus, args)
	if err != nil || !out.Success || out.Repair != nil {
		t.Fatalf("authorized empty directory lost: %v %+v", err, out)
	}
}
