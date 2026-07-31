# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T08:20:03Z
- sweep_start_ts: 20260731-012003
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260731-012003 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 139s | 36 | read=0,repo_map=0,list=0,trace=9,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 四态 `157.248+5.604+70.338+0=233.190ms` 正确，显式窗的 Trace 因果投影/自动补齐仍在。限频回答错误：正文以“实际 558MHz/硬件峰值 2270MHz”和 830 次 transition 推断 CPU4“明显受限”，而 typed authority 明示 transition count 仅 background；直接窗内限制证据其实是 `cpu_frequency_limits cpu0 max=1530000,count=16` 与 `cpu4 max=2100000,count=28`。真正 `1.53GHz` witness 只在系统 footer 的另一链节点出现，旧全答 contains 因此假 PASS；4 个无结果 event_search 也显示精确 limit witness 没有进入 head-safe authority。 |
| 2 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260731-012003 | log_regex,answer_regex | none | 246s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 业务计算已正确：4 条贡献、GroupA/B/C=`17/4/5`、reference projection=`17,0,5`、reconcile=pass。终态仍因 decisions 第 7 行起引用模型 ordinal `R2`，而累计规则 ID 是 `rule_1..rule_6`，触发 unknown_rule_ref；NormalizeResult 只在存在 source-backed evidence_refs 时才有任意 fallback，本轮规则只有 typed notes，未做唯一 ordinal alias 归一。之后在 next_stage=complete 又尝试禁用的 custom_transform 是该 validation failure 的级联。初轮 `read_text` 非法 action 已一次修复，记效率波动，不单独做 case 特判。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
