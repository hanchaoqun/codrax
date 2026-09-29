# HMC §194 双例人工审计

固定25203正式exit0，恰好2并行×1，无第三例追绿。生产revision `63c9e69fccec`，快照`.codrax/tmp/codrax-selected-20260929-043535`；末版4ac37132a仅改接缝测试，生产代码相同。机器1 PASS/1 TIMEOUT；人工通过0/2：一份完整答案FAIL，另一份超时未完成，不把中间草稿伪称最终答案。

| case | 机器 | 人工 | 总时间 | 结论 |
| --- | --- | --- | ---: | --- |
| trace_sqlite_wal_records | PASS | FAIL | 112s | WAL及统一SQL角色实际生效；摘要数量、未知名称和进程归属仍误述。 |
| read_combo_command_current_source_explanation | TIMEOUT | 未完成；过程已证系统缺陷 | 1201s | 标量统计被强制走语义清单与逐项成文，工具供给矛盾；389测量被200项展示清单挤出主位。 |

## 1. Trace输入、查询和实际上下文

自然问题/oracle未改：整理1.040–1.080秒的启动记录及同期系统事件，只附main文件，默认发现WAL。收据`.codrax/trace-input-1289999f71aad595159a686e3e594692/preparation.json`：main61,440字节、WAL8,272字节、CommitFrame2；镜像SHA=`3d7e3c502044604e79e2381017bea090110fc51a7674dafd1561a54a6ceca16a`。回放后main/WAL指纹与§193一致，分别`40b91431aec185ab73c9fe853779a80f84da7d2b68a1b848955856703b67c23e`、`278e9be77900e1bd1ffc40accb22af334042b458e71b18d7458b4001989ab731`。不代签在线持续写入或未提交磁盘尾覆盖。

两次trace_query均1.04–1.08窗、零源码读取。实际finalizer日志2018行有8个去重行：两启动记录4端点、4条系统事件。HiSys均cpu=-1/emitter_tid=0/emitter_tgid=0、source.tid=27599；已命中已知/未知名称统一角色修复，不再因名称完整补CPU0/TGID。

逐行字段完整：1.046s域APP_LAUNCH/事件UI_READY；1.050s域NULL未知/事件STAGE_READY；1.052s域APP_LAUNCH/事件名引用50未解析；1.058s域APP_LAUNCH/事件BOOTSTRAP、合法引用0。contents完整，未知身份状态未截断。两段启动保owner_pid27599、完整起止及各8ms。

预分析本次只发source/summary，没有错误duration/PID/signals；实际消息明确未验证元数据不作测量。不能宣称live重现了§192负控；自授权拒绝、错误数值不入实际finalizer及独立已证信号不丢失由公共agent正反回归证明。finalizer约59,287估算tokens，全链最大82,762/200,000=41%；不主张比前批58,850更省tokens。

## 2. Trace完整答案仍FAIL

两启动名称/起止/8ms/PID正确、表头单位完整，无图、无patch。系统事件表4行，开头却写5条，结构化aggregate已有value4。1.052s域APP_LAUNCH被当作“未解析事件名”的名称值；又称系统事件均来自同一进程，source.tid不独立证明进程映射。“注明未解析”不能抵消字段角色错配。

本轮没再写旧答案的重叠错误，旧FAIL不倒签。已精准供证后的数量/角色成文错误挂16.4/18.2，尚未证明纯模型波动，不加事件名或答案原文关键词硬门。重复“项目”列和内部术语属呈现债，非本次主要失败依据。

旁路`.codrax/output/20260929-043725.759-71905.root-causes.json`为schema2、空root_causes、trace_root_cause_contract_not_active；没有把同期事件升根因。适配器仍请求/首响应10分钟、静默5分钟；分析器另有既有3分钟terminal-emit预算，不能说所有阶段均无独立预算。本批不改等待/降级策略。

## 3. 只读统计＋源码解释：系统路由与义务冲突

问题未改，独立完整命令与模型实际命令均得389；运行期间Go生产文件未增删，200不是版本变化。

1. 分类同时给is_count_question/is_scalar_answer与source_inventory_profile.target_roles=[file]。首10轮仅repo_map和两个emit；缺typed局部scope又强制全仓根范围。repo_map拒绝纯file角色、推荐list_files，当前工具表却没有它。本次模型没尝试不可用list_files，因此机器unavailable_tool_attempts=0不能反证矛盾不存在。
2. 释放工具后已测到389，04:42:51提交总数及源码解释仍被要求完整principal member_set，12次完成尝试未直接闭合。模型进一步混淆aggregate_facts数目与成员数、误写awk语法，改交head -200样本。内部成员数一致性门不能放松；问题在于不必要的全成员义务和错选principal。

没有已接受成文：首稿开头“统计值200”，后文披露总数389/截断189，仍不是合格标量答案。三次patch转而抄200行、修行身份，源码链路解释被清单替换，第5轮请求尚在进行。48个explorer轮次、11次repo_map、11次read_file、2次list_files、4次成文拒绝、13次中途提示；上下文由初始finalizer59,635升至109,252/200,000=55%。

评测预设总时限1200秒使worker退出124，runner在1201秒收尾，补记partial_result=1/TIMEOUT。日志末尾仍有语义流活动，最后一轮仅约74秒；是评测总预算终止，不是产品按4分钟/首响应超时降级，未产生最终答案。原日志/过程保留，不追加第三例。只读运行无源码写入，runner终止后对应进程组无残留。

提升01.2/01.3/16.4/18.5为P1：按typed目标区分标量测量、路径枚举、语义清单、源码解释，统一路由/工具/范围/最终义务，复用原生命令证据，避免模型重抄。验收覆盖局部/全仓、混合说明、大小集合、范围改代、只读安全；不为389写特判，也不仅延时求绿。该路由不在本批生产修改范围，属相邻回放识别的现存缺口，尚未销账。

## 4. 原始凭证

| 工件 | SHA-256 |
| --- | --- |
| Trace完整日志 | `d6a7104b37ed9e015074701ab018ca3ed1d9bb3375bec55392070259dee7d142` |
| Trace最终正文 | `b0c98c429da305b683c1cc6d0b101fa33753eb45f440e3e7370cc5113e6cb655` |
| Trace准备收据 | `3db4b3c6df0617a91b099b9d1e31a9df096580fe79e3adcf0cf71490a17de0d0` |
| Trace根因旁路 | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |
| 只读完整日志 | `7be045beeab8043c1ab1424ec6c7c44209a67b22875ce91b3db7ebc73d94fd24` |
| 只读run-1.out（非最终答案） | `60e78b2728c80973809b40c3ddb5c2e42aaf036463006d3d11225242cebf4c94` |
| 只读partial metrics | `729e4e54fa6b73fac337cd7a3647ecaeffddef2aa24e79b05d126c6a06d3ab9c` |
| runner日志/tmp/codrax-hmc194-eval.log | `660c2444f54467582845f6f5fe6abe50b15d87f915df9ce2ce9169dc2bab7e5c` |

两个结果目录为eval/results/下`trace_sqlite_wal_records-20260929-043537`与`read_combo_command_current_source_explanation-20260929-043537`；日志各为run-1.logs/codrax-20260929-043538-000-71905.log和71891.log。小型机器原判/metrics/wall与Trace正文随批登记，大日志/过程及准备材料留本地并固定指纹。全仓/推送见主账本§194。累计完整能力16、开放63、本批子能力2、稳定验收父项5；另记一份答案FAIL和一份未完成，不把重跑次数冒充新任务。
