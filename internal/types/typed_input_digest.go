package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"reflect"
	"sort"
	"strings"
)

// typed_input_digest.go — a content digest of the typed state an answer
// request is built from. Its one question: would a request built now be built
// from the same inputs as a request built earlier in this run?
//
// The digest is structural. It walks every field reachable from the bus and
// its mutable state, so a carrier added later is covered without being listed
// anywhere; nothing here depends on a mutation being reported. What the walk
// leaves out is closed and small:
//
//   - clock readings (time.Time and the other struct types of package time):
//     two samples of one state must not differ by when they were taken;
//   - locks, contexts, OS handles, functions and channels: they hold no typed
//     data (a function or channel contributes only whether it is set);
//   - the fields named in typedInputExcludedFields, each with the reason it
//     cannot change what an answer request contains.
//
// The fields named in typedInputHandleFields hold a service or an index, not
// typed data. The object behind one is not entered: the digest records that
// it is there, and two samples agree on it only when it is the same object.
//
// Anything the walk cannot read faithfully (an unsafe pointer, a sync.Map, a
// state too large to walk) makes the sample unknown, and an unknown sample is
// the same as nothing, itself included. Every error is therefore on the side
// of rebuilding: an input the walk does not understand, or a volatile field
// nobody listed, makes two samples differ.
//
// The digest holds no address and no clock reading, and maps are walked in
// key-content order, so equal typed inputs give equal digests in any process.
// Object identity is compared between two samples of one run only and is no
// part of the digest.

// AnswerDispatchStamp carries the per-dispatch inputs the scheduler hands the
// answer stage outside the bus.
type AnswerDispatchStamp struct {
	// EmitRetryAttempt is the emit-retry counter the next dispatch consumes.
	EmitRetryAttempt int
}

// TypedInputSample is one sample of the typed state an answer request is
// built from.
type TypedInputSample struct {
	digest     string
	components map[string]string
	// handles holds, per carrier, the objects compared by identity. Holding
	// them keeps each one alive, so its address cannot pass to another object
	// while the sample exists.
	handles map[string][]reflect.Value
	unknown string
}

// Digest is the sample's content digest, or "" when the state could not be
// read faithfully.
func (s TypedInputSample) Digest() string { return s.digest }

// Unknown names what the walk could not represent, or "".
func (s TypedInputSample) Unknown() string { return s.unknown }

// Same reports whether two samples are the same inputs: equal content, and
// the same object behind every handle. An unknown sample is never the same as
// anything.
func (s TypedInputSample) Same(other TypedInputSample) bool {
	if s.digest == "" || s.digest != other.digest {
		return false
	}
	// Equal digests name the same carriers with the same number of handles.
	for name, held := range s.handles {
		if !sameTypedInputHandles(held, other.handles[name]) {
			return false
		}
	}
	return true
}

// Changed lists, sorted, the top-level carriers that differ between two
// samples, by content or by the object behind a handle. Both samples name the
// same carriers; an unknown sample names none, so it lists nothing. It is an
// operator diagnostic; Same is the decision.
func (s TypedInputSample) Changed(other TypedInputSample) []string {
	var names []string
	for name, digest := range s.components {
		if other.components[name] != digest || !sameTypedInputHandles(s.handles[name], other.handles[name]) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// sameTypedInputHandles reports whether two carriers hold the same objects in
// the same places. The digest already carries each object's type.
func sameTypedInputHandles(a, b []reflect.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Pointer() != b[i].Pointer() {
			return false
		}
	}
	return true
}

// typedInputNodeLimit bounds one sample. A state larger than this is reported
// unknown instead of stalling the run; the caller then rebuilds.
const typedInputNodeLimit = 1 << 25

// typedInputExcludedFields lists the struct fields a sample leaves out. An
// entry removes a field from the comparison, so each one states why the field
// cannot change what an answer request contains. Locks need no entry; the
// type rules cover them. The census test keeps every entry pointing at a
// field that exists, and pins who may read the scheduler ledgers.
var typedInputExcludedFields = map[reflect.Type]map[string]string{
	reflect.TypeOf(BusContext{}): {
		"Mutable":                    "sampled field by field as its own carriers",
		"TraceID":                    "the run's identity, built from the clock once at the start of the run",
		"answerSurfaceCacheKey":      "key of a cache derived from the sampled carriers",
		"answerSurfacePlanCached":    "state of a cache derived from the sampled carriers",
		"answerSemanticViewCached":   "state of a cache derived from the sampled carriers",
		"answerSupportPlanCached":    "state of a cache derived from the sampled carriers",
		"answerSurfacePlan":          "cache derived from the sampled carriers",
		"answerSemanticView":         "cache derived from the sampled carriers",
		"answerSupportPlan":          "cache derived from the sampled carriers",
		"groundingContextCacheKey":   "key of a cache derived from the sampled carriers",
		"groundingContextCacheValue": "cache derived from the sampled carriers",
	},
	reflect.TypeOf(TaskState{}): {
		"LastError": "cleared at the start of every dispatch, before the stage's context is built",
	},
	reflect.TypeOf(MutableState{}): {
		"finalizeRequests": "the refusal record itself, written after the sample it carries",
	},
	reflect.TypeOf(EvidenceClosure{}): {
		"nodeExecStatus":   "the scheduler's node status ledger; read by the scheduler and the read-run snapshot, never by a stage's context",
		"nodeExecAttempts": "the scheduler's node attempt ledger; read by the scheduler only",
	},
}

// typedInputHandleFields lists the struct fields that hold a service or an
// index instead of typed data. The object such a field points to is compared
// by identity, so each entry states why that object's own state cannot change
// an answer request while the object stays in place. A map is compared key by
// key, each value as a handle. A value that is not a pointer has no identity
// apart from its content and is walked like any other field.
var typedInputHandleFields = map[reflect.Type]map[string]string{
	reflect.TypeOf(BusContext{}): {
		"MultiGraph":  "the run's repository index, installed at run entry and immutable for the run by the field's contract; what it loads on demand is a cache of the repository on disk",
		"SearchGraph": "a read-only repository index handed to tool dispatches that do not receive the mutable state",
		"Memory":      "the read service of the memory store; what a stage recalls through it reaches a request as a tool result",
	},
	reflect.TypeOf(MutableState{}): {
		"searchGraph":        "the run's repository index; SetSearchGraph installs another one and advances searchGraphRevision, which is sampled, and the grounding context is already cached on that revision",
		"symbolOracle":       "the symbol index built from the repository index",
		"scopedSearchGraphs": "repository indexes kept per subdirectory for repo_map calls, whose output reaches a request as a tool result",
		"searchGraphDerived": "immutable indices derived from the repository index",
	},
}

// SampleAnswerRequestInputs samples the typed state the next answer dispatch
// would be built from.
func SampleAnswerRequestInputs(bus *BusContext, dispatch AnswerDispatchStamp) TypedInputSample {
	sample := TypedInputSample{components: map[string]string{}, handles: map[string][]reflect.Value{}}
	if bus == nil {
		sample.unknown = "no bus"
		return sample
	}
	budget := typedInputNodeLimit
	add := func(name string, walk func(*typedInputWalker)) {
		if sample.unknown != "" {
			return
		}
		w := &typedInputWalker{h: sha256.New(), seen: map[typedInputRef]int{}, budget: &budget}
		// The roots are sampled field by field; a field that points back at
		// one of them is a reference, not a second copy to walk.
		w.revisit(reflect.ValueOf(bus))
		if bus.Mutable != nil {
			w.revisit(reflect.ValueOf(bus.Mutable))
		}
		w.h.Reset()
		w.guarded(func() { walk(w) })
		if w.unknown != "" {
			sample.unknown = name + ": " + w.unknown
			return
		}
		sample.components[name] = hex.EncodeToString(w.h.Sum(nil))
		if len(w.handles) > 0 {
			sample.handles[name] = w.handles
		}
	}
	addFields := func(prefix string, v reflect.Value) {
		t := v.Type()
		excluded := typedInputExcludedFields[t]
		for i := 0; i < t.NumField(); i++ {
			if _, skip := excluded[t.Field(i).Name]; skip {
				continue
			}
			add(prefix+t.Field(i).Name, func(w *typedInputWalker) { w.field(t, i, v.Field(i)) })
		}
	}
	addFields("bus.", reflect.ValueOf(bus).Elem())
	if bus.Mutable != nil {
		bus.Mutable.mu.RLock()
		addFields("state.", reflect.ValueOf(bus.Mutable).Elem())
		bus.Mutable.mu.RUnlock()
	}
	add("dispatch", func(w *typedInputWalker) { w.walk(reflect.ValueOf(dispatch)) })
	if sample.unknown != "" {
		sample.components, sample.handles = nil, nil
		return sample
	}
	names := make([]string, 0, len(sample.components))
	for name := range sample.components {
		names = append(names, name)
	}
	sort.Strings(names)
	all := sha256.New()
	for _, name := range names {
		fmt.Fprintf(all, "%s=%s\n", name, sample.components[name])
	}
	sample.digest = hex.EncodeToString(all.Sum(nil))
	return sample
}

type typedInputRef struct {
	ptr uintptr
	typ reflect.Type
}

type typedInputWalker struct {
	h       hash.Hash
	seen    map[typedInputRef]int
	handles []reflect.Value
	budget  *int
	unknown string
	scratch [binary.MaxVarintLen64]byte
}

func (w *typedInputWalker) walkGuarded(v reflect.Value) {
	w.guarded(func() { w.walk(v) })
}

// guarded runs one walk and reports a value reflection rejects as unknown; it
// never ends the run.
func (w *typedInputWalker) guarded(walk func()) {
	defer func() {
		if r := recover(); r != nil && w.unknown == "" {
			w.unknown = fmt.Sprintf("unreadable value: %v", r)
		}
	}()
	walk()
}

func (w *typedInputWalker) tag(b byte) {
	w.scratch[0] = b
	w.h.Write(w.scratch[:1])
}

func (w *typedInputWalker) number(n uint64) {
	w.h.Write(w.scratch[:binary.PutUvarint(w.scratch[:], n)])
}

func (w *typedInputWalker) text(s string) {
	w.number(uint64(len(s)))
	w.h.Write([]byte(s))
}

// typedInputTypeRule classifies a type the walk does not enter.
type typedInputTypeRule int

const (
	typedInputWalk    typedInputTypeRule = iota
	typedInputNoData                     // holds no typed data: contributes its presence only
	typedInputUnknown                    // cannot be read faithfully: poisons the sample
)

func typedInputRuleFor(t reflect.Type) typedInputTypeRule {
	target := t
	if t.Kind() == reflect.Pointer {
		target = t.Elem()
	}
	switch target.PkgPath() {
	case "time":
		// time.Duration is a plain quantity and is walked as a number; the
		// struct types are clock readings and timers.
		if target.Kind() == reflect.Struct {
			return typedInputNoData
		}
	case "context", "os":
		return typedInputNoData
	case "sync":
		switch target.Name() {
		case "Mutex", "RWMutex", "Once", "WaitGroup", "Cond":
			return typedInputNoData
		}
		if target.Kind() == reflect.Struct && target.NumField() > 0 {
			return typedInputUnknown
		}
	case "reflect", "unsafe":
		return typedInputUnknown
	}
	return typedInputWalk
}

func (w *typedInputWalker) walk(v reflect.Value) {
	if w.unknown != "" {
		return
	}
	if *w.budget--; *w.budget < 0 {
		w.unknown = "state too large to sample"
		return
	}
	t := v.Type()
	switch typedInputRuleFor(t) {
	case typedInputNoData:
		w.tag('-')
		return
	case typedInputUnknown:
		w.unknown = "unsupported type " + t.String()
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		w.tag('b')
		if v.Bool() {
			w.number(1)
		} else {
			w.number(0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.tag('i')
		w.number(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.tag('u')
		w.number(v.Uint())
	case reflect.Float32, reflect.Float64:
		w.tag('f')
		w.number(math.Float64bits(v.Float()))
	case reflect.Complex64, reflect.Complex128:
		w.tag('c')
		w.number(math.Float64bits(real(v.Complex())))
		w.number(math.Float64bits(imag(v.Complex())))
	case reflect.String:
		w.tag('s')
		w.text(v.String())
	case reflect.Slice:
		if v.IsNil() {
			w.tag('0')
			return
		}
		w.tag('[')
		w.number(uint64(v.Len()))
		if t.Elem().Kind() == reflect.Uint8 {
			w.h.Write(v.Bytes())
			return
		}
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	case reflect.Array:
		w.tag('a')
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() {
			w.tag('0')
			return
		}
		if w.revisit(v) {
			return
		}
		w.walkMap(v, w.walk)
	case reflect.Pointer:
		if v.IsNil() {
			w.tag('0')
			return
		}
		if w.revisit(v) {
			return
		}
		w.tag('*')
		w.walk(v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			w.tag('0')
			return
		}
		elem := v.Elem()
		w.tag('I')
		w.text(elem.Type().String())
		w.walk(elem)
	case reflect.Struct:
		w.tag('{')
		for i := 0; i < t.NumField(); i++ {
			w.field(t, i, v.Field(i))
		}
		w.tag('}')
	case reflect.Func, reflect.Chan:
		w.tag('F')
		if v.IsNil() {
			w.number(0)
		} else {
			w.number(1)
		}
	default:
		// An unsafe pointer, or a kind added to the language later.
		if v.Kind() == reflect.UnsafePointer && v.IsNil() {
			w.tag('0')
			return
		}
		w.unknown = "unsupported kind " + v.Kind().String()
	}
}

// field walks one struct field under the rule its owner declares for it.
func (w *typedInputWalker) field(owner reflect.Type, i int, v reflect.Value) {
	name := owner.Field(i).Name
	if _, excluded := typedInputExcludedFields[owner][name]; excluded {
		return
	}
	if _, held := typedInputHandleFields[owner][name]; held {
		w.handle(v)
		return
	}
	w.walk(v)
}

// handle walks a field declared in typedInputHandleFields. The object it
// points to is written as present and kept for the identity comparison; it is
// not entered.
func (w *typedInputWalker) handle(v reflect.Value) {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			w.tag('0')
			return
		}
		w.tag('I')
		w.text(v.Elem().Type().String())
		w.handle(v.Elem())
	case reflect.Pointer:
		if v.IsNil() {
			w.tag('0')
			return
		}
		w.tag('H')
		w.handles = append(w.handles, v)
	case reflect.Map:
		if v.IsNil() {
			w.tag('0')
			return
		}
		w.walkMap(v, w.handle)
	default:
		w.walk(v)
	}
}

// revisit writes a back-reference when a pointer or map was already walked in
// this sample. The reference is the order of first visit, never the address,
// and it also ends cycles.
func (w *typedInputWalker) revisit(v reflect.Value) bool {
	ref := typedInputRef{ptr: v.Pointer(), typ: v.Type()}
	if index, ok := w.seen[ref]; ok {
		w.tag('^')
		w.number(uint64(index))
		return true
	}
	w.seen[ref] = len(w.seen)
	return false
}

// walkMap walks a map in the order of its keys' content, so the digest does
// not depend on iteration order or on where a key lives in memory. Each value
// is walked by the given rule.
func (w *typedInputWalker) walkMap(v reflect.Value, value func(reflect.Value)) {
	type entry struct {
		key   []byte
		value reflect.Value
	}
	entries := make([]entry, 0, v.Len())
	for iter := v.MapRange(); iter.Next(); {
		keyWalker := &typedInputWalker{h: sha256.New(), seen: map[typedInputRef]int{}, budget: w.budget}
		keyWalker.walk(iter.Key())
		if keyWalker.unknown != "" {
			w.unknown = keyWalker.unknown
			return
		}
		entries = append(entries, entry{key: keyWalker.h.Sum(nil), value: iter.Value()})
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i].key, entries[j].key) < 0 })
	w.tag('m')
	w.number(uint64(len(entries)))
	for i, e := range entries {
		if i > 0 && bytes.Equal(entries[i-1].key, e.key) {
			// Two keys with equal content (distinct pointers): no order of
			// their values is canonical.
			w.unknown = "map " + v.Type().String() + " has keys with equal content"
			return
		}
		w.h.Write(e.key)
		value(e.value)
	}
}

// typedInputFieldCensus renders one of the two field tables for the report
// and for the census tests: "<type>.<field>: <reason>", sorted.
func typedInputFieldCensus(table map[reflect.Type]map[string]string) []string {
	var rows []string
	for t, fields := range table {
		for name, reason := range fields {
			rows = append(rows, t.Name()+"."+name+": "+strings.TrimSpace(reason))
		}
	}
	sort.Strings(rows)
	return rows
}
