# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T22:28:27Z
- sweep_start_ts: 20260806-152825
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260806-152827 | answer_regex | none | 125s | 20 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | S31 已使 `run -> walker::collect_files @20` 直接 grounded，原 20→10/nearest_call 消失；但模型把 `collect_files -> walk` 错报在递归行 19，系统规范成 `walk -> walk` 后仍用原 summary 反馈为 grounded，Explorer 未补第 6 行，diagram 两次 reject 后被删。 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-152827 | answer_regex,answer_contains | none | 172s | 20 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=1/0,fin_reject=2,unavail=0,prune=0 | fail | 文字说明基本正确，但用户要求 console 后端完整路径；最终图因把 caller/guard/virtual dispatch 都画成调用而两拒后删除，且主列表没有把 `console -> ConsoleSink` 选择分支与 `ConsoleSink::write` 终点作为独立、精确引用的链段。runner 只验关键词，属假绿。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `EVAL-B200-QUALCALL1` 的原始 `::` leaf gap 已获生产正证：Rust 第 20 行保持 exact Grounded，零 `nearest_call`、零行号错迁。
- 新确认 `EVAL-B201-DIRCALL1=P1`：显式 caller/callee 与所报行冲突时，系统按该行静默换成另一条真实边，模型内部 summary 仍描述原边，
  形成“typed 字段已改、反馈语义未改”的同轮误导。最优修复是用完整 typed caller+callee 对在已读同文件内定向恢复；不能仅按 callee 邻近，
  也不能放宽下游 diagram gate。
- 新登记 `EVAL-B202-POLYGRAPH1=P1`：动态分发/工厂选择问题需要把 direct call、guard、selection binding、declared type、runtime dispatch
  分层表达。当前 schema 已有 guard/register/type_relation，但 Explorer 没有形成 console 分支的 typed binding/return 链，模型便把概念节点都画成 call，
  两拒后删图。后续需跨 C++/Java/ArkTS/Cangjie 一起审计，不按 C++ 类名特判。
