# Selected Eval Manual Audit Scaffold

- date: 2026-08-18T09:22:47Z
- sweep_start_ts: 20260818-022246
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_c_platform_fork | PASS | eval/results/sr_c_platform_fork-20260818-022247 | answer_regex,answer_contains | none | 132s | 25 | read=3,repo_map=2,list=1,trace=0,source_lens=1 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 平台与 handler 结论正确，7 条 selected-definition body call 仍在；但 B1062 稳定复现且更明确。模型提交 14 个 citation，并在表格位置单元格明确写出 `clock.c:15, 17`、`:28, 30`、`handlers.c:32, 34, 38`，系统却只保留每组一个、再把 Apple/POSIX/negative handler 改绑到分支 guard 或普通 body 行。最终 7 条引用没有完整支持表格的 8 个实际调用坐标。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260818-022247 | answer_regex,answer_contains | none | 258s | 30 | read=4,repo_map=0,list=0,trace=0,source_lens=0 | midloop=6,inv=3/0,fin_reject=2,unavail=0,prune=0 | pass | B1061 生产正证：explorer 25→12 轮、completion 6→3 次、351→258 秒，`normalizer.Normalize`/`compiler.Compile` 精确调用不再被 compact-relation 文件轴阻塞；终图与列表继续准确表达 `buildAnalysisIR → RunWith ← gate.Run`。两次 finalizer reject 来自独立 B1063：首次 patch 只补 `edge_anchors` 时漏带 replacement block 已有 `claim_uses`，第二次再补才通过。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### B1061 production closure

- 相同 QF case 在 r675/r676 的 explorer 轮次为 `25→12`、investigation-complete 调用为 `6→3`、墙钟为
  `351s→258s`；没有再出现限定调用名因 caller 文件路径不含 receiver 而反复失败。
- 最终 diagram、principal edge list 和 supporting function list 均保留，且明确声明请求的两个端点没有直接
  有向路径，只共享 `RunWith` 终点。`B1061-QUALIFIEDIDENTITYCOMPACTRELATION1` 可生产收账。

### B1062 exact structured location-set loss

- C 模型提交的结构化表格已经给出精确位置集合，citation pool 也包含全部实际调用行；这不是“模型没有证据”。
- 旧单位置解析把非连续 `file:15, 17` 压成第一行，后续 generic label/claim repair 又把错位项迁到
  `_WIN32/__APPLE__/else` guard、函数定义或普通 handler body。最终引用与模型自己的位置列不一致。
- 根修应只在一个结构化 cell 的完整值是 `file:line[, line...]` 时，将它绑定到同一已提交池中的完整引用集合。
  不从请求、摘要、item prose、API 词或最终答案推断，不新增证据，不改变事实或表格内容；位置缺失时清掉
  机械猜测的邻近引用，保持诚实未引用。

### B1063 patch replacement preservation

- 首个 reject 要求给既有 `s1` 增加 `edge_anchors`。模型使用 `replace_blocks` 时复制了 items/facets，却漏掉
  原块 `claim_uses`；replace 语义于是删除它，触发第二次同类 reject。
- 这是修补提示/patch 心智负担问题，不影响最终正确性。先以 P2 观察：后续可让 typed repair payload 显式携带
  “完整旧块 + 唯一新增字段”，或提供字段级 delta；不得把 replace 静默改成 merge，避免无法删除旧字段。

### Red-line review

- 本轮无 Trace 查询/实现改动；显式时间窗、Trace 因果投影、自动补齐、链上-only 主因和双轴结论均保持。
- 两例都持续生成并正常完成，没有固定 4ms 无完整答案降级；系统未替模型写图、关系或结论。
