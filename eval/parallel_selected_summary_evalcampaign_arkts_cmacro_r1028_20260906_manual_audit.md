# Selected Eval Manual Audit Scaffold

- date: 2026-09-07T06:34:11Z
- sweep_start_ts: 20260906-233409
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_c_xmacro_table | PASS | eval/results/sr_c_xmacro_table-20260906-233412 | answer_regex,answer_contains | none | 128s | 28 | read=8,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=3/0,fin_reject=0,unavail=0,prune=0 | 主体通过，引用质量待改 | 五行与两次宏展开正确；机制正文未绑引用导致11条未使用引用清理；无图合理 |
| 1 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260906-233412 | typed_inventory_rowset,answer_contains | none | 299s | 29 | read=0,repo_map=2,list=1,trace=0,source_lens=2 | midloop=1,inv=3/0,fin_reject=0,unavail=0,prune=0 | 六条清单通过，正文有瑕疵 | 合并表触发oracle误分组；内部枚举泄漏/成员方法误称顶层；B1587阶段建议冲突及B1586c重复调度仍成立 |

## 人工审计（冻结 d67e3b2c94cb，原结果不改）

- ArkTS答案 `.codrax/output/20260906-233908.778-39858.md`：逐文件确认Entry四项Index/ParentComponent/StyledPage/ListPage，Builder两项defaultHeader/GlobalCard，六来源正确。排除BuilderParam/仅Component/无Entry的EntryAbility。defaultHeader在CardComponent内，不是顶层；`source_class=thirdparty`与顶层误称均来自原emit。无系统替写、无图需求。
- `runner_lib.sh:1013`先按section_label子串选表：含Entry+Builder的合并标题命中Builder导致计数6，逐行分类明明为4+2。Entry的完整label未命中，反而行marker得4。保留原FAIL，不改精确成员要求或答案格式迁就oracle。
- all.log约625–649：repo_map file-only拒绝教recursive=true，模型照做被analyzer要求recursive=false硬拒，立B1587。共享机械交接1907及1951–54已完整带双marker，B1586a真实生效；三次materialization均保存4+2，B1586c继续开放。首轮analyzer输出未emit_analysis为模型工具遗漏，非活跃流被4分钟截止；日志持续字节/语义后正常结束。
- C答案 `.codrax/output/20260906-233616.525-39869.md`：cmds.def:6–10五行ping/echo/stat/sleep/quit与0/1/0/1/0正确，dispatch.c:6–8声明、12–16初始化正确，运行时handler调用与预处理展开区分。先铺文件分层再答问题、略去arity分支是质量改进方向；未断言绕过参数校验。
- C日志3066原payload机制prose无item evidence_ids，故21引用只保留10个行/handler引用，不是系统删掉11段答案。2335/2521附近canonical教学已明示引用池不等于全局支持，先留模型绑定遗漏，不增加正文关键词门。第二阶段重读的旧文件摘要在1580–1640已截短，不盲记无效重复；首次completion DOWNGRADED未入reject计数归既有B777。
- 下一高ROI是B1587工具建议符合阶段权限，不回放同题求绿；之后r1029具体窗Trace和跨仓TypeScript写两路。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
