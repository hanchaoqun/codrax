package types

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

// TestBusContextShallowClone_PropagatesEveryExportedField pins the
// ShallowClone contract from the propagation side: every exported
// BusContext field set to a non-zero sentinel on the source must come
// back equal (same shallow-aliasing semantics as a `cp := *ctx` value
// copy) on the clone. A new exported field whose type the generic
// sentinel builder cannot populate fails loudly with the field name —
// add a handler, never skip the field.
func TestBusContextShallowClone_PropagatesEveryExportedField(t *testing.T) {
	src := &BusContext{}
	populateExportedSentinels(t, reflect.ValueOf(src).Elem())

	clone := src.ShallowClone()
	if clone == nil {
		t.Fatal("ShallowClone returned nil for non-nil BusContext")
	}
	assertExportedFieldsEqual(t, reflect.ValueOf(src).Elem(), reflect.ValueOf(clone).Elem())
}

// TestAgentContextShallowClone_PropagatesEveryExportedField mirrors
// the propagation pin for AgentContext (same unexported cache block,
// same constructor discipline).
func TestAgentContextShallowClone_PropagatesEveryExportedField(t *testing.T) {
	src := &AgentContext{}
	populateExportedSentinels(t, reflect.ValueOf(src).Elem())

	clone := src.ShallowClone()
	if clone == nil {
		t.Fatal("ShallowClone returned nil for non-nil AgentContext")
	}
	assertExportedFieldsEqual(t, reflect.ValueOf(src).Elem(), reflect.ValueOf(clone).Elem())
}

// TestBusContextShallowClone_FreshAnswerPlanCache pins the reset side
// of the contract: the clone must NOT inherit the source's compiled
// answer-surface projections (they are keyed to the source's
// MutableState revision), and the two caches must be fully
// independent afterwards — storing on one never becomes visible on
// the other.
func TestBusContextShallowClone_FreshAnswerPlanCache(t *testing.T) {
	src := &BusContext{AnalysisIR: &AnalysisIR{}}
	srcPlan := &AnswerSurfacePlan{SubRepoNames: []string{"src-cached"}}
	src.storeAnswerSurfacePlan(srcPlan)
	if got := src.cachedAnswerSurfacePlan(); got == nil {
		t.Fatal("test setup: source cache store did not take")
	}

	clone := src.ShallowClone()

	// Reset: the clone starts with an empty cache even though the
	// cache key inputs (AnalysisIR pointer, slice lengths, Mutable
	// revision) are identical to the source's.
	if got := clone.cachedAnswerSurfacePlan(); got != nil {
		t.Fatalf("clone inherited the source's cached AnswerSurfacePlan: %+v", got)
	}

	// Independence, clone → source: storing on the clone must not
	// replace or invalidate the source's cached plan.
	clone.storeAnswerSurfacePlan(&AnswerSurfacePlan{SubRepoNames: []string{"clone-cached"}})
	got := src.cachedAnswerSurfacePlan()
	if got == nil {
		t.Fatal("clone store invalidated the source's cache")
	}
	if len(got.SubRepoNames) != 1 || got.SubRepoNames[0] != "src-cached" {
		t.Fatalf("clone store leaked into the source's cache: %+v", got)
	}

	// Independence, source → clone: invalidating the source must not
	// drop the clone's entry.
	src.InvalidateAnswerPlanCache()
	if got := clone.cachedAnswerSurfacePlan(); got == nil || got.SubRepoNames[0] != "clone-cached" {
		t.Fatalf("source invalidation reached the clone's cache: %+v", got)
	}
}

// TestBusContextShallowClone_RaceWithSourceCacheUse is the regression
// pin for the explore_parallel_dispatch copylocks fix. The replaced
// pattern (`workerBus := *o.busCtx` inside each worker goroutine) read
// the unexported cache fields without the lock, so any concurrent
// locked cache write on the source raced with the copy. ShallowClone
// reads only exported fields, so cloning is safe even while the source
// cache is actively used. Run under -race; the old pattern fails here.
func TestBusContextShallowClone_RaceWithSourceCacheUse(t *testing.T) {
	src := &BusContext{
		AnalysisIR:    &AnalysisIR{},
		EvidenceItems: []EvidenceItem{{}},
	}

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})

	// Source-side cache churn: locked writes + invalidations, the
	// counterpart the unlocked struct copy used to race with.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 200; i++ {
			src.storeAnswerSurfacePlan(&AnswerSurfacePlan{SubRepoNames: []string{"churn"}})
			_ = src.cachedAnswerSurfacePlan()
			src.InvalidateAnswerPlanCache()
		}
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 50; i++ {
				clone := src.ShallowClone()
				// The dispatch-site overrides on the worker's own copy.
				clone.PipelineStage = StageExplore
				clone.TaskState.Stage = StageExplore
				// Worker-local cache use on the independent mutex.
				clone.storeAnswerSurfacePlan(&AnswerSurfacePlan{SubRepoNames: []string{"worker"}})
				if got := clone.cachedAnswerSurfacePlan(); got == nil {
					t.Error("worker clone lost its own cache entry")
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()
}

// populateExportedSentinels sets every exported field of v (an
// addressable struct value) to a non-zero sentinel. Fails the test
// naming any field it cannot populate, so adding an exported field
// with a novel type to BusContext/AgentContext forces this test to be
// taught about it rather than silently shrinking coverage.
func populateExportedSentinels(t *testing.T, v reflect.Value) {
	t.Helper()
	st := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue // unexported: the cache block, intentionally excluded
		}
		name := st.Field(i).Name
		sentinel, ok := sentinelValue(f.Type(), name)
		if !ok {
			t.Errorf("%s.%s: no sentinel handler for type %s — extend sentinelValue so the ShallowClone propagation pin keeps covering every exported field",
				st.Name(), name, f.Type())
			continue
		}
		if sentinel.IsZero() {
			t.Errorf("%s.%s: sentinel for type %s is zero-valued — the propagation check would pass vacuously",
				st.Name(), name, f.Type())
			continue
		}
		f.Set(sentinel)
	}
}

// ctxCloneSentinelKey is the private context key for the Ctx-field
// sentinel below.
type ctxCloneSentinelKey struct{}

// sentinelValue builds a non-zero value of type ft. Interface fields
// need concrete handlers; everything else is derived generically from
// the type's kind.
func sentinelValue(ft reflect.Type, name string) (reflect.Value, bool) {
	switch ft.Kind() {
	case reflect.String:
		return reflect.ValueOf("sentinel-" + name).Convert(ft), true
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(ft), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflect.ValueOf(int64(7)).Convert(ft), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflect.ValueOf(uint64(7)).Convert(ft), true
	case reflect.Float32, reflect.Float64:
		return reflect.ValueOf(float64(7)).Convert(ft), true
	case reflect.Slice:
		s := reflect.MakeSlice(ft, 1, 1)
		if elem, ok := sentinelValue(ft.Elem(), name); ok {
			s.Index(0).Set(elem)
		}
		return s, true // non-nil slice is non-zero even with a zero element
	case reflect.Map:
		m := reflect.MakeMap(ft)
		k, kok := sentinelValue(ft.Key(), name)
		val, vok := sentinelValue(ft.Elem(), name)
		if kok && vok {
			m.SetMapIndex(k, val)
		}
		return m, true // non-nil map is non-zero
	case reflect.Ptr:
		p := reflect.New(ft.Elem())
		if elem, ok := sentinelValue(ft.Elem(), name); ok {
			p.Elem().Set(elem)
		}
		return p, true // non-nil pointer is non-zero
	case reflect.Struct:
		s := reflect.New(ft).Elem()
		populated := false
		for i := 0; i < s.NumField(); i++ {
			f := s.Field(i)
			if !f.CanSet() {
				continue
			}
			if fv, ok := sentinelValue(f.Type(), name); ok {
				f.Set(fv)
				populated = true
				break
			}
		}
		return s, populated
	case reflect.Interface:
		switch ft {
		case reflect.TypeOf((*context.Context)(nil)).Elem():
			// context.Background() is a zero-valued struct in modern
			// Go; WithValue yields a non-nil *valueCtx so the
			// vacuous-sentinel guard keeps its teeth.
			return reflect.ValueOf(context.WithValue(context.Background(), ctxCloneSentinelKey{}, name)), true
		case reflect.TypeOf((*MemoryReader)(nil)).Elem():
			return reflect.ValueOf(nopMemoryReader{}).Convert(ft), true
		}
		if ft.NumMethod() == 0 { // any
			return reflect.ValueOf("sentinel-any-" + name), true
		}
		return reflect.Value{}, false
	default:
		return reflect.Value{}, false
	}
}

// assertExportedFieldsEqual checks that every exported field of clone
// deep-equals the source's. Shallow-clone semantics make this exact:
// reference kinds alias the same backing data, value kinds are copies.
func assertExportedFieldsEqual(t *testing.T, src, clone reflect.Value) {
	t.Helper()
	st := src.Type()
	for i := 0; i < src.NumField(); i++ {
		if !clone.Field(i).CanSet() {
			continue // unexported cache block: reset by design, pinned elsewhere
		}
		name := st.Field(i).Name
		if !reflect.DeepEqual(src.Field(i).Interface(), clone.Field(i).Interface()) {
			t.Errorf("%s.%s: ShallowClone dropped or altered the field\n  src:   %#v\n  clone: %#v",
				st.Name(), name, src.Field(i).Interface(), clone.Field(i).Interface())
		}
	}
}
