package tool

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Whole-original public-entry regressions for HMC377 E3 (business clues on the
// chain). Opt-in per capture through an environment variable; digest and size
// are checked before anything is parsed. Every expected number comes from the
// independent raw oracle (hmc377/design/business-clue-seats/clue_oracle.py:
// standalone Python over the raw ftrace, integer microseconds), never from
// this engine.

type hmc377ClueRow struct {
	tid                int
	namePrefix         string
	total, wall, self  float64 // self < 0: no own-phase account expected
	basis              string
	sleep, running     float64 // own phase scheduler split (checked when > 0)
	d, io, runnable    float64
	checkSchedulerPart bool
}

type hmc377ClueCase struct {
	name, env, digest string
	size              int64
	tid               int
	start, end        float64
	want              []hmc377ClueRow
	absentTIDs        []int // threads whose only link is temporal proximity
	wantRows          int   // exact row count (0 = not pinned)
}

func hmc377ClueCases() []hmc377ClueCase {
	return []hmc377ClueCase{
		{
			// Device-reboot cold start, bindApplication of the main thread. Before
			// HMC377 E3 seven 480-528ms spans of s.location:core-10879 (on the chain
			// for one 1.648ms window) took seven of the eight seats.
			name: "douyin reboot cold start", env: "CODRAX_HMC377_DOUYIN_REBOOT",
			digest: "8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e", size: 138438125,
			tid: 11105, start: 140.911304, end: 141.835458,
			want: []hmc377ClueRow{
				{tid: 11105, namePrefix: "bindApplication", total: 924.154, wall: 924.154, self: 263.748, basis: "self",
					running: 584.023, runnable: 24.510, sleep: 273.049, d: 24.612, io: 17.960, checkSchedulerPart: true},
				{tid: 11105, namePrefix: "makeApplication", total: 305.047, wall: 305.047, self: 1.226, basis: "self"},
				{tid: 11105, namePrefix: "attachBaseContext", total: 303.032, wall: 303.032, self: 250.480, basis: "self"},
				{tid: 11105, namePrefix: "zip:/data/app/", total: 180.996, wall: 180.996, self: 0.699, basis: "self"},
				{tid: 11105, namePrefix: "monitor contention with owner #tp-io-18 (2220) at void java.lang.Runtime.loadLibrary0", total: 50.246, wall: 50.246, self: 50.246, basis: "self"},
				{tid: 11105, namePrefix: "LZ4 decompress block", total: 103.152, wall: 103.152, self: 103.152, basis: "self"},
				// The thread the main thread waited for was itself waiting on
				// the native-library lock: its part inside the 50.184ms wait.
				{tid: 11293, namePrefix: "monitor contention with owner #tp-default-11 (2211) at void java.lang.Runtime.loadLibrary0", total: 23.023, wall: 23.023, self: -1, basis: "chain_member"},
			},
			// s.location:core (chain member for 1.648ms) and RenderThread (its
			// wakeups ended 0.008ms of the target's waits) stay off the face.
			absentTIDs: []int{10879, 11115},
			wantRows:   15,
		},
		{
			// Protected batch-376 window: the delivered report showed exactly these.
			name: "honeybird cold start (protected window)", env: "CODRAX_HMC365_HONEYBIRD",
			digest: "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e", size: 106896120,
			tid: 64156, start: 11343.248, end: 11343.310,
			want: []hmc377ClueRow{
				{tid: 64156, namePrefix: "ZygoteInit", total: 6.464, wall: 6.464, self: 2.596, basis: "self"},
				{tid: 64156, namePrefix: "PostFork", total: 2.047, wall: 2.047, self: 1.871, basis: "self"},
				{tid: 64176, namePrefix: "AttachCurrentThread", total: 1.165, wall: 1.165, self: -1, basis: "chain_member"},
				{tid: 64176, namePrefix: "Thread::Attach", total: 1.089, wall: 1.089, self: -1, basis: "chain_member"},
			},
			wantRows: 4,
		},
		{
			// Protected batch-376 window: the collector the main thread waited for
			// keeps its phases, counted inside the 123.003ms wait it ended.
			name: "netease reboot frame (protected window)", env: "CODRAX_HMC376_NETEASE_REBOOT_FRAME",
			digest: "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc", size: 104144939,
			tid: 11077, start: 153.763595, end: 153.887340,
			want: []hmc377ClueRow{
				{tid: 11077, namePrefix: "Choreographer#doFrame 2910", total: 123.745, wall: 123.745, self: 0.516, basis: "self"},
				{tid: 11098, namePrefix: "Background concurrent copying GC", total: 122.897, wall: 123.314, self: -1, basis: "chain_member"},
				{tid: 11098, namePrefix: "ReclaimPhase", total: 65.382, wall: 65.382, self: -1, basis: "chain_member"},
				{tid: 11098, namePrefix: "CopyingPhase", total: 56.828, wall: 57.245, self: -1, basis: "chain_member"},
			},
		},
	}
}

func TestHMC377BusinessCluesKeepOwnPhasesAndOnlyOnChainDependencyParts(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.0015 }
	for _, c := range hmc377ClueCases() {
		t.Run(c.name, func(t *testing.T) {
			path := os.Getenv(c.env)
			if path == "" {
				t.Skipf("requires %s (the pinned complete capture)", c.env)
			}
			if h, n := hmc176bRepresentativeDigest(t, path); h != c.digest || n != c.size {
				t.Fatalf("%s is not the pinned capture", c.env)
			}
			hmc176DisableExternalProviders(t)
			ctx, _, _ := hmc17NamedPathContext(t)
			r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": c.tid, "target_scope": "thread", "time_start": c.start, "time_end": c.end})
			value := hmc367NativeResult(t, r, "root_cause_rank")
			if value.RootCauseRank == nil || value.RootCauseRank.BusinessSpanMentions == nil {
				t.Fatal("no business-clue face")
			}
			fams := value.RootCauseRank.BusinessSpanMentions.Families
			if c.wantRows > 0 && len(fams) != c.wantRows {
				t.Errorf("want %d rows, got %d", c.wantRows, len(fams))
			}
			seenDependency := false
			for _, f := range fams {
				own := f.OnChainBasis == tracequery.BusinessSpanMentionBasisSelf
				if own && seenDependency {
					t.Errorf("an own phase reads after a dependency thread: %s", f.Name)
				}
				seenDependency = seenDependency || !own
				for _, tid := range c.absentTIDs {
					if f.Thread.PID == tid {
						t.Errorf("thread %d is linked only by temporal proximity but publishes %q %.3fms", tid, f.Name, f.TotalMs)
					}
				}
				if f.SpanWallMs+0.0005 < f.TotalMs {
					t.Errorf("counted part above the raw wall: %+v", f)
				}
			}
			find := func(want hmc377ClueRow) *tracequery.BusinessSpanMention {
				for i := range fams {
					if fams[i].Thread.PID == want.tid && strings.HasPrefix(fams[i].Name, want.namePrefix) {
						return &fams[i]
					}
				}
				return nil
			}
			for _, want := range c.want {
				got := find(want)
				if got == nil {
					t.Errorf("missing business clue %d %q", want.tid, want.namePrefix)
					continue
				}
				if got.OnChainBasis != want.basis || !near(got.TotalMs, want.total) || !near(got.SpanWallMs, want.wall) {
					t.Errorf("%q: basis=%s total=%.3f wall=%.3f, oracle basis=%s total=%.3f wall=%.3f", want.namePrefix, got.OnChainBasis, got.TotalMs, got.SpanWallMs, want.basis, want.total, want.wall)
				}
				if want.self >= 0 && (got.Phase == nil || !near(got.Phase.SelfMs, want.self)) {
					t.Errorf("%q: own time %+v, oracle %.3f", want.namePrefix, got.Phase, want.self)
				}
				if want.self < 0 && got.Phase != nil {
					t.Errorf("%q: a dependency thread carries no own-phase account: %+v", want.namePrefix, got.Phase)
				}
				if want.checkSchedulerPart {
					st := got.Phase.States
					if st == nil || !near(st.RunningMs, want.running) || !near(st.RunnableMs, want.runnable) || !near(st.SleepMs, want.sleep) || !near(st.DStateMs, want.d) || !near(st.IOWaitMs, want.io) {
						t.Errorf("%q: scheduler split %+v disagrees with the raw oracle", want.namePrefix, st)
					}
				}
			}
			// The same facts reach the projection the answer is written from.
			projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: r.Observations})
			if len(projection.BusinessSpanMentions) != len(fams) {
				t.Fatalf("projection carries %d of %d business clues", len(projection.BusinessSpanMentions), len(fams))
			}
			for i, m := range projection.BusinessSpanMentions {
				f := fams[i]
				if m.Name != f.Name || (f.Phase != nil) != m.SelfKnown || !near(m.WallMS, f.SpanWallMs) {
					t.Errorf("projection row %d lost the engine's account: %+v vs %+v", i, m, f)
				}
				if f.Phase != nil && f.Phase.States != nil && m.States == nil {
					t.Errorf("projection row %d lost the scheduler split: %+v", i, m)
				}
			}
		})
	}
}
