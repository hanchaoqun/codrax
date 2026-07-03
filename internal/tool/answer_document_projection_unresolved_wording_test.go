package tool

// R1+R3 presentation fixes (customer complaint 2026-07-03):
//   R1 — the unknown-thread sentinel renders as typed unresolved-peer wording
//        ("未定位线程" was unreadable): blocking_span → 阻塞等待(对端未解析),
//        d_state_or_io_wait → D状态/IO等待(对端未解析), generic otherwise; all
//        wording lives in runtimeTraceCausalProjectionUnresolvedPeerText.
//   R3fold — the "其余 N 项合并" fold line keeps the folded thread names via
//        the typed MergedSubjects roster.
//   Legend — the 树读法/口径 explainers render as one-clause-per-line lists,
//        not run-on paragraphs.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceProjectionUnresolvedPeerTextCentralWording(t *testing.T) {
	cases := []struct {
		kind   string
		zh, en string
	}{
		{"blocking_span", "阻塞等待(对端未解析)", "blocking wait (peer unresolved)"},
		{"d_state_or_io_wait", "D状态/IO等待(对端未解析)", "D-state/IO wait (peer unresolved)"},
		{"", "对端线程未解析", "unresolved wait peer"},
		{"something_else", "对端线程未解析", "unresolved wait peer"},
	}
	for _, tc := range cases {
		if got := runtimeTraceCausalProjectionUnresolvedPeerText(tc.kind, true); got != tc.zh {
			t.Fatalf("kind %q zh wording = %q, want %q", tc.kind, got, tc.zh)
		}
		if got := runtimeTraceCausalProjectionUnresolvedPeerText(tc.kind, false); got != tc.en {
			t.Fatalf("kind %q en wording = %q, want %q", tc.kind, got, tc.en)
		}
	}
}

func TestTraceProjectionUnresolvedPeerKindTypedTokenScan(t *testing.T) {
	cases := []struct {
		name string
		node types.TraceCausalProjectionNode
		want string
	}{
		{"type token wins", types.TraceCausalProjectionNode{TypeToken: "blocking_span", Predicate: "critical_blocking"}, "blocking_span"},
		{"predicate lane", types.TraceCausalProjectionNode{Predicate: "d_state_or_io_wait"}, "d_state_or_io_wait"},
		{"object lane", types.TraceCausalProjectionNode{Predicate: "root_cause_primary", Object: "d_state_or_io_wait"}, "d_state_or_io_wait"},
		{"case/space canonical", types.TraceCausalProjectionNode{TypeToken: " Blocking_Span "}, "blocking_span"},
		{"no typed kind", types.TraceCausalProjectionNode{Predicate: "wakeup_causal_impact", Object: "cpu_pressure"}, ""},
	}
	for _, tc := range cases {
		if got := runtimeTraceCausalProjectionUnresolvedPeerKind(tc.node); got != tc.want {
			t.Fatalf("%s: kind = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTraceProjectionDisplaySubjectNameTypedUnresolved(t *testing.T) {
	node := types.TraceCausalProjectionNode{Subject: "unknown-thread", TypeToken: "blocking_span"}
	if got := runtimeTraceCausalProjectionDisplaySubjectName(node, true); got != "阻塞等待(对端未解析)" {
		t.Fatalf("sentinel subject with blocking_span kind = %q", got)
	}
	generic := types.TraceCausalProjectionNode{Subject: "unknown-thread", Predicate: "wakeup_causal_impact"}
	if got := runtimeTraceCausalProjectionDisplaySubjectName(generic, true); got != "对端线程未解析" {
		t.Fatalf("sentinel subject without typed kind = %q", got)
	}
	real := types.TraceCausalProjectionNode{Subject: "isplogcat-1494"}
	if got := runtimeTraceCausalProjectionDisplaySubjectName(real, true); got != "isplogcat-1494" {
		t.Fatalf("real subject must render verbatim, got %q", got)
	}
}

// End-to-end: a blocking_span critical_blocking row whose subject could not be
// resolved (and whose payload parsed NO lock semantics) renders the typed
// blocked-wait wording, never the bare sentinel label — ZH and EN.
func TestTraceProjectionBlockingSpanUnresolvedSubjectRendersTypedWording(t *testing.T) {
	obs := func() []types.ObservationRecord {
		return []types.ObservationRecord{
			projV3Obs("cb-span", "critical_blocking", "critical_blocking:blocking_span",
				"unknown-thread", "AssetsUtil_Operate_0-42067", "112.223", 112.223, 45696, 45696,
				"type=blocking_span", "chain_relevance=on_chain", "causality=on_wakeup_chain"),
		}
	}
	zhMD := audit730Render(t, audit730Bus(""), obs(), "")
	if !strings.Contains(zhMD, "阻塞等待(对端未解析)") {
		t.Fatalf("blocking_span unresolved subject must render typed wording:\n%s", zhMD)
	}
	for _, banned := range []string{"未定位线程", "unknown-thread"} {
		if strings.Contains(zhMD, banned) {
			t.Fatalf("sentinel label %q must not render:\n%s", banned, zhMD)
		}
	}
	enMD := audit730Render(t, audit730Bus("en"), obs(), "en")
	if !strings.Contains(enMD, "blocking wait (peer unresolved)") {
		t.Fatalf("EN blocking_span unresolved subject must render typed wording:\n%s", enMD)
	}
}

// R3fold rendering: the subjectless fold row names its folded members; the 等
// (zh) / … (en) marker appears only when MergedCount exceeds the roster.
func TestTraceProjectionMergedFoldRowKeepsThreadNames(t *testing.T) {
	full := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		MergedCount:    2,
		MergedSubjects: []string{"isplogcat-1494", "xlogcat-1431"},
	}}
	if got := runtimeTraceProjRowName(full, true); got != "其余 2 项合并(isplogcat-1494、xlogcat-1431)" {
		t.Fatalf("zh fold row = %q", got)
	}
	if got := runtimeTraceProjRowName(full, false); got != "2 more folded (isplogcat-1494, xlogcat-1431)" {
		t.Fatalf("en fold row = %q", got)
	}
	overflow := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		MergedCount:    6,
		MergedSubjects: []string{"a-1", "b-2", "c-3", "d-4"},
	}}
	if got := runtimeTraceProjRowName(overflow, true); got != "其余 6 项合并(a-1、b-2、c-3、d-4 等)" {
		t.Fatalf("zh overflow fold row = %q", got)
	}
	if got := runtimeTraceProjRowName(overflow, false); got != "6 more folded (a-1, b-2, c-3, d-4, …)" {
		t.Fatalf("en overflow fold row = %q", got)
	}
	// No roster (e.g. legacy payloads) keeps the bare count line.
	bare := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{MergedCount: 3}}
	if got := runtimeTraceProjRowName(bare, true); got != "其余 3 项合并" {
		t.Fatalf("zh bare fold row = %q", got)
	}
}

// Legend itemization: both explainer stanzas render as an intro line followed
// by plain "- " items (one clause per line, no run-on paragraph).
func TestTraceProjectionLegendsRenderAsItemLists(t *testing.T) {
	zhMD := audit730Render(t, audit730Bus(""), audit730ChainObs(), "")
	for _, want := range []string{
		"树读法:\n- 自上而下 = 从关注线程向上游追溯。",
		"- `⛔` = 窗口内无匹配 sched_wakeup,链止于此。",
		"口径:\n- 窗口投影 = 节点在用户窗口内的投影影响。",
		"- 背景行仅作压力/环境证据,不自动等同 on-chain 主因。",
	} {
		if !strings.Contains(zhMD, want) {
			t.Fatalf("zh legend must itemize (%q missing):\n%s", want, zhMD)
		}
	}
	// The old run-on separators must be gone from the two stanzas.
	for _, banned := range []string{"树读法: 自上而下=", "口径:窗口投影="} {
		if strings.Contains(zhMD, banned) {
			t.Fatalf("zh legend must not keep the run-on paragraph form (%q):\n%s", banned, zhMD)
		}
	}
	enMD := audit730Render(t, audit730Bus("en"), audit730ChainObs(), "en")
	for _, want := range []string{
		"Tree reading:\n- Top-down = tracing upstream from the focused thread.",
		"Legend:\n- window projection = the node's projected impact inside the user window.",
	} {
		if !strings.Contains(enMD, want) {
			t.Fatalf("en legend must itemize (%q missing):\n%s", want, enMD)
		}
	}
}
