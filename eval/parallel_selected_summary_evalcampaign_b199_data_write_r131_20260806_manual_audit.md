# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T22:10:32Z
- sweep_start_ts: 20260806-151030
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_jsonl_filter_count | PASS | eval/results/data_jsonl_filter_count-20260806-151032 | log_regex,answer_regex | none | 66s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 最终严格单行 `2`。初始脚本没有实际消费声明为 required 的 instructions.md，精确材料门触发一次 repair；修复后同时消费 instructions.md/events.jsonl 并直接终态完成。没有畸形 JSON、字段猜测、可选 ledger 强制重建或 evaluator 越权。 |
| 2 | patch_c_typo | PASS | eval/results/patch_c_typo-20260806-151032 | write_apply,write_patch_oracle,answer_contains | none | 123s | 21 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | isolated worktree 中仅 `main.c` 一行 `retrun buf;`→`return buf;`，plan kind=patch，1 个测试通过，最终 verified；未扩文件、未以 modify 覆盖整文件、未清空累计验证域。Analyzer 首次把 target_kind 写成 schema 不支持的 keyword 后一次自修为 string_literal，属单次模型 enum 失误，教学已给 accepted enum，未形成矛盾循环。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- 数据与写模式均人工通过。两处一次性修复分别对应真实材料未消费和模型选错 schema enum；没有“同一合同同时必带/必拒”的系统矛盾，也没有成文校验循环。
- JSON 单通道教学继续稳定；终态 evaluator 权限、write plan→apply→verify 累计验证与 micro-scope patch 约束均正常。
- 本批未发现需要立即施工的新高 ROI gap。
