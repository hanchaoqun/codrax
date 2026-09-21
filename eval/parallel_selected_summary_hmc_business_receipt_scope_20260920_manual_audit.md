# 7f 固定双例人工审计：机器1/2、人工0/2，不销旧账

- date: 2026-09-21T05:07:30Z
- sweep_start_ts: 20260920-220727
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_business_receipt_scope_20260920

干净revision `7f29399e38e2`，快照 `.codrax/tmp/codrax-selected-20260920-220727`；两例并行、各一次，runner正式exit0。全仓87包通过不代替本单人工验收；后续混合合同/成文区间修复不在该快照内。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_business_receipt_scope_20260920/trace_query_wakeup_causal_io_chain-20260920-220730 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 198s | 43 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=2/2,fin_reject=1,unavail=0,prune=0 | FAIL | 唤醒误当窗内sched-in、累计状态配错区间、有效排名误当实测最长；成文handoff仍误标occurrence_interval |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_business_receipt_scope_20260920/trace_query_business_marker_io_chain-20260920-220730 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 329s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 混合因果/有限判断遭结构硬拒后降级，投影缺失；主业务账正确，子业务账混用及否定证明越界仍在 |

## 业务：实际混合合同不可表达，不能归模型波动

已完整读primary、最终`.codrax/output/20260920-221257.335-45440.md`、根因JSON、实际工具参数/返回及成文输入。MD SHA256 `1f0e9880d80ea538419550b699d7f6caa92da2b5fa0ec469a4e6aa28d04639a8`与available归属收据一致；primary提取文件SHA `6f23f36beba9aa1a0daba8604adb1563c1678af9f3811518e29e8b847bbc4b09`。

1. **确证合同自冲突，而非只读模型推理得出的猜测。** 6次emit、5次拒绝。业务日志931的第4次真实payload同时声明`scope=causal_diagnosis`、required `causal_contributor_set`（引用“哪条依赖真正拖住了响应”）、required `target_effect_verdict`（引用“能否把耗时较长的 IO 直接当成这次操作的根因”），无fact_families；936/937仍因一律禁止target_effect_verdict而硬拒。后一次bounded_effect又被root_cause标签拒（1000/1001），最终1046/1052改explain/有限效果且将因果维度改泛作用后被接受。首轮确有无来源引用/目标身份问题，不能说所有重试都由混合合同造成。
2. **投影真的为零。** 本轮有效分类是bounded_effect，报告权威正确执行该分类，不是计数器或渲染丢图。mandatory旁路schema2、空数组、unavailable/trace_root_cause_contract_not_active。应修组合合同，而非把有限查询一律扩大，或由系统填因果维度。
3. **进步与失败分账。** primary5、11–15保完整50ms及5/44/1ms，后台47ms写没有加冕；主链等待也没有转给左端。但23行在40ms子业务旁写更宽线程窗的9.5ms运行量，真实实例8/1/31=40ms；5行把0.999..1.052差值53ms写52ms。31行从completion_woke_issuer=false推出“未触发任何线程唤醒”，这是将未证误作不存在；31/32及21仍直接泄漏内部字段。40行将35ms请求驻留作主瓶颈量，虽另列31ms实际阻塞，二者角色仍未说清，结尾又称两段IO均非根因，形成表达冲突。
4. **准确上下文已经送达的部分，不重复加门。** 最终输入2414明确false不证明未唤醒，2415同列35ms请求驻留与31ms闭合S等待，2424同列LoadDocumentIndex起止1.0045..1.0445及8/1/31本实例账。原始fixture行8/14、9/11、10/12可逐项核验。不能声称本次这些错误全因没证据，更不能用答案扫描硬门重写结论。
5. **§51新显示分支未被live命中。** 有效分类runtime_work_relation_requested=false，模型没有提交结构工作关系行；5次Trace查询虽找到完整业务，最终未发49/50双口径回执，不以底层正确量或主账改善倒签该分支生产命中。该修复的公开真实渲染及全仓收据独立有效。
6. 最终一次完整emit，无成文拒绝；系统仅修确定性metadata/inline-code后合同全绿（2602–2645），没有系统代写以上主结论。无空答案、无活跃流年龄截断。

## 明确窗：图和根因文件保住，成文时间语义仍有系统缺口

独立完整审计primary、最终`.codrax/output/20260920-221046.286-45414.md`、日志/图和根因文件。MD SHA256 `6a7d581111a087fd0ffe9dba25db4a57129a07c3d145097a7f0b7ed007f33483`，归属绑定一致；primary/principal与收据对应字段SHA均为`42bd8006d56926ab1e791303bad06dceb22ee483015fdb938613e771a25a45d8`。

1. primary1/9/42将2.020000唤醒当切回CPU1，真实sched-in为2.020020、在显式窗外。第3行把IO11ms已证可消除排名第一改称整链最长单点阻塞，同句却列S17/S14。39行network14ms配2.002..2.018（实际睡眠到2.016）；42行cookie17ms配2.000..2.020（实际2.001..2.018）。
2. **确定性供给共因保留复现。** 实际成文log2472–2474、2487用`occurrence_interval`标cookie2..2.02、threadpool2.002..2.016、network2.001..2.018，这些是依赖分析/定位包络。生产`internal/agent/answer_document_final_decision_boundary.go:1836,2001`无条件把node.StartTs/EndTs写成发生区间。最终图/明细194–196、423–434已中性化，handoff两面未同步，独立后续修。不能声称所有模型错误仅由此造成。
3. 16/51行把kernel caller扩写为已知页面缓存位并引向预取/回收，虽67行限定资源/后端未知，仍越出该符号支持的结论；63行无竞争/延迟凭证建议专核降跨核延迟。实际输入2524保护窗后运行、2533区分有效/实测、2545限制caller含义都已在，不增散文硬门来拟合。
4. **系统投影保护通过。** 最终83–87实测/有效分账，102定义主根因为链上单项最大可消除量，121/130区分定位包络相交与实际重叠，151–170分开S17/S14与Runnable1候选，同源1+0保持，211说明重复发布不等于物理发生次数，420–422不铸IO请求/对端身份。3个text围栏闭合、表格合法。92–95已有同主体重复占用行仍未销。
5. schema2根因JSON available、4项11/1/1/1ms，显式2..2.020不变，lower_priority_dependency_candidate及runnable1/running_deficit0等证据保持；定位句中性。模型description重复“最长/页面缓存位”，不误称为系统新铸的机理。
6. analyzer一次拒后成功、explorer一次凭证拒后成功；finalizer因缺block id拒一次后完整重发（2613/2683/2695），最终合同2710–2717零违规。四线程/四CPU链、Binder未知非排除均保住；机器PASS不能代销正文错误。

下一步先修已证组合合同与handoff范围两子缺陷；§47.1 IO fold、XERR、跨模式B2–B6及其它HMC任务仍开放。两例人工FAIL不回写，固定版本不追加第三次运行追绿。
