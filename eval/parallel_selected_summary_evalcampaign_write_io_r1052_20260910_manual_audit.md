# r1052 写模式与显式窗 IO 人工审计

- date: 2026-09-10T09:42:38Z
- sweep_start_ts: 20260910-024237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

基线：干净构建 `2ca6a4947116`，不可变快照 `codrax-selected-20260910-024237`。严格并发两路，原case/oracle/预算不改。机器1 PASS/1 FAIL；人工两项均有实质错误，不记全绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_tokenizers_newline_run_multirepo_py | PASS | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260910-024238 | log_regex,write_apply,answer_regex,answer_contains | none | 249s | 28 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 持久交付与原两项测试真实通过；单换行仍被错误折叠，且会误触后续合并。不能凭required合同闭合推定全行为正确。 |
| 2 | real_trace_h3_iofam_one_seat | FAIL | eval/results/real_trace_h3_iofam_one_seat-20260910-024238 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 319s | 48 | read=12,repo_map=0,list=0,trace=8,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | 机器仅格式regex未命中；人工另有口径/证明/机制归属错误。关键1.347/1.337/4.384及隐藏190项未丢，默认空根因旁路正常。 |

## Python：实际修改与验证范围

- `run-1.write-apply.json` 同计划 `plan-1789033557594077000-58488` 的 `post_apply_verify` passed，final complete/verified；不是计划阶段旧探针冒充应用后验证。仅 `bindings-py/fastlex/tokenizer.py` 产生产品改动，原五换行测试与期望保留。
- 本轮明确子仓内相对路径教学已实际入模（初log437–440、610–613）；后续read实际用 `fastlex/tokenizer.py` 等，不重现r1035重复 `bindings-py/` 前缀。
- 在持久 `run-1.applied-tree` 复验，Python3.9.6、native扩展不可用，明确只证明fallback。原两项unittest通过：`.codrax/tmp/20260910-r1052-python-native-tests.log`。
- 独立原生行为检查见 `.codrax/tmp/20260910-r1052-python-boundaries.py` / `...-independent-boundaries.log`：单LF得到 `[300]`，应保留 `[10]`；单LF+x进一步得到 `[301]`，应为 `[10,120]`。2/3/5/6 LF整体折叠、无规则、普通hi/aaaaa、最低rank、UTF-8及多LF折叠后继续BPE对照通过。
- 根复读产品代码，错误来自预处理只判断 `ids[i]==10`，未要求至少一对换行。probe前两条assert确验五LF与hi；第三条虽注释“无规则”，实际仍带LF规则且仅print输出，无assert。不得把模型的 `PASS:` 打印当成独立行为证明。
- 正式计划只有 `existing-merge-behavior` 为required、精确比较器为hi→256；另8项为planning-only。最终交付卡明确只对批次及必需结构化义务签已验证，不宣称自然语言清单逐项执行。本轮单LF不属于该required比较器，沿既有B1221裁定区分模型补丁/探针边界遗漏与证明域，不据自然语言扫描新造硬合同。B1561原生逐assertion等旧债不因这次required闭合而销账。

## H3：正确部分与人工错误分开记账

- 最终工件：`.codrax/output/20260910-024755.466-58451.md` / `.html`。八次trace_query均用原目标和13762.791708..13763.024898（233.190ms），没有实际跨窗查询。该题是有限IO指标对照，不强制因果投影；同名 `.root-causes.json` 131bytes，schema_version=2、root_causes=[]、status=unavailable、reason_code=trace_root_cause_contract_not_active，必选旁路正常。
- 核心正证：单请求驻留1.347ms、对应S型完成闭合阻塞1.337ms、四段0.782/1.027/1.238/1.337的并集下界4.384ms。6条明确写“已渲染请求”不升总数；隐藏190项41.329 request·ms的归属较r1013正确，不再错抄请求结束为查询窗结束。
- 机器唯一失败是 `41.329.*(非墙钟|不可相加|non.wall.clock|non.additive)` 的同一行词序：正文前置标题已经写非墙钟，属于格式敏感oracle失配。本次不改oracle刷绿；人工fail另有独立理由。
- 模型主体仍有错误：把调度标记IO清单的0.000ms时长称为非墙钟；将D/IO分列口径改述为D且IO；对request·ms作“不可取平均”的绝对禁止（同一请求集合/相应分母的平均请求耗时有意义）；把Binder等IPC等待归为目标IO主要组成；把来自 `io_latency_overflow_request_ms` 的41.329误挂为 `storage_latency_by_layer` 分组sum。
- 两个“是否唤醒发送方=否”仅据 `completion_woke_issuer=false`。该字段按 `internal/tracequery/types.go` 定义在absence/ambiguity时也为false，不能推出事实上的未唤醒。新B1644只补入模证明布尔边界，保留算法/API与模型原文，不给缺证据请求增加根因资格。
- 最终上下文log3731–3775已经分别提供请求/线程阻塞/全局隐藏请求三尺、target_owned与背景、独立Binder非根因边界；原始emit log4005的2334字符主体完整保留于MD17–64，没有被系统改写。系统只附MD66–68目标调度清单。其它整理错误先保留模型精度观察，不用prose硬门或改答案修复。

## 过程、旧裁定与后续

- 一轮 `emit_analysis` 同时填 `requested_scope=full_artifact` 与非零时间端点，为模型字段组合不一致；WS1既裁允许full_artifact清除多余time，不可类推bounded_selector把它自动升级explicit window。数值查询窗本次仍正确，附注“探索子范围”反映该typed分类，不冒称数据被扩窗。可另研究schema组合教学，不以此案例改scope所有权。
- log1093–1098拒绝读取派生grep-full并给返回原JSON导航，1219–1220原结果读取成功，符合B1624b“纯导航不放行派生”边界；不是新权限合同冲突。外部emit_evidence被跳过为外部观测，模型随后用completion reason/aggregates承接，未抬成源码证据。
- 两路无超时/活跃流年龄降级；Trace首次成文成功、零finalizer拒绝/patch，非JSON丢答案。独立4ms/旧总cap活跃流保护专项收据见统一文档§123.1731。
- B1640教学的新二进制已使用，但本轮不重复r1051那两条schema冲突，不将“没撞到”冒称因果证明；B1641/B1642新关系/引用分支未在本批触发。先收B1643通用多关系预算及B1644证明布尔上下文，再轮转异构关系、显式窗根因与其它write域；不因单case弱模型失误无限堆硬约束。
