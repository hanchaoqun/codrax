package tracequery

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

const (
	traceInputAdmissionPolicyVersion = "trace-text-admission-v2"
	traceInputAdmissionCacheLimit    = 256
)

type traceInputAdmissionCacheEntry struct{ key string }

type traceInputAdmissionCall struct {
	done chan struct{}
	err  error
}

type traceInputAdmissionMeasureFunc func(context.Context, *os.File, traceFileIdentity, string) error

type traceInputAdmissionCache struct {
	mu       sync.Mutex
	lru      *list.List
	entries  map[string]*list.Element
	inflight map[string]*traceInputAdmissionCall
	measure  traceInputAdmissionMeasureFunc
}

var traceInputAdmissions = newTraceInputAdmissionCache()

var errTraceInputAdmissionCold = errors.New("full trace text admission is not cached")

func newTraceInputAdmissionCache() *traceInputAdmissionCache {
	return &traceInputAdmissionCache{
		lru:      list.New(),
		entries:  make(map[string]*list.Element),
		inflight: make(map[string]*traceInputAdmissionCall),
		measure:  measureHeldTraceInputAdmission,
	}
}

func measureHeldTraceInputAdmission(ctx context.Context, file *os.File, identity traceFileIdentity, displayPath string) error {
	probeErr := attachment.ValidateTextReaderAtFull(
		ctx, attachment.KindTrace, displayPath, file, identity.Size(), attachment.TracePhysicalLineMaxBytes,
	)
	if identityErr := validateHeldTraceFileIdentityAfterRead(file, identity, "full trace input admission"); identityErr != nil {
		// A mixed-generation scan has no content authority. Identity failure
		// therefore dominates a provisional text verdict.
		return identityErr
	}
	return probeErr
}

func (cache *traceInputAdmissionCache) validate(ctx context.Context, file *os.File, identity traceFileIdentity, displayPath string, allowColdRead bool) error {
	if cache == nil {
		return fmt.Errorf("trace input admission cache is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if file == nil || !identity.Initialized() {
		return fmt.Errorf("trace input admission has no held generation")
	}
	if !identity.Strong() {
		// Weak generations cannot authorize a cross-open positive cache hit.
		if !allowColdRead {
			return errTraceInputAdmissionCold
		}
		if cache.measure == nil {
			return fmt.Errorf("trace input admission measure function is nil")
		}
		return cache.measure(ctx, file, identity, displayPath)
	}
	key := traceInputAdmissionPolicyVersion + "\x00" + identity.CacheToken()
	for {
		cache.mu.Lock()
		if element := cache.entries[key]; element != nil {
			cache.lru.MoveToFront(element)
			cache.mu.Unlock()
			if err := ctx.Err(); err != nil {
				return err
			}
			return validateHeldTraceFileIdentityAfterRead(file, identity, "cached full trace input admission")
		}
		if !allowColdRead {
			cache.mu.Unlock()
			return errTraceInputAdmissionCold
		}
		if call := cache.inflight[key]; call != nil {
			cache.mu.Unlock()
			select {
			case <-call.done:
				if err := ctx.Err(); err != nil {
					return err
				}
				// Only positive attestations are shared. A failed leader may carry
				// another hard-link display path or its own cancellation.
				if call.err != nil {
					continue
				}
				return validateHeldTraceFileIdentityAfterRead(file, identity, "joined full trace input admission")
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		measure := cache.measure
		if measure == nil {
			cache.mu.Unlock()
			return fmt.Errorf("trace input admission measure function is nil")
		}
		call := &traceInputAdmissionCall{done: make(chan struct{})}
		cache.inflight[key] = call
		cache.mu.Unlock()

		measureErr := measure(ctx, file, identity, displayPath)
		if measureErr == nil {
			// Match the cache-hit and joined-call contracts: a positive
			// attestation is publishable only while this caller still holds the
			// exact generation that was measured. This second check also closes
			// the measure-return/cache-insert scheduling window.
			measureErr = validateHeldTraceFileIdentityAfterRead(file, identity, "completed full trace input admission")
		}
		call.err = measureErr

		cache.mu.Lock()
		delete(cache.inflight, key)
		if measureErr == nil {
			if existing := cache.entries[key]; existing != nil {
				cache.lru.MoveToFront(existing)
			} else {
				element := cache.lru.PushFront(traceInputAdmissionCacheEntry{key: key})
				cache.entries[key] = element
				for cache.lru.Len() > traceInputAdmissionCacheLimit {
					oldest := cache.lru.Back()
					entry := oldest.Value.(traceInputAdmissionCacheEntry)
					delete(cache.entries, entry.key)
					cache.lru.Remove(oldest)
				}
			}
		}
		close(call.done)
		cache.mu.Unlock()
		return measureErr
	}
}

func resetTraceInputAdmissionsForTest() {
	traceInputAdmissions.resetForTest()
}

func (cache *traceInputAdmissionCache) resetForTest() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.inflight) != 0 {
		panic("trace input admission reset while a measurement is in flight")
	}
	cache.lru.Init()
	cache.entries = make(map[string]*list.Element)
	cache.inflight = make(map[string]*traceInputAdmissionCall)
}
