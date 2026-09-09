# r1045 人工审计：来源隔离复验与异构方法顺序（2026-09-09）

## 基线与覆盖选择

- main已推送`55d497b92`；clean binary `0.1.20260909`，built `2026-09-09T07:08:40Z`，revision `55d497b92196`。
- 本机243例：215 read、25 apply、3 plan。按客户风险、直接影响面与覆盖老化选H4显式窗状态/频率读和距r1037八批的Python cooperative MRO读。近期r1042 Python write、r1044 C++ write已覆盖真实apply，不为同题求绿挤掉异构轮换。
- `20260909-000921`开始，`PARALLEL=2 / TIMEOUT=1200`，只有这两路live。case、oracle、fixture及历史输出不改。机器汇总见同名非manual文件；机器判据不代替人工正确性。

| case | 机器 | 人工 | runner时长 / context | 实际过程 |
|---|---|---|---|---|
| real_trace_h4_supply_thermal_witness | FAIL | fail（核心数值/限制边界正确，仍有信息交接和过称） | 119s / 32% | 1次精确trace_query；首次成文接受，随后1次展示补丁被拒，原正文保留 |
| sr_py_mro_order | PASS | fail（handle顺序正确，完整MRO漏ABC） | 139s / 30% | read=6、repo_map=1；成文零拒绝，1次展示补丁成功 |

## H4：测量和来源正证，不掩盖残余

最终文件`.codrax/output/20260909-001118.415-99829.md`；日志`eval/results/real_trace_h4_supply_thermal_witness-20260909-000921/run-1.logs/codrax-20260909-000923-000-99829.log`。

1. 查询准确命中`13762.791708..13763.024898`、目标17267，只有一次window_stats（日志866、879–880）。正文完整保留233.190ms窗、Running157.248、Runnable5.604、Sleep70.338、D0.000ms及8CPU明细（96.081/35.960/11.030/7.155/4.479/2.221/.184/.138ms）。没有两核小计冒充总量。
2. B1630c的16/28条库存与单条代表上下限区别实际进入上下文（1713–1715、1784–1785），正文未把总数说成该档次数；但未复述计数，只记上下文正证。B1631两处关联带同结果范围（1716–1730、1786–1794），CPU4代表558000与策略max2100000分开；CPU12/7等不借CPU0/4策略。最终明确缺乏目标切片与策略的时间重叠或绑定证据，机器regex未匹配，保原FAIL，人工认为此边界已表达。单查询live不冒充跨源live正证；跨捕获/过滤域错误由真实Execute→双成文入口先红后绿针证明。
3. **B1633a/P1信息交接确认**：全线程`window_stats.top_running`只有17267的CPU12/4入榜，CPU7目标11.030ms来自完整running_by_cpu。原JSON `cpu_occupancy.per_cpu_top[6].top[1]`却有17267/CPU7/11.030ms/1280000kHz，现formatter只带线程和时长，typed链未携该频率。不是B1631 strict join丢行。per_cpu_top也是TopN，后续应复用目标域完整的已有聚合，不靠加大TopN或借背景频率，保桶代表值而非全程驻留。
4. **B1633b/P1附注频点域确认**：系统CPU7的558/640/1380MHz来自其他线程不同状态桶（#ActionReaper、sh、CompThread_0），不是17267。`prose_fact_juxtaposition.go`收集各线程RichNotes cpu/freq，却只打印CPU。后续应保来源/线程域，不能拿该旁证回填目标频率或证明策略实际作用。
5. 模型过称：MD18由8核推“频繁迁移”缺频次凭证；MD50“无频率记录”把目标TopN桶缺失扩大成全部频率缺失；MD28/42将D与IO wait混作状态标签；MD33复述内部枚举unproven。既有迁移/范围教学已入模，不为词句造硬门；系统信息交接缺口单独修。MD57保零D/显式iowait不等于所有层级无IO。
6. Binder5次/3.094ms“已计入Sleep”有同目标同窗typed包含证据：5条confirmed occurrence均s_sleep，1.409+.924+.068+.120+.573=3.094ms。65个已构造Sleep区间中的其余60条未关联，不宣称全部Binder闭合。
7. 成文后模型提交未发布`add_facet_id`，1886调用、1889拒绝、1929–1930保留首稿退出。表格缺enumeration_item/逐项EvidenceIDs，原子分支依法不开；动态schema和Execute复用同一候选投影。既有提示是“发布才用，否则完整replace_blocks”，不是必带且必拒。原native schema未完整写日志，不拿模型thinking当发布证据。记P2能力专属教学观察，不先立合同自冲突。重复状态表已在模型原稿，不由系统删除它。
8. 本题有限窗状态/供给事实不强套帧/根因合同。默认旁路131B，`schema_version=2/root_causes=[]/status=unavailable/reason_code=trace_root_cause_contract_not_active`诚实存在，不是实测零根因或投影回归。

## Python：方法执行链不等于完整MRO

最终文件`.codrax/output/20260909-001137.965-99831.md`；日志`eval/results/sr_py_mro_order-20260909-000921/run-1.logs/codrax-20260909-000923-000-99831.log`。

1. MD9/14声称完整C3 MRO却漏ABC；fixture与只读原生`__mro__`复算为`JsonPlugin → TimestampMixin → ValidationMixin → BasePlugin → ABC → object`。BasePlugin(abc.ABC)已入模。错误从首稿3658就在场，3704补丁没有删除它，属模型推断遗漏，机器PASS不能签人工全过。
2. Timestamp→Validation→Base的handle顺序及body缺失抛ValueError正确。原生探针确认两次浅复制、输入不变、返回新dict（固定time123，结果含ingested_at123、processed=True）。正文未讲复制细节，但“注入payload”不能单独证明宣称原地修改；不把未询问的每个细节强制出厂。
3. 无图且用户不要求图，不是图被吞。3671首稿accepted；3677–3685软展示提醒要求member_set，3704只加该facet，3717accepted，四条正文、引用及其他字段不变。fin_reject0/patch1与过程一致。分析阶段另一次合法JSON缺条件必填端点对象，修补后通过，不混计成文拒绝。
4. B1620说明实际标记model_notes/advisory（3331、3340），不继承记录证明权限。完整源码/继承声明可用，没有上下文裁剪或教学要求漏ABC。
5. **B1632/P1系统附注越界确认**：MD32“答案未完整呈现…主路径上的关系”来自validateFacetCoverage只核FacetIDs/ClaimUses.FacetID；正文已有调用顺序和call_edge，未标principal_path_edge仍触发软concern。提示3682自己说明“未确认≠正文确实缺失”，`repair_caveat_materializer.go:167`却断言未呈现。通用修法只中性化12类已知facet的中英显示，保判据/义务/正文，不扫描prose自动补标、不强制图。

## 任务顺序与边界

1. B1631已交付`55d497b92`，全仓86包绿；本轮单查询上下文正证，跨源入口红绿，不混称多源live通过。
2. B1632先做真实coverage→公共发布入口中英红绿：系统只披露尚未确认的核查范围，不断言正文缺失；known facets/混合unknown fallback/去重cap/模型块不变。小批提交推送。
3. B1633a目标每CPU频率供给与B1633b附注线程域分开设计，不能扩大为全程频率、策略持续时间或后台升目标。优先于重复H4求绿。
4. B1624b纯导航谱系、B1629b/B1626/B1622/B1616b/B1561保原队列；Python完整MRO及H4迁移/枚举措辞保模型观察，禁止案例硬拟合。

两路SSE正常但不足4m，不能作超过4m的live证明。B1631已有实际HTTP/SSE、工具/隐藏推理/heartbeat、4ms分帧与旧4m年龄保护count3；真实停滞、显式deadline和取消仍有效。未新增按无答案时长降级。Trace精确窗、链上根因、占用/规则可消双维度、D/IO/语义/业务证据、投影及自动补齐不改；系统不代写正文/图/结论。
