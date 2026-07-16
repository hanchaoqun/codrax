package tracequery

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const traceBundleDigestAttestationLimit = 128

var errTraceBundleDigestAttestationCold = errors.New("trace bundle child digest attestation is not cached")

type traceBundleDigestAttestation struct {
	bytes  int64
	sha256 string
}

type traceBundleDigestAttestationEntry struct {
	key   string
	value traceBundleDigestAttestation
}

type traceBundleDigestAttestationCall struct {
	done            chan struct{}
	value           traceBundleDigestAttestation
	err             error
	contextCanceled bool
}

type traceBundleDigestMeasureFunc func(context.Context, *os.File) (int64, string, traceFileIdentity, error)

type traceBundleDigestAttestationCache struct {
	mu       sync.Mutex
	lru      *list.List
	entries  map[string]*list.Element
	inflight map[string]*traceBundleDigestAttestationCall
	measure  traceBundleDigestMeasureFunc
	// onJoin is a deterministic test seam. Production caches leave it nil.
	onJoin func(string)
}

var bundleDigestAttestations = newTraceBundleDigestAttestationCache()

func newTraceBundleDigestAttestationCache() *traceBundleDigestAttestationCache {
	return &traceBundleDigestAttestationCache{
		lru:      list.New(),
		entries:  make(map[string]*list.Element),
		inflight: make(map[string]*traceBundleDigestAttestationCall),
		measure:  tracebundle.MeasureFile,
	}
}

func attestTraceBundleChild(
	ctx context.Context,
	file *os.File,
	identity traceFileIdentity,
	expectedBytes int64,
	expectedSHA256 string,
	allowColdRead bool,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if file == nil || !identity.Initialized() || !identity.Strong() {
		return fmt.Errorf("trace bundle child has no held strong generation")
	}
	if identity.Size() != expectedBytes {
		return fmt.Errorf("trace bundle child bytes mismatch: got=%d want=%d", identity.Size(), expectedBytes)
	}
	key := identity.CacheToken()
	value, err := bundleDigestAttestations.loadOrMeasure(ctx, key, file, identity, allowColdRead)
	if err != nil {
		return err
	}
	if value.bytes != expectedBytes {
		return fmt.Errorf("trace bundle child measured bytes mismatch: got=%d want=%d", value.bytes, expectedBytes)
	}
	if value.sha256 != expectedSHA256 {
		return fmt.Errorf("trace bundle child sha256 mismatch: got=%s want=%s", value.sha256, expectedSHA256)
	}
	return nil
}

func (cache *traceBundleDigestAttestationCache) loadOrMeasure(
	ctx context.Context,
	key string,
	file *os.File,
	identity traceFileIdentity,
	allowColdRead bool,
) (traceBundleDigestAttestation, error) {
	if cache == nil {
		return traceBundleDigestAttestation{}, fmt.Errorf("trace bundle digest cache is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		cache.mu.Lock()
		if element := cache.entries[key]; element != nil {
			cache.lru.MoveToFront(element)
			value := element.Value.(traceBundleDigestAttestationEntry).value
			cache.mu.Unlock()
			return value, nil
		}
		if !allowColdRead {
			cache.mu.Unlock()
			return traceBundleDigestAttestation{}, errTraceBundleDigestAttestationCold
		}
		if call := cache.inflight[key]; call != nil {
			onJoin := cache.onJoin
			cache.mu.Unlock()
			if onJoin != nil {
				onJoin(key)
			}
			select {
			case <-call.done:
				if err := ctx.Err(); err != nil {
					return traceBundleDigestAttestation{}, err
				}
				// The measuring caller owns only its own cancellation. A live
				// waiter must retry under its own context instead of inheriting a
				// transient cancellation and silently demoting a valid optional
				// bundle to the direct-file lane.
				if call.contextCanceled {
					continue
				}
				return call.value, call.err
			case <-ctx.Done():
				return traceBundleDigestAttestation{}, ctx.Err()
			}
		}
		measure := cache.measure
		if measure == nil {
			cache.mu.Unlock()
			return traceBundleDigestAttestation{}, fmt.Errorf("trace bundle digest measure function is nil")
		}
		call := &traceBundleDigestAttestationCall{done: make(chan struct{})}
		cache.inflight[key] = call
		cache.mu.Unlock()

		measuredBytes, measuredSHA256, measuredIdentity, measureErr := measure(ctx, file)
		if measureErr == nil && !identity.SameVersion(measuredIdentity) {
			measureErr = fmt.Errorf("trace bundle child generation changed while hashing")
		}
		call.value = traceBundleDigestAttestation{bytes: measuredBytes, sha256: measuredSHA256}
		call.err = measureErr
		call.contextCanceled = measureErr != nil && ctx.Err() != nil && errors.Is(measureErr, ctx.Err())

		cache.mu.Lock()
		delete(cache.inflight, key)
		if measureErr == nil {
			if existing := cache.entries[key]; existing != nil {
				existing.Value = traceBundleDigestAttestationEntry{key: key, value: call.value}
				cache.lru.MoveToFront(existing)
			} else {
				element := cache.lru.PushFront(traceBundleDigestAttestationEntry{key: key, value: call.value})
				cache.entries[key] = element
				for cache.lru.Len() > traceBundleDigestAttestationLimit {
					oldest := cache.lru.Back()
					entry := oldest.Value.(traceBundleDigestAttestationEntry)
					delete(cache.entries, entry.key)
					cache.lru.Remove(oldest)
				}
			}
		}
		close(call.done)
		cache.mu.Unlock()
		return call.value, call.err
	}
}

func resetTraceBundleDigestAttestationsForTest() {
	bundleDigestAttestations.resetForTest()
}

func (cache *traceBundleDigestAttestationCache) resetForTest() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.inflight) != 0 {
		panic("trace bundle digest attestation reset while a measurement is in flight")
	}
	cache.lru.Init()
	cache.entries = make(map[string]*list.Element)
	cache.inflight = make(map[string]*traceBundleDigestAttestationCall)
}
