# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T20:53:07Z
- sweep_start_ts: 20260806-135306
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260806-135307 | write_apply,write_patch_oracle,answer_contains | none | 98s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 单文件单行 patch 精确，应用后 `go test -json ./...` 通过；计划、apply、verify、finish 状态一致，未见累计校验域丢失或 JSON 降级。9 次 pipeline dispatch 是 Auto Pilot 多阶段基线开销，未改变修改范围。 |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260806-135307 | answer_regex,answer_contains | none | 195s | 23 | read=13,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=4/0,fin_reject=1,unavail=0,prune=0 | fail | S25 生效：第二稿使用实现类型→接口的 `type_relation` 边并被接受，12 个生产实现、文件及引用正确，只有第一次反向作图被合理拒绝。但 Analyzer 将“主要实现类型”误作 `public_exported`；typed exclusion 随后把 12 个未导出成员全部清空，仅遗留 12 条 notes，连续三次把合法 `members[]` 误报为 empty，第四次靠低增量熔断放行。Finalizer 收到 value=0/members=[]/notes=12 后追加虚假的“证据较弱”提示。确认 EVAL-B194-MEMBERCARRIER1。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- r125 runner 2/2 PASS；人工 1/2 PASS。写模式基准保持健康。
- `EVAL-B193-IMPLHANDOFF1` 可按生产回放闭环：类型关系权威已进入 finalizer，正确方向不再假拒绝。
- 新 P0 `EVAL-B194-MEMBERCARRIER1` 不是 malformed JSON。原始 tool-call JSON 可完整解析且确有 12 个成员；错误发生在 typed visibility/exclusion 后处理。系统把自身过滤结果伪装成模型未传数组，既制造三轮无效 JSON 重试，又破坏位置对齐载体并向客户发布错误 caveat。
