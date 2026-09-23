package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public query producers, the causal compiler and the actual
// answer mutation/render boundary. A block-request pair proves a residence
// measurement; its independently closed issuer wait does not prove that the
// same amount is device latency. No model description is rewritten here.
func TestIOVerdictPublicClosedWaitDoesNotClaimDeviceCause(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, state, rootType string
		pid                   int
	}{
		{"upstream_sleep", "S", "io_latency", 100},
		{"upstream_uninterruptible", "D", "d_state_or_io_wait", 100},
		{"target_is_issuer", "S", "io_latency", 200},
	} {
		for _, renamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/renamed=%t", tc.name, renamed), func(t *testing.T) {
				trace := strings.Replace(string(body), "prev_comm=document-worker prev_pid=200 prev_prio=120 prev_state=S", "prev_comm=document-worker prev_pid=200 prev_prio=120 prev_state="+tc.state, 1)
				issuer, completer, background := "document-worker-200", "storage-irq-80", "backup-900"
				if renamed {
					rename := strings.NewReplacer("document-worker", "loader", "storage-irq", "completion", "backup", "archiver", "app-main", "client")
					trace, issuer, completer, background = rename.Replace(trace), rename.Replace(issuer), rename.Replace(completer), rename.Replace(background)
				}
				path := hmc081WriteTrace(t, trace)
				bus, _, _ := hmc17NamedPathContext(t)
				var records []types.ObservationRecord
				for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
					result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": tc.pid, "time_start": 1.0, "time_end": 1.051})
					if !result.Success {
						t.Fatalf("public %s query failed: %s", view, result.Summary)
					}
					records = append(records, result.Observations...)
				}
				ledger := types.ObservationLedger{Records: records}
				projection := types.CompileTraceCausalProjection(ledger)
				if projection.PrimaryRootCause == nil {
					t.Fatal("fixture must retain a primary root")
				}
				if root := projection.PrimaryRootCause; root.Subject != issuer || root.TypeToken != tc.rootType || root.EffectiveImpactMS != 31 {
					t.Fatalf("fixture must retain the original 31ms issuer root: subject=%s type=%s impact=%.3f", root.Subject, root.TypeToken, root.EffectiveImpactMS)
				}
				ledgerBefore, _ := json.Marshal(ledger)
				projectionBefore, _ := json.Marshal(projection)
				for _, lang := range []string{"zh", "en"} {
					zh := lang == "zh"
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
					overview := runtimeTraceProjElimOverviewFence(projection, model, zh)
					root, rawWord := "issuer I/O wait", "block request residence"
					if zh {
						root, rawWord = "提交线程IO等待", "块设备请求耗时"
					}
					if tc.rootType == "d_state_or_io_wait" {
						root = "IO blocking·uninterruptible (cause unproven)"
						if zh {
							root = "IO阻塞·不可中断(原因未证)"
						}
					}
					seat := ""
					for _, line := range strings.Split(overview, "\n") {
						if strings.Contains(line, issuer) && strings.Contains(line, "31.000ms") {
							seat = line
							break
						}
					}
					// The long EN D-state verdict can put the following credential
					// on a continuation line. Its complete ruled word must survive;
					// an extra device suffix is not a valid ending on either shape.
					if !strings.Contains(seat, " · "+root+" ·") && !strings.HasSuffix(seat, " · "+root) {
						t.Errorf("PUBLIC_IO_VERDICT: closed issuer wait must retain its source-qualified verdict, not a device-cause refinement (%s): %q", lang, seat)
					}
					tree := runtimeTraceProjTreeFence(model, zh)
					if !strings.Contains(tree, rawWord) || !strings.Contains(tree, completer) || !strings.Contains(tree, "31.000ms") || !strings.Contains(tree, background) || !strings.Contains(tree, "47.000ms") {
						t.Errorf("tree must retain source words, counterpart identity and both native values (%s): %s", lang, tree)
					}
					// This includes the real overview and its conditional legend,
					// not merely a private type-label helper's return value.
					final := p3mRenderUserFace(t, records, lang)
					if !strings.Contains(final, root) || !strings.Contains(final, "31.000ms") || !strings.Contains(final, "47.000ms") {
						t.Errorf("final answer lost the unchanged verdict/value channels (%s)", lang)
					}
					for _, forbidden := range []string{"IO阻塞·设备延迟", "IO blocking·device latency"} {
						if strings.Contains(final, forbidden) {
							t.Errorf("PUBLIC_IO_VERDICT: actual answer/legend still teaches an unproved device cause: %s", forbidden)
						}
					}
				}
				ledgerAfter, _ := json.Marshal(ledger)
				projectionAfter, _ := json.Marshal(projection)
				if string(ledgerBefore) != string(ledgerAfter) || string(projectionBefore) != string(projectionAfter) {
					t.Fatal("display changed observations, ranking, values or causal authority")
				}
			})
		}
	}
}

func TestIOVerdictPublicMissingClosureDoesNotAcquireChainAuthority(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_business_io_chain", "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	// Only the authored completion-to-issuer wake event is removed; the
	// request pair and its physical 35ms residence remain genuine inputs.
	wake := "storage-irq-80 (2) [002] .... 1.040010: sched_wakeup: comm=document-worker pid=200 prio=120 target_cpu=002\n"
	if strings.Count(string(body), wake) != 1 {
		t.Fatal("fixture must contain one exact completion wake")
	}
	path := hmc081WriteTrace(t, strings.Replace(string(body), wake, "", 1))
	bus, _, _ := hmc17NamedPathContext(t)
	var records []types.ObservationRecord
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank", "critical_blocking_calls"} {
		result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 1.0, "time_end": 1.051})
		if !result.Success {
			t.Fatalf("public %s query failed: %s", view, result.Summary)
		}
		records = append(records, result.Observations...)
	}
	projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: records})
	for _, node := range projection.OnChainCauses {
		if node.TypeToken == "io_latency" {
			t.Errorf("missing completion proof must not acquire an on-chain IO seat: %s", node.Subject)
		}
	}
	nonChain := false
	for _, node := range append(append([]types.TraceCausalProjectionNode(nil), projection.AdjacentCauses...), projection.BackgroundCauses...) {
		if node.Subject == "document-worker-200" && node.TypeToken == "io_latency" && node.ImpactMS == 35 && !node.ResourceCompletionClosure {
			nonChain = true
		}
	}
	if !nonChain {
		t.Fatal("the unclosed request must retain its 35ms non-chain measurement")
	}
	for _, lang := range []string{"zh", "en"} {
		final := p3mRenderUserFace(t, records, lang)
		if !strings.Contains(final, "35.000ms") || !strings.Contains(final, "document-worker-200") {
			t.Errorf("missing closure must not erase the measured request (%s)", lang)
		}
	}
}

func TestIOVerdictUnmappedAndContextWordsKeepTheirAuthority(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, token := range []string{"sleep_wait", "unknown", "binder_wait", "not_a_registered_type"} {
			if word, mapped := runtimeTraceProjElimVerdictTokenWord(types.TraceCausalProjectionNode{}, token, zh); mapped {
				t.Errorf("unmapped %q acquired an IO/device diagnosis: %q", token, word)
			}
		}
		for _, relevance := range []string{"adjacent", "background"} {
			row := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{Role: types.TraceCausalRoleRootCauseContext, Predicate: "root_cause_" + relevance, Object: "io_latency", TypeToken: "io_latency", ChainRelevance: relevance}}
			want := "I/O duration (measurement unspecified)"
			if zh {
				want = "IO观测时长（口径未明确）"
			}
			if word := runtimeTraceProjElimClassWord(row, zh, false, nil); word != want {
				t.Errorf("non-diagnostic %s context must keep its original type word: got %q want %q", relevance, word, want)
			}
		}
	}
}
