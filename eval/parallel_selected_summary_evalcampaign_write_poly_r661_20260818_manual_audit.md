# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T03:50:50Z
- sweep_start_ts: 20260817-205047
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-205050 | write_apply,answer_regex | none | 140s | 25 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | pass-with-unverified-boundary | 两个目标文件均正确落地：truthiness 改为 `!== undefined`，并加入 false/0/空串三组回归，保留 existing-default。`make check` 只由 Python source-static checker 执行；宿主无 Node，后置 final report 正确降为 unverified，runner FAIL 不是代码失败。Controller 模型一度把 `passed` 误读成 all_verified，但确定性终态没有签绿。最终用户面仍裸露 reason code `production_verification_source_static_only`，记入既有 B756 客户语言债，不以扫描最终文字替换。 |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260817-205050 | answer_regex | none | 216s | 27 | read=6,repo_map=2,list=2,trace=0,source_lens=1 | midloop=4,inv=3/0,fin_reject=1,unavail=0,prune=0 | partial | Python `_HAVE_NATIVE` 分支、`_fastlex.tokenize_bytes`、PyO3 注册、Rust wrapper/core 与 `_tokenize_slow` fallback 均被读取；相较 r600 的 458s/5 reject 降为 216s/1 reject。首稿关系块无 edge anchors 被正确拒绝，补丁只提交入口 call、registration 和 fallback 内部 `list` call；正文仍把 wrapper→core 等多跳写成链，机器关系所有权不完整。并有“注册到 fastlex 包名”“吞吐显著下降”等未充分支持措辞。系统上下文已有 typed recipes，故不以 prose 硬门或系统代写修补。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch judgment

1. Runner `1 PASS / 1 FAIL`；人工为写交付 pass-with-unverified-boundary、跨语言关系 partial。
2. 新确认 `B1041-CALLCHAINSINGLESOURCEWIRE1`：Analyzer 明确知道当前请求中的起点
   `FastTokenizer.tokenize`，但误填 `discover_path + source + empty sink` 后，旧 wire validator 硬拒，迫使下一轮把
   source 清空。系统因此丢掉本可保留的 ordered start authority，后续只能约束“至少一条关系”，无法用既有
   `discover_terminal` 车道收敛到完整静态终点路径。
3. 根修只消费 schema 字段：`discover_path` 携带一个 source 且 sink 为空时，有 typed runtime-selection obligation
   则归一为 `discover`，否则归一为 `discover_terminal`；随后仍由现有 current-request provenance 校验保留或降回
   authority-free `discover_path`。不读 request/final prose，不创造 endpoint、edge、diagram 或结论。
4. “每个关系块至少一条 anchor”仍不能证明每段可见 prose 都有结构化所有权；本轮不通过数正文箭头、label 或
   claim-use 重复数加硬门。先用 B1041 恢复正确 endpoint lane 回放，若 typed terminal/path receipt 已充分而模型仍
   只提交子集，再按模型服从问题留档或设计独立的低心智 typed path carrier。
5. 写模式另见既有 B756：用户面原样显示 `production_verification_source_static_only`。这是确定性 renderer 展示债，
   后续应由 typed reason-code 字典输出客户语言，同时保留审计原值；禁止扫描模型答案做字符串替换。
6. 本批不改 Trace。显式时间窗、因果投影、自动补齐、链上-only 主因、实际占用/业务线索和规则可消除量双轴均
   保持；邻近/背景仍只作支持，活跃流未因固定 4ms 或累计年龄降级。
