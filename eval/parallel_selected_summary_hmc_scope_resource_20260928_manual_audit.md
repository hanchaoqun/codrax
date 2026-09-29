# HMC §189：资源身份与查询范围人工审计

- date: 2026-09-29T06:52:17Z
- sweep_start_ts: 20260928-235216
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定25451，两例并行×1，正式exit0；runner正常收尾不代表答案通过。生产快照`f31a5b5eeda2`，build time `2026-09-29T06:51:55Z`。机器0/2，完整人工0/2；没有第三次追跑，不回写旧失败。03.2仍待验收，稳定任务数仍79=16交付+63开放。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260928-235217 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 218s | 46 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 窗内13条完整到场；正文5/7混计，系统事件错时/重复，未知启动名未说明 |
| 1 | trace_resource_identity_inventory | FAIL | eval/results/trace_resource_identity_inventory-20260928-235217 | basic_output（静态发现标签，实际继承了primary/log检查） | perf_triage+trace_query | 233s | 37 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 8个资源字段/5+5记录完整；正文推断有效地址、匿名映射、空子类含义及虚构零起点 |

## 1. 资源：精确原值通过，完整解释失败

自然问题不向用户暴露防误判条款；只有窗口、明细字段、各类总量变化和“能说明什么问题”。沿用旧fixture及其独立oracle，不把README/预期答案给模型。5个I操作及5个C采样正确分开，0.006秒窗外FD未计入最终表；五组signed/hex、0/NULL/空名称/缺名称数值列对应正确，中文管道名称正确转义。两份相同查询收据共享10个完整展示行，omitted=0；最终日志2730–2742明确保留8个资源字段、精确整数、每行来源/时间和NULL原因。实际选择器为0.000450–0.005550，独立请求窗说明0.000500–0.005500仍在，观察包络0.001–0.005不冒充查找窗。

完整答案仍不能通过：

- 开头把0x0020000000000001称为“有效地址”，随后把-1/all-one称为“地址尚未解析”；源只证明精确位型，不证明有效性/解析失败。
- 把子类0的已知空字符串解释为“不关联特定子类”，把未知映射名称变成匿名内存，并推测初始化阶段行为，均无独立证据。
- 概述称MmapSize从0升至8192，但首个观察为4096；正文又给正确4096→8192，内部矛盾。HeapSize首末8192→4128净变-4064，却将最后一步+32说成窗口净增长。
- source_heap_size单位未获证明，被改写为“确定不是字节/不同量纲”；未知单位不是排除字节的证据。两次映射的数量也不能被解释成已经证明的匿名申请字节数。
- 窗内0.005秒地址是已知最小signed值，尾部仍把它混入地址缺失说明。表格与正文互相冲突。

机器FAIL仅因未出现执行/泄漏/根因边界词，不能把这一措辞检查当人工失败理由；上述值/语义错误独立成立。新case通过shell source复用旧case，实际run.sh已执行继承的primary/log oracle（verdict可以验证），但静态declared-oracle发现只标basic_output。保留原summary标签，登记为18.5评测元数据发现边界；不为改标签重跑或改变原判。

## 2. 启动数据库：新范围生效，事件角色与计数仍错

默认闭合SQLite接入、合法trace_mark/hi_sysevent查询、无源码读取均命中。7份event_search收据加1次span_window查询；finalizer日志3000–3016中，宽窄查询独立，窗内全体13条=8启动端点+5系统事件，hi_sysevent子查询精确5条。16个共享展示对象还保留3条窗外上下文，无成员遗漏。7个prompt_metadata_omission均只省略同一21713字节caveats数组，查询、coverage、row_refs与业务字段不缺失。旧扩窗探索未被禁用。

独立oracle是四段启动8/4/4/5ms；五条系统事件在1.012、1.046、1.050、1.052、1.058秒。答案四段时间表正确，却概述“5个阶段、7条系统事件全部在窗内”；系统表把STAGE_READY从1.012误移到1.010，又在1.018凭空复制BOOTSTRAP（真实为1.058），得到6行。NULL/歧义启动名称仍写成真实startup段，未解释业务名称未知；过程主体只从展示线程名提升为应用名，尚未得到独立主体关系交接证明。

原8/5ms机器正则仍不接受单位位于表头，属于已知误报，原判照存；人工错误并不依赖它。与上一批相比漏BOOTSTRAP问题转成错误复制/时刻，不能说问题已完全解决，也不能从单次变化证明纯模型波动。未再出现前10ms“无任何事件”的同一表述，但这不足以验收所有范围解释。

## 3. 实际上下文审计与下一系统修复

最终输入完整保留字段和新scan_scope；estimated context tokens为资源57576、启动63419（调用日志值，不当真实计费量）。仅10/16个展示行仍收到大量通用调度、根因、源码关系教学、重复查询备注/修复卡；模型已有错误中间汇总，不能让它因有support_refs便自动升级为同源字段证明。实际finalizer已省略Structured Aggregate Facts并明确直接观察优先，因此**不能武断归因为最终阶段直接照抄了错误aggregate**；影响路径还需以消息构造和引用绑定做公开对照。

下一高ROI缺陷轨应量化消息块，按已声明问题/证据能力投递相关教学，移除同一规则竞争表述；在源→字段→成员→表格/计算之间保精确查询身份和行绑定，不扫描用户问题/模型正文，不用全部root-cause教学覆盖单纯库存问题。unknown“不知道”与确定否定也需统一字段合同。保持所有链上因果、业务线索与扩窗能力；不继续只为-1或某个事件名叠专属提示。03.2解释验收留原账，先推进完整启动实例/普通viewer兼容等参考能力。

两份root-causes.json均schema2、空列表、trace_root_cause_contract_not_active；没有把这些资源/系统事件选成链上根因。Markdown/HTML为表格，无关系图；核对内容与表格结构，未进行截图视觉验收，不宣称全图类型验收。真实LLM请求仍为first_byte_timeout=10m、stall_timeout=5m、timeout=10m，本批没有改短或引入活跃流提前降级。

## 4. 原始证据与摘要

| 文件 | SHA-256 |
| --- | --- |
| 资源run-1.primary.md | `9f6d79338083299c6a8777f50a5e2d05a497fc3824eb21e495dcfffe78534e51` |
| 资源run-1.logs.all.log | `02e4df01d67fe96c7479c3cb6ecdff51ae0a4ae958c72cbe537dbc3fe7b55057` |
| 启动run-1.primary.md | `cf8edfa2b15a9059c1bb7510ae894c2df1ed140cf31192fd65b44b8194eaeed3` |
| 启动run-1.logs.all.log | `80aea8deb36b81b7de531be786314e5ef1b2feeb5366c7590b92085fdbcd1995` |

完整生成工件：`.codrax/output/20260928-235607.674-92818.{md,html,root-causes.json}`和`.codrax/output/20260928-235552.458-92819.{md,html,root-causes.json}`。原文件不覆盖，不把本次FAIL改写成PASS。
