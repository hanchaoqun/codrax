package tracequery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestBundleManifestSnapshotSwitchCannotPublishAnotherCapture(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	other := filepath.Join(dir, "other.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, other, traceBundleTestWakeupRow(99, 20))
	writeBundleMembershipFixture(t, bundle, `{"version":"A","systrace":"capture.systrace"}`)

	var key parseCacheKey
	var switched atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), requested, BuildOptions{}, func(phase traceIndexBuildPhase, got parseCacheKey) {
		if phase != traceIndexPhaseSelectionFrozen || !switched.CompareAndSwap(false, true) {
			return
		}
		key = got
		if writeErr := os.WriteFile(bundle, []byte(`{"version":"B","systrace":"other.systrace"}`), 0o644); writeErr != nil {
			t.Errorf("switch manifest: %v", writeErr)
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("manifest generation switch published an index: idx=%+v err=%v", idx, err)
	}
	if !strings.Contains(err.Error(), "generation changed") {
		t.Fatalf("manifest switch error lost generation verdict: %v", err)
	}
	if _, ok := indexCache.Load(key); ok {
		t.Fatal("failed manifest generation was published into the index cache")
	}
}

func TestBundleManifestABACannotRestoreSnapshotAuthority(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	other := filepath.Join(dir, "other.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	manifestA := `{"version":"A","systrace":"capture.systrace"}`
	manifestB := `{"version":"B","systrace":"other.systrace"}`
	writeBundleMembershipFixture(t, requested, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, other, traceBundleTestWakeupRow(99, 20))
	writeBundleMembershipFixture(t, bundle, manifestA)

	replace := func(name, body string) error {
		tmp := filepath.Join(dir, name)
		if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, bundle)
	}
	var switched atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), requested, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase != traceIndexPhaseSelectionFrozen || !switched.CompareAndSwap(false, true) {
			return
		}
		if replaceErr := replace("manifest-b.tmp", manifestB); replaceErr != nil {
			t.Skipf("filesystem cannot atomically replace an open manifest: %v", replaceErr)
		}
		if replaceErr := replace("manifest-a2.tmp", manifestA); replaceErr != nil {
			t.Fatalf("restore manifest A through a new generation: %v", replaceErr)
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("A→B→A path replacement restored stale snapshot authority: idx=%+v err=%v", idx, err)
	}
	if !strings.Contains(err.Error(), "generation changed") {
		t.Fatalf("A→B→A error lost generation verdict: %v", err)
	}
}

func TestUniverseValidationFailureCannotPoisonIndexCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache-poison.systrace")
	writeBundleMembershipFixture(t, path, traceBundleTestWakeupRow(20, 10))

	var key parseCacheKey
	var changed atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), path, BuildOptions{}, func(phase traceIndexBuildPhase, got parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			key = got
		}
		if phase != traceIndexPhaseBeforeUniverseValidation || !changed.CompareAndSwap(false, true) {
			return
		}
		if writeErr := os.WriteFile(path, []byte(traceBundleTestWakeupRow(99, 20)), 0o644); writeErr != nil {
			t.Errorf("replace source after parse: %v", writeErr)
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("post-parse generation failure published an index: idx=%+v err=%v", idx, err)
	}
	if _, ok := indexCache.Load(key); ok {
		t.Fatal("post-parse generation failure left an index cache entry")
	}
	indexBuildMu.Lock()
	_, inflight := indexBuilds[key]
	indexBuildMu.Unlock()
	if inflight {
		t.Fatal("post-parse generation failure left an in-flight entry")
	}
}

func TestSingleflightWaitersReceiveOnlyValidatedFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "singleflight-validation.systrace")
	writeBundleMembershipFixture(t, path, traceBundleTestWakeupRow(20, 10))

	leaderAtValidation := make(chan struct{})
	releaseLeader := make(chan struct{})
	waiterJoined := make(chan struct{})
	var key parseCacheKey
	var parseCount atomic.Int32

	type buildResult struct {
		idx *Index
		err error
	}
	leaderResult := make(chan buildResult, 1)
	go func() {
		idx, err := buildIndexWithObserver(context.Background(), path, BuildOptions{}, func(phase traceIndexBuildPhase, got parseCacheKey) {
			if phase == traceIndexPhaseSelectionFrozen {
				key = got
			}
			if phase == traceIndexPhaseBeforeUniverseValidation {
				parseCount.Add(1)
				close(leaderAtValidation)
				<-releaseLeader
			}
		})
		leaderResult <- buildResult{idx: idx, err: err}
	}()

	select {
	case <-leaderAtValidation:
	case <-time.After(5 * time.Second):
		t.Fatal("leader did not reach terminal universe validation")
	}

	waiterResult := make(chan buildResult, 1)
	go func() {
		idx, err := buildIndexWithObserver(context.Background(), path, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
			if phase == traceIndexPhaseSingleflightJoined {
				close(waiterJoined)
			}
		})
		waiterResult <- buildResult{idx: idx, err: err}
	}()

	select {
	case <-waiterJoined:
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not join the in-flight build")
	}
	if _, ok := indexCache.Load(key); ok {
		t.Fatal("leader published cache before terminal validation")
	}
	if err := os.WriteFile(path, []byte(traceBundleTestWakeupRow(99, 20)), 0o644); err != nil {
		t.Fatal(err)
	}
	close(releaseLeader)

	for name, ch := range map[string]<-chan buildResult{"leader": leaderResult, "waiter": waiterResult} {
		select {
		case result := <-ch:
			if result.err == nil || result.idx != nil {
				t.Fatalf("%s received unvalidated success: idx=%+v err=%v", name, result.idx, result.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
	if got := parseCount.Load(); got != 1 {
		t.Fatalf("singleflight parsed %d times, want exactly one", got)
	}
	if _, ok := indexCache.Load(key); ok {
		t.Fatal("failed singleflight left an index cache entry")
	}
	indexBuildMu.Lock()
	_, inflight := indexBuilds[key]
	indexBuildMu.Unlock()
	if inflight {
		t.Fatal("failed singleflight left an in-flight entry")
	}
}

func TestCacheHitAndFullDeriveRevalidateFrozenUniverse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "warm-cache.systrace")
	writeBundleMembershipFixture(t, path, traceBundleTestWakeupRow(20, 10))

	var fullKey parseCacheKey
	first, err := buildIndexWithObserver(t.Context(), path, BuildOptions{}, func(phase traceIndexBuildPhase, key parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			fullKey = key
		}
	})
	if err != nil || first == nil {
		t.Fatalf("prime full cache: idx=%+v err=%v", first, err)
	}
	if _, ok := indexCache.Load(fullKey); !ok {
		t.Fatal("full cache was not primed")
	}

	var changed atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), path, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen && changed.CompareAndSwap(false, true) {
			if writeErr := os.WriteFile(path, []byte(traceBundleTestWakeupRow(21, 11)), 0o644); writeErr != nil {
				t.Errorf("mutate before cache load: %v", writeErr)
			}
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("warm cache returned after source drift: idx=%+v err=%v", idx, err)
	}
	if _, ok := indexCache.Load(fullKey); ok {
		t.Fatal("failed warm-cache validation did not evict the stale key")
	}

	var nextFullKey parseCacheKey
	if _, err := buildIndexWithObserver(t.Context(), path, BuildOptions{}, func(phase traceIndexBuildPhase, key parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			nextFullKey = key
		}
	}); err != nil {
		t.Fatalf("re-prime full cache: %v", err)
	}
	changed.Store(false)
	windowOpts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeEndSet: true, TimeStart: 10, TimeEnd: 12}
	idx, err = buildIndexWithObserver(t.Context(), path, windowOpts, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen && changed.CompareAndSwap(false, true) {
			if writeErr := os.WriteFile(path, []byte(traceBundleTestWakeupRow(22, 12)), 0o644); writeErr != nil {
				t.Errorf("mutate before full-cache derive: %v", writeErr)
			}
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("full-cache derive returned after source drift: idx=%+v err=%v", idx, err)
	}
	if _, ok := indexCache.Load(nextFullKey); ok {
		t.Fatal("failed full-cache derive did not evict the stale full key")
	}
}

func TestSchedulerHeadRejectsWrongSelectedGenerationAsHardError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "head-generation.systrace")
	body := traceBundleTestWakeupRow(20, 4) + traceBundleTestWakeupRow(20, 5)
	writeBundleMembershipFixture(t, path, body)
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeEndSet: true, TimeStart: 5, TimeEnd: 6}
	var changed atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), path, opts, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase != traceIndexPhaseBeforeSchedulerHead || !changed.CompareAndSwap(false, true) {
			return
		}
		if writeErr := os.WriteFile(path, []byte(traceBundleTestWakeupRow(99, 9)), 0o644); writeErr != nil {
			t.Errorf("replace scheduler-head source: %v", writeErr)
		}
	})
	if !errors.Is(err, errSchedulerHeadSourceGeneration) || idx != nil {
		t.Fatalf("scheduler head generation mismatch degraded instead of failing hard: idx=%+v err=%v", idx, err)
	}
}

func TestTraceSourceVersionUsesSiblingBundleUniverse(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	aux := filepath.Join(dir, "aux.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, aux, "app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=App dso=lib.so source=test\n")
	writeBundleMembershipFixture(t, bundle, `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"aux.perftrace"}
  ]
}`)

	version, err := CaptureTraceSourceVersion(requested)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aux, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := version.Validate(requested); err == nil {
		t.Fatal("source version ignored a non-basename bundle child generation")
	}
}

func TestOptionalOversizedSiblingFallsBackButExplicitBundleFails(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, traceBundleTestWakeupRow(20, 10))
	file, err := os.Create(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(tracebundle.MaxManifestBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildIndex(t.Context(), bundle)
	if !errors.Is(err, tracebundle.ErrTooLarge) || idx != nil {
		t.Fatalf("explicit oversized bundle was not rejected atomically: idx=%+v err=%v", idx, err)
	}
	var becameValid atomic.Bool
	direct, err := buildIndexWithObserver(t.Context(), requested, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen && becameValid.CompareAndSwap(false, true) {
			writeBundleMembershipFixture(t, bundle, `{"version":"valid","systrace":"capture.systrace"}`)
		}
	})
	if err != nil || direct == nil || direct.Path != canonicalTraceIndexPath(requested) {
		t.Fatalf("frozen direct selection was hijacked when optional metadata became valid: idx=%+v err=%v", direct, err)
	}

	promoted, err := BuildIndex(t.Context(), requested)
	if err != nil || promoted == nil {
		t.Fatalf("newly valid optional bundle was not reconsidered: idx=%+v err=%v", promoted, err)
	}
	if promoted.Path != canonicalTraceIndexPath(bundle) || promoted == direct {
		t.Fatalf("direct fallback was incorrectly cached across a new valid manifest: direct=%p promoted=%p path=%q", direct, promoted, promoted.Path)
	}
}

func TestBundleLocalAppearanceChangesLegacyCWDFallbackUniverse(t *testing.T) {
	root := t.TempDir()
	bundleDir := filepath.Join(root, "bundle")
	if err := os.Mkdir(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cwdTrace := filepath.Join(root, "legacy.systrace")
	bundleTrace := filepath.Join(bundleDir, "legacy.systrace")
	bundle := filepath.Join(bundleDir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, cwdTrace, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, bundle, `{"version":"legacy","systrace":"legacy.systrace"}`)

	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousCWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	resolver := newTraceBundleArtifactPathResolver(bundleDir)
	if got := resolver.resolve("legacy.systrace"); got != canonicalTraceIndexPath(cwdTrace) {
		t.Fatalf("legacy resolver did not initially select CWD child: %q", got)
	}
	var appeared atomic.Bool
	legacy, err := buildIndexWithObserver(t.Context(), bundle, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen && appeared.CompareAndSwap(false, true) {
			writeBundleMembershipFixture(t, bundleTrace, traceBundleTestWakeupRow(99, 20))
		}
	})
	if err != nil || legacy == nil || len(legacy.Events) != 1 || legacy.Events[0].PID != 20 {
		t.Fatalf("bundle-local appearance changed a frozen CWD child: idx=%+v err=%v", legacy, err)
	}
	if got := resolver.resolve("legacy.systrace"); got != canonicalTraceIndexPath(cwdTrace) {
		t.Fatalf("build-local resolver changed an already frozen relative path: %q", got)
	}
	if got := newTraceBundleArtifactPathResolver(bundleDir).resolve("legacy.systrace"); got != canonicalTraceIndexPath(bundleTrace) {
		t.Fatalf("new resolver did not observe bundle-local child: %q", got)
	}
	local, err := BuildIndex(t.Context(), bundle)
	if err != nil || local == nil || len(local.Events) != 1 || local.Events[0].PID != 99 {
		t.Fatalf("new bundle-local child did not replace the legacy CWD fallback universe: idx=%+v err=%v", local, err)
	}
	if local == legacy {
		t.Fatal("bundle-local path appearance reused the CWD-fallback cache entry")
	}
}

func TestBundleLocalLossCannotFallBackToCWDWithinBuild(t *testing.T) {
	root := t.TempDir()
	bundleDir := filepath.Join(root, "bundle")
	if err := os.Mkdir(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cwdTrace := filepath.Join(root, "selected.systrace")
	bundleTrace := filepath.Join(bundleDir, "selected.systrace")
	bundle := filepath.Join(bundleDir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, cwdTrace, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, bundleTrace, traceBundleTestWakeupRow(99, 20))
	writeBundleMembershipFixture(t, bundle, `{"version":"local","systrace":"selected.systrace"}`)

	previousCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousCWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	var removed atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), bundle, BuildOptions{}, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen && removed.CompareAndSwap(false, true) {
			if removeErr := os.Remove(bundleTrace); removeErr != nil {
				t.Errorf("remove frozen bundle-local child: %v", removeErr)
			}
		}
	})
	if err == nil || idx != nil {
		t.Fatalf("lost bundle-local child silently fell back to CWD during one build: idx=%+v err=%v", idx, err)
	}
}

func TestSchedulerHeadMissingSelectedSourceIsHardGenerationError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "head-missing.systrace")
	body := traceBundleTestWakeupRow(20, 4) + traceBundleTestWakeupRow(20, 5)
	writeBundleMembershipFixture(t, path, body)
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeEndSet: true, TimeStart: 5, TimeEnd: 6}
	var removed atomic.Bool
	idx, err := buildIndexWithObserver(t.Context(), path, opts, func(phase traceIndexBuildPhase, _ parseCacheKey) {
		if phase == traceIndexPhaseBeforeSchedulerHead && removed.CompareAndSwap(false, true) {
			if removeErr := os.Remove(path); removeErr != nil {
				t.Errorf("remove scheduler-head source: %v", removeErr)
			}
		}
	})
	if !errors.Is(err, errSchedulerHeadSourceGeneration) || idx != nil {
		t.Fatalf("scheduler-head open failure degraded instead of failing generation-hard: idx=%+v err=%v", idx, err)
	}
}

func TestSecondaryIsolatedSystraceCannotAuthorizeSiblingPromotion(t *testing.T) {
	dir := t.TempDir()
	requested := filepath.Join(dir, "capture.systrace")
	primary := filepath.Join(dir, "primary.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleMembershipFixture(t, requested, traceBundleTestWakeupRow(20, 10))
	writeBundleMembershipFixture(t, primary, traceBundleTestWakeupRow(99, 20))
	writeBundleMembershipFixture(t, bundle, `{
  "version":"test",
  "systrace":"primary.systrace",
  "artifacts":[
    {"type":"systrace","path":"primary.systrace"},
    {"type":"systrace","path":"capture.systrace"}
  ]
}`)

	if got := promoteSiblingTraceBundlePath(requested); got != canonicalTraceIndexPath(requested) {
		t.Fatalf("secondary isolated systrace authorized promotion: %q", got)
	}
	idx, err := BuildIndex(t.Context(), requested)
	if err != nil || idx == nil || idx.Path != canonicalTraceIndexPath(requested) || len(idx.Events) != 1 || idx.Events[0].PID != 20 {
		t.Fatalf("secondary systrace hijacked direct result: idx=%+v err=%v", idx, err)
	}
}

func traceBundleTestWakeupRow(pid int, ts int) string {
	return "app-" + strconv.Itoa(pid) + " (" + strconv.Itoa(pid) + ") [001] .... " + strconv.Itoa(ts) + ".000000: sched_wakeup: comm=app pid=" + strconv.Itoa(pid) + " prio=20 target_cpu=001\n"
}
