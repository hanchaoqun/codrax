package tracequery

// texture_upload_tex_test.go — TEX 批 engine pins (ledger
// docs/design/real_trace_campaign_20260705.md §28.1/§28.2, user ruling
// 2026-07-09): "Texture upload" is the FIFTH semantic span class with exactly
// the same treatment as VerifyClass/Shader/JIT —
//
//   TEX-1  classifier: normalized-name prefix match ("texture upload", id and
//          WxH suffixes tolerated), conservative negatives ("upload texture",
//          "TextureCache", "texture uploads" stay out);
//   TEX-2  registry: texture_upload token on cpu_work|wall_clock_per_thread|
//          per_thread|row + the semantic_class family-fold lane (golden files
//          pin the exact rows);
//   TEX-3  equal treatment, verified per lane: on-chain rows participate in
//          the ordinary root-cause election;
//          non-chain rows join the background composite board (background_rank
//          mention gate); co-primary whitelist keeps excluding semantic types;
//   TEX-4  customer-specimen e2e (trace_texture_upload.txt verbatim, lines
//          8/34/53 size variants): classification hits, the family folds by
//          interval union of the Texture-upload spans THEMSELVES (nested
//          AllocPages/MapMemory/Alloc Ioctl/GraphicsLoad child spans are not
//          class members and never double-count), the dangling third variant
//          rides the incomplete-pair caveat;
//   T2     B|pid ≠ emitting-thread tgid: spans attach to the EMITTING thread's
//          timeline (tid 51342), never to the payload pid (18998) — the
//          payload pid survives as SpanPID identity only.

import (
	"strings"
	"testing"
)

// texUploadSpecimenTrace is the customer specimen VERBATIM
// (/Users/han/opt/customlogs/trace_texture_upload.txt, 2026-07-09): the
// RenderThread emits B|18998|Texture upload(15283) {52x50,512x67,512x194}
// with nested AllocPages/MapMemory/Alloc Ioctl/GraphicsLoad child spans while
// dh-irq-bind threads run at prio=301 and ohos.devicetest closes an
// E|6855|I39 tagged pair.
const texUploadSpecimenTrace = `    RenderThread-51342 (50820) [012] .... 15151.960538: print: B|18998|GraphicsLoad: 0, 0, 0, 0, 0, 1, 0
    RenderThread-51342 (50820) [012] .... 15151.960539: print: E|18998
 ohos.devicetest-6855  ( 6855) [006] .... 15151.960541: sched_switch: prev_comm=dh-irq-bind-6 prev_pid=105 prev_prio=301 prev_state=S ==> next_comm=ohos.devicetest next_pid=6855 next_prio=40 next_info=1fff,14,2,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960559: sched_wakeup: comm=gpu-pm-request pid=368 prio=100 target_cpu=001
  tppmgr-idle-11-307   (    2) [011] .... 15151.960560: cpu_idle: state=2 cpu_id=11
    RenderThread-51342 (50820) [012] .... 15151.960563: print: E|18998
   dh-irq-bind-4-103   (   72) [004] .... 15151.960563: sched_wakeup: comm=kworker/4:1 pid=28553 prio=20 target_cpu=004
    RenderThread-51342 (50820) [012] .... 15151.960565: print: B|18998|Texture upload(15283) 52x50
    RenderThread-51342 (50820) [012] .... 15151.960570: print: B|18998|AllocPages: 2
     wk:2/1/0/14-41149 (    2) [001] .... 15151.960570: sched_switch: prev_comm=wk:2/1/0/14 prev_pid=41149 prev_prio=51 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=-1 next_info=1,0,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960574: print: E|18998
  gpu-pm-request-368   (   72) [001] .... 15151.960576: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=-1 prev_state=R ==> next_comm=gpu-pm-request next_pid=368 next_prio=100 next_info=1fff,2,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960579: print: B|18998|MapMemory
     kworker/7:1-60238 (   72) [007] .... 15151.960590: sched_switch: prev_comm=dh-irq-bind-7 prev_pid=106 prev_prio=301 prev_state=S ==> next_comm=kworker/7:1 next_pid=60238 next_prio=20 next_info=80,0,0,0,0,0
 ohos.devicetest-6855  ( 6855) [006] .... 15151.960598: print: B|6855|H:NotifyLooperIdleStart::PostIdleCheckTask|I39
    RenderThread-51342 (50820) [012] .... 15151.960599: print: E|18998
 OS_IPC_22_57670-57670 ( 1075) [002] .... 15151.960601: clock_set_rate: thermal_inte1 state=2200000 cpu_id=2
   dh-irq-bind-3-102   (   72) [003] .... 15151.960603: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
   dh-irq-bind-3-102   (   72) [003] .... 15151.960605: clock_set_rate: m3_vote_delay state=701 cpu_id=0
   dh-irq-bind-3-102   (   72) [003] .... 15151.960606: clock_set_rate: m3_c1_freq state=417000 cpu_id=2
   dh-irq-bind-3-102   (   72) [003] .... 15151.960607: clock_set_rate: m3_c2_freq state=1200000 cpu_id=10
   dh-irq-bind-3-102   (   72) [003] .... 15151.960608: clock_set_rate: m3_c3_freq state=2750000 cpu_id=12
 OS_IPC_22_57670-57670 ( 1075) [002] .... 15151.960609: clock_set_rate: pid_task_freq state=5419 cpu_id=2
 OS_IPC_22_57670-57670 ( 1075) [002] .... 15151.960611: sched_switch: prev_comm=para_anon_recla prev_pid=15 prev_prio=20 prev_state=R+ ==> next_comm=OS_IPC_22_57670 next_pid=57670 next_prio=41 next_info=1fff,7,3,0,0,5
   binder:500_21-16047 (11808) [005] .... 15151.960612: sched_switch: prev_comm=dh-irq-bind-5 prev_pid=104 prev_prio=301 prev_state=S ==> next_comm=binder:500_21 next_pid=16047 next_prio=20 next_info=1fff,98,2,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960612: print: B|18998|Alloc Ioctl
        base.log-51140 (50820) [003] .... 15151.960618: sched_switch: prev_comm=dh-irq-bind-3 prev_pid=102 prev_prio=301 prev_state=S ==> next_comm=base.log next_pid=51140 next_prio=20 next_info=1fff,100,2,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960625: print: E|18998
 para_anon_recla-15    (    2) [009] .... 15151.960627: sched_switch: prev_comm=dh-irq-bind-9 prev_pid=108 prev_prio=301 prev_state=S ==> next_comm=para_anon_recla next_pid=15 next_prio=20 next_info=fff,130,0,0,0,0
     kworker/4:1-28553 (   72) [004] .... 15151.960633: sched_switch: prev_comm=dh-irq-bind-4 prev_pid=103 prev_prio=301 prev_state=S ==> next_comm=kworker/4:1 next_pid=28553 next_prio=20 next_info=10,0,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960633: print: B|18998|GraphicsLoad: 0, 0, 0, 0, 0, 1, 0
    RenderThread-51342 (50820) [012] .... 15151.960634: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960641: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960642: print: B|18998|Texture upload(15283) 512x67
    RenderThread-51342 (50820) [012] .... 15151.960645: print: B|18998|AllocPages: 1
    RenderThread-51342 (50820) [012] .... 15151.960647: print: E|18998
 ohos.devicetest-6855  ( 6855) [006] .... 15151.960653: print: E|6855|I39
    RenderThread-51342 (50820) [012] .... 15151.960654: print: B|18998|Alloc Ioctl
 sysmgr-reclaim0-9     (    2) [007] .... 15151.960654: sched_switch: prev_comm=kworker/7:1 prev_pid=60238 prev_prio=20 prev_state=S ==> next_comm=sysmgr-reclaim0 next_pid=9 next_prio=20 next_info=1fff,24,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960661: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960662: print: B|18998|MapMemory
 unyuan.app.chat-50820 (50820) [010] .... 15151.960674: sched_wakeup: comm=queued-work-loo pid=51001 prio=22 target_cpu=002
    RenderThread-51342 (50820) [012] .... 15151.960678: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960679: print: B|18998|MapMemory
    RenderThread-51342 (50820) [012] .... 15151.960694: print: E|18998
 queued-work-loo-51001 (50820) [006] .... 15151.960697: sched_switch: prev_comm=ohos.devicetest prev_pid=6855 prev_prio=40 prev_state=S ==> next_comm=queued-work-loo next_pid=51001 next_prio=22 next_info=1fff,99,2,0,0,0
      android.ui-11848 (11808) [004] .... 15151.960710: sched_switch: prev_comm=kworker/4:1 prev_pid=28553 prev_prio=20 prev_state=S ==> next_comm=android.ui next_pid=11848 next_prio=22 next_info=1fff,104,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960731: print: B|18998|Alloc Ioctl
    RenderThread-51342 (50820) [012] .... 15151.960745: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960752: print: B|18998|GraphicsLoad: 0, 0, 0, 0, 0, 1, 0
    RenderThread-51342 (50820) [012] .... 15151.960753: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960760: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960761: print: B|18998|Texture upload(15283) 512x194
    RenderThread-51342 (50820) [012] .... 15151.960765: print: B|18998|Alloc Ioctl
    RenderThread-51342 (50820) [012] .... 15151.960774: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960775: print: B|18998|MapMemory
 para_anon_recla-14    (    2) [006] .... 15151.960780: sched_switch: prev_comm=queued-work-loo prev_pid=51001 prev_prio=22 prev_state=S ==> next_comm=para_anon_recla next_pid=14 next_prio=20 next_info=fff,131,0,0,0,0
    RenderThread-51342 (50820) [012] .... 15151.960796: print: E|18998
    RenderThread-51342 (50820) [012] .... 15151.960797: print: B|18998|MapMemory
 OS_IPC_22_57670-57670 ( 1075) [002] .... 15151.960797: print: B|1075|H:DispatchResource PluginMgr ,resType[48],resTypeStr[RES_TYPE_ANCO_CUST],value[1009],pluginlist:[libsocperf_plugin.z.so,libcgroup_sched.z.so,]|I62
 OS_IPC_22_57670-57670 ( 1075) [002] .... 15151.960803: print: E|1075|I62
    RenderThread-51342 (50820) [012] .... 15151.960808: print: E|18998`

// --- TEX-1: classifier -------------------------------------------------------

func TestTEXTextureUploadClassifierMatchesCustomerShapes(t *testing.T) {
	for _, name := range []string{
		// The three verbatim size variants (specimen lines 8/34/53).
		"Texture upload(15283) 52x50",
		"Texture upload(15283) 512x67",
		"Texture upload(15283) 512x194",
		// Suffix-free and separator drift within the ruled prefix shape.
		"Texture upload",
		"texture upload(7)",
		"Texture_upload(15283) 512x194",
		"TextureUpload(1) 8x8",
		// F1 (复核收尾, 2026-07-09): the hitrace "H:" user-space span prefix
		// must not defeat the prefix anchor — dual-stack platforms (§28.5 T11)
		// emit the same class under H:, and the four substring-matched
		// classmates already pass through it (完全同待遇).
		"H:Texture upload(1) 2x2",
		"h:texture upload(15283) 512x194",
	} {
		work, ok := traceSpanSemanticWorkClass(name)
		if !ok {
			t.Fatalf("%q must classify as texture_upload", name)
		}
		if work.SemanticClass != "texture_upload" || work.RootCauseType != "texture_upload" {
			t.Fatalf("%q classified into the wrong class: %+v", name, work)
		}
		if work.Label != "Texture upload" {
			t.Fatalf("class label must stay the English proper term (§22.2.1 专名尺子), got %q", work.Label)
		}
	}
}

func TestTEXTextureUploadClassifierConservativeNegatives(t *testing.T) {
	for _, name := range []string{
		// Word order is load-bearing: reversed phrasing never matches.
		"upload texture",
		"UploadTexture(15283) 512x194",
		// Vocabulary neighbours without the exact phrase.
		"TextureCache",
		"TextureCache flush",
		"GLTextureUploader",
		// Boundary: the char after the prefix must not be alphanumeric.
		"texture uploads pending",
		"texture uploader",
		"texture_upload2",
		// F1: the h: strip is single-point — the boundary rule still applies
		// to the stripped name, and a bare "H:" carries no class.
		"H:texture uploads pending",
		"H:",
		// Specimen sibling spans (nested children of the texture upload).
		"AllocPages: 2",
		"MapMemory",
		"Alloc Ioctl",
		"GraphicsLoad: 0, 0, 0, 0, 0, 1, 0",
	} {
		if _, ok := traceSpanSemanticWorkClass(name); ok {
			t.Fatalf("%q must NOT classify as a semantic span class", name)
		}
	}
}

func TestTEXTextureUploadNearMissAdvisoryOnly(t *testing.T) {
	// Drifted namings mention both words but miss the strict prefix — they
	// must surface on the ADVISORY near-miss lane (caveat wording), never as a
	// classified candidate.
	for _, name := range []string{"UploadTexture(15283) 512x194", "GLES texture async upload"} {
		if _, ok := traceSpanSemanticWorkClass(name); ok {
			t.Fatalf("%q must not classify", name)
		}
		if !traceSpanNearMissesSemanticWorkClassification(name) {
			t.Fatalf("%q must trip the texture+upload near-miss advisory", name)
		}
	}
	// One word alone is not a near-miss (no false drift alarms on every
	// texture-ish or upload-ish span).
	for _, name := range []string{"TextureCache flush", "upload metrics batch"} {
		if traceSpanNearMissesSemanticWorkClassification(name) {
			t.Fatalf("%q must not trip the near-miss advisory", name)
		}
	}
}

// F1 (复核收尾, 2026-07-09): the h: strip is scoped to the texture matcher —
// the four classmates keep their own arms byte-identical, and an H:-prefixed
// classmate name still classifies through ITS substring arm (never through
// the texture arm).
func TestTEXHitracePrefixStripDoesNotTouchClassmateArms(t *testing.T) {
	work, ok := traceSpanSemanticWorkClass("H:VerifyClass com.example.Foo")
	if !ok || work.SemanticClass != "class_verification" {
		t.Fatalf("H:VerifyClass must keep classifying via the class_verification arm: %+v ok=%v", work, ok)
	}
	work, ok = traceSpanSemanticWorkClass("H:JITCompileTask")
	if !ok || work.SemanticClass != "jit_compile" {
		t.Fatalf("H:JITCompileTask must keep classifying via the jit_compile arm: %+v ok=%v", work, ok)
	}
}

// --- TEX-2: registry ---------------------------------------------------------

func TestTEXTextureUploadRegistryLaneMirrorsSemanticClassmates(t *testing.T) {
	spec, ok := CausalTokenSpecFor("texture_upload")
	if !ok {
		t.Fatalf("texture_upload must be a registered causal token")
	}
	reference, _ := CausalTokenSpecFor("class_verification")
	if spec.Lane != reference.Lane || spec.Additivity != reference.Additivity ||
		spec.Subject != reference.Subject || spec.RowToken != reference.RowToken {
		t.Fatalf("texture_upload must ride the same registry shape as the other semantic classes: got %+v want %+v", spec, reference)
	}
	if CausalTokenFamilyFoldLane("texture_upload") != CausalFamilyFoldSemanticClass {
		t.Fatalf("texture_upload must family-fold on the semantic_class lane")
	}
}

// --- TEX-3: equal treatment on the rank lanes ---------------------------------

func TestTEXOnChainTextureUploadParticipatesInRootCauseElection(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "texture_upload", ImpactMs: 5, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		{Type: "runnable_wait", ImpactMs: 9, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
	}
	assignRootCauseRanksAndTiers(items)
	if items[0].Tier != "primary" {
		t.Fatalf("top on-chain texture_upload must be a primary candidate: %+v", items[0])
	}
	if items[1].Tier != "primary" {
		t.Fatalf("the existing on-chain runnable co-primary rule must remain: %+v", items[1])
	}
	if !rootCauseTypeCanBeDirectOnChain("texture_upload") {
		t.Fatalf("texture_upload must be direct-on-chain admissible like its classmates")
	}
	if rootCauseShouldBeCoPrimary(items[0]) {
		t.Fatalf("semantic rows use positional election, not blanket co-primary: %+v", items[0])
	}
}

func TestTEXNonChainTextureUploadCarriesBackgroundRank(t *testing.T) {
	items := []RootCauseRankItem{
		{Type: "supply_pressure", ImpactMs: 40, ChainRelevance: "background"},
		{Type: "texture_upload", ImpactMs: 4, ChainRelevance: "background", Causality: "background"},
	}
	assignRootCauseRanksAndTiers(items)
	// §23.1 ③ typed mention gate: the background board position counts every
	// published non-on-chain row; the FIELD lands on semantic rows only.
	if items[1].BackgroundRank != 2 {
		t.Fatalf("non-chain texture_upload must carry its typed background board position: %+v", items[1])
	}
	if items[0].BackgroundRank != 0 {
		t.Fatalf("non-semantic rows never carry the background_rank field: %+v", items[0])
	}
}

// --- TEX-4 + T2: customer-specimen end-to-end ---------------------------------

func TestTEXSpecimenTextureUploadFamilyFoldsIntervalUnionOnEmittingThread(t *testing.T) {
	idx := buildTraceIndex(t, "texture_upload_specimen.systrace", texUploadSpecimenTrace)
	res := Run(idx, Query{View: "window_stats", PID: 50820, TimeStart: 15151.960530, TimeEnd: 15151.960810})
	if res.WindowStats == nil {
		t.Fatalf("window_stats must build on the specimen")
	}
	var texSpans []TraceSpanSummary
	for _, span := range res.WindowStats.TraceSpans {
		if strings.HasPrefix(span.Name, "Texture upload(15283)") {
			texSpans = append(texSpans, span)
		}
	}
	// Exactly the two CLOSED size variants pair into spans; the third
	// (512x194, specimen line 53) never closes inside the capture.
	if len(texSpans) != 2 {
		t.Fatalf("expected the 2 closed Texture upload spans, got %d: %+v", len(texSpans), texSpans)
	}
	for _, span := range texSpans {
		// T2 (§28.2): the span attaches to the EMITTING thread's timeline —
		// tid 51342 (tgid 50820) — never to the B|18998 payload pid, which
		// survives as the SpanPID identity only.
		if span.Thread.PID != 51342 || span.Thread.Comm != "RenderThread" {
			t.Fatalf("T2: span must attach to the emitting thread RenderThread-51342, got %+v", span.Thread)
		}
		if span.SpanPID != 18998 {
			t.Fatalf("T2: the payload pid must survive as SpanPID identity, got %+v", span)
		}
		if span.SemanticClass != "texture_upload" {
			t.Fatalf("specimen span must classify as texture_upload: %+v", span)
		}
	}
	// Family fold (§24.10 treatment parity): ONE (thread, texture_upload)
	// family whose participation value is the interval union of the Texture
	// upload spans THEMSELVES — 52x50 [960565..960641] = 0.076ms plus 512x67
	// [960642..960760] = 0.118ms, disjoint, so union == Σ == 0.194ms. The
	// nested AllocPages/MapMemory/Alloc Ioctl/GraphicsLoad child spans are NOT
	// class members and contribute nothing (no double count).
	families := FoldSemanticSpanFamilies(nil, res.WindowStats.TraceSpans)
	var tex *SemanticSpanFamily
	for i := range families {
		if families[i].SemanticClass == "texture_upload" {
			if tex != nil {
				t.Fatalf("expected ONE texture_upload family, got a second: %+v", families)
			}
			tex = &families[i]
		}
	}
	if tex == nil {
		t.Fatalf("expected a texture_upload family: %+v", families)
	}
	if tex.Thread.PID != 51342 {
		t.Fatalf("family thread must be the emitting thread: %+v", tex.Thread)
	}
	if len(tex.Members) != 2 {
		t.Fatalf("family members must be exactly the 2 closed Texture upload spans: %+v", tex.Members)
	}
	for _, member := range tex.Members {
		if !strings.HasPrefix(member.Name, "Texture upload(15283)") {
			t.Fatalf("nested child spans must never enter the texture_upload family: %+v", member)
		}
	}
	const wantTotal = 0.076 + 0.118
	if tex.TotalMs < wantTotal-0.001 || tex.TotalMs > wantTotal+0.001 {
		t.Fatalf("family value must be the interval-union projection total ≈%.3fms, got %.6f", wantTotal, tex.TotalMs)
	}
	if tex.FoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("the two closed variants are disjoint — caliber must be sum_disjoint, got %q", tex.FoldCaliber)
	}
	// The dangling 512x194 begin (never closed inside the capture) rides the
	// fail-loud incomplete-pair caveat instead of silently vanishing.
	danglingNamed := false
	for _, caveat := range res.WindowStats.Caveats {
		if strings.Contains(caveat, "Texture upload(15283) 512x194") {
			danglingNamed = true
		}
	}
	if !danglingNamed {
		t.Fatalf("dangling semantic begin must be named in the caveats: %+v", res.WindowStats.Caveats)
	}
}

func TestTEXSpecimenRankMintsSingleTextureUploadFamilyRow(t *testing.T) {
	idx := buildTraceIndex(t, "texture_upload_specimen_rank.systrace", texUploadSpecimenTrace)
	rank := BuildRootCauseRank(idx, Query{PID: 50820, TimeStart: 15151.960530, TimeEnd: 15151.960810, MinDurationMs: 0.01, Limit: 12})
	var tex *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "texture_upload" {
			if tex != nil {
				t.Fatalf("the family must compete as ONE merged contender, got a second texture_upload row: %+v", rank.Items)
			}
			tex = &rank.Items[i]
		}
	}
	if tex == nil {
		t.Fatalf("expected a texture_upload rank row: %+v", rank.Items)
	}
	if tex.SemanticClass != "texture_upload" || tex.MemberCount != 2 {
		t.Fatalf("rank row must be the ×2 family form: %+v", tex)
	}
	// Roster keeps the verbatim span names — the (id) and WxH suffixes are the
	// member distinguishing keys (§28.1: 原文留 roster 区分键).
	roster := strings.Join(tex.MemberRoster, " | ")
	for _, want := range []string{"Texture upload(15283) 512x67", "Texture upload(15283) 52x50"} {
		if !strings.Contains(roster, want) {
			t.Fatalf("member roster must carry the verbatim span name %q: %+v", want, tex.MemberRoster)
		}
	}
	const wantTotal = 0.076 + 0.118
	if tex.CumulativeImpactMs < wantTotal-0.001 || tex.CumulativeImpactMs > wantTotal+0.001 {
		t.Fatalf("family participation value must be the window-projection total ≈%.3fms, got %.6f", wantTotal, tex.CumulativeImpactMs)
	}
	// Zero-chain specimen: the family competes on the background composite
	// board, so the §23.1 ③ typed mention gate has a position to read.
	if rootCauseItemIsOnChain(*tex) {
		t.Fatalf("specimen has no wakeup chain for the target — the family must ride the background lane: %+v", tex)
	}
	if tex.BackgroundRank <= 0 {
		t.Fatalf("background texture_upload family must carry its typed board position: %+v", tex)
	}
	// The two member spans must not double-publish as generic trace_span rows.
	for _, item := range rank.Items {
		if item.Type == "trace_span" && strings.HasPrefix(item.SpanName, "Texture upload(") {
			t.Fatalf("family-consumed span must not double-mint as trace_span: %+v", item)
		}
	}
}
