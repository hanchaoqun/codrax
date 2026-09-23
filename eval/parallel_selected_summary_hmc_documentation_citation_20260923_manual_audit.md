# Selected Eval Manual Audit

- date: 2026-09-23T05:08:48Z
- sweep_start_ts: 20260922-220848
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_documentation_citation_20260923

本批冻结构建`4e0243e08b0a`，恰好2并行、每例1次；机器1/2通过，主审与独立完整人工审计0/2通过。原始报告/日志/oracle均不改写，未追加第三例追绿。下表使用runner墙钟（case自身摘要分别为285/396秒，不混作同一计时口径）。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | empty_python_module_apply | FAIL | eval/results/hmc_documentation_citation_20260923/empty_python_module_apply-20260922-220848 | write_apply,write_patch_oracle,answer_contains | none | 287s | 28 | read=8,repo_map=2,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL（功能/交付PASS） | 仅totals.py实现，原4个native测试真绿、测试原字节不变。无源码改动补证计划有来源谱系却无共享交付身份，正确PTO被拒后不能生成当前原生执行收据；最终诚实unverified。 |
| 1 | trace_capability_discovery | PASS | eval/results/hmc_documentation_citation_20260923/trace_capability_discovery-20260922-220848 | log_regex,primary_answer | none | 398s | 30 | read=0,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=7/0,fin_reject=0,unavail=0,prune=0 | FAIL | 完整目录到达Finalizer且零伪引用，查询名/抢占等待/采样权重已改善；单位与缺测条件仍有事实错误。非源码说明尚无独立完成通道，重复调查成本确定存在。 |

## 完整答案与过程结论

### 能力目录咨询

Analyzer首请求包含本批宿主/目标仓教学，第一轮仍多余repo_map，随后已明确纠正为宿主目录、intent=explain。Explorer真实取完整45902字节目录（21视图/41计量语义族/8输入格式），却尝试伪发16条source，系统拒绝正确。完成阶段的5个function_or_purpose席位仍要求源码producer/consumer/branch操作，运行时waiver因无附件被忽略亦正确。连续降级后又拆三个源码调查子分支（最多2个并行），重复取目录；最终16轮Explorer、4次目录调用（3完整/1概览）、5次全部被拒的emit_evidence、7次completion，不是超时或流活跃期间降级。

本批交接实际命中：日志3196–3236的Finalizer独立Tool Documentation节保留完整detail与compact两份去重JSON，`static_only=true/evidence=false/capture_availability=not_evaluated`；files_read/evidence_items/observations均为0。最终5块、0引用、无伪源码附录，回答包含window_stats/scheduler_latency_stats/perf_stats，正确区分wake-to-run与preempt-to-run，明确采样权重不等于CPU时间。这证明承载修复生效，不证明模型全文正确，也没有命中“最终伪引用被删除”的runtime分支（该分支由公开正反测试验收）。无实际Trace附件/计量查询/因果合同，不能据此验收Trace因果投影、根因旁路或图关系。

完整人工仍FAIL，主要有充分目录上下文下的事实误述：

- primary第11行把file_io“IO计数、总延迟、最大延迟”全归ms；目录明确count/completion_count为count、bytes为bytes、两个延迟为ms。
- 第9/18/34行把block rq/bio配对说成全部IO分析必需，并把缺某一事件族泛化成全部分位不可算；目录允许符合精确身份的filesystem/storage层各自完整配对，不能将独立来源用AND收紧。
- 第65行缺sched_wakeup/sched_waking却说仍能看到waker edge；目录要求其中至少一种，没有提供替代凭证。
- 第66行缺sched_blocked_reason却称blocked_reasons仍有计数；无事件不能虚构事件计数。
- 第97行缺sched_switch却称“调度侧指标仍然有效”，与目录及自己的前文矛盾。

本轮不能整体归为模型波动：单位/缺测误述发生在充分合同到达之后，单列模型解释债；重复调查则是已证系统表达缺口。现有request字段不能承载“只要求工具说明”，自由文本理由已正确但compiler默认file_line/operation席位与structural-empty仍只识别原源码/运行时通道。应以正交typed请求说明范围+本轮成功文档的共同准入补齐通用说明完成通道，不伪造外部来源、不把所有非证据工具当调查完成、不放宽混合源码/Trace根因要求。沿HMC-01.3/16.4/18.4留账。

### 已有空Python模块写入

交付commit `51a3eab0cf9a1d83a518f4330f911d9ae58a0581`仅totals.py新增7物理行，SHA256 `0cae86e789ea5437664bd3a3d08eca47b4c79df1b1ee2f4bf48c50930b1f7003`；原tests/test_totals.py在fixture/seed/交付/工作树均为`95143baf97adf98c6343795287cf6cab334533ddc5b34bea7e1887067c1e5556`，tests/__init__.py和README也未变，无配置/依赖改动。worktree HEAD是交付commit且clean，主仓保持seed（框架另生成未跟踪.gitignore，未进入交付），materialization只选实际源码计划。实现通过sum(values)处理整数、空输入及一次性迭代器；实际4原生测试通过，后来5项本地检查是1probe+4native，不是5个原生测试。

旧修复真实命中：空文件读取为0行/0字节，两次create_path_exists修补明确空文件用insert_at_eof，不再误教micro不允许的modify。旧未销债也再次命中：初计划把PTO写成unittest@tests::TotalTest，真实身份是tests.test_totals.TotalTest；无源码改动补证计划后来填对PTO仍被现有门拒绝，删掉PTO后probe与native测试绿，但该计划没有自己的AppliedCommitSHA/PatchEffect，共享交付身份解析尚缺，不能生成新的existing_test_executions。cumulative_verification_scope确有source_plan_ids，不能误报“谱系完全丢失”，也不能沿用旧成功回执补签新执行。

最终`required_existing_test_not_executed/unavailable`及unverified是完整证明未闭合，终稿诚实说明局部检查通过但不能标记已验证，没有误销账。整体FAIL继续归HMC-18.5原§149/153.1，不把功能PASS代签。定位条目虽出现totals.py:8而现存源码仅7行，补审确认原计划只给合法insert_at_eof，并未声称已读第8行；应用后planEditOwnerLineCandidates按当前末尾换行重算EOF候选，FindEnclosingOwner返回附近的total结构，供路径owner-localization使用。本例另有真实diff第1–7行anchors，未见`:8`独自放行或进入ReadCoverage/citation，不能据此认定虚构源码证明。EOF坐标与物理行共用LineStart/LineEnd的语义债另留公开反例核验，不计新确定回归，也不代销旧空基线义务。首次write_analyzer将数组编码成字符串的一次重试已恢复，暂不为该单次编码再加专用硬门。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
