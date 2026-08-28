# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T23:15:55Z
- sweep_start_ts: 20260828-161554
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_python_typo | PASS | eval/results/patch_python_typo-20260828-161555 | write_plan,write_patch_oracle | none | 47s | 26 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划只修改 main.py 第 20 行 `retrun`→`return`，统一 diff 可应用，目标路径/owner anchor/风险/验收和 Python import+greet probe 均闭合；未扩大到其他文件，也未跳过 write 风险与计划门。 |
| 2 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260828-161555 | answer_regex | none | 178s | 29 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=3,unavail=0,prune=0 | partial | 终稿正确说明 main→run、walker 文件发现、index_file 逐行调用 Matcher.is_match，以及 walker 不参与匹配；但把 RegexLikeMatcher 的 `split(".*")`+顺序 `find` 简写成“正则匹配”。更严重的是模型第二稿丢失 required summary，系统明确记录 summary=0 却按 soft advisory 接受，形成 mandatory 教学与执行矛盾（B1420，已根修为 typed summary 同回合硬修）。三次 reject 分别为 blocks 字符串且第三 block 畸形、缺 edge_anchors、缺 from/to identity；初始上下文已有完整主链 recipes，后续候选因 selected evidence IDs 丢失按字母序截断，只给出末端实现边（B1421 next）。不扫描正文或由系统补写关系。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
