# 85ca 固定双例人工审计：混合因果与有限频率判断

- date: 2026-09-21T05:40:43Z
- sweep_start_ts: 20260920-224033
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_mixed_runtime_contract_20260920

固定干净 `85ca9941ec4a`，2并行×1，runner正式exit0。机器1/2，人工1/2；旧业务FAIL不回写。源/案例/oracle均未在运行中更改，无同版第三次追绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | PASS | eval/results/hmc_mixed_runtime_contract_20260920/real_trace_h4_supply_thermal_witness-20260920-224043 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 130s | 39 | read=3,repo_map=0,list=0,trace=2,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | PASS | 四态/精确窗/CPU4上限与binding未知均正确；窄D/IO与Binder分开；旁路正常空结果 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_mixed_runtime_contract_20260920/trace_query_business_marker_io_chain-20260920-224043 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 272s | 45 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 因果投影/根因JSON和工作回执恢复；35ms请求量遗漏，业务/查询状态口径仍混用，false被写成没有唤醒 |

## 1. 业务响应：FAIL，但混合合同修复真实命中

以下日志行号均指该case的`run-1.logs.all.log`，正文指`run-1.primary.md`。

- 原生fixture第4/19行确立OpenDocument 1.000..1.050=50ms，业务自身状态为5/1/44；第8/14行LoadDocumentIndex 1.0045..1.0445=40ms、状态8/1/31；读请求第9/11行1.005..1.040=35ms，实际S等待第10/12行31ms，worker唤醒后调度1ms。backup请求47ms只是独立背景。
- **仍不通过**：正文1/7/15以1.000..1.051查询范围的6ms运行量主答业务耗时，没有给业务自身5ms分账；虽然7行分别写明50ms业务和51ms查询，没有直接把两者宣称相等，但仍未完成用户要求的业务响应拆分。正文全文漏35ms请求自身耗时，不能由31ms线程等待代替。29行将`completion_woke_issuer=false`扩写为“没有发出…任何线程”，并泄露内部字段。11行“不叠加”和29行“不能直接累加”已有部分不加和解释，机器regex未命中不等于完全没教；但缺35ms仍使三把尺无法完整区分，不能调低oracle销账。
- **明确改善**：3/19行实际命中模型选ID→系统工作回执，分别保50ms/40ms、自己的区间、查询范围及“单凭打点未证因果”限定；15行子业务运行8ms正确，不再用宽窗9.5ms替代。该live只命中完整span落在查询内分支，不冒称49/50裁剪分支已live验证。17行调度小计2ms有系统图的精确互斥小计支撑，不误列为无权加和。23行真实唤醒链和31ms S态IO闭合保留。
- **分类收敛**：首次混合对象（822行）已声明required causal_attribution+target_effect_verdict及work=true，却多带fact_families；824–825按新保全目标拒绝。第二次862行保全部维度/布尔、仅去冲突families，随后865–866因另一个generic/diagnostic不一致被拒；第三次900–903选择performance_bottleneck后成功，混合两角色/work=true继续保留。共3次emit/2次reject，相对7f的6/5减少；不是零重试，也不能把另一次拒绝算成已修互斥复发。
- **供给审计**：2412/2445行已提供35ms请求、31ms闭合S等待且禁止跨尺相加；2444明确false不是已证明未唤醒；2431提供业务5/1/44及禁止替换为宽查询量，2432提供子业务8/1/31。上述正文缺口不是本轮缺供给，不能继续叠加同义提示或用散文关键词硬拒。较早预检叙述有不准确估算，但最终typed recap已有正确值；仍需后续检查通用上下文优先级/阅读负担，不将一次遵循失败简单定为稳定模型波动。
- **成文/结构**：2737首次emit成功，2741起为工作关系软提示；2785/2791一个patch选择两个已发布精确业务ID，保留原文，不是重写/修补JSON失败。无成文硬拒、空答案、活跃流超时或备用降级。
- **系统图与旁路**：最终MD64起有完整Trace因果投影，66定义括注不变；88–143两段text图闭合，根因仅链上，47ms背景隔离，业务线索独立，定位包络不冒充连续状态。mandatory schema2 available，3个根因31/1/1ms，范围1..1.051、限定not_applicable；不再是7f的contract_not_active。原始占用/调度镜像重复等旧显示债仍保留，不能据此签全图能力闭环。

输出绑定：`.codrax/output/20260920-224513.346-59784.{md,html,root-causes.json,answer-surfaces.json}`（日志3175–3178）。Markdown SHA `5b571c3d3ba1bd962050bc7bb6693d93ba3e35794459ab75065ddf2d4a3a3996`与receipt相符；primary及receipt内文本SHA `1fda40eced1ac3f690386b6a63ca35a87bb13811467c2498b3e2b501d235da31`一致。available receipt的answer hash不与去抽取文件hash混为同一口径。

## 2. 真实H4有限判断：PASS

- 正文3/13–17保精确13762.791708..13763.024898窗、Running157.248/Runnable5.604/Sleep70.338/D0、合计233.190ms和8个CPU；无132.041错误跨尺相加。
- 正文5/23–29分开CPU0策略和CPU4策略；CPU4直接2100000kHz上限、目标35.960ms运行与实际558000kHz均保留，但没有把“有上限”偷换成“上限正在约束该线程”。日志954/961为直接原始记录，1834–1853提供同CPU比较及未证限定。
- 正文7分开闭合Binder等待5次/3.094ms与窄D/IO清单0次，没有从S推主动休眠，也没有由内核调用点猜具体后端。没有要求图且为bounded_fact_set（日志512），无因果图不是缺失；schema2空根因、unavailable/trace_root_cause_contract_not_active是有限问题的正常旁路，不是文件未生成。
- 过程1009曾误述开放尾部未纳入统计、以低于上限判未触顶；1871/1934–1948已给正确末端口径，最终正文没有复述错误。1997–2004首次成文成功，2055–2061接受unchanged-only patch；关于facet_ids的模型解释不准确但没有事务失败/正文丢失，四态表确实存在。无活跃流截断。
- **独立新教学债**：1862明确blocked_reason_records=50，1878的缺失说明却不带“若”并把窄D/IO空清单写作“没有匹配的等待原因记录”。英语近条件式泛化句，中文尤其容易误读成当前事实；根因在`answer_document_trace_principal_value_authority.go`说明的范围与条件，不是typed计数错误。本次最终正文未受污染，不倒判本例FAIL；另做窄修，将“若该窄清单为零”与独立blocked-reason/Binder域分开，不新增门。

输出绑定：`.codrax/output/20260920-224251.806-59783.*`（日志2179–2182），输出receipt与eval receipt字节相同。Markdown SHA `7ca8375bebd1ccebfc585a963004e08a2c4c332fa90c6a446b49602e6fa6f800`；receipt SHA `877f709a151a9a4a41d405deb0a9889725399ff6aad551fbe9e5c4c57b432484`；primary/principal与receipt内文本SHA `be661dbbdd731f0b456bfc37e16d4ceb9add8a39155b013191c00ee3553a3852`。

## 3. 后续次序

先根修H4暴露的窄空值说明，再推进已登记IO fold的peer身份/口径显示和XERR成员凭证。业务正文遗留暂不靠第三次同版重跑或关键词硬门解决，寻找可泛化的typed上下文/展示所有权方案。未因此核销HMC父任务，仍13/79交付、66开放。
