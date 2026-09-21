# efd86524c84f 双例人工审计：0/2 通过

- date: 2026-09-21T03:56:26Z
- sweep_start_ts: 20260920-205626
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_measurement_owner_20260920

固定干净二进制2并行×1；机器1/2通过，人工0/2。完整审读primary、实际模型上下文、原始fixture、最终Trace投影及根因旁路。后续eff3d1365教学/普查修复与IO卡修复均未进入本次二进制，不能倒签覆盖。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/hmc_measurement_owner_20260920/trace_query_wakeup_causal_io_chain-20260920-205626 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 140s | 40 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | own等待17/14/11归属已改善；正文仍推断未证文件系统机理，旁路编造再睡/再唤醒时序；系统重叠说明独立核对 |
| 1 | trace_query_business_marker_io_chain | FAIL | eval/results/hmc_measurement_owner_20260920/trace_query_business_marker_io_chain-20260920-205626 | log_regex,typed_trace_projection_count,trace_attachment,primary_answer | perf_triage+trace_query | 250s | 40 | read=0,repo_map=0,list=0,trace=14,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=1,prune=0 | FAIL | 50ms业务窗仍套6/1/44=51ms；LoadDocumentIndex缺失，非相加口径未入正文；35ms请求与31ms等待已分开但尾部自相矛盾 |

## 业务样本

- primary首段/运行明细仍把1..1.051的6ms套在OpenDocument 1..1.050上；真实marker应5ms运行+1ms调度等待+44ms睡眠=50ms。工具log1425、1441已给正确marker账；finalizer同时有全窗6/1/44（3945、3962、3989），explorer completion4044还把OpenDocument/LoadDocumentIndex各写40ms。已知供给并不保证采用，不能删掉更宽窗口或替模型选择实例来求绿。
- 正文保留完整50ms、storage-irq→document-worker→app链、35ms读请求、31ms线程等待、1ms唤醒后调度，后台47ms明确为写且未升根因；这些较519改善。
- LoadDocumentIndex仍没进入primary，非相加解释只出现在系统补充而非模型正文，机器FAIL合理。两条wakee等待值旁又泄漏`pre_wakeup_wait`，尾段把已证storage完成唤醒概括为证明不完整；对应IO次生卡供给问题另修，不全部归波动。
- `20260920-210034.019-54866.root-causes.json`必选旁路已产生，但为schema2 unavailable/空数组，原因no_selectable_typed_on_chain_candidates。本轮没有运行root_cause_rank，不能把空旁路签为根因JSON内容合格或伪称产物缺失；Trace因果投影本身仍在。
- `.answer-surfaces.json` available，绑定完整Markdown与模型正文；无通过系统补充冒充primary的测量器问题。缺子业务/窗混账/旁路可选候选缺失继续开放。

## 明确窗样本

- primary第7行已正确cookie17ms、network14ms、threadpool11ms；不再把右端wakee14/17/20移给左端。工具blob `trace_query-9d663ad8.txt:46–54`、log1884–1887/2182–2190证实owner与统计窗口教学真实到达。20ms显式窗口、完整唤醒链、PIC各1ms、IO11ms、Trace投影均保留。
- primary3/13由fscache调用点推到页面缓存/文件系统IO完成，11进一步指向NFS/FUSE/预取；log2455明确缺具体资源/后端证明。把切入CPU后是否抢占当醒后调度延迟的判断也混淆切入前/后。
- `20260920-205843.678-54848.root-causes.json`为available，但21把cookie唤醒app放在threadpool11ms IO“期间”（IO止2.014，后续唤醒2.016/2.018/2.020）；51编造cookie被唤醒后又睡再唤醒，fixture未有该事件。原生量正确不代表模型旁路描述正确。
- 最终MD101/106/111/116的“同状态族物理重叠”说明待独立追生成所有权/身份；S与runnable在原始fixture互斥，不能以累计/查询包络推出物理重叠。系统与模型责任分开，不能把此疑点误归同一机理错误。
- 可读性仍有waker_cpu/wakee_target_cpu/wakeup_ts及旁路“席位/修向”术语。无活跃流被总时长强行截断的迹象；机器PASS不覆盖这些语义问题。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
