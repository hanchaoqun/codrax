# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T09:12:05Z
- sweep_start_ts: 20260812-021203
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260812-021205 | log_regex,answer_regex | none | 39s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Data lane 正确消费 instructions.md 与 users.json，按源顺序筛出 active id，最终严格只输出 `{"ids":["u1","u3"]}`。零额外散文、零 JSON repair、零重试；当前 JSON 教学与终态投影一致。 |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260812-021205 | answer_regex,answer_contains,mermaid_edge_count | none | 728s | 52 | read=15,repo_map=2,list=0,trace=0,source_lens=0 | midloop=15,inv=4/1,fin_reject=13,unavail=0,prune=1 | fail | B630b gate 可接受 exact participant endpoint ID，但 repair payload 未声明 participant 落在 candidate 的 from/to 哪一端。模型把 ctx.Mutable.* 技术方法画成独立 endpoint，再反复新增无证 method→Mutable 桥，或把 Mutable 错留为 disconnected；关系门正确拒绝，13 次后恢复旧稿。最终图无 BusContext containment、Mutable 仍断开，且内部方法术语过多。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- JSON 严格输出通过，未发现系统教学诱发的畸形 JSON、额外解释或错误恢复；该 lane 当前无需加硬约束。
- 逻辑图确认 `B630c-DIAGRAMPARTICIPANTENDPOINTSIDE1/P0`：candidate 已有精确技术 endpoint 和方向，但缺少 participant 对应哪一端的 typed 映射，模型被迫猜并生成无证桥边。
- 逻辑图全程保持活跃并运行 728s；最终 degraded 是 13 次精确结构拒绝后的失败恢复，不是 elapsed-age 或 4 分钟触发。固定总时长仍未参与降级判定。
