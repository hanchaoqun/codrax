# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T17:13:36Z
- sweep_start_ts: 20260807-101335
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260807-101336 | dimension_substring,answer_contains | none | 90s | 20 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | partial | typed inventory 的 1 个 extend、1 个 foreign func、3 个 public class 及 package/file/line 全部正确；patch 将 required table 删除后另加三段，导致摘要与主体重复，并在 authority complete 时追加“覆盖可能不充分”泛化 caveat。确认 EVAL-B295-PATCHREQUIREDKINDDRIFT1。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260807-101336 | answer_regex,answer_contains | none | 266s | 30 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=9,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | 最终把 gate.RunWith/gate.Run 包装方向说反，并把 buildAnalysisIR 的 sibling fan-out 说成 20 hop path；系统 copy-ready payload 自带重复箭头/anchor，且固定前 8 条截断使 gate.RunWith 等承重关系缺席。S37af 未获生产 witness，因为 Explorer 没有发 gate.Run 到 RunWith 的 typed call edge。确认 B292/B293/B294。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
