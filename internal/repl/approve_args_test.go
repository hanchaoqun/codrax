package repl

import "testing"

// TestParseApproveArgs covers every recognised /approve argument
// shape — positional plan-id, --plan-id=, --merge-to=,
// --merge-to <branch>, --skip-verify, and combinations. Locking
// this contract protects /approve from quietly losing a flag when
// a future refactor replaces parseApproveArgs.
func TestParseApproveArgs(t *testing.T) {
	for _, tc := range []struct {
		in              string
		wantPlanID      string
		wantMergeTo     string
		wantSkipVerify  bool
	}{
		// Bare /approve: no args.
		{"/approve", "", "", false},

		// Positional plan-id.
		{"/approve plan-1730834521-12345", "plan-1730834521-12345", "", false},

		// --plan-id= long form.
		{"/approve --plan-id=plan-abc", "plan-abc", "", false},

		// --plan-id <id> two-token form.
		{"/approve --plan-id plan-xyz", "plan-xyz", "", false},

		// --merge-to= flag.
		{"/approve --merge-to=fix/foo", "", "fix/foo", false},

		// --skip-verify flag.
		{"/approve --skip-verify", "", "", true},

		// Combination: positional plan-id + --merge-to + --skip-verify.
		{"/approve plan-99 --merge-to=fix/x --skip-verify", "plan-99", "fix/x", true},

		// Combination: --plan-id= + --skip-verify.
		{"/approve --plan-id=plan-1 --skip-verify", "plan-1", "", true},

		// First plan-XYZ wins on positional shape.
		{"/approve plan-aaa plan-bbb", "plan-aaa", "", false},

		// Token that doesn't start with plan- isn't taken as plan-id.
		{"/approve fix/foo", "", "", false},

		// Stray junk: unknown flags ignored.
		{"/approve --bogus=zzz plan-1", "plan-1", "", false},
	} {
		gotPlanID, gotMergeTo, gotSkipVerify := parseApproveArgs(tc.in)
		if gotPlanID != tc.wantPlanID || gotMergeTo != tc.wantMergeTo || gotSkipVerify != tc.wantSkipVerify {
			t.Errorf("parseApproveArgs(%q):\n got planID=%q mergeTo=%q skipVerify=%v\nwant planID=%q mergeTo=%q skipVerify=%v",
				tc.in, gotPlanID, gotMergeTo, gotSkipVerify,
				tc.wantPlanID, tc.wantMergeTo, tc.wantSkipVerify)
		}
	}
}

// TestParseMergeToArgBackcompat confirms the legacy single-purpose
// parseMergeToArg helper still returns the same value as the new
// composite parser. Keeps existing /merge tests working without
// edits.
func TestParseMergeToArgBackcompat(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"/approve --merge-to=fix/foo", "fix/foo"},
		{"/approve --merge-to fix/bar", "fix/bar"},
		{"/approve plan-1 --merge-to=fix/baz", "fix/baz"},
		{"/approve", ""},
	} {
		got := parseMergeToArg(tc.in)
		if got != tc.want {
			t.Errorf("parseMergeToArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
