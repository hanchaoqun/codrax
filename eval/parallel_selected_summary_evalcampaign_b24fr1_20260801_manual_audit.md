# Selected Eval Manual Audit Scaffold

- date: 2026-08-01T20:02:31Z
- sweep_start_ts: 20260801-130229
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_c_typo | PASS | eval/results/patch_c_typo-20260801-130231 | write_apply,write_patch_oracle,answer_contains | none | 116s | 19 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | plan/apply/verify 完成；applied tree 仍只有 `main.c` 的 `retrun`→`return` 一行。 |
| 2 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260801-130231 | answer_regex,answer_contains | none | 472s | 30 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=7,inv=2/1,fin_reject=10,unavail=0,prune=0 | pass | `call_chain_endpoint_omitted` hard gate 生效；最终结构化列表/表保留 `gate.Run` 并明确说明源码实际调用 `gate.RunWith`，图为 exact call-evidence star。新 gap：endpoint item 被 enumeration normalizer 反复删除，导致 10 rejects/472s；上下文文件 `analyzer.go` 也被误升为 endpoint。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- `EVAL-B24-ENDPOINT1` 的用户可见正确性已覆盖：不能再静默用 `gate.RunWith` 替代 `gate.Run`；无旧 rejected 图回流。
- 新登记 `EVAL-B24-ENDPPRUNE1/P1`：typed endpoint item 与 principal enumeration row-set 属于不同身份轴，现有 pruning 把前者反复删除，形成 hard retry loop。
- 新登记 `EVAL-B24-ENDPOINTSCOPE1/P1`：`MentionedEntities` 中的源码文件上下文 `analyzer.go` 被与 source/sink 同等提升为 endpoint，造成无关锚点行。
- `EVAL-B24-KEYSET1/P2` 加剧：最终可见 19 项模型列表 + 32 项系统完整清册 + 2 项补充 + 3 项锚点表；答案虽正确但远超“关键函数”所需，后续需 typed key-subset/complete-roster scope，不扫描用户原文“关键”。
