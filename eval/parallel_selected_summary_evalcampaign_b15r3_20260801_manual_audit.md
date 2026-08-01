# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T02:18:29Z
- sweep_start_ts: 20260731-191827
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_config_absent_present_mix | PASS | eval/results/read_combo_config_absent_present_mix-20260731-191829 | answer_regex,answer_contains | none | 150s | 20 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=4,inv=3/1,fin_reject=0,unavail=0,prune=0 | fail | NEG1 生效：system lead 精确列出 phantom target 在 cmd/root.go、codrax.yaml.example、runtime.go 三个已验证 file scope，且明示 unlisted=unproven/cross-target borrowing forbidden；但它前面仍渲染 document-level “当前已验证范围内未找到完全一致的精确目标”，与同一答案中第二个 target 的 present/value=0 相矛盾。机器 PASS 掩盖 mixed-target exact-resolution 不能由一个全局 absent 状态表达的 XR1。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260731-191829 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 177s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 系统投影非回退：完整 Trace 因果投影、窗内可消除量、VerifyClass/类校验 0.285ms、最晚相关边 34579.496810s、凭证=直接裸边均在。runner 仅因 oracle 使用 ASCII 逗号而系统板使用中点分隔失败，属契约标点漂移；但模型摘要又把 34579.495841–34579.496126 的 span 错说成不在 34579.490–34579.500 窗内，整体人工仍 fail。typed 系统板正确，先作为 model window-membership variance 留档，不增加答案原文硬门。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B15-NEG1`：真实回放 covered。三条系统权限行均来自本轮 accepted
  negative evidence，没有跨目标借用，也没有把未列 scope 扩成全仓结论。
- `EVAL-B15-XR1`：升级为 P1。多目标 contract 下一个 document-level
  `exact_resolution=absent` 没有 target_ref，不能安全代表“一项 absent、
  另一项 present”；当前 renderer 因而发布了错误的全局横幅。
- `EVAL-B15-H8O1`：runner oracle drift。期望的语义、值、边时间和凭证均在，
  仅分隔符从 `,` 变成 `·`；更新 case oracle，而不改变生产文案。
- `EVAL-B15-H8MV1`：模型把目标窗内 span 叙述为窗外；typed projection 正确且
  `trace_query_final_projection_blocks=2`。先按模型波动留档，等待跨 case
  复现，不做请求/答案关键词扫描或 case-specific gate。
