package tracequery

// tex_tokenizer_verification_test.go — TEX 批 tokenizer 四验证 (ledger
// docs/design/real_trace_campaign_20260705.md §28.2 T2/T3/T4 + §28.6 ④,
// 2026-07-09), pinned on the customer specimen's verbatim line shapes
// (trace_texture_upload.txt):
//
//   T3   trace-mark form coverage: pure B|pid|name (no H: prefix, no |I tail,
//        name with spaces/parentheses/DIGITSxDIGITS), bare E|pid close,
//        E|pid|I39 payload-tagged close (pairs with its tagged begin).
//   T4   prio=301 (dh-irq-bind class): Donghu/Harmony priority semantics
//        classify it system_or_kernel — NEVER ohos_rt/ohos_cfs — so the R5g
//        below-RT-preempted disclosure and the inversion relation stay
//        conservative (no fake comparable-priority inversion candidates).
//   §28.6④ hmfs_ prefix joins the filesystem-event family (HarmonyOS FS layer,
//        f2fs-isomorphic kv shape reuses populateFileIOFields verbatim).
//   HYG  clampString byte cuts go through the shared rune-safe primitive
//        (no manufactured broken rune at a CJK cut point; ASCII byte-identical).

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- T3: trace-mark form coverage ---------------------------------------------

func TestT3ParseTraceMarkSpecimenForms(t *testing.T) {
	// Pure B|pid|name — no H: prefix, no |I tail; the name carries spaces,
	// parentheses and a DIGITSxDIGITS size suffix (specimen line 8).
	action, spanPID, name, _ := parseTraceMark("B|18998|Texture upload(15283) 52x50")
	if action != "B" || spanPID != 18998 || name != "Texture upload(15283) 52x50" {
		t.Fatalf("B|pid|name form drifted: action=%q pid=%d name=%q", action, spanPID, name)
	}
	// Name with colon, commas and spaces (specimen line 1).
	action, spanPID, name, _ = parseTraceMark("B|18998|GraphicsLoad: 0, 0, 0, 0, 0, 1, 0")
	if action != "B" || spanPID != 18998 || name != "GraphicsLoad: 0, 0, 0, 0, 0, 1, 0" {
		t.Fatalf("B|pid|name (punctuated name) form drifted: action=%q pid=%d name=%q", action, spanPID, name)
	}
	// Bare E|pid close (specimen line 2).
	action, spanPID, _, _ = parseTraceMark("E|18998")
	if action != "E" || spanPID != 18998 {
		t.Fatalf("bare E|pid form drifted: action=%q pid=%d", action, spanPID)
	}
	// E|pid|I39 payload-tagged close (specimen line 37).
	action, spanPID, _, _ = parseTraceMark("E|6855|I39")
	if action != "E" || spanPID != 6855 {
		t.Fatalf("E|pid|Itag form drifted: action=%q pid=%d", action, spanPID)
	}
}

func TestT3TaggedEndClosesTaggedBeginOnEmittingThread(t *testing.T) {
	// Specimen lines 15/37: B|6855|H:NotifyLooperIdleStart::PostIdleCheckTask|I39
	// closed by E|6855|I39 on the same emitting thread — the |I39 tail on the
	// End row must not break the per-thread B/E pairing.
	idx := buildTraceIndex(t, "tagged_pair.systrace", texUploadSpecimenTrace)
	res := Run(idx, Query{View: "window_stats", PID: 6855, TimeStart: 15151.960530, TimeEnd: 15151.960810})
	if res.WindowStats == nil {
		t.Fatalf("window_stats must build")
	}
	found := false
	for _, span := range res.WindowStats.TraceSpans {
		if span.Name == "H:NotifyLooperIdleStart::PostIdleCheckTask" {
			found = true
			if span.Thread.PID != 6855 {
				t.Fatalf("tagged pair must attach to the emitting thread 6855: %+v", span.Thread)
			}
			if span.DurationMs <= 0 {
				t.Fatalf("tagged pair must mint a positive-duration span: %+v", span)
			}
		}
	}
	if !found {
		t.Fatalf("E|6855|I39 must close B|6855|H:…|I39 into a span: %+v", res.WindowStats.TraceSpans)
	}
}

// --- T4: prio=301 stays out of the comparable-priority lanes -------------------

func TestT4Prio301ClassifiesSystemOrKernelNeverRT(t *testing.T) {
	// dh-irq-bind threads carry prio=301 (specimen lines 3/14/25/…): outside
	// the documented HarmonyOS user-space 1-40 CFS / 41-159 RT bands.
	if got := classifyTracePriority(TraceFlavorHarmonyHitrace, 301); got != "system_or_kernel" {
		t.Fatalf("prio=301 must classify system_or_kernel under Harmony semantics, got %q", got)
	}
	// Band edges stay pinned so the 301 verdict is the >159 arm, not an
	// accidental band shift.
	if got := classifyTracePriority(TraceFlavorHarmonyHitrace, 139); got != "ohos_rt" {
		t.Fatalf("prio=139 must stay ohos_rt, got %q", got)
	}
	for _, prio := range []int{140, 142, 157, 159} {
		if got := classifyTracePriority(TraceFlavorHarmonyHitrace, prio); got != "ohos_rt" {
			t.Fatalf("microkernel prio=%d must stay ohos_rt, got %q", prio, got)
		}
	}
	for _, prio := range []int{160, 301} {
		if got := classifyTracePriority(TraceFlavorHarmonyHitrace, prio); got != "system_or_kernel" {
			t.Fatalf("raw prio=%d must classify system_or_kernel, got %q", prio, got)
		}
	}
}

func TestT4Prio301CompetitorNeverMintsBelowRTPreempted(t *testing.T) {
	// R5g / SYM-2 R2 disclosure gate: the competitor arm requires the literal
	// ohos_rt class — a prio=301 system_or_kernel competitor (dh-irq-bind
	// shape) must NOT stamp the 「优先级低于RT」 disclosure (cross-scale
	// priorities are not comparable RT evidence).
	target := ThreadRef{Comm: "app", PID: 100}
	mkItems := func() []RootCauseRankItem {
		return []RootCauseRankItem{{
			Type: "runnable_wait", Thread: target, SubjectIsAnalysisTarget: true,
			runnableCPU: 2, runnableCPUKnown: true,
		}}
	}
	contexts := []RunnableContextSummary{{
		Thread: target, CPU: 2, PriorityClass: "ohos_cfs",
		SameCPUTopRunning: []ThreadDuration{{
			Thread: ThreadRef{Comm: "dh-irq-bind-3", PID: 102}, DurationMs: 3,
			Priority: 301, PriorityClass: "system_or_kernel",
		}},
	}}
	items := mkItems()
	stampRunnableSelfBelowRTPreempted(items, contexts)
	if items[0].RunnableBelowRTPreempted {
		t.Fatalf("system_or_kernel (prio=301) competitor must not mint the below-RT disclosure: %+v", items[0])
	}
	// Positive control (guards the pin from vacuity): a genuine ohos_rt
	// competitor on the same shape does stamp.
	contexts[0].SameCPUTopRunning[0].PriorityClass = "ohos_rt"
	items = mkItems()
	stampRunnableSelfBelowRTPreempted(items, contexts)
	if !items[0].RunnableBelowRTPreempted {
		t.Fatalf("ohos_rt competitor control must stamp the disclosure: %+v", items[0])
	}
}

func TestT4Prio301DependencyNeverReadsAsLowerPriority(t *testing.T) {
	// The inversion candidate gate requires relation=lower_priority_dependency.
	// prio=301 is outside the documented Harmony userspace bands. It is a raw
	// system/kernel token, not a numerically comparable user priority, so it can
	// never seed either a lower- or higher-priority dependency claim.
	if got := dependencyPriorityRelation(TraceFlavorHarmonyHitrace, 40, 301, 1); got != "raw_priority_uninterpreted" {
		t.Fatalf("prio=301 dependency vs user-band target must stay uninterpreted, got %q", got)
	}
	// F4 (复核收尾, 2026-07-09) positive control — the lower arm still fires
	// for genuine user-band pairs (dependency 10 below target 40), so the 301
	// verdict above is cross-scale conservatism, not a dead predicate.
	if got := dependencyPriorityRelation(TraceFlavorHarmonyHitrace, 40, 10, 1); got != "lower_priority_dependency" {
		t.Fatalf("user-band lower-priority dependency must read lower_priority_dependency, got %q", got)
	}
	// Non-Harmony flavors stay uninterpreted — no band mapping is guessed.
	if got := dependencyPriorityRelation(TraceFlavorAndroidAtrace, 120, 301, 1); got != "raw_priority_uninterpreted" {
		t.Fatalf("non-Harmony flavors must not interpret raw priorities, got %q", got)
	}
}

func TestT4PriorityRelationRequiresTwoComparableHarmonyUserBands(t *testing.T) {
	for _, tc := range []struct {
		name               string
		wakee, waker       int
		target, dependency int
	}{
		{name: "raw waker", wakee: 53, waker: 301, target: 53, dependency: 301},
		{name: "raw wakee", wakee: 301, waker: 20, target: 301, dependency: 20},
		{name: "both raw", wakee: 301, waker: 160, target: 301, dependency: 160},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := priorityRelation(TraceFlavorHarmonyHitrace, tc.wakee, tc.waker); got != "raw_priority_uninterpreted" {
				t.Fatalf("raw/system wake edge minted relation %q", got)
			}
			if got := dependencyPriorityRelation(TraceFlavorHarmonyHitrace, tc.target, tc.dependency, 1); got != "raw_priority_uninterpreted" {
				t.Fatalf("raw/system dependency minted relation %q", got)
			}
		})
	}

	if got := priorityRelation(TraceFlavorHarmonyHitrace, 53, 20); got != "lower_priority_waker" {
		t.Fatalf("comparable CFS->RT positive control lost: %q", got)
	}
	if got := priorityRelation(TraceFlavorHarmonyHitrace, 20, 53); got != "higher_priority_waker" {
		t.Fatalf("comparable RT->CFS positive control lost: %q", got)
	}
	if got := priorityRelation(TraceFlavorHarmonyHitrace, 159, 140); got != "lower_priority_waker" {
		t.Fatalf("microkernel 140->159 wake relation must remain comparable, got %q", got)
	}
	if got := dependencyPriorityRelation(TraceFlavorHarmonyHitrace, 159, 140, 1); got != "lower_priority_dependency" {
		t.Fatalf("microkernel 140->159 dependency must remain inversion-comparable, got %q", got)
	}
}

// --- §28.6 ④: hmfs_ filesystem-event family ------------------------------------

func TestHmfsEventsClassifyAsFilesystemWithF2fsIsomorphicFields(t *testing.T) {
	if !isFilesystemEvent("hmfs_dataread_start") || !isFilesystemEvent("hmfs_write_end") {
		t.Fatalf("hmfs_ prefixed events must classify as filesystem events")
	}
	// f2fs-isomorphic kv shape (dev/ino/pos/bytes) reuses the generic
	// populateFileIOFields parse — pinned end-to-end through a real line.
	trace := `
        app-100 (100) [001] .... 5.000000: hmfs_dataread_start: dev = (253,6), ino = 148242, pos = 4096, bytes = 8192, pid = 100, pathname = /data/app/base.hap, command = app
        app-100 (100) [001] .... 5.000100: hmfs_dataread_end: dev = (253,6), ino = 148242, pos = 4096, bytes = 8192
`
	idx := buildTraceIndex(t, "hmfs.systrace", trace)
	var hmfs *Event
	for i := range idx.Events {
		if idx.Events[i].Name == "hmfs_dataread_start" {
			hmfs = &idx.Events[i]
			break
		}
	}
	if hmfs == nil {
		t.Fatalf("hmfs_dataread_start must be indexed: %+v", idx.Events)
	}
	if hmfs.Type != EventFilesystem {
		t.Fatalf("hmfs event must classify EventFilesystem, got %v", hmfs.Type)
	}
	if hmfs.SubsystemKind != "fs_hmfs" {
		t.Fatalf("hmfs event must keep its honest fs_hmfs subsystem word, got %q", hmfs.SubsystemKind)
	}
	if hmfs.FileFields == nil {
		t.Fatalf("hmfs event must populate FileFields via the shared kv parse")
	}
	if hmfs.FileFields.Ino != "148242" {
		t.Fatalf("hmfs ino kv must parse into FileFields.Ino, got %+v", hmfs.FileFields)
	}
}

// --- HYG: clampString rune safety ----------------------------------------------

func TestClampStringRuneSafeCJKCut(t *testing.T) {
	// ASCII behavior is byte-identical to the legacy s[:n-3]+"..." shape.
	if got := clampString("abcdefghij", 8); got != "abcde..." {
		t.Fatalf("ASCII clamp drifted: %q", got)
	}
	if got := clampString("short", 8); got != "short" {
		t.Fatalf("under-budget strings pass through: %q", got)
	}
	// A budget landing inside a CJK rune must back off to the rune boundary —
	// never emit a broken tail (the U+FFFD mojibake class).
	got := clampString("纹理上传纹理上传", 8) // 8 bytes = 2.67 CJK runes
	if !utf8.ValidString(got) {
		t.Fatalf("clampString manufactured a broken rune: %q", got)
	}
	if !strings.HasSuffix(got, "...") || !strings.HasPrefix(got, "纹") {
		t.Fatalf("CJK clamp shape drifted: %q", got)
	}
	// Tiny budgets (n<=3, no ellipsis) stay rune-safe too.
	if got := clampString("纹理", 2); got != "" || !utf8.ValidString(got) {
		t.Fatalf("n<=3 arm must cut rune-safely (2 bytes < one CJK rune → empty), got %q", got)
	}
}
