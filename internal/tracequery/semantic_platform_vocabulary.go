package tracequery

import "strings"

// Platform vocabulary for deterministic semantic work.
//
// The built-in arms of traceSpanSemanticWorkClass know a handful of
// hand-written marker shapes. Runtime work recorded with a platform's own
// marker names (ArkTS collection phases, an ART thread waiting for a
// collection, render-service and driver pipeline compilation) fell through
// them and was ranked as an anonymous span. This file is the data that closes
// that gap: one typed row per marker, consulted by the one classifier AFTER
// every built-in arm has declined, so a name a built-in arm classifies keeps
// exactly that result.
//
// What a row may and may not do:
//   - It is matched against a captured trace record field (the marker name),
//     never against a question or an answer.
//   - It matches only at the operation head: after one "H:" prefix and one
//     trace-chain prefix are stripped, the name must START with the row's
//     head and the next byte must satisfy the row's boundary. A domain that is
//     merely mentioned in a namespace, a class name, an argument list or a
//     trailing word never classifies.
//   - Only an Operation row mints one of the six existing classes. Cycle,
//     Phase and Related rows are recognised and deliberately not minted: the
//     pause is measured where it hurts (on the thread that runs or waits for
//     the work), so a collector's own cycle and its helper steps can never be
//     promoted from the background.
//   - Classification is not chain membership. The chain engine alone decides
//     whether a minted span sits on the dependency chain of the target.
//
// Adding a marker is one row. There is no per-row code path.

type semanticVocabRole uint8

const (
	// semanticVocabOperation: the span IS one unit of the family's work on
	// the thread that records it. Mints Family.
	semanticVocabOperation semanticVocabRole = iota + 1
	// semanticVocabCycle: a collector's own cycle on a runtime thread.
	// Recognised, not minted.
	semanticVocabCycle
	// semanticVocabPhase: a sub-step nested in an operation or a cycle.
	// Recognised, never minted (its parent carries the interval).
	semanticVocabPhase
	// semanticVocabRelated: same domain, different operation (a request, a
	// finalizer callback, cache reads and writes, initialisation).
	// Recognised, never minted.
	semanticVocabRelated
)

// semanticVocabStance is meaningful for Operation and Cycle rows only.
type semanticVocabStance uint8

const (
	// semanticVocabExecutes: the recording thread performs the work.
	semanticVocabExecutes semanticVocabStance = iota + 1
	// semanticVocabWaits: the recording thread is parked until someone
	// else's work ends.
	semanticVocabWaits
)

type semanticVocabBoundary uint8

const (
	// semanticVocabBoundaryNonLetter: the byte after the head is not an
	// ASCII letter (end of name, a digit, ';', ' ', '(', '_' ...).
	semanticVocabBoundaryNonLetter semanticVocabBoundary = iota + 1
	// semanticVocabBoundaryEnd: whole-name equality.
	semanticVocabBoundaryEnd
	// semanticVocabBoundaryAny: a namespace prefix such as "gc::". Never
	// allowed on an Operation row.
	semanticVocabBoundaryAny
)

// semanticVocabAttestation says where a row's marker was observed.
type semanticVocabAttestation uint8

const (
	// semanticVocabSeenInCapture: observed in real captures (the Evidence
	// text carries the census).
	semanticVocabSeenInCapture semanticVocabAttestation = iota + 1
	// semanticVocabReferenceOnly: attested by the reference tool's
	// vocabulary or test data only; not yet seen in any capture. The first
	// real capture that contains the marker must become a verbatim-line
	// fixture, or the row must be corrected or removed.
	semanticVocabReferenceOnly
)

type semanticVocabEntry struct {
	ID          string // stable row identity, e.g. "arkts.gc.full"
	Platform    string
	Family      string // one of the six existing semantic classes; never a new token
	Role        semanticVocabRole
	Stance      semanticVocabStance
	Head        string // lower-case ASCII literal; must be the START of the operation head
	Boundary    semanticVocabBoundary
	LabelEN     string // reader words
	LabelZH     string
	Attestation semanticVocabAttestation
	Evidence    string
}

const (
	vocabOp      = semanticVocabOperation
	vocabCycle   = semanticVocabCycle
	vocabPhase   = semanticVocabPhase
	vocabRelated = semanticVocabRelated
	vocabRuns    = semanticVocabExecutes
	vocabWaits   = semanticVocabWaits
	vocabWord    = semanticVocabBoundaryNonLetter
	vocabWhole   = semanticVocabBoundaryEnd
	vocabPrefix  = semanticVocabBoundaryAny
	vocabSeen    = semanticVocabSeenInCapture
	vocabRefOnly = semanticVocabReferenceOnly

	// Every reference-only row's Evidence starts with this sentence.
	vocabRefOnlyPrefix = "reference-attested, not yet seen in a capture: "
)

// The census behind the "seen" rows: 75 real cold-start captures were counted
// by marker line, and 15 of them (one per app, version and cold-start type)
// were paired into spans in full. An Evidence text quotes one of the two.
var semanticPlatformVocabulary = []semanticVocabEntry{
	// --- ArkTS runtime: a local collection runs on the thread that owns the heap.
	{"arkts.gc.partial", "arkts_runtime", "gc_pause", vocabOp, vocabRuns, "partialgc::runphases", vocabWord,
		"ArkTS partial collection (runs on the JS thread)", "ArkTS 局部 GC（在 JS 线程上执行）", vocabSeen, "100 marker lines in 64 of 75 captures"},
	{"arkts.gc.full", "arkts_runtime", "gc_pause", vocabOp, vocabRuns, "fullgc::runphases", vocabWord,
		"ArkTS full collection (runs on the JS thread)", "ArkTS 全量 GC（在 JS 线程上执行）", vocabSeen, "135 marker lines in 43 of 75 captures"},
	{"arkts.gc.conc_mark_start", "arkts_runtime", "gc_pause", vocabOp, vocabRuns, "concurrentmarker::mark", vocabWord,
		"ArkTS concurrent-mark start (root marking on the JS thread)", "ArkTS 并发标记启动（JS 线程根标记）", vocabSeen, "26 spans in the 15 paired captures (plain form in 14), JS threads only"},
	{"arkts.gc.wait_shared", "arkts_runtime", "gc_pause", vocabOp, vocabWaits, "suspendtime::waitgcfinished", vocabWord,
		"waiting for the shared-heap collection to finish", "等待共享堆 GC 完成", vocabSeen, "17 marker lines in 17 of 75 captures"},
	// --- ART (Android container): a mutator waits for a running collection.
	{"art.gc.wait_completion", "art_runtime", "gc_pause", vocabOp, vocabWaits, "gc: wait for completion", vocabWord,
		"waiting for a running collection to complete", "等待 GC 完成", vocabSeen, "24 marker lines in 5 of 75 captures"},

	// --- Collector cycles on their own thread: recognised, not minted.
	{"arkts.gc.shared_cycle", "arkts_runtime", "gc_pause", vocabCycle, vocabRuns, "sharedgc::runphases", vocabWord,
		"ArkTS shared-heap collection cycle (collector thread)", "ArkTS 共享堆 GC 周期（GC 线程）", vocabSeen, "2 marker lines in 2 of 75 captures"},
	{"arkts.gc.shared_full_cycle", "arkts_runtime", "gc_pause", vocabCycle, vocabRuns, "sharedfullgc::runphases", vocabWord,
		"ArkTS shared-heap full collection cycle (collector thread)", "ArkTS 共享堆全量 GC 周期（GC 线程）", vocabSeen, "38 marker lines in 23 of 75 captures"},
	{"arkts.gc.shared_cc_cycle", "arkts_runtime", "gc_pause", vocabCycle, vocabRuns, "sharedcc::runphases", vocabWord,
		"ArkTS shared-heap concurrent-copy cycle (collector thread)", "ArkTS 共享堆并发复制周期（GC 线程）", vocabSeen, "59 marker lines in 29 of 75 captures"},
	{"art.gc.cc_background", "art_runtime", "gc_pause", vocabCycle, vocabRuns, "background concurrent copying gc", vocabWhole,
		"ART background concurrent-copying collection cycle", "ART 后台并发复制 GC 周期", vocabSeen, "288 marker lines in 75 of 75 captures"},
	{"art.gc.cc_background_young", "art_runtime", "gc_pause", vocabCycle, vocabRuns, "background young concurrent copying gc", vocabWhole,
		"ART background young-generation collection cycle", "ART 后台年轻代 GC 周期", vocabSeen, "349 marker lines in 74 of 75 captures"},
	{"art.gc.cc_nativealloc", "art_runtime", "gc_pause", vocabCycle, vocabRuns, "nativealloc concurrent copying gc", vocabWhole,
		"ART collection cycle triggered by native allocation", "ART 原生分配触发的 GC 周期", vocabSeen, "26 marker lines in 25 of 75 captures"},
	{"art.gc.cc_transition", "art_runtime", "gc_pause", vocabCycle, vocabRuns, "collectortransition concurrent copying gc", vocabWhole,
		"ART collector-transition collection cycle", "ART 回收器切换 GC 周期", vocabSeen, "32 marker lines in 30 of 75 captures"},
	{"art.gc.cc_explicit", "art_runtime", "gc_pause", vocabCycle, vocabRuns, "explicit concurrent copying gc", vocabWhole,
		"ART explicitly requested collection cycle (on the calling thread)", "ART 显式请求的 GC 周期（调用线程）", vocabSeen, "23 marker lines in 7 of 75 captures"},

	// --- Steps nested in an operation or a cycle (namespace prefixes; the
	// longest head wins, so the operations above take precedence).
	{"arkts.gc.phase.partial", "arkts_runtime", "gc_pause", vocabPhase, 0, "partialgc::", vocabPrefix,
		"ArkTS partial collection step", "ArkTS 局部 GC 步骤", vocabSeen, "steps nested in the partial collection in the paired captures"},
	{"arkts.gc.phase.full", "arkts_runtime", "gc_pause", vocabPhase, 0, "fullgc::", vocabPrefix,
		"ArkTS full collection step", "ArkTS 全量 GC 步骤", vocabSeen, "steps nested in the full collection in the paired captures"},
	{"arkts.gc.phase.shared", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedgc::", vocabPrefix,
		"ArkTS shared-heap collection step", "ArkTS 共享堆 GC 步骤", vocabSeen, "18 spans in 7 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.sharedfull", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedfullgc::", vocabPrefix,
		"ArkTS shared-heap full collection step", "ArkTS 共享堆全量 GC 步骤", vocabSeen, "4 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.sharedcc", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedcc::", vocabPrefix,
		"ArkTS shared-heap concurrent-copy step", "ArkTS 共享堆并发复制步骤", vocabSeen, "5 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.gc", "arkts_runtime", "gc_pause", vocabPhase, 0, "gc::", vocabPrefix,
		"ArkTS collection step", "ArkTS GC 步骤", vocabSeen, "nested in a collection or on a helper thread, 14 of 15 paired captures"},
	{"arkts.gc.phase.concmarker", "arkts_runtime", "gc_pause", vocabPhase, 0, "concurrentmarker::", vocabPrefix,
		"ArkTS concurrent-marker step", "ArkTS 并发标记步骤", vocabSeen, "26 reset spans in the 15 paired captures (plain form in 13)"},
	{"arkts.gc.phase.sharedconcmarker", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedconcurrentmarker::", vocabPrefix,
		"ArkTS shared-heap concurrent-marker step", "ArkTS 共享堆并发标记步骤", vocabSeen, "11 spans in 5 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.sharedmarker", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedgcmarker", vocabPrefix,
		"ArkTS shared-heap marker step", "ArkTS 共享堆 GC 标记步骤", vocabSeen, "7 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.concsweeper", "arkts_runtime", "gc_pause", vocabPhase, 0, "concurrentsweeper::", vocabPrefix,
		"ArkTS concurrent sweep (collector thread)", "ArkTS 并发清扫（GC 线程）", vocabSeen, "72 spans in 10 of 15 paired captures, collector thread only"},
	{"arkts.gc.phase.sharedconcsweeper", "arkts_runtime", "gc_pause", vocabPhase, 0, "sharedconcurrentsweeper::", vocabPrefix,
		"ArkTS shared-heap concurrent sweep (collector thread)", "ArkTS 共享堆并发清扫（GC 线程）", vocabSeen, "18 spans in 7 of 15 paired captures, collector thread only"},
	{"gles.link_program", "gles_driver", "shader_compile", vocabPhase, 0, "driver_link_program", vocabWord,
		"GLES program link (a step of shader compilation)", "GLES 程序链接（着色器编译子步骤）", vocabSeen, "13 marker lines in 7 of 75 captures; nested in a shader compile on a cache miss"},

	// --- Same domain, different operation.
	{"arkts.gc.related.finalize", "arkts_runtime", "gc_pause", vocabRelated, 0, "invokenativefinalizecallbacks", vocabWord,
		"native finalizer callbacks after a collection", "GC 后原生终结回调", vocabSeen, "53 spans in the 15 paired captures (plain form in 13)"},
	{"arkts.gc.related.runfinalize", "arkts_runtime", "gc_pause", vocabRelated, 0, "runfinalizecallbacks", vocabWord,
		"finalizer callbacks after a collection", "GC 后终结回调", vocabSeen, "60 spans in 14 of 15 paired captures"},
	{"arkts.gc.related.trigger_main", "arkts_runtime", "gc_pause", vocabRelated, 0, "triggermainthreadlocalgc", vocabWord,
		"request for a local collection on the main thread", "请求主线程局部 GC", vocabSeen, "43 spans in the 15 paired captures (chain-prefixed form in 13), requester threads only"},
	{"arkts.gc.related.trigger_main_shared", "arkts_runtime", "gc_pause", vocabRelated, 0, "triggermainthreadsharedgc", vocabWord,
		"request for a shared-heap collection from the main thread", "请求主线程共享堆 GC", vocabSeen, "37 spans in the 15 paired captures (chain-prefixed form in 13), requester threads only"},
	{"arkts.gc.related.trigger_worker", "arkts_runtime", "gc_pause", vocabRelated, 0, "triggerworkerthreadlocalgc", vocabWord,
		"request for a local collection on a worker thread", "请求工作线程局部 GC", vocabSeen, "5 spans in the 15 paired captures"},
	{"arkts.gc.related.idle_task", "arkts_runtime", "gc_pause", vocabRelated, 0, "runtriggergctaskcallback", vocabWord,
		"idle-time collection task callback", "空闲 GC 任务回调", vocabSeen, "14 spans in 8 of 15 paired captures"},
	{"arkts.gc.related.smart_gc", "arkts_runtime", "gc_pause", vocabRelated, 0, "smart gc end with", vocabWord,
		"collection scheduling note", "GC 调度记录", vocabSeen, "349 spans in 15 of 15 paired captures"},
	{"hwui.shadercache", "hwui_skia", "shader_compile", vocabRelated, 0, "shadercache::", vocabPrefix,
		"shader cache read or write", "着色器缓存读写", vocabSeen, "368 cache-load spans in 15 of 15 paired captures"},
	{"hwui.shadercache.init", "hwui_skia", "shader_compile", vocabRelated, 0, "initshaderdiskcache", vocabWord,
		"shader disk cache initialisation", "着色器磁盘缓存初始化", vocabSeen, "27 spans in 15 of 15 paired captures"},
	{"hwui.shadercache.ram", "hwui_skia", "shader_compile", vocabRelated, 0, "hwui ram cache:", vocabPrefix,
		"shader memory cache note", "着色器内存缓存记录", vocabSeen, "368 spans in 15 of 15 paired captures"},
	{"rs.shaderdfx", "rs_ddgr", "shader_compile", vocabRelated, 0, "shaderdfx", vocabPrefix,
		"shader diagnostics note", "着色器诊断记录", vocabSeen, "seen in 4 of 15 paired captures"},
	{"art.jit.memregion_init", "art_runtime", "jit_compile", vocabRelated, 0, "bool art::jit::jitmemoryregion::initialize", vocabWord,
		"JIT code-cache memory region initialisation (not a compilation)", "JIT 代码缓存内存区初始化（不是编译）", vocabSeen, "39 spans in 15 of 15 paired captures"},

	// --- Shader modules and pipelines.
	{"rs.shader.create_modules", "rs_ddgr", "shader_compile", vocabOp, vocabRuns, "ddgr createshadermodules", vocabWord,
		"render-service shader module creation", "渲染服务着色器模块创建", vocabSeen, "39 marker lines in 17 of 75 captures"},
	// Reference-only rows. A pipeline creation is shader compilation by
	// definition, so they mint; none of them has been seen in a capture.
	// The reference tool lists the bare tokens and matches them anywhere in
	// a name; its own test data spells the driver markers with an "ML: "
	// prefix and the render-service wrapper in the plural, so those
	// spellings have rows of their own (a bare-token row cannot match them
	// at the head).
	{"vk.pipeline.create", "vulkan", "shader_compile", vocabOp, vocabRuns, "vkcreategraphicspipelines", vocabWord,
		"Vulkan graphics pipeline creation", "Vulkan 图形管线创建", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"rs.pipeline.create", "rs_ddgr", "shader_compile", vocabOp, vocabRuns, "creategraphicspipeline", vocabWord,
		"graphics pipeline creation", "图形管线创建", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"rs.pipelines.create", "rs_ddgr", "shader_compile", vocabOp, vocabRuns, "creategraphicspipelines", vocabWord,
		"graphics pipeline creation", "图形管线创建", vocabRefOnly, vocabRefOnlyPrefix + "plural spelling in the reference tool's test data"},
	{"ohos.spirv.compile", "vulkan", "shader_compile", vocabOp, vocabRuns, "ohos_compilespriv", vocabWord,
		"SPIR-V shader compilation", "SPIR-V 着色器编译", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"bisheng.pipeline.compile", "vulkan", "shader_compile", vocabOp, vocabRuns, "bisheng_graphic_pipeline_compilation", vocabWord,
		"driver pipeline compilation", "驱动管线编译", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"bisheng.pipeline.compile.ml", "vulkan", "shader_compile", vocabOp, vocabRuns, "ml: bisheng_graphic_pipeline_compilation", vocabWord,
		"driver pipeline compilation", "驱动管线编译", vocabRefOnly, vocabRefOnlyPrefix + "prefixed spelling in the reference tool's test data"},
	{"bisheng.backend.compile", "vulkan", "shader_compile", vocabOp, vocabRuns, "bisheng_backend_compilation", vocabWord,
		"driver backend compilation", "驱动后端编译", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"bisheng.backend.compile.ml", "vulkan", "shader_compile", vocabOp, vocabRuns, "ml: bisheng_backend_compilation", vocabWord,
		"driver backend compilation", "驱动后端编译", vocabRefOnly, vocabRefOnlyPrefix + "prefixed spelling in the reference tool's test data"},
	// Reference-only, recognised and not minted: the token names no compile
	// operation, or the operation is a creation whose upload cost is unknown.
	{"bisheng.converter", "vulkan", "shader_compile", vocabRelated, 0, "runsingleconverter", vocabWord,
		"shader conversion step", "着色器转换步骤", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, shader-compile group"},
	{"bisheng.converter.ml", "vulkan", "shader_compile", vocabRelated, 0, "ml: runsingleconverter", vocabWord,
		"shader conversion step", "着色器转换步骤", vocabRefOnly, vocabRefOnlyPrefix + "prefixed spelling in the reference tool's test data"},
	{"rs.texture.create", "rs_ddgr", "texture_upload", vocabRelated, 0, "oncreatetexture", vocabWord,
		"GPU texture creation", "GPU 纹理创建", vocabRefOnly, vocabRefOnlyPrefix + "reference vocabulary, texture-creation group"},
}

// semanticVocabOperationHead strips, at most once each and in this order:
// surrounding spaces, one "H:" marker prefix (either case), and one
// trace-chain prefix of the exact machine shape "[hex,hex,hex]#". The
// remainder keeps its original case; matching folds ASCII case.
func semanticVocabOperationHead(name string) string {
	s := strings.TrimSpace(name)
	if len(s) >= 2 && (s[0] == 'H' || s[0] == 'h') && s[1] == ':' {
		s = strings.TrimLeft(s[2:], " ")
	}
	if len(s) > 0 && s[0] == '[' {
		if end := strings.Index(s, "]#"); end > 0 && semanticVocabIsChainID(s[1:end]) {
			s = s[end+2:]
		}
	}
	return s
}

// semanticVocabIsChainID reports whether body is exactly three comma-separated
// runs of 1..16 lower-case hex digits.
func semanticVocabIsChainID(body string) bool {
	parts, run := 1, 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'):
			run++
			if run > 16 {
				return false
			}
		case c == ',':
			if run == 0 {
				return false
			}
			parts++
			run = 0
		default:
			return false
		}
	}
	return parts == 3 && run > 0
}

func semanticVocabASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func (e *semanticVocabEntry) matchesHead(head string) bool {
	if len(head) < len(e.Head) {
		return false
	}
	for i := 0; i < len(e.Head); i++ {
		c := head[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != e.Head[i] {
			return false
		}
	}
	rest := head[len(e.Head):]
	switch e.Boundary {
	case semanticVocabBoundaryAny:
		return true
	case semanticVocabBoundaryEnd:
		return rest == ""
	case semanticVocabBoundaryNonLetter:
		return rest == "" || !semanticVocabASCIILetter(rest[0])
	}
	return false
}

// semanticPlatformVocabularyLookup returns the row whose head starts the
// operation head of name. Among matching rows the longest head wins, so an
// operation ("partialgc::runphases") beats the namespace of its steps
// ("partialgc::").
func semanticPlatformVocabularyLookup(name string) (semanticVocabEntry, bool) {
	head := semanticVocabOperationHead(name)
	if head == "" {
		return semanticVocabEntry{}, false
	}
	best := -1
	for i := range semanticPlatformVocabulary {
		e := &semanticPlatformVocabulary[i]
		if (best < 0 || len(e.Head) > len(semanticPlatformVocabulary[best].Head)) && e.matchesHead(head) {
			best = i
		}
	}
	if best < 0 {
		return semanticVocabEntry{}, false
	}
	return semanticPlatformVocabulary[best], true
}

// semanticPlatformVocabularyNearMiss reports that an operation head occurs in
// the (lower-cased) name but not at the head position, or without its
// boundary. Advisory only: it feeds the naming-drift disclosure and nothing
// else.
func semanticPlatformVocabularyNearMiss(lower string) bool {
	for i := range semanticPlatformVocabulary {
		e := &semanticPlatformVocabulary[i]
		if e.Role == semanticVocabOperation && strings.Contains(lower, e.Head) {
			return true
		}
	}
	return false
}

// semanticSpanMintedByPlatformVocabulary reports that the span's class comes
// from a vocabulary Operation row, i.e. no built-in arm classifies the name.
// It reads the classifier's own verdict, so there is no second list of
// built-in arms to keep in step.
func semanticSpanMintedByPlatformVocabulary(name string) bool {
	_, owner, ok := traceSpanSemanticWorkClassWithOwner(name)
	return ok && owner == traceSpanSemanticOwnerPlatformVocabulary
}

// semanticSpanAdmitsHostEdgeLane is false only for a span minted by a
// vocabulary Operation row whose stance is "waits".
//
// The host-edge lane puts a span of a thread other than the target on the
// chain when that thread holds a typed wakeup edge toward the target. A wait
// recorded on another thread is not such a dependency: it is the same pause
// seen by a sibling victim, which neither runs on the target nor blocks it.
// The target's own wait (self lane) and a wait that intersects a typed chain
// window of its thread (interval lane) are unaffected.
//
// This bar applies to vocabulary rows only; names classified by a built-in
// arm keep the host-edge lane unchanged. It is tied to the open
// dependency-credit bound of the host-edge lane (the lane does not yet bound
// how much of a host's time before its wakeup edge is credited as a
// dependency of the target) and is to be revisited together with that bound.
func semanticSpanAdmitsHostEdgeLane(name string) bool {
	if !semanticSpanMintedByPlatformVocabulary(name) {
		return true
	}
	entry, ok := semanticPlatformVocabularyLookup(name)
	return !ok || entry.Stance != semanticVocabWaits
}

// semanticSpanSeatTier orders the platform seat pool by a typed identity,
// never by duration alone: 0 = recorded on the query target thread, 1 = work
// executed on another thread, 2 = a wait recorded on another thread.
func semanticSpanSeatTier(span TraceSpanSummary, targetPID int) int {
	if targetPID > 0 && span.Thread.PID == targetPID {
		return 0
	}
	if !semanticSpanAdmitsHostEdgeLane(span.Name) {
		return 2
	}
	return 1
}

// TraceSpanVocabularyHit is the reader-facing description of a vocabulary
// row. Role is "operation", "cycle", "phase" or "related"; Stance is
// "executes", "waits" or empty.
type TraceSpanVocabularyHit struct {
	ID       string
	Platform string
	Family   string
	Role     string
	Stance   string
	LabelEN  string
	LabelZH  string
}

// TraceSpanVocabulary is the exported thin wrapper over the vocabulary
// lookup for deterministic display consumers (the trace census). It
// describes the name; whether the name carries a class is answered by
// TraceSpanSemanticClass alone. It must never gate or promote anything.
func TraceSpanVocabulary(name string) (TraceSpanVocabularyHit, bool) {
	entry, ok := semanticPlatformVocabularyLookup(name)
	if !ok {
		return TraceSpanVocabularyHit{}, false
	}
	hit := TraceSpanVocabularyHit{
		ID: entry.ID, Platform: entry.Platform, Family: entry.Family,
		LabelEN: entry.LabelEN, LabelZH: entry.LabelZH,
	}
	switch entry.Role {
	case semanticVocabOperation:
		hit.Role = "operation"
	case semanticVocabCycle:
		hit.Role = "cycle"
	case semanticVocabPhase:
		hit.Role = "phase"
	case semanticVocabRelated:
		hit.Role = "related"
	}
	switch entry.Stance {
	case semanticVocabExecutes:
		hit.Stance = "executes"
	case semanticVocabWaits:
		hit.Stance = "waits"
	}
	return hit, true
}
