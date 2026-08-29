# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T11:42:12Z
- sweep_start_ts: 20260829-044210
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260829-044212 | log_attachment,answer_contains | log_triage | 148s | 28 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=3/1,fin_reject=0,unavail=0,prune=0 | partial | B1452 部分生产转正：两种语言首帧/调用者及 peer 关系边界准确，上一轮凭 opaque 标签臆造的“JVM 线程”消失，analyzer 从 5 次降为 3 次。但第 1 次仍被通用 intent 枚举中“附加日志+代码位置=根因”带进 root-cause，与同一合同的 finite crash-frame/bounded-fact 规则冲突；随后又因无逐字用户禁源码句而修一次 source exclusion。完整结构化外部栈旁仍并排提供被 sanitizer 替换的原始日志，模型最终把 `<unverified-external-source>` 抄入读者 caveat。 |
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260829-044212 | answer_regex,answer_contains | none | 354s | 42 | read=15,repo_map=2,list=0,trace=0,source_lens=1 | midloop=16,inv=6/0,fin_reject=2,unavail=0,prune=2 | pass | 正文、引用和 Mermaid 主图均正确。首稿同时画了无证的 CondPre 聚合入口、为隐藏 L/P 锚未画对应边、且三条主链缺 visible label；validator 精确列出局部失败。模型第 2 轮误用 whole replace 被原子 repair lease 拒绝，第 3 轮改用 6 个局部 edge edit 后成功，最终保留 `analyze → explore → extract → finalize` 三条已证顺序边并移除无证入口。B1453 关系保真核心闭环，残余仅为 patch schema 心智/时延观察。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
