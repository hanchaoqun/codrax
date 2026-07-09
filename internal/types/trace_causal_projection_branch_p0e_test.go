package types

// P0-E CHAIN-PATH election pins (ledger §22.1, 2026-07-09) — EVOLUTION of the
// B1/B1-b anchor-election family (bar only strengthened, evolution recorded in
// the traceCausalProjectionSelectWakeupPath doc): candidates are now
// per-branch TRUE path records (typed branch= rich note), so the election
// picks a BRANCH. The B1/B1-b law itself is unchanged (any-position match,
// last-occurrence truncation, 位置0不可当选, Subject > deepest > publication
// tie ladder, legacy candidates[0] fallback) — those clauses stay pinned by
// the b1/b1b test files, which build identity-less records and therefore keep
// exercising the legacy lane byte-stably.
//
// MUTATION self-checks:
//   - deleting the branch-form pool switch reds
//     TestTraceCausalProjectionBranchFormCandidatesOwnThePool (the earlier
//     legacy candidate wins publication order again);
//   - dropping the rootDepth return reds
//     TestTraceCausalProjectionTruncatedElectionCarriesRootDepth.

import (
	"reflect"
	"testing"
)

func branchP0EPathRecord(id, target, path string, branch, branches int) ObservationRecord {
	record := anchorB1PathRecord(id, target, path)
	record.RichNotes = []string{
		"path=" + path,
		"target=" + target,
		"branch=" + itoaP0E(branch),
		"branches=" + itoaP0E(branches),
	}
	return record
}

func itoaP0E(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// Pool switch: when ANY branch-form candidate exists, identity-less
// candidates (the legacy flattened record / the text-parse reconstruction
// lane) never compete — even when they published FIRST and would win the
// legacy publication-order fallback.
func TestTraceCausalProjectionBranchFormCandidatesOwnThePool(t *testing.T) {
	legacyFirst := anchorB1PathRecord("legacy-flat", "oney.hmn.berlin-42591",
		"VSyncGenerator-2270 -> oney.hmn.berlin-42591 -> VSyncGenerator-2270 -> oney.hmn.berlin-42591")
	branch := branchP0EPathRecord("branch-8", "oney.hmn.berlin-42591",
		"tppmgr-idle-0-185 -> gpu-pm-release-327 -> RSUniRenderThre-1963 -> oney.hmn.berlin-42591", 8, 8)

	// No entities: the legacy fallback is candidates[0] — with the pool
	// switch, candidates[0] of the BRANCH pool, never the flattened record.
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{legacyFirst, branch})
	want := []string{"tppmgr-idle-0-185", "gpu-pm-release-327", "RSUniRenderThre-1963", "oney.hmn.berlin-42591"}
	if !reflect.DeepEqual(got.WakeupPath, want) {
		t.Fatalf("branch-form candidate must own the pool over an earlier legacy candidate,\n got %v\nwant %v", got.WakeupPath, want)
	}
	if got.WakeupPathBranch != 8 {
		t.Fatalf("elected branch ordinal must publish, got %d want 8", got.WakeupPathBranch)
	}
	if got.WakeupPathRootDepth != 0 {
		t.Fatalf("untruncated election must carry rootDepth 0, got %d", got.WakeupPathRootDepth)
	}

	// Entity election inside the branch pool: the target entity matches every
	// branch candidate's END; the deepest position wins → the longest branch.
	shallow := branchP0EPathRecord("branch-1", "oney.hmn.berlin-42591",
		"VSyncGenerator-2270 -> oney.hmn.berlin-42591", 1, 8)
	elected := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{legacyFirst, shallow, branch}, []string{"42591"})
	if !reflect.DeepEqual(elected.WakeupPath, want) || !elected.WakeupPathUserElected {
		t.Fatalf("deepest-position tie break must pick the longest branch, got %v elected=%v",
			elected.WakeupPath, elected.WakeupPathUserElected)
	}
	if elected.WakeupPathBranch != 8 {
		t.Fatalf("elected branch ordinal must publish on entity elections too, got %d", elected.WakeupPathBranch)
	}
}

// A pure-legacy pool (no branch-form candidate anywhere) stays byte-stable —
// the B1/B1-b files keep pinning that lane; this is the switch's negative arm.
func TestTraceCausalProjectionLegacyPoolStaysByteStable(t *testing.T) {
	legacy := anchorB1PathRecord("legacy-flat", "oney.hmn.berlin-42591",
		"VSyncGenerator-2270 -> oney.hmn.berlin-42591")
	got := TraceCausalProjectionFromObservationRecords([]ObservationRecord{legacy})
	if got.WakeupPathBranch != 0 || got.WakeupPathRootDepth != 0 {
		t.Fatalf("legacy pool must not claim a branch identity, got branch=%d rootDepth=%d",
			got.WakeupPathBranch, got.WakeupPathRootDepth)
	}
	if !reflect.DeepEqual(got.WakeupPath, []string{"VSyncGenerator-2270", "oney.hmn.berlin-42591"}) {
		t.Fatalf("legacy pool path changed: %v", got.WakeupPath)
	}
}

// 复核收尾③a: the four B1/B1-b election-law arms pinned INSIDE the branch
// pool (the code path is shared with the legacy lane today, but a future
// branch-arm fork must not drop a clause silently):
//
//	末次截断 / 位置0不可当选 / typed-Subject-hit > deeper position-only hit /
//	发布序终极 tie-break.
func TestTraceCausalProjectionBranchPoolElectionLawFourArmsP0E(t *testing.T) {
	// ① last-occurrence truncation: the entity appears twice mid-branch —
	// truncate at the LAST hit (earlier duplicate stays on the trunk as the
	// legitimate cycle shape).
	twice := branchP0EPathRecord("branch-3", "tail.thread-88",
		"up.thread-11 -> oney.hmn.berlin-42591 -> mid.thread-22 -> oney.hmn.berlin-42591 -> tail.thread-88", 3, 3)
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{twice}, []string{"42591"})
	want := []string{"up.thread-11", "oney.hmn.berlin-42591", "mid.thread-22", "oney.hmn.berlin-42591"}
	if !reflect.DeepEqual(got.WakeupPath, want) || !got.WakeupPathUserElected {
		t.Fatalf("①末次截断: got %v elected=%v", got.WakeupPath, got.WakeupPathUserElected)
	}
	if got.WakeupPathRootDepth != 1 {
		t.Fatalf("①rootDepth = len-1-pos = 5-1-3 = 1, got %d", got.WakeupPathRootDepth)
	}

	// ② position-0 hits are not electable: the sole branch candidate has the
	// entity at index 0 only — legacy fallback keeps the full path unelected.
	posZero := branchP0EPathRecord("branch-1", "tail.thread-88",
		"oney.hmn.berlin-42591 -> tail.thread-88", 1, 1)
	got = TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{posZero}, []string{"42591"})
	if got.WakeupPathUserElected || len(got.WakeupPath) != 2 {
		t.Fatalf("②位置0不可当选: got %v elected=%v", got.WakeupPath, got.WakeupPathUserElected)
	}

	// ③ typed Subject hit outranks a DEEPER position-only hit: candidate A's
	// chain target IS the entity (subject match, pos 1); candidate B carries
	// the entity deeper as a transit of another thread's chain.
	subjectHit := branchP0EPathRecord("branch-1", "oney.hmn.berlin-42591",
		"waker.thread-33 -> oney.hmn.berlin-42591", 1, 2)
	positionHit := branchP0EPathRecord("branch-2", "other.thread-99",
		"a.thread-1 -> b.thread-2 -> oney.hmn.berlin-42591 -> other.thread-99", 2, 2)
	got = TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{positionHit, subjectHit}, []string{"42591"})
	if !reflect.DeepEqual(got.WakeupPath, []string{"waker.thread-33", "oney.hmn.berlin-42591"}) ||
		got.WakeupPathBranch != 1 {
		t.Fatalf("③Subject>位置: got %v branch=%d", got.WakeupPath, got.WakeupPathBranch)
	}

	// ④ publication order is the terminal tie-break: two subject-matching
	// candidates with equal hit position — the FIRST published wins.
	first := branchP0EPathRecord("branch-4", "oney.hmn.berlin-42591",
		"waker.a-44 -> oney.hmn.berlin-42591", 4, 5)
	second := branchP0EPathRecord("branch-5", "oney.hmn.berlin-42591",
		"waker.b-55 -> oney.hmn.berlin-42591", 5, 5)
	got = TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{first, second}, []string{"42591"})
	if got.WakeupPathBranch != 4 {
		t.Fatalf("④发布序终极 tie: first-published branch 4 must win, got branch=%d path=%v",
			got.WakeupPathBranch, got.WakeupPath)
	}
}

// Truncated mid-branch election: the user entity sits mid-path as a transit
// of a longer chain — the elected path is cut at the entity (B1-b law) and
// WakeupPathRootDepth carries the engine depth of the new END element, so the
// display tree can re-base its (branch, depth) attach onto the truncated
// trunk instead of pairing full-chain depths with a shorter trunk.
func TestTraceCausalProjectionTruncatedElectionCarriesRootDepth(t *testing.T) {
	// Full branch: depths root←3,2,1,0→target; the entity 42591 sits at
	// engine depth 2 (index 1) — truncation keeps indexes 0..1.
	record := branchP0EPathRecord("branch-2", "other.thread-77",
		"gpu-pm-release-327 -> oney.hmn.berlin-42591 -> RSUniRenderThre-1963 -> other.thread-77", 2, 3)
	got := TraceCausalProjectionFromObservationRecordsForUserEntities(
		[]ObservationRecord{record}, []string{"42591"})
	want := []string{"gpu-pm-release-327", "oney.hmn.berlin-42591"}
	if !reflect.DeepEqual(got.WakeupPath, want) || !got.WakeupPathUserElected {
		t.Fatalf("mid-branch entity must elect+truncate, got %v elected=%v", got.WakeupPath, got.WakeupPathUserElected)
	}
	if got.WakeupPathBranch != 2 {
		t.Fatalf("elected branch ordinal wanted 2, got %d", got.WakeupPathBranch)
	}
	if got.WakeupPathRootDepth != 2 {
		t.Fatalf("rootDepth must be the engine depth of the truncated END (len-1-pos = 4-1-1 = 2), got %d",
			got.WakeupPathRootDepth)
	}
}
