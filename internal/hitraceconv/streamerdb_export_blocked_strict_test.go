package hitraceconv

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestTraceDBBlockedStrictThreadStateScalarsAndStableIdentity(t *testing.T) {
	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements, traceDBBlockedStrictArgs(100)...)
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 0, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (2, 1900, 100, 4095, 1, 'R', 20)",
		// Source ID 0 and the negative int32 representation of uint32
		// 2147483648 are both valid stable row identities.
		"INSERT INTO thread_state VALUES (0, 1000, NULL, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (-2147483648, 2000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		// -1 and 4294967295 are the same full-uint32 stable identity. The
		// entire duplicate cohort must be rejected even at different times.
		"INSERT INTO thread_state VALUES (-1, 3000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (4294967295, 4000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		// Every hard field below uses a deliberately untyped column so SQLite
		// retains the adversarial storage class instead of applying affinity.
		"INSERT INTO thread_state VALUES (5, CAST(5000 AS TEXT), 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (6, 6000, CAST(100 AS REAL), NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (7, 7000, 100, NULL, CAST(1 AS TEXT), 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (8, 8000, 100, NULL, 1, CAST(562 AS TEXT), 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (9, 9000, 100, NULL, 1, 562, CAST(500 AS TEXT), 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (10, 10000, 100, NULL, 1, 562, 500, CAST('D-IO' AS BLOB), 100)",
		"INSERT INTO thread_state VALUES (11, 11000, 100, NULL, 1, 562, 500, 'D-IO', CAST(100 AS TEXT))",
		"INSERT INTO thread_state VALUES (CAST(12 AS TEXT), 12000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (CAST(13 AS REAL), 13000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (14, 14000, 100, NULL, 4294967295, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (15, 15000, 100, NULL, 1, 2147483648, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (16, 16000, 100, NULL, 1, 562, 2147483648, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (17, 17000, 100, NULL, 1, 562, 500, 'D-IO', 4294967295)",
		"INSERT INTO thread_state VALUES (18, CAST('18000' AS BLOB), 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (x'ff', 19000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
	)
	// This test isolates the blocked-row decoder from the lifecycle collector:
	// several rows deliberately corrupt lifecycle activity fields. Production
	// propagation of that collector poison is pinned separately below.
	body, item := exportTraceDBBlockedStrictFixture(t, statements)
	if got := strings.Count(body, "sched_blocked_reason: pid=562"); got != 2 {
		t.Fatalf("strict blocked state rows emitted=%d, want 2 legal siblings:\n%s\ncoverage=%+v", got, body, item)
	}
	if item.RowsRead != 19 || item.RowsEmitted != 2 ||
		!strings.Contains(item.Skipped, "duplicate_source_id=2") ||
		!strings.Contains(item.Skipped, "invalid_thread_state_metadata=15") {
		t.Fatalf("strict thread_state coverage mismatch: %+v", item)
	}
	if item.FieldSources["stable_identity"] != "thread_state.id with exact full-uint32 signed-int32 projection" ||
		item.FieldSources["same_timestamp_order"] != "thread_state.ts,canonical_uint32(thread_state.id)" {
		t.Fatalf("thread_state stable identity provenance missing: %+v", item.FieldSources)
	}
}

func TestTraceDBBlockedMalformedLifecycleActivitySuppressesOtherwiseValidCandidate(t *testing.T) {
	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements, traceDBBlockedStrictArgs(100)...)
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (2, CAST(2000 AS TEXT), 100, NULL, 1, 562, 500, 'D-IO', 100)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if strings.Contains(body, "sched_blocked_reason:") {
		t.Fatalf("malformed lifecycle activity failed open for a legal sibling:\n%s", body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	for _, want := range []string{"invalid_thread_state_metadata=1", "lifecycle_rejected_thread_state_candidate=1"} {
		if !strings.Contains(item.Skipped, want) {
			t.Fatalf("malformed lifecycle suppression missing %q: %+v", want, item)
		}
	}
}

func TestTraceDBBlockedStableIdentityCanonicalOrderAndSignedMax(t *testing.T) {
	build := func(t *testing.T, reverse bool) string {
		t.Helper()
		statements := traceDBBlockedStrictFixtureSchema()
		statements = append(statements,
			"INSERT INTO process VALUES (2, 600, 'Peer')",
			"INSERT INTO thread VALUES (2, 563, 2, 'blocked-563', 0, 0, 1)",
		)
		statements = append(statements, traceDBBlockedStrictArgs(100)...)
		statements = append(statements,
			"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
			"INSERT INTO sched_slice VALUES (2, 900, 100, 5, 2, 'R', 20)",
		)
		low := "INSERT INTO thread_state VALUES (2147483647, 1000, 100, NULL, 1, 562, 500, 'D-IO', 100)"
		high := "INSERT INTO thread_state VALUES (-2147483648, 1000, 100, NULL, 2, 563, 600, 'D-IO', 100)"
		if reverse {
			statements = append(statements, low, high)
		} else {
			statements = append(statements, high, low)
		}
		body, _, _ := exportSchedulerFixture(t, statements)
		var blocked []string
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "sched_blocked_reason:") {
				blocked = append(blocked, line)
			}
		}
		return strings.Join(blocked, "\n")
	}
	forward := build(t, false)
	reverse := build(t, true)
	if forward != reverse {
		t.Fatalf("thread_state insertion order changed canonical stable order:\nforward:\n%s\nreverse:\n%s", forward, reverse)
	}
	if low, high := strings.Index(forward, "pid=562"), strings.Index(forward, "pid=563"); low < 0 || high < 0 || low >= high {
		t.Fatalf("canonical uint32 order did not place 2147483647 before signed -2147483648:\n%s", forward)
	}

	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements, traceDBBlockedStrictArgs(100)...)
	statements = append(statements,
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
		"INSERT INTO thread_state VALUES (-1, 1000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if !strings.Contains(body, "sched_blocked_reason: pid=562") || requireBlockedReasonCoverage(t, coverage).RowsEmitted != 1 {
		t.Fatalf("singleton signed -1 stable ID was mistaken for an internal sentinel:\n%s\n%+v", body, coverage)
	}
}

func TestTraceDBBlockedArgsetsUseSharedStrictResolver(t *testing.T) {
	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements,
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO data_dict VALUES (4, 'unrelated')",
		"INSERT INTO data_dict VALUES (5, 'iowait')",
		"INSERT INTO data_dict VALUES (5, 'unrelated')",
		"INSERT INTO data_dict VALUES (6, 'io_wait')",
		// Valid relevant keys plus a malformed KNOWN unrelated key: the
		// unrelated poison must not suppress the blocked-reason fact.
		"INSERT INTO args VALUES (1, 1, 0, 1, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 100)",
		"INSERT INTO args VALUES (3, 4, CAST(0 AS TEXT), 7, 100)",
		// Numeric-looking TEXT in a relevant scalar is not SQLite INTEGER.
		"INSERT INTO args VALUES (4, 1, CAST(0 AS TEXT), 1, 101)",
		"INSERT INTO args VALUES (5, 2, 1, 3, 101)",
		// Ambiguous dictionary identity can hide a relevant key behind an
		// unrelated alias; it poisons the consuming argset.
		"INSERT INTO args VALUES (6, 5, 0, 1, 102)",
		"INSERT INTO args VALUES (7, 2, 1, 3, 102)",
		// Duplicate canonical relevant key is monotonic poison.
		"INSERT INTO args VALUES (8, 1, 0, 1, 103)",
		"INSERT INTO args VALUES (9, 1, 0, 1, 103)",
		"INSERT INTO args VALUES (10, 2, 1, 3, 103)",
		// Canonical and compatibility aliases may coexist only when equal.
		"INSERT INTO args VALUES (11, 1, 0, 1, 104)",
		"INSERT INTO args VALUES (12, 6, 0, 1, 104)",
		"INSERT INTO args VALUES (13, 2, 1, 3, 104)",
	)
	for i := int64(0); i < 5; i++ {
		start := int64(1000) + i*1000
		statements = append(statements,
			"INSERT INTO sched_slice VALUES ("+itoa64(i+1)+", "+itoa64(start-100)+", 100, 4, 1, 'R', 20)",
			"INSERT INTO thread_state VALUES ("+itoa64(i+1)+", "+itoa64(start)+", 100, NULL, 1, 562, 500, 'D-IO', "+itoa64(100+i)+")",
		)
	}
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if got := strings.Count(body, "sched_blocked_reason: pid=562"); got != 2 {
		t.Fatalf("shared strict argset resolver emitted=%d, want valid argsets 100 and 104 only:\n%s", got, body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	if item.RowsRead != 5 || item.RowsEmitted != 2 || !strings.Contains(item.Skipped, "invalid_blocked_argset=3") {
		t.Fatalf("strict blocked argset coverage mismatch: %+v", item)
	}
	if !strings.Contains(item.FieldSources["argset"], "shared strict args/data_dict resolver") {
		t.Fatalf("blocked argset authority was not disclosed: %+v", item.FieldSources)
	}
}

func TestTraceDBBlockedBoundaryStrictScalarsStableIdentityAndExactState(t *testing.T) {
	tests := []struct {
		name       string
		schedRows  []string
		stateToken string
		wantEmit   bool
		wantSkip   string
	}{
		{
			name:      "numeric text CPU is not an integer witness",
			schedRows: []string{"INSERT INTO sched_slice VALUES (1, 900, 100, CAST(4 AS TEXT), 1, 'R', 20)"},
			wantSkip:  "invalid_prev_sched_slice_cpu=1",
		},
		{
			name:      "numeric blob CPU is not an integer witness",
			schedRows: []string{"INSERT INTO sched_slice VALUES (1, 900, 100, CAST('4' AS BLOB), 1, 'R', 20)"},
			wantSkip:  "invalid_prev_sched_slice_cpu=1",
		},
		{
			name:      "nonnumeric blob CPU is row-local poison",
			schedRows: []string{"INSERT INTO sched_slice VALUES (1, 900, 100, x'ff', 1, 'R', 20)"},
			wantSkip:  "invalid_prev_sched_slice_cpu=1",
		},
		{
			name:      "numeric text itid creates timestamp barrier",
			schedRows: []string{"INSERT INTO sched_slice VALUES (1, 900, 100, 4, CAST(1 AS TEXT), 'R', 20)"},
			wantSkip:  "global_prev_sched_slice_barrier=1",
		},
		{
			name:      "numeric text duration creates local lower bound",
			schedRows: []string{"INSERT INTO sched_slice VALUES (1, 900, CAST(100 AS TEXT), 4, 1, 'R', 20)"},
			wantSkip:  "prev_sched_slice_lower_bound=1",
		},
		{
			name: "placeable unrelated malformed identity does not poison",
			schedRows: []string{
				"INSERT INTO sched_slice VALUES (1, 100, 100, 3, CAST(1 AS TEXT), 'R', 20)",
				"INSERT INTO sched_slice VALUES (2, 900, 100, 4, 1, 'R', 20)",
			},
			wantEmit: true,
		},
		{
			name: "later open sched tail does not poison earlier boundary",
			schedRows: []string{
				"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
				"INSERT INTO sched_slice VALUES (2, 1100, NULL, 5, 1, 'R', 20)",
			},
			wantEmit: true,
		},
		{
			name: "earlier open sched tail blocks later boundary",
			schedRows: []string{
				"INSERT INTO sched_slice VALUES (1, 800, NULL, 5, 1, 'R', 20)",
				"INSERT INTO sched_slice VALUES (2, 900, 100, 4, 1, 'R', 20)",
			},
			wantSkip: "prev_sched_slice_lower_bound=1",
		},
		{
			name:       "near token state cannot enter hard semantics",
			schedRows:  []string{"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)"},
			stateToken: " d-io ",
			wantSkip:   "invalid_thread_state_metadata=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := traceDBBlockedStrictFixtureSchema()
			statements = append(statements, traceDBBlockedStrictArgs(100)...)
			statements = append(statements, test.schedRows...)
			state := test.stateToken
			if state == "" {
				state = "D-IO"
			}
			statements = append(statements, "INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, '"+state+"', 100)")
			body, coverage, _ := exportSchedulerFixture(t, statements)
			emitted := strings.Contains(body, "sched_blocked_reason: pid=562")
			if emitted != test.wantEmit {
				t.Fatalf("blocked boundary emitted=%t, want %t:\n%s", emitted, test.wantEmit, body)
			}
			item := requireBlockedReasonCoverage(t, coverage)
			if test.wantSkip != "" && !strings.Contains(item.Skipped, test.wantSkip) {
				t.Fatalf("blocked boundary coverage missing %q: %+v", test.wantSkip, item)
			}
		})
	}
}

func TestTraceDBBlockedSemanticCohortAndCanonicalTGIDFailClosed(t *testing.T) {
	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements,
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 100)",
		"INSERT INTO args VALUES (3, 1, 0, 1, 101)",
		"INSERT INTO args VALUES (4, 2, CAST(1 AS TEXT), 3, 101)",
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (2, 1900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (3, 2900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (4, 3900, 100, 4, 1, 'R', 20)",
		// Same physical episode, distinct stable rows: one payload is valid and
		// one poisoned. The valid row must not escape the semantic cohort gate.
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (2, 1000, 100, NULL, 1, 562, 500, 'D-IO', 101)",
		// Both directions of TGID mismatch are row-local failures.
		"INSERT INTO thread_state VALUES (3, 2000, 100, NULL, 1, 562, 0, 'D-IO', 100)",
		"INSERT INTO thread_state VALUES (4, 3000, 100, NULL, 1, 562, 501, 'D-IO', 100)",
		// Independent exact sibling survives.
		"INSERT INTO thread_state VALUES (5, 4000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if got := strings.Count(body, "sched_blocked_reason: pid=562"); got != 1 || !strings.Contains(body, "0.000004: sched_blocked_reason") {
		t.Fatalf("semantic/TGID poison did not remain cohort- and row-local:\n%s", body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	for _, want := range []string{"ambiguous_thread_state_candidate=1", "invalid_blocked_argset=1", "invalid_thread_state_metadata=1", "thread_tgid_mismatch=1"} {
		if !strings.Contains(item.Skipped, want) {
			t.Fatalf("blocked semantic/TGID coverage missing %q: %+v", want, item)
		}
	}
	if item.RowsRead != 5 || item.RowsEmitted != 1 {
		t.Fatalf("blocked semantic/TGID coverage mismatch: %+v", item)
	}
}

func TestTraceDBBlockedDelaySentinelAndValidSemanticTwinFailClosed(t *testing.T) {
	statements := traceDBBlockedStrictFixtureSchema()
	statements = append(statements,
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO data_dict VALUES (4, 'delay')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 100)",
		"INSERT INTO args VALUES (3, 4, 0, 4294967295, 100)",
		"INSERT INTO args VALUES (4, 1, 0, 1, 101)",
		"INSERT INTO args VALUES (5, 2, 1, 3, 101)",
		"INSERT INTO args VALUES (6, 4, 0, 4294967294, 101)",
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (2, 1900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (3, 2900, 100, 4, 1, 'R', 20)",
		"INSERT INTO sched_slice VALUES (4, 3900, 100, 4, 1, 'R', 20)",
		// UINT32_MAX is the upstream missing-internal-value sentinel, not a
		// publishable delay. It must fail locally without poisoning siblings.
		"INSERT INTO thread_state VALUES (1, 1000, 100, NULL, 1, 562, 500, 'D-IO', 100)",
		// Two otherwise valid rows for one physical (itid, state-start) episode
		// must both lose authority; stable row identity cannot create two facts.
		"INSERT INTO thread_state VALUES (2, 2000, 100, NULL, 1, 562, 500, 'D-IO', 101)",
		"INSERT INTO thread_state VALUES (3, 2000, 100, NULL, 1, 562, 500, 'D-IO', 101)",
		// A closed-set near-token row is a malformed potential sibling even
		// without args. It must prevent the valid same-episode row from escaping.
		"INSERT INTO thread_state VALUES (4, 3000, 100, NULL, 1, 562, 500, 'D-IO', 101)",
		"INSERT INTO thread_state VALUES (5, 3000, 100, NULL, 1, 562, 500, 'd-io', NULL)",
		"INSERT INTO thread_state VALUES (6, 4000, 100, NULL, 1, 562, 500, 'D-IO', 101)",
	)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if got := strings.Count(body, "sched_blocked_reason: pid=562"); got != 1 ||
		!strings.Contains(body, "0.000004: sched_blocked_reason") || !strings.Contains(body, "delay=4294967294") {
		t.Fatalf("delay sentinel or semantic twin escaped strict blocked gates:\n%s", body)
	}
	item := requireBlockedReasonCoverage(t, coverage)
	for _, want := range []string{"invalid_delay_arg=1", "ambiguous_thread_state_candidate=3", "invalid_thread_state_metadata=1"} {
		if !strings.Contains(item.Skipped, want) {
			t.Fatalf("blocked delay/twin coverage missing %q: %+v", want, item)
		}
	}
	if item.RowsRead != 6 || item.RowsEmitted != 1 {
		t.Fatalf("blocked delay/twin coverage mismatch: %+v", item)
	}
}

func TestTraceDBBlockedPublicIdentityAndArgsetZeroBoundaries(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 2147483647, 'MaxProc')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 2147483647, 1, 'max-thread', 0, 1, 1)",
		"CREATE TABLE sched_slice (id, ts, dur, cpu, itid, end_state, priority)",
		"INSERT INTO sched_slice VALUES (1, 900, 100, 4095, 1, 'R', 20)",
		"CREATE TABLE thread_state (id, ts, dur, cpu, itid, tid, pid, state, arg_setid)",
		"INSERT INTO thread_state VALUES (0, 1000, NULL, NULL, 1, 2147483647, 2147483647, 'S', 0)",
		"CREATE TABLE args (id, key, datatype, value, argset)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO args VALUES (1, 1, 0, 1, 0)",
		"INSERT INTO args VALUES (2, 2, 1, 3, 0)",
	}
	statements = append(statements, traceDBBlockedStrictLifecycleAuthoritySchema()...)
	body, coverage, _ := exportSchedulerFixture(t, statements)
	if !strings.Contains(body, "max-thread-2147483647 (2147483647) [4095]") || !strings.Contains(body, "sched_blocked_reason: pid=2147483647") {
		t.Fatalf("valid public/source/argset/CPU upper and zero boundaries were lost:\n%s", body)
	}
	if item := requireBlockedReasonCoverage(t, coverage); item.RowsEmitted != 1 || item.Skipped != "" {
		t.Fatalf("valid blocked boundary coverage mismatch: %+v", item)
	}
}

func TestTraceDBBlockedLegacyStableRowIDProfileAndNoRowIDFailure(t *testing.T) {
	t.Run("id-less tables use audited rowid", func(t *testing.T) {
		statements := traceDBBlockedStrictFixtureSchemaWithoutSourceIDs(false)
		statements = append(statements, traceDBBlockedStrictArgsWithoutID(100)...)
		statements = append(statements,
			"INSERT INTO sched_slice VALUES (900, 100, 4, 1, 'R', 20)",
			"INSERT INTO sched_slice VALUES (1900, 100, 5, 1, 'R', 20)",
			"INSERT INTO thread_state(rowid, ts, dur, cpu, itid, tid, pid, state, arg_setid) VALUES (-1, 1000, 100, NULL, 1, 562, 500, 'S', 100)",
			"INSERT INTO thread_state(rowid, ts, dur, cpu, itid, tid, pid, state, arg_setid) VALUES (0, 2000, 100, NULL, 1, 562, 500, 'S', 100)",
		)
		body, coverage, _ := exportSchedulerFixture(t, statements)
		if strings.Count(body, "sched_blocked_reason: pid=562") != 2 {
			t.Fatalf("id-less signed rowid compatibility profile lost a legal blocked marker:\n%s", body)
		}
		item := requireBlockedReasonCoverage(t, coverage)
		if item.FieldSources["stable_identity"] != "thread_state.rowid" {
			t.Fatalf("rowid compatibility provenance missing: %+v", item.FieldSources)
		}
	})

	t.Run("WITHOUT ROWID and no source id fails closed", func(t *testing.T) {
		statements := traceDBBlockedStrictFixtureSchemaWithoutSourceIDs(true)
		statements = append(statements, traceDBBlockedStrictArgsWithoutID(100)...)
		body, coverage, _ := exportSchedulerFixture(t, statements)
		if strings.Contains(body, "sched_blocked_reason:") {
			t.Fatalf("identity-less WITHOUT ROWID table minted blocked evidence:\n%s", body)
		}
		item := requireBlockedReasonCoverage(t, coverage)
		if !strings.Contains(item.Skipped, "missing thread_state.id and usable SQLite rowid") {
			t.Fatalf("missing stable identity was not disclosed: %+v", item)
		}
	})
}

func TestTraceDBBlockedImplementationBansSQLCoercionAndPrefilterBypass(t *testing.T) {
	content, err := os.ReadFile("streamerdb_export_blocked.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, banned := range []string{
		"sql.NullInt64", "sql.NullString",
		"WHERE arg_setid IS NOT NULL",
		"WHERE itid IS NOT NULL",
		"WHERE key_dict.data IN",
		"strings.ToUpper(strings.TrimSpace(state))",
		"firstNonZero(candidate.Process.PID",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("blocked strict endpoint retained bypass %q", banned)
		}
	}
}

func traceDBBlockedStrictFixtureSchema() []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 500, 'App')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 562, 1, 'blocked-562', 0, 0, 1)",
		"CREATE TABLE sched_slice (id, ts, dur, cpu, itid, end_state, priority)",
		"CREATE TABLE thread_state (id, ts, dur, cpu, itid, tid, pid, state, arg_setid)",
		"CREATE TABLE args (id, key, datatype, value, argset)",
		"CREATE TABLE data_dict (id, data)",
	}
	return append(statements, traceDBBlockedStrictLifecycleAuthoritySchema()...)
}

func traceDBBlockedStrictFixtureSchemaWithoutSourceIDs(withoutRowID bool) []string {
	threadState := "CREATE TABLE thread_state (ts, dur, cpu, itid, tid, pid, state, arg_setid)"
	sched := "CREATE TABLE sched_slice (ts, dur, cpu, itid, end_state, priority)"
	if withoutRowID {
		threadState = "CREATE TABLE thread_state (ts INTEGER PRIMARY KEY, dur, cpu, itid, tid, pid, state, arg_setid) WITHOUT ROWID"
	}
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 500, 'App')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 562, 1, 'blocked-562', 0, 0, 1)",
		sched,
		threadState,
		"CREATE TABLE args (argset, key, datatype, value, PRIMARY KEY (argset, key)) WITHOUT ROWID",
		"CREATE TABLE data_dict (id PRIMARY KEY, data) WITHOUT ROWID",
	}
	return append(statements, traceDBBlockedStrictLifecycleAuthoritySchema()...)
}

func traceDBBlockedStrictLifecycleAuthoritySchema() []string {
	return []string{
		"CREATE TABLE instant (ts, name, ref, ref_type)",
		"CREATE TABLE callstack (ts, itid, callid)",
		"CREATE TABLE syscall (ts, itid)",
		"CREATE TABLE native_hook (start_ts, itid)",
		"CREATE TABLE frame_slice (id, type, ts, itid)",
	}
}

func exportTraceDBBlockedStrictFixture(t *testing.T, statements []string) (string, TraceDBCoverage) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	index, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBBlockedReasons(context.Background(), tdb, sink,
		traceDBTestCompleteSchedulerAuthority(index))
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	if _, err := sink.prepareAndWriteForTest(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	return body.String(), coverage
}

func traceDBBlockedStrictArgs(argset int64) []string {
	return []string{
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO args VALUES (1, 1, 0, 1, " + itoa64(argset) + ")",
		"INSERT INTO args VALUES (2, 2, 1, 3, " + itoa64(argset) + ")",
	}
}

func traceDBBlockedStrictArgsWithoutID(argset int64) []string {
	return []string{
		"INSERT INTO data_dict VALUES (1, 'iowait')",
		"INSERT INTO data_dict VALUES (2, 'caller')",
		"INSERT INTO data_dict VALUES (3, 'schedule_timeout')",
		"INSERT INTO args VALUES (" + itoa64(argset) + ", 1, 0, 1)",
		"INSERT INTO args VALUES (" + itoa64(argset) + ", 2, 1, 3)",
	}
}

func itoa64(value int64) string {
	return strconv.FormatInt(value, 10)
}
