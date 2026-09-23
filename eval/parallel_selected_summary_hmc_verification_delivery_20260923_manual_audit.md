# 补证交付身份固定双例人工审计

- date: 2026-09-23T06:35:50Z
- sweep_start_ts: 20260922-233548
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_verification_delivery_20260923

冻结干净构建`f1a65844f766`，22385正式exit0；Go提交`eec64d80f`/`9787be9b9`。59657正式exit0，恰好2并行×1，机器1/2、完整人工1/2。1800秒仅评测外层预算，不改产品600/300/600秒及活跃流保护。未追第三例。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | empty_python_module_apply | PASS | eval/results/hmc_verification_delivery_20260923/empty_python_module_apply-20260922-233550 | write_apply,write_patch_oracle,answer_contains | none | 144s | 28 | read=6,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS | 功能与交付通过；新补证快照未命中 |
| 2 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_verification_delivery_20260923/trace_query_business_marker_io_chain-20260922-233550 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 304s | 46 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 业务窗、请求耗时与等待混淆；正确上下文已到场 |

## 1. 写模式：交付通过，不替旧失败销账

主审与独立审计读取完整计划、报告、final、两阶段日志和交付树。交付commit`3763b047259bff3103da5c66c84c565b40c603b1`仅totals.py三行，实现sum(values)；递归对比原fixture仅此文件不同。源码SHA256=`e21ba9a91716eb728b3769cb3582e8c55cebaf17044f558d59a5f42e381324a7`；原测试各副本SHA256=`95143baf97adf98c6343795287cf6cab334533ddc5b34bea7e1887067c1e5556`，测试/配置/依赖未入交付改动。

实际执行`python3 -m unittest discover -v`，空输入、正负整数、大整数、一次性迭代4/4通过。报告6/15/24/33行结果与82行命令共享`native:66fb:18d7dfd3f9c17440:1`。临时主仓HEAD仍seed`3cba5c00…`且totals零字节；交付工作树HEAD等于上述commit且状态干净。主仓另有未跟踪.gitignore，仅既有Codrax运行目录忽略块，不能声称主目录字节完全未触及。

最终out167–183准确披露工作树交付、4条验证通过及自然语言清单不等于独立行为证明。中间controller正文161泛称所有契约均满足，不是typed逐合同收据。本次无run_existing_test、PTO、verification probes、source-free replan或AppliedSources；10条合同均planning_only_ungrounded，hard/soft required=0。故不代签新快照真实模型分支或§153.1/B2–B6。摘要probe_count=1包含syntax preflight，不证明Python probe发生。

早期两次create-on-existing安全拒绝后改insert_at_eof；首次含错误pytest身份PTO的计划**实际被接受**，日志1408随后因空基线缺owner/evidence发生controller定位重试，模型后续删除PTO，不是unknown contract refs拒绝。空基线定位、声明丢失仍留旧账；3行尾换行对应owner第4行只作待核坐标，不臆造新故障。外层144秒、case/metrics142秒。

## 2. Trace：正确上下文到场，最终答案未通过

这是作者构造的文本夹具，不代签客户捕获/原始二进制。真值：OpenDocument1.000–1.050秒=50ms，5ms运行+44ms睡眠+1ms调度等待；worker请求1.005–1.040秒=35ms，完成唤醒闭合的S阻塞1.009010–1.040010秒=31ms，worker/main各1ms唤醒后调度等待。后台请求47ms缺目标依赖闭合，只能为背景；各计时口径不可重复相加。

完整principal失败：1/14/21/43行把52ms查询窗6ms运行用于50ms业务，还出现“6ms（5%）”、1.046–1.050运行5ms；41行将31ms阻塞区间说成请求耗时，缺35ms（机器同样FAIL）。16/21/42行将业务完成依赖当机制事实，57行又承认未证；35行把醒来后1ms叫醒来前等待。缺完成唤醒证明被写成没有任何唤醒。表头列1至列5、completion_closed/runnable_wait/IO延迟座位等内部词仍有可读性问题。

上下文logs.all:2426已明确业务50ms、5/44/1ms及禁止宽窗替代，2440有请求35ms/阻塞31ms及各端点，2439有“缺证明不等于没有唤醒”。主要口径/算术/机理错误不是字段缺失或截断；单次也不足以证明随机波动。

## 3. 确定性系统缺口及保留边界

1. **组合修复提示（16.4/18.4，优先窄修）**：logs822同时列目标引用错误和fact_families冲突，却要求repair only后者；第二次只修一项，892再拒，第三次才成功。emit_analysis.go:5287局部教学应与组合错误汇合一致，不放松门。
2. **业务实例与全工件范围（01.3/02.4，开放）**：三次分析把“分析附加trace”声明full_artifact，1497合法业务实例已接受，但trace_query_supplement_business_focus.go:46全工件保护阻止其参与补采，1593/1600仍1.000–1.052。应修类型表达/教学，不直接删除全工件保护或覆盖显式窗。
3. **确定性图的IO名称（16.4/16.5，开放）**：文本投影124/164/291行将47ms请求驻留叫IO等待/IO阻塞候选；runtimeTraceCausalProjectionResolvedPeerText按io_latency统一命名，虽未晋升主因仍混淆计时口径。52ms捕获建议80–150ms热点窗仅待审，不证明是成文错因。

保住：一次系统文本Trace投影，无Mermaid解析错误；storage-irq-80→document-worker-200→app-main-100、LoadDocumentIndex及链上31/1/1ms保留，后台47ms不入主因，TID80未换成TGID2，未读取源码。旁路`.codrax/output/20260922-234051.895-25800.root-causes.json`合法schema2/available、三项31/1/1ms，来源/查询窗齐全；描述仍有runnable_wait，不冒称语言层全通过。

外层304秒、case301秒正常结束，未因活跃流超过4分钟降级；不是10分钟极限等待的实测证明。原FAIL保留，无第三例，不增加或误销父任务。
