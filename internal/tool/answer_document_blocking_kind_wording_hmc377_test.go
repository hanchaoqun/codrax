package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 D2 — a blocking row must say what was proven, never 持锁阻塞
// without lock evidence. Batch 376 case B: the NetEase main thread's own
// "GC: Wait For Completion Alloc" wait and its fellow waiters were labelled
// 持锁阻塞 in the evidence index and the cross-check appendix although the
// window holds no lock or monitor record.

func TestHMC377BlockingWordComesFromTypedKind(t *testing.T) {
	cases := []struct {
		name, kind, basis string
		holder            bool
		zh, en            string
	}{
		{"recorded wait, converged", "", tracequery.BlockingValueBasisWaitSegments, false, "阻塞等待", "blocking wait"},
		{"recorded wait, legacy", "", "", false, "阻塞等待", "blocking wait"},
		{"recorded wait, envelope", "", tracequery.BlockingValueBasisSpanEnvelope, false, "span 包络(含运行)", "span envelope (includes running)"},
		{"recorded wait, holder note without payload", "", "", true, "阻塞等待", "blocking wait"},
		{"monitor contention, waiter", "monitor_contention", tracequery.BlockingValueBasisWaitSegments, false, "锁竞争等待", "lock contention wait"},
		{"lock contention, waiter", "lock_contention", "", false, "锁竞争等待", "lock contention wait"},
		{"monitor contention, holder", "monitor_contention", "", true, "持锁阻塞", "lock-holder blocking"},
	}
	for _, c := range cases {
		if got := TraceBlockingRowDisplayLabel(c.kind, c.basis, c.holder, true); got != c.zh {
			t.Errorf("%s zh = %q, want %q", c.name, got, c.zh)
		}
		if got := TraceBlockingRowDisplayLabel(c.kind, c.basis, c.holder, false); got != c.en {
			t.Errorf("%s en = %q, want %q", c.name, got, c.en)
		}
	}
	// The bare token cannot prove a lock: the token-only lexicon is the
	// kind-neutral recorded-wait word on every lane that has no row.
	for _, zh := range []bool{true, false} {
		want := "blocking wait"
		if zh {
			want = "阻塞等待"
		}
		for _, got := range []string{
			TraceRootCauseTypeDisplayLabel("blocking_span", zh),
			TraceMeasurementTypeDisplayLabel("blocking_span", "", zh),
			runtimeTraceCausalProjectionNarrativeCauseName("blocking_span", zh),
		} {
			if got != want {
				t.Fatalf("token-only blocking word = %q, want the kind-neutral %q", got, want)
			}
		}
	}
}

func hmc377BlockingNode(kind string, holder bool) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		EvidenceID: "cb-1", Subject: "urs_CalculatePu-11876", Predicate: "critical_blocking",
		Object: "ease.cloudmusic-11077", TypeToken: "blocking_span", ChainRelevance: "background",
		BlockingKind: kind, BlockingSubjectIsHolder: holder,
		BlockingValueBasis: tracequery.BlockingValueBasisWaitSegments, ImpactMS: 123.462, Confidence: 0.62,
	}
}

// The evidence index (third reader surface) carried 证据类型：持锁阻塞 on the
// GC completion wait and its fellow waiters (sealed 0212 E16–E18, E20–E21).
func TestHMC377EvidenceIndexTypeWordFollowsBlockingKind(t *testing.T) {
	for _, c := range []struct {
		kind   string
		holder bool
		want   string
	}{
		{"", false, "证据类型：阻塞等待"},
		{"monitor_contention", false, "证据类型：锁竞争等待"},
		{"monitor_contention", true, "证据类型：持锁阻塞"},
	} {
		got := runtimeTraceCausalProjectionEvidenceDisplayDetail(hmc377BlockingNode(c.kind, c.holder), true, false)
		if !strings.Contains(got, c.want) {
			t.Fatalf("kind=%q holder=%v: evidence detail %q lacks %q", c.kind, c.holder, got, c.want)
		}
		if c.kind == "" && strings.Contains(got, "锁") {
			t.Fatalf("a blocking row without a lock payload names a lock: %q", got)
		}
	}
}

// A fellow waiter is named as such, beside the upstream party; a waker that
// did the awaited work keeps the neutral reading (no relay note).
func TestHMC377RelayWakerIsNamedAFellowWaiter(t *testing.T) {
	node := hmc377BlockingNode("", false)
	neutral := runtimeTraceCausalProjectionDisplayCauseNameNode(node, true)
	if neutral != "阻塞等待(span 期间最后唤醒者(推断) ease.cloudmusic-11077)" {
		t.Fatalf("neutral relation form drifted: %q", neutral)
	}
	node.BlockingWakeRelay = true
	node.BlockingWakeRelayUpstream = "HeapTaskDaemon-11098"
	if got := runtimeTraceCausalProjectionDisplayCauseNameNode(node, true); got != "阻塞等待(唤醒由同在等待的 ease.cloudmusic-11077 转发;上游 HeapTaskDaemon-11098)" {
		t.Fatalf("relay relation form = %q", got)
	}
	if got := runtimeTraceCausalProjectionDisplayCauseNameNode(node, false); got != "blocking wait (wakeup passed on by fellow waiter ease.cloudmusic-11077; upstream HeapTaskDaemon-11098)" {
		t.Fatalf("relay relation form (en) = %q", got)
	}
	// A typed lock row never takes the relay form, whatever its notes.
	lock := hmc377BlockingNode("monitor_contention", false)
	lock.BlockingWakeRelay, lock.BlockingWakeRelayUpstream = true, "HeapTaskDaemon-11098"
	if got := runtimeTraceCausalProjectionDisplayCauseNameNode(lock, true); strings.Contains(got, "转发") {
		t.Fatalf("typed lock row wore the relay form: %q", got)
	}
}

// The holder-subject rank row of a real monitor contention keeps 持锁阻塞 on
// the narrative, cause and detail type lanes; a waiter-subject rank row (its
// holder unresolved) is a lock-contention WAIT, never a hold.
func TestHMC377LockRankRowsKeepLockWords(t *testing.T) {
	holder := types.TraceCausalProjectionNode{
		EvidenceID: "rk-1", Subject: "#LogReaper-11293", Predicate: "root_cause_primary", Object: "blocking_span",
		TypeToken: "blocking_span", BlockingKind: "monitor_contention", BlockingSubjectIsHolder: true,
		BlockingPeer: ".ugc.aweme.lite-11105", Rank: 1, Tier: "primary", ChainRelevance: "on_chain",
	}
	for name, got := range map[string]string{
		"narrative": runtimeTraceCausalProjectionNarrativeCauseNameNode(holder, true),
		"cause":     runtimeTraceCausalProjectionDisplayCauseNameNode(holder, true),
		"type":      runtimeTraceCausalProjectionDetailTypeLabel(holder, true),
	} {
		if got != "持锁阻塞" {
			t.Errorf("holder rank row %s word = %q, want 持锁阻塞", name, got)
		}
	}
	waiter := holder
	waiter.BlockingSubjectIsHolder = false
	if got := runtimeTraceCausalProjectionNarrativeCauseNameNode(waiter, true); got != "锁竞争等待" {
		t.Fatalf("waiter-subject lock rank row narrative = %q, want 锁竞争等待", got)
	}
}

// One source: the lock words exist as string literals in exactly one
// non-test file of this package (the single blocking word source). Scans the
// real source files.
func TestHMC377LockWordsHaveOneSource(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, word := range []string{"持锁阻塞", "锁竞争等待", "lock-holder blocking", "lock contention wait"} {
				if strings.Contains(value, word) {
					owners[file] = true
				}
			}
			return true
		})
	}
	if len(owners) != 1 || !owners["answer_document_measurement_display.go"] {
		t.Fatalf("lock words must live only in the single blocking word source, found in %v", owners)
	}
}
