# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T08:25:14Z
- sweep_start_ts: 20260808-012513
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_java_typo | PASS | eval/results/patch_java_typo-20260808-012514 | write_plan,write_patch_oracle | none | 53s | 20 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B328 核心生产正证：typed write-route shortcut 生效，analyzer 首轮零 prescan 发射 generic 场景，无 field/source-inventory/change-impact 等 read-only profile；总时长由 86s 降为 53s。模型 broad intent 仍选 `root_cause`，系统按既有 write-mode tolerance 放行，后续 write analyzer/plan 正确生成单行 Java patch；记 P2 taxonomy 观察，不以硬门纠正。 |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260808-012514 | answer_regex,answer_contains | none | 295s | 40 | read=3,repo_map=9,list=0,trace=0,source_lens=9 | midloop=13,inv=7/0,fin_reject=0,unavail=0,prune=2 | partial | 最终四 stage/职责/Mermaid 三边正确，finalizer 0 reject。B336 v1 仅部分生效：时长 579s→295s、repo_map 19→9，但 analyzer 改发 `completeness=true + requires_const_set + values` 且无 declared_count；structural-facet 保留臂让 source inventory 再次获得 completion 权，仍循环至第 20 轮。已据此扩展统一 typed 互斥。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human conclusion

- Human correctness: **1 PASS / 1 PARTIAL**。Java 写路径达到目标；图表用户答案正确，但 source-inventory 完成循环未闭环。
- `EVAL-B328` 的高 ROI 部分获得生产正证：typed mode/route 让 analyzer 零 read prescan、generic scenario、零 read-only optional profile。模型因 broad intent enum 缺少 change 类仍选 `root_cause`，记 `EVAL-B337-WRITEROUTEINTENTTAXONOMY1=P2-observe`，不以系统硬改 intent 处理。
- `EVAL-B336` 判 partial：v1 只覆盖 declared-count diagram；r199 的同义 typed 形使用 active completeness，并附加 const-set/values。完成权仍错置，不能按 295s 改善宣称关闭。
- B336 v2 的统一判据只读 typed `explain + architecture + mechanism + required diagram + (declared count 或 active completeness) + 非 category/count/relational`。在这个高层概念机制形中，模型附加的 const-set/values 不能反向确权；显式 typed source scope 仍可保留真正声明清单。
- r199 仍无“成文校验未通过”：图表 finalizer 一轮接受，循环全部发生在 explorer completion authority；模型答案未被系统替换、删除或改写。
