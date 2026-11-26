package types

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

// typed_input_digest_test.go — the sample is default-include. These tests pin
// that property structurally: every field of the bus and of the mutable state
// moves the digest unless the closed exclusion table or a type rule says why
// it cannot, and whatever the walk cannot read makes the sample unknown. The
// fields that hold a service or an index are the one other closed table: the
// object behind each is compared by identity.

func sampleOf(bus *BusContext) TypedInputSample {
	return SampleAnswerRequestInputs(bus, AnswerDispatchStamp{})
}

// digestOf runs the walker over one value with its own budget.
func digestOf(t *testing.T, value any) (digest, unknown string) {
	t.Helper()
	budget := typedInputNodeLimit
	w := &typedInputWalker{h: sha256.New(), seen: map[typedInputRef]int{}, budget: &budget}
	w.walkGuarded(reflect.ValueOf(value))
	return hex.EncodeToString(w.h.Sum(nil)), w.unknown
}

func TestTypedInputSampleCoversEveryExportedBusField(t *testing.T) {
	noDataByTypeRule := map[string]bool{"Ctx": true}
	typ := reflect.TypeOf(BusContext{})
	excluded := typedInputExcludedFields[typ]
	covered := 0
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Name == "Mutable" {
			continue
		}
		base := &BusContext{Mutable: NewMutableState("question")}
		before := sampleOf(base)
		if before.Digest() == "" {
			t.Fatalf("baseline bus must be readable: %s", before.Unknown())
		}
		sentinel, ok := sentinelValue(field.Type, field.Name)
		if !ok || sentinel.IsZero() {
			t.Errorf("BusContext.%s: no sentinel for type %s — teach sentinelValue so the field stays covered", field.Name, field.Type)
			continue
		}
		reflect.ValueOf(base).Elem().Field(i).Set(sentinel)
		after := sampleOf(base)
		_, listed := excluded[field.Name]
		if listed || noDataByTypeRule[field.Name] {
			if !before.Same(after) {
				t.Errorf("BusContext.%s is declared to carry no input but moved the digest: %v", field.Name, before.Changed(after))
			}
			continue
		}
		covered++
		if before.Same(after) {
			t.Errorf("BusContext.%s changed and the digest did not: a rebuild would be suppressed", field.Name)
			continue
		}
		if got := before.Changed(after); len(got) != 1 || got[0] != "bus."+field.Name {
			t.Errorf("BusContext.%s: changed carriers = %v, want exactly bus.%s", field.Name, got, field.Name)
		}
	}
	if covered < 40 {
		t.Fatalf("vacuous: only %d bus fields were exercised", covered)
	}
}

type digestStubOracle struct{ tier int }

func (o digestStubOracle) SymbolExists(string) (bool, int)     { return true, o.tier }
func (o digestStubOracle) SymbolExistsFlat(string) (bool, int) { return true, o.tier }

var interfaceChanges = map[reflect.Type]any{
	reflect.TypeOf((*SymbolOracle)(nil)).Elem(): digestStubOracle{tier: 2},
}

// forceChange mutates an addressable value in place so that its content
// differs from before, entering unexported fields. It reports false when the
// value holds nothing the sample reads (a lock, a clock reading).
func forceChange(v reflect.Value, depth int) bool {
	if depth > 6 || typedInputRuleFor(v.Type()) != typedInputWalk {
		return false
	}
	if v.CanAddr() && !v.CanSet() {
		v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "+changed")
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 7)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(v.Uint() + 7)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 7)
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	case reflect.Map:
		grown := reflect.MakeMap(v.Type())
		for iter := v.MapRange(); iter.Next(); {
			grown.SetMapIndex(iter.Key(), iter.Value())
		}
		key := reflect.New(v.Type().Key()).Elem()
		if !forceChange(key, depth+1) {
			return false
		}
		grown.SetMapIndex(key, reflect.Zero(v.Type().Elem()))
		v.Set(grown)
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
			return true
		}
		return forceChange(v.Elem(), depth+1)
	case reflect.Interface:
		if v.NumMethod() == 0 {
			v.Set(reflect.ValueOf("changed"))
			return true
		}
		// An interface with methods needs an implementation; one that is
		// missing here surfaces as an unexpected "carries no input" field.
		impl, ok := interfaceChanges[v.Type()]
		if !ok {
			return false
		}
		v.Set(reflect.ValueOf(impl))
	case reflect.Struct:
		excluded := typedInputExcludedFields[v.Type()]
		for i := 0; i < v.NumField(); i++ {
			if _, skip := excluded[v.Type().Field(i).Name]; skip {
				continue
			}
			if forceChange(v.Field(i), depth+1) {
				return true
			}
		}
		return false
	case reflect.Array:
		return v.Len() > 0 && forceChange(v.Index(0), depth+1)
	case reflect.Func:
		v.Set(reflect.MakeFunc(v.Type(), func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, v.Type().NumOut())
			for i := range out {
				out[i] = reflect.Zero(v.Type().Out(i))
			}
			return out
		}))
	case reflect.Chan:
		v.Set(reflect.MakeChan(v.Type(), 0))
	default:
		return false
	}
	return true
}

func TestTypedInputSampleCoversEveryMutableStateField(t *testing.T) {
	typ := reflect.TypeOf(MutableState{})
	excluded := typedInputExcludedFields[typ]
	var holdsNoInput []string
	covered := 0
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		bus := &BusContext{Mutable: NewMutableState("question")}
		before := sampleOf(bus)
		if before.Digest() == "" {
			t.Fatalf("baseline state must be readable: %s", before.Unknown())
		}
		changed := forceChange(reflect.ValueOf(bus.Mutable).Elem().Field(i), 0)
		after := sampleOf(bus)
		if _, listed := excluded[name]; listed || !changed {
			if !changed {
				holdsNoInput = append(holdsNoInput, name)
			}
			if !before.Same(after) {
				t.Errorf("MutableState.%s is declared to carry no input but moved the digest: %v", name, before.Changed(after))
			}
			continue
		}
		covered++
		if after.Digest() == "" {
			t.Errorf("MutableState.%s: the forced change must stay readable so the field's own digest is checked: %s", name, after.Unknown())
			continue
		}
		if before.Same(after) {
			t.Errorf("MutableState.%s changed and the digest did not: a rebuild would be suppressed", name)
			continue
		}
		if got := before.Changed(after); len(got) != 1 || got[0] != "state."+name {
			t.Errorf("MutableState.%s: changed carriers = %v, want exactly state.%s", name, got, name)
		}
	}
	// Fields the walk reads nothing from, by type rule alone. A new name
	// here needs the same scrutiny as a new entry in the exclusion table.
	want := []string{"mu"}
	sort.Strings(holdsNoInput)
	if strings.Join(holdsNoInput, ",") != strings.Join(want, ",") {
		t.Errorf("fields of MutableState that carry no sampled input = %v, want %v", holdsNoInput, want)
	}
	if covered < 150 {
		t.Fatalf("vacuous: only %d state fields were exercised", covered)
	}
	t.Logf("state fields exercised: %d of %d", covered, typ.NumField())
}

type digestProbe struct {
	Name     string
	count    int
	at       time.Time
	took     time.Duration
	lock     sync.Mutex
	cancel   context.Context
	hook     func()
	signal   chan int
	labels   map[string][]string
	children []*digestProbe
	parent   *digestProbe
	any      any
}

func TestTypedInputDigestIsContentNotIdentityOrClock(t *testing.T) {
	build := func(order []string, at time.Time) *digestProbe {
		root := &digestProbe{Name: "root", count: 3, at: at, took: time.Second, labels: map[string][]string{}}
		for _, key := range order {
			root.labels[key] = []string{key + "-1", key + "-2"}
		}
		child := &digestProbe{Name: "child", parent: root, any: []int{1, 2}}
		root.children = []*digestProbe{child, child}
		return root
	}
	first, unknown := digestOf(t, build([]string{"a", "b", "c", "d", "e", "f"}, time.Unix(1, 0)))
	if unknown != "" {
		t.Fatalf("a cyclic, aliased value must be readable: %s", unknown)
	}
	// Separate allocations, another insertion order, another clock reading,
	// a held lock, a set context: the same content.
	other := build([]string{"f", "d", "b", "e", "c", "a"}, time.Unix(999999, 0))
	other.lock.Lock()
	other.cancel = context.Background()
	if second, _ := digestOf(t, other); second != first {
		t.Fatal("equal typed content must give an equal digest whatever its addresses, map order, clock reading or lock state")
	}
	for name, mutate := range map[string]func(*digestProbe){
		"string":           func(p *digestProbe) { p.Name = "Root" },
		"unexported int":   func(p *digestProbe) { p.count++ },
		"duration":         func(p *digestProbe) { p.took += time.Nanosecond },
		"map value":        func(p *digestProbe) { p.labels["c"][1] = "changed" },
		"map key":          func(p *digestProbe) { p.labels["g"] = nil },
		"nil versus empty": func(p *digestProbe) { p.labels["a"] = p.labels["a"][:0]; p.labels["b"] = nil },
		"nested pointer":   func(p *digestProbe) { p.children[0].Name = "other" },
		"alias broken":     func(p *digestProbe) { p.children[1] = &digestProbe{Name: "child", parent: p, any: []int{1, 2}} },
		"interface value":  func(p *digestProbe) { p.children[0].any = []int{1, 3} },
		"interface type":   func(p *digestProbe) { p.children[0].any = []int64{1, 2} },
		"function set":     func(p *digestProbe) { p.hook = func() {} },
		"channel set":      func(p *digestProbe) { p.signal = make(chan int) },
	} {
		probe := build([]string{"a", "b", "c", "d", "e", "f"}, time.Unix(1, 0))
		mutate(probe)
		if got, unknown := digestOf(t, probe); got == first || unknown != "" {
			t.Errorf("%s: a content change must move the digest (unknown=%q)", name, unknown)
		}
	}
	// A nil and an empty collection are different content: they serialize
	// differently, so a request built from one is not the request built from
	// the other.
	nilSlice, _ := digestOf(t, struct{ s []string }{})
	emptySlice, _ := digestOf(t, struct{ s []string }{s: []string{}})
	nilMap, _ := digestOf(t, struct{ m map[string]int }{})
	emptyMap, _ := digestOf(t, struct{ m map[string]int }{m: map[string]int{}})
	if nilSlice == "" || nilMap == "" || nilSlice == emptySlice || nilMap == emptyMap {
		t.Error("a nil collection and an empty one must not share a digest")
	}
}

func TestTypedInputDigestRefusesWhatItCannotRead(t *testing.T) {
	number := 1
	twinA, twinB := &digestProbe{Name: "twin"}, &digestProbe{Name: "twin"}
	for name, value := range map[string]any{
		"unsafe pointer":          struct{ p unsafe.Pointer }{unsafe.Pointer(&number)},
		"sync.Map":                &struct{ m sync.Map }{},
		"reflect value":           struct{ v reflect.Value }{reflect.ValueOf(number)},
		"keys with equal content": map[*digestProbe]int{twinA: 1, twinB: 2},
	} {
		if _, unknown := digestOf(t, value); unknown == "" {
			t.Errorf("%s: the walk cannot read this faithfully and must say so", name)
		}
	}
	// A value reflection itself rejects ends the sample; it never ends the run.
	limit := typedInputNodeLimit
	invalid := &typedInputWalker{h: sha256.New(), seen: map[typedInputRef]int{}, budget: &limit}
	invalid.walkGuarded(reflect.Value{})
	if !strings.HasPrefix(invalid.unknown, "unreadable value: ") {
		t.Errorf("a value the walk panics on must be reported unknown, got %q", invalid.unknown)
	}
	budget := 3
	w := &typedInputWalker{h: sha256.New(), seen: map[typedInputRef]int{}, budget: &budget}
	w.walkGuarded(reflect.ValueOf([]int{1, 2, 3, 4, 5, 6}))
	if w.unknown == "" {
		t.Error("a state past the node limit must be reported unknown, not truncated")
	}
}

// An unknown sample equals nothing, itself included: the caller rebuilds.
func TestUnknownTypedInputSampleIsNeverTheSame(t *testing.T) {
	bus := &BusContext{Mutable: NewMutableState("question")}
	bus.Mutable.repairExecutionPlan = &sync.Map{}
	unknown := sampleOf(bus)
	if unknown.Digest() != "" || unknown.Unknown() == "" || !strings.Contains(unknown.Unknown(), "state.repairExecutionPlan") {
		t.Fatalf("an unreadable carrier must make the sample unknown and be named: %+v", unknown)
	}
	if unknown.Same(unknown) || unknown.Same(sampleOf(bus)) {
		t.Fatal("an unknown sample must not equal itself")
	}
	known := sampleOf(&BusContext{Mutable: NewMutableState("question")})
	if known.Same(unknown) || unknown.Same(known) || !known.Same(known) {
		t.Fatal("only two readable, equal samples are the same")
	}
	if sampleOf(nil).Digest() != "" || (TypedInputSample{}).Same(TypedInputSample{}) {
		t.Fatal("a missing bus and the zero sample are unknown")
	}
}

func TestTypedInputSampleReadsTheDispatchStampAndIgnoresTheRunIdentity(t *testing.T) {
	bus := &BusContext{Mutable: NewMutableState("question"), TraceID: "trace-1"}
	before := SampleAnswerRequestInputs(bus, AnswerDispatchStamp{})
	stamped := SampleAnswerRequestInputs(bus, AnswerDispatchStamp{EmitRetryAttempt: 1})
	if got := before.Changed(stamped); before.Same(stamped) || len(got) != 1 || got[0] != "dispatch" {
		t.Fatalf("a different dispatch stamp is a different input: %v", got)
	}
	bus.TraceID = "trace-2"
	bus.TaskState.LastError = "earlier failure"
	if !before.Same(SampleAnswerRequestInputs(bus, AnswerDispatchStamp{})) {
		t.Fatal("the run identity and the cleared error surface are not inputs")
	}
	// The same typed content in a second, separately built state: the digest
	// holds no address, so the two agree.
	twin := &BusContext{Mutable: NewMutableState("question"), TraceID: "trace-3"}
	if !before.Same(SampleAnswerRequestInputs(twin, AnswerDispatchStamp{})) {
		t.Fatal("equal typed inputs in two states must give equal digests")
	}
}

func TestTypedInputExclusionTableNamesRealFields(t *testing.T) {
	rows := typedInputFieldCensus(typedInputExcludedFields)
	if len(rows) < 10 {
		t.Fatalf("vacuous exclusion table: %v", rows)
	}
	for typ, fields := range typedInputExcludedFields {
		for name, reason := range fields {
			field, ok := typ.FieldByName(name)
			if !ok {
				t.Errorf("%s.%s is excluded but does not exist; remove the entry or follow the rename", typ.Name(), name)
				continue
			}
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s.%s is excluded without a reason", typ.Name(), name)
			}
			if typedInputRuleFor(field.Type) != typedInputWalk {
				t.Errorf("%s.%s needs no entry: the type rules already leave %s out", typ.Name(), name, field.Type)
			}
		}
	}
	// The table is closed. Leaving a field out of the sample is a claim that
	// no answer request is built from it, so a new entry is made here, next
	// to that claim's evidence, and never only in the table.
	want := []string{
		"BusContext.Mutable",
		"BusContext.TraceID",
		"BusContext.answerSemanticView",
		"BusContext.answerSemanticViewCached",
		"BusContext.answerSupportPlan",
		"BusContext.answerSupportPlanCached",
		"BusContext.answerSurfaceCacheKey",
		"BusContext.answerSurfacePlan",
		"BusContext.answerSurfacePlanCached",
		"BusContext.groundingContextCacheKey",
		"BusContext.groundingContextCacheValue",
		"EvidenceClosure.nodeExecAttempts",
		"EvidenceClosure.nodeExecStatus",
		"MutableState.finalizeRequests",
		"TaskState.LastError",
	}
	var got []string
	for _, row := range rows {
		name, _, _ := strings.Cut(row, ": ")
		got = append(got, name)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the exclusion table changed:\n got: %v\nwant: %v", got, want)
	}
}

// digestService stands for a service or an index behind a handle field. The
// walk cannot read its state (a sync.Map makes a walked sample unknown), so a
// readable sample proves the object was not entered.
type digestService struct {
	name  string
	cache sync.Map
}

func (*digestService) SymbolExists(string) (bool, int)                    { return false, 0 }
func (*digestService) SymbolExistsFlat(string) (bool, int)                { return false, 0 }
func (*digestService) Search(string, MemorySearchOpts) []MemoryIndexEntry { return nil }

// setHandle points a handle field of a sampled state at the given object. A
// map of handles gets the object under one key. A nil object clears the field
// (an empty map for a map of handles).
func setHandle(t *testing.T, bus *BusContext, owner reflect.Type, name string, service *digestService) {
	t.Helper()
	var root reflect.Value
	switch owner {
	case reflect.TypeOf(BusContext{}):
		root = reflect.ValueOf(bus).Elem()
	case reflect.TypeOf(MutableState{}):
		root = reflect.ValueOf(bus.Mutable).Elem()
	default:
		t.Fatalf("handle field on %s: teach this helper where that struct lives in a sampled state", owner)
	}
	field := root.FieldByName(name)
	field = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	switch field.Kind() {
	case reflect.Interface:
		if service == nil {
			field.Set(reflect.Zero(field.Type()))
			return
		}
		field.Set(reflect.ValueOf(service))
	case reflect.Map:
		held := reflect.MakeMap(field.Type())
		if service != nil {
			held.SetMapIndex(reflect.ValueOf("scope"), reflect.ValueOf(service))
		}
		field.Set(held)
	default:
		t.Fatalf("%s.%s: a handle field is an interface or a map of them, not %s", owner.Name(), name, field.Type())
	}
}

func typedInputCarrierName(owner reflect.Type, field string) string {
	if owner == reflect.TypeOf(BusContext{}) {
		return "bus." + field
	}
	return "state." + field
}

// A handle field holds a service or an index. Its object is not entered; two
// samples agree on it only when it is the same object.
func TestHandleFieldsAreComparedByIdentity(t *testing.T) {
	exercised := 0
	for owner, fields := range typedInputHandleFields {
		for name := range fields {
			carrier := typedInputCarrierName(owner, name)
			bus := &BusContext{Mutable: NewMutableState("question")}
			first, second := &digestService{name: "index"}, &digestService{name: "index"}
			setHandle(t, bus, owner, name, nil)
			absent := sampleOf(bus)
			setHandle(t, bus, owner, name, first)
			held := sampleOf(bus)
			if held.Digest() == "" {
				t.Errorf("%s: the object behind a handle must not be entered: %s", carrier, held.Unknown())
				continue
			}
			exercised++
			if absent.Same(held) || absent.Digest() == held.Digest() {
				t.Errorf("%s: a handle that appears is a different input", carrier)
			}
			if !held.Same(sampleOf(bus)) {
				t.Errorf("%s: the same object twice is the same input", carrier)
			}
			// What the object keeps for itself is not read: a memo filled
			// behind the same object leaves the sample as it was.
			first.name = "index with a memo"
			first.cache.Store("memo", 1)
			if !held.Same(sampleOf(bus)) {
				t.Errorf("%s: the state behind an unchanged handle is not sampled", carrier)
			}
			setHandle(t, bus, owner, name, second)
			replaced := sampleOf(bus)
			if replaced.Digest() != held.Digest() {
				t.Errorf("%s: the digest holds no address, so another object of the same type in the same place gives the same digest", carrier)
			}
			if held.Same(replaced) || replaced.Same(held) {
				t.Errorf("%s: another object behind the handle is another input; a rebuild would be suppressed", carrier)
			}
			if got := held.Changed(replaced); len(got) != 1 || got[0] != carrier {
				t.Errorf("%s: changed carriers = %v, want exactly this one", carrier, got)
			}
		}
	}
	if exercised < 7 {
		t.Fatalf("vacuous: %d handle fields exercised", exercised)
	}
}

// A handle field that holds a plain value has nothing to identify but its
// content, and an unset pointer is not the object it could point to.
func TestHandleFieldWithoutAnObjectIsReadByContent(t *testing.T) {
	bus := &BusContext{Mutable: NewMutableState("question")}
	digests := map[string]string{}
	for name, value := range map[string]any{
		"unset":         nil,
		"text":          "graph v1",
		"other text":    "graph v2",
		"unset pointer": (*digestService)(nil),
		"object":        &digestService{},
		"other object":  &digestPreparer{},
	} {
		bus.SearchGraph = value
		sample := sampleOf(bus)
		if sample.Digest() == "" || !sample.Same(sampleOf(bus)) {
			t.Fatalf("%s: must be readable and stable: %s", name, sample.Unknown())
		}
		for other, digest := range digests {
			if digest == sample.Digest() {
				t.Errorf("%s and %s are different inputs and share a digest", name, other)
			}
		}
		digests[name] = sample.Digest()
	}
	// A map of handles: the keys are content, each value is a handle.
	state := NewMutableState("question")
	shared := &digestService{}
	state.scopedSearchGraphs = map[string]any{"a": shared}
	one := sampleOf(&BusContext{Mutable: state})
	state.scopedSearchGraphs = map[string]any{"b": shared}
	if other := sampleOf(&BusContext{Mutable: state}); one.Digest() == "" || one.Digest() == other.Digest() {
		t.Error("the keys of a map of handles are content")
	}
	state.scopedSearchGraphs = map[string]any{}
	empty := sampleOf(&BusContext{Mutable: state})
	state.scopedSearchGraphs = nil
	if none := sampleOf(&BusContext{Mutable: state}); none.Digest() == "" || none.Digest() == empty.Digest() || empty.Digest() == one.Digest() {
		t.Error("a map of handles that is not there, an empty one and a filled one are three inputs")
	}
}

// Two carriers hold the same objects only when they hold as many, each the
// same one in the same place.
func TestSameTypedInputHandlesComparesEveryPlace(t *testing.T) {
	a, b := reflect.ValueOf(&digestService{}), reflect.ValueOf(&digestService{})
	for name, tc := range map[string]struct {
		left, right []reflect.Value
		want        bool
	}{
		"none":           {nil, nil, true},
		"same":           {[]reflect.Value{a, b}, []reflect.Value{a, b}, true},
		"one more":       {[]reflect.Value{a}, []reflect.Value{a, b}, false},
		"one fewer":      {[]reflect.Value{a, b}, []reflect.Value{a}, false},
		"another object": {[]reflect.Value{a, a}, []reflect.Value{a, b}, false},
		"another order":  {[]reflect.Value{a, b}, []reflect.Value{b, a}, false},
	} {
		if got := sameTypedInputHandles(tc.left, tc.right); got != tc.want {
			t.Errorf("%s: same = %t, want %t", name, got, tc.want)
		}
	}
}

// The rule belongs to the field, wherever its struct is met: a state reached
// through another carrier keeps its handles closed too.
func TestHandleFieldsStayClosedInsideAWalkedValue(t *testing.T) {
	nested := NewMutableState("question")
	nested.searchGraph = &digestService{}
	if _, unknown := digestOf(t, struct{ owner *MutableState }{nested}); unknown != "" {
		t.Fatalf("a handle inside a walked state must not be entered: %s", unknown)
	}
}

// The sample holds the objects it compares, so none can be collected and its
// address handed to another object while the sample stands.
func TestTypedInputSampleKeepsItsHandlesAlive(t *testing.T) {
	collected := make(chan struct{})
	bus := &BusContext{Mutable: NewMutableState("question")}
	func() {
		service := &digestService{name: "index"}
		runtime.SetFinalizer(service, func(*digestService) { close(collected) })
		bus.SearchGraph = service
	}()
	sample := sampleOf(bus)
	bus.SearchGraph = nil
	for i := 0; i < 5; i++ {
		runtime.GC()
		select {
		case <-collected:
			t.Fatal("an object a standing sample compares by identity was collected")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if sample.Digest() == "" {
		t.Fatalf("fixture: %s", sample.Unknown())
	}
	runtime.KeepAlive(sample)
	// Control: nothing else holds the object, so it goes once the sample does.
	sample = TypedInputSample{}
	for i := 0; i < 250; i++ {
		runtime.GC()
		select {
		case <-collected:
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("control failed: the object outlived its sample, so the first half proved nothing")
}

type digestPreparer struct{ prepared []*attachment.TraceMaterial }

func (*digestPreparer) Prepare(context.Context, string) (*attachment.TraceMaterial, error) {
	return nil, nil
}
func (p *digestPreparer) PreparedMaterials() []*attachment.TraceMaterial { return p.prepared }

// The trace preparer is a service too, but what it has prepared is read when
// a request is built, so its state is an input and stays walked by content.
func TestTracePreparerStateIsSampledByContent(t *testing.T) {
	preparer := &digestPreparer{}
	bus := &BusContext{Mutable: NewMutableState("question"), TraceInputPreparer: preparer}
	before := sampleOf(bus)
	preparer.prepared = append(preparer.prepared, nil)
	after := sampleOf(bus)
	if before.Digest() == "" || after.Digest() == "" || before.Same(after) {
		t.Fatalf("a material prepared behind the same preparer is a new input: %q %q", before.Unknown(), after.Unknown())
	}
}

func TestTypedInputHandleTableNamesRealFields(t *testing.T) {
	for typ, fields := range typedInputHandleFields {
		for name, reason := range fields {
			field, ok := typ.FieldByName(name)
			if !ok {
				t.Errorf("%s.%s is a handle field but does not exist; remove the entry or follow the rename", typ.Name(), name)
				continue
			}
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s.%s is compared by identity without a reason", typ.Name(), name)
			}
			if _, both := typedInputExcludedFields[typ][name]; both {
				t.Errorf("%s.%s is both excluded and a handle", typ.Name(), name)
			}
			if !holdsInterface(field.Type) {
				t.Errorf("%s.%s: the handle rule reads an interface or a map of them, not %s", typ.Name(), name, field.Type)
			}
		}
	}
	// The table is closed. Comparing a field by identity is a claim that the
	// object behind it cannot change an answer request while it stays in
	// place, so a new entry is made here, next to that claim's evidence.
	want := []string{
		"BusContext.Memory",
		"BusContext.MultiGraph",
		"BusContext.SearchGraph",
		"MutableState.scopedSearchGraphs",
		"MutableState.searchGraph",
		"MutableState.searchGraphDerived",
		"MutableState.symbolOracle",
	}
	var got []string
	for _, row := range typedInputFieldCensus(typedInputHandleFields) {
		name, _, _ := strings.Cut(row, ": ")
		got = append(got, name)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the handle table changed:\n got: %v\nwant: %v", got, want)
	}
}

func holdsInterface(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Pointer:
		return t.Elem().Kind() == reflect.Interface
	}
	return false
}

// Every field of the two sampled roots that can hold an object of any type is
// decided here. A service slot added later is walked by content until someone
// says otherwise; this list makes that a decision instead of an accident,
// because a service the walk cannot read leaves every sample unknown.
func TestEveryOpenSlotOfTheSampledRootsIsDecided(t *testing.T) {
	want := map[string]string{
		"BusContext.Ctx":         "no data",
		"BusContext.Memory":      "handle",
		"BusContext.MultiGraph":  "handle",
		"BusContext.SearchGraph": "handle",
		// What it has prepared is read when a request is built.
		"BusContext.TraceInputPreparer":         "content",
		"BusContext.groundingContextCacheValue": "excluded",
		// Typed data, carried as any for an import cycle.
		"MutableState.repairExecutionPlan": "content",
		"MutableState.scopedSearchGraphs":  "handle",
		"MutableState.searchGraph":         "handle",
		"MutableState.searchGraphDerived":  "handle",
		"MutableState.symbolOracle":        "handle",
	}
	got := map[string]string{}
	for _, typ := range []reflect.Type{reflect.TypeOf(BusContext{}), reflect.TypeOf(MutableState{})} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !holdsInterface(field.Type) {
				continue
			}
			rule := "content"
			if _, ok := typedInputExcludedFields[typ][field.Name]; ok {
				rule = "excluded"
			} else if _, ok := typedInputHandleFields[typ][field.Name]; ok {
				rule = "handle"
			} else if typedInputRuleFor(field.Type) == typedInputNoData {
				rule = "no data"
			}
			got[typ.Name()+"."+field.Name] = rule
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("open slots of the sampled roots:\n got: %v\nwant: %v", got, want)
	}
}

// selectorCensus scans the module's non-test Go files for selector
// expressions with one of the given names and fails for each one outside the
// allowed files. A package-qualified name (types.X) is a type reference, not
// a read, and is skipped. It returns the number of allowed occurrences.
func selectorCensus(t *testing.T, names, allowed map[string]bool, premise string) int {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	scanned, allowedHits := 0, 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			scanned++
			rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || !names[sel.Sel.Name] {
					return true
				}
				if pkg, qualified := sel.X.(*ast.Ident); qualified && pkg.Name == "types" {
					return true
				}
				if allowed[rel] {
					allowedHits++
					return true
				}
				t.Errorf("%s uses %s: %s", fset.Position(sel.Pos()), sel.Sel.Name, premise)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 100 {
		t.Fatalf("vacuous census: scanned %d files", scanned)
	}
	return allowedHits
}

// The scheduler's node ledgers are left out of the sample because no stage
// context reads them. This census keeps that true: a new reader outside the
// scheduler and the read-run snapshot must first take the two fields back
// into the sample.
func TestSchedulerNodeLedgerReadersStayOutsideStageContexts(t *testing.T) {
	hits := selectorCensus(t,
		map[string]bool{
			"NodeExecStatus": true, "NodeExecStatuses": true, "NodeExecAttempt": true, "NodeExecAttempts": true,
			"nodeExecStatus": true, "nodeExecAttempts": true,
		},
		map[string]bool{
			"internal/orchestrator/scheduler_status.go":    true,
			"internal/types/read_run_snapshot.go":          true,
			"internal/types/evidence_closure.go":           true,
			"internal/types/evidence_closure_node_exec.go": true,
		},
		"the scheduler's node ledger is left out of the answer-input sample on the premise that only the scheduler and the read-run snapshot read it")
	if hits < 8 {
		t.Fatalf("vacuous census: %d known reads", hits)
	}
}

// The refusal record is scheduler bookkeeping. Only the scheduler's refusal
// handling reads or writes it; no stage, context builder or tool does, so
// nothing of it can reach a model.
func TestRefusalRecordIsReadOnlyByTheScheduler(t *testing.T) {
	hits := selectorCensus(t,
		map[string]bool{
			"FinalizerRequestRefusal": true, "RecordFinalizeRequestRefusal": true, "RecordFinalizeDispatchNotRefused": true,
			"finalizeRequests": true,
		},
		map[string]bool{
			"internal/orchestrator/finalizer_request_refusal.go": true,
			"internal/types/finalizer_request_refusal.go":        true,
		},
		"the refusal record is read and written by the scheduler's refusal handling only")
	if hits < 6 {
		t.Fatalf("vacuous census: %d known uses", hits)
	}
}
