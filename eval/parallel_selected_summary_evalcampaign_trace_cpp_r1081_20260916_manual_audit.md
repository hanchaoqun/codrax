# r1081 人工审计：H7 真实 Trace / C++ 虚调用

- date: 2026-09-16T06:32:55Z
- sweep_start_ts: 20260915-233254
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁源码 `91e7467e32ba`，构建 `2026-09-16T06:32:31Z`；CAP5、两路各一次，不改题目/夹具/机器oracle、不追跑第三例。机评2PASS不等于人工验收通过。本页只审计，不修改两份模型答案或正式旁路。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260915-233255 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 210s | 55 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 核心状态/双轴/投影/12项旁路保留；正文误归小计、错计数、过称完整，旁路模型描述9/10互换 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260915-233255 | answer_regex,answer_contains | none | 460s | 53 | read=10,repo_map=1,list=0,trace=0,source_lens=0 | midloop=11,inv=2/0,fin_reject=13,unavail=0,prune=0 | fail | 核心虚派发正确，但虚构时间戳/调用方失败动作、漏过滤；模型主动删图，最终图N/A；13次修补拒绝另审 |

## 1. 证据与保全

- Trace：`.codrax/output/20260915-233623.353-98778.{md,html,root-causes.json}`；单日志为该case目录`run-1.logs/codrax-20260915-233257-000-98778.log`。
- C++：`.codrax/output/20260915-234033.366-98780.{md,html}`；单日志为该case目录`run-1.logs/codrax-20260915-233257-000-98780.log`。
- 下文日志行号均指对应单日志；MD/JSON行号指上述原件，不是审计重排后的副本。
- SHA收据：`.codrax/tmp/20260916-r1081-{cases-trace-before,fixtures-before,h7-original-before-audit,cpp-original-before-audit,machine-summary-before-audit}.sha`。原题/trace/fixture核对一致；原MD/HTML/JSON、run out/log与机器summary保持不动。

## 2. H7：精确事实已供给，不能用投影存在抵销模型误用

保留的核心能力：MD15/104/579为明确233.190ms窗，running74.915、runnable1.576、sleep118.586、D36.757、IO0，已归账231.834、未知1.356；MD46/132/211–214将真实running与折算65.912（ideal9.003）分账。D11物理段、最长3.853正确，等待调用点没有被最终正文当作具体资源或持有者。MD390准确披露logd49.656=链上0.033+背景49.623，未把背景提升为主因。MD198/200/1072有未入榜链上7项、未计价占用、枚举未完整；MD1078与日志1126–1128保留系统补采critical_blocking_calls。模型查询3次，不能把系统补采算成第4次模型查询。

整份人工FAIL的可定位问题：

| 最终误用 | 此前实际供给 |
|---|---|
| MD17把JankManager单席3.077写成双席小计5.324，又单列Reade2的2.247；与MD32/42自身矛盾 | 日志1808/1813/2540精确区分单席与双席小计 |
| MD56/76/92完整11段后另称4条溢出未列 | 2431–2444给完整11段；2499–2504区分12条caller记录与等待清单，并禁止差值推测 |
| MD80声称所有链上场次、不遗漏 | 2500–2503明确全榜枚举不完整 |
| MD94将idle唤醒写6，分组12+7+6+5也不等于总29 | 1706、1711–1715给idle 2+1+1+1=5，另5个单次，总29 |
| MD19/76把Binder的11/18人口写错 | 2226/2451分别给unresolved=11、unassociated=18 |
| root-causes.json第9/10的模型description对调主体和值 | 候选2082/2106正确；模型patch2731已提交错误description，JSON235–276保留原文 |

旁路为schema2/available/12项；所选ID到主体、数值、窗和口径的系统映射均保真，错误在模型自写附言，不能称系统错绑，也不能用字段映射正确宣称整份旁路语义正确。未改其description、排名或候选选择。

新来源修复的生产覆盖须收窄：模型两次emit_analysis将排除原句放在artifact_citation_quotes，却漏专用排除quote（525/568）；526–527、569–570撤回无效排除，574最终接受default/external_only，实际lane为allowed_optional。专用字段教学已入模（109），并非未告知。零源码读取、旧源码索引附注未复现可记，不能签CurrentSourceLaneExcluded自然命中；external-only块分支与公共针分列。首稿六块有external_observation，后继patch遗漏claim_uses仅soft advisory（2733）。两轮成文、0拒绝、1patch，无JSON恢复，无Mermaid，图渲染N/A。

## 3. C++：主干事实部分正确，完整性与解释仍失败

最终保留unique_ptr注入、virtual write→ConsoleSink→fputs/fputc→stderr、Error条件flush及Console继承空实现，三种工厂分支及unknown nullptr均在。没有照搬README里普通log经过format_value的陈旧描述。

但MD17/24虚构时间戳，实际logger.cpp:33–35只有级别标签、空格、message；MD16称首先reserve/size，漏掉line30的null/min_level早退（已供给日志2844/2891/3050/3455）。MD11/36将unknown nullptr升级为调用方实际检测/loud fail，并暗示make_sink创建Logger；夹具没有装配调用站点或失败动作实现。均不是缺少源码数据，应保留为模型质量债，不用正文关键词门强纠。

引用MD41–53共13个quote与run-1.repo实际行去首尾空白后全部字节一致；logger.cpp:27确为`explicit Logger(...)`，不是限定实现形式。fixture/run-tree/HEAD一致。引用存在且摘录准确不等于说明范围充分：flush只引调用38未引条件37及默认实现，工厂只引if而未引return表达式。

原full摘要与最终MD11逐字相同；最终patch `tool-call_function_878nt30xfpoo_1-emit_answer_document_patch-params-27898c40.json` 的hop1全部item正文和8条visible_label原样入最终稿。MD22–29只是这些模型所选anchor的文本投影，时间戳等错误不是系统追加。

图的所有权：首full有sequence，模型在3656/3697主动删除seq1，第二次已staged；最终MD无Mermaid fence、HTML无Mermaid元素。原题未强制图，不能据此强补图或判必需图缺失；同时也不能把`mermaid_source_repair_applied=1`（已弃首稿）当最终渲染/完整图关系闭环，最终图N/A。

## 4. 十三次成文拒绝：模型错误、准确合同与教学缺口分账

1. 首full缺锚标签、非图关系声明无anchors、选择列表误声明principal_path_edge、图边无有效anchors；相关要求已供给。早期工厂方向误解来自analyzer自写摘要（595/618），后来作为历史分析传递；原repo_map只有Logger→move、make_sink→create，不是解析器方向污染，源码读取后2082已正确。
2. 第一patch同时order+remove，原模型参数3656明确二者同在；2600及工具description早已教不能组合。3662未staged/base不变；后继模型移除order后接受删除。未证同一声明必带必拒。
3. 四轮atomic add误把ra1放入failure_ref；随后whole replacement漏identity/label，attach补identity后又以whole replacement丢掉它。全量替换不是merge已在共享教学说明；identity与label问题在3974同轮均报告，并非逐条隐瞒。
4. `ra1-612...`为模型从rf1换前缀制造；此前有效的`ra1-255...`在4160 whole replacement已产生新底稿后不在当前8条授权清单内。4224/4285均不提交，cap key不变。不是无提交偷偷换代；最后4337补齐后接受。全部请求attempt1、没有传输重试或超时，460s父runner/458s工件，1full+13patch/14成文轮、上下文53%。
5. 独立确认P2教学遗漏：`pre_emit_check.go`普通非图full-replace fallback把五字段配方称为“complete”，却漏掉校验必需的visible_label。共享补丁schema其它段落正确；当前live先走atomic再由模型转全量，不能把全部13拒绝归此遗漏。该通用教学闭包另片公开先红后绿修复，见统一账本§123.1832，不修改本次结果。
6. 有界候选覆盖与底稿可见性继续P2观察：当前最多8条，Console不同身份方言占多个槽；supported但本轮未发布、错误引用角色与unknown/stale用语较难区分。缺label时只回端点/块清单，未回完整当前anchors，增加复制遗漏风险。后续应基于精确当前记录说明覆盖和给出无损修改指引，不扩大授权、不自动写标签、不合并未知身份，不据本例直接立P1。

## 5. 交付边界与下一步

本批两项前置修复已推`653d99056`/`91e7467e3`，公共RED→GREEN、count3/race3、独立冷审、最终冻结86测试包及清洁构建通过。r1081两份最终人审均FAIL，机器原2PASS保持；没有新确证P1合同自冲突。完整图表达、Trace模型解释、root附言一致性和跨语言执行证明债继续OPEN，不能借本批单例/无图销账。下一对继续轮转较少覆盖的写/计划模式，不追绿本对。

首响应600s、真实静默300s、非流600s不变；公开活跃语义/心跳/调用方取消回归通过。本对请求自身均短于这些上限，不冒充一次10分钟长流实测；持续活跃流不因4ms/旧4m无正文提前降级，调用方独立预算仍有效。
