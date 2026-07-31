# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T14:24:30Z
- sweep_start_ts: 20260731-072430
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-072430 | log_regex,answer_regex,answer_contains | none | 178s | 35 | read=1,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | fail | 当前代码已不发布 `Trace 因果投影`，144.557ms、0.556ms、CPU frequency=90、单边 VSync/频率和不可直对齐结论正确；但纯 trace 比较被升级成 architecture/current_code_path 合同，触发 3 次 completion 与 external waiver。最终又把工具已明确给出的 Harmony `1-40=CFS, 41-159=RT` 反写为 `1-40=RT, 41-159=normal`，故人工不通过。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260731-072430 | typed_inventory_rowset,dimension_substring,answer_contains | none | 283s | 27 | read=17,repo_map=2,list=3,trace=0,source_lens=2 | midloop=10,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass | 最终清单与 name/location/package 维度完整；但两个 source-inventory lens 仍只有 Go 行，模型靠 17 次 read、3 次 list、18 explorer rounds 才逐文件恢复。持久图已有 Cangjie 路径/census，辅助投影也执行，却把同路径 path-only/ParseTier4 行当成“已覆盖”跳过，暴露结构化库存刷新 GAP。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual conclusions

- Trace 的批 G 发布权限已经覆盖：`trace_query_final_projection_blocks=0`，因此这类全工件覆盖/采样比较不需要 `Trace 因果投影`。显式时间窗、诊断、root-cause、call-chain 和真实 publication-grade causal row 的正臂仍由现有专项测试保留。
- Trace 的新主问题是“运行时 observation 被套进源码 architecture 合同”，不是因果投影再次丢失。修复应读取 validated runtime scope 与 source-lane typed policy，不能扫问题/答案关键词。
- Cangjie 自动 PASS 证明最终可恢复，不证明库存工具健康。高 ROI 修点是允许有界辅助投影刷新同路径空壳/降级 FileInfo，并在 lens 返回后恢复原持久图。
- Harmony priority 反写与工具权威行直接矛盾，但当前只有一次模型 witness；先作为模型波动记账，不建立答案正文关键词 hard gate。若后续不同模型/多 case 复现，再设计 typed priority observation 的可见字段投影。
