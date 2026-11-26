package tool

import (
	"regexp"
	"strings"
	"testing"
)

// Own-work pricing on the reader face, public trace_query entry, complete
// original capture (opt-in; skipped without the variable, failing when the
// file is not the pinned original).
//
// The engine row and the model-facing board row of own-work pricing are pinned
// on a capture that a real evaluation case also uses. The reader face is
// pinned here on a capture no evaluation case reads: Honeybird, device-reboot
// cold start 5, main thread 10566, the closed bindApplication window
// [162.372184, 163.616645).
//
// Every expected number comes from a standalone raw oracle (Python over the
// ftrace text only: the thread's own B/E markers and its sched_switch
// records), never from this engine: 129 class-verification instances in the
// window, 37.529 ms as their union, the largest 4.784 ms, 33.090 ms of the
// union on a CPU, 294.846 ms of the window running.
//
// The assertions read numbers and their measure labels only. The English
// count noun of the class clause and the position words of semantic rows are
// left out on purpose: neither is what this pin guards.
func TestHMC378WholeOriginalOwnWorkReaderFace(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_HONEYBIRD_REBOOT",
		"54405241db707b901057b41a5c94e69a3fc233425fdf7a1f745e43585cf248d4", 124269104)
	const tid, start, end = 10566, 162.372184, 163.616645
	delivered := hmc377DeliverWindow(t, path, tid, start, end)
	if delivered.rank == nil {
		t.Fatal("no ranking was published")
	}
	row := hmc377OwnRow(delivered.rank, tid, "class_verification")
	if row == nil {
		t.Fatalf("the main thread's own class verification is not a ranked on-chain cause:\n%s", hmc376Board(delivered.rank))
	}
	if row.ChainRelevance != "on_chain" || row.MemberCount != 129 || !hmc376Near(row.EffectiveImpactMs, 37.529) ||
		!hmc376Near(row.OwnWorkOnCPUMs, 33.090) || !hmc376Near(row.MemberMaxMs, 4.784) {
		t.Fatalf("the own verification row must be the raw oracle's 129 members / 37.529 ms / 33.090 ms on a CPU / largest 4.784 ms: %s members=%d on_cpu=%.3f max=%.3f",
			hmc376RowLabel(*row), row.MemberCount, row.OwnWorkOnCPUMs, row.MemberMaxMs)
	}

	// The two reader sentences that carry the account, in both languages.
	// Occupancy table: the row is the record wall (the union), with the
	// record count and the record maximum.
	occupancy := map[string]*regexp.Regexp{
		"zh": regexp.MustCompile(`\| 确定性工作 span（墙钟） \| [^|]*-10566 / VerifyClass [^|]* \| 37\.529ms \| 4\.784ms（记录最大值） \| 129 条统计记录 \|`),
		"en": regexp.MustCompile(`\| Deterministic work spans \(wall clock\) \| [^|]*-10566 / VerifyClass [^|]* \| 37\.529ms \| 4\.784ms \(record maximum\) \| 129 records \|`),
	}
	// Four-state line: the deterministic part of running time is the on-CPU
	// share of the union, and the class wall beside it is named as a wall.
	fourState := map[string][]string{
		"zh": {"running 294.846ms: 确定性工作 33.090ms(运行∩语义span;", "最大类墙钟 37.529ms"},
		"en": {"running 294.846ms: deterministic work 33.090ms (running ∩ semantic spans;", "largest class wall 37.529ms"},
	}
	for language, face := range map[string]string{"zh": delivered.zh, "en": delivered.en} {
		if !occupancy[language].MatchString(face) {
			t.Errorf("%s occupancy table lost the own verification row (129 records, wall 37.529 ms, record maximum 4.784 ms)", language)
		}
		line := ""
		for _, candidate := range strings.Split(face, "\n") {
			if strings.Contains(candidate, fourState[language][0]) {
				line = candidate
				break
			}
		}
		if line == "" || !strings.Contains(line, fourState[language][1]) {
			t.Errorf("%s four-state line must state 33.090 ms as running∩semantic and 37.529 ms as the class wall, got %q", language, line)
		}
		// The on-CPU share never reads as the class wall or the reverse.
		for _, swapped := range []string{"确定性工作 37.529ms", "最大类墙钟 33.090ms", "deterministic work 37.529ms", "largest class wall 33.090ms"} {
			if strings.Contains(face, swapped) {
				t.Errorf("%s face states a number under the other measure's label: %q", language, swapped)
			}
		}
	}
}
