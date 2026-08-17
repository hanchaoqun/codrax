# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T21:58:57Z
- sweep_start_ts: 20260817-145855
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260817-145857 | log_regex,answer_regex | none | 45s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | data_rounds=1,action_failed=0,fin_reject=0 | pass | 可见结果严格为 `{"ids":["u1","u3"]}`；本轮模型直接发出一个终态 custom_transform，1 批完成、无 deferred queue。证明 B1015 修复未伤害合法终态脚本，但未命中多 rank 脚本后缀的生产路径，后者仍由代码 pin 覆盖。 |
| 2 | read_combo_config_two_knobs_precedence | PASS | eval/results/read_combo_config_two_knobs_precedence-20260817-145857 | answer_regex,answer_contains | none | 205s | 34 | read=9,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | `pipeline_max_steps=50` 与三层顺序正确；`pipeline_max_retries_per_stage` 错把 CLI 未指定哨兵 0 / 过期 YAML 示例注释 2 当作代码默认，真实生产基线是 `pipelineSettings.MaxRetriesPerStage: 3`（cmd/root.go:3147）。Runner 的独立数字 3 正则被其他行号/文本误命中而假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner `2/2 PASS`，人工 `1 PASS / 1 FAIL`。
- B1015 兼容性正证：简单终态脚本仍可直接执行并发布严格 JSON；本轮不构成多 rank deferred-script 生产闭环，状态保持 `pending-production-replay`。
- 新确认 P1 `B1016-CONFIGDEFAULTPROVENANCE1`。Config-precedence 家族只要求“有若干 precedence evidence”，没有教清也没有检查三种数值口径：生产代码播种的 resolved baseline、配置示例中的文档默认、CLI flag 注册用的 inherit sentinel。Explorer 读了大文件开头和若干局部 flag/merge 行，却未读 `PipelineSettings` 初始化；仍以 high confidence 关闭，错误 aggregate 再污染 Finalizer。
- 泛化修向：在 typed `QFConfigPrecedence` 车道软引导每个用户 bucket 分别核对 production baseline、config override、CLI Changed/override guard；明确示例注释和 CLI sentinel 不能代替代码默认。Finalizer 同样只可把 production initializer/constant 作为代码默认；缺证则披露未核实，不得从相邻层猜值。该规则只依赖 question family、typed buckets/dimensions 和证据角色，不扫描用户或答案 prose 作硬门。
- Eval oracle 同批收紧为“代码默认值为/=/is 3”或生产初始化 `MaxRetriesPerStage: 3`，不再接受正文任意位置的裸数字 3。表格重复首列是次级展示问题，先观察异构表格再决定是否单独立案。
- 本批不触达 Trace 查询/投影；显式窗、自动补齐、链上-only 主因、背景 support-only 均保持。两条活跃流没有固定 4ms/4s 无答案降级。
