package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

type materialAwareRunner struct {
	logAwareRunner
	material      *attachment.TraceMaterial
	seenMaterials []*attachment.TraceMaterial
	seenSources   []string
	publishOrder  []string
}

func (r *materialAwareRunner) AttachedHitrace() string                          { return r.curTrace }
func (r *materialAwareRunner) AttachedHitraceSource() string                    { return r.curTraceSource }
func (r *materialAwareRunner) AttachedTraceMaterial() *attachment.TraceMaterial { return r.material }
func (r *materialAwareRunner) SetAttachedTraceMaterial(m *attachment.TraceMaterial) {
	r.material = m
	if m == nil {
		r.publishOrder = append(r.publishOrder, "material:nil")
	} else {
		r.publishOrder = append(r.publishOrder, "material:prepared")
	}
}
func (r *materialAwareRunner) SetAttachedHitrace(body string) {
	r.logAwareRunner.SetAttachedHitrace(body)
	r.material = nil // Real orchestrators withdraw old receipts on a plain setter.
	r.publishOrder = append(r.publishOrder, "raw")
}
func (r *materialAwareRunner) SetAttachedHitraceSource(source string) {
	r.logAwareRunner.SetAttachedHitraceSource(source)
	r.publishOrder = append(r.publishOrder, "source")
}
func (r *materialAwareRunner) SetReadRunSnapshotSeed(*types.ReadRunSnapshot) {}
func (r *materialAwareRunner) Run(request, repo, branch string) (*types.BusContext, error) {
	r.seenMaterials = append(r.seenMaterials, r.material)
	r.seenSources = append(r.seenSources, r.curTraceSource)
	return r.logAwareRunner.Run(request, repo, branch)
}

func replPreparedMaterial(t *testing.T) *attachment.TraceMaterial {
	t.Helper()
	path := filepath.Join(t.TempDir(), "complete.sys")
	body := strings.Repeat("# head filler\n", 100) + "waker-12 (12) [000] .... 9.000000: sched_wakeup: comm=tail pid=42 prio=120 target_cpu=000\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(material.Preview(), "comm=tail") {
		t.Fatal("fixture tail leaked into preview")
	}
	return material
}

func newMaterialREPL(t *testing.T, material *attachment.TraceMaterial) (*REPL, *materialAwareRunner, *bytes.Buffer) {
	t.Helper()
	runner := &materialAwareRunner{material: material}
	runner.curTrace, runner.curTraceSource = material.Preview(), "harmony_hitrace"
	out := &bytes.Buffer{}
	store, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatal(err)
	}
	r := New(Config{Runner: runner, Store: store, In: strings.NewReader(""), Out: out, Language: "en", Render: renderNothing})
	return r, runner, out
}

func TestPreparedTraceREPLSeedAndDispatchPreserveCompleteMaterial(t *testing.T) {
	material := replPreparedMaterial(t)
	r, runner, _ := newMaterialREPL(t, material)
	if r.attachedTraceMaterial != material || r.attachedHitrace != material.Preview() {
		t.Fatal("constructor dropped CLI complete-material receipt")
	}
	r.dispatch("inspect this attached trace", "inspect this attached trace")
	if len(runner.seenMaterials) != 1 || runner.seenMaterials[0] != material || runner.seenTraces[0] != material.Preview() {
		t.Fatalf("dispatch reduced complete material to preview: materials=%v traces=%q", runner.seenMaterials, runner.seenTraces)
	}
}

func TestPreparedTraceREPLStaleMaterialBlocksDispatchWithoutFallback(t *testing.T) {
	material := replPreparedMaterial(t)
	r, runner, out := newMaterialREPL(t, material)
	if err := os.WriteFile(material.SourcePath(), []byte("replacement trace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.dispatch("inspect this attached trace", "inspect this attached trace")
	if len(runner.seenMaterials) != 0 || !strings.Contains(out.String(), "changed") {
		t.Fatalf("stale material fell back to preview: runs=%d out=%s", len(runner.seenMaterials), out.String())
	}
}

func TestPreparedTraceREPLTextReplacementClearAndFailureLifetimes(t *testing.T) {
	material := replPreparedMaterial(t)
	r, runner, _ := newMaterialREPL(t, material)
	r.handleHitraceCmd("/htrace /missing/new.sys")
	if r.attachedTraceMaterial != material || runner.material != material {
		t.Fatal("failed new attachment removed old material")
	}
	path := filepath.Join(t.TempDir(), "new.systrace")
	if err := os.WriteFile(path, []byte("plain replacement text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.handleHitraceCmd("/htrace " + path)
	if r.attachedTraceMaterial != nil || runner.material != nil || !strings.Contains(r.attachedHitrace, "replacement") {
		t.Fatal("new text inherited old material receipt")
	}
	r.attachedHitrace, r.attachedTraceMaterial, runner.material = material.Preview(), material, material
	r.handleHitraceCmd("/htrace clear")
	if r.attachedTraceMaterial != nil || runner.material != nil || r.attachedHitrace != "" || runner.curTrace != "" {
		t.Fatal("clear retained prepared state")
	}
}

func TestPreparedTraceREPLReplacementPublishesBeforeDirectReadResume(t *testing.T) {
	for _, action := range []string{"load", "import", "clear", "failed load", "failed import"} {
		t.Run(action, func(t *testing.T) {
			material := replPreparedMaterial(t)
			r, runner, out := newMaterialREPL(t, material)
			r.readRunSnapshotStore = NewReadRunSnapshotStore(t.TempDir())
			if _, err := r.readRunSnapshotStore.Save(&types.ReadRunSnapshot{
				SchemaVersion: types.ReadRunSnapshotSchemaVersion,
				RunID:         "receipt-replacement", Request: "inspect the current attached trace",
				RepoRoot: t.TempDir(), TaskGraphHash: strings.Repeat("b", 64),
			}); err != nil {
				t.Fatal(err)
			}
			wantBody, wantSource, wantMaterial := material.Preview(), "harmony_hitrace", material
			wantOrder := ""
			switch action {
			case "load":
				path := filepath.Join(t.TempDir(), "replacement.atrace")
				body := "new text trace\n"
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				wantBody, wantSource, wantMaterial = "# codrax-source: "+path+"\n"+body, "android_atrace", nil
				r.handleSlash("/htrace " + path)
			case "import", "failed import":
				bundle := t.TempDir()
				body := "imported replacement trace\n"
				manifest := exportBundleManifest{SchemaVersion: exportBundleSchemaVersion, TraceSHA256: sha256Hex(body), TraceSource: "android_atrace"}
				if action == "failed import" {
					manifest.TraceSHA256 = strings.Repeat("0", 64)
				} else {
					wantBody, wantSource, wantMaterial = body, "android_atrace", nil
				}
				encoded, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				for name, content := range map[string][]byte{"manifest.json": encoded, "trace.txt": []byte(body)} {
					if err := os.WriteFile(filepath.Join(bundle, name), content, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				r.handleSlash("/import " + bundle)
			case "clear":
				wantBody, wantSource, wantMaterial = "", "", nil
				r.handleSlash("/htrace clear")
			case "failed load":
				r.handleSlash("/htrace " + filepath.Join(t.TempDir(), "missing.sys"))
			}
			if !strings.HasPrefix(action, "failed ") {
				wantOrder = "raw,source,material:nil"
			}
			if got := strings.Join(runner.publishOrder, ","); got != wantOrder {
				t.Fatalf("publication order=%q want=%q", got, wantOrder)
			}
			if r.attachedHitrace != wantBody || r.attachedHitraceSource != wantSource || r.attachedTraceMaterial != wantMaterial ||
				runner.curTrace != wantBody || runner.curTraceSource != wantSource || runner.material != wantMaterial {
				t.Fatalf("replacement did not synchronize attachment tuple: repl=(%q,%q,%p) runner=(%q,%q,%p)",
					r.attachedHitrace, r.attachedHitraceSource, r.attachedTraceMaterial, runner.curTrace, runner.curTraceSource, runner.material)
			}
			// This command calls Runner.Run without ordinary dispatch's trace
			// propagation. It must see the replacement, never old preview+nil.
			if r.handleSlash("/read-runs resume receipt-replacement") {
				t.Fatal("read resume requested exit")
			}
			if len(runner.seenTraces) != 1 || runner.seenTraces[0] != wantBody || runner.seenSources[0] != wantSource || runner.seenMaterials[0] != wantMaterial {
				t.Fatalf("direct Run observed stale attachment: traces=%q sources=%q materials=%v out=%s", runner.seenTraces, runner.seenSources, runner.seenMaterials, out.String())
			}
		})
	}
}

func TestPreparedTraceREPLDurablePreviewRequiresReattachment(t *testing.T) {
	material := replPreparedMaterial(t)
	r, _, _ := newMaterialREPL(t, material)
	r.runtimeArtifactStore = NewRuntimeArtifactStore(filepath.Join(t.TempDir(), "artifacts"))
	snapshot := r.persistCurrentRuntimeArtifactSnapshot()
	if !snapshot.Trace.TraceRequiresReattach || snapshot.Trace.TraceOriginalPath != material.SourcePath() || snapshot.Trace.SchemaVersion != runtimeArtifactPreparedSchemaVersion {
		t.Fatalf("missing restore restriction: %+v", snapshot.Trace)
	}
	stored, err := r.runtimeArtifactStore.LoadLatest()
	if err != nil || !stored.Trace.TraceRequiresReattach || stored.SchemaVersion != runtimeArtifactPreparedSchemaVersion {
		t.Fatalf("disk metadata lost restriction: %+v %v", stored, err)
	}
	if body, err := r.runtimeArtifactStore.Load(stored.Trace, 1024); err == nil || body != "" {
		t.Fatalf("preview restored as complete capture: %q %v", body, err)
	}
	plain, err := r.runtimeArtifactStore.Put("trace", strings.TrimSpace(material.Preview()), "harmony_hitrace")
	if err != nil || plain.ID == stored.Trace.ID || plain.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		t.Fatalf("prepared and plain metadata identities collided: plain=%+v prepared=%+v err=%v", plain, stored.Trace, err)
	}
	freshOut := &bytes.Buffer{}
	fresh := New(Config{In: strings.NewReader(""), Out: freshOut, Language: "en", RuntimeArtifactStore: r.runtimeArtifactStore})
	fresh.maybeRestoreRuntimeArtifactForPolicy(TurnPolicy{Route: RouteRepo, Source: "prior_context"})
	if fresh.attachedHitrace != "" || fresh.attachedTraceMaterial != nil || !strings.Contains(freshOut.String(), "Reattach original file") {
		t.Fatalf("unsafe durable restoration: trace=%q out=%s", fresh.attachedHitrace, freshOut.String())
	}
}

func TestPreparedTraceREPLExportCannotImportPreviewAsCompleteCapture(t *testing.T) {
	material := replPreparedMaterial(t)
	r, _, _ := newMaterialREPL(t, material)
	r.runtimeAnchor = t.TempDir()
	r.attachedLog = "new exported log"
	r.handleExportCmd("/export")
	entries, err := os.ReadDir(filepath.Join(r.runtimeAnchor, "exports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("export directories=%v err=%v", entries, err)
	}
	bundle := filepath.Join(r.runtimeAnchor, "exports", entries[0].Name())
	body, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest exportBundleManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != exportPreparedTraceSchemaVersion || !manifest.TraceRequiresReattach || manifest.TraceOriginalPath != material.SourcePath() {
		t.Fatalf("export lost preview restriction: %+v", manifest)
	}
	importer, runner, out := newMaterialREPL(t, material)
	importer.attachedLog = "existing log"
	importer.handleImportCmd("/import " + bundle)
	if importer.attachedHitrace != material.Preview() || importer.attachedTraceMaterial != material || runner.material != material || importer.attachedLog != "existing log" || !strings.Contains(out.String(), "Reattach original file") {
		t.Fatalf("preview-only import changed attachments: trace=%q log=%q out=%s", importer.attachedHitrace, importer.attachedLog, out.String())
	}
}

func TestPreparedTraceREPLPlainImportClearsPriorReceipt(t *testing.T) {
	material := replPreparedMaterial(t)
	r, runner, _ := newMaterialREPL(t, material)
	bundle := t.TempDir()
	trace := "ordinary imported text trace\n"
	manifest, err := json.Marshal(exportBundleManifest{SchemaVersion: exportBundleSchemaVersion, TraceSHA256: sha256Hex(trace), TraceSource: "android_atrace"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "trace.txt"), []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	r.handleImportCmd("/import " + bundle)
	if r.attachedHitrace != trace || r.attachedTraceMaterial != nil || runner.material != nil {
		t.Fatalf("plain import inherited prepared receipt: trace=%q material=%v runner=%v", r.attachedHitrace, r.attachedTraceMaterial, runner.material)
	}
}

func TestPreparedTraceREPLCompleteCLITextKeepsLegacyPersistenceAndExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "complete-small.sys")
	body := "worker-12 (12) [000] .... 9.000000: sched_wakeup: comm=tail pid=42 prio=120 target_cpu=000\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	// This is the same preparation factory used by CLI file attachment; New
	// then imports its preview + receipt from the preconfigured runner.
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 4096})
	if err != nil || !material.SelfContainedText() {
		t.Fatalf("complete text preparation=%v err=%v", material, err)
	}
	r, _, _ := newMaterialREPL(t, material)
	r.runtimeAnchor = t.TempDir()
	r.runtimeArtifactStore = NewRuntimeArtifactStore(filepath.Join(r.runtimeAnchor, "artifacts"))
	snapshot := r.persistCurrentRuntimeArtifactSnapshot()
	if snapshot.Trace.TraceRequiresReattach || snapshot.Trace.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		t.Fatalf("ordinary complete text was demoted to preview-only: %+v", snapshot.Trace)
	}
	latest, err := r.runtimeArtifactStore.LoadLatest()
	if err != nil || latest.SchemaVersion != runtimeArtifactStoreSchemaVersion {
		t.Fatalf("complete-text latest schema changed: %+v %v", latest, err)
	}
	r.handleExportCmd("/export")
	entries, err := os.ReadDir(filepath.Join(r.runtimeAnchor, "exports"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("complete-text export=%v %v", entries, err)
	}
	bundle := filepath.Join(r.runtimeAnchor, "exports", entries[0].Name())
	manifestBody, err := os.ReadFile(filepath.Join(bundle, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest exportBundleManifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != exportBundleSchemaVersion || manifest.TraceRequiresReattach {
		t.Fatalf("complete text is no longer portable to schema1 readers: %+v", manifest)
	}
	// A self-contained text snapshot must not depend on the original file
	// surviving; restored text gets no serialized physical-file authority.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	restoredRunner := &materialAwareRunner{}
	fresh := New(Config{Runner: restoredRunner, In: strings.NewReader(""), Out: &bytes.Buffer{}, Language: "en", RuntimeArtifactStore: r.runtimeArtifactStore})
	fresh.maybeRestoreRuntimeArtifactForPolicy(TurnPolicy{Route: RouteRepo, Source: "prior_context"})
	if fresh.attachedHitrace != strings.TrimSpace(material.Preview()) || fresh.attachedTraceMaterial != nil {
		t.Fatalf("complete snapshot failed legacy text restore: %q material=%v", fresh.attachedHitrace, fresh.attachedTraceMaterial)
	}
	if restoredRunner.curTrace != fresh.attachedHitrace || restoredRunner.curTraceSource != fresh.attachedHitraceSource || restoredRunner.material != nil ||
		strings.Join(restoredRunner.publishOrder, ",") != "raw,source,material:nil" {
		t.Fatalf("snapshot restoration did not synchronize runner: trace=%q source=%q material=%v order=%v", restoredRunner.curTrace, restoredRunner.curTraceSource, restoredRunner.material, restoredRunner.publishOrder)
	}
	importer := New(Config{In: strings.NewReader(""), Out: &bytes.Buffer{}, Language: "en"})
	importer.handleImportCmd("/import " + bundle)
	if importer.attachedHitrace != material.Preview() || importer.attachedTraceMaterial != nil {
		t.Fatalf("complete text export/import failed: %q material=%v", importer.attachedHitrace, importer.attachedTraceMaterial)
	}
}
