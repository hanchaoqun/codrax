# 有效回答语言批次：完整人工审计

- date: 2026-09-22T07:15:10Z
- sweep_start_ts: 20260922-001507
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_language_authority_20260922

冻结代码 `4f3206c8c655`，构建时间 `2026-09-22T07:14:44Z`。runner 30328 正式 exit0；严格 2 并行 × 各 1 次，无第三次追跑、无 oracle 修改。机器 2/2 PASS，完整人工 0/2 PASS。后续仅补测试夹具的显式语言元数据，不改本批生产代码，不伪称又跑过 live。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_language_authority_20260922/real_trace_g1_english_dstate-20260922-001510 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 36 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 原始 D/IO 与语言通过；调用点机理越权、搜索截断冒充捕获缺失 |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_language_authority_20260922/trace_query_wakeup_causal_io_chain-20260922-001510 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 389s | 47 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | FAIL | 显式窗/投影/IO/调度候选保留；错误状态时序、占用冒充归因、未证机理及收益 |

## 1. G1：主审及独立完整人工均 FAIL

完整阅读 `run-1.answer-transcript.md`、最终 `.codrax/output/20260922-001735.669-51041.md`、同名 root-causes 旁路及日志 `run-1.logs/codrax-20260922-001513-000-51041.log`，核对实际原始记录。

- 全捕获窗 `34579.450627..34579.595184`、tid59566、三段 D+IO `0.138 + 0.147 + 0.350 = 0.635ms` 准确。原始 switch-out→wake/原因行分别90→117/118、225→249/250、2427→2532/2533，内核调用点均 `sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]`。五态 `52.478+5.529+85.915+0+0.635=144.557ms`，完整15623行；pid157背景未冒充目标主因。最终模型由早期两段误读恢复三段，不能说模型全程正确。
- 最终正文25行把调用点提升为“均为同步缓冲区读取导致”；29行把返回40条、总匹配620条写成“全量sched_switch上限620”，又推成 Trace 截断可能漏极短D。实际原生全窗统计完整；搜索展示限制不等于采集缺失。另一次精确子串查询 `prev_pid=59566 prev_state=D` 因中间有 `prev_prio` 实际零匹配，不能宣称该搜索验证了全部D。非IO D为零由独立状态账支撑，不由该搜索支撑。
- 日志1650已有调用点边界教学；1833–1836已供完整三段等待，1149–1157完整扫描。不能把这些错误全部归因于模型缺材料，也未证明随机波动；不追加正文关键词门。
- 系统附录与项目中文一致；两次分析均选中文，所以本例**未命中项目/合同语言冲突**，不能用此次 live 代替公开8格RED/GREEN。分析器首次 role_locate 缺 answer_subject 的拒绝正确；成文首次接受、patch0，无JSON/超时降级。schema2空旁路原因 `trace_root_cause_contract_not_active` 对有限事实任务合理；未要求的图缺席不判错。

## 2. 显式20ms因果链：完整人工 FAIL

完整405行答案、内联15行Trace、最终 `.codrax/output/20260922-002136.898-51030.md`/同名旁路和日志 `run-1.logs/codrax-20260922-001513-000-51030.log` 全面核对。

- 明确范围 `2.000..2.020`、目标app100、S20ms、四节点 threadpool→network→cookie→app、三次唤醒2.016/2.018/2.020及CPU4→3→2→1均保留。链上IO11ms、三个低优先级依赖各1ms候选没有被删除；Trace因果投影存在，链旁/背景不替主因。
- 正文虚构 network/cookie 被唤醒后再次睡眠；原始记录是 wake→runnable→running。将cookie唤醒app说成“app获得CPU”也不成立：app实际切入Running在2.020020，位于指定窗外。将cookie17ms睡眠叫“直接传导量”、将app20ms叫整个链起点到终点耗时、断言app不持有锁或IO均没有对应凭证。
- 调用点 `fscache_page_wait_on_page_bit` 被升级成已证页缓存资源/具体机理；后文承认没有业务绑定不能撤销前文越权。最终输入3736已明确调用点不授权资源/持有者。系统自身仍有§104“已证最大可消”“11ms可消”以及状态表重复占用行，独立保留为系统债；不以模型错误遮盖系统错误。
- §111缺口仍可见：日志3722–3724同一 `state_value_authority` 并列cookie sleep17与候选归因1、threadpool IO11与候选归因1，却没有归因量本身的分量说明；3737方向leader同样缺失。详细旁路已有runnable1+running_deficit0分解，宜复用同源说明，不能重算或改结论。不能仅凭并置断言它必然导致本次模型错误。
- 成文首次接受五个模型块；可选旁路第一次遗漏schema_version，正文保留；一次patch只补schema_version2，五块不改。最终旁路 available、模型选IO和cookie两个合法候选；未系统强填其它候选，但其模型description仍带机理越权，因此旁路合法不等于语义全对。教学3365–3368已有正确原生对象示例，不为单次漏字段增加矛盾要求。
- 探索三轮/13次查询：两次已接受completion后，DAG仍因未满足current_source而继续（日志1433–1435、2051，末轮2645才自动完成）；成文3491–3495又明确该义务soft、runtime_only_with_caveat=true、hard_block=false。列为需公开复现的**源码适用域/完成条件接缝P1**，不能只据日志就断言哪一条件应删除。一次emit_evidence字符串内JSON畸形被严格拒绝；原生array重交后9条external行正确不进入源码池。没有把运行时行硬塞源码证据。

## 3. 验收边界与队列

本片共享语言解析不改数值、窗口、链上资格或模型结论；实际系统附录中文一致，不代表全域内部词汇/非中英翻译已完成。真实读模式live没有新增写模式验收。前批Go实际apply收据仍属于前批，B2–B6不销。

父账79项中13已交付、66开放。优先继续§111归因分量身份、§114标量legacy来源、§104未验证收益；新soft-source接缝先公开复现，再定修复边界。模型越权、搜索/捕获误述留人工FAIL，不用机器2/2或语言定向通过倒签。
