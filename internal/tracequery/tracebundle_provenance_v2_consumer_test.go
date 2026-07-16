package tracequery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTraceBundleV2ConsumerRejectsSameSizeChildReplacement(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	original := consumerV2WakeupRow(20, 10)
	replacement := consumerV2WakeupRow(99, 20)
	if len(original) != len(replacement) {
		t.Fatalf("same-size replacement fixture drifted: original=%d replacement=%d", len(original), len(replacement))
	}
	consumerV2WriteFile(t, child, []byte(original))
	writeTraceBundleV2ForTest(t, bundle, consumerV2SingleSystraceManifest())
	consumerV2WriteFile(t, child, []byte(replacement))

	idx, err := BuildIndex(context.Background(), bundle)
	if err == nil || idx != nil {
		t.Fatalf("explicit V2 bundle published a same-size replacement: idx=%+v err=%v", idx, err)
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("same-size replacement lost its digest verdict: %v", err)
	}
}

func TestTraceBundleV2ConsumerOptionalMismatchFallsBackAndRepairPromotes(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	original := consumerV2WakeupRow(20, 10)
	replacement := consumerV2WakeupRow(99, 20)
	consumerV2WriteFile(t, child, []byte(original))
	writeTraceBundleV2ForTest(t, bundle, consumerV2SingleSystraceManifest())
	consumerV2WriteFile(t, child, []byte(replacement))

	direct, err := BuildIndex(context.Background(), child)
	if err != nil || direct == nil {
		t.Fatalf("invalid optional V2 bundle made the direct child unusable: idx=%+v err=%v", direct, err)
	}
	if direct.Path != canonicalTraceIndexPath(child) || len(direct.TraceArtifacts) != 1 || len(direct.Events) != 1 || direct.Events[0].PID != 99 {
		t.Fatalf("optional mismatch did not fall back atomically to the current direct child: path=%q artifacts=%+v events=%+v", direct.Path, direct.TraceArtifacts, direct.Events)
	}

	writeTraceBundleV2ForTest(t, bundle, consumerV2SingleSystraceManifest())
	promoted, err := BuildIndex(context.Background(), child)
	if err != nil || promoted == nil {
		t.Fatalf("repaired optional V2 bundle was not reconsidered: idx=%+v err=%v", promoted, err)
	}
	if promoted.Path != canonicalTraceIndexPath(bundle) || len(promoted.Events) != 1 || promoted.Events[0].PID != 99 {
		t.Fatalf("repaired optional V2 bundle did not promote the requested child: path=%q events=%+v", promoted.Path, promoted.Events)
	}
	if len(promoted.TraceArtifacts) != 1 || promoted.TraceArtifacts[0].BundleSchema == "" || promoted.TraceArtifacts[0].CaptureID == "" || promoted.TraceArtifacts[0].SourceSHA256 == "" {
		t.Fatalf("promoted child omitted its persistent V2 binding: %+v", promoted.TraceArtifacts)
	}
}

func TestTraceBundleV2ConsumerLegacyPolicy(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	perf := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	consumerV2WriteFile(t, child, []byte(consumerV2WakeupRow(20, 10)))
	legacySingle := []byte(`{
  "version":"legacy",
  "systrace":"capture.systrace",
  "artifacts":[{"type":" systrace ","path":"capture.systrace","caveats":["must-not-publish"]}],
  "caveats":["must-not-publish"]
}`)
	consumerV2WriteFile(t, bundle, legacySingle)

	direct, err := BuildIndex(context.Background(), child)
	if err != nil || direct == nil {
		t.Fatalf("legacy optional manifest made direct child unusable: idx=%+v err=%v", direct, err)
	}
	if direct.Path != canonicalTraceIndexPath(child) || containsSubstring(direct.Caveats, "tracebundle_legacy_unbound") {
		t.Fatalf("legacy optional manifest was promoted or leaked metadata: path=%q caveats=%v", direct.Path, direct.Caveats)
	}

	legacy, err := BuildIndex(context.Background(), bundle)
	if err != nil || legacy == nil {
		t.Fatalf("explicit legacy single-systrace compatibility failed: idx=%+v err=%v", legacy, err)
	}
	if legacy.Path != canonicalTraceIndexPath(bundle) || !containsSubstring(legacy.Caveats, "tracebundle_legacy_unbound=true") {
		t.Fatalf("explicit legacy single lacked its bounded disclosure: path=%q caveats=%v", legacy.Path, legacy.Caveats)
	}
	if containsSubstring(legacy.Caveats, "must-not-publish") {
		t.Fatalf("unbound legacy metadata retained authority: %v", legacy.Caveats)
	}
	if len(legacy.TraceArtifacts) != 1 || legacy.TraceArtifacts[0].BundleSchema != "" || legacy.TraceArtifacts[0].CaptureID != "" || legacy.TraceArtifacts[0].SourceSHA256 != "" {
		t.Fatalf("legacy single was mislabeled as V2-bound: %+v", legacy.TraceArtifacts)
	}

	consumerV2WriteFile(t, perf, []byte("app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=X dso=lib.so source=test\n"))
	consumerV2WriteFile(t, bundle, []byte(`{
  "version":"legacy",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace","perf_capability":{"trace_query_ready":true}}
  ]
}`))
	composite, err := BuildIndex(context.Background(), bundle)
	if err == nil || composite != nil {
		t.Fatalf("explicit legacy composite minted an unbound cross-artifact universe: idx=%+v err=%v", composite, err)
	}
	if !strings.Contains(err.Error(), "unbound perf child") {
		t.Fatalf("legacy composite error lost its actionable verdict: %v", err)
	}
}

func TestTraceBundleV2ConsumerDoesNotDiscoverBarePerfSibling(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	perf := filepath.Join(dir, "capture.perftrace")
	consumerV2WriteFile(t, child, []byte(consumerV2WakeupRow(20, 10)))
	consumerV2WriteFile(t, perf, []byte("app-99 (99) [001] .... 10.001000: perf_sample: cpu=1 pid=99 tid=99 period=1 event=cpu-cycles symbol=Poison dso=lib.so source=test\n"))

	if TracePathRequiresCompositeIndex(child) {
		t.Fatal("a bare same-basename perftrace still forced the systrace into a composite lane")
	}
	idx, err := BuildIndex(context.Background(), child)
	if err != nil || idx == nil {
		t.Fatalf("build direct systrace with bare perf sibling: idx=%+v err=%v", idx, err)
	}
	if idx.Path != canonicalTraceIndexPath(child) || len(idx.TraceArtifacts) != 1 || idx.TraceArtifacts[0].SourcePath != canonicalTraceIndexPath(child) {
		t.Fatalf("bare perf sibling entered the direct source universe: path=%q artifacts=%+v", idx.Path, idx.TraceArtifacts)
	}
}

func TestTraceBundleV2ConsumerPreCanceledColdDigestDoesNotPopulateAttestation(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	consumerV2WriteFile(t, child, []byte(consumerV2WakeupRow(20, 10)))
	writeTraceBundleV2ForTest(t, bundle, consumerV2SingleSystraceManifest())

	selection, coldErr := resolveTraceIndexSelectionWithPolicy(context.Background(), bundle, false)
	if !errors.Is(coldErr, errTraceBundleDigestAttestationCold) || selection != nil {
		t.Fatalf("cold-read-disabled selection did not stop before digest measurement: selection=%+v err=%v", selection, coldErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	idx, err := BuildIndex(ctx, bundle)
	if !errors.Is(err, context.Canceled) || idx != nil {
		t.Fatalf("pre-canceled cold V2 build did not stop at the cancellation boundary: idx=%+v err=%v", idx, err)
	}
	bundleDigestAttestations.mu.Lock()
	entries := len(bundleDigestAttestations.entries)
	inflight := len(bundleDigestAttestations.inflight)
	bundleDigestAttestations.mu.Unlock()
	if entries != 0 || inflight != 0 {
		t.Fatalf("pre-canceled cold build populated or left a digest scan: entries=%d inflight=%d", entries, inflight)
	}
}

func TestTraceBundleV2ConsumerRejectsDuplicateAndUnknownSchema(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	consumerV2WriteFile(t, child, []byte(consumerV2WakeupRow(20, 10)))
	valid := traceBundleV2JSONForTest(t, bundle, consumerV2SingleSystraceManifest())

	var duplicate traceBundleFile
	if err := json.Unmarshal(valid, &duplicate); err != nil {
		t.Fatal(err)
	}
	duplicate.Artifacts = append(duplicate.Artifacts, duplicate.Artifacts[0])
	duplicateJSON, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	consumerV2WriteFile(t, bundle, duplicateJSON)
	idx, err := BuildIndex(context.Background(), bundle)
	if err == nil || idx != nil || !strings.Contains(err.Error(), "duplicate causal child") {
		t.Fatalf("explicit duplicate V2 child was not rejected: idx=%+v err=%v", idx, err)
	}

	var unknown map[string]any
	if err := json.Unmarshal(valid, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["schema"] = "codrax.tracebundle/v3"
	unknownJSON, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	consumerV2WriteFile(t, bundle, unknownJSON)
	idx, err = BuildIndex(context.Background(), bundle)
	if err == nil || idx != nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("explicit unknown schema was not rejected: idx=%+v err=%v", idx, err)
	}
	direct, err := BuildIndex(context.Background(), child)
	if err != nil || direct == nil || direct.Path != canonicalTraceIndexPath(child) {
		t.Fatalf("unknown optional schema hijacked the direct child: idx=%+v err=%v", direct, err)
	}
}

func TestTraceBundleV2ConsumerRejectsPaddedCausalTypeTokens(t *testing.T) {
	for _, test := range []struct {
		name       string
		artifact   string
		paddedType string
		manifest   []byte
	}{
		{
			name:       "systrace",
			artifact:   "capture.systrace",
			paddedType: " systrace ",
			manifest:   consumerV2SingleSystraceManifest(),
		},
		{
			name:       "perftrace",
			artifact:   "capture.perftrace",
			paddedType: " perftrace ",
			manifest: []byte(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace"}
  ]
}`),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			resetTraceBundleDigestAttestationsForTest()
			dir := t.TempDir()
			child := filepath.Join(dir, "capture.systrace")
			perf := filepath.Join(dir, "capture.perftrace")
			bundlePath := filepath.Join(dir, "capture.tracebundle.json")
			consumerV2WriteFile(t, child, []byte(consumerV2WakeupRow(20, 10)))
			consumerV2WriteFile(t, perf, []byte("app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=X dso=lib.so source=test\n"))

			var bundle traceBundleFile
			if err := json.Unmarshal(traceBundleV2JSONForTest(t, bundlePath, test.manifest), &bundle); err != nil {
				t.Fatal(err)
			}
			found := false
			for index := range bundle.Artifacts {
				if bundle.Artifacts[index].Path == test.artifact {
					bundle.Artifacts[index].Type = test.paddedType
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("fixture lacks artifact %q: %+v", test.artifact, bundle.Artifacts)
			}
			body, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			consumerV2WriteFile(t, bundlePath, body)

			idx, err := BuildIndex(context.Background(), bundlePath)
			if err == nil || idx != nil || !strings.Contains(err.Error(), "type must be exact and unpadded") {
				t.Fatalf("explicit V2 accepted padded %s token: idx=%+v err=%v", test.name, idx, err)
			}
		})
	}
}

func TestTraceBundleV2ConsumerSurvivesDirectoryMoveAndIgnoresCWD(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	root := t.TempDir()
	originalDir := filepath.Join(root, "original")
	movedDir := filepath.Join(root, "moved")
	poisonDir := filepath.Join(root, "cwd-poison")
	for _, dir := range []string{originalDir, poisonDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	originalChild := filepath.Join(originalDir, "capture.systrace")
	originalBundle := filepath.Join(originalDir, "capture.tracebundle.json")
	consumerV2WriteFile(t, originalChild, []byte(consumerV2WakeupRow(20, 10)))
	writeTraceBundleV2ForTest(t, originalBundle, consumerV2SingleSystraceManifest())
	if err := os.Rename(originalDir, movedDir); err != nil {
		t.Fatal(err)
	}
	consumerV2WriteFile(t, filepath.Join(poisonDir, "capture.systrace"), []byte(consumerV2WakeupRow(99, 20)))

	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(poisonDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldCWD); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})

	movedChild := filepath.Join(movedDir, "capture.systrace")
	movedBundle := filepath.Join(movedDir, "capture.tracebundle.json")
	idx, err := BuildIndex(context.Background(), movedChild)
	if err != nil || idx == nil {
		t.Fatalf("moved V2 bundle failed outside its original CWD: idx=%+v err=%v", idx, err)
	}
	if idx.Path != canonicalTraceIndexPath(movedBundle) || len(idx.Events) != 1 || idx.Events[0].PID != 20 {
		t.Fatalf("moved V2 bundle resolved the CWD poison instead of its relative child: path=%q events=%+v", idx.Path, idx.Events)
	}
}

func TestTraceBundleV2ConsumerDigestAttestationLRUEvictsOldestAt128(t *testing.T) {
	file, identity := consumerV2DigestTestFile(t)
	cache := newTraceBundleDigestAttestationCache()
	var measurements atomic.Int64
	cache.measure = func(context.Context, *os.File) (int64, string, traceFileIdentity, error) {
		measurements.Add(1)
		return identity.Size(), "digest", identity, nil
	}

	for index := 0; index < traceBundleDigestAttestationLimit; index++ {
		key := fmt.Sprintf("generation-%03d", index)
		if _, err := cache.loadOrMeasure(context.Background(), key, file, identity, true); err != nil {
			t.Fatalf("prime digest attestation %s: %v", key, err)
		}
	}
	if _, err := cache.loadOrMeasure(context.Background(), "generation-000", file, identity, true); err != nil {
		t.Fatalf("refresh oldest digest attestation: %v", err)
	}
	if _, err := cache.loadOrMeasure(context.Background(), "generation-128", file, identity, true); err != nil {
		t.Fatalf("insert over digest attestation bound: %v", err)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) != traceBundleDigestAttestationLimit || cache.lru.Len() != traceBundleDigestAttestationLimit {
		t.Fatalf("digest LRU exceeded its fixed bound: entries=%d lru=%d limit=%d", len(cache.entries), cache.lru.Len(), traceBundleDigestAttestationLimit)
	}
	if cache.entries["generation-001"] != nil {
		t.Fatal("digest LRU retained the true least-recently-used generation")
	}
	if cache.entries["generation-000"] == nil || cache.entries["generation-128"] == nil {
		t.Fatalf("digest LRU evicted a refreshed/new generation: refreshed=%v new=%v", cache.entries["generation-000"] != nil, cache.entries["generation-128"] != nil)
	}
	seen := make(map[string]struct{}, cache.lru.Len())
	for element := cache.lru.Front(); element != nil; element = element.Next() {
		entry := element.Value.(traceBundleDigestAttestationEntry)
		if _, duplicate := seen[entry.key]; duplicate {
			t.Fatalf("digest LRU contains duplicate key %q", entry.key)
		}
		seen[entry.key] = struct{}{}
		if cache.entries[entry.key] != element {
			t.Fatalf("digest LRU map/list identity drifted for %q", entry.key)
		}
	}
	if got := measurements.Load(); got != traceBundleDigestAttestationLimit+1 {
		t.Fatalf("digest LRU measured %d generations, want %d (refresh must be a hit)", got, traceBundleDigestAttestationLimit+1)
	}
}

func TestTraceBundleV2ConsumerDigestAttestationSingleflightAndWaiterCancellation(t *testing.T) {
	file, identity := consumerV2DigestTestFile(t)
	cache := newTraceBundleDigestAttestationCache()
	started := make(chan struct{})
	release := make(chan struct{})
	joined := make(chan string, 9)
	var measurements atomic.Int64
	cache.measure = func(context.Context, *os.File) (int64, string, traceFileIdentity, error) {
		if measurements.Add(1) != 1 {
			return 0, "", traceFileIdentity{}, fmt.Errorf("same-key digest measured more than once")
		}
		close(started)
		<-release
		return identity.Size(), "digest", identity, nil
	}
	cache.onJoin = func(key string) { joined <- key }

	type result struct {
		value traceBundleDigestAttestation
		err   error
	}
	leaderResult := make(chan result, 1)
	go func() {
		value, err := cache.loadOrMeasure(context.Background(), "same-generation", file, identity, true)
		leaderResult <- result{value: value, err: err}
	}()
	<-started

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	canceledWaiter := make(chan error, 1)
	go func() {
		_, err := cache.loadOrMeasure(waiterCtx, "same-generation", file, identity, true)
		canceledWaiter <- err
	}()
	if key := <-joined; key != "same-generation" {
		t.Fatalf("canceled waiter joined wrong digest key %q", key)
	}
	cancelWaiter()
	if err := <-canceledWaiter; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter inherited a different verdict: %v", err)
	}
	select {
	case result := <-leaderResult:
		t.Fatalf("waiter cancellation stopped the measuring leader: %+v", result)
	default:
	}

	const successfulWaiters = 8
	waiterResults := make([]chan result, successfulWaiters)
	for index := range waiterResults {
		waiterResults[index] = make(chan result, 1)
		go func(output chan<- result) {
			value, err := cache.loadOrMeasure(context.Background(), "same-generation", file, identity, true)
			output <- result{value: value, err: err}
		}(waiterResults[index])
	}
	for range successfulWaiters {
		if key := <-joined; key != "same-generation" {
			t.Fatalf("successful waiter joined wrong digest key %q", key)
		}
	}
	close(release)

	leader := <-leaderResult
	if leader.err != nil || leader.value.sha256 != "digest" {
		t.Fatalf("digest leader failed: %+v", leader)
	}
	for index, output := range waiterResults {
		waiter := <-output
		if waiter.err != nil || waiter.value != leader.value {
			t.Fatalf("digest waiter %d did not receive the leader result: waiter=%+v leader=%+v", index, waiter, leader)
		}
	}
	if got := measurements.Load(); got != 1 {
		t.Fatalf("same-key digest singleflight measured %d times, want 1", got)
	}
	cache.mu.Lock()
	entries, inflight := len(cache.entries), len(cache.inflight)
	cache.mu.Unlock()
	if entries != 1 || inflight != 0 {
		t.Fatalf("same-key digest singleflight terminal state drifted: entries=%d inflight=%d", entries, inflight)
	}
}

func TestTraceBundleV2ConsumerDigestWaiterRetriesAfterLeaderCancellation(t *testing.T) {
	file, identity := consumerV2DigestTestFile(t)
	cache := newTraceBundleDigestAttestationCache()
	firstStarted := make(chan struct{})
	joined := make(chan string, 1)
	var measurements atomic.Int64
	cache.measure = func(ctx context.Context, _ *os.File) (int64, string, traceFileIdentity, error) {
		switch measurements.Add(1) {
		case 1:
			close(firstStarted)
			<-ctx.Done()
			return 0, "", traceFileIdentity{}, ctx.Err()
		case 2:
			return identity.Size(), "digest", identity, nil
		default:
			return 0, "", traceFileIdentity{}, fmt.Errorf("unexpected extra digest measurement")
		}
	}
	cache.onJoin = func(key string) { joined <- key }

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan error, 1)
	go func() {
		_, err := cache.loadOrMeasure(leaderCtx, "same-generation", file, identity, true)
		leaderResult <- err
	}()
	<-firstStarted
	waiterResult := make(chan error, 1)
	go func() {
		_, err := cache.loadOrMeasure(context.Background(), "same-generation", file, identity, true)
		waiterResult <- err
	}()
	if key := <-joined; key != "same-generation" {
		t.Fatalf("live waiter joined wrong digest key %q", key)
	}
	cancelLeader()
	if err := <-leaderResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled digest leader returned %v", err)
	}
	if err := <-waiterResult; err != nil {
		t.Fatalf("live waiter inherited another caller's cancellation: %v", err)
	}
	if got := measurements.Load(); got != 2 {
		t.Fatalf("live waiter performed %d digest measurements after leader cancellation, want one retry", got)
	}
	cache.mu.Lock()
	entries, inflight := len(cache.entries), len(cache.inflight)
	cache.mu.Unlock()
	if entries != 1 || inflight != 0 {
		t.Fatalf("leader-cancellation retry terminal state drifted: entries=%d inflight=%d", entries, inflight)
	}
}

func TestTraceBundleV2ConsumerDigestResetFailsLoudDuringInflight(t *testing.T) {
	file, identity := consumerV2DigestTestFile(t)
	cache := newTraceBundleDigestAttestationCache()
	started := make(chan struct{})
	release := make(chan struct{})
	cache.measure = func(context.Context, *os.File) (int64, string, traceFileIdentity, error) {
		close(started)
		<-release
		return identity.Size(), "digest", identity, nil
	}
	type result struct {
		value traceBundleDigestAttestation
		err   error
	}
	measurement := make(chan result, 1)
	go func() {
		value, err := cache.loadOrMeasure(context.Background(), "active", file, identity, true)
		measurement <- result{value: value, err: err}
	}()
	<-started

	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		cache.resetForTest()
	}()
	if !panicked {
		t.Fatal("digest reset silently discarded an active same-key singleflight")
	}
	select {
	case result := <-measurement:
		t.Fatalf("failing reset disturbed the active digest measurement: %+v", result)
	default:
	}
	close(release)
	measurementResult := <-measurement
	if measurementResult.err != nil || measurementResult.value.sha256 != "digest" {
		t.Fatalf("active digest measurement did not survive failing reset: %+v", measurementResult)
	}
	cache.resetForTest()
	cache.mu.Lock()
	entries, inflight, lru := len(cache.entries), len(cache.inflight), cache.lru.Len()
	cache.mu.Unlock()
	if entries != 0 || inflight != 0 || lru != 0 {
		t.Fatalf("quiescent digest reset did not clear cache: entries=%d inflight=%d lru=%d", entries, inflight, lru)
	}
}

func consumerV2SingleSystraceManifest() []byte {
	return []byte(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[{"type":"systrace","path":"capture.systrace"}]
}`)
}

func consumerV2WakeupRow(pid, ts int) string {
	return fmt.Sprintf("app-%d (%d) [001] .... %d.000000: sched_wakeup: comm=app pid=%d prio=20 target_cpu=001\n", pid, pid, ts, pid)
}

func consumerV2WriteFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func consumerV2DigestTestFile(t *testing.T) (*os.File, traceFileIdentity) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "digest-source.systrace")
	consumerV2WriteFile(t, path, []byte(consumerV2WakeupRow(20, 10)))
	file, identity, err := openTraceSourceRegular(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close digest fixture: %v", err)
		}
	})
	return file, identity
}
