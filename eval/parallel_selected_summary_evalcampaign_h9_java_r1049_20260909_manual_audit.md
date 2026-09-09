# r1049 人工审计：H9 明确窗单基准 Trace / Java 分层调用链

- date: 2026-09-09T12:45:17Z
- sweep_start_ts: 20260909-054517
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

冻结版本 `99e39916318a`，built `2026-09-09T12:44:29Z`。243 个 case 按影响、覆盖老化和关系表达风险轮换；上一批已有真实 C 写，本批严格两路、各一次、1200s，不改 case/oracle/默认步数。机器表原样保存；人审同时核对最终答案、实际入模内容、原始来源和修补过程。r1049 **不包含** 尚在施工的 B1636/B1637。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260909-054517 | primary_answer | none | 148s | 43 | read=10,repo_map=0,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | 主调用路径保留；持久化误称、图消息/条件表达错误；另确认 B1637 系统所有权合同自冲突 |
| 1 | real_trace_h9_conversion_single_basis | PASS | eval/results/real_trace_h9_conversion_single_basis-20260909-054517 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 361s | 55 | read=1,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | 明确窗、单基准和双轴主值正确；正文机理措辞/修订未完全闭合，系统次级显示仍有残余 |

## Java：真实链保留不等于答案正确

工件：`.codrax/output/20260909-054742.936-81568.{md,html}`；日志 `eval/results/sr_java_call_chain-20260909-054517/run-1.logs/codrax-20260909-054518-000-81568.log`。运行仓与原 fixture 源文件 SHA 一致、运行仓状态干净；无源码改动。

1. 原代码：Controller.create 先拒绝 null/isBlank reason，再调用 Service.schedule；schedule 依次取阈值与当前数量，`count >= max` 抛错，否则 insert；insert 只做 `ArrayList.add`，然后 AuditLog.record 调 `System.out.println`，最后返回 rows.size。没有数据库持久化。阈值为代码默认20、配置文件50、非空环境变量覆盖，不能无条件断言运行值50。
2. 最终 MD11/21 仍称 insert“持久化”，导语仍叫“审计落库”；MD13 的概念目标核对注没有消除正文矛盾。六条调用边保留，七项列表含一个条件步骤，不是七条方法调用。
3. 图 MD37 的 `C->>S: create(...)` 把入口方法当成被调用方法，真实调用为 schedule。容量检查仅一条 Note，没有失败分支，也没有明确限定为通过检查的成功路径。图语法本身合法：实际原文经仓库 Mermaid JS 解析并渲染，SVG 25330 bytes，记录 `.codrax/tmp/20260909-r1049-java-mermaid.json`。不能把语法绿签成关系/逻辑绿。
4. 精准入模已有 exact_call_only、名称不能证明持久化，以及 `List.add` / `System.out.println` 原操作（log3178–3200）。持久化误称从模型早期探索/首稿就有，补丁3338只修元数据、3393加目标核对；不由系统改写模型原文，也不加关键词硬门。
5. **B1637-STANDALONERELATIONOWNERDOMAIN1/P1 真系统冲突**：教学 log2332/3171 允许混合块显式声明 call_edge、guard_condition 等；首稿3237 的 hop-list 已声明两种。`preCheckStandaloneCallChainRelationAnchorPresence` 却只将三种 principal 调用关系写入集合，再用该集合审所有 relation anchors，log3257 错报条件声明缺失，模型3338随后删掉对应锚。修向是分离主路径资格集合与同块完整显式所有权集合，不扩大主路径资格、不免除方向/证据核验。另一个 S→S 图锚指向 IllegalStateException 确实不等价，不能把全部拒绝都归本缺口。
6. **机器假绿**：原 primary_answer 正则在同一个正文段落匹配“未完成访问数 … 持久化 … System.out.println”；“未”修饰访问数，不是否认持久化。已用原 case 正则及 run.sh 的同一 primary scope 复验，不是页脚/引用污染。本轮保留原 PASS 与人工 FAIL，不为这一个句形改产品或拟合 oracle；后续评估语义验收能力，不能用机评销账。
7. 首轮 analyzer 缺 runtime_selection_profile 后补齐是合法 schema 拒绝。没有证据说明该项是教学自冲突；没有额外 JSON 必填字段修复需求。

## H9：计算和保护能力正证，残余逐域记录

工件：`.codrax/output/20260909-055115.700-81565.{md,html,root-causes.json}`；日志 `eval/results/real_trace_h9_conversion_single_basis-20260909-054517/run-1.logs/codrax-20260909-054518-000-81565.log`；原查询载体 `.codrax/blob/20260909-054518-000-81565/trace-query-result-{1b987d2c,564f6f0c,7ea3ce76,2bf51009}.json`。

1. 三次模型查询和一次 frame_root_cause_bundle 系统补采都在同目标17597、`13762.791708..13763.024898` 的233.190ms精确窗。全程仅trace来源；不能把第四份系统补采说成模型扩窗。
2. 原 JSON 复算：17267 的143.499−91.764347＝51.734653ms；17597原始running7.305/折算4.958；keva-3原始running2.579、runnable1.023＋running缺口2.286041＝3.309041；keva-1原始running1.419、runnable2.181＋缺口1.248＝3.429。不能把原始running与缺口混为同一量，也不将1.419误写成2.529。
3. MD57–86两轴保留，链上IO15.304及小贡献/语义业务线索在；JIT2.388与非链上业务span/邻近资源只作背景，不获根因资格。1份因果投影仍在。状态账户230.969已归账、2.221未归账如实披露；这是 B1636 修复前二进制，不能据此签新数值批live。
4. **B1635a live正证**：MD95–97独立范围注只一次，分母163.223、已归因120.553/74%、余42.670/26%、未计入自身58.358保留，不声称全窗完整。**B1635b live正证**：MD185–187/415 的target_self同CPU席原始0.084/有效0.035，发“优先级反转候选·同核可运行重叠”；真正依赖链 keva 候选不改成同CPU词。
5. 默认root-causes旁路存在，status=available，5项是模型选择集合，不是全13席榜；frame causality未证仍披露。不因为行数少就自行补齐模型根因选择。
6. 模型质量仍有问题：MD15“直接根因不在自身”与自身#3过强冲突；MD19/40把核能力折算简化为低频原因，并把原始总量叫“实测折算后总量”。完整精确数值、默认能力比和机理未证限制已经入模，未证明系统缺事实；保留partial，不伪造新硬判据。
7. 修订过程：log2957首稿accepted；3042将普通wakeup impact选成runtime_work_relation收据，3045精确候选门拒绝，旧模型首稿继续发布，没有空答案。不是 field_not_published，没有确认新的 JSON 教学冲突；额外 schema_version 被隔离也不导致系统代选。
8. **独立显示残余**：MD250/277称keva-1原始running未发布，但原JSON及MD467已有1.419，事实冲突已证，具体载体传播根修仍待追。MD470同物理CPU0/4策略事件跨四结果重复长句，共占8条；每份query人口/ObservedAt不同，原完整来源去重不应放松。本案没有第三CPU库存，不能声称已挤掉其它CPU；后续仅可研究同物理事件显示分组、逐query统计/来源完整保留，并以3CPU×多收据验证公平cap。

## 本批边界与后续

- B1637 P1合同冲突先根修；B1636 P1 headless wakeup账户独立数值批。两者都需公开入口红绿、负例、旧套件差异归因后分批提交；本报告不冒签已推送。
- B676 proof-only生命周期、B1634d配置原值交接、B1634b图可见标签、B1634c已应用计划oracle域、B1122失败摘要以及旧队列不撤销。Java自然语言真假/图消息错误与H9措辞继续异构观察，不单题拟合。
- 新一轮尚未启动；本轮两例是读模式，写模式不冒领验证。日志最长单LLM调用78.473s；H9总361s不是连续超过4分钟单流证据。已有实际SSE工程针验证4ms/旧4m活跃增量不能触发年龄降级，真实停滞/取消/调用方deadline仍有效。
- 零系统改写模型正文/图/结论，零用户请求/答案关键词硬门；Trace显式窗、投影、自动补采、链上两轴/IO/语义/业务方向保留。
