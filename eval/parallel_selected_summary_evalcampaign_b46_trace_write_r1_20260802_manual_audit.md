# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T18:42:35Z
- sweep_start_ts: 20260802-114235
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | eval/results/real_trace_h5_smr_multirow_disposition-20260802-114235 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 218s | 42 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 显式用户窗、自动补采、两维结论、根因榜、唤醒链、窗内可消除量和完整 Trace 因果投影均在；runner 只因模型正文没有固定词形“等待对象 dma_fence_default_w”失败，而 typed projection/校验事实已多次携带该内核调用点。真正的人工失败是模型把 `fix_direction=frequency_thermal` 这个修向桶升级成“受频率热节流”，同一上下文的频率边界却明确禁止由策略上限/非最高频单独证明热节流。上下文材料足量，但 remedy bucket 与 mechanism authority 的角色提示不够尖锐。 |
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-114235 | log_regex,write_apply,write_patch_oracle | none | 331s | 19 | read=8,repo_map=3,list=2,trace=0,source_lens=1 | midloop=3,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 补丁本身完全正确且只改 `python-sdk/memoclaw/client.py`：sync/async 均为 `POST /v1/search` + JSON body，旧路由/urlencode 删除；两个行为探针和独立 `make check` 人工复跑都通过。系统却终态 `unverified`：typed TestSurface 已发现 `make@.` 且其精确 declared roster 含变更文件，但 plan-touched Python 偏好合成 pytest，并被同目录去重阻止 Make 入队；pytest 实际没有独立断言，报告退化为 probe-only，又因 probes 缺 `contract_refs` 产生 10 个 proof missing。属于跨语言 meta-runner 选择假阴性，不是 Python 补丁失败。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 逐案人工结论

### 1. 显式窗口 Trace

- 非回归：请求的 `13762.791708..13763.024898` 始终是主窗；6 次 `trace_query` 都带同一窗和 PID 17267。最终先由模型给出三块总结，再追加明确标注的系统确定性 projection；系统没有删除或替换模型正文。
- 两个根因维度都存在：模型先给规则计价的可消除榜，又给“主要时间占用/关键路径候选”；projection 中保留业务 span 的次数/单次最大/合计，以及供给、调度、锁优先级、IO 等现规则修向。
- typed 事实足量：`CompThread_0-2955` 的 `dma_fence_default_w` 同时出现在投影树、明细和系统校验事实。旧 oracle 要求固定自然语言前缀“等待对象”过硬；不得据此增加答案原文 regex 门或系统替写句子。
- 上下文精度 gap：`fix_direction=frequency_thermal` 是修复方向 registry 的桶名，系统 rank handoff 只说它代表 compute-delivery head-room；但 explore/finalizer guidance 没有逐字禁止把桶名当成已证 thermal mechanism。模型因而写出“频率热节流折算”，与同一 prompt 的“策略上下限不能单独证明热节流”冲突。
- 另有模型波动：模型没把已提供的 fence 调用点提升到前置总结，并把 running wall-clock 说成“包含等待间隔”。先观察跨例复现；不扫描最终 prose 做 hard gate。

### 2. 多仓 Python write

- 代码与 API Reference 对齐，diff 只有一个生产文件；sync/async 两个独立行为探针均执行成功，fixture 的 `make check` 人工复跑成功。
- `BuildTestSurface` 已正确发布 `make@.`、`target=check`、`has_test_signal=true`，并从 Make recipe/test script 建出 `declared_coverage_paths=[memoclaw/client.py, tests/check_search_client.py]`。
- `preferred_runner=python` 后，默认队列按 runner family 查不到兼容 Make，就合成根目录 pytest；同目录 `seenDirs` 随后丢掉真实 Make。pytest 不存在时只有 probe 结果，`reportPassedOnlyByVerificationProbes=true`，未填写的六个 hard contract refs 与四个 fallback refs 全进入 missing，累计审查最终标成 `verification_proof_incomplete`。
- 泛化修点不是给 Python/该文件加例外：当 repository-declared test candidate 的 bounded exact roster 覆盖全部 recognized changed source paths 时，该 candidate 应作为 selection priority 先于 extension-inferred synthetic runner；roster 只参与选路，不直接铸行为/changed-path proof，既有 fail-closed 权限不放宽。
- probes 未填写 `contract_refs` 仍是 planner 质量问题；无独立项目测试时保持 unverified 是正确的。后续 proof-only batch 没有成功补写 metadata 的循环另记 watch，不用自动猜 probe 证明了哪些合同。
