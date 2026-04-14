# Blob-file path leak into final answer

**Status**: **UNRESOLVED** — independent of P0/P1 ships; pre-existing
since the blob offload infrastructure landed in `3b3152a`
**Discovered**: 2026-04-14 during post-P1.1 grid validation
(`t4 run-5`, HEAD `5e695d7`)
**Related code**:
- `internal/orchestrator/orchestrator.go:165` (work-dir creation)
- `internal/tool/blob.go:80-188` (`StoreBlob` / `StoreBlobHeadOnly` —
  preview hint embeds the blob path so the LLM can page it)
- `internal/tool/builtin.go` `grep` / `read_file` (no sanitization
  of blob-file paths on output)
- `internal/agent/evidence.go` (evidence extraction — does not
  distinguish blob-file paths from repo paths)
**Related commit that introduced blob offload**: `3b3152a` "Offload
large tool outputs to per-trace blob files"
**Related memory**: `reference_openai_branch_commits.md`

---

## TL;DR (EN)

When a tool's output exceeds `blob_max_inline_bytes`, the pipeline
saves the full output to a per-trace temporary file under
`/tmp/codrax-<TraceID>-*/` and embeds that path into the tool's
preview hint ("Full content saved to `/tmp/codrax-trace-.../grep-
c793fbc9.txt`"). The LLM legitimately reads or greps that blob file
to recover the paged content — but the grep output lines arrive as
`/tmp/codrax-trace-.../grep-c793fbc9.txt:138:internal/orchestrator/
orchestrator_test.go:190:...`, a nested `path:line:path:line`
string where the OUTER path is the blob file and the INNER path is
the true repo file.

Downstream evidence extraction, grounding, and answer-symbol layers
do **not** recognise blob-file paths as "not a repo file." The outer
blob-file path wins because it is at column 0 and `filename:line`
pattern matchers accept it first. The finalizer then renders the
blob-file path as a legitimate cite in the final answer.

**Observed symptom** (`t4 run-5`, `5e695d7`):

```
• 如果没有可行的转换且步骤耗尽，会触发finalize以确保流程关闭
  (/tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt:860)。
...
• /tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt:860
  — 触发finalize的条件。
```

This path ceases to exist the moment the orchestrator's `defer
os.RemoveAll(workDir)` fires at end of run, so the cite is
unverifiable even immediately after the run ends.

## TL;DR (中文)

当工具输出超过 `blob_max_inline_bytes`，pipeline 把完整内容写到
`/tmp/codrax-<TraceID>-*/` 下的 blob 文件，并在 preview hint 中嵌入
该路径。LLM 合法地用 `read_file` / `grep` 去分页该 blob 文件 —
但 grep 返回的每行是 `/tmp/codrax-trace-.../grep-xxx.txt:138:
internal/orchestrator/orchestrator_test.go:190:...` 这样的嵌套
`path:line:path:line` 字符串，**外层**是 blob 文件路径，**内层**
才是真正的 repo 源文件。

下游 evidence / grounding / answer-symbol 层没有任何代码区分
"这是 blob 文件路径，不是仓库文件"。`filename:line` 模式匹配器
总是先匹到 col 0 的外层 blob 路径。finalizer 最终把 blob 路径当
合法 cite 渲染到答案里。

pipeline 结束时 `defer os.RemoveAll(workDir)` 会把这个临时目录
删除，所以即使在运行结束后立即检查，这个 cite 也无法验证。

---

## Evidence chain (t4 run-5)

Log: `eval/results/t4-20260414-081325/run-5.logs/codrax-*.log`.
trace: `1776154906697709871-1196883473`, task `t4`.

### Step 1 — Work dir created

```
line 11:  INFO [orchestrator] work dir: /tmp/codrax-trace-1776154906697709871-1196883473
```

From `internal/orchestrator/orchestrator.go:165`:

```go
if workDir, err := os.MkdirTemp("", "codrax-"+o.busCtx.TraceID+"-"); err != nil {
    logging.Warning(...)
} else {
    o.busCtx.WorkDir = workDir
    logging.Info("[orchestrator] work dir: %s", workDir)
    defer func() {
        if rmErr := os.RemoveAll(workDir); rmErr != nil { ... }
    }()
}
```

The path is intentionally opaque to the LLM — it is infrastructure,
not repo content.

### Step 2 — Large grep output offloaded to blob

An earlier iteration produced a grep output > `MaxInlineBytes`.
`StoreBlob` in `internal/tool/blob.go:93` writes full content to
`<WorkDir>/grep-<sha8>.txt` and returns a preview that **embeds the
path**:

```go
// internal/tool/blob.go:178
hint = fmt.Sprintf(
    "\n\n…[showing first %d of %d bytes (%d of %d lines). Full content saved to %s — call read_file with that path (use offset/limit to page) or grep it for specific patterns.]",
    headEnd, total, headLines, totalLines, ref,
)
```

`ref` is `/tmp/codrax-trace-1776154906697709871-1196883473/
grep-c793fbc9.txt`. The LLM is **instructed** to call
`read_file`/`grep` on this path. That design is fine by itself — it
lets the LLM page the remainder of a large tool output.

### Step 3 — Explorer reads the blob file

```
line 879:  DEBUG [diag explorer] iter=12 call[1] tool=read_file
  params={"limit":500,"offset":0,
          "path":"/tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt"}
```

The read_file output banner then shows the blob-file path as a
quoted header:

```
line 918:  [/tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt: showing lines 1-280 of 881 total]
```

Still fine — the LLM is paging the blob.

### Step 4 — Explorer greps the blob file → nested `path:line` format

```
line 956:  DEBUG [diag explorer] iter=14 call[0] tool=grep
  params={"context_lines":3,"file_type":"go","files_only":false,
          "path":"/tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt",
          "pattern":"dispatch|stage|transition|priority"}
```

The grep tool produces output with the blob-file path as the match's
"filename" column:

```
line 962:  /tmp/codrax-trace-.../grep-c793fbc9.txt:138:internal/orchestrator/orchestrator_test.go:190:...
line 964:  /tmp/codrax-trace-.../grep-c793fbc9.txt:140:internal/orchestrator/orchestrator_test.go:192:...
```

**This is the leak**. A standard `grep <pattern> <path>` returns
`<path>:<line>:<content>`. When `<path>` happens to be the blob
file, the outer column is `/tmp/codrax-trace-.../grep-c793fbc9.txt`.
The inner content lines START with `internal/orchestrator/
orchestrator_test.go:190:` because the blob itself stored an earlier
grep result — so the grep output is literally a `path:line:
path:line:content` chain.

### Step 5 — Evidence extraction picks the OUTER path

The evidence-item parser and grounder use leading-column `FILE:LINE`
extraction. The outer `/tmp/codrax-trace-.../grep-c793fbc9.txt:138`
wins the match because there is no blob-file-path filter anywhere in
`internal/agent/evidence.go` or the explorer's cross-reference
builder. The inner `orchestrator_test.go:190` is the real content
but ends up buried inside the evidence string's "body" field, not
its source field.

### Step 6 — Finalizer renders the leaked path

The final answer from `run-5.out`:

```
• 如果没有可行的转换且步骤耗尽，会触发finalize以确保流程关闭
  (/tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt:860)。
...
• /tmp/codrax-trace-1776154906697709871-1196883473/grep-c793fbc9.txt:860
  — 触发finalize的条件。
```

`:860` is a synthesized line number; the blob file only has 881
lines and the original repo location could be anywhere inside them.
The cite is unverifiable the moment the orchestrator's
`defer os.RemoveAll(workDir)` fires.

---

## Why this is not a P0/P1.1 regression

`3b3152a "Offload large tool outputs to per-trace blob files"`
introduced the blob infrastructure long before the 2026-04-14
P0/P1.1 work. Neither:

- `7e4ecad` (P0.1 `/ungrounded` tag) — touches only
  `formatEvidenceItems` rendering
- `41f4b61` (P0.2 runtime validators) — touches only `finalizer.go`
  and `finalizer_validators.go`
- `5e695d7` (P1.1 emit_evidence tool) — default off, unregistered

...touches `blob.go`, the orchestrator work-dir path, or any
evidence-extraction code that could recognise a blob-file path.

The leak is simply the first time a grid run happened to chain
"grep too large → offload → LLM reads blob → LLM greps blob →
nested path" deep enough to surface the leaked path in the final
answer. The trip rate is governed by how many tokens the first
grep produced; at `blob_max_inline_bytes` default, this chain
likely fires on ≤5% of runs.

## Root cause statement

**The blob-file path is trusted input everywhere downstream of the
tool layer.** The blob preview hint tells the LLM "Full content
saved to `<path>`," the LLM legitimately invokes tools on that
path, grep returns the path as the filename column, and no code
anywhere between the tool output and the finalizer prompt asks
"is this path inside the repo, or is it my own blob cache?"
The finalizer therefore renders it indistinguishably from a real
repo cite.

Structurally, there are two leaks:

1. **Grep output is not sanitised.** `grep <blob>` produces
   `/tmp/codrax-.../xxx.txt:N:...` which contaminates every
   downstream column-0 path extraction.
2. **Evidence-extraction has no "is this a repo file" filter.**
   `internal/agent/evidence.go` and friends accept any
   `FILE:LINE` pattern, whether it points under the repo root or
   under `os.TempDir()`.

## Reproducibility

- **Rate across this grid**: 1 out of 35 runs (t4 run-5). All
  other t4 runs cite `internal/orchestrator/orchestrator.go:*`
  or `config/orchestrator.yaml:*` cleanly.
- **Triggering condition**: a pre-finalize grep or read_file
  whose output exceeds `blob_max_inline_bytes` AND the LLM
  subsequently re-greps the blob file (as opposed to just
  reading it).
- **Clean cases**: when the LLM uses `read_file` with
  offset/limit on the blob (the content preserves inner
  `FILE:LINE` at column 0, no outer column wrapping), the leak
  does NOT surface.

## Why the `defer RemoveAll` is a silent-fail amplifier

Line 171 of `orchestrator.go`:

```go
defer func() {
    if rmErr := os.RemoveAll(workDir); rmErr != nil {
        logging.Warning("[orchestrator] work dir cleanup failed: %v", rmErr)
    }
}()
```

By the time the user sees the answer, the blob file is gone.
A user or reviewer clicking the cite to verify it will see
"file not found." The leak is therefore self-erasing — it
produces a confident-looking cite that cannot be disproven
until a future run happens to regurgitate the same symptom.
This is the opposite of the `feedback_honesty_over_cleverness`
contract.

---

## Fix shape (NOT landing this session)

Per the roadmap guardrails in
`docs/architecture-root-cause-remediation.md` §7, this memo is
**documentation-only**. The class is newly established (N=1 so
far), so by the over-fitting-audit rule we wait for N=2 before
generalising. A second occurrence on a later run would promote
the class.

When the fix is designed, consider:

1. **Sanitise grep / read_file output at the tool boundary.**
   Prefix-strip the blob-file path so downstream sees the
   inner `FILE:LINE:content` directly. Lowest blast radius.
2. **Add a repo-root check to `extractEvidenceFromLine`.** Any
   `FILE:LINE` whose FILE does not resolve to an actual file
   under the repo root gets demoted or dropped. Broadest coverage,
   touches the evidence layer.
3. **Prepend a visible `[BLOB]` marker to blob-file paths in the
   preview hint.** The hint becomes `Full content saved to
   [BLOB] /tmp/... — use read_file (offset/limit), NOT grep`.
   Smallest code change, but relies on LLM discipline.
4. **Remove the blob-file preview path entirely** and force
   re-paging via a cache-handle token. Biggest refactor, most
   robust.

Do NOT:

- Remove the blob offload — large tool outputs would explode the
  context budget.
- Make the blob work-dir persistent — the `defer RemoveAll`ist here
  on purpose, to bound disk usage.
- Hard-code `/tmp/codrax-trace-` string match as a filter — path
  prefix may drift across Linux/macOS/Windows, and `os.TempDir()`
  can resolve to other locations under sandbox. Use a structural
  predicate ("is this path equal to or under `ctx.WorkDir`")
  instead.

## Action items

- [ ] Watch for a second occurrence. On N=2, promote this to a
      shipped-in-P2.x fix with an over-fitting audit on the
      predicate.
- [ ] Add a grid probe case that deliberately produces a
      >MaxInlineBytes grep so the leak condition can be
      triggered on demand (currently only luck fires it).
- [ ] Log a WARNING when the finalizer answer string contains
      `ctx.WorkDir` as a substring — this gives an honest
      fail-loud signal even before the underlying fix lands.

## Historical references

- `3b3152a` — blob offload introduction commit
- `internal/tool/blob.go` — the current offload contract
- `feedback_honesty_over_cleverness.md` — the contract this bug
  violates (silently substituting a plausible cite)
- `feedback_first_principles_root_cause.md` — this is NOT "LLM
  randomness," it is a structural lack of a repo-root predicate
- `docs/bug-extractanswersymbols-enumeration-completeness-gap.md`
  — the sibling UNRESOLVED memo from the same grid validation
  session
