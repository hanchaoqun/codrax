# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T10:50:39Z
- sweep_start_ts: 20260821-035039
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-035039 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 184s | 41 | read=0,repo_map=0,list=0,trace=10,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | 精确 2.000..2.020s；4 节点唤醒链；11ms 链上 IO 第一席；三个独立 1ms 反转候选；实际占时/可消除双账、业务下钻和完整 Trace 因果投影均在。邻近/背景未加冕，活动流未按固定耗时降级；内核调用点明确披露为调用位置而非具体持有者/资源。 |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260821-035039 | answer_regex,answer_contains | none | 649s | 47 | read=16,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=2/0,fin_reject=9,unavail=0,prune=6 | fail | 前两轮漏 diagram kind，第三轮进入关系 delta；B1288 发布 3 个 live addition_ref，模型也逐一选择。执行前第一个 add 的 action 值断裂为 `action\": \"add`，同一坏参数连续复用，最终只能降级展示旧草稿。不是 relation authority 再误拒；确认为通用 Schema 枚举 JSON 自愈缺口 B1290。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 人工结论

1. `B1288` 的生产信号为正：live candidate ref 已进入 retry capsule，模型没有再猜 Stage/Agent 身份，而是明确选择三个允许的 stage precedence 候选。该次未能验证原子执行成功，仅因为调用参数在 schema 校验前发生独立 JSON 枚举值断裂。
2. 读模式降级答案仍保留 Mermaid 和表格，没有“空答案”；但关系稿未经最终结构校验，因此该 case 必须判 fail。降级发生于 9 次真实成文拒绝之后，不是活动流或固定 4ms/4m 阈值触发。
3. `B1290-ENUMFRAGMENT1` 采用通用确定性方案：只有完整字符串能还原为“当前 schema 字段本身 + 当前 string enum 的精确成员”时才修复；不同字段、未知枚举、多个字段、前后 prose 一律不动。修复只恢复结构化参数，不决定 action、关系、节点、措辞或结论。
