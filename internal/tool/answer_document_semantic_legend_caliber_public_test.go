package tool

import (
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Start with the live eval's physical trace: the worker's 5ms semantic wall
// clock crosses its 5.005000 wakeup; 4.6ms precedes that edge and 0.4ms follows.
// The target's own running account is only 1.2ms. None of these are aliases.
func semanticLegendPublicWorkerTrace(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_frame_semantic_span_optimization.case")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(data), "HTRACE='")
	trace, _, closed := strings.Cut(rest, "\n'\n")
	if !ok || !closed || !strings.Contains(trace, "5.005400") {
		t.Fatal("the live eval must expose its original native trace")
	}
	return trace + "\n"
}

func semanticLegendAssertReader(t *testing.T, text, lang string, generic, self, mention bool) {
	t.Helper()
	old := "is measured in the target thread's in-window running time"
	genericNeedles := []string{"Semantic-span wall clock belongs to its host thread and recorded range", "not interchangeable with target running time", "does not create a wakeup edge"}
	selfNeedles := []string{"Only rows marked `target-self·deterministic-optimization`", "target's own semantic-span wall clock", "in-window projection union"}
	mentionNeedles := []string{"`optimization point · below the top-N root-cause board`", "preserves on-chain semantic clues", "does not grant target-self running-time caliber or a ranking position"}
	if lang == "zh" {
		old = "按目标线程窗内运行时间计"
		genericNeedles = []string{"语义片段的墙钟时长属于片段所在的线程及记录范围", "不能替换为目标线程运行时间", "不据此补造唤醒边"}
		selfNeedles = []string{"仅带`目标自身·确定性优化`凭证的项目", "目标自身语义片段墙钟", "窗内投影并集"}
		mentionNeedles = []string{"`优化点·未入根因排序前N`", "保留链上语义线索", "不授予目标自身运行时间口径或根因榜位"}
	}
	if strings.Contains(text, old) {
		t.Errorf("generic semantic legend incorrectly grants target-running caliber: %q", old)
	}
	for _, group := range []struct {
		want bool
		pins []string
	}{{generic, genericNeedles}, {self, selfNeedles}, {mention, mentionNeedles}} {
		for _, pin := range group.pins {
			if strings.Contains(text, pin) != group.want {
				t.Errorf("reader definition %q presence=%t, want=%t", pin, strings.Contains(text, pin), group.want)
			}
		}
	}
}

func TestSemanticLegendCaliberPublicNativeQueryEmitPatch(t *testing.T) {
	worker := semanticLegendPublicWorkerTrace(t)
	mixed := worker
	// Insert a separate native target-self span while app is running. It must
	// not lend its self credential to the worker's independently measured span.
	lines := strings.Split(mixed, "\n")
	for i, line := range lines {
		if strings.Contains(line, "5.007000: sched_switch") {
			lines = append(lines[:i], append([]string{
				"app-100 (100) [001] .... 5.005900: tracing_mark_write: B|100|VerifyClass Target",
				"app-100 (100) [001] .... 5.006900: tracing_mark_write: E|100",
			}, lines[i:]...)...)
			break
		}
	}
	mixed = strings.Join(lines, "\n")
	var selfLines []string
	for _, line := range strings.Split(mixed, "\n") {
		if strings.Contains(line, "tracing_mark_write:") && strings.Contains(line, "worker-200") {
			continue
		}
		selfLines = append(selfLines, line)
	}
	self := strings.Join(selfLines, "\n")
	selfSleep := strings.Replace(self, "app-100 (100) [001] .... 5.006900:", `app-100 (100) [001] .... 5.006200: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
worker-200 (200) [002] .... 5.006600: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
app-100 (100) [001] .... 5.006800: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
app-100 (100) [001] .... 5.006900:`, 1)
	if selfSleep == self || !strings.Contains(selfSleep, "5.006200: sched_switch:") || !strings.Contains(selfSleep, "5.006600: sched_wakeup:") || !strings.Contains(selfSleep, "5.006800: sched_switch:") {
		t.Fatal("native target-self sleep excursion was not inserted")
	}
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name, trace  string
			worker, self bool
			selfMS       float64
		}{
			{"worker", worker, true, false, 0},
			{"self", self, false, true, 1},
			{"self_sleep", selfSleep, false, true, 1},
			{"mixed", mixed, true, true, 1},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx, path := optimizationCaliberPublicQuery(t, tc.trace, 100, 5, 5.007)
				optimizationCaliberPublicRequest(ctx, path, lang, "app-100", 100, 5, 5.007, false)
				system, set := optimizationCaliberPublicPublish(t, ctx, path, lang, false)
				if len(set.Projections) != 1 {
					t.Fatalf("native window must produce one projection, got %d", len(set.Projections))
				}
				p := set.Projections[0]
				if p.WindowStartTs != 5 || p.WindowEndTs != 5.007 {
					t.Fatal("native selected window changed")
				}
				foundWorker, foundSelf := false, false
				var ranks []string
				for _, n := range p.RankedSeats {
					ranks = append(ranks, n.Subject+"/"+n.Object)
					if n.Subject == "worker-200" && n.SemanticClass == "class_verification" {
						foundWorker = true
						if n.Rank != 1 || math.Abs(n.EffectiveImpactMS-4.6) > 1e-6 || n.OnChainBasis != types.TraceCausalOnChainBasisSemanticChainIntervalRelation {
							t.Errorf("worker's rank/pre-edge value/credential changed: %+v", n)
						}
					}
					if n.Subject == "app-100" && n.OnChainBasis == "self_deterministic_span" {
						foundSelf = true
						t.Logf("native target-self rank=%d value=%.3f basis=%s", n.Rank, n.EffectiveImpactMS, n.OnChainBasis)
						if math.Abs(n.EffectiveImpactMS-tc.selfMS) > 1e-6 {
							t.Errorf("target-self native value=%.3f, want %.3f", n.EffectiveImpactMS, tc.selfMS)
						}
					}
				}
				if foundWorker != tc.worker || foundSelf != tc.self {
					t.Fatalf("fixture failed to exercise native worker/self credentials: %t/%t; ranks=%+v", foundWorker, foundSelf, p.RankedSeats)
				}
				if tc.name == "self_sleep" && (!strings.Contains(system, "1.000ms") || !strings.Contains(system, "0.600ms") || !strings.Contains(system, "5.400ms")) {
					t.Error("self span must retain 1ms wall clock while target running is 0.6ms and sleep is 5.4ms")
				}
				if tc.name == "worker" && !reflect.DeepEqual(ranks, []string{"worker-200/class_verification", "app-100/runnable_wait"}) {
					t.Errorf("existing native ordering changed: %v", ranks)
				}
				if tc.worker {
					for _, value := range []string{"5.000ms", "4.600ms", "0.800ms", "1.200ms", "VerifyClass com.example.Foo"} {
						if !strings.Contains(system, value) {
							t.Errorf("native worker/target value or identity lost: %s", value)
						}
					}
					if !reflect.DeepEqual(p.WakeupPath, []string{"worker-200", "app-100"}) {
						t.Errorf("native cross-thread wakeup path changed: %v", p.WakeupPath)
					}
				}
				model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
				_ = runtimeTraceProjTreeFence(model, lang == "zh")
				if !model.Marks.has(runtimeTraceProjMarkSemanticSpan) || model.Marks.has(runtimeTraceProjMarkSelfDeterministicBasis) != tc.self || model.Marks.has(runtimeTraceProjMarkSemanticMentionFloor) {
					t.Fatal("native rendered marks do not exercise the intended ordinary/self branches")
				}
				reader := strings.Join(runtimeTraceProjReaderLegendLines(model.Marks, lang == "zh", false), "\n")
				semanticLegendAssertReader(t, reader, lang, true, tc.self, false)
				semanticLegendAssertReader(t, system, lang, true, tc.self, false)
				if tc.self {
					full := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, lang == "zh"), "\n")
					old, want := "inside the target thread's own running segments", "target thread's own semantic-span wall clock"
					if lang == "zh" {
						old, want = "目标线程自身运行段内的确定性语义工作", "目标线程自身确定性语义片段的墙钟"
					}
					if strings.Contains(full, old) || !strings.Contains(full, want) {
						t.Errorf("full self legend must describe native span extent, not a running intersection: old=%t new=%t", strings.Contains(full, old), strings.Contains(full, want))
					}
				}
			})
		}
	}
}

func TestSemanticLegendCaliberPublicNativeMentionOnly(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, semanticLegendPublicWorkerTrace(t))
			result := businessRefTestQuery(t, ctx, map[string]any{
				"source": "path", "path": path, "view": "wakeup_chain", "pid": 100,
				"time_start": 5.0, "time_end": 5.007, "include_window_stats": true,
				"max_depth": 8, "limit": 32, "trace_flavor": "harmony_hitrace",
			})
			if !result.Success || len(result.Observations) == 0 {
				t.Fatalf("native mention query failed: %s", result.Summary)
			}
			optimizationCaliberPublicRequest(ctx, path, lang, "app-100", 100, 5, 5.007, false)
			system, set := optimizationCaliberPublicPublish(t, ctx, path, lang, false)
			if len(set.Projections) != 1 || len(set.Projections[0].RankedSeats) != 0 {
				t.Fatalf("wakeup-only native query must not fabricate a rank board: %+v", set.Projections)
			}
			model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
			fence := runtimeTraceProjTreeFence(model, lang == "zh")
			if !model.Marks.has(runtimeTraceProjMarkSemanticSpan) || !model.Marks.has(runtimeTraceProjMarkSemanticMentionFloor) || model.Marks.has(runtimeTraceProjMarkSelfDeterministicBasis) {
				t.Fatalf("native wakeup-only fixture must emit semantic mention without a self credential: %s", fence)
			}
			if !strings.Contains(system, "VerifyClass com.example.Foo") || !strings.Contains(system, "5.000ms") || !strings.Contains(system, "4.600ms") {
				t.Error("mention-only publication lost semantic identity or native measurement")
			}
			reader := strings.Join(runtimeTraceProjReaderLegendLines(model.Marks, lang == "zh", false), "\n")
			semanticLegendAssertReader(t, reader, lang, true, false, true)
			semanticLegendAssertReader(t, system, lang, true, false, true)
		})
	}
}

// These are explicit renderer-mark boundary fixtures, NOT native trace or
// engine evidence. Native self and mixed credentials are exercised above;
// this matrix isolates mention-only/empty combinations and never grants an
// omitted row a new measurement, rank, relationship or self credential.
func TestSemanticLegendCaliberTypedMarkBoundaries(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for bits := 0; bits < 8; bits++ {
			t.Run(lang+"/"+string(rune('0'+bits)), func(t *testing.T) {
				marks := &runtimeTraceProjMarkSet{}
				for i, mark := range []runtimeTraceProjMark{runtimeTraceProjMarkSemanticSpan, runtimeTraceProjMarkSelfDeterministicBasis, runtimeTraceProjMarkSemanticMentionFloor} {
					if bits&(1<<i) != 0 {
						marks.mark(mark)
					}
				}
				before := *marks
				reader := strings.Join(runtimeTraceProjReaderLegendLines(marks, lang == "zh", false), "\n")
				semanticLegendAssertReader(t, reader, lang, bits&1 != 0, bits&2 != 0, bits&4 != 0)
				if before != *marks {
					t.Error("reading legend changed emission marks")
				}
			})
		}
	}
}
