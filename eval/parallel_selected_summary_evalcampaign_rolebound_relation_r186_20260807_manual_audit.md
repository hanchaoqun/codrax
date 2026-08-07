# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T20:44:03Z
- sweep_start_ts: 20260807-134402
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260807-134404 | answer_regex,answer_contains | none | 170s | 22 | read=4,repo_map=4,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | S37as 获得生产正证：finalizer 收到 `Logger.log -> Sink.write`、`ConsoleSink -> Sink`、`ConsoleSink.write -> fputs/fputc` 三类 typed 事实和 candidate-only dynamic composition。模型仍另画 `Sink::write 虚调用 -> ConsoleSink::write` 的伪 call，且给首节点追加说明词而偏离 exact typed identity；validator 正确拒绝，patch 仅删除 optional diagram。正文主体正确区分工厂选择与虚分派，但把 error-only 的 `flush` 写成写完后一般动作，故不能判全对。B217 无标签 flowchart 本轮仍未被实际发射。 |
| 2 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260807-134404 | answer_regex,answer_contains | none | 208s | 21 | read=6,repo_map=3,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | S37aq 生产生效：Explorer 的 typed role-bound frontier 明确给出并促成落证 `HttpTransport.send -> this.dispatchOnce`，此前漏段恢复。Finalizer 的关系图却仍把 `client.fetchUser`/`ApiClient.fetchUser`、`this.dispatchOnce`/`HttpTransport.dispatchOnce` 视为三组件，因为 AST extractor 没把 `const client = new ApiClient` 与 class 内 `this` 铸成精确 receiver identity；模型靠源码自行拼接后又误称 GET、TCP 建连。首稿 `blocks` 字符串内部还在第二个 item 前漏 `{`，现有 recovery 正确拒绝部分发布，第二稿完整重发成功；登记 receiver graph P1 与 bounded JSON 自愈 P2。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
