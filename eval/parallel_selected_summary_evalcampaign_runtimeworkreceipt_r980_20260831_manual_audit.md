# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T16:41:04Z
- sweep_start_ts: 20260831-094103
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260831-094104 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 183s | 42 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1496 核心生产命中：模型从 schema 精确选择 `VerifyClass ...` 行及较保守的关系结论，模型 principal 块与读者行都显示工作名、T7、0.285ms，系统未替模型选择结论；显式窗、4 次 typed query、链上根因、双账户、业务线索、自动补齐和 Trace 因果投影完整。但保守结论的本地化只写“关系未证”，丢掉同一 typed 行已有的“宿主随后直接唤醒目标”凭证与 completion/target-wait/frame 边界；模型 caveat 还复述了机器枚举 token。确认 B1497：事实展示必须保留凭证，枚举泄漏只做软教学，不能扫描改写正文。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260831-094104 | typed_inventory_rowset,dimension_substring,answer_contains | none | 270s | 28 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=3,inv=7/6,fin_reject=1,unavail=6,prune=0 | pass | 最终答案严格正确：extend=2、foreign func=2、public class=8，12 行符号/路径/package 全部一致，r979 的 5/8 包数错误未复现。过程仍有 6 次不可用 grep 尝试、7 次 completion 中 6 次因形态/错误子集被拒，以及 1 次标准表格 cells 修补；typed source-inventory 单源成功阻止错误 explorer closure 污染最终答案。确认独立 B1498：explorer 当前可用工具面与 prompt 建议不一致造成跨 inventory 的无效 churn。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: 2/2 PASS; human: Trace fail, Cangjie pass.
- `B1496` 的“模型选择精确行与结论”已获得生产正证；严格失败来自同一行读者展示丢凭证及模型复述内部枚举，归入 `B1497` 小批。
- `B1498-EXPLORERTOOLCAPABILITYALIGN1/P1`：只剩 completion 工具的 explorer 轮仍收到/沿用 grep 搜索教学，造成 6 次不可用调用和错误 closure 反复修补；必须按真实工具能力裁剪软教学，不能靠关键词或 case 豁免。
