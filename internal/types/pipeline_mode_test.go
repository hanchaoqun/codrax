package types

import "testing"

// TestPipelineMode_IsValid covers every constant + the zero-value +
// one unknown value. The empty string MUST be valid so legacy
// BusContext initializations (pre-B0 tests, zero-value structs in
// the wild) continue to pass the orchestrator's validation gate.
func TestPipelineMode_IsValid(t *testing.T) {
	cases := []struct {
		mode PipelineMode
		want bool
	}{
		{"", true}, // zero-value / legacy → ModeRead equivalent
		{ModeRead, true},
		{ModePlan, true},
		{ModeApply, true},
		{ModeVerify, true},
		{PipelineMode("read_only"), false},
		{PipelineMode("PLAN"), false}, // case-sensitive by design
		{PipelineMode("bogus"), false},
	}
	for _, c := range cases {
		if got := c.mode.IsValid(); got != c.want {
			t.Errorf("PipelineMode(%q).IsValid() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// TestPipelineMode_IsWrite locks the classification used by the CLI
// layer to require --auto-apply and by the orchestrator to stand up
// a worktree. ModeRead + empty are both non-write; every other valid
// mode is a write mode including ModePlan (which produces a
// ChangePlan artifact on disk — a write-phase deliverable even
// though repo bytes stay clean).
func TestPipelineMode_IsWrite(t *testing.T) {
	cases := []struct {
		mode PipelineMode
		want bool
	}{
		{"", false},
		{ModeRead, false},
		{ModePlan, true},
		{ModeApply, true},
		{ModeVerify, true},
	}
	for _, c := range cases {
		if got := c.mode.IsWrite(); got != c.want {
			t.Errorf("PipelineMode(%q).IsWrite() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// TestPipelineMode_Normalize pins the "" → ModeRead coercion that
// the orchestrator relies on at Run() entry. All other values pass
// through untouched so downstream switch equality is exact.
func TestPipelineMode_Normalize(t *testing.T) {
	if got := PipelineMode("").Normalize(); got != ModeRead {
		t.Errorf(`PipelineMode("").Normalize() = %q, want %q`, got, ModeRead)
	}
	for _, m := range []PipelineMode{ModeRead, ModePlan, ModeApply, ModeVerify} {
		if got := m.Normalize(); got != m {
			t.Errorf("PipelineMode(%q).Normalize() = %q, want %q (identity)", m, got, m)
		}
	}
	// Unknown values still pass through — IsValid is the gate, not
	// Normalize. This lets the CLI layer report a clean error
	// message instead of silently remapping a typo.
	bogus := PipelineMode("bogus")
	if got := bogus.Normalize(); got != bogus {
		t.Errorf("PipelineMode(bogus).Normalize() = %q, want %q (pass-through)", got, bogus)
	}
}

// TestPipelineMode_StringLiteralsMatch locks the on-the-wire values
// so yaml configs, CLI flags, and log messages all share one
// canonical form. If these literals drift from the constants, every
// downstream consumer breaks silently.
func TestPipelineMode_StringLiteralsMatch(t *testing.T) {
	want := map[PipelineMode]string{
		ModeRead:   "read",
		ModePlan:   "plan",
		ModeApply:  "apply",
		ModeVerify: "verify",
	}
	for m, s := range want {
		if string(m) != s {
			t.Errorf("PipelineMode constant drifted: %q, want %q", string(m), s)
		}
	}
}
