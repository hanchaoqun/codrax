package env

import "time"

// mustParseTime is a test-only helper that panics on bad input.
// Used to build NetworkStatus.ProbedAt fixtures with a known
// non-zero year (the demote heuristic checks ProbedAt.Year() > 0).
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
