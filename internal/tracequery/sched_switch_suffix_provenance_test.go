package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchedSwitchSuffixProvenanceParseLine(t *testing.T) {
	intern := newStringInterner()
	line := func(fields string) string {
		return "task-10 (10) [001] .... 1.000000: sched_switch: " + fields
	}

	t.Run("real six tuple suffix and scheduler domains", func(t *testing.T) {
		ev, ok := ParseLine(1, line("prev_comm=Signal Catcher prev_pid=812 prev_prio=301 prev_state=R+ ==> next_comm=Jit thread pool next_pid=900 next_prio=-2 next_info=f,11,2,1,3,17 cg=top-app"), intern)
		if !ok {
			t.Fatal("canonical Donghu sched_switch must parse")
		}
		if ev.PrevComm != "Signal Catcher" || ev.NextComm != "Jit thread pool" ||
			ev.PrevPID != 812 || ev.NextPID != 900 || ev.PrevPrio != 301 || ev.NextPrio != -2 || ev.PrevState != "R+" {
			t.Fatalf("scheduler core changed: %+v", ev)
		}
		if ev.NextInfo != "f,11,2,1,3,17" || ev.CGroup != "top-app" ||
			ev.NextInfoAffinity != "f" || ev.NextInfoLoad != 11 || ev.NextInfoGroup != 2 ||
			!ev.NextInfoRestricted || ev.NextInfoExpel != 3 || ev.NextInfoCGID != 17 {
			t.Fatalf("real six-tuple suffix did not retain typed metadata: %+v", ev)
		}
	})

	t.Run("real five tuple with external cgroup", func(t *testing.T) {
		ev, ok := ParseLine(2, line("prev_comm=idle/1 prev_pid=0 prev_prio=-2 prev_state=R ==> next_comm=worker pool next_pid=200 next_prio=301 next_info=3,4,1,1,0 cgroup=foreground"), intern)
		if !ok || ev.NextInfo != "3,4,1,1,0" || ev.CGroup != "foreground" ||
			ev.NextInfoAffinity != "3" || ev.NextInfoLoad != 4 || ev.NextInfoGroup != 1 ||
			!ev.NextInfoRestricted || ev.NextInfoExpel != 0 || ev.NextInfoCGID != 0 {
			t.Fatalf("five-tuple/external-cgroup suffix mismatch: ok=%v ev=%+v", ok, ev)
		}
	})

	t.Run("key looking comm text is not a suffix", func(t *testing.T) {
		prevComm := "Signal next_info=bad cg=prev-forged Catcher"
		nextComm := "Jit cg=next-forged next_info=ffff,2046,3,1,7,31 thread pool"
		ev, ok := ParseLine(3, line(fmt.Sprintf("prev_comm=%s prev_pid=812 prev_prio=301 prev_state=R+ ==> next_comm=%s next_pid=900 next_prio=-2", prevComm, nextComm)), intern)
		if !ok {
			t.Fatal("key-looking text inside legal comm values must not reject the scheduler core")
		}
		if ev.PrevComm != prevComm || ev.NextComm != nextComm {
			t.Fatalf("comm text was truncated at a fake optional key: %+v", ev)
		}
		if ev.NextInfo != "" || ev.CGroup != "" || ev.NextInfoAffinity != "" || len(ev.NextInfoAllowedCPUs) != 0 {
			t.Fatalf("comm text crossed into typed scheduler metadata: %+v", ev)
		}
	})

	t.Run("standalone arrow text remains part of dynamic comm", func(t *testing.T) {
		prevComm := "worker ==> io stage"
		nextComm := "render ==> submit stage"
		ev, ok := ParseLine(3, line(fmt.Sprintf("prev_comm=%s prev_pid=812 prev_prio=301 prev_state=R+ ==> next_comm=%s next_pid=900 next_prio=-2 next_info=f,11,2,1,3,17", prevComm, nextComm)), intern)
		if !ok || ev.PrevComm != prevComm || ev.NextComm != nextComm || ev.NextInfo != "f,11,2,1,3,17" {
			t.Fatalf("standalone delimiter-looking comm text changed structure: ok=%v ev=%+v", ok, ev)
		}
	})

	t.Run("real suffix wins without rewriting comm", func(t *testing.T) {
		nextComm := "worker next_info=comm-only cg=comm-only name"
		ev, ok := ParseLine(4, line(fmt.Sprintf("prev_comm=idle/1 prev_pid=0 prev_prio=-2 prev_state=R ==> next_comm=%s next_pid=200 next_prio=301 expeller_type=0 next_info=3,4,1,1,0 cg=real", nextComm)), intern)
		if !ok || ev.NextComm != nextComm || ev.NextInfo != "3,4,1,1,0" || ev.CGroup != "real" {
			t.Fatalf("real suffix and key-looking comm must remain separate: ok=%v ev=%+v", ok, ev)
		}
	})

	t.Run("long space bearing comm is parsed before field text clamp", func(t *testing.T) {
		nextComm := strings.Repeat("long segment ", 36) + "next_info=comm-only cg=comm-only tail"
		ev, ok := ParseLine(5, line(fmt.Sprintf("prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=%s next_pid=200 next_prio=20", nextComm)), intern)
		if !ok || ev.NextComm != nextComm || ev.NextInfo != "" || ev.CGroup != "" {
			t.Fatalf("long comm provenance mismatch: ok=%v len(got)=%d len(want)=%d ev=%+v", ok, len(ev.NextComm), len(nextComm), ev)
		}
		if len(ev.FieldText) != 300 || !strings.HasSuffix(ev.FieldText, "...") {
			t.Fatalf("test must exercise parsing from full fields before the display clamp: len=%d text=%q", len(ev.FieldText), ev.FieldText)
		}
	})

	t.Run("optional duplicate authorities omit only their family", func(t *testing.T) {
		base := "prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 "
		tests := []struct {
			name         string
			suffix       string
			wantNextInfo string
			wantCGroup   string
		}{
			{name: "duplicate next info same value", suffix: "next_info=3,4,1,1,0 next_info=3,4,1,1,0 cg=real", wantCGroup: "real"},
			{name: "duplicate next info different value", suffix: "next_info=3,4,1,1,0 next_info=f,6,2,0,1,7 cgroup=real", wantCGroup: "real"},
			{name: "cg and cgroup aliases", suffix: "next_info=3,4,1,1,0 cg=one cgroup=one", wantNextInfo: "3,4,1,1,0"},
			{name: "duplicate cg same value", suffix: "next_info=3,4,1,1,0 cg=one cg=one", wantNextInfo: "3,4,1,1,0"},
			{name: "quoted unrelated value cannot inject", suffix: `note="foo next_info=evil cg=evil" cg=real`, wantCGroup: "real"},
			{name: "single quoted unrelated value cannot inject", suffix: `note='foo next_info=evil cg=evil' next_info=3,4,1,1,0`, wantNextInfo: "3,4,1,1,0"},
			{name: "reverse known family order", suffix: "cg=real next_info=3,4,1,1,0", wantNextInfo: "3,4,1,1,0", wantCGroup: "real"},
			{name: "tab separated renderer suffix", suffix: "expeller_type=0\tnext_info=3,4,1,1,0\tcg=real", wantNextInfo: "3,4,1,1,0", wantCGroup: "real"},
			{name: "empty next info declaration poisons only next info", suffix: "next_info= next_info=3,4,1,1,0 cg=real", wantCGroup: "real"},
			{name: "empty cgroup declaration poisons only cgroup", suffix: "cg= cgroup=real next_info=3,4,1,1,0", wantNextInfo: "3,4,1,1,0"},
			{name: "non-token substring cannot inject", suffix: "note-next_info=evil note-cg=evil"},
		}
		for i, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ev, ok := ParseLine(10+i, line(base+tt.suffix), intern)
				if !ok || ev.NextInfo != tt.wantNextInfo || ev.CGroup != tt.wantCGroup {
					t.Fatalf("optional authority mismatch: ok=%v next_info=%q cgroup=%q ev=%+v", ok, ev.NextInfo, ev.CGroup, ev)
				}
			})
		}
	})

	t.Run("malformed optional suffix cannot mint metadata", func(t *testing.T) {
		base := "prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 "
		for i, suffix := range []string{
			`note="unterminated next_info=forged cg=forged`,
			`note='unterminated next_info=forged cg=forged`,
			"next_info=3,4,1,1,0 cg=real bare-tail",
			"note-next_info=forged next_info=3,4,1,1,0 trailing",
		} {
			ev, ok := ParseLine(40+i, line(base+suffix), intern)
			if !ok {
				t.Fatalf("optional syntax failure must retain the validated core, suffix=%q", suffix)
			}
			if ev.NextInfo != "" || ev.CGroup != "" || ev.NextInfoAffinity != "" {
				t.Fatalf("syntax-bad suffix minted optional metadata, suffix=%q ev=%+v", suffix, ev)
			}
		}
	})

	t.Run("complete fake structural arms fail closed", func(t *testing.T) {
		for i, fields := range []string{
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 next_pid=999 next_prio=1 next_info=forged cg=forged",
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=999 next_prio=1 alias next_pid=200 next_prio=20 next_info=forged",
			"prev_comm=fake prev_pid=9 prev_prio=1 prev_state=S ==> next_comm=fake-name prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20",
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 next_prio=1 next_info=forged",
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 next_pid=999 next_info=forged",
			`prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=999 next_prio=1 note="x next_pid=200 next_prio=abc" next_info=f,11,2,1,3,17 cg=forged`,
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=999 next_prio=1 note=x next_pid=200 next_prio=abc next_info=f,11,2,1,3,17 cg=forged",
			`prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=999 next_prio=1 note="x next_pid = 200 next_prio = abc" next_info=f,11,2,1,3,17 cg=forged`,
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=999 next_prio=1 note=x\tnext_pid\t=\t200\tnext_prio\t=\tabc next_info=f,11,2,1,3,17 cg=forged",
			"prev_comm=evil prev_pid=9 prev_prio=1 prev_state=R ==> next_comm=x prev_pid=0 prev_prio=abc prev_state=S ==> next_comm=worker next_pid=200 next_prio=20 next_info=f,11,2,1,3,17 cg=forged",
			"prev_comm=foo next_pid=999 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_prio=20",
		} {
			if ev, ok := ParseLine(50+i, line(fields), intern); ok {
				t.Fatalf("ambiguous/reserved core suffix must reject the row, fields=%q ev=%+v", fields, ev)
			}
		}
	})

	t.Run("empty comm labels do not erase PID identity", func(t *testing.T) {
		for i, fields := range []string{
			"prev_comm= prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20",
			"prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm= next_pid=200 next_prio=20",
			"prev_comm=\tprev_pid=0\tprev_prio=120\tprev_state=R\t==>\tnext_comm=\tnext_pid=200\tnext_prio=20",
		} {
			ev, ok := ParseLine(60+i, line(fields), intern)
			if !ok || ev.PrevPID != 0 || ev.NextPID != 200 {
				t.Fatalf("empty display label must retain typed scheduler identity: fields=%q ok=%v ev=%+v", fields, ok, ev)
			}
		}
	})

	t.Run("core numeric boundaries and canonical spelling", func(t *testing.T) {
		valid := "prev_comm=prev prev_pid=2147483647 prev_prio=-2147483648 prev_state=R+ ==> next_comm=next next_pid=0 next_prio=2147483647"
		ev, ok := ParseLine(20, line(valid), intern)
		if !ok || ev.PrevPID != 2147483647 || ev.PrevPrio != -2147483648 || ev.NextPID != 0 || ev.NextPrio != 2147483647 {
			t.Fatalf("signed-int32/PID boundary changed: ok=%v ev=%+v", ok, ev)
		}
		invalid := []string{
			"prev_comm=prev prev_pid=2147483648 prev_prio=20 prev_state=R ==> next_comm=next next_pid=1 next_prio=20 next_info=forged",
			"prev_comm=prev prev_pid=1 prev_prio=-2147483649 prev_state=R ==> next_comm=next next_pid=2 next_prio=20 cg=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=2147483648 next_info=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=+20 cg=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=-0 cg=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=020 cg=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=20.0 cg=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2",
			"prev_comm=prev prev_pid=01 prev_prio=20 prev_state=R ==> next_comm=next next_pid=2 next_prio=20 next_info=forged",
			"prev_comm=prev prev_pid=1 prev_prio=20 prev_state=R ==> next_comm=next next_prio=20 next_pid=2 next_info=forged",
		}
		for i, fields := range invalid {
			if ev, ok := ParseLine(30+i, line(fields), intern); ok {
				t.Fatalf("non-canonical/malformed scheduler core must fail closed, fields=%q ev=%+v", fields, ev)
			}
		}
	})

	t.Run("Harmony priority domains are values not parser policy", func(t *testing.T) {
		for i, prio := range []string{"-2", "140", "159", "301", "65535"} {
			ev, ok := ParseLine(70+i, line("prev_comm=idle prev_pid=0 prev_prio="+prio+" prev_state=R ==> next_comm=worker next_pid=200 next_prio="+prio), intern)
			if !ok || fmt.Sprint(ev.PrevPrio) != prio || fmt.Sprint(ev.NextPrio) != prio {
				t.Fatalf("canonical Harmony priority must parse unchanged, prio=%s ok=%v ev=%+v", prio, ok, ev)
			}
		}
	})

	t.Run("non ASCII whitespace never creates a sibling token", func(t *testing.T) {
		ev, ok := ParseLine(80, line("prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 next_info=opaque\u00a0cg=forged"), intern)
		if !ok || ev.CGroup != "" {
			t.Fatalf("NBSP must not be treated as an ASCII field separator: ok=%v ev=%+v", ok, ev)
		}
	})
}

func TestSchedSwitchParseFailureReasonIsNotFabricatedMissingIdentity(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "invalid next priority",
			line: "task-10 (10) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=abc",
			want: "next_prio_invalid",
		},
		{
			name: "ambiguous right core",
			line: "task-10 (10) [001] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20 next_pid=999 next_prio=1",
			want: "next_pid_ambiguous",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failure := schedulerRowValidationFailure(1, tc.line)
			if failure == nil || !containsSubstring(failure.Fields, tc.want) {
				t.Fatalf("typed parser reason lost: want=%q failure=%+v", tc.want, failure)
			}
			if containsSubstring(failure.Fields, "prev_pid_missing") || containsSubstring(failure.Fields, "next_pid_missing") {
				t.Fatalf("failure fabricated an unrelated missing identity: %+v", failure)
			}
		})
	}
}

func TestSchedSwitchSuffixProvenanceExternalAndConverterRoundTrip(t *testing.T) {
	externalFakeComm := "Signal Catcher next_info=comm-only cg=comm-only"
	converterFakeComm := strings.Repeat("converted worker ", 24) + "next_info=comm-only cg=comm-only tail"
	lines := []string{
		fmt.Sprintf("external-10 (10) [001] d..3 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R+ ==> next_comm=%s next_pid=100 next_prio=-2", externalFakeComm),
		"external-10 (10) [001] d..3 2.100000: sched_switch: prev_comm=external app prev_pid=100 prev_prio=-2 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=301 next_info=3,4,1,1,0 cgroup=foreground",
		fmt.Sprintf("converted-20 (20) [002] .... 2.200000: sched_switch: prev_comm=tppmgr-idle-2 prev_pid=0 prev_prio=-1 prev_state=R ==> next_comm=%s next_pid=200 next_prio=301", converterFakeComm),
		"converted-20 (20) [002] .... 2.300000: sched_switch: prev_comm=converted worker prev_pid=200 prev_prio=301 prev_state=S ==> next_comm=tppmgr-idle-2 next_pid=0 next_prio=-1 next_info=f,11,2,1,3,17 cg=top-app",
	}
	path := filepath.Join(t.TempDir(), "sched-suffix-roundtrip.systrace")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("build external/converter-shaped scheduler index: %v", err)
	}
	if len(idx.Events) != 4 {
		t.Fatalf("roundtrip event count: got=%d unparsed=%d events=%+v", len(idx.Events), idx.UnparsedLines, idx.Events)
	}
	externalFake, externalReal := idx.Events[0], idx.Events[1]
	converterFake, converterReal := idx.Events[2], idx.Events[3]
	if externalFake.NextComm != externalFakeComm || externalFake.NextInfo != "" || externalFake.CGroup != "" ||
		externalFake.NextPrio != -2 || externalFake.PrevState != "R+" {
		t.Fatalf("external ftrace fake suffix escaped comm boundary: %+v", externalFake)
	}
	if externalReal.NextInfo != "3,4,1,1,0" || externalReal.CGroup != "foreground" || externalReal.NextPrio != 301 {
		t.Fatalf("external ftrace real suffix was lost: %+v", externalReal)
	}
	if converterFake.NextComm != converterFakeComm || converterFake.NextInfo != "" || converterFake.CGroup != "" || converterFake.NextPrio != 301 {
		t.Fatalf("converter-shaped fake suffix escaped comm boundary: %+v", converterFake)
	}
	if converterReal.NextInfo != "f,11,2,1,3,17" || converterReal.CGroup != "top-app" ||
		converterReal.NextInfoCGID != 17 || converterReal.NextPrio != -1 {
		t.Fatalf("converter-shaped real suffix did not round-trip: %+v", converterReal)
	}
}
