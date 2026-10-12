package types

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
)

func TestHMC224TraceAdmissionCatalogOneShotLifecycle(t *testing.T) {
	for _, mutation := range []string{"initialization", "changed_objective", "same_objective_new_request", "early_exit", "old_fork", "foreign_run"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			paths := [2]string{filepath.Join(root, "a"), filepath.Join(root, "b")}
			for _, path := range paths {
				if err := os.WriteFile(path, []byte("capture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			c, err := tracecatalog.DiscoverIndependentPair(context.Background(), paths)
			if err != nil {
				t.Fatal(err)
			}
			m := NewMutableState("compare")
			c, err = m.InstallTraceCatalog(c)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.StageTraceCatalogForInvestigation(c); err != nil {
				t.Fatal(err)
			}
			fork := m.ForkForExploreDispatch()
			switch mutation {
			case "changed_objective":
				m.SetObjective("other request")
			case "same_objective_new_request":
				m.SetObjective("compare")
			case "early_exit":
				m.ClearStagedTraceCatalogForInvestigation()
			case "foreign_run":
				m = NewMutableState("compare")
			}
			m.ResetTurnAArtifacts()
			want := mutation == "initialization" || mutation == "old_fork"
			if (len(m.TraceCatalogs()) == 1) != want {
				t.Fatal("staging crossed the request/initialization boundary")
			}
			if want && len(m.TraceCatalogs()[0].Snapshot().Queries) != 0 {
				t.Fatal("navigation minted query authority")
			}
			fork.ResetTurnAArtifacts()
			if len(fork.TraceCatalogs()) != 0 {
				t.Fatal("fork copied private pending admission")
			}
			m.ResetTurnAArtifacts()
			m.MergeExploreFork(fork)
			if len(m.TraceCatalogs()) != 0 {
				t.Fatal("second reset or old fork resurrected staged navigation")
			}
		})
	}
}
