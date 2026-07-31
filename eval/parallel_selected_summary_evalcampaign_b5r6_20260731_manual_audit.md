# Selected Eval Manual Audit Scaffold

- date: 2026-07-31T15:07:49Z
- sweep_start_ts: 20260731-080749
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_e2_cross_trace_asymmetry | PASS | eval/results/real_trace_e2_cross_trace_asymmetry-20260731-080749 | log_regex,answer_regex,answer_contains | none | 149s | 36 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | T9/T10 主路径已生效：无 Trace 因果投影，90 个 cpu_frequency rows 与 323 个 clock_set_rate events 分席，144.557ms/0.556ms 和不能直接对齐的安全结论正确。但 last-mile `trace_query 关键观测核对` 绕过共享 report authority，追加约 40 条与覆盖比较无关的 sleep/I/O 背景观测，T9 仍是 partial。正文另有 `156ba_frame` 生成损坏、两条 zero-match pattern 说明互换；validator 已软提示前者，不以答案正文扫描加 hard gate。local `alignment=identity` 被表述为两工件“时间基准相同”仍属 T7 filed。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260731-080749 | typed_inventory_rowset,dimension_substring,answer_contains | none | 159s | 28 | read=14,repo_map=2,list=3,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 最终 2 extend、2 foreign func、8 public class 的 path/name/package 与 oracle 一致。但 analyzer 把构造词写入非法 role enum 后，解析器将 profile 置 nil；稍后虽合成 function/method/type 并保留已验证整句 source quote，却已错过 required-file softening，模型猜测的 parser.go 成为唯一硬范围，同时合法 requested_fields.package 被默认 summary 覆盖。确定性 lens 因此只返回 Go parser 行，答案靠 14 read/3 list 回退。新增通用 S9。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: 2/2 PASS；人工：1/2 PASS。
- 已验收：T10；T9 结构化 materializer 部分已验收，但 last-mile renderer 未接线，不能关账。
- 新施工：T11（共享 last-mile report authority）与 S9（可选 profile 局部坏字段不得污染其余已验证字段/范围）。
