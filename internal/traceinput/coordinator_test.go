package traceinput

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func coordinatorFixture(t *testing.T) (Options, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.sys")
	body := realRMQFixture(40)
	writeTestFile(t, path, body)
	return Options{RuntimeAnchor: filepath.Join(dir, "runtime"), PreviewBytes: 640}, path, body
}

func TestCoordinatorConcurrentBinarySharesReceiptAndFullQuery(t *testing.T) {
	opts, path, original := coordinatorFixture(t)
	opts.InputPath = filepath.Join(t.TempDir(), "must-not-use-this.sys")
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return hitraceconv.ConvertFile(ctx, opts)
	})
	const n = 24
	var wg sync.WaitGroup
	materials := make([]*attachment.TraceMaterial, n)
	errs := make([]error, n)
	for i := range materials {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			materials[i], errs[i] = c.Prepare(context.Background(), path)
		}(i)
	}
	<-entered
	close(release)
	wg.Wait()
	for i, material := range materials {
		if errs[i] != nil || material == nil || material != materials[0] {
			t.Fatalf("caller %d did not share one receipt: %v %v", i, material, errs[i])
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("singleflight converted %d times", calls.Load())
	}
	m := materials[0]
	for _, alias := range []string{path, m.QueryPath(), filepath.Join(filepath.Dir(path), ".", filepath.Base(path))} {
		got, err := c.Prepare(context.Background(), alias)
		if err != nil || got != m || calls.Load() != 1 {
			t.Fatalf("exact original/query alias lost receipt: %s %v %v", alias, got, err)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), m.QueryPath())
	if err != nil || len(idx.Events) != 40 || idx.Events[39].WakeePID != 139 {
		t.Fatalf("full material lost tail: %v %v", idx, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("source changed: %v", err)
	}
	// A new Run receives a new authority rather than an immortal process cache.
	fresh, err := NewCoordinator(opts).Prepare(context.Background(), path)
	if err != nil || fresh == m || fresh.QueryPath() == m.QueryPath() {
		t.Fatalf("another Run reused prior receipt: %v %v", fresh, err)
	}
}

func TestCoordinatorCachedGenerationChangeFailsWithoutReconversion(t *testing.T) {
	for _, member := range []string{"original", "query", "bundle_child", "receipt"} {
		t.Run(member, func(t *testing.T) {
			opts, path, _ := coordinatorFixture(t)
			var calls atomic.Int32
			c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				calls.Add(1)
				return hitraceconv.ConvertFile(ctx, opts)
			})
			m, err := c.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			changed := map[string]string{
				"original": path, "query": m.QueryPath(),
				"bundle_child": filepath.Join(filepath.Dir(m.QueryPath()), "capture.systrace"),
				"receipt":      filepath.Join(filepath.Dir(m.QueryPath()), "preparation.json"),
			}[member]
			if _, err := os.Stat(changed); err != nil {
				t.Fatalf("missing fixture member: %v", err)
			}
			writeTestFile(t, changed, []byte("changed generation"))
			for _, alias := range []string{path, m.QueryPath(), path} {
				got, err := c.Prepare(context.Background(), alias)
				if got != nil || err == nil || calls.Load() != 1 {
					t.Fatalf("stale %s reissued authority: %v %v calls=%d", member, got, err, calls.Load())
				}
			}
		})
	}
}

// This context exposes the exact point where a follower enters its wait
// select, so cancellation tests need no scheduling sleeps.
type coordinatorObservedContext struct {
	context.Context
	once     sync.Once
	observed chan struct{}
}

func (c *coordinatorObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

type coordinatedResult struct {
	material *attachment.TraceMaterial
	err      error
}

func coordinatorAsync(c *Coordinator, ctx context.Context, path string) <-chan coordinatedResult {
	done := make(chan coordinatedResult, 1)
	go func() {
		m, err := c.Prepare(ctx, path)
		done <- coordinatedResult{m, err}
	}()
	return done
}

func coordinatorAwait(t *testing.T, ch <-chan coordinatedResult) coordinatedResult {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("coordinator did not finish")
		return coordinatedResult{}
	}
}

func TestCoordinatorCanceledWaiterDoesNotCancelLeader(t *testing.T) {
	opts, path, _ := coordinatorFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		calls.Add(1)
		close(entered)
		<-release
		return hitraceconv.ConvertFile(ctx, opts)
	})
	leader := coordinatorAsync(c, context.Background(), path)
	<-entered
	waitCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &coordinatorObservedContext{Context: waitCtx, observed: make(chan struct{})}
	waiter := coordinatorAsync(c, observed, path)
	<-observed.observed
	cancel()
	got := coordinatorAwait(t, waiter)
	if got.material != nil || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("waiter cancellation lost: %+v", got)
	}
	close(release)
	got = coordinatorAwait(t, leader)
	if got.err != nil || got.material == nil || calls.Load() != 1 {
		t.Fatalf("waiter disrupted leader: %+v calls=%d", got, calls.Load())
	}
}

func TestCoordinatorLiveWaiterRetriesCanceledLeaderAfterRollback(t *testing.T) {
	opts, path, original := coordinatorFixture(t)
	entered := make(chan struct{})
	var calls atomic.Int32
	var discardedDirectory string
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		if calls.Add(1) == 1 {
			discardedDirectory = filepath.Dir(opts.OutputPath)
			if err := os.WriteFile(opts.OutputPath, []byte("unpublished partial"), 0600); err != nil {
				return hitraceconv.Result{}, err
			}
			close(entered)
			<-ctx.Done()
			return hitraceconv.Result{}, ctx.Err()
		}
		if _, err := os.Stat(discardedDirectory); !os.IsNotExist(err) {
			return hitraceconv.Result{}, errors.New("canceled leader released waiter before rollback")
		}
		return hitraceconv.ConvertFile(ctx, opts)
	})
	leaderCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	leader := coordinatorAsync(c, leaderCtx, path)
	<-entered
	observed := &coordinatorObservedContext{Context: context.Background(), observed: make(chan struct{})}
	waiter := coordinatorAsync(c, observed, path)
	<-observed.observed
	cancel()
	failed := coordinatorAwait(t, leader)
	if failed.material != nil || !errors.Is(failed.err, context.Canceled) {
		t.Fatalf("canceled leader published: %+v", failed)
	}
	got := coordinatorAwait(t, waiter)
	if got.err != nil || got.material == nil || calls.Load() != 2 {
		t.Fatalf("live waiter did not recover: %+v calls=%d", got, calls.Load())
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, original) {
		t.Fatalf("rollback modified original: %v", err)
	}
}

func TestCoordinatorFailureIsSharedButNotNegativelyCached(t *testing.T) {
	opts, path, _ := coordinatorFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	failure := errors.New("transient converter failure")
	var calls atomic.Int32
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			return hitraceconv.Result{}, failure
		}
		return hitraceconv.ConvertFile(ctx, opts)
	})
	leader := coordinatorAsync(c, context.Background(), path)
	<-entered
	observed := &coordinatorObservedContext{Context: context.Background(), observed: make(chan struct{})}
	waiter := coordinatorAsync(c, observed, path)
	<-observed.observed
	close(release)
	for _, result := range []<-chan coordinatedResult{leader, waiter} {
		got := coordinatorAwait(t, result)
		if got.material != nil || !errors.Is(got.err, failure) {
			t.Fatalf("failure was not shared: %+v", got)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("permanent-looking error caused a waiter retry storm: %d", calls.Load())
	}
	got, err := c.Prepare(context.Background(), path)
	if err != nil || got == nil || calls.Load() != 2 {
		t.Fatalf("later retry was poisoned: %v %v calls=%d", got, err, calls.Load())
	}
}

func TestCoordinatorRejectsInvalidOrCanceledCallWithoutAuthority(t *testing.T) {
	opts, path, _ := coordinatorFixture(t)
	for _, c := range []*Coordinator{nil, {}} {
		if m, err := c.Prepare(context.Background(), path); m != nil || err == nil {
			t.Fatal("uninitialized coordinator admitted material")
		}
	}
	c := NewCoordinator(opts)
	for _, path := range []string{"", "relative.sys", `\\.\pipe\capture`} {
		if m, err := c.Prepare(context.Background(), path); m != nil || err == nil {
			t.Fatalf("invalid path admitted: %s", path)
		}
	}
	for _, warm := range []bool{false, true} {
		if warm {
			if _, err := c.Prepare(nil, path); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if m, err := c.Prepare(ctx, path); m != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("warm=%v canceled call gained authority: %v %v", warm, m, err)
		}
	}
}

func TestCoordinatorPreparedMaterialsIsStableIndependentSnapshot(t *testing.T) {
	opts, first, _ := coordinatorFixture(t)
	otherDir := t.TempDir()
	second := filepath.Join(otherDir, filepath.Base(first))
	writeTestFile(t, second, realRMQFixture(2))
	c := NewCoordinator(opts)
	if got := c.PreparedMaterials(); len(got) != 0 {
		t.Fatalf("snapshot started preparation: %+v", got)
	}
	var nilCoordinator *Coordinator
	if got := nilCoordinator.PreparedMaterials(); got != nil {
		t.Fatalf("nil coordinator snapshot: %+v", got)
	}
	firstMaterial, err := c.Prepare(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	secondMaterial, err := c.Prepare(context.Background(), second)
	if err != nil || secondMaterial == firstMaterial {
		t.Fatalf("same basename merged unrelated captures: %v %v", secondMaterial, err)
	}
	got := c.PreparedMaterials()
	if len(got) != 2 || got[0].SourcePath() >= got[1].SourcePath() {
		t.Fatalf("snapshot order/dedup incorrect: %+v", got)
	}
	got[0] = nil
	if again := c.PreparedMaterials(); len(again) != 2 || again[0] == nil {
		t.Fatalf("returned slice mutated coordinator: %+v", again)
	}
	writeTestFile(t, first, []byte("changed"))
	if got := c.PreparedMaterials(); len(got) != 2 {
		t.Fatal("snapshot removed stale success and rearmed conversion")
	}
	if _, err := c.Prepare(context.Background(), firstMaterial.QueryPath()); err == nil {
		t.Fatal("snapshot revived stale generation")
	}
	if m, err := c.Prepare(context.Background(), second); err != nil || m != secondMaterial {
		t.Fatalf("unrelated source was poisoned: %v %v", m, err)
	}
}

func TestCoordinatorDistinctInputsPrepareIndependently(t *testing.T) {
	opts, first, _ := coordinatorFixture(t)
	second := filepath.Join(t.TempDir(), "second.sys")
	writeTestFile(t, second, realRMQFixture(3))
	entered := make(chan string, 2)
	release := make(chan struct{})
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		entered <- opts.InputPath
		<-release
		return hitraceconv.ConvertFile(ctx, opts)
	})
	firstResult := coordinatorAsync(c, context.Background(), first)
	secondResult := coordinatorAsync(c, context.Background(), second)
	seen := make(map[string]bool)
	for range 2 {
		select {
		case path := <-entered:
			seen[path] = true
		case <-time.After(10 * time.Second):
			close(release)
			t.Fatal("one source blocked independent preparation")
		}
	}
	if len(seen) != 2 || len(c.PreparedMaterials()) != 0 {
		t.Fatal("pending paths merged or exposed unfinished material")
	}
	close(release)
	for _, ch := range []<-chan coordinatedResult{firstResult, secondResult} {
		got := coordinatorAwait(t, ch)
		if got.err != nil || got.material == nil {
			t.Fatalf("independent preparation failed: %+v", got)
		}
	}
	if len(c.PreparedMaterials()) != 2 {
		t.Fatal("successful distinct captures did not remain independent")
	}
}

func coordinatorBundleFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	paths := []string{"capture.systrace", "non_basename.perftrace"}
	bodies := []string{
		"app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n",
		"app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=App dso=lib.so source=test\n",
	}
	var members []tracebundle.CaptureMember
	var artifacts []map[string]any
	for i, name := range paths {
		writeTestFile(t, filepath.Join(dir, name), []byte(bodies[i]))
		digest := sha256.Sum256([]byte(bodies[i]))
		kind := []string{"systrace", "perftrace"}[i]
		sha := hex.EncodeToString(digest[:])
		members = append(members, tracebundle.CaptureMember{Type: kind, Path: name, Bytes: int64(len(bodies[i])), SHA256: sha})
		artifacts = append(artifacts, map[string]any{"type": kind, "path": name, "bytes": len(bodies[i]), "sha256": sha})
	}
	captureID, err := tracebundle.CaptureID(members)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"schema": tracebundle.SchemaV2, "capture_id": captureID, "systrace": paths[0], "artifacts": artifacts})
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeTestFile(t, bundle, body)
	return filepath.Join(dir, paths[0]), bundle, filepath.Join(dir, paths[1])
}

func TestCoordinatorPlainTextFreezesEngineSelectedSiblingUniverse(t *testing.T) {
	for _, change := range []string{"child", "manifest", "appears", "disappears"} {
		t.Run(change, func(t *testing.T) {
			path, bundle, perf := coordinatorBundleFixture(t)
			if change == "appears" {
				if err := os.Rename(bundle, bundle+".saved"); err != nil {
					t.Fatal(err)
				}
			}
			c := NewCoordinator(Options{})
			m, err := c.Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "child":
				writeTestFile(t, perf, []byte("changed child\n"))
			case "manifest":
				body, err := os.ReadFile(bundle)
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, bundle, append(body, '\n'))
			case "appears":
				if err := os.Rename(bundle+".saved", bundle); err != nil {
					t.Fatal(err)
				}
			case "disappears":
				if err := os.Rename(bundle, bundle+".saved"); err != nil {
					t.Fatal(err)
				}
			}
			// The physical text itself is unchanged. Only the engine's shared
			// selection contract can detect this universe transition.
			if err := m.Validate(context.Background(), m.Preview()); err != nil {
				t.Fatalf("fixture changed the primary text: %v", err)
			}
			for i := 0; i < 2; i++ {
				if got, err := c.Prepare(context.Background(), path); err == nil || got != nil {
					t.Fatalf("%s changed engine universe without invalidating receipt: %v %v", change, got, err)
				}
			}
		})
	}
}

func TestCoordinatorBareSiblingCannotExpandInputUniverse(t *testing.T) {
	path, bundle, perf := coordinatorBundleFixture(t)
	if err := os.Rename(bundle, bundle+".not-a-bundle"); err != nil {
		t.Fatal(err)
	}
	c := NewCoordinator(Options{})
	m, err := c.Prepare(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, perf, []byte("unbound sibling changed\n"))
	got, err := c.Prepare(context.Background(), path)
	if err != nil || got != m {
		t.Fatalf("unbound sibling poisoned the selected capture: %v %v", got, err)
	}
}

func TestCoordinatorCanonicalAliasCannotReprepareChangedCapture(t *testing.T) {
	for _, firstUsesAlias := range []bool{false, true} {
		t.Run(map[bool]string{false: "physical_first", true: "alias_first"}[firstUsesAlias], func(t *testing.T) {
			opts, physical, _ := coordinatorFixture(t)
			alias := filepath.Join(t.TempDir(), "alias.sys")
			if err := os.Symlink(physical, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			first, next := physical, alias
			if firstUsesAlias {
				first, next = alias, physical
			}
			c := NewCoordinator(opts)
			m, err := c.Prepare(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := c.Prepare(context.Background(), next); err != nil || got != m {
				t.Fatalf("same physical capture was converted again through canonical alias: %v %v", got, err)
			}
			writeTestFile(t, physical, realRMQFixture(3))
			freshAlias := filepath.Join(t.TempDir(), "fresh_alias.sys")
			if err := os.Symlink(physical, freshAlias); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{first, next, freshAlias} {
				if got, err := c.Prepare(context.Background(), path); err == nil || got != nil {
					t.Fatalf("stale success was bypassed through %s: %v %v", path, got, err)
				}
			}
		})
	}
}

func TestCoordinatorSeenAliasCannotRetargetAnotherCapture(t *testing.T) {
	opts, physical, _ := coordinatorFixture(t)
	other := filepath.Join(t.TempDir(), "other.sys")
	writeTestFile(t, other, realRMQFixture(2))
	alias := filepath.Join(t.TempDir(), "alias.sys")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	c := NewCoordinator(opts)
	if _, err := c.Prepare(context.Background(), physical); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prepare(context.Background(), alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(alias, alias+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Prepare(context.Background(), alias); err == nil || got != nil {
		t.Fatalf("seen alias silently switched capture: %v %v", got, err)
	}
	if got, err := c.Prepare(context.Background(), other); err != nil || got == nil {
		t.Fatalf("unrelated explicit capture was blocked: %v %v", got, err)
	}
}

func TestCoordinatorCanonicalAliasesShareFlightAndRejectWaitingRetarget(t *testing.T) {
	for _, retarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_target", true: "retarget"}[retarget], func(t *testing.T) {
			opts, physical, _ := coordinatorFixture(t)
			alias := filepath.Join(t.TempDir(), "alias.sys")
			if err := os.Symlink(physical, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				if calls.Add(1) == 1 {
					close(entered)
				}
				<-release
				return hitraceconv.ConvertFile(ctx, opts)
			})
			leader := coordinatorAsync(c, context.Background(), physical)
			<-entered
			observed := &coordinatorObservedContext{Context: context.Background(), observed: make(chan struct{})}
			waiter := coordinatorAsync(c, observed, alias)
			select {
			case <-observed.observed:
			case <-time.After(10 * time.Second):
				close(release)
				t.Fatal("canonical alias did not join the original flight")
			}
			if retarget {
				other := filepath.Join(t.TempDir(), "other.sys")
				writeTestFile(t, other, realRMQFixture(2))
				if err := os.Rename(alias, alias+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, alias); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			gotLeader, gotWaiter := coordinatorAwait(t, leader), coordinatorAwait(t, waiter)
			if gotLeader.err != nil || gotLeader.material == nil || calls.Load() != 1 {
				t.Fatalf("alias changed leader: %+v calls=%d", gotLeader, calls.Load())
			}
			if retarget {
				if gotWaiter.err == nil || gotWaiter.material != nil {
					t.Fatal("waiting alias silently selected a different capture")
				}
			} else if gotWaiter.err != nil || gotWaiter.material != gotLeader.material {
				t.Fatal("canonical aliases did not share the committed receipt")
			}
		})
	}
}

func coordinatorCaseAliasFixture(t *testing.T) (Options, string, string) {
	t.Helper()
	opts, source, body := coordinatorFixture(t)
	dir := filepath.Join(filepath.Dir(source), "MiXeD")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	physical := filepath.Join(dir, "capture.sys")
	writeTestFile(t, physical, body)
	alias := filepath.Join(filepath.Dir(source), "mixed", "CAPTURE.SYS")
	physicalInfo, err := os.Stat(physical)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(alias)
	if err != nil || !os.SameFile(physicalInfo, aliasInfo) {
		t.Skip("fixture volume does not alias directory and filename case")
	}
	return opts, physical, alias
}

func TestCoordinatorCaseAliasesShareReceiptAndCannotReprepareChangedCapture(t *testing.T) {
	for _, aliasFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "physical_first", true: "alias_first"}[aliasFirst], func(t *testing.T) {
			opts, physical, alias := coordinatorCaseAliasFixture(t)
			var calls atomic.Int32
			c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
				calls.Add(1)
				return hitraceconv.ConvertFile(ctx, opts)
			})
			first, next := physical, alias
			if aliasFirst {
				first, next = alias, physical
			}
			m, err := c.Prepare(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := c.Prepare(context.Background(), next); err != nil || got != m || calls.Load() != 1 {
				t.Fatalf("case alias failed to share physical capture: %v %v calls=%d", got, err, calls.Load())
			}
			writeTestFile(t, physical, realRMQFixture(3))
			freshAlias := filepath.Join(filepath.Dir(alias), "CaPtUrE.sYs")
			for _, path := range []string{first, next, freshAlias} {
				if got, err := c.Prepare(context.Background(), path); err == nil || got != nil || calls.Load() != 1 {
					t.Fatalf("case alias silently prepared changed capture: %s %v %v calls=%d", path, got, err, calls.Load())
				}
			}
		})
	}
}

func TestCoordinatorCaseAliasesShareFlight(t *testing.T) {
	opts, physical, alias := coordinatorCaseAliasFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := newCoordinator(opts, func(ctx context.Context, opts hitraceconv.Options) (hitraceconv.Result, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return hitraceconv.ConvertFile(ctx, opts)
	})
	leader := coordinatorAsync(c, context.Background(), physical)
	<-entered
	observed := &coordinatorObservedContext{Context: context.Background(), observed: make(chan struct{})}
	waiter := coordinatorAsync(c, observed, alias)
	select {
	case <-observed.observed:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("case alias did not join the physical source flight")
	}
	close(release)
	a, b := coordinatorAwait(t, leader), coordinatorAwait(t, waiter)
	if a.err != nil || b.err != nil || a.material == nil || a.material != b.material || calls.Load() != 1 {
		t.Fatalf("case aliases did not share successful flight: a=%+v b=%+v calls=%d", a, b, calls.Load())
	}
}

func TestCoordinatorExactDirectoryEntriesRemainIndependent(t *testing.T) {
	for _, mode := range []string{"different_case_file", "different_case_hardlink", "different_name_hardlink"} {
		t.Run(mode, func(t *testing.T) {
			opts, first, body := coordinatorFixture(t)
			second := filepath.Join(filepath.Dir(first), strings.ToUpper(filepath.Base(first)))
			if mode == "different_name_hardlink" {
				second = filepath.Join(filepath.Dir(first), "independent.sys")
			}
			if strings.HasSuffix(mode, "hardlink") {
				if err := os.Link(first, second); err != nil {
					if os.IsExist(err) {
						t.Skip("fixture volume does not support distinct case-only directory entries")
					}
					t.Skipf("hardlink unavailable: %v", err)
				}
			} else {
				f, err := os.OpenFile(second, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if os.IsExist(err) {
					t.Skip("fixture volume does not support distinct case-only directory entries")
				}
				if err != nil {
					t.Fatal(err)
				}
				_, writeErr := f.Write(body)
				if err := errors.Join(writeErr, f.Close()); err != nil {
					t.Fatal(err)
				}
			}
			c := NewCoordinator(opts)
			a, err := c.Prepare(context.Background(), first)
			if err != nil {
				t.Fatal(err)
			}
			b, err := c.Prepare(context.Background(), second)
			if err != nil || a == b || a.QueryPath() == b.QueryPath() || len(c.PreparedMaterials()) != 2 {
				t.Fatalf("independent exact entries were merged: %v %v %v", a, b, err)
			}
		})
	}
}

func TestCoordinatorCanonicalEntryPagedLookupAndCancellation(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 260; i++ {
		writeTestFile(t, filepath.Join(dir, fmt.Sprintf("filler-%04d", i)), []byte("x"))
	}
	writeTestFile(t, filepath.Join(dir, "capture.sys"), []byte("source"))
	got, err := preparationDirectoryEntry(context.Background(), dir, "capture.sys")
	if err != nil || got != "capture.sys" {
		t.Fatalf("paged directory lookup failed: %q %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preparationDirectoryEntry(ctx, dir, "capture.sys"); !errors.Is(err, context.Canceled) {
		t.Fatalf("directory lookup ignored cancellation: %v", err)
	}
}

func TestCoordinatorLexicalPathKeysNeverFoldDirectoryEntries(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "CaseSensitive", "A.sys")
	second := filepath.Join(root, "CaseSensitive", "a.sys")
	if preparationPathKey(first) != first || preparationPathKey(second) != second || preparationPathKey(first) == preparationPathKey(second) {
		t.Fatal("lexical path keys folded distinct directory entries before filesystem validation")
	}
}

func TestCoordinatorReadOnlyListableInputRemainsSupported(t *testing.T) {
	opts, source, original := coordinatorFixture(t)
	opts.RuntimeAnchor = t.TempDir()
	dir := filepath.Dir(source)
	if err := os.Chmod(source, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	c := NewCoordinator(opts)
	m, err := c.Prepare(context.Background(), source)
	if err != nil || m == nil {
		t.Fatalf("read-only input with listable parent was rejected: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("read-only source changed: %v", err)
	}
}

func TestCoordinatorSearchOnlyParentReportsIdentityPermissionBoundary(t *testing.T) {
	opts, source, original := coordinatorFixture(t)
	opts.RuntimeAnchor = t.TempDir()
	dir := filepath.Dir(source)
	if err := os.Chmod(dir, 0o100); err != nil {
		t.Skipf("host cannot set directory search-only permissions: %v", err)
	}
	defer os.Chmod(dir, 0o700)
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Skipf("host cannot establish a readable input in a search-only directory: %v", err)
	}
	if d, err := os.Open(dir); err == nil {
		_, listErr := d.ReadDir(1)
		d.Close()
		if listErr == nil {
			t.Skip("host privileges or filesystem still allow directory listing")
		}
	}
	c := NewCoordinator(opts)
	m, err := c.Prepare(context.Background(), source)
	if m != nil || err == nil || !strings.Contains(err.Error(), "trace source identity requires listing parent directory") || len(c.PreparedMaterials()) != 0 {
		t.Fatalf("search-only identity uncertainty was hidden or misclassified: %v %v", m, err)
	}
}
