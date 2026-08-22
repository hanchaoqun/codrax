# Selected Eval Manual Audit Scaffold

- date: 2026-08-22T02:15:25Z
- sweep_start_ts: 20260821-191525
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | FAIL | eval/results/patch_c_typo-20260821-191526 | write_apply,write_patch_oracle,answer_contains | none | 147s | 27 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 补丁只改 `retrun`→`return`，`make test` 真实通过且 changed-path capability=`target_behavior`；但精确源码行合同 `compile-success` 没有 exact `contract_ref` observation。系统追加 verify-only cumulative review，原计划又没有 probe；C 不在 inline probe runtime roster，第二次只能重复同一 `make test`，最后把正确交付降为 `verification_proof_incomplete`。确认 B1319。 |
| 2 | sr_rust_cross_module_chain | FAIL | eval/results/sr_rust_cross_module_chain-20260821-191526 | answer_regex | none | 270s | 41 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=8,unavail=0,prune=0 | fail | 首稿 8 条调用 hop、源码坐标和 Mermaid 主体可用；系统正确要求从 citation index 切到 stable evidence ID，并同时发布 participant+relation delta。模型随后把完整 block 填进 `replace_snippets`；兼容层修复了 string-wrapped array，却把 block 字段当 unknown 隔离，旧 block 的 citation_ref 因而原样保留，8 次都报同一 evidence_ids 缺口并降级。确认 B1320。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B1319-WRITESOURCECONTRACTOBS1/P1`：写模式把“精确源码位置满足精确源码值”的 hard contract 与运行时行为合同混在同一证明出口。已有 project runner 已证明目标行为、post-apply 文件又能精确验证该源码断言，但系统缺少语言无关的 typed source-contract observation；在不支持 inline probe 的语言上会进入不可增强的 verify-only 循环。最优修复是仅对可无歧义解析的 repo-relative `path:line` + hard operator 做 post-apply 精确读取，铸造独立 `source_contract_refs` observation；运行时/异常/返回值等合同仍必须由 probe 或 exact project assertion 证明。
- `B1320-PATCHOPERATIONSHAPE1/P1`：patch JSON 已无损表达一个完整 block replacement，但落在 `replace_snippets` 名下。现兼容层只修 carrier 外壳、不修可唯一判定的操作形，随后静默 quarantine，导致“输入已修复”与“修复内容被丢弃”相矛盾。最优修复是在 schema normalization 前，仅当每个元素都唯一满足完整 block shape、且不同时提交 `replace_blocks` 时，把该数组整体重映射为 `replace_blocks`；混合 snippet/block、缺 block id、重复目标或并存冲突继续 fail-closed。系统不选择内容、关系、动作或结论。
- 两项均不读取用户请求、模型 thinking、答案 prose 或 Mermaid label 来做硬门；不改变 Trace 显式窗、因果投影、自动补齐、链上根因、业务线索、双轴归因或活动流降级边界。
