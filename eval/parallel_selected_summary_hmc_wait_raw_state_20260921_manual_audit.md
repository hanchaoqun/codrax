# 原始调度状态贯通：固定双例人工审计

- date: 2026-09-22T01:59:47Z
- sweep_start_ts: 20260921-185945
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_wait_raw_state_20260921

冻结代码 `f31f71d53`，干净构建 revision `f31f71d5355d`，buildTime `2026-09-22T01:59:03Z`。runner4607正式exit0，2并行×1；未追第三例、未改oracle。机器2/2，完整人工0/2；原始D状态交付和完整因果投影分别验收，不抵销模型语义错误。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_g1_english_dstate | PASS | eval/results/hmc_wait_raw_state_20260921/real_trace_g1_english_dstate-20260921-185947 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 84s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 三段D+IO与0.635ms正确；调用点仍被模型升为确定机制。另有已过滤模型聚合被校验回退复活的软提示系统gap |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_wait_raw_state_20260921/trace_query_wakeup_causal_io_chain-20260921-185947 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 208s | 47 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 四节点链、20ms窗、11ms IO及三1ms候选/旁路保留；调用点越权解释和未验证收益承诺仍在。首次参数混入传输标记拒绝后恢复 |

## G1：事实交付通过，完整答案仍失败

全文 `.codrax/output/20260921-190109.182-34415.md` 与同名schema2旁路、对应run日志均已阅读。全工件窗口34579.450627..34579.595184准确。正文明确三段原始D且IO标记为1，0.138/0.147/0.350ms合计0.635ms，非IO-D为0；不再说没有D或把三段说成S。真实finalizer输入1603–1605行有原始状态与统计类别双轴，窄修复确实触达。

本轮直接命中详细等待reader，未出现principal occurrence摘要或系统状态附录，不冒称所有消费者均live覆盖；公开确定性测试另覆盖这些出口。等待family未选，状态摘要未生成的具体路径仍未定位，不能全部归因于return_value。

完整FAIL：模型1740行仍将`sync_buffer_read_wi`说成确定的同步缓冲区读取机制及触发原因；1473行输入已明确调用点名称不证明机制/资源/持有者。属于模型未遵循已有边界，不新增正文扫描门。schema2空旁路理由`trace_root_cause_contract_not_active`符合有限事实问题，不是漏生成。

独立系统gap：explorer918行把首/第三次D入口差19.671ms提交为总等待；native正确总量0.635ms。typed answer projection已排除这条复述，成文prompt没有重放它；pre-emit两个读取点却把空投影误当缺失，回退Mutable原始facts，1742行发出要求所有聚合数值出现的soft advisory。最终答案没有接受该错数，零拒绝/修补；不能把此风险写成硬门或本轮错误答案。下一批按plan存在性修通用回退，不改原始审计记录。

## 因果IO：链与投影保留，语义债不销

全文 `.codrax/output/20260921-190313.240-34372.md`（404行）、schema2旁路及实际查询/成文/修补日志均已阅读。明确窗口2.000..2.020；app-100自身睡眠20ms，线程链threadpool-400→network-300→cookie-200→app-100及CPU4→3→2→1完整。链上IO11ms、三个各1ms的调度/优先级候选保留；背景不进入主因排序。确定性因果树保留全部四节点。旁路available=true，含模型选择的IO与cookie两项，未强迫把所有候选写成根因。调度候选保限定，不强称已证锁持有者或实际反转。

完整FAIL：正文先由`fscache_page_wait_on_page_bit`推成网络文件系统/缓存后端机制，末尾又承认证据没有对象/后端身份；模型自相矛盾。系统投影仍有“11ms可消除”的未验证收益承诺，属主账本§104已确认待修。内部轴名/状态词与重复上下文行仍是读者质量债，不据此篡改原生统计。旁路模型描述把11ms写成造成20ms完整阻塞，仍需模型语义审计，不改typed计量补救其措辞。

首次emit参数的blocks是字符串且混入`</parameter>`传输标记，日志2572明确在执行前被拒绝；第二次完整JSON恢复，随后一次patch只调整section的surface_role。无证据表明是原始状态字段或Mermaid解析拒绝。最终未保留模型Mermaid，但系统因果投影存在；本轮不拟合单次传输污染增设修复硬门，另留JSON恢复观察。

## 状态

原始字段窄交付完成；§104收益措辞、空投影回退和调用点语义越权分别留账。旧人工FAIL不回写，父任务13/79交付、66开放不变。超时仍600/300/600秒，未改变活跃流保护。
