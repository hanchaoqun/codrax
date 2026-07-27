package tracequery

import (
	"fmt"
	"testing"
)

// next_info_epoch_seam_test.go — §15.12 批甲 EPOCH-SEAM (2026-07-26): the
// B↔E2 seam defects. next_info is authoritative kernel fact (R-AUTH): adding
// a mask-silent binding witness must never weaken the mask proof; event
// order must never decide the hard gate; a legal time-unbounded query must
// not zero out binding-lane attribution; and the big-core tier-exclusion
// pair (绑核排除大核 → 算力供给影响, user ruling 2026-07-26) must survive the
// epoch overlay onto the cause faces.

func epochSeamSeat(t *testing.T, name, content string, q Query) *CPUConstraintSummary {
	t.Helper()
	idx := buildTraceIndex(t, name, content)
	stats := ComputeWindowStats(idx, q)
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			return &stats.CPUConstraints[i]
		}
	}
	t.Fatalf("expected a constraint seat for app-100: %+v", stats.CPUConstraints)
	return nil
}

// SEAM-1: a name-only cpuset_attach (no mask — the common HarmonyOS shape)
// is a mask-SILENT witness, not a mask conflict. The kernel next_info mask,
// its exclusion proof, and the restriction verdict must survive intact —
// two sources together must never be weaker than one.
func TestEpochSeamMaskSilentBindingDoesNotPoisonUniformity(t *testing.T) {
	trace := `
       idle/4-0   (    0) [004] .... 1.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle/4 next_pid=0 next_prio=120
       ctrl-300   (  900) [001] .... 1.000500: cpuset_attach: comm=app pid=100 cpuset=top-app
       ctrl-300   (  900) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.012, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	seat := epochSeamSeat(t, "epoch_seam_masksilent.systrace", trace, q)
	if len(seat.AllowedCPUs) == 0 || len(seat.ExcludedCPUs) == 0 {
		t.Fatalf("mask-silent binding must not wipe the kernel mask payload: %+v", seat)
	}
	if seat.RestrictionProof != CPUConstraintRestrictionProofAllowedMaskExcludesUniverse {
		t.Fatalf("mask proof must survive a mask-silent binding witness: %+v", seat)
	}
	if !cpuConstraintRestrictsExecution(*seat) {
		t.Fatalf("two sources together must not be weaker than next_info alone: %+v", seat)
	}
}

// SEAM-2: identical evidence in either order must yield the identical proof.
// A real binding event's proof survives even when the next_info epoch's mask
// covers the whole universe (no exclusion proof of its own).
func TestEpochSeamProofMergeOrderIndependent(t *testing.T) {
	nextInfoFirst := `
        app-100   (  100) [000] .... 1.005000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app
       ctrl-300   (  900) [001] .... 1.006000: sched_setaffinity: comm=app pid=100 mask=0x3 cpuset=top-app target_cpu=0 policy=bind
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`
	bindingFirst := `
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x3 cpuset=top-app target_cpu=0 policy=bind
        app-100   (  100) [000] .... 1.005000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`
	// Universe is exactly {0,1} (only CPUs 0/1 appear anywhere), so the mask
	// 0x3 covers it — no exclusion proof; the binding event is the only
	// restriction proof either way.
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.012, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	first := epochSeamSeat(t, "epoch_seam_order_a.systrace", nextInfoFirst, q)
	second := epochSeamSeat(t, "epoch_seam_order_b.systrace", bindingFirst, q)
	if first.RestrictionProof != CPUConstraintRestrictionProofBindingEvent ||
		second.RestrictionProof != CPUConstraintRestrictionProofBindingEvent {
		t.Fatalf("binding proof must survive in BOTH orders: next_info-first=%q binding-first=%q",
			first.RestrictionProof, second.RestrictionProof)
	}
	if cpuConstraintRestrictsExecution(*first) != cpuConstraintRestrictsExecution(*second) {
		t.Fatalf("event order must never decide the hard gate: %+v vs %+v", first, second)
	}
	// The contradictory state (binding provenance without any proof) must
	// not be constructible from this evidence set.
	if first.CPUSetIsBinding && first.RestrictionProof == "" {
		t.Fatalf("IsBinding=true with empty proof is the forbidden dead-weight state: %+v", first)
	}
}

// SEAM-3: a legal time-unbounded query (line window only — a shape the tool
// face explicitly teaches) must not zero out the binding lane's restricted
// runnable attribution.
func TestEpochSeamTimeUnboundedBindingStillAttributes(t *testing.T) {
	trace := `
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x1 cpuset=bg target_cpu=0 policy=bind
       ctrl-300   (  900) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52
        app-100   (  100) [000] .... 1.013000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
       idle/4-0   (    0) [004] .... 1.014000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle/4 next_pid=0 next_prio=120
	`
	bounded := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.014, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	unbounded := Query{PID: 100, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	seatBounded := epochSeamSeat(t, "epoch_seam_unbounded_a.systrace", trace, bounded)
	seatUnbounded := epochSeamSeat(t, "epoch_seam_unbounded_b.systrace", trace, unbounded)
	if seatBounded.RestrictedRunnableWaitMs <= 0 {
		t.Fatalf("bounded baseline must attribute the binding-lane runnable: %+v", seatBounded)
	}
	if seatUnbounded.RestrictedRunnableWaitMs <= 0 {
		t.Fatalf("time-unbounded query must not zero the binding-lane attribution (trailing epoch must close at the last scanned event): %+v", seatUnbounded)
	}
}

// SEAM-4: AllowedCPUsAuthority answers "who published the MASK" — a
// mask-silent witness must not stamp mixed_precise.
func TestEpochSeamAuthorityReflectsMaskPublishers(t *testing.T) {
	trace := `
       ctrl-300   (  900) [001] .... 1.000500: cpuset_attach: comm=app pid=100 cpuset=top-app
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.012, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	seat := epochSeamSeat(t, "epoch_seam_authority.systrace", trace, q)
	if seat.AllowedCPUsAuthority != CPUConstraintAllowedCPUsAuthorityKernelNextInfo {
		t.Fatalf("mask authority must name the mask publisher only (kernel next_info), got %q: %+v",
			seat.AllowedCPUsAuthority, seat)
	}
}

// USER RULING (2026-07-26): a cpuset binding whose mask excludes the bigger
// core tier is a COMPUTE-SUPPLY restriction — the tier-exclusion pair must
// survive the epoch overlay so the cause face can speak 绑核排除更大核档 (the
// render word is pinned by the r10wire word-face tests; this pins the typed
// pair riding the overlay's mask payload — including past a mask-silent
// binding witness sitting at epochs[0]).
func TestEpochSeamTierExclusionSurvivesMultiEpoch(t *testing.T) {
	maskEpoch := func(load int32) CPUConstraintEpoch {
		return CPUConstraintEpoch{
			SourceAuthority:    CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
			RestrictionProof:   CPUConstraintRestrictionProofAllowedMaskExcludesUniverse,
			AllowedCPUs:        []int{0, 1},
			ExcludedCPUs:       []int{4},
			AllowedCoreClasses: []string{"small"},
			AllowedMaxTierKHz:  900000,
			GlobalMaxTierKHz:   2400000,
			RawNextInfo:        "3,4,1,0,0",
			Load:               load, LoadKnown: true,
		}
	}
	bindingEpoch := CPUConstraintEpoch{
		SourceAuthority:  CPUConstraintAllowedCPUsAuthorityConstraintEvent,
		RestrictionProof: CPUConstraintRestrictionProofBindingEvent,
		CPUSet:           "top-app", CPUSetIsBinding: true,
	}
	account := cpuConstraintEpochAccounting{
		epochs:                   []CPUConstraintEpoch{bindingEpoch, maskEpoch(4), maskEpoch(5)},
		total:                    3,
		restrictionEpochCount:    3,
		restrictedRunnableWaitMs: 12,
		sourceAuthority:          CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
	}
	account.allowedUniform = cpuConstraintEpochAllowedSetsUniform(account.epochs)
	if !account.allowedUniform {
		t.Fatalf("mask-silent binding epoch must not break uniformity")
	}
	epochSeamMintMergeInputs(&account)
	item := CPUConstraintSummary{Thread: ThreadRef{Comm: "app", PID: 100}}
	applyCPUConstraintEpochOverlay(&item, account)
	// The supply-capability pair rides the mask payload to the top level —
	// even with a mask-silent binding witness at epochs[0].
	if item.AllowedMaxTierKHz != 900000 || item.GlobalMaxTierKHz != 2400000 {
		t.Fatalf("tier-exclusion pair (绑核排除大核 → 算力供给) must survive the epoch overlay: %+v", item)
	}
	if len(item.AllowedCPUs) != 2 || len(item.ExcludedCPUs) != 1 {
		t.Fatalf("mask payload must come from the first mask-bearing epoch: %+v", item)
	}
	// Uniform mask + mask proof: the merged proof is the exclusion proof.
	if item.RestrictionProof != CPUConstraintRestrictionProofAllowedMaskExcludesUniverse {
		t.Fatalf("uniform mask-exclusion proof must own the merge: %+v", item)
	}
	// Changing masks withdraw the pair with the rest of the payload (§15.10)
	// — no pseudo-simultaneous supply claim.
	changed := maskEpoch(4)
	changed.AllowedCPUs = []int{4}
	changedAccount := account
	changedAccount.epochs = []CPUConstraintEpoch{bindingEpoch, maskEpoch(4), changed}
	changedAccount.allowedUniform = cpuConstraintEpochAllowedSetsUniform(changedAccount.epochs)
	epochSeamMintMergeInputs(&changedAccount)
	item2 := CPUConstraintSummary{Thread: ThreadRef{Comm: "app", PID: 100}}
	applyCPUConstraintEpochOverlay(&item2, changedAccount)
	if item2.AllowedMaxTierKHz != 0 || item2.GlobalMaxTierKHz != 0 || len(item2.AllowedCPUs) != 0 {
		t.Fatalf("changing masks must withdraw the whole payload including the tier pair: %+v", item2)
	}
	if item2.RestrictionProof != CPUConstraintRestrictionProofBindingEvent {
		t.Fatalf("the binding event's proof survives mask history: %+v", item2)
	}
}

// epochSeamMintMergeInputs mirrors the accounting mint for hand-built
// rosters: the overlay reads the FULL-roster merge results carried on the
// account (V-2: the display cap never caps accounting).
func epochSeamMintMergeInputs(account *cpuConstraintEpochAccounting) {
	account.mergedProof = cpuConstraintEpochMergedRestrictionProof(account.epochs, account.allowedUniform, account.restrictedRunnableWaitMs)
	if owner := cpuConstraintEpochFirstMaskBearing(account.epochs); owner != nil {
		ownerCopy := *owner
		account.maskOwner = &ownerCopy
	}
}

// V-1 (§15.12 批甲 verify): binding-ness is censused from the TYPED bit —
// a mask-bearing binding event legitimately mints mask_excludes as its
// per-epoch proof, but its binding evidence must survive changing masks
// (two sources must never be weaker than the binding source alone).
func TestEpochSeamMaskBearingBindingSurvivesChangingMasks(t *testing.T) {
	bindingWithMask := CPUConstraintEpoch{
		SourceAuthority:  CPUConstraintAllowedCPUsAuthorityConstraintEvent,
		RestrictionProof: CPUConstraintRestrictionProofAllowedMaskExcludesUniverse,
		AllowedCPUs:      []int{0},
		ExcludedCPUs:     []int{1, 4},
		CPUSet:           "bg", CPUSetIsBinding: true,
	}
	nextInfo := CPUConstraintEpoch{
		SourceAuthority:  CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
		RestrictionProof: CPUConstraintRestrictionProofAllowedMaskExcludesUniverse,
		AllowedCPUs:      []int{0, 1},
		ExcludedCPUs:     []int{4},
	}
	epochs := []CPUConstraintEpoch{nextInfo, bindingWithMask}
	uniform := cpuConstraintEpochAllowedSetsUniform(epochs)
	if uniform {
		t.Fatal("fixture must be a changing-mask roster")
	}
	// Zero runnable overlap: the mask proofs withdraw to nothing under
	// changing masks, but the BINDING evidence is mask-independent.
	if got := cpuConstraintEpochMergedRestrictionProof(epochs, uniform, 0); got != CPUConstraintRestrictionProofBindingEvent {
		t.Fatalf("a mask-bearing binding event must survive changing masks: %q", got)
	}
}

// V-2 (§15.12 批甲 verify): the merged proof and mask owner are computed on
// the FULL roster before the display cap — a binding event arriving past the
// 16-epoch cap must not vanish from the hard-gate census.
func TestEpochSeamMergeInputsComputedBeforeDisplayCap(t *testing.T) {
	intern := newStringInterner()
	var events []Event
	line := 1
	addLine := func(text string) {
		ev, ok := ParseLine(line, text, intern)
		if !ok {
			t.Fatalf("fixture line %d must parse: %s", line, text)
		}
		events = append(events, ev)
		line++
	}
	for i := 0; i < 17; i++ {
		// 17 distinct-load universe-covering next_info snapshots — each raw
		// change mints its own versioned epoch.
		addLine(fmt.Sprintf(`       idle/0-0   (    0) [000] .... 1.%03d000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3,%d,1,0,0 cg=top-app`, i, i+1))
	}
	addLine(`       ctrl-300   (  900) [001] .... 1.020000: cpuset_attach: comm=app pid=100 cpuset=top-app`)
	indexes := make([]int, len(events))
	for i := range indexes {
		indexes[i] = i
	}
	account := computeCPUConstraintEpochAccounting(
		events, indexes,
		Query{TimeStart: 1, TimeEnd: 1.05},
		1.05, nil,
		map[int]bool{0: true, 1: true},
		map[int]string{0: "small", 1: "small"},
		coreCapabilityMap{}, nil,
	)[100]
	if account.total != 18 || len(account.epochs) != cpuConstraintEpochDisplayCap {
		t.Fatalf("fixture must exceed the display cap: total=%d emitted=%d", account.total, len(account.epochs))
	}
	if account.mergedProof != CPUConstraintRestrictionProofBindingEvent {
		t.Fatalf("the binding event past the display cap must stay in the hard-gate census: %q", account.mergedProof)
	}
}

// V-3 (§15.12 批甲 verify): on a time-unbounded query the trailing epoch
// closes at the last EXAMINED event ts — never the whole-trace end across a
// region the query filtered out (an unseen mask change may sit there).
func TestEpochSeamTrailingEpochNeverClaimsBeyondExaminedWindow(t *testing.T) {
	trace := `
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x1 cpuset=bg target_cpu=0 policy=bind
       ctrl-300   (  900) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52
        app-100   (  100) [000] .... 1.013000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
       idle/4-0   (    0) [004] .... 9.500000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle/4 next_pid=0 next_prio=120
	`
	q := Query{PID: 100, LineStart: 1, LineEnd: 4, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	seat := epochSeamSeat(t, "epoch_seam_examined.systrace", trace, q)
	if len(seat.Epochs) == 0 {
		t.Fatalf("expected an epoch roster: %+v", seat)
	}
	last := seat.Epochs[len(seat.Epochs)-1]
	if last.EndTs > 1.013001 {
		t.Fatalf("trailing epoch must not claim persistence beyond the examined line window (unexamined 9.5s region): %+v", last)
	}
	if seat.RestrictedRunnableWaitMs <= 0 {
		t.Fatalf("the examined-window close must still attribute the binding-lane runnable: %+v", seat)
	}
}
