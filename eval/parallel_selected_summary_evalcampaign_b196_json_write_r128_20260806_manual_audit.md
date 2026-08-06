# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T21:34:25Z
- sweep_start_ts: 20260806-143423
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260806-143425 | write_plan,write_patch_oracle | none | 64s | 20 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确读取 Main.java 并生成单行 `retrun`→`return` patch 计划，范围、验收与用户要求一致；没有数据合同改动侵入写模式。 |
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260806-143425 | log_regex,answer_regex | none | 371s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | S28 单元边界仍未进入生产：原终态两动作 DAG 被依赖拆批后，prefix/remainder 的 continue_after 均被系统强制改为 true；首批正确 `{"ids":["u1","u3"]}` 只能成为 emitted_payload，随后 optional ledger 被扩成强制链，6 次 repair/8 批后把字段漂成 user_ids 并失败。另确认 complete typed state 会把 evaluator 明确的 continue_data 静默改成 complete，属于系统覆盖模型语义结论的红线。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case judgment

- `EVAL-B196-DATAJSONTERM1` 仍未生产闭环。S28 修复的 terminal child contract 本身正确，但依赖拆批器篡改了原计划的终态意图，导致该分支从未触发。
- 新增 `EVAL-B197-SPLITTERM1=P0/system-contract-contradiction`：dependency prefix/remainder 不能把原始 `continue_after=false` 改成 true；deferred queue 已是独立 continuation authority。
- 新增 `EVAL-B198-EVALAUTH1=P0/red-line`：typed 结构完成只证明可交付，不证明值满足用户语义；不得把模型明确的 continue/repair 改写为 complete。无工具调用的保守 fallback 可单独收敛。
- JSON 教学存在同源措辞分叉：一面说脚本必须调用 emit/emit_result，一面允许 `result=value`；应统一为一个推荐通道和两个明确备选，`print` 只作调试。
- Java 写模式保持正确，说明本轮故障集中在 data workflow 的拆批/完成/评估权限链。
