# Selected Eval Manual Audit — HMC §235

- date: 2026-10-13T09:15:22Z
- sweep_start_ts: 20261013-021521
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_sampling_query_handoff_20261013

真实代码快照`b9f689018e11`；两例同时启动，各一次，无第三例追绿。机器2/2通过，完整人工0/2通过。10份case/输入/oracle/runner SHA-256前后一致；同批后继确定性补救不得回写此verdict。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_gzip_sqlite_jank_multiapp_inventory | PASS | eval/results/hmc_sampling_query_handoff_20261013/trace_gzip_sqlite_jank_multiapp_inventory-20261013-021522 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 88s | 41 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 全量取数正确；最终7帧行appid错配，分组3/1而非2/2 |
| 2 | trace_lossless_sampling_tree | PASS | eval/results/hmc_sampling_query_handoff_20261013/trace_lossless_sampling_tree-20261013-021522 | successful_tool_result,trace_attachment,answer_contains | perf_triage+trace_query | 160s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | tree不可用被算工具成功；未修复，改event_search后混分母/拆帧 |

## A：完整材料与成员已送达，最终字段误抄

原gzip→只读SQLite→完整文本准备成功；唯一event_search保持1.0–1.5秒窗，未额外限制appid。完整结果18条事件，其中5条jank marker；至少3帧正确为4条。查询未使用event_field_filters，模型在完整原生成员上筛选，因此本次没有自然触发source-required失败/重试，不能把确定性重试回归说成live覆盖。

正确四元组分别为27599/3/1020000000–1040000000/20ms，27599/5/1100000000–1160000000/60ms，41002/4/1180000000–1240000000/60ms，41002/7/1320000000–1420000000/100ms。explorer完成结果及finalizer原生库存均正确。finalizer日志行1983的row-16在jank字段、marker_pid、emitter身份、原文及语义载体均保41002，没有字段截断、预览替代或源身份漂移。

最终普通table把最后一行appid写成27599，正文重复错误分组3/1；emit被受理，后续patch仅补facet，完整成文仍错。无图。该次能确认模型误抄及普通结构化表缺少原生行/字段绑定，不能凭一例证明“偶发波动”。后继应统一原生行/列引用和分组口径的确定性复用，不扫描正文、不对appid或某行做专门补丁。弱regex检查各数字存在，无法识别跨列错配，故机器PASS不代替人工。

## B：不可用查询的错误交接及采样语义回退

pre-triage曾把窗外900及不同来源/事件混入权重，并把字面帧`sys;root`误拆。实际tree调用加了不支持的pattern，原生返回`unavailable/requires_time_and_thread_selectors_only`，却被工具Success=true包装；模型未修复该调用，转读event_search五条采样并提交错误aggregate。该次没有取得可用原生树，因此没有自然覆盖本批合法decorated selector修复后的展示正路径。

最终正文和多张表仍将raw cycles35、raw instructions100、simpleperf cycles7相加为142，把A=122/142、B=20/142写成热点比例；又合并两种producer的cycles为42。原生应独立给出raw cycles完整分母35（有栈30、未知5），A10/35、B20/35，另外两域100与7分别统计。`sys;root`是单帧，不能拆成sys→root；缺少精确帧的样本不能被补成同一调用层次。尾注称单位不同没有纠正前文混加/热点推断，整体FAIL；无图。

优先补工具所选能力的availability→Success/Repair交接及准确参数教学，保不可用原件、原窗和未知状态；旧统计合法过滤但伴生tree不可用不可一刀切失败。该系统缺口归09.4/01.3/16.4/18.4；原有任务ID复用，不新增重复父项。原始机器结果、模型草稿、工具载荷、最终成文及同批后继确定性回归分别留存。

## 同批修复与验收边界

后继代码`0285ff251e51`已将直接采样树的精确不支持选择器结果转为可修复失败，保原始载荷、来源和时间窗。公开工具显式重试及实际finalizer回归通过，35/7/100分母、未知5与字面帧保留；旧四视图合法过滤与其他不可用边界保持。该确定性补救没有第三次真实运行，不能改写上表FAIL或宣称自然模型已正确重试。

A的原生事件行/字段绑定与分组展示仍未实施，按原任务保P1。B修后的自然验收、A业务元组校验及轮换只读登记/二进制剩余边界继续开放；本批完整能力新增0，累计21交付、58开放。

## 冻结与证据

代码`b9f689018e11`及同批补救`0285ff251e51`已普通推送。末版构建、相关/race及全仓正式exit0，全仓90测试包PASS、13无测试包，全部fail动作与非JSON错误行均为空。这里只认证确定性回归，不改变两次真实答案的FAIL。

发布前79份工具载荷/准备文件/答案产物/审计/失败/验证及代码推送收据逐件保原字节，见结果根`evidence-sha256-index.txt`；文档发布收据后续追加。所有10份输入/oracle/runner前后校验一致。
