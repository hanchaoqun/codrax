package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The two contention payload forms of the fixture: one names its owner thread,
// the other gives only an owner tid, which names a thread only when the capture
// has a row of that thread.
const (
	hmc378NamedOwnerPayload = "monitor contention with owner owner (4242) at void com.example.Store.list()(Store.java:12) waiters=1 blocking from void com.example.Store.read()(Store.java:34)"
	hmc378OwnerTidPayload   = "Lock contention on a monitor lock (owner tid: 4242)"
)

// hmc378LockCapture: app-100 blocks in a span carrying the contention payload.
// With counterpart it is woken by worker-200 and thread 4242 has a scheduler
// row of its own in the capture; without, the capture records neither a waker
// nor the owner.
func hmc378LockCapture(t *testing.T, payload string, counterpart bool) string {
	t.Helper()
	var c hmc376Trace
	c.run(4.990000, "app", 100, 100, 1)
	c.add(5.000000, "app", 100, 100, 1, "tracing_mark_write: B|100|%s", payload)
	c.sleep(5.000100, "app", 100, 100, 1)
	if counterpart {
		c.run(5.000200, "worker", 200, 100, 2)
		c.add(5.112000, "worker", 200, 100, 2, "sched_wakeup: comm=app pid=100 prio=120 target_cpu=001")
	}
	c.run(5.112223, "app", 100, 100, 1)
	c.add(5.112300, "app", 100, 100, 1, "tracing_mark_write: E|100")
	c.sleep(5.114000, "app", 100, 100, 1)
	if counterpart {
		c.run(5.200000, "owner", 4242, 100, 3)
		c.sleep(5.210000, "owner", 4242, 100, 3)
	}
	return c.write(t)
}

func hmc378NoteValue(record types.ObservationRecord, key string) string {
	for _, note := range record.RichNotes {
		if value, ok := strings.CutPrefix(strings.TrimSpace(note), key+"="); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hmc378BlockingWord(record types.ObservationRecord, zh bool) string {
	return TraceBlockingRowDisplayLabel(hmc378NoteValue(record, types.TraceNoteKeyBlockingKind),
		hmc378NoteValue(record, types.TraceNoteKeyBlockingValueBasis),
		hmc378NoteValue(record, types.TraceNoteKeySubjectIsLockHolder) == "true", zh)
}

// hmc378LockRankRecordAndMirror returns the typed lock rank record of a result
// and the evidence record that mirrors it.
func hmc378LockRankRecordAndMirror(t *testing.T, result types.ToolResult) (rank, mirror types.ObservationRecord) {
	t.Helper()
	found := 0
	for _, record := range result.Observations {
		if record.Object != "blocking_span" || !strings.HasPrefix(record.Predicate, "root_cause_") || record.ClaimKey != record.Predicate {
			continue
		}
		rank = record
		found++
	}
	if found != 1 {
		t.Fatalf("fixture: want one lock rank record, got %d", found)
	}
	found = 0
	for _, record := range result.Observations {
		if strings.HasPrefix(record.ClaimKey, "evidence_fact:") && record.Subject == rank.Subject && record.Object == "blocking_span" {
			mirror = record
			found++
		}
	}
	if found != 1 {
		t.Fatalf("fixture: want one evidence record mirroring the lock rank record, got %d", found)
	}
	return rank, mirror
}

// The evidence record mirroring a typed lock rank row carries that row's
// blocking kind and holder role, so both are worded the same way. No other
// mirror gains a note, and no projection lane reads the mirror's notes.
func TestHMC378EvidenceFactTwinCarriesBlockingKind(t *testing.T) {
	for _, c := range []struct {
		name        string
		payload     string
		counterpart bool
		holder      bool
		zh, en      string
	}{
		{"resolved-holder", hmc378NamedOwnerPayload, true, true, "持锁阻塞", "lock-holder blocking"},
		{"unresolved-owner", hmc378OwnerTidPayload, false, false, "锁竞争等待", "lock contention wait"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := hmc378LockCapture(t, c.payload, c.counterpart)
			hmc176DisableExternalProviders(t)
			ctx, _, _ := hmc17NamedPathContext(t)
			result := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "target_scope": "thread", "time_start": 4.99, "time_end": 5.115})
			if !result.Success {
				t.Fatalf("query failed: %s", result.Summary)
			}
			rank, mirror := hmc378LockRankRecordAndMirror(t, result)
			kind, holder := hmc378NoteValue(rank, types.TraceNoteKeyBlockingKind), hmc378NoteValue(rank, types.TraceNoteKeySubjectIsLockHolder)
			if kind == "" {
				t.Fatalf("fixture: the lock rank record carries no kind: %v", rank.RichNotes)
			}
			if (holder == "true") != c.holder {
				t.Fatalf("fixture: want the lock rank record's subject to be the holder=%v: %s %v", c.holder, rank.Subject, rank.RichNotes)
			}
			want := []string{types.TraceNoteKeyBlockingKind + "=" + kind}
			if holder == "true" {
				want = append(want, types.TraceNoteKeySubjectIsLockHolder+"=true")
			}
			if !reflect.DeepEqual(mirror.RichNotes, want) {
				t.Fatalf("the mirror of the lock rank record carries %v, want %v", mirror.RichNotes, want)
			}
			for _, zh := range []bool{true, false} {
				rankWord, mirrorWord := hmc378BlockingWord(rank, zh), hmc378BlockingWord(mirror, zh)
				if rankWord != mirrorWord {
					t.Fatalf("the mirror says %q where its rank row says %q", mirrorWord, rankWord)
				}
				if expect := map[bool]string{true: c.zh, false: c.en}[zh]; mirrorWord != expect {
					t.Fatalf("the mirror says %q, want %q", mirrorWord, expect)
				}
			}
			for _, record := range result.Observations {
				if !strings.HasPrefix(record.ClaimKey, "evidence_fact:") || record.ID == mirror.ID {
					continue
				}
				if len(record.RichNotes) != 0 {
					t.Fatalf("a mirror of a row without a blocking kind gained notes: %s %v", record.ClaimKey, record.RichNotes)
				}
			}

			// The same ledger with the mirror's notes removed compiles to the
			// same projection and the same reader face.
			bare := result
			bare.Observations = append([]types.ObservationRecord(nil), result.Observations...)
			for i := range bare.Observations {
				if bare.Observations[i].ID == mirror.ID {
					bare.Observations[i].RichNotes = nil
				}
			}
			compile := func(r types.ToolResult) string {
				bus := newBusForMutationTest()
				bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
				bus.ToolResults = []types.ToolResult{r}
				set := types.CompileTraceCausalProjectionSet(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit)))
				raw, err := json.Marshal(set)
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			}
			if !strings.Contains(compile(result), "blocking_span") {
				t.Fatalf("fixture: the lock row is not on the projection: %s", compile(result))
			}
			if compile(result) != compile(bare) {
				t.Fatalf("a projection lane read the notes of the evidence mirror")
			}
			if hmc378ReaderFace(t, result) != hmc378ReaderFace(t, bare) {
				t.Fatalf("the reader face read the notes of the evidence mirror")
			}
		})
	}
}

// The same on a complete original capture (Donghu baseline 100 / Douyin Lite /
// cold start after a container restart, capture 1): the main-thread window
// whose first rank row is a resolved lock holder. The capture is read from
// CODRAX_HMC376_GALLERY_FULLGC and is never copied into the repo.
func TestHMC378WholeOriginalLockHolderTwinWord(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_GALLERY_FULLGC",
		"d27ff641787dfa0768928e0ce3d11a7d2a6caa0f2910c941fae3e97f2256e22e", 245420208)
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 56984, "target_scope": "thread", "time_start": 2498.683045, "time_end": 2498.883045})
	if !result.Success {
		t.Fatalf("query failed: %s", result.Summary)
	}
	mirrors := map[string]types.ObservationRecord{}
	for _, record := range result.Observations {
		if _, position, ok := strings.Cut(record.ID, "#evidence_fact:"); ok {
			mirrors[position] = record
		}
	}
	holders := 0
	for _, rank := range result.Observations {
		_, position, ok := strings.Cut(rank.ID, "#root_cause_rank:")
		kind := hmc378NoteValue(rank, types.TraceNoteKeyBlockingKind)
		if !ok || rank.Object != "blocking_span" || kind == "" {
			continue
		}
		mirror, ok := mirrors[position]
		if !ok {
			continue // beyond the evidence pack
		}
		if mirror.Subject != rank.Subject || mirror.Object != rank.Object {
			t.Fatalf("evidence record %s does not mirror rank record %s", mirror.ID, rank.ID)
		}
		for _, zh := range []bool{true, false} {
			if rankWord, mirrorWord := hmc378BlockingWord(rank, zh), hmc378BlockingWord(mirror, zh); rankWord != mirrorWord {
				t.Fatalf("the mirror of %s says %q where its rank row says %q", rank.Subject, mirrorWord, rankWord)
			}
		}
		if hmc378NoteValue(rank, types.TraceNoteKeySubjectIsLockHolder) == "true" {
			holders++
			if word := hmc378BlockingWord(mirror, true); word != "持锁阻塞" {
				t.Fatalf("the mirror of the holder %s says %q", rank.Subject, word)
			}
		}
	}
	if holders == 0 {
		t.Fatalf("the window no longer ranks a resolved lock holder inside the evidence pack")
	}
}
