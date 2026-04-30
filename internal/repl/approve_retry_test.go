package repl

import (
	"testing"
)

// TestParseApproveArgsAll_RetryFlag pins the new --retry parsing.
func TestParseApproveArgsAll_RetryFlag(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"/approve", false},
		{"/approve --retry", true},
		{"/approve plan-abc --retry", true},
		{"/approve --retry plan-abc", true},
		{"/approve --skip-verify --retry", true},
		{"/approve --skip-verify", false},
		{"/approve --retry --merge-to=main", true},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			_, _, _, retry := parseApproveArgsAll(c.line)
			if retry != c.want {
				t.Errorf("parseApproveArgsAll(%q) retry = %v; want %v", c.line, retry, c.want)
			}
		})
	}
}

// TestParseApproveArgsAll_RetryWithOtherFlags verifies retry +
// other flags compose correctly (no flag fields collide).
func TestParseApproveArgsAll_RetryWithOtherFlags(t *testing.T) {
	planID, mergeTo, skipVerify, retry := parseApproveArgsAll(
		"/approve plan-abc --retry --skip-verify --merge-to=main")
	if planID != "plan-abc" {
		t.Errorf("planID = %q", planID)
	}
	if mergeTo != "main" {
		t.Errorf("mergeTo = %q", mergeTo)
	}
	if !skipVerify {
		t.Error("skipVerify expected true")
	}
	if !retry {
		t.Error("retry expected true")
	}
}

// TestParseApproveArgs_BackCompat verifies the original
// 3-return-value parseApproveArgs still works for callers that
// don't care about retry.
func TestParseApproveArgs_BackCompat(t *testing.T) {
	planID, mergeTo, skipVerify := parseApproveArgs("/approve plan-x --skip-verify --merge-to=branch")
	if planID != "plan-x" || mergeTo != "branch" || !skipVerify {
		t.Errorf("back-compat parseApproveArgs drift: planID=%q mergeTo=%q skipVerify=%v",
			planID, mergeTo, skipVerify)
	}
}
