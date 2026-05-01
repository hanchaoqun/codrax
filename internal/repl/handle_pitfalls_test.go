package repl

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeTaxonomy is a minimal FailureTaxonomyReader for the
// /pitfalls handler tests. Tracks whether Clear was called so
// the writeEnabled-gate test can confirm refusal didn't
// mutate state.
type fakeTaxonomy struct {
	patterns   []types.FailurePattern
	clearCount int
}

func (f *fakeTaxonomy) All() []types.FailurePattern { return f.patterns }
func (f *fakeTaxonomy) Clear() error {
	f.clearCount++
	f.patterns = nil
	return nil
}

// TestHandlePitfallsCmd_ClearGatedOnWriteEnabled pins commit
// 39's P1 fix: a read-mode REPL session cannot blow away the
// per-repo learned-pitfall cache. /pitfalls list and show
// stay read-only and skip the gate.
func TestHandlePitfallsCmd_ClearGatedOnWriteEnabled(t *testing.T) {
	tx := &fakeTaxonomy{
		patterns: []types.FailurePattern{{
			ID: "fp-1", Name: "ok pattern", Description: strings.Repeat("a", 50),
			Trigger: strings.Repeat("b", 20), Confidence: 0.7, LastSeen: time.Now(),
		}},
	}
	r, out := newScriptedREPL(t, NewPlanStore(t.TempDir()))
	r.failureTaxonomy = tx
	r.writeEnabled = false

	r.handlePitfallsCmd("/pitfalls clear")

	if tx.clearCount != 0 {
		t.Errorf("read-mode /pitfalls clear must NOT call Clear; got %d calls", tx.clearCount)
	}
	got := out.String()
	if !strings.Contains(got, "/pitfalls clear") {
		t.Errorf("expected the gated command name in disabled message; got %q", got)
	}
	if !strings.Contains(got, "write") {
		t.Errorf("expected 'write' mentioned in disabled message; got %q", got)
	}
}

// TestHandlePitfallsCmd_ListReadableInReadMode pins the
// negative case: list/show stay read-only and work in read
// mode (no writeEnabled gate on the inspection paths).
func TestHandlePitfallsCmd_ListReadableInReadMode(t *testing.T) {
	tx := &fakeTaxonomy{
		patterns: []types.FailurePattern{{
			ID: "fp-readable", Name: "ok pattern visible", Description: strings.Repeat("a", 50),
			Trigger: strings.Repeat("b", 20), Confidence: 0.7, LastSeen: time.Now(),
		}},
	}
	r, out := newScriptedREPL(t, NewPlanStore(t.TempDir()))
	r.failureTaxonomy = tx
	r.writeEnabled = false

	r.handlePitfallsCmd("/pitfalls list")
	got := out.String()
	if !strings.Contains(got, "fp-readable") {
		t.Errorf("expected list to render the pattern id; got %q", got)
	}
}

// TestHandlePitfallsCmd_ClearWorksInWriteMode pins the
// happy path: writeEnabled=true → Clear actually fires.
func TestHandlePitfallsCmd_ClearWorksInWriteMode(t *testing.T) {
	tx := &fakeTaxonomy{
		patterns: []types.FailurePattern{{
			ID: "fp-x", Name: "ok pattern visible", Description: strings.Repeat("a", 50),
			Trigger: strings.Repeat("b", 20), Confidence: 0.7, LastSeen: time.Now(),
		}},
	}
	r, _ := newScriptedREPL(t, NewPlanStore(t.TempDir()))
	r.failureTaxonomy = tx
	r.writeEnabled = true

	r.handlePitfallsCmd("/pitfalls clear")
	if tx.clearCount != 1 {
		t.Errorf("write-mode clear should call Clear exactly once; got %d", tx.clearCount)
	}
}

// TestFailureTaxonomyBannerLine pins commit 41 UX#4 — the
// boot banner surfaces the count + /pitfalls list path so
// the feature is discoverable without reading /help.
func TestFailureTaxonomyBannerLine(t *testing.T) {
	en := failureTaxonomyBannerLine("en", 3)
	if !strings.Contains(en, "3 learned pitfall") {
		t.Errorf("EN line missing count; got %q", en)
	}
	if !strings.Contains(en, "/pitfalls list") {
		t.Errorf("EN line missing /pitfalls list path; got %q", en)
	}
	zh := failureTaxonomyBannerLine("zh", 3)
	if !strings.Contains(zh, "3") {
		t.Errorf("ZH line missing count; got %q", zh)
	}
	if !strings.Contains(zh, "/pitfalls list") {
		t.Errorf("ZH line missing /pitfalls list path; got %q", zh)
	}
}

// TestHandlePitfallsCmd_DisabledWhenStoreNil pins the
// feature-disabled branch: nil store → friendly message,
// no panic.
func TestHandlePitfallsCmd_DisabledWhenStoreNil(t *testing.T) {
	r, out := newScriptedREPL(t, NewPlanStore(t.TempDir()))
	r.failureTaxonomy = nil

	r.handlePitfallsCmd("/pitfalls list")
	got := out.String()
	if !strings.Contains(got, "disabled") {
		t.Errorf("expected disabled message; got %q", got)
	}
}
