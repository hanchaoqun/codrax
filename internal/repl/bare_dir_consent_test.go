package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The consent decision is a pure function of three typed booleans.
// Full truth table pinned so a future tier can't silently change the
// prompt conditions.
func TestBareDirConsentNeededTruthTable(t *testing.T) {
	cases := []struct {
		autoInit, scaffold, empty bool
		want                      bool
	}{
		{false, false, false, true}, // init missing → ask
		{false, false, true, true},  // both missing, empty → ask
		{false, true, false, true},  // init missing → ask
		{false, true, true, true},   // init missing → ask
		{true, false, false, false}, // init pre-auth, non-empty → silent
		{true, false, true, true},   // empty + scaffold missing → ask
		{true, true, false, false},  // fully pre-auth → silent
		{true, true, true, false},   // fully pre-auth → silent
	}
	for _, tc := range cases {
		got := bareDirConsentNeeded(tc.autoInit, tc.scaffold, tc.empty)
		if got != tc.want {
			t.Errorf("bareDirConsentNeeded(autoInit=%t scaffold=%t empty=%t) = %t, want %t",
				tc.autoInit, tc.scaffold, tc.empty, got, tc.want)
		}
	}
}

type bareDirStubRunner struct {
	stubRunner
	autoInit []bool
	scaffold []bool
}

func (s *bareDirStubRunner) SetAutoInitRepo(on bool)    { s.autoInit = append(s.autoInit, on) }
func (s *bareDirStubRunner) SetScaffoldEnabled(on bool) { s.scaffold = append(s.scaffold, on) }

// The per-Run authorization must lift exactly the consented tiers and
// the restore func must reset each one to the boot-time value —
// leaking the lift would silently authorize later Runs against
// different repos.
func TestForwardBareDirAuthorizationSetAndRestore(t *testing.T) {
	stub := &bareDirStubRunner{}
	r := &REPL{
		runner:               stub,
		out:                  &bytes.Buffer{},
		writeAutoInitRepo:    false,
		writeScaffoldEnabled: false,
	}

	restore := r.forwardBareDirAuthorization(true)
	if len(stub.autoInit) != 1 || stub.autoInit[0] != true {
		t.Fatalf("init lift: got %v, want [true]", stub.autoInit)
	}
	if len(stub.scaffold) != 1 || stub.scaffold[0] != true {
		t.Fatalf("scaffold lift: got %v, want [true]", stub.scaffold)
	}
	restore()
	if len(stub.autoInit) != 2 || stub.autoInit[1] != false {
		t.Fatalf("init restore: got %v, want [true false]", stub.autoInit)
	}
	if len(stub.scaffold) != 2 || stub.scaffold[1] != false {
		t.Fatalf("scaffold restore: got %v, want [true false]", stub.scaffold)
	}
}

// withScaffold=false must not touch the scaffold setter at all.
func TestForwardBareDirAuthorizationInitOnly(t *testing.T) {
	stub := &bareDirStubRunner{}
	r := &REPL{
		runner:            stub,
		out:               &bytes.Buffer{},
		writeAutoInitRepo: false,
	}
	restore := r.forwardBareDirAuthorization(false)
	restore()
	if len(stub.scaffold) != 0 {
		t.Fatalf("scaffold setter touched on init-only consent: %v", stub.scaffold)
	}
	if len(stub.autoInit) != 2 {
		t.Fatalf("init set+restore expected, got %v", stub.autoInit)
	}
}

// A runner without the setter capabilities (test stubs) must not
// panic; the restore func is a no-op.
func TestForwardBareDirAuthorizationStubRunner(t *testing.T) {
	r := &REPL{runner: stubRunner{}, out: &bytes.Buffer{}}
	restore := r.forwardBareDirAuthorization(true)
	restore()
}

// Pre-dispatch consent must short-circuit silently in read mode, in
// scripted sessions, and against an already-initialized repo — those
// paths keep their pre-existing behavior byte-for-byte.
func TestPreRunBareDirConsentShortCircuits(t *testing.T) {
	// Read mode: never prompts regardless of repo state.
	r := &REPL{
		runner:      stubRunner{},
		out:         &bytes.Buffer{},
		currentMode: types.ModeRead,
		repoRoot:    t.TempDir(), // bare dir, would need init
	}
	if ok, restore := r.preRunBareDirConsent(); !ok || restore != nil {
		t.Fatalf("read mode must proceed silently")
	}

	// Scripted session (r.in != nil): the orchestrator gate keeps the
	// flag ceremony; no prompt, no lift.
	r = &REPL{
		runner:      stubRunner{},
		out:         &bytes.Buffer{},
		in:          strings.NewReader(""),
		currentMode: types.ModePlan,
		repoRoot:    t.TempDir(),
	}
	if ok, restore := r.preRunBareDirConsent(); !ok || restore != nil {
		t.Fatalf("scripted session must fall through to the orchestrator gate")
	}
}

// Bilingual pins for the three new consent surfaces.
func TestBareDirConsentMessages(t *testing.T) {
	zhTitle := autoInitScaffoldConsentTitle("zh", "/tmp/x", "not_initialized")
	if !strings.Contains(zhTitle, "git init") || !strings.Contains(zhTitle, "从零") {
		t.Fatalf("zh scaffold consent title must name both tiers: %q", zhTitle)
	}
	enTitle := autoInitScaffoldConsentTitle("en", "/tmp/x", "not_initialized")
	if !strings.Contains(enTitle, "git init") || !strings.Contains(enTitle, "scratch") {
		t.Fatalf("en scaffold consent title must name both tiers: %q", enTitle)
	}

	for _, lang := range []string{"zh", "en"} {
		lines := autoInitDeclinedPreRun(lang)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "write_auto_init_repo") || !strings.Contains(joined, "--auto-init-repo") {
			t.Fatalf("lang=%s decline lines missing pre-auth recovery: %q", lang, joined)
		}
		if !strings.Contains(joined, "--allow-scaffold") {
			t.Fatalf("lang=%s decline lines missing scaffold recovery: %q", lang, joined)
		}
	}

	zhOK := autoInitAuthorizedPreRun("zh", "/tmp/x")
	enOK := autoInitAuthorizedPreRun("en", "/tmp/x")
	if !strings.Contains(zhOK, "apply") || !strings.Contains(enOK, "apply") {
		t.Fatalf("authorized lines must say init is executed at apply stage: %q / %q", zhOK, enOK)
	}
}
