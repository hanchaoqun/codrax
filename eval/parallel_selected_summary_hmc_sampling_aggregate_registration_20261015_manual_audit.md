# HMC245 manual audit

## A — original sample-only binary: FAIL

Unchanged natural question and complete original `test_brbe.data`, 30,062,585 bytes, SHA-256 `da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`. Offline production preparation, actual CLI preflight (zero provider calls) and an independent reference-parser run agree: 12,000 samples overall; 1,393 in `[486489.4,486489.6)`; tid1609/poll_report has 164 records and 259 cycles weight.

The actual run prepared `.codrax/trace-input-b30deccb317748bb1c3c7a5f57804e80/capture.tracebundle.json` and queried complete members, not the attached preview. Kernel IP `0xffffff800b829d24` has 82 records / 174 weight on recorded CPU 1; `SensorCallbackProxy::OnDataEvent_(...)` has 82 / 85 on recorded CPU 0. Record shares are 50% / 50%; weight shares are 67.18% / 32.82%. Both CPU coordinates and these exact quantities are now correct in the final prose.

The final MD/HTML still fail:

- The 32-row callback preview ends at `486489.464680`; all 82 callbacks actually end at `486489.465612`. The answer calls the preview the complete callback population.
- It places that fragment at the start of the requested window and claims the following approximately 199.4 ms contain no samples. Callback records begin 64.088 ms after the window starts, and all 82 kernel samples occur later at `486489.588096–486489.589609`.
- All 164 records retain `sample_kind=unknown`. The prose nonetheless calls them CPU execution fragments and uses missing scheduler intervals to speculate about sleep/blocking. Correct appended quality notes and a no-chain-root-cause projection do not repair those statements.
- Hotspot prose is repeated and two similarly numbered quality sections repeat 164/259. The direct query is half-open while the automatic root context carries an inclusive endpoint, so equal numbers alone do not permit merging their populations. No optional native sampling fact selector is used in the final answer. Native number correctness does not establish complete answer correctness.

The analyzer naturally repaired its initially conflicting bounded/root-cause tuple to `causal_diagnosis`. The new aggregate-only synopsis replacement is intentionally inapplicable to causal obligations; this run does not establish a failure of that exception. The exploration completion reason contains a weight-to-CPU-time error, but the finalizer's delivered context does not replay an `Investigation Narrative` / `Current model synthesis` section: do not claim this run proved that specific summary-replay mechanism. Exact preview truncation and state caveats were disclosed; this is not a newly demonstrated native parser/counting failure, nor is one run enough to label the remaining errors random model variation.

Automatic causal supplementation and Trace projection remain active and correctly report no chain-supported root cause. Explicit model tool calls are two `trace_query` calls; system supplementation is separate. No failed sampling query followed by natural retry, no Mermaid graph and no native fact-selector diagram were triggered. They are not signed off by this case.

The system supplement also labels `edge_count=1` on this adjacent/data-gap item as “已证关系数：1”. This is recorded as a display-qualification issue requiring producer-semantic review, not a claim that the entire result contains no valid noncausal relationship and not a new CPU-computation failure. A missing selectable root cause alone cannot disprove every relationship. HTML text was audited; browser-pixel rendering was not exercised.

## B — natural controller read-only registration: bounded PASS

Question: “实现已修好，请运行保留的现有测试，确认返回值正确。不要再修改源码、测试、配置或依赖。” Real parent controller, default controller skill and planner read existing code/tests, register the original assertion and persist the plan. An independent child process restores production workflow state, uses a real controller, reruns the existing native test and reaches `all_batches_verified`.

The fixture prepares initial source delivery, original contract and old execution through public tools; verification uses public `RunTests`, not a verifier model. This is not full CLI, natural initial code authoring, cross-framework acceptance or a restored interactive REPL.

- Parent PID 71944; child PID 80748. New invocation `native:13b6c:18dea4735198ae58:1` differs from old `native:11908:18dea45f8a904860:1` and source-plan invocations.
- Source plan `plan-1792050431859015000-71944`; registered proof plan `plan-1792050509170098000-71944`. Actual `increment-result` assertion witness is required; prior passing execution alone does not sign it.
- Source, proof and overall workflow end verified. Intermediate unverified attempts remain in history; production typed normalization rejects premature finish without using raw reason text as proof.
- Child audit has empty git diff/status and unchanged HEAD `5a8e39a320efeafd45913cffe6a28fdb7b19d4f1`. Source/test hashes match retained originals; registration authorization is revoked after the proof workflow.
- Six unique context packs retain distinct scoped logical/artifact identities across memory, save/load and child restoration. Repeated persistence does not accumulate references.

One native test is executed four times across operator preparation/source reverification/fresh child, not four independent tests. Parent controller makes five model decisions, planner two model calls, restored child two decisions. `source-verify-report.json` is the latest source-plan reverification snapshot (`:3`), not the first source verification (`:2`) byte image; original terminal/execution receipts and logs distinguish them. Final persistent completion uses recorded verification facts, not an advisory model reason.

## Accounting

Stable task census remains **79 = 21 delivered + 58 open**, no duplicates. This batch delivers two production sub-capabilities (aggregate-vs-audit-preview receipts; recorded CPU plus independent count/weight facts) and one real-controller acceptance entry. Complete parent capabilities added: **0**. One grouped sampling-answer acceptance defect remains; do not add it again to the 58. Registration's previously untriggered natural controller path is now accepted within the boundary above; HMC-18.5 remains continuous coverage.

Final full-suite, frozen-input and archival receipts are recorded in central ledger §245. Raw evidence is local/ignored; this report and code/documentation are the pushed review surface.
