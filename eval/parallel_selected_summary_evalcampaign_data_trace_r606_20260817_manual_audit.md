# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T09:35:06Z
- sweep_start_ts: 20260817-023505
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260817-023506 | log_regex,answer_regex | none | 71s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | B958 的两次固定修补仍不足。首稿先缺 `actions[0].input_paths`；第一修补前进后暴露 complete-reference 的 `reference_path` 缠失；第二修补继续前进后才暴露 join alias 冲突。每一轮都修正了一个精确 typed locus，但固定 pass=2 在第三个独立错误前终止，仍未执行、对账或产出答案。确认 B959：结构修补必须按“参数对象前进且失败指纹变化”继续，在重复参数/重复失败时立即停，并保留独立安全上限。 |
| 2 | trace_query_wakeup_causal_runnable | PASS | eval/results/trace_query_wakeup_causal_runnable-20260817-023506 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 160s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass-core/context-partial | 两次显式窗 trace_query、链上 worker-200、8.300ms 有效归因、10.000ms 目标 sleep、3.500ms 背景分流、系统补采和 Trace 因果投影都在；无 4ms 降级，系统未替写模型结论。人工发现 B960：Analyzer 明明收到 route current_source=optional，却自行发 `external_observation_policy.current_source_mode=allow`，该无独立 source obligation 的 enum 被当硬权威，触发 4,187 文件源码图；旧 B957 只修 invalid-exclude 分支，生产 gap 未闭。另有 B961 软上下文 gap：模型把 8.300ms runnable 口头绑定到 1.009500–1.010000（实际该段是 running 0.500ms），typed 表/投影正确；先改 typed 上下文表达和教学，不增加正文扫描或系统改写。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
