# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T22:23:53Z
- sweep_start_ts: 20260807-152351
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | PASS | eval/results/data_json_strict_ids-20260807-152353 | log_regex,answer_regex | none | 38s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | S37az production-positive：`instructions.md` 独立选择 `planner_distilled` 并携带两条具体规则，`users.json` 选择 `script_consumed` 且脚本真实读取；1 round、0 repair，最终严格为 `{"ids":["u1","u3"]}`。评估器思考中仍把 planner-distilled 材料说成“未覆盖”，但 typed 终态和答案未受影响，记为覆盖口径观察项。 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-152353 | answer_regex,answer_contains | none | 136s | 21 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | runner 的宽 oracle 通过，但人工答案不合格：补丁删除了全部三条已证调用箭头，却保留三条 `edge_anchors`，产出 node/Note-only 空关系图；正文还把 conditional flush 说成无条件，并把 `ConsoleSink.name()` 误作 registry 匹配依据。第一次拒绝未证 `SinkRegistry.create -> ConsoleSink` call 是正确的。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. JSON 的逐材料 usage-mode 教学获得干净生产正证，`EVAL-B298` 可关闭；没有 JSON 畸形、自愈、系统代写或输出关键词门。
2. C++ 第一次成文拒绝属于正确的 typed relation gate：factory registration/return 不能伪装为 direct call。唯一成文重试不算“次数过多”。
3. C++ patch 口头说只删未证边，实际 Mermaid body 删除全部箭头，却保留 `L->S`、`L->F`、`MS->SR` 三条 typed anchors；validator 只检查 visible edge -> anchor，没有检查 diagram-local anchor -> visible edge，因而接受结构自相矛盾的空图。此 gap 跨所有可执行语言，不是 C++ 特例。
4. 探索证据原本包含 `level >= kError` 条件和 `kind == "console" -> make_unique<ConsoleSink>` 多行注册事实；finalizer 的 Primary Evidence 却分别只剩第 38 行 `sink_->flush()` 与第 15 行函数签名。模型由此把 flush 扩大成无条件，并把相邻 `name()` 返回值误接成选择依据。立为独立 typed evidence span 失真件，不用输出字符串硬门修正文。
5. 本轮没有 runtime artifact，未触发 Trace；后续修复必须继续隔离 `QFRootCauseTrace`，不得影响显式时间窗、因果投影、自动补齐、根因排序、唤醒链和可消除量。
