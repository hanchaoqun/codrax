package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHMC264ZIPRoutingInventoryBoundsKeepExactNamesAndTotal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		shown int
	}{
		{"entry_count", func() []string {
			var names []string
			for i := 0; i < 70; i++ {
				names = append(names, fmt.Sprintf("成员-%03d.data", i))
			}
			return names
		}(), 64},
		{"long_name", []string{strings.Repeat("长", 6000) + ".data", "exact.data"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "资料.zip")
			var members []traceArchiveTestMember
			for _, name := range tc.names {
				members = append(members, traceArchiveTestMember{name: name, body: []byte("PERFILE2")})
			}
			traceArchiveTestZIP(t, input, members...)
			got := ProbeInputRoutingCapability(context.Background(), input, t.TempDir())
			inventory := got.ArchiveCandidates
			if inventory == nil || inventory.CandidateCount != len(tc.names) || inventory.MembersComplete || !inventory.SelectionRequired || len(inventory.Members) != tc.shown {
				t.Fatalf("preview became full inventory: %+v", inventory)
			}
			nameBytes := 0
			for _, name := range inventory.Members {
				found := false
				for _, original := range tc.names {
					found = found || name == original
				}
				if !found {
					t.Fatalf("member name truncated/rewritten: %q", name)
				}
				nameBytes += len(name)
			}
			if nameBytes > inputRoutingZIPMaxMemberNamesBytes {
				t.Fatal("name budget exceeded")
			}
		})
	}
}

type hmc264RoutingContext struct {
	context.Context
	checks, at int
	action     func()
	cancelled  bool
}

func (c *hmc264RoutingContext) Err() error {
	c.checks++
	if c.at > 0 && c.checks == c.at {
		c.action()
	}
	if c.cancelled {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestHMC264ZIPRoutingGenerationAndCancellationClearAllNavigation(t *testing.T) {
	for _, kind := range []string{"same_bytes_new_generation", "cancel_after_discovery"} {
		t.Run(kind, func(t *testing.T) {
			input := filepath.Join(t.TempDir(), "capture.zip")
			body := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.data", body: traceArchiveTestBuiltinBody()})
			baseline := &hmc264RoutingContext{Context: context.Background()}
			if got := ProbeInputRoutingCapability(baseline, input, t.TempDir()); got.ArchiveCandidates == nil {
				t.Fatalf("control probe: %+v", got)
			}
			// The penultimate checkpoint is after candidate classification and
			// before final held-generation validation; do not rely on a fixed count.
			ctx := &hmc264RoutingContext{Context: context.Background(), at: baseline.checks - 1}
			ctx.action = func() {
				if kind == "cancel_after_discovery" {
					ctx.cancelled = true
					return
				}
				replacement := input + ".replacement"
				if err := os.WriteFile(replacement, body, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, input); err != nil {
					t.Fatal(err)
				}
			}
			got := ProbeInputRoutingCapability(ctx, input, t.TempDir())
			if got.NativeReader != "" || got.ArchiveCandidates != nil || len(got.CandidateViews) > 0 || got.Status != "unknown" {
				t.Fatalf("stale/cancelled candidates retained: %+v", got)
			}
		})
	}
}
