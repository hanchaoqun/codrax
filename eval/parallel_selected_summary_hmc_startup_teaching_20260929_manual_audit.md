# HMC §190：启动源区间与任务相关教学人工审计

- date: 2026-09-29T07:43:49Z
- sweep_start_ts: 20260929-004348
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定36755，两例并行×1，runner正式exit0；机器0/2、完整人工0/2，没有第三次追跑。生产快照a78ed6086399-dirty，build time为2026-09-29T07:42:26Z；dirty仅当时文档，不含未提交Go/build输入。未修改旧问题/机器oracle，也不把本轮失败改签。

此后`25a1b07c5`只迁移实际消息测试；`cb57f3a1d`另修发布前发现的转义长名称预算兼容回归，公开编码RED→真实转换/查询GREEN及相邻race见统一账本§190.7。两例live未覆盖该长度边界，不改写成“末版全部生产分支已真实验收”。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260929-004349 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 170s | 41 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=3,inv=1/0,fin_reject=2,unavail=0,prune=0 | FAIL | 13条及源区间全到场；格式修补将已有启动表整体换成summary，最终缺表；系统五行正确 |
| 2 | trace_resource_identity_inventory | FAIL | eval/results/trace_resource_identity_inventory-20260929-004349 | basic_output（静态发现；实际继承primary/log oracle） | perf_triage+trace_query | 188s | 37 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 精确身份和5+5正确，仍无据推断地址无效、完全释放、4KiB页及映射失败 |

## 1. 启动源记录：实现命中，但修订丢失答案

默认SQLite接入命中；explorer仅查询event_search与span_window各一次，均指定1.000–1.080秒，无源码读取。真实finalizer唯一库存13行=8启动端点+5系统事件；8个端点均携带本批source.row_id/owner_ipid/start_ns/end_ns/duration_ns。rowid2/3/4/5同ipid1分别保留1.010–1.018、1.022–1.026、1.030–1.034、1.040–1.045秒与8/4/4/5ms；NULL与未解析业务名称也在场。没有用相邻事件补造区间。

实际问题发生在通用成文修补路径，不是本批字段未传入：

1. 初次emit有两个table；第一个ID叫summary-1但kind是table，承载四条启动记录，没有columns；第二个ID为table-2，承载系统事件。校验明确拒绝缺summary。
2. 第一次patch试图add summary-1并保留不存在的table-1，精确ID检查拒绝，未修改基底。
3. 第二次patch用summary替换原summary-1的整个table，只保留table-2，四条启动记录随替换丢失。该修补满足形状合同，被接受；随后维度提示仍未促使模型恢复表格。
4. 最终原始Markdown、primary及HTML一致只剩系统事件表，摘要却仍称“以下两张表”。缺LoadPreferences/8ms/5ms这次是真实内容丢失，不能沿用历史“单位在表头”误报解释。

系统表五个时刻1.012/1.046/1.050/1.052/1.058及内容正确，未知域/名没有冒充已知；初稿把未知启动名写成startup、正文把线程展示名当进程名仍需保留审计边界。两次尝试不是渲染器静默删除：完整替换是模型提交的操作。按01.3/16.4/18.4提升“修订意图与数据载体保真”优先级，下一方案须围绕精确block ID/kind/载荷/维度绑定、追加缺块与改已有块的区别；不按ID词面猜kind、不扫模型正文、不无条件禁止合法整块改写，也不由系统代写答案。

## 2. 资源身份：原值到场不等于解释正确

一次trace_mark查询，10个展示行无遗漏，5次I操作与5次C采样正确分开。大整数、signed/hex、子类7/0/8/99/NULL及中文管道名称保持精确，HeapSize 8192→4096→4128、MmapSize4096→8192顺序正确，未再虚构Mmap零起点。但完整答案仍FAIL：

- 用未证明的平台地址空间把大整数判为异常/测试注入，将负数位型认定无效地址、映射失败/内核占位；源仅证明原始位型，不证明这些解释。
- FreeEvent地址0且end_ts_ns0被解释为“已完全释放”；这是把原始字段值升级成资源生命周期完成判定。
- 把4096、8192称为1/2页4KiB，并把数量32直接叫字节；此载体并未证明单位、页大小或操作与计数器变化之间的一一因果关系。
- 名称列将已知空字符串写为“（空）”，其余NULL和没有提供名称统一成“未记录”；机器空字符串措辞检查存在表达限制，但人工失败并不依赖它。缺子类编号实际仅末条，开头“大量缺失”也夸大。

分析阶段第一次把trace里的线程冒称用户指定目标，既有精确引用校验实际拒绝并修复为no_named_target；不能把已纠正尝试记成最终授权污染。分析rationale的“8条资源操作”仍错，但实际finalizer初始消息未发现该句，最终正确为5次；不武断把它定为最终幻觉根因。仍有Harmony优先级旁注到场，按整体噪声余项记录。没有因本轮结果为特殊地址或某事件追加专属教学。

## 3. 上下文测量、根因边界与图

以下为真实INIT消息/调用日志，与§189比较；字节不是计费token，单次观察不证明降噪直接提升准确率。

| 指标 | 启动：§189→§190 | 资源：§189→§190 |
| --- | ---: | ---: |
| system字节 | 114481→85816 | 113165→88240 |
| Workflow字节 | 71188→42523 | 69872→44947 |
| 动态user字节 | 96038→95646 | 75767→68001 |
| Runtime Trace Answer Guidance字节 | 7531→4183 | 8523→5175 |
| 初次finalizer估算tokens | 63419→55554 | 57576→49258 |
| 共享展示行/其JSON字节 | 16/22932→13/22936 | 10/15623→10/15623 |

两例实际system均不再包含TRACE ANSWER SKELETON、ROOT-CAUSE BOARD ORDER及纯调度等待教学，精确字段/来源/范围仍在；源区间增加而唯一启动展示行减少，是查询范围/数量改变，不当作删除证据收益。请求数8→2、2→1仅为单次轨迹，不承诺稳定性能倍数。因果问题、明确窗、retry和legacy保留路径以公开实际adapter回归验收，不拿这两例有限清单代替因果live。

两份root-causes.json均schema2、空列表、trace_root_cause_contract_not_active；没有提升链外事件为根因。两份答案均为表格，无关系图；核对Markdown/HTML文本和表格内容，没有截图视觉验收，不宣称全图类型验收。实际LLM日志仍timeout=10m、first_byte_timeout=10m、stall_timeout=5m；未缩短活跃流等待。

## 4. 原始证据

| 文件 | SHA-256 |
| --- | --- |
| 启动run-1.primary.md | ad18d418b3d5eced1e5b2ff93684a95d2c00342c1d885918aafe3d5283ce03c5 |
| 启动run-1.logs.all.log | 44d00f87e2ab4df57b96d50523cf1bda1adc7eb1b5ad63fed4771b2843930d5a |
| 资源run-1.primary.md | f05a7bd7ca0e005f0fc1be58122491e5c048f3434e76c6977721adee4daa2583 |
| 资源run-1.logs.all.log | 9a2231edab6dfbd69bc4b6203e0319945af9051f36eb57c7b69619635dc41e10 |

完整工件`.codrax/output/20260929-004636.232-23162.{md,html,root-causes.json}`、`.codrax/output/20260929-004655.458-23161.{md,html,root-causes.json}`保留。稳定任务仍79=16已交付+63开放，五个稳定验收父项不销账；本批两份完整人工FAIL独立记录，不重复增加父任务。
