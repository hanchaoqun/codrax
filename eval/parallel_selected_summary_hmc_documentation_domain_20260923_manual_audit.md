# Selected Eval Manual Audit Scaffold

- date: 2026-09-23T11:50:15Z
- sweep_start_ts: 20260923-045012
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_documentation_domain_20260923

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_capability_and_window_analysis | FAIL | eval/results/hmc_documentation_domain_20260923/trace_capability_and_window_analysis-20260923-045015 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | missing_runtime_authority | 86s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 附件已就绪，整轮却被分到operation；没有进入Trace调查，最终误称附件缺失。未验到mixed文档完成通道。 |
| 1 | trace_capability_discovery | PASS | eval/results/hmc_documentation_domain_20260923/trace_capability_discovery-20260923-045015 | log_regex,primary_answer | none | 139s | 28 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 纯说明真实完成且无伪源码引用，但单位/压缩输入承诺泛化过度；仍附不适用的源码定位降级说明。 |

## 完整终审：机器1/2，人工0/2

固定2并行×1，52356正式exit0；外层139/86秒与pure内部137秒不混用。构建来自`b7b808262930`，Go输入干净，构建时仅文档未提交导致revision带dirty；不是全树干净发布收据。7966是此前launcher在启动LLM前拒绝未提交Go输入的exit2，不计第三个模型样本。未重跑追绿，原机器判定及答案不回写。

### 纯工具说明

主审通读`run-1.primary.md`、`run-1.out`、answer-surfaces及实际工具日志。模型正确声明only；04:50:51读取11,395字节简目录，04:51:19读取45,902字节完整说明，04:51:52真实接受完成，04:52:32成文一次成功。中间把7条目录项当源码证据提交，被现有证据校验全部拒绝；没有放宽证据门。新说明完成凭证、完整文档保留及无源码引用成文真实命中，无采集/因果/源码测量冒充，不需要图。

仍有完整答案问题：

- “所有视图…统一毫秒”过宽，完整目录已逐字段声明原生duration_ns、reported_duration_ns仍为ns；所问常见IO/调度耗时确为ms，不能扩成所有视图。
- 将gzip和ZIP概括成任意受支持文本/原生成员可自动处理，漏掉ZIP仅唯一合格常规`.sys/.htrace`成员、格式/完整性/安全边界。原生转换的magic/version/provider限制本身有说明，不把整段判为完全缺失。
- 已知边界自行断言系统不会披露具体缺失事件，目录只要求检查实际coverage/可选字段，不能从静态未测量推导该全局否定。`cohort.weight_unit`、conditional_conversion等内部表达及5次重复标题降低可读性；不以关键词硬门改写答案。
- 确定性系统接缝：实际完成后04:51:52日志仍报`followup_coverage`缺task_map/file_map/semantic_subgraph；最终控制台补充说明因此称定位/钻取未执行。纯说明没有当前源码义务，这个源检查不适用。应消费同一份纯说明、当前真实接受凭证，不能把mixed/未完成也跳过。

已正确区分采样count和同事件/单位cohort权重、查询秒/纳秒换算、未知与实测零。模型在完整上下文上的过度概括继续留01.3/16.4/18.4，不声称已证为随机波动；先修确定的系统接缝，不追加单题重试或长提示拟合。

### 工具说明＋实际业务窗

独立审查与主审均确认：2408字节附件已加载，路径正确，模型预览有界但查询材料完整。`run-1.logs.all.log:57`的single-shot classifier却将整轮选为operation，理由仅覆盖第一部分。之后三轮操作计划、两次错误find，没有trace_capabilities、trace_query、analyze或finalizer；最后因stub_repo中没有Trace而要求重新提供附件。无报告/answer-surface/root-cause投影，机器FAIL正确。

因此本例没有测到35ms请求、31ms工作线程S态IO等待及1ms唤醒后调度；47ms后台备份也未进入因果判断。不将“没有错误归因”当PASS，更不能宣称mixed/显式窗通道已经live验收。

共享路由教学接缝挂01.3/16.4：schema及repo路由已经允许log/trace/MCP调查，但operation词典仍把investigate限成fresh code/repo reads，公开宿主目录没有明确能力归属。修法是统一静态说明与真实观测调查的pipeline边界，强调完整组合任务；不按附件存在或原文关键词强转，因为真正文件/机器操作仍可合法使用operation。原fixture和机器判定保留。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
