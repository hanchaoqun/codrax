package tracequery

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Platform vocabulary for deterministic semantic work — package pins.
// The public-entry tests live in internal/tool
// (trace_query_platform_semantic_hmc376_test.go) and internal/tracediag.

// --- table invariants ---------------------------------------------------------

func TestHMC376PlatformVocabularyTableInvariants(t *testing.T) {
	ids, heads := map[string]bool{}, map[string]bool{}
	for _, e := range semanticPlatformVocabulary {
		if e.ID == "" || ids[e.ID] {
			t.Fatalf("row id %q is empty or duplicated", e.ID)
		}
		ids[e.ID] = true
		if e.Head == "" || heads[e.Head] {
			t.Fatalf("%s: head %q is empty or duplicated", e.ID, e.Head)
		}
		heads[e.Head] = true
		for i := 0; i < len(e.Head); i++ {
			if c := e.Head[i]; c < 0x20 || c > 0x7e || (c >= 'A' && c <= 'Z') {
				t.Fatalf("%s: head %q must be lower-case printable ASCII", e.ID, e.Head)
			}
		}
		if strings.TrimSpace(e.Head) != e.Head || strings.HasPrefix(e.Head, "h:") || strings.HasPrefix(e.Head, "[") {
			t.Fatalf("%s: head %q must be the operation head after the marker and chain prefixes are stripped", e.ID, e.Head)
		}
		if _, ok := traceSpanSemanticWorkForClass(e.Family); !ok {
			t.Fatalf("%s: family %q is not one of the existing semantic classes", e.ID, e.Family)
		}
		if e.LabelEN == "" || e.LabelZH == "" || e.Evidence == "" {
			t.Fatalf("%s: both reader labels and evidence are required: %+v", e.ID, e)
		}
		switch e.Platform {
		case "arkts_runtime", "art_runtime", "rs_ddgr", "vulkan", "gles_driver", "hwui_skia":
		default:
			t.Fatalf("%s: unknown platform %q", e.ID, e.Platform)
		}
		switch e.Role {
		case semanticVocabOperation:
			if e.Stance != semanticVocabExecutes && e.Stance != semanticVocabWaits {
				t.Fatalf("%s: an operation needs a stance", e.ID)
			}
			if e.Boundary == semanticVocabBoundaryAny {
				t.Fatalf("%s: an operation must end at a boundary; an open namespace prefix can never mint", e.ID)
			}
		case semanticVocabCycle:
			if e.Stance != semanticVocabExecutes && e.Stance != semanticVocabWaits {
				t.Fatalf("%s: a cycle needs a stance", e.ID)
			}
		case semanticVocabPhase, semanticVocabRelated:
			if e.Stance != 0 {
				t.Fatalf("%s: stance is meaningful for operations and cycles only", e.ID)
			}
		default:
			t.Fatalf("%s: unknown role %d", e.ID, e.Role)
		}
		switch e.Boundary {
		case semanticVocabBoundaryNonLetter, semanticVocabBoundaryEnd, semanticVocabBoundaryAny:
		default:
			t.Fatalf("%s: unknown boundary %d", e.ID, e.Boundary)
		}
		switch e.Attestation {
		case semanticVocabSeenInCapture:
			if strings.HasPrefix(e.Evidence, vocabRefOnlyPrefix) {
				t.Fatalf("%s: a row seen in captures must quote its census, not the reference", e.ID)
			}
		case semanticVocabReferenceOnly:
			if !strings.HasPrefix(e.Evidence, vocabRefOnlyPrefix) || len(e.Evidence) == len(vocabRefOnlyPrefix) {
				t.Fatalf("%s: a reference-only row must say so and name its source: %q", e.ID, e.Evidence)
			}
		default:
			t.Fatalf("%s: unknown attestation %d", e.ID, e.Attestation)
		}
	}
	// The longest head wins. An operation whose head extends another
	// operation's head must not match more loosely than the shorter one.
	strictness := map[semanticVocabBoundary]int{semanticVocabBoundaryAny: 0, semanticVocabBoundaryNonLetter: 1, semanticVocabBoundaryEnd: 2}
	for _, short := range semanticPlatformVocabulary {
		for _, long := range semanticPlatformVocabulary {
			if short.Role != semanticVocabOperation || long.Role != semanticVocabOperation ||
				len(long.Head) <= len(short.Head) || !strings.HasPrefix(long.Head, short.Head) {
				continue
			}
			if strictness[long.Boundary] < strictness[short.Boundary] || long.Family != short.Family {
				t.Fatalf("%s extends %s with a weaker boundary or another family", long.ID, short.ID)
			}
		}
	}
}

// The reference-only rows are pinned by identity: the first real capture that
// contains one of these markers must become a verbatim-line fixture and move
// the row out of this list (or correct or remove the row).
func TestHMC376ReferenceOnlyRowsArePinned(t *testing.T) {
	want := []string{
		"bisheng.backend.compile", "bisheng.backend.compile.ml", "bisheng.converter", "bisheng.converter.ml",
		"bisheng.pipeline.compile", "bisheng.pipeline.compile.ml", "ohos.spirv.compile",
		"rs.pipeline.create", "rs.pipelines.create", "rs.texture.create",
	}
	var got []string
	for _, e := range semanticPlatformVocabulary {
		if e.Attestation == semanticVocabReferenceOnly {
			got = append(got, e.ID)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reference-only rows changed:\n got %v\nwant %v", got, want)
	}
	for _, e := range semanticPlatformVocabulary {
		if e.ID == "vk.pipeline.create" && (e.Attestation != semanticVocabSeenInCapture || strings.HasPrefix(e.Evidence, vocabRefOnlyPrefix)) {
			t.Fatalf("the Vulkan pipeline marker has real spans and must not be attested as reference-only: %+v", e)
		}
	}
}

// --- structural consumer pins -------------------------------------------------

// hmc376Consumers returns, for every non-test file of this package, the
// functions that reference ident, plus every reference outside a function
// declaration (package-level tables, function values, closures in var
// initialisers). The declaration of ident itself is not a reference.
func hmc376Consumers(t *testing.T, ident string) (inside map[string]bool, outside []string) {
	t.Helper()
	inside = map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		in, out := hmc376ScanConsumers(fset, file, ident)
		for fn := range in {
			inside[fn] = true
		}
		outside = append(outside, out...)
	}
	return inside, outside
}

func hmc376ScanConsumers(fset *token.FileSet, file *ast.File, ident string) (inside map[string]bool, outside []string) {
	inside = map[string]bool{}
	declared := map[*ast.Ident]bool{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			declared[d.Name] = true
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for _, name := range s.Names {
						declared[name] = true
					}
				case *ast.TypeSpec:
					declared[s.Name] = true
				}
			}
		}
	}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		ast.Inspect(decl, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || id.Name != ident || declared[id] {
				return true
			}
			if isFunc {
				inside[fn.Name.Name] = true
			} else {
				outside = append(outside, fset.Position(id.Pos()).String())
			}
			return true
		})
	}
	return inside, outside
}

func TestHMC376PlatformVocabularyConsumersPinned(t *testing.T) {
	// Scanner self-check against the bypass shapes a body-only walk misses.
	fset := token.NewFileSet()
	bypass, err := parser.ParseFile(fset, "bypass_shapes.go", `package tracequery

var table = map[string]func(string) (semanticVocabEntry, bool){"t": semanticPlatformVocabularyLookup}

var fn = semanticPlatformVocabularyLookup

var closure = func(n string) bool { _, ok := semanticPlatformVocabularyLookup(n); return ok }

func semanticPlatformVocabularyLookup(name string) (semanticVocabEntry, bool) { return semanticVocabEntry{}, false }

func consumer(n string) bool { _, ok := semanticPlatformVocabularyLookup(n); return ok }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if in, out := hmc376ScanConsumers(fset, bypass, "semanticPlatformVocabularyLookup"); len(out) != 3 || len(in) != 1 || !in["consumer"] {
		t.Fatalf("scanner self-check failed: inside=%v outside=%v", in, out)
	}

	// Each symbol and the complete set of functions that may reference it.
	// The vocabulary may only feed the one classifier, the advisory
	// naming-drift signal, the seat pools and the wait-lane bar. Adding a
	// consumer fails here on purpose: no gate, completion path or score may
	// read the vocabulary.
	pins := map[string][]string{
		"semanticPlatformVocabulary":             {"semanticPlatformVocabularyLookup", "semanticPlatformVocabularyNearMiss"},
		"semanticPlatformVocabularyLookup":       {"traceSpanSemanticWorkClassWithOwner", "traceSpanNearMissesSemanticWorkClassification", "semanticSpanAdmitsHostEdgeLane", "TraceSpanVocabulary"},
		"semanticPlatformVocabularyNearMiss":     {"traceSpanNearMissesSemanticWorkClassification"},
		"traceSpanSemanticWorkClassWithOwner":    {"traceSpanSemanticWorkClass", "semanticSpanMintedByPlatformVocabulary", "partitionTraceMarkSpansBySemanticOwner"},
		"semanticSpanMintedByPlatformVocabulary": {"init", "semanticSpanAdmitsHostEdgeLane", "semanticSpanCapLowerBoundCaveat", "semanticFamilyMintedByPlatformVocabulary", "FoldSemanticSpanFamilies", "computeBusinessSpanMentions"},
		"semanticSpanAdmitsHostEdgeLane":         {"semanticTraceSpanProjectionForRootCause", "semanticSpanFamilyLane", "semanticSpanSeatTier", "computeBusinessSpanMentions"},
		"semanticSpanSeatTier":                   {"tierOf"},
		"TraceSpanVocabulary":                    {},
		// The advisory naming-drift signal: two caveat builders and the
		// exported census wrapper. No lane, tier, gate or score reads it.
		"traceSpanNearMissesSemanticWorkClassification": {"TraceSpanNearMissesSemanticWork", "incompleteSemanticTraceMarkCaveats", "buildRootCauseRankFromWithCache"},
	}
	for ident, allowed := range pins {
		inside, outside := hmc376Consumers(t, ident)
		if len(outside) > 0 {
			t.Fatalf("%s is referenced outside any function declaration: %v", ident, outside)
		}
		want := map[string]bool{}
		for _, fn := range allowed {
			want[fn] = true
		}
		if !reflect.DeepEqual(inside, want) {
			t.Fatalf("%s consumers changed:\n got %v\nwant %v", ident, inside, want)
		}
	}
}

// The exported description has exactly one consumer outside this package: the
// zero-model trace census. (The advisory wrapper beside it has the same one.)
func TestHMC376ExportedVocabularyHasOnlyTheCensusConsumer(t *testing.T) {
	for symbol, want := range map[string][]string{
		"tracequery.TraceSpanVocabulary(":             {"tracediag/census.go"},
		"tracequery.TraceSpanNearMissesSemanticWork(": {"tracediag/census.go"},
	} {
		var got []string
		err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(body), symbol) {
				got = append(got, filepath.ToSlash(strings.TrimPrefix(path, ".."+string(filepath.Separator))))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s consumers changed: got %v want %v", symbol, got, want)
		}
	}
}

// --- classification matrix on real marker names --------------------------------

type hmc376Name struct {
	name      string
	class     string // "" = no class
	row       string // "" = not recognised
	nearMiss  bool
	platform  bool // class minted by the vocabulary (not by a built-in arm)
	hostEdge  bool // may take the host-edge lane
	wantOwner traceSpanSemanticOwner
}

func hmc376Check(t *testing.T, c hmc376Name) {
	t.Helper()
	if got := TraceSpanSemanticClass(c.name); got != c.class {
		t.Errorf("%q: class %q, want %q", c.name, got, c.class)
	}
	row := ""
	if entry, ok := semanticPlatformVocabularyLookup(c.name); ok {
		row = entry.ID
	}
	if row != c.row {
		t.Errorf("%q: vocabulary row %q, want %q", c.name, row, c.row)
	}
	if got := TraceSpanNearMissesSemanticWork(c.name); got != c.nearMiss {
		t.Errorf("%q: naming-drift suspect = %v, want %v", c.name, got, c.nearMiss)
	}
	if got := semanticSpanMintedByPlatformVocabulary(c.name); got != c.platform {
		t.Errorf("%q: minted by the vocabulary = %v, want %v", c.name, got, c.platform)
	}
	if got := semanticSpanAdmitsHostEdgeLane(c.name); got != c.hostEdge {
		t.Errorf("%q: admits the host-edge lane = %v, want %v", c.name, got, c.hostEdge)
	}
	if _, owner, _ := traceSpanSemanticWorkClassWithOwner(c.name); owner != c.wantOwner {
		t.Errorf("%q: owner %d, want %d", c.name, owner, c.wantOwner)
	}
}

// Marker names below are verbatim from real captures unless a comment says
// otherwise.
func TestHMC376PlatformVocabularyClassifiesRealMarkerNames(t *testing.T) {
	minted := func(name, class, row string, hostEdge bool) hmc376Name {
		return hmc376Name{name: name, class: class, row: row, platform: true, hostEdge: hostEdge, wantOwner: traceSpanSemanticOwnerPlatformVocabulary}
	}
	known := func(name, row string) hmc376Name {
		return hmc376Name{name: name, row: row, hostEdge: true}
	}
	for _, c := range []hmc376Name{
		// Operations: the nine newly classified shapes, with their parameter
		// tails and both trace-chain forms.
		minted("H:PartialGC::RunPhases0;GCReason2;MarkReason3;Sensitive0;Cms0;IsInBackground0;Startup3;ConMark2;Young3407872;Old61865984;TotalCommit92798976;NativeBindingSize10759;NativeLimitSize73272731;ObjSizeBeforeSensitive0", "gc_pause", "arkts.gc.partial", true),
		minted("H:[b34913e60932d42,d37980,3bdde2c]#PartialGC::RunPhases0;GCReason12;MarkReason3;Sensitive0;Cms0;IsInBackground0;Startup3;ConMark2;Young5505024;Old75235328;TotalCommit110624768;NativeBindingSize3005443;NativeLimitSize72634001;ObjSizeBeforeSensitive0", "gc_pause", "arkts.gc.partial", true),
		minted("H:[a92abafdf8ae06e,0,0]#PartialGC::RunPhases0;GCReason12;MarkReason0;Sensitive0;Cms0;IsInBackground0;Startup3;ConMark2;Young1835008;Old61865984;TotalCommit91226112;NativeBindingSize10871;NativeLimitSize73272731;ObjSizeBeforeSensitive0", "gc_pause", "arkts.gc.partial", true),
		minted("H:FullGC::RunPhases;GCReason2;Sensitive0;IsInBackground0;Startup3;Young17825792;Old4194304;huge2359296;NonMov3670016;TotCommit34603008;ObjSizeBeforeSensitive0", "gc_pause", "arkts.gc.full", true),
		minted("H:[616e656c75646f6,2387ebc,6]#FullGC::RunPhases;GCReason2;Sensitive0;IsInBackground0;Startup3;Young2621440;Old524288;huge786432;NonMov2883584;TotCommit13369344;ObjSizeBeforeSensitive0", "gc_pause", "arkts.gc.full", true),
		minted("H:ConcurrentMarker::Mark0;MarkReason3;Sensitive0;Cms0;IsInBackground0;Startup3;ConMark0;Young3407872;Old61865984;TotalCommit92798976;NativeBindingSize10759;NativeLimitSize73272731", "gc_pause", "arkts.gc.conc_mark_start", true),
		minted("H:[b34913e60932d42,d37980,3bdde2c]#ConcurrentMarker::Mark0;MarkReason3;Sensitive0;Cms0;IsInBackground0;Startup3;ConMark0;Young5505024;Old75235328;TotalCommit110624768;NativeBindingSize3005443;NativeLimitSize72634001", "gc_pause", "arkts.gc.conc_mark_start", true),
		minted("H:SuspendTime::WaitGCFinished", "gc_pause", "arkts.gc.wait_shared", false),
		func() hmc376Name {
			// The built-in token test already lists every name carrying a
			// "gc" token; a classified name keeps that result unchanged.
			c := minted("GC: Wait For Completion Alloc", "gc_pause", "art.gc.wait_completion", false)
			c.nearMiss = true
			return c
		}(),
		func() hmc376Name {
			c := minted("GC: Wait For Completion Background", "gc_pause", "art.gc.wait_completion", false)
			c.nearMiss = true
			return c
		}(),
		func() hmc376Name {
			// "shader" in a classified name keeps the built-in token result.
			c := minted("H:DDGR CreateShaderModules pipelineKey = 10027053796648471134", "shader_compile", "rs.shader.create_modules", true)
			c.nearMiss = true
			return c
		}(),

		// Collector cycles on their own thread: recognised, never a class.
		known("Background concurrent copying GC", "art.gc.cc_background"),
		known("Background young concurrent copying GC", "art.gc.cc_background_young"),
		known("NativeAlloc concurrent copying GC", "art.gc.cc_nativealloc"),
		known("CollectorTransition concurrent copying GC", "art.gc.cc_transition"),
		known("Explicit concurrent copying GC", "art.gc.cc_explicit"),
		known("H:SharedCC::RunPhases;GCReason 2;MarkReason13", "arkts.gc.shared_cc_cycle"),
		known("H:SharedFullGC::RunPhases;GCReason2;Sensitive0;IsInBackground0;Startup3;Old7077888;huge1835008;NonMov262144;TotCommit9437184;NativeBindingSize0;NativeLimitGC1073741824;NativeLimitCM314572800", "arkts.gc.shared_full_cycle"),
		// Nested steps.
		known("H:PartialGC::Mark", "arkts.gc.phase.partial"),
		known("H:FullGC::Mark", "arkts.gc.phase.full"),
		known("H:FullGC::Sweep", "arkts.gc.phase.full"),
		known("H:GC::MarkRoots", "arkts.gc.phase.gc"),
		known("H:GC::ProcessWorkloads", "arkts.gc.phase.gc"),
		known("H:GC::WaitFinished", "arkts.gc.phase.gc"),
		known("H:GC::MarkJitCodeMap", "arkts.gc.phase.gc"),
		known("H:[b34913e60932d42,d37980,3bdde2c]#GC::MarkRoots", "arkts.gc.phase.gc"),
		known("H:ConcurrentMarker::Reset", "arkts.gc.phase.concmarker"),
		known("H:SharedConcurrentMarker::Mark;MarkReason13;Sensitive0;IsInBackground0;Startup3;Old4980736", "arkts.gc.phase.sharedconcmarker"),
		known("H:ConcurrentSweeper::Sweep", "arkts.gc.phase.concsweeper"),
		known("H:SharedGCMarkerBase::MarkRoots", "arkts.gc.phase.sharedmarker"),
		known("H:SharedGC::Finish;Reason2;Sensitive0;IsInBackground0;Startup0;Old4980736", "arkts.gc.phase.shared"),
		known("driver_link_program", "gles.link_program"),
		// Same domain, different operation.
		known("H:InvokeNativeFinalizeCallbacks num:109", "arkts.gc.related.finalize"),
		known("H:RunFinalizeCallbacks:14", "arkts.gc.related.runfinalize"),
		known("H:[746e657665004e4,1c36006,1]#TriggerMainThreadLocalGC", "arkts.gc.related.trigger_main"),
		known("H:TriggerMainThreadSharedGC", "arkts.gc.related.trigger_main_shared"),
		known("H:TriggerWorkerThreadLocalGC", "arkts.gc.related.trigger_worker"),
		known("H:RunTriggerGCTaskCallback", "arkts.gc.related.idle_task"),
		known("H:smart gc end with no request frame(app_start or push_page)!", "arkts.gc.related.smart_gc"),
		known("ShaderCache::load", "hwui.shadercache"),
		known("ShaderCache::saveToDiskLocked", "hwui.shadercache"),
		known("initShaderDiskCache", "hwui.shadercache.init"),
		known("HWUI RAM cache: 1 shaders", "hwui.shadercache.ram"),
		known("H:ShaderDfx: isNeedSearch: 0, isUber: 0, pipelineKey: 7203965118492653086", "rs.shaderdfx"),
		// The batch-365 marker: JIT-domain work that is not a compilation.
		known("bool art::jit::JitMemoryRegion::Initialize(size_t, size_t, bool, bool, std::string *)", "art.jit.memregion_init"),
		// Not verbatim: an operation head that fails its boundary falls to
		// the namespace of its steps and is not minted.
		known("H:PartialGC::RunPhasesExtra", "arkts.gc.phase.partial"),

		// A domain mentioned somewhere in the name never classifies: no
		// class and no row.
		{name: "void art::jit::JitCodeCache::RemoveMethodsIn(Thread *, const LinearAlloc &)", hostEdge: true},
		{name: "bool art::ProfileCompilationInfo::Load(const std::string &, bool)", hostEdge: true},
		{name: "H:PipelineContext::OnSurfaceChanged", hostEdge: true},
		{name: "H:RSRenderPipelineAgent::CreateNodeAndSurface, nodeId: 235536006512663, uniqueId: 64037962", hostEdge: true},
		{name: "H:CalculateDelayTime pipelineOffset: 19435325, actualTimestamp: 2492713418890", hostEdge: true},
		{name: "TextureOp", hostEdge: true},
		{name: "Texture release(16)", hostEdge: true},
		{name: "TextureView#draw()", hostEdge: true},
		{name: "H:ProcessLazyTexture", hostEdge: true},
		{name: "Lcom/android/org/conscrypt/OpenSSLAeadCipherAES$GCM;", hostEdge: true},
		{name: "Not compiling because of pathological case", hostEdge: true},
		// Deliberately absent from the table: not specific to a collection.
		{name: "FullSuspendCheck", hostEdge: true},
		{name: "CopyingPhase", hostEdge: true},
		{name: "Suspending mutator threads", hostEdge: true},
		// Unrecognised names that keep their existing naming-drift flag.
		{name: "H:[a92abde608cabe6,0,0]#libsmart_gc_plugin.z.so", hostEdge: true, nearMiss: true},
		{name: "Verify dex file /data/user_de/0/com.huawei.hwid/files/kits/com.huawei.hms.fwkit/61800320/c", hostEdge: true, nearMiss: true},
		{name: "ancoAddMaliThread mali-compiler", hostEdge: true, nearMiss: true},
		// Not verbatim: an operation head away from the head position, or
		// without its boundary, is a naming-drift suspect and never a class.
		{name: "void Foo::PartialGC::RunPhases()", hostEdge: true, nearMiss: true},
		{name: "H:MyFullGC::RunPhases", hostEdge: true, nearMiss: true},
		{name: "GC: Wait For Completions", hostEdge: true, nearMiss: true},
		{name: "H:[Snapshot]#FullGC::RunPhases", hostEdge: true, nearMiss: true},
		{name: "H:H:FullGC::RunPhases", hostEdge: true, nearMiss: true},
		{name: "H:[1,2,3]#[4,5,6]#FullGC::RunPhases", hostEdge: true, nearMiss: true},
		{name: "Alloc concurrent copying GC", hostEdge: true, nearMiss: true},
	} {
		hmc376Check(t, c)
	}
}

// The Vulkan pipeline marker as recorded in a real render-service capture, with
// its level tag, and the two names nested around and inside it.
func TestHMC376VulkanPipelineRealSpellings(t *testing.T) {
	const row = "vk.pipeline.create"
	for _, c := range []hmc376Name{
		{name: "H:vkCreateGraphicsPipelines|I38", class: "shader_compile", row: row, platform: true, hostEdge: true, wantOwner: traceSpanSemanticOwnerPlatformVocabulary},
		{name: "H:vkCreateGraphicsPipelines", class: "shader_compile", row: row, platform: true, hostEdge: true, wantOwner: traceSpanSemanticOwnerPlatformVocabulary},
		{name: "vkCreateGraphicsPipelines", class: "shader_compile", row: row, platform: true, hostEdge: true, wantOwner: traceSpanSemanticOwnerPlatformVocabulary},
		// The diagnostics wrapper around one pipeline request carries no class.
		{name: "H:ShaderDfx: isNeedSearch: 0, isUber: 0, pipelineKey: 15613981270037230703, fp:[], dump:{", row: "rs.shaderdfx", hostEdge: true},
	} {
		hmc376Check(t, c)
	}
	if class, owner, ok := traceSpanSemanticWorkClassWithOwner("H:Load/Compile Shaders|I38"); !ok || class.SemanticClass != "shader_compile" || owner != traceSpanSemanticOwnerBuiltIn {
		t.Fatalf("the nested shader load keeps its built-in class: %+v owner=%d ok=%v", class, owner, ok)
	}
}

// The boundary after an operation head: a name that continues the operation is
// a different operation, whatever separator it continues with.
func TestHMC376OperationBoundaryRefusesContinuations(t *testing.T) {
	for name, wantClass := range map[string]string{
		// Ends the operation: end of name, parameters, level tag, numeric stamp.
		"H:FullGC::RunPhases":                                    "gc_pause",
		"H:FullGC::RunPhases;GCReason2":                          "gc_pause",
		"H:PartialGC::RunPhases0;GCReason12":                     "gc_pause",
		"H:vkCreateGraphicsPipelines|I38":                        "shader_compile",
		"H:CreateGraphicsPipeline pipelineKey = 7":               "shader_compile",
		"H:CreateGraphicsPipeline(1)":                            "shader_compile",
		"H:CreateGraphicsPipeline_42":                            "shader_compile",
		"H:CreateGraphicsPipeline::42":                           "shader_compile",
		"ML: bisheng_graphic_pipeline_compilation_095330.7_vs_5": "shader_compile",
		// Continues the operation: never minted as the parent operation.
		"H:FullGC::RunPhases::Sweep":            "",
		"H:ConcurrentMarker::Mark::Reset":       "",
		"H:CreateGraphicsPipeline_Cache":        "",
		"H:CreateGraphicsPipeline::Cache":       "",
		"bisheng_backend_compilation_cache_hit": "",
		"H:CreateGraphicsPipelineCache":         "",
	} {
		if got := TraceSpanSemanticClass(name); got != wantClass {
			t.Errorf("%q: class %q, want %q", name, got, wantClass)
		}
	}
	// A continued collection operation is a step of that collection.
	for name, row := range map[string]string{
		"H:FullGC::RunPhases::Sweep":      "arkts.gc.phase.full",
		"H:ConcurrentMarker::Mark::Reset": "arkts.gc.phase.concmarker",
	} {
		if hit, ok := TraceSpanVocabulary(name); !ok || hit.ID != row {
			t.Errorf("%q: recognised as %+v, want %s", name, hit, row)
		}
	}
}

// The pipeline and driver compile markers are attested by the reference tool
// only; no capture that contains them exists yet, so these names are
// synthetic spellings taken from the reference vocabulary and its test data.
func TestHMC376ReferenceOnlyMarkersSyntheticMatrix(t *testing.T) {
	minted := func(name, row string) hmc376Name {
		return hmc376Name{name: name, class: "shader_compile", row: row, platform: true, hostEdge: true, wantOwner: traceSpanSemanticOwnerPlatformVocabulary}
	}
	nearMinted := func(name, row string) hmc376Name {
		c := minted(name, row)
		c.nearMiss = true // carries a compile token; classified names keep the built-in token result
		return c
	}
	for _, c := range []hmc376Name{
		minted("H:CreateGraphicsPipeline", "rs.pipeline.create"),
		minted("H:CreateGraphicsPipeline pipelineKey = 7", "rs.pipeline.create"),
		minted("H:CreateGraphicsPipelines", "rs.pipelines.create"),
		nearMinted("OHOS_CompileSpriV", "ohos.spirv.compile"),
		nearMinted("bisheng_graphic_pipeline_compilation", "bisheng.pipeline.compile"),
		nearMinted("ML: bisheng_graphic_pipeline_compilation_095330.712504_vs_5a78626348643431316772436a75336e302f34786646327a59534d3d", "bisheng.pipeline.compile.ml"),
		nearMinted("bisheng_backend_compilation", "bisheng.backend.compile"),
		nearMinted("ML: bisheng_backend_compilation", "bisheng.backend.compile.ml"),
		// Recognised, not minted.
		{name: "runSingleConverter", row: "bisheng.converter", hostEdge: true},
		{name: "ML: runSingleConverter", row: "bisheng.converter.ml", hostEdge: true},
		{name: "H:onCreateTexture", row: "rs.texture.create", hostEdge: true},
		// Boundary and position failures: suspects, never a class.
		{name: "vkCreateGraphicsPipelinesKHR", hostEdge: true, nearMiss: true},
		{name: "H:CreateGraphicsPipelineCache", hostEdge: true, nearMiss: true},
		{name: "OHOS_CompileSpriVModule", hostEdge: true, nearMiss: true},
		{name: "XX: bisheng_backend_compilation", hostEdge: true, nearMiss: true},
		{name: "bisheng_backend_compilationx", hostEdge: true, nearMiss: true},
	} {
		hmc376Check(t, c)
	}
}

// --- composition with the built-in arms -----------------------------------------

// Every name a built-in arm classified before the vocabulary existed is still
// classified by that arm: the vocabulary never owns it, even where a table
// row would also match.
func TestHMC376BuiltInArmsKeepTheirNames(t *testing.T) {
	for _, c := range []struct{ name, class string }{
		// Positives of the existing collection, compiler, class-verification,
		// shader and texture pins.
		{"GC", "gc_pause"}, {"GC pause", "gc_pause"}, {"GC: Young collection", "gc_pause"},
		{"H:GC concurrent mark", "gc_pause"}, {"Garbage Collection", "gc_pause"}, {"garbage_collection full", "gc_pause"},
		{"SuspendAllForGC", "gc_pause"}, {"WaitForGcToComplete", "gc_pause"}, {"CollectGarbage", "gc_pause"},
		{"JIT compiling void pkg.Entry.run() (kind=Baseline)", "jit_compile"}, {"H:JITCompileTask", "jit_compile"},
		{"void vm::jit::JitCompiler::CompileMethod(int)", "jit_compile"},
		{"H:Ark runtime compile", "runtime_compile"}, {"H:ark::RuntimeCompiler::CompileMethod(int)", "runtime_compile"},
		{"VerifyClass com.example.Foo", "class_verification"}, {"VerifyClassLoaderContextMatch", "class_verification"},
		{"ShaderCompile pipeline warmup", "shader_compile"}, {"shader_compile", "shader_compile"},
		{"driver_compile_shader", "shader_compile"}, {"H:shader_cache: load/compile shaders, use_cache:1", "shader_compile"},
		{"cmpbe_v2_compile_multiple_shaders", "shader_compile"}, {"H:Load/Compile Shaders", "shader_compile"},
		{"Texture upload(15283) 512x194", "texture_upload"}, {"Texture upload", "texture_upload"}, {"H:Texture upload(1) 2x2", "texture_upload"},
		// A built-in arm and a table row both match: the built-in arm owns
		// the name. The table would call the first a nested step.
		{"H:GC::Mark", "gc_pause"},
		{"H:DDGR CreateShaderModules compile", "shader_compile"},
		{"GC: Wait For Completion collector", "gc_pause"},
	} {
		work, owner, ok := traceSpanSemanticWorkClassWithOwner(c.name)
		if !ok || work.SemanticClass != c.class || owner != traceSpanSemanticOwnerBuiltIn {
			t.Errorf("%q: class=%q owner=%d ok=%v, want %q owned by a built-in arm", c.name, work.SemanticClass, owner, ok, c.class)
		}
		if semanticSpanMintedByPlatformVocabulary(c.name) {
			t.Errorf("%q: a built-in name must never sit in the platform seat pool", c.name)
		}
		if !semanticSpanAdmitsHostEdgeLane(c.name) {
			t.Errorf("%q: a built-in name keeps the host-edge lane", c.name)
		}
	}
	// The built-in negatives stay negative.
	for _, name := range []string{
		"GCLockerMetrics", "gc_cache lookup", "GC metrics flush", "ImageGCrop", "magicgcvalue", "garbage collector metrics", "H:gc_cache",
		"void vm::jit::JitCompiler::LoadCacheEntry(int)", "H:RuntimeCompiler::Initialize(ArkMethod*)", "H:Ark runtime compiler cache lookup", "JIT MemoryRegion Setup",
		"PreCompileCache", "Choreographer#doFrame",
	} {
		if class := TraceSpanSemanticClass(name); class != "" {
			t.Errorf("%q must stay unclassified, got %q", name, class)
		}
		if _, ok := TraceSpanVocabulary(name); ok {
			t.Errorf("%q must not be a vocabulary row", name)
		}
	}
}

// Operator-configured patterns are consulted after the vocabulary: they can
// still classify a name nobody knows, but they cannot reclassify a name the
// vocabulary mints, nor give a class to a name it recognises without minting.
func TestHMC376OperatorPatternsComeAfterTheVocabulary(t *testing.T) {
	SetSemanticSpanPatterns([]SemanticSpanPattern{
		{SemanticClass: "gc_pause", Contains: []string{"concurrent copying gc", "gc::processworkloads"}},
		{SemanticClass: "class_verification", Contains: []string{"ddgr createshadermodules"}},
		{SemanticClass: "shader_compile", Contains: []string{"ArkPipelineWarm"}},
	})
	t.Cleanup(func() { SetSemanticSpanPatterns(nil) })
	for name, want := range map[string]string{
		// A name the vocabulary only recognises may be classified by an
		// operator pattern: it carries no class of its own.
		"Background concurrent copying GC": "gc_pause",
		"H:GC::ProcessWorkloads":           "gc_pause",
		// A row that mints stays ahead of every operator pattern.
		"H:DDGR CreateShaderModules pipelineKey = 1": "shader_compile",
		"ArkPipelineWarm stage 2":                    "shader_compile",
	} {
		if got := TraceSpanSemanticClass(name); got != want {
			t.Errorf("%q: class %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]traceSpanSemanticOwner{
		"ArkPipelineWarm stage 2":                    traceSpanSemanticOwnerOperatorPattern,
		"Background concurrent copying GC":           traceSpanSemanticOwnerOperatorPattern,
		"H:GC::ProcessWorkloads":                     traceSpanSemanticOwnerOperatorPattern,
		"H:DDGR CreateShaderModules pipelineKey = 1": traceSpanSemanticOwnerPlatformVocabulary,
	} {
		if _, owner, _ := traceSpanSemanticWorkClassWithOwner(name); owner != want {
			t.Errorf("%q: owner %d, want %d", name, owner, want)
		}
	}
	for _, name := range []string{"ArkPipelineWarm stage 2", "Background concurrent copying GC"} {
		if semanticSpanMintedByPlatformVocabulary(name) {
			t.Errorf("%q: an operator-classified name sits in the built-in seat pool, as before", name)
		}
	}
	// Without a pattern the recognised name stays unclassified.
	SetSemanticSpanPatterns(nil)
	if got := TraceSpanSemanticClass("Background concurrent copying GC"); got != "" {
		t.Fatalf("a collector cycle carries no class of its own, got %q", got)
	}
}

// --- trace-chain prefix shape ----------------------------------------------------

func TestHMC376TraceChainPrefixShape(t *testing.T) {
	for name, want := range map[string]string{
		// Stripped: one marker prefix, one chain prefix of three hex runs.
		"H:[b34913e60932d42,d37980,3bdde2c]#PartialGC::RunPhases0": "PartialGC::RunPhases0",
		"H:[3,391ef62,1283793]#FullGC::Finish":                     "FullGC::Finish",
		"[a92abafdf8ae06e,0,0]#X":                                  "X",
		"h:FullGC::RunPhases":                                      "FullGC::RunPhases",
		"  H: FullGC::RunPhases  ":                                 "FullGC::RunPhases",
		"H:[0123456789abcdef,0,0]#X":                               "X",
		// Not a chain prefix: left in place.
		"H:[Snapshot]#FullGC::RunPhases":                                "[Snapshot]#FullGC::RunPhases",
		"H:[100012]HpaeAudioFormatConverterNode::DoProcess rate[48000]": "[100012]HpaeAudioFormatConverterNode::DoProcess rate[48000]",
		"[1,2]#x":                   "[1,2]#x",
		"[zz,1,2]#x":                "[zz,1,2]#x",
		"[1,2,3,4]#x":               "[1,2,3,4]#x",
		"[,1,2]#x":                  "[,1,2]#x",
		"[1,2,]#x":                  "[1,2,]#x",
		"[0123456789abcdef0,1,2]#x": "[0123456789abcdef0,1,2]#x",
		"[A1,2,3]#x":                "[A1,2,3]#x",
		"[1, 2,3]#x":                "[1, 2,3]#x",
		"[1,2,3]x":                  "[1,2,3]x",
		// At most once each.
		"H:H:FullGC::RunPhases":               "H:FullGC::RunPhases",
		"H:[1,2,3]#[4,5,6]#FullGC::RunPhases": "[4,5,6]#FullGC::RunPhases",
		"":                                    "",
	} {
		if got := semanticVocabOperationHead(name); got != want {
			t.Errorf("operation head of %q = %q, want %q", name, got, want)
		}
	}
}

// --- wait-lane bar ----------------------------------------------------------------

// A wait recorded on a thread other than the target never takes the host-edge
// lane, on both faces (family fold and single span). Work executed on the same
// host, and a built-in class on the same host, take it exactly as before. The
// target's own wait, and a wait that intersects a typed chain window of its
// thread, keep their lanes.
func TestHMC376WaitOnAnotherThreadNeverTakesHostEdgeLane(t *testing.T) {
	chain := r3SyntheticChain() // target app-100; relay-200 is a chain node; hostw-300 holds a direct edge at 6.006
	host := ThreadRef{Comm: "hostw", PID: 300}
	relay := ThreadRef{Comm: "relay", PID: 200}
	target := ThreadRef{Comm: "app", PID: 100}
	span := func(thread ThreadRef, name string, start, end float64) TraceSpanSummary {
		return TraceSpanSummary{Thread: thread, Name: name, Kind: "sync", SemanticClass: traceSpanSemanticClass(name),
			StartTs: start, EndTs: end, DurationMs: (end - start) * 1000, StartLine: 30, EndLine: 32}
	}
	const wait, work, builtIn = "GC: Wait For Completion Alloc", "H:FullGC::RunPhases;GCReason2", "VerifyClass com.example.Foo"
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}
	for _, c := range []struct {
		label     string
		span      TraceSpanSummary
		lane      semanticSpanChainLane
		onChain   bool
		wantBasis string
	}{
		{"wait on a host with a wakeup edge", span(host, wait, 6.001, 6.002), semanticSpanLaneOff, false, ""},
		{"shared-heap wait on a host with a wakeup edge", span(host, "H:SuspendTime::WaitGCFinished", 6.001, 6.002), semanticSpanLaneOff, false, ""},
		{"work executed on the same host", span(host, work, 6.001, 6.002), semanticSpanLaneChainEdgeAnchored, true, RootCauseOnChainBasisHostWakeupEdge},
		{"built-in class on the same host", span(host, builtIn, 6.001, 6.002), semanticSpanLaneChainEdgeAnchored, true, RootCauseOnChainBasisHostWakeupEdge},
		{"wait on a chain member outside its chain window", span(relay, wait, 6.0041, 6.0044), semanticSpanLaneOff, false, ""},
		{"wait on a chain member inside its chain window", span(relay, wait, 6.001, 6.002), semanticSpanLaneChainOverlap, true, RootCauseOnChainBasisSemanticChainIntervalRelation},
		{"the target's own wait inside its chain window", span(target, wait, 6.001, 6.002), semanticSpanLaneChainOverlap, true, ""},
		{"the target's own wait outside any chain window", span(target, wait, 6.007, 6.008), semanticSpanLaneChainSelf, true, RootCauseOnChainBasisSelfDeterministicSpan},
	} {
		if lane, _, _ := semanticSpanFamilyLane(&chain, c.span); lane != c.lane {
			t.Errorf("%s: family lane %d, want %d", c.label, lane, c.lane)
		}
		projection, _ := semanticTraceSpanProjectionForRootCause(q, chain, c.span)
		if projection.OnChain != c.onChain || projection.OnChainBasis != c.wantBasis {
			t.Errorf("%s: single-span projection on_chain=%v basis=%q, want %v %q", c.label, projection.OnChain, projection.OnChainBasis, c.onChain, c.wantBasis)
		}
		if !c.onChain && projection.ImpactMs <= 0 {
			t.Errorf("%s: an off-chain span still mints its in-window extent for the background board", c.label)
		}
	}
}

// --- lower-bound caveat -------------------------------------------------------------

func TestHMC376LowerBoundCaveatCountsBuiltInPoolOnly(t *testing.T) {
	var spans []TraceSpanSummary
	add := func(n int, name string) {
		for i := 0; i < n; i++ {
			spans = append(spans, TraceSpanSummary{Name: name, SemanticClass: traceSpanSemanticClass(name), Thread: ThreadRef{Comm: "t", PID: 500 + len(spans)}})
		}
	}
	add(12, "VerifyClass com.example.Foo")
	add(4, "GC: Wait For Completion Alloc")
	if caveat, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans}); ok {
		t.Fatalf("12 built-in + 4 platform spans do not fill the 16 built-in seats: %q", caveat)
	}
	add(4, "VerifyClass com.example.Bar")
	caveat, ok := semanticSpanCapLowerBoundCaveat(WindowStats{TraceSpans: spans})
	if !ok || !strings.Contains(caveat, "the semantic span capacity (16) is exactly full") {
		t.Fatalf("16 built-in spans fill the pool and must say so: %q ok=%v", caveat, ok)
	}
}

// --- seat pools ---------------------------------------------------------------------

// hmc376PreviousBound is the selection as it was before the platform seat
// pool existed, kept here as the reference the built-in pool is compared
// with. It must not be edited to follow the production code.
func hmc376PreviousBound(spans []TraceSpanSummary, max int) ([]TraceSpanSummary, traceMarkSemanticBoundInfo) {
	if max <= 0 || len(spans) <= max {
		return spans, traceMarkSemanticBoundInfo{}
	}
	var generic, semantic []TraceSpanSummary
	for _, span := range spans {
		if _, ok := traceSpanSemanticWorkClass(span.Name); ok {
			semantic = append(semantic, span)
		} else {
			generic = append(generic, span)
		}
	}
	if len(generic) > max {
		generic = generic[:max]
	}
	info := traceMarkSemanticBoundInfo{totalSpans: len(semantic)}
	if len(semantic) > traceMarkSemanticSpanCap {
		type semanticSeat struct {
			span     TraceSpanSummary
			family   string
			selected bool
		}
		seats := make([]semanticSeat, len(semantic))
		familySeen := map[string]bool{}
		familyKept := map[string]bool{}
		for i, span := range semantic {
			class := firstNonEmpty(strings.TrimSpace(span.SemanticClass), traceSpanSemanticClass(span.Name))
			family := threadKey(span.Thread) + "\x00" + class + "\x00" + span.Subcategory
			seats[i] = semanticSeat{span: span, family: family}
			familySeen[family] = true
			if familyKept[family] || info.keptSpans >= traceMarkSemanticSpanCap {
				continue
			}
			seats[i].selected = true
			familyKept[family] = true
			info.keptSpans++
		}
		for i := range seats {
			if info.keptSpans >= traceMarkSemanticSpanCap {
				break
			}
			if seats[i].selected {
				continue
			}
			seats[i].selected = true
			info.keptSpans++
		}
		semantic = semantic[:0]
		omittedSeen := map[string]bool{}
		for _, seat := range seats {
			if seat.selected {
				semantic = append(semantic, seat.span)
				continue
			}
			if familyKept[seat.family] || omittedSeen[seat.family] {
				continue
			}
			omittedSeen[seat.family] = true
			info.omittedFamilies++
			if len(info.omittedRoster) < 8 {
				omittedClass := firstNonEmpty(strings.TrimSpace(seat.span.SemanticClass), traceSpanSemanticClass(seat.span.Name), "unknown_class")
				if strings.HasPrefix(seat.span.Subcategory, "shader_cache_") {
					omittedClass += "(" + strings.TrimPrefix(seat.span.Subcategory, "shader_") + ")"
				}
				info.omittedRoster = append(info.omittedRoster, fmt.Sprintf("%s@%s", omittedClass, threadLabel(seat.span.Thread)))
			}
		}
		info.totalFamilies = len(familySeen)
	} else {
		families := map[string]bool{}
		for _, span := range semantic {
			class := firstNonEmpty(strings.TrimSpace(span.SemanticClass), traceSpanSemanticClass(span.Name))
			families[threadKey(span.Thread)+"\x00"+class+"\x00"+span.Subcategory] = true
		}
		info.totalFamilies = len(families)
		info.keptSpans = len(semantic)
	}
	out := make([]TraceSpanSummary, 0, len(generic)+len(semantic))
	out = append(out, generic...)
	out = append(out, semantic...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DurationMs != out[j].DurationMs {
			return out[i].DurationMs > out[j].DurationMs
		}
		return out[i].StartLine < out[j].StartLine
	})
	return out, info
}

func hmc376SeatsFor(tid int) semanticSeatQuery {
	return semanticSeatQuery{target: ThreadRef{PID: tid}}
}

func hmc376SortSpans(spans []TraceSpanSummary) {
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].DurationMs != spans[j].DurationMs {
			return spans[i].DurationMs > spans[j].DurationMs
		}
		return spans[i].StartLine < spans[j].StartLine
	})
}

func hmc376Population(rng *rand.Rand, count, threads int, names []string, firstLine int) []TraceSpanSummary {
	spans := make([]TraceSpanSummary, 0, count)
	for i := 0; i < count; i++ {
		name := names[rng.Intn(len(names))]
		tid := 1000 + rng.Intn(threads)
		span := TraceSpanSummary{
			Thread: ThreadRef{Comm: fmt.Sprintf("t%d", tid), PID: tid}, Name: name, Kind: "sync",
			Category: traceSpanCategory(name), Subcategory: traceSpanSubcategory(name), SemanticClass: traceSpanSemanticClass(name),
			// A small duration alphabet produces ties, so the line order
			// matters too.
			DurationMs: float64(1+rng.Intn(40)) / 4, StartLine: firstLine + i, EndLine: firstLine + i,
		}
		if span.SemanticClass == "shader_compile" && rng.Intn(3) == 0 {
			span.Subcategory = "shader_cache_hit"
		}
		spans = append(spans, span)
	}
	return spans
}

// For any window without a platform span the bound is what it was: same kept
// spans in the same order, same disclosure. With platform spans added, the
// spans that were classified before keep exactly the seats they had, and the
// generic spans keep theirs.
func TestHMC376BuiltInSeatSelectionMatchesPreviousAlgorithm(t *testing.T) {
	previousNames := []string{
		"VerifyClass com.example.A", "VerifyClass com.example.B", "JIT compiling void a.b() (kind=Baseline)",
		"H:Ark runtime compile", "shader_compile", "Texture upload(1) 2x2", "GC pause", "H:GC concurrent mark",
		"Choreographer#doFrame", "H:FlushLayoutTask", "binder transaction", "H:GC::MarkRoots", "CopyingPhase",
	}
	platformNames := []string{
		"H:FullGC::RunPhases;GCReason2", "H:PartialGC::RunPhases0;GCReason12", "GC: Wait For Completion Alloc",
		"H:SuspendTime::WaitGCFinished", "H:DDGR CreateShaderModules pipelineKey = 1", "vkCreateGraphicsPipelines",
	}
	rng := rand.New(rand.NewSource(376))
	sawFullPool, sawOmittedFamily := false, false
	for trial := 0; trial < 400; trial++ {
		before := hmc376Population(rng, rng.Intn(90), 1+rng.Intn(30), previousNames, 1)
		hmc376SortSpans(before)
		wantOut, wantInfo := hmc376PreviousBound(append([]TraceSpanSummary(nil), before...), 8)
		gotOut, gotInfo := boundTraceMarkSpansForTarget(append([]TraceSpanSummary(nil), before...), 8, hmc376SeatsFor(1000))
		if !reflect.DeepEqual(gotOut, wantOut) || !reflect.DeepEqual(gotInfo, wantInfo) {
			t.Fatalf("trial %d: a window without platform spans is bounded differently than before:\n got %+v\nwant %+v", trial, gotInfo, wantInfo)
		}
		sawFullPool = sawFullPool || wantInfo.totalSpans > traceMarkSemanticSpanCap
		sawOmittedFamily = sawOmittedFamily || wantInfo.omittedFamilies > 0

		with := append(append([]TraceSpanSummary(nil), before...), hmc376Population(rng, 1+rng.Intn(30), 1+rng.Intn(30), platformNames, 10000)...)
		hmc376SortSpans(with)
		mixed, _ := boundTraceMarkSpansForTarget(append([]TraceSpanSummary(nil), with...), 8, hmc376SeatsFor(1000))
		var keptBefore, keptAfter []TraceSpanSummary
		for _, span := range wantOut {
			keptBefore = append(keptBefore, span)
		}
		platformKept := 0
		for _, span := range mixed {
			if semanticSpanMintedByPlatformVocabulary(span.Name) {
				platformKept++
				continue
			}
			keptAfter = append(keptAfter, span)
		}
		if !reflect.DeepEqual(keptAfter, keptBefore) {
			t.Fatalf("trial %d: platform spans changed the seats of previously classified or generic spans", trial)
		}
		if platformKept > traceMarkPlatformSemanticSpanCap {
			t.Fatalf("trial %d: %d platform spans kept, cap is %d", trial, platformKept, traceMarkPlatformSemanticSpanCap)
		}
	}
	if !sawFullPool || !sawOmittedFamily {
		t.Fatalf("the trials must exercise a full built-in pool and a wholly omitted family: full=%v omitted=%v", sawFullPool, sawOmittedFamily)
	}
}

// Inside the platform pool the query target is seated first, then work
// executed on other threads, then waits recorded on other threads — whatever
// their durations.
func TestHMC376PlatformSeatPoolSeatsTargetFirst(t *testing.T) {
	var spans []TraceSpanSummary
	add := func(tid int, name string, ms float64) {
		spans = append(spans, TraceSpanSummary{Thread: ThreadRef{Comm: fmt.Sprintf("t%d", tid), PID: tid}, Name: name,
			SemanticClass: traceSpanSemanticClass(name), DurationMs: ms, StartLine: len(spans) + 1, EndLine: len(spans) + 1})
	}
	for i := 0; i < 20; i++ {
		add(200+i, "GC: Wait For Completion Alloc", 300+float64(i)) // siblings wait longest
	}
	for i := 0; i < 3; i++ {
		add(300+i, "H:FullGC::RunPhases;GCReason2", 50+float64(i)) // work on other threads
	}
	add(100, "GC: Wait For Completion Alloc", 2) // the target's own wait is the shortest span
	add(100, "H:PartialGC::RunPhases0;GCReason12", 1)
	for i := 0; i < 9; i++ {
		add(400+i, "Choreographer#doFrame", 500) // generic spans force the bound
	}
	hmc376SortSpans(spans)
	out, info := boundTraceMarkSpansForTarget(spans, 8, hmc376SeatsFor(100))
	var kept []string
	for _, span := range out {
		if semanticSpanMintedByPlatformVocabulary(span.Name) {
			kept = append(kept, fmt.Sprintf("%d %.0f", span.Thread.PID, span.DurationMs))
		}
	}
	sort.Strings(kept)
	// The target tier is seated completely first (both members of its one
	// family), then the three working threads, then the longest sibling waits
	// in the seats that are left.
	want := []string{"100 1", "100 2", "217 317", "218 318", "219 319", "300 50", "301 51", "302 52"}
	if !reflect.DeepEqual(kept, want) {
		t.Fatalf("platform seats:\n got %v\nwant %v", kept, want)
	}
	if info.totalSpans != 25 || info.keptSpans != 8 || info.totalFamilies != 24 || info.omittedFamilies != 17 || len(info.omittedRoster) != 8 {
		t.Fatalf("disclosure must count every omitted sibling family exactly: %+v", info)
	}
	// Without a known target the first tier is empty: thread 100 is seated
	// as one more working thread (its collection), and its wait — the
	// shortest wait of the window — has no claim to a seat.
	noTarget, _ := boundTraceMarkSpansForTarget(spans, 8, hmc376SeatsFor(0))
	kept = kept[:0]
	for _, span := range noTarget {
		if semanticSpanMintedByPlatformVocabulary(span.Name) {
			kept = append(kept, fmt.Sprintf("%d %.0f", span.Thread.PID, span.DurationMs))
		}
	}
	sort.Strings(kept)
	want = []string{"100 1", "216 316", "217 317", "218 318", "219 319", "300 50", "301 51", "302 52"}
	if !reflect.DeepEqual(kept, want) {
		t.Fatalf("platform seats without a target:\n got %v\nwant %v", kept, want)
	}

	// Spare seats go to the target's further members before anyone else's.
	spans = spans[:0]
	for i := 0; i < 5; i++ {
		add(100, "H:PartialGC::RunPhases0;GCReason12", 1+float64(i))
	}
	for i := 0; i < 4; i++ {
		add(300, "H:FullGC::RunPhases;GCReason2", 60+float64(i))
	}
	add(200, "GC: Wait For Completion Alloc", 300)
	add(201, "GC: Wait For Completion Alloc", 301)
	hmc376SortSpans(spans)
	out, info = boundTraceMarkSpansForTarget(spans, 8, hmc376SeatsFor(100))
	perThread := map[int]int{}
	for _, span := range out {
		perThread[span.Thread.PID]++
	}
	if !reflect.DeepEqual(perThread, map[int]int{100: 5, 300: 1, 200: 1, 201: 1}) || info.keptSpans != 8 || info.totalSpans != 11 || info.omittedFamilies != 0 {
		t.Fatalf("spare seats must complete the target's family first: %v %+v", perThread, info)
	}
}

// A family is the same family in both pools: it is counted once, and it is
// "wholly omitted" only when it kept no member in either pool.
func TestHMC376BoundDisclosureCountsFamiliesAcrossPools(t *testing.T) {
	var spans []TraceSpanSummary
	add := func(tid int, name string, ms float64) {
		spans = append(spans, TraceSpanSummary{Thread: ThreadRef{Comm: fmt.Sprintf("t%d", tid), PID: tid}, Name: name,
			Subcategory: traceSpanSubcategory(name), SemanticClass: traceSpanSemanticClass(name), DurationMs: ms, StartLine: len(spans) + 1, EndLine: len(spans) + 1})
	}
	add(700, "H:Load/Compile Shaders", 40)   // built-in member of (t700, shader_compile)
	add(700, "vkCreateGraphicsPipelines", 1) // platform member of the same family; the shortest platform span
	for i := 0; i < 8; i++ {
		add(800+i, "H:FullGC::RunPhases;GCReason2", 10+float64(i)) // eight other platform families fill the pool
	}
	for i := 0; i < 9; i++ {
		add(900+i, "Choreographer#doFrame", 100)
	}
	hmc376SortSpans(spans)
	out, info := boundTraceMarkSpansForTarget(spans, 8, hmc376SeatsFor(0))
	for _, span := range out {
		if span.Name == "vkCreateGraphicsPipelines" {
			t.Fatalf("the ninth platform span must be the omitted one")
		}
	}
	if info.totalSpans != 10 || info.keptSpans != 9 || info.totalFamilies != 9 || info.omittedFamilies != 0 || len(info.omittedRoster) != 0 {
		t.Fatalf("the omitted span belongs to a family that kept its built-in member: %+v", info)
	}
	if caveat := info.caveat(); !strings.Contains(caveat, "kept 9/10 classified span(s) across 9 family/families; omitted 1 span(s) from 0 whole family/families") {
		t.Fatalf("disclosure wording drifted: %q", caveat)
	}
}

// --- seat tiers ------------------------------------------------------------------

func hmc376PlatformSpan(tid int, name string, ms, start float64) TraceSpanSummary {
	return TraceSpanSummary{Thread: ThreadRef{Comm: fmt.Sprintf("t%d", tid), PID: tid}, Name: name,
		SemanticClass: traceSpanSemanticClass(name), DurationMs: ms, StartTs: start, EndTs: start + ms/1000,
		StartLine: int(start*1000) + tid, EndLine: int(start*1000) + tid}
}

func hmc376KeptPlatformThreads(spans []TraceSpanSummary) map[int]int {
	kept := map[int]int{}
	for _, span := range spans {
		if semanticSpanMintedByPlatformVocabulary(span.Name) {
			kept[span.Thread.PID]++
		}
	}
	return kept
}

// A chain member's span that intersects one of its typed chain windows is
// seated with the target; the same thread's span outside the window is not.
func TestHMC376ChainWindowSpansAreSeatedWithTheTarget(t *testing.T) {
	const wait, work = "GC: Wait For Completion Alloc", "H:FullGC::RunPhases;GCReason2"
	seats := semanticSeatQuery{target: ThreadRef{PID: 100}, chainWindows: map[int][]TimeWindow{500: {{StartTs: 10.0, EndTs: 10.1}}}}
	for _, c := range []struct {
		label string
		span  TraceSpanSummary
		want  int
	}{
		{"the target's own wait", hmc376PlatformSpan(100, wait, 5, 20), 0},
		{"a chain member's wait inside its window", hmc376PlatformSpan(500, wait, 5, 10.02), 0},
		{"a chain member's work inside its window", hmc376PlatformSpan(500, work, 5, 10.02), 0},
		{"a chain member's work outside its window", hmc376PlatformSpan(500, work, 5, 10.5), 1},
		{"a chain member's wait outside its window", hmc376PlatformSpan(500, wait, 5, 10.5), 2},
		{"work of an unrelated thread at the same time", hmc376PlatformSpan(600, work, 5, 10.02), 1},
		{"a wait of an unrelated thread at the same time", hmc376PlatformSpan(600, wait, 5, 10.02), 2},
	} {
		if got := semanticSpanSeatTier(c.span, seats); got != c.want {
			t.Errorf("%s: tier %d, want %d", c.label, got, c.want)
		}
	}
	var spans []TraceSpanSummary
	spans = append(spans, hmc376PlatformSpan(500, wait, 1, 10.02))
	for i := 0; i < 12; i++ {
		spans = append(spans, hmc376PlatformSpan(700+i, work, 300+float64(i), 10))
	}
	for i := 0; i < 9; i++ {
		spans = append(spans, hmc376PlatformSpan(900+i, "Choreographer#doFrame", 500, 10))
	}
	hmc376SortSpans(spans)
	out, _ := boundTraceMarkSpansForTarget(spans, 8, seats)
	if kept := hmc376KeptPlatformThreads(out); kept[500] != 1 {
		t.Fatalf("a short on-chain wait lost its seat to longer unrelated collections: %v", kept)
	}
}

// The target is the identity the query resolves to, however it was addressed.
func TestHMC376SeatTargetIsTheResolvedQueryTarget(t *testing.T) {
	var b strings.Builder
	b.WriteString("# tracer: nop\n")
	b.WriteString("            main-100   (    100) [001] ....  10.000000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=main next_pid=100 next_prio=120\n")
	b.WriteString("          worker-200   (    100) [002] ....  10.001000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120\n")
	b.WriteString("          worker-300   (    100) [003] ....  10.002000: sched_switch: prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=300 next_prio=120\n")
	path := filepath.Join(t.TempDir(), "synthetic.ftrace")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(q Query) ThreadRef {
		seats := semanticSeatQueryFor(idx, normalizeQuery(idx, q))
		if seats.resolve != nil {
			return seats.resolve()
		}
		return seats.target
	}
	for label, q := range map[string]Query{
		"thread id":        {PID: 100},
		"bare thread name": {Thread: "main"},
		"name and id":      {Thread: "main-100"},
		"selector with id": {ThreadInput: "pid=100"},
	} {
		if got := resolve(q); got.PID != 100 {
			t.Errorf("%s: resolved target %+v", label, got)
		}
	}
	// An ambiguous name has no target: no thread is favoured.
	if got := resolve(Query{Thread: "worker"}); got.PID != 0 {
		t.Errorf("an ambiguous name must not pick a target: %+v", got)
	}
}

// Every member of the target's family is seated before any other thread's
// representative.
func TestHMC376TargetMembersAreSeatedBeforeOtherThreads(t *testing.T) {
	const work = "H:PartialGC::RunPhases0;GCReason12"
	var spans []TraceSpanSummary
	for i := 0; i < 10; i++ {
		spans = append(spans, hmc376PlatformSpan(100, work, 10, 1+float64(i)))
	}
	for i := 0; i < 12; i++ {
		spans = append(spans, hmc376PlatformSpan(300+i, work, 5, 1))
	}
	for i := 0; i < 9; i++ {
		spans = append(spans, hmc376PlatformSpan(900+i, "Choreographer#doFrame", 500, 1))
	}
	hmc376SortSpans(spans)
	out, info := boundTraceMarkSpansForTarget(spans, 8, hmc376SeatsFor(100))
	if kept := hmc376KeptPlatformThreads(out); !reflect.DeepEqual(kept, map[int]int{100: 8}) {
		t.Fatalf("shorter spans of other threads took seats from the target: %v", kept)
	}
	if info.totalSpans != 22 || info.keptSpans != 8 || info.omittedFamilies != 12 {
		t.Fatalf("the omitted families must be disclosed: %+v", info)
	}
}

// --- later bounded faces -----------------------------------------------------------

func hmc376BoardRow(typ string, tid int, effective float64, relevance string, vocabulary bool) RootCauseRankItem {
	return RootCauseRankItem{Type: typ, Thread: ThreadRef{Comm: fmt.Sprintf("t%d", tid), PID: tid}, EffectiveImpactMs: effective,
		ImpactMs: effective, ChainRelevance: relevance, platformVocabularyMinted: vocabulary}
}

// An off-chain row of vocabulary-classified work sorts behind every other row
// and survives the board cap last; an on-chain one competes normally and is
// additional to the cap.
func TestHMC376BoardSeatsYieldToPreviouslyPublishedRows(t *testing.T) {
	var items []RootCauseRankItem
	for i := 0; i < 12; i++ {
		items = append(items, hmc376BoardRow("runnable_wait", 100+i, 1+float64(i), "background", false))
	}
	items = append(items,
		hmc376BoardRow("gc_pause", 300, 500, "background", true),
		hmc376BoardRow("gc_pause", 301, 400, "adjacent", true),
		hmc376BoardRow("gc_pause", 302, 300, "on_chain", true))
	sortRootCauseRankItems(items, true)
	if items[0].Thread.PID != 302 {
		t.Fatalf("an on-chain vocabulary row competes by value: %+v", items[0])
	}
	if items[len(items)-1].Thread.PID != 301 && items[len(items)-1].Thread.PID != 300 {
		t.Fatalf("off-chain vocabulary rows must sort last: %+v", items[len(items)-1])
	}
	survivors, dead := selectRootCauseRankCapSurvivors(items, 12)
	kept := map[int]bool{}
	for _, row := range survivors {
		kept[row.Thread.PID] = true
	}
	for i := 0; i < 12; i++ {
		if !kept[100+i] {
			t.Fatalf("row %d lost its board seat to a vocabulary row: survivors=%v", 100+i, kept)
		}
	}
	if !kept[302] || kept[300] || kept[301] || len(survivors) != 13 || len(dead) != 2 {
		t.Fatalf("the on-chain vocabulary row is additional to the cap and off-chain ones wait for free seats: survivors=%v dead=%d", kept, len(dead))
	}
	// A free seat is filled.
	survivors, dead = selectRootCauseRankCapSurvivors(items, 14)
	if len(survivors) != 15 || len(dead) != 0 {
		t.Fatalf("off-chain vocabulary rows take seats nobody else needs: survivors=%d dead=%d", len(survivors), len(dead))
	}
}

// Off the chain, a vocabulary-classified span never folds into the family of a
// span classified without the vocabulary, and its family comes last.
func TestHMC376OffChainVocabularyFamiliesFoldApartAndLast(t *testing.T) {
	chain := r3SyntheticChain()
	const builtIn, vocabulary = "H:Load/Compile Shaders", "H:vkCreateGraphicsPipelines"
	other := ThreadRef{Comm: "other", PID: 900}
	mk := func(name string, start float64) TraceSpanSummary {
		return TraceSpanSummary{Thread: other, Name: name, Kind: "sync", SemanticClass: traceSpanSemanticClass(name), Subcategory: "shader_compile",
			StartTs: start, EndTs: start + 0.002, DurationMs: 2, StartLine: int(start * 1000), EndLine: int(start * 1000)}
	}
	families := FoldSemanticSpanFamilies(&chain, []TraceSpanSummary{mk(vocabulary, 6.001), mk(builtIn, 6.004), mk(builtIn, 6.007)})
	if len(families) != 2 {
		t.Fatalf("expected two families, got %d", len(families))
	}
	if families[0].platformVocabularyMinted || len(families[0].Members) != 2 || !hmc376Near(families[0].TotalMs, 4) {
		t.Fatalf("the family classified without the vocabulary keeps its value and comes first: %+v", families[0])
	}
	if !families[1].platformVocabularyMinted || len(families[1].Members) != 1 {
		t.Fatalf("the vocabulary family comes last: %+v", families[1])
	}
}

func hmc376Near(got, want float64) bool { return got > want-0.0005 && got < want+0.0005 }

// A family the vocabulary classifies that lost every member at the seat bound
// stays visible on the business mention face; a family that holds a seat does
// not appear twice, and a wait on another thread takes no wakeup-edge credit.
func TestHMC376OmittedVocabularyFamilyStaysOnTheMentionFace(t *testing.T) {
	chain := r3SyntheticChain() // target app-100; relay-200 is a chain node; hostw-300 holds a direct edge at 6.006
	relay, host := ThreadRef{Comm: "relay", PID: 200}, ThreadRef{Comm: "hostw", PID: 300}
	const work, wait = "H:FullGC::RunPhases;GCReason2", "GC: Wait For Completion Alloc"
	mk := func(thread ThreadRef, name string, start float64) TraceSpanSummary {
		return TraceSpanSummary{Thread: thread, Name: name, Kind: "sync", SemanticClass: traceSpanSemanticClass(name),
			StartTs: start, EndTs: start + 0.002, DurationMs: 2, StartLine: int(start * 10000), EndLine: int(start*10000) + 1}
	}
	q := Query{PID: 100, TimeStart: 6.000, TimeEnd: 6.010}
	mentions := func(seated []TraceSpanSummary, inventory ...TraceSpanSummary) map[string]bool {
		res := computeBusinessSpanMentions(q, chain, wakeupChainThreadSet(chain), WindowStats{TraceSpans: seated, traceSpanFullInventory: inventory})
		out := map[string]bool{}
		if res != nil {
			for _, fam := range res.Families {
				out[fmt.Sprintf("%d %s", fam.Thread.PID, fam.Name)] = true
			}
		}
		return out
	}
	omitted := mk(relay, work, 6.001)
	if got := mentions(nil, omitted); !got["200 "+work] {
		t.Fatalf("an on-chain family without a seat must stay visible: %v", got)
	}
	other := mk(relay, work, 6.004)
	if got := mentions([]TraceSpanSummary{omitted}, omitted, other); len(got) != 0 {
		t.Fatalf("a family that holds a seat keeps its value face only: %v", got)
	}
	if got := mentions(nil, mk(host, wait, 6.001)); len(got) != 0 {
		t.Fatalf("a wait on another thread takes no wakeup-edge credit on this face either: %v", got)
	}
	if got := mentions(nil, mk(host, work, 6.001)); !got["300 "+work] {
		t.Fatalf("executed work before the wakeup edge keeps the credential: %v", got)
	}
}
